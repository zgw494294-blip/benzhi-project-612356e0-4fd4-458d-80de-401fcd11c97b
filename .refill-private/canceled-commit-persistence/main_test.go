package canceledcommitpersistence

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"sonarqa/internal/application"
	"sonarqa/internal/domain"
	"sonarqa/internal/store"
)

func TestCanceledCommitDoesNotPersist(t *testing.T) {
	directory := t.TempDir()
	value, err := store.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	acceptance, err := domain.NewAcceptance(
		"acc-cancel", "CANCEL-001",
		domain.AreaBoundary{Points: []domain.Point{{Longitude: 120, Latitude: 30}, {Longitude: 121, Latitude: 30}, {Longitude: 121, Latitude: 31}}},
		"CGCS2000",
		domain.QualityThresholds{MaxCoverageGapRatio: 0.2, MaxEchoGapRatio: 0.1, MaxHeadingDeviation: 5, MinPositionConfidence: 0.9, MaxSideLobeNoise: 0.1},
		[]string{"L-1"}, "processor", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = value.Commit(ctx, application.CommitRequest{
		Acceptance: acceptance, ExpectedVersion: 0, Operation: "create:CANCEL-001",
		IdempotencyKey: "idempotency-cancel", Result: json.RawMessage(`{"acceptanceID":"acc-cancel"}`),
		Actor: "processor", OccurredAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if _, err := value.Load(context.Background(), "acc-cancel"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("canceled commit persisted aggregate: %v", err)
	}
}
