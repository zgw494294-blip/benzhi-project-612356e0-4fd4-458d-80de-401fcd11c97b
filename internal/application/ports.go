package application

import (
	"context"
	"encoding/json"
	"time"

	"sonarqa/internal/domain"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type CommitRequest struct {
	Acceptance      *domain.SurveyAcceptance
	ExpectedVersion int64
	Operation       string
	IdempotencyKey  string
	Result          json.RawMessage
	Actor           string
	OccurredAt      time.Time
}

type CommitResult struct {
	Result   json.RawMessage
	Replayed bool
}

type Repository interface {
	Load(ctx context.Context, id string) (*domain.SurveyAcceptance, error)
	FindIdempotent(ctx context.Context, operation, key string) (json.RawMessage, bool, error)
	Commit(ctx context.Context, request CommitRequest) (CommitResult, error)
	List(ctx context.Context) ([]*domain.SurveyAcceptance, error)
}

type AuditReader interface {
	Audit(ctx context.Context, acceptanceID string, cursor int64, limit int, operation string) ([]domain.AuditEvent, int64, error)
}

type IDGenerator interface {
	NewID(prefix string) string
}
