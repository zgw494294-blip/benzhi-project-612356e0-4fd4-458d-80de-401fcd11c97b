package snapshot_projection_validation_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sonarqa/internal/application"
	"sonarqa/internal/domain"
	"sonarqa/internal/store"
)

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

func TestRecoveryRejectsInvalidSnapshotProjection(t *testing.T) {
	directory := t.TempDir()
	repository, err := store.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := domain.NewAcceptance(
		"acc-snapshot", "SNAPSHOT-001",
		domain.AreaBoundary{Points: []domain.Point{{Longitude: 120, Latitude: 30}, {Longitude: 121, Latitude: 30}, {Longitude: 121, Latitude: 31}}},
		"CGCS2000",
		domain.QualityThresholds{MaxCoverageGapRatio: 0.2, MaxEchoGapRatio: 0.1, MaxHeadingDeviation: 5, MinPositionConfidence: 0.9, MaxSideLobeNoise: 0.1},
		[]string{"L-1"}, "processor-1", time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.Commit(context.Background(), application.CommitRequest{
		Acceptance: aggregate, ExpectedVersion: 0, Operation: "create:SNAPSHOT-001",
		IdempotencyKey: "snapshot-create-001", Result: json.RawMessage(`{"acceptanceID":"acc-snapshot","version":1}`),
		Actor: "processor-1", OccurredAt: time.Date(2026, 8, 25, 8, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "snapshot.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot snapshotFile
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Aggregates[aggregate.ID].Status = domain.Status("Corrupted")
	digest, err := json.Marshal(snapshotHashInput{
		SchemaVersion: snapshot.SchemaVersion, EventSequence: snapshot.EventSequence,
		EventHash: snapshot.EventHash, Aggregates: snapshot.Aggregates,
		Idempotency: snapshot.Idempotency, SavedAt: snapshot.SavedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(digest)
	snapshot.Checksum = hex.EncodeToString(sum[:])
	data, err = json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Open(directory); err == nil {
		t.Fatal("TestRecoveryRejectsInvalidSnapshotProjection: 恢复接受了业务状态无效但 checksum 自洽的快照")
	}
}
