package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"sonarqa/internal/application"
	"sonarqa/internal/domain"
)

type emptyRepository struct{}

func (emptyRepository) Load(context.Context, string) (*domain.SurveyAcceptance, error) {
	return nil, domain.ErrNotFound
}
func (emptyRepository) FindIdempotent(context.Context, string, string) (json.RawMessage, bool, error) {
	return nil, false, nil
}
func (emptyRepository) Commit(context.Context, application.CommitRequest) (application.CommitResult, error) {
	return application.CommitResult{}, nil
}
func (emptyRepository) List(context.Context) ([]*domain.SurveyAcceptance, error) { return nil, nil }

type fixedIDs struct{}

func (fixedIDs) NewID(prefix string) string { return prefix + "-fixed" }

func TestStrictJSONAndContentType(t *testing.T) {
	service := application.NewService(emptyRepository{}, application.SystemClock{}, fixedIDs{})
	handler := NewServer(service, nil).Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/acceptances", bytes.NewBufferString(`{"unknown":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/acceptances", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "text/plain")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("content type status = %d", response.Code)
	}
}
