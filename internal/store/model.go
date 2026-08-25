package store

import (
	"encoding/json"
	"time"

	"sonarqa/internal/domain"
)

const snapshotSchemaVersion = 1

type eventRecord struct {
	Sequence         int64                    `json:"sequence"`
	PreviousHash     string                   `json:"previousHash"`
	Hash             string                   `json:"hash"`
	Operation        string                   `json:"operation"`
	AcceptanceID     string                   `json:"acceptanceID"`
	AggregateVersion int64                    `json:"aggregateVersion"`
	Actor            string                   `json:"actor"`
	OccurredAt       time.Time                `json:"occurredAt"`
	IdempotencyKey   string                   `json:"idempotencyKey"`
	Result           json.RawMessage          `json:"result"`
	State            *domain.SurveyAcceptance `json:"state"`
}

type eventHashInput struct {
	Sequence         int64                    `json:"sequence"`
	PreviousHash     string                   `json:"previousHash"`
	Operation        string                   `json:"operation"`
	AcceptanceID     string                   `json:"acceptanceID"`
	AggregateVersion int64                    `json:"aggregateVersion"`
	Actor            string                   `json:"actor"`
	OccurredAt       time.Time                `json:"occurredAt"`
	IdempotencyKey   string                   `json:"idempotencyKey"`
	Result           json.RawMessage          `json:"result"`
	State            *domain.SurveyAcceptance `json:"state"`
}

type idempotentValue struct {
	Operation string          `json:"operation"`
	Key       string          `json:"key"`
	Result    json.RawMessage `json:"result"`
}

type snapshotFile struct {
	SchemaVersion int                                 `json:"schemaVersion"`
	EventSequence int64                               `json:"eventSequence"`
	EventHash     string                              `json:"eventHash"`
	Aggregates    map[string]*domain.SurveyAcceptance `json:"aggregates"`
	Idempotency   []idempotentValue                   `json:"idempotency"`
	SavedAt       time.Time                           `json:"savedAt"`
	Checksum      string                              `json:"checksum"`
}

type snapshotHashInput struct {
	SchemaVersion int                                 `json:"schemaVersion"`
	EventSequence int64                               `json:"eventSequence"`
	EventHash     string                              `json:"eventHash"`
	Aggregates    map[string]*domain.SurveyAcceptance `json:"aggregates"`
	Idempotency   []idempotentValue                   `json:"idempotency"`
	SavedAt       time.Time                           `json:"savedAt"`
}
