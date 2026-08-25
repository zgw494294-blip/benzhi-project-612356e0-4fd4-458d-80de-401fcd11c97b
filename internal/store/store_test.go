package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sonarqa/internal/application"
	"sonarqa/internal/domain"
)

func validAggregate(t *testing.T) *domain.SurveyAcceptance {
	t.Helper()
	value, err := domain.NewAcceptance(
		"acc-store", "STORE-001",
		domain.AreaBoundary{Points: []domain.Point{{Longitude: 120, Latitude: 30}, {Longitude: 121, Latitude: 30}, {Longitude: 121, Latitude: 31}}},
		"CGCS2000", domain.QualityThresholds{MaxCoverageGapRatio: 0.2, MaxEchoGapRatio: 0.1, MaxHeadingDeviation: 5, MinPositionConfidence: 0.9, MaxSideLobeNoise: 0.1},
		[]string{"L-1"}, "processor", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestRecoverAndReplayIdempotency(t *testing.T) {
	directory := t.TempDir()
	first, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	result := json.RawMessage(`{"acceptanceID":"acc-store","version":1}`)
	_, err = first.Commit(context.Background(), application.CommitRequest{Acceptance: validAggregate(t), ExpectedVersion: 0, Operation: "create:STORE-001", IdempotencyKey: "idempotency-001", Result: result, Actor: "processor", OccurredAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := second.Load(context.Background(), "acc-store")
	if err != nil || loaded.Version != 1 {
		t.Fatalf("recovery failed: %#v %v", loaded, err)
	}
	replayed, exists, err := second.FindIdempotent(context.Background(), "create:STORE-001", "idempotency-001")
	var originalValue, replayedValue map[string]any
	originalErr := json.Unmarshal(result, &originalValue)
	replayedErr := json.Unmarshal(replayed, &replayedValue)
	if err != nil || !exists || originalErr != nil || replayedErr != nil || originalValue["acceptanceID"] != replayedValue["acceptanceID"] || originalValue["version"] != replayedValue["version"] {
		t.Fatalf("idempotency recovery failed: %s %t %v", replayed, exists, err)
	}
}

func TestRejectIncompleteEventTail(t *testing.T) {
	directory := t.TempDir()
	value, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	_, err = value.Commit(context.Background(), application.CommitRequest{Acceptance: validAggregate(t), ExpectedVersion: 0, Operation: "create:STORE-001", IdempotencyKey: "idempotency-001", Result: json.RawMessage(`{}`), Actor: "processor", OccurredAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(directory, "events.jsonl"), os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"sequence":2`); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(directory); err == nil {
		t.Fatal("expected incomplete tail error")
	}
}

func TestAuditIsReadOnlyAndPaged(t *testing.T) {
	directory := t.TempDir()
	value, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	_, err = value.Commit(context.Background(), application.CommitRequest{Acceptance: validAggregate(t), ExpectedVersion: 0, Operation: "create:STORE-001", IdempotencyKey: "idempotency-audit", Result: json.RawMessage(`{}`), Actor: "processor", OccurredAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "events.jsonl")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	events, next, err := value.Audit(context.Background(), "acc-store", 0, 1, "create")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].AggregateVersion != 1 || next != 0 {
		t.Fatalf("unexpected audit page: %#v, %d", events, next)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("audit query changed event log")
	}
}
