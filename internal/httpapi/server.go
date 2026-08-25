package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sonarqa/internal/application"
	"sonarqa/internal/domain"
)

const maxRequestBytes = 1 << 20

type Server struct {
	service *application.Service
	logger  *log.Logger
}

func NewServer(service *application.Service, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	return &Server{service: service, logger: logger}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.Health)
	mux.HandleFunc("GET /api/v1/acceptances", s.ListAcceptances)
	mux.HandleFunc("POST /api/v1/acceptances", s.CreateAcceptance)
	mux.HandleFunc("GET /api/v1/acceptances/{id}", s.GetAcceptance)
	mux.HandleFunc("GET /api/v1/acceptances/{id}/lines/{lineID}/revisions", s.GetRevisionHistory)
	mux.HandleFunc("GET /api/v1/acceptances/{id}/lines/{lineID}/revisions/{revisionID}", s.GetRevisionHistory)
	mux.HandleFunc("GET /api/v1/acceptances/{id}/assessments", s.GetAssessmentSummary)
	mux.HandleFunc("GET /api/v1/acceptances/{id}/assessments/{assessmentID}", s.GetAssessmentSummary)
	mux.HandleFunc("GET /api/v1/acceptances/{id}/review-workbench", s.GetReviewWorkbench)
	mux.HandleFunc("GET /api/v1/acceptances/{id}/audit", s.GetAudit)
	mux.HandleFunc("POST /api/v1/acceptances/{id}/revisions", s.SubmitRevision)
	mux.HandleFunc("POST /api/v1/acceptances/{id}/assessments", s.Evaluate)
	mux.HandleFunc("POST /api/v1/acceptances/{id}/findings/{findingID}/remediation", s.RemediateFinding)
	mux.HandleFunc("POST /api/v1/acceptances/{id}/findings/remediation-batch", s.RemediateFindingsBatch)
	mux.HandleFunc("POST /api/v1/acceptances/{id}/findings/{findingID}/review", s.ReviewFinding)
	mux.HandleFunc("POST /api/v1/acceptances/{id}/review-decision", s.DecideReview)
	mux.HandleFunc("POST /api/v1/acceptances/{id}/freeze", s.Freeze)
	mux.HandleFunc("GET /api/v1/acceptances/{id}/manifest", s.GetManifest)
	mux.HandleFunc("POST /api/v1/acceptances/{id}/release", s.Release)
	mux.HandleFunc("GET /api/v1/acceptances/{id}/release", s.GetRelease)
	return requestLimit(mux)
}

func requestLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Body != nil {
			request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBytes)
		}
		next.ServeHTTP(writer, request)
	})
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Field   string `json:"field,omitempty"`
	} `json:"error"`
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := application.ErrorCode(err)
	message := err.Error()
	field := ""
	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		code, message, field = domainErr.Code, domainErr.Message, domainErr.Field
	}
	switch code {
	case "VALIDATION_FAILED", "ACTOR_REQUIRED", "INVALID_JSON":
		status = http.StatusBadRequest
	case "ROLE_FORBIDDEN":
		status = http.StatusForbidden
	case "UNSUPPORTED_MEDIA_TYPE":
		status = http.StatusUnsupportedMediaType
	case "NOT_FOUND", "FINDING_NOT_FOUND", "RELEASE_NOT_FOUND", "MANIFEST_NOT_FOUND", "LINE_NOT_FOUND", "REVISION_NOT_FOUND", "ASSESSMENT_NOT_FOUND":
		status = http.StatusNotFound
	case "VERSION_CONFLICT", "STATE_CONFLICT", "DATASET_FROZEN", "REVISION_EXISTS", "PROJECT_CODE_EXISTS", "QUALITY_BLOCKED", "LINES_INCOMPLETE", "EVIDENCE_INCOMPLETE", "FINDINGS_OPEN", "FINDING_NOT_READY", "REVIEW_REQUIRED", "FROZEN_REQUIRED", "RELEASE_EXISTS", "ASSESSMENT_REQUIRED", "ASSESSMENT_STALE", "INDEPENDENT_REVIEW_REQUIRED", "MANIFEST_TAMPERED", "RELEASE_TAMPERED", "ASSESSMENT_RULE_VERSION_MISMATCH", "ASSESSMENT_INCONSISTENT", "AUDIT_CHAIN_INVALID", "REVISION_HISTORY_INVALID", "DATA_INVALID":
		status = http.StatusConflict
	}
	response := errorResponse{}
	response.Error.Code, response.Error.Message, response.Error.Field = code, message, field
	writeJSON(writer, status, response)
}

func decodeJSON(request *http.Request, target any) error {
	if contentType := request.Header.Get("Content-Type"); contentType != "" && !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		return domain.NewError("UNSUPPORTED_MEDIA_TYPE", "请求 Content-Type 必须为 application/json")
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return domain.NewError("INVALID_JSON", "请求体不能为空")
		}
		return domain.NewError("INVALID_JSON", "请求 JSON 无效")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return domain.NewError("INVALID_JSON", "请求体只能包含一个 JSON 对象")
	}
	return nil
}

func actorFrom(request *http.Request) application.Actor {
	return application.Actor{ID: strings.TrimSpace(request.Header.Get("X-Actor-ID")), Role: strings.TrimSpace(request.Header.Get("X-Actor-Role"))}
}

func idemKey(request *http.Request) string {
	return strings.TrimSpace(request.Header.Get("Idempotency-Key"))
}

func pathID(request *http.Request) string { return request.PathValue("id") }

func (s *Server) Health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok", "service": "sonarqa"})
}

func (s *Server) ListAcceptances(writer http.ResponseWriter, request *http.Request) {
	actor := actorFrom(request)
	if strings.TrimSpace(actor.ID) == "" {
		writeError(writer, domain.NewError("ACTOR_REQUIRED", "必须提供操作者编号"))
		return
	}
	if actor.Role != application.RoleProcessor && actor.Role != application.RoleReviewer {
		writeError(writer, domain.ErrUnauthorized)
		return
	}
	query := request.URL.Query()
	page, err := positiveInt(query.Get("page"), 1, "page")
	if err != nil {
		writeError(writer, err)
		return
	}
	pageSize, err := positiveInt(query.Get("pageSize"), 20, "pageSize")
	if err != nil {
		writeError(writer, err)
		return
	}
	from, err := optionalTime(query.Get("createdFrom"), "createdFrom")
	if err != nil {
		writeError(writer, err)
		return
	}
	to, err := optionalTime(query.Get("createdTo"), "createdTo")
	if err != nil {
		writeError(writer, err)
		return
	}
	keyword := strings.TrimSpace(query.Get("projectCode"))
	if keyword == "" {
		keyword = strings.TrimSpace(query.Get("keyword"))
	}
	if keyword == "" {
		keyword = strings.TrimSpace(query.Get("taskID"))
	}
	items, err := s.service.ListQuery(request.Context(), application.ListQuery{Status: strings.TrimSpace(query.Get("status")), ProjectCode: keyword, CreatedFrom: from, CreatedTo: to, Page: page, PageSize: pageSize})
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, items)
}

func positiveInt(raw string, fallback int, field string) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, domain.FieldError(field, "必须为正整数")
	}
	return value, nil
}
func optionalTime(raw, field string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, domain.FieldError(field, "时间必须使用 RFC3339 格式")
	}
	value = value.UTC()
	return &value, nil
}
func strictBool(raw, field string, fallback bool) (bool, error) {
	if raw == "" {
		return fallback, nil
	}
	if raw == "true" {
		return true, nil
	}
	if raw == "false" {
		return false, nil
	}
	return false, domain.FieldError(field, "必须为 true 或 false")
}

func (s *Server) CreateAcceptance(writer http.ResponseWriter, request *http.Request) {
	var command application.CreateAcceptanceCommand
	if err := decodeJSON(request, &command); err != nil {
		writeError(writer, err)
		return
	}
	command.Actor, command.IdempotencyKey = actorFrom(request), idemKey(request)
	result, err := s.service.Create(request.Context(), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, result)
}

func (s *Server) GetAcceptance(writer http.ResponseWriter, request *http.Request) {
	view, err := s.service.Get(request.Context(), pathID(request))
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, view)
}

func (s *Server) GetRevisionHistory(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	page, err := positiveInt(query.Get("page"), 1, "page")
	if err != nil {
		writeError(writer, err)
		return
	}
	pageSize, err := positiveInt(query.Get("pageSize"), 20, "pageSize")
	if err != nil {
		writeError(writer, err)
		return
	}
	if pageSize > 100 {
		writeError(writer, domain.FieldError("pageSize", "每页数量不得超过 100"))
		return
	}
	revisionID := strings.TrimSpace(query.Get("revisionID"))
	if revisionID == "" {
		revisionID = request.PathValue("revisionID")
	}
	items, err := s.service.RevisionHistory(request.Context(), pathID(request), request.PathValue("lineID"), revisionID)
	if err != nil {
		writeError(writer, err)
		return
	}
	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": items[start:end], "total": total, "hasNext": end < total})
}

func (s *Server) GetAssessmentSummary(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	blocking, err := strictBool(query.Get("blockingOnly"), "blockingOnly", false)
	if err != nil {
		writeError(writer, err)
		return
	}
	assessmentID := strings.TrimSpace(query.Get("assessmentID"))
	if assessmentID == "" {
		assessmentID = request.PathValue("assessmentID")
	}
	summary, err := s.service.AssessmentSummary(request.Context(), pathID(request), assessmentID, strings.TrimSpace(query.Get("lineID")), blocking)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, summary)
}

func (s *Server) GetReviewWorkbench(writer http.ResponseWriter, request *http.Request) {
	actor := actorFrom(request)
	if err := actor.Require(application.RoleReviewer); err != nil {
		writeError(writer, err)
		return
	}
	query := request.URL.Query()
	status := query.Get("status")
	switch domain.FindingStatus(status) {
	case "", domain.FindingReadyForReview, domain.FindingApproved, domain.FindingRejected, domain.FindingOpen, domain.FindingEvidenceSubmitted, domain.FindingSuperseded:
	default:
		writeError(writer, domain.FieldError("status", "未知异常状态"))
		return
	}
	view, err := s.service.ReviewWorkbench(request.Context(), pathID(request))
	if err != nil {
		writeError(writer, err)
		return
	}
	if actor.ID == view.CreatedBy {
		writeError(writer, domain.ErrUnauthorized)
		return
	}
	for _, group := range [][]domain.ReviewWorkbenchItem{view.ReadyForReview, view.Approved, view.Rejected, view.Other} {
		for _, item := range group {
			if item.RemediatedBy != "" && actor.ID == item.RemediatedBy {
				writeError(writer, domain.ErrUnauthorized)
				return
			}
		}
	}
	lineID := strings.TrimSpace(query.Get("lineID"))
	filter := func(items []domain.ReviewWorkbenchItem) []domain.ReviewWorkbenchItem {
		result := make([]domain.ReviewWorkbenchItem, 0)
		for _, item := range items {
			if lineID != "" && item.LineID != lineID {
				continue
			}
			if status != "" && string(item.Status) != status {
				continue
			}
			result = append(result, item)
		}
		return result
	}
	view.ReadyForReview = filter(view.ReadyForReview)
	view.Approved = filter(view.Approved)
	view.Rejected = filter(view.Rejected)
	view.Other = filter(view.Other)
	writeJSON(writer, http.StatusOK, view)
}

func (s *Server) GetAudit(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	operation := strings.TrimSpace(query.Get("operation"))
	if operation != "" {
		switch operation {
		case "create", "submit-revision", "evaluate", "remediate-finding", "remediate-findings-batch", "review-finding", "review-decision", "freeze", "release":
		default:
			writeError(writer, domain.FieldError("operation", "未知操作类型"))
			return
		}
	}
	cursor := int64(0)
	if raw := query.Get("cursor"); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 0 {
			writeError(writer, domain.FieldError("cursor", "游标必须为非负整数"))
			return
		}
		cursor = value
	}
	limit, err := positiveInt(query.Get("limit"), 50, "limit")
	if err != nil {
		writeError(writer, err)
		return
	}
	result, err := s.service.Audit(request.Context(), pathID(request), cursor, limit, operation)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (s *Server) SubmitRevision(writer http.ResponseWriter, request *http.Request) {
	var command application.SubmitRevisionCommand
	if err := decodeJSON(request, &command); err != nil {
		writeError(writer, err)
		return
	}
	command.AcceptanceID, command.Actor, command.IdempotencyKey = pathID(request), actorFrom(request), idemKey(request)
	result, err := s.service.SubmitRevision(request.Context(), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, result)
}

func (s *Server) Evaluate(writer http.ResponseWriter, request *http.Request) {
	var command application.EvaluateCommand
	if err := decodeJSON(request, &command); err != nil {
		writeError(writer, err)
		return
	}
	command.AcceptanceID, command.Actor, command.IdempotencyKey = pathID(request), actorFrom(request), idemKey(request)
	result, err := s.service.Evaluate(request.Context(), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, result)
}

func (s *Server) RemediateFinding(writer http.ResponseWriter, request *http.Request) {
	var command application.RemediateFindingCommand
	if err := decodeJSON(request, &command); err != nil {
		writeError(writer, err)
		return
	}
	command.AcceptanceID, command.FindingID = pathID(request), request.PathValue("findingID")
	command.Actor, command.IdempotencyKey = actorFrom(request), idemKey(request)
	result, err := s.service.RemediateFinding(request.Context(), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (s *Server) RemediateFindingsBatch(writer http.ResponseWriter, request *http.Request) {
	var command application.RemediationBatchCommand
	if err := decodeJSON(request, &command); err != nil {
		writeError(writer, err)
		return
	}
	command.AcceptanceID, command.Actor, command.IdempotencyKey = pathID(request), actorFrom(request), idemKey(request)
	result, err := s.service.RemediateFindingsBatch(request.Context(), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (s *Server) ReviewFinding(writer http.ResponseWriter, request *http.Request) {
	var command application.ReviewFindingCommand
	if err := decodeJSON(request, &command); err != nil {
		writeError(writer, err)
		return
	}
	command.AcceptanceID, command.FindingID = pathID(request), request.PathValue("findingID")
	command.Actor, command.IdempotencyKey = actorFrom(request), idemKey(request)
	result, err := s.service.ReviewFinding(request.Context(), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (s *Server) DecideReview(writer http.ResponseWriter, request *http.Request) {
	var command application.DecideReviewCommand
	if err := decodeJSON(request, &command); err != nil {
		writeError(writer, err)
		return
	}
	command.AcceptanceID, command.Actor, command.IdempotencyKey = pathID(request), actorFrom(request), idemKey(request)
	result, err := s.service.DecideReview(request.Context(), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (s *Server) Freeze(writer http.ResponseWriter, request *http.Request) {
	var command application.FreezeCommand
	if err := decodeJSON(request, &command); err != nil {
		writeError(writer, err)
		return
	}
	command.AcceptanceID, command.Actor, command.IdempotencyKey = pathID(request), actorFrom(request), idemKey(request)
	result, err := s.service.Freeze(request.Context(), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (s *Server) GetManifest(writer http.ResponseWriter, request *http.Request) {
	manifest, err := s.service.Manifest(request.Context(), pathID(request))
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, manifest)
}

func (s *Server) Release(writer http.ResponseWriter, request *http.Request) {
	var command application.ReleaseCommand
	if err := decodeJSON(request, &command); err != nil {
		writeError(writer, err)
		return
	}
	command.AcceptanceID, command.Actor, command.IdempotencyKey = pathID(request), actorFrom(request), idemKey(request)
	result, err := s.service.Release(request.Context(), command)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, result)
}

func (s *Server) GetRelease(writer http.ResponseWriter, request *http.Request) {
	release, err := s.service.ReleaseCredential(request.Context(), pathID(request))
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, release)
}

func (s *Server) ListenAndServe(ctx context.Context, address string) error {
	if strings.TrimSpace(address) == "" {
		return fmt.Errorf("监听地址不能为空")
	}
	server := &http.Server{Addr: address, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
	closed := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			closed <- err
			return
		}
		closed <- nil
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return errors.Join(server.Shutdown(shutdownCtx), <-closed)
	case err := <-closed:
		return err
	}
}
