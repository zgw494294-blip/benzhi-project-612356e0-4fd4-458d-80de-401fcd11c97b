package application

import (
	"context"
	"sonarqa/internal/domain"
	"sort"
	"strings"
)

func (s *Service) ListQuery(ctx context.Context, query ListQuery) (ListResult, error) {
	if query.Page <= 0 {
		return ListResult{}, domain.FieldError("page", "页码必须为正整数")
	}
	if query.PageSize <= 0 || query.PageSize > 100 {
		return ListResult{}, domain.FieldError("pageSize", "每页数量必须在 1 到 100 之间")
	}
	switch domain.Status(query.Status) {
	case "", domain.StatusDraft, domain.StatusCollecting, domain.StatusReviewing, domain.StatusRework, domain.StatusFrozen, domain.StatusReleased:
	default:
		return ListResult{}, domain.FieldError("status", "未知任务状态")
	}
	if query.CreatedFrom != nil && query.CreatedTo != nil && query.CreatedFrom.After(*query.CreatedTo) {
		return ListResult{}, domain.FieldError("createdFrom", "创建时间范围无效")
	}
	values, err := s.repository.List(ctx)
	if err != nil {
		return ListResult{}, err
	}
	filtered := make([]*domain.SurveyAcceptance, 0)
	for _, a := range values {
		if query.Status != "" && a.Status != domain.Status(query.Status) {
			continue
		}
		if query.ProjectCode != "" && !strings.Contains(strings.ToLower(a.ProjectCode), strings.ToLower(query.ProjectCode)) {
			continue
		}
		if query.CreatedFrom != nil && a.CreatedAt.Before(*query.CreatedFrom) {
			continue
		}
		if query.CreatedTo != nil && a.CreatedAt.After(*query.CreatedTo) {
			continue
		}
		filtered = append(filtered, a)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].CreatedAt.Equal(filtered[j].CreatedAt) {
			return filtered[i].ProjectCode < filtered[j].ProjectCode
		}
		return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
	})
	counts := map[domain.Status]int{domain.StatusDraft: 0, domain.StatusCollecting: 0, domain.StatusReviewing: 0, domain.StatusRework: 0, domain.StatusFrozen: 0, domain.StatusReleased: 0}
	for _, a := range filtered {
		if !validStatusForList(a.Status) {
			return ListResult{}, domain.NewError("DATA_INVALID", "任务状态或质量数据缺失")
		}
		counts[a.Status]++
	}
	total := len(filtered)
	start := (query.Page - 1) * query.PageSize
	if start > total {
		start = total
	}
	end := start + query.PageSize
	if end > total {
		end = total
	}
	items := make([]ListItem, 0, end-start)
	for _, a := range filtered[start:end] {
		v := BuildView(a)
		blocking := 0
		if a.LatestAssessment() != nil {
			blocking = a.LatestAssessment().BlockingCount
		}
		open := 0
		for _, f := range a.Findings {
			if f.Status != domain.FindingApproved && f.Status != domain.FindingSuperseded {
				open++
			}
		}
		items = append(items, ListItem{AcceptanceView: v, LatestAssessmentBlocking: blocking, OpenFindings: open, Released: a.Status == domain.StatusReleased})
	}
	return ListResult{Items: items, Total: total, HasNext: end < total, StatusCounts: counts}, nil
}
func validStatusForList(status domain.Status) bool {
	switch status {
	case domain.StatusDraft, domain.StatusCollecting, domain.StatusReviewing, domain.StatusRework, domain.StatusFrozen, domain.StatusReleased:
		return true
	}
	return false
}

func (s *Service) RevisionHistory(ctx context.Context, id, lineID, revisionID string) ([]domain.RevisionHistoryItem, error) {
	a, err := s.repository.Load(ctx, id)
	if err != nil {
		return nil, err
	}
	return a.RevisionHistory(lineID, revisionID)
}
func (s *Service) AssessmentSummary(ctx context.Context, id, assessmentID, lineID string, blockingOnly bool) (domain.AssessmentSummary, error) {
	a, err := s.repository.Load(ctx, id)
	if err != nil {
		return domain.AssessmentSummary{}, err
	}
	summary, err := a.AssessmentSummary(assessmentID, lineID, blockingOnly)
	if err != nil {
		return summary, err
	}
	verification, vErr := a.VerifyAssessment(assessmentID)
	if vErr != nil {
		summary.Verification = verification
		return summary, vErr
	}
	summary.Verification = verification
	return summary, nil
}
func (s *Service) ReviewWorkbench(ctx context.Context, id string) (domain.ReviewWorkbench, error) {
	a, err := s.repository.Load(ctx, id)
	if err != nil {
		return domain.ReviewWorkbench{}, err
	}
	return a.BuildReviewWorkbench()
}
func (s *Service) Audit(ctx context.Context, id string, cursor int64, limit int, operation string) (map[string]any, error) {
	reader, ok := s.repository.(AuditReader)
	if !ok {
		return nil, domain.NewError("AUDIT_UNAVAILABLE", "审计查询不可用")
	}
	if limit <= 0 || limit > 100 {
		return nil, domain.FieldError("limit", "条数必须在 1 到 100 之间")
	}
	events, next, err := reader.Audit(ctx, id, cursor, limit, operation)
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": events, "nextCursor": next, "verification": "verifiable"}, nil
}
