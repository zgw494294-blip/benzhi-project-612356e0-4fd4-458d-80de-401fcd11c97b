package audit_cursor_request_isolation_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"sonarqa/internal/application"
	"sonarqa/internal/domain"
	"sonarqa/internal/store"
)

func TestAuditCursorIsRequestScoped(t *testing.T) {
	t.Parallel()

	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	acceptance, err := domain.NewAcceptance(
		"acc-audit-isolation",
		"AUDIT-ISOLATION-001",
		domain.AreaBoundary{Points: []domain.Point{
			{Longitude: 120, Latitude: 30},
			{Longitude: 121, Latitude: 30},
			{Longitude: 121, Latitude: 31},
		}},
		"CGCS2000",
		domain.QualityThresholds{
			MaxCoverageGapRatio:   0.2,
			MaxEchoGapRatio:       0.1,
			MaxHeadingDeviation:   5,
			MinPositionConfidence: 0.9,
			MaxSideLobeNoise:      0.1,
		},
		[]string{"L-1"},
		"processor-a",
		createdAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	commit(t, repository, acceptance, 0, "create:AUDIT-ISOLATION-001", "audit-create-001", createdAt)

	loaded, err := repository.Load(context.Background(), acceptance.ID)
	if err != nil {
		t.Fatal(err)
	}
	err = loaded.SubmitRevision(domain.SonarLineRevision{
		RevisionID: "rev-audit-1", AcceptanceID: loaded.ID, LineID: "L-1", Sequence: 1,
		CoverageSamples: []domain.CoverageSample{{AlongTrackMeter: 0, Covered: true}, {AlongTrackMeter: 100, Covered: true}},
		EchoGapRatio:    0.02, HeadingDeviation: 1, PositionConfidence: 0.98, SideLobeNoise: 0.02,
		CalibrationRef: "cal-audit-1", SubmittedBy: "processor-a", SubmittedAt: createdAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	commit(t, repository, loaded, 1, "submit-revision:"+loaded.ID, "audit-revision-001", createdAt.Add(time.Minute))

	first, _, err := repository.Audit(context.Background(), acceptance.ID, 0, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := repository.Audit(context.Background(), acceptance.ID, 0, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].Sequence != 1 {
		t.Fatalf("首次 cursor=0 查询结果异常: %#v", first)
	}
	if len(second) != 1 || second[0].Sequence != 1 {
		t.Fatalf("第二个独立 cursor=0 请求没有从首条事件开始: %#v", second)
	}
}

func commit(t *testing.T, repository *store.Store, acceptance *domain.SurveyAcceptance, expected int64, operation, key string, occurredAt time.Time) {
	t.Helper()
	_, err := repository.Commit(context.Background(), application.CommitRequest{
		Acceptance: acceptance, ExpectedVersion: expected, Operation: operation,
		IdempotencyKey: key, Result: json.RawMessage(`{"ok":true}`), Actor: "processor-a", OccurredAt: occurredAt,
	})
	if err != nil {
		t.Fatal(err)
	}
}
