package query_view_alias_pollution_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"sonarqa/internal/application"
	"sonarqa/internal/domain"
	"sonarqa/internal/store"
)

func TestQueryViewMutationDoesNotPolluteStoredProjection(t *testing.T) {
	directory := t.TempDir()
	repository, err := store.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	acceptance, err := domain.NewAcceptance(
		"acc-alias", "ALIAS-001",
		domain.AreaBoundary{Points: []domain.Point{{Longitude: 120, Latitude: 30}, {Longitude: 121, Latitude: 30}, {Longitude: 121, Latitude: 31}}},
		"CGCS2000",
		domain.QualityThresholds{MaxCoverageGapRatio: 0.2, MaxEchoGapRatio: 0.1, MaxHeadingDeviation: 5, MinPositionConfidence: 0.9, MaxSideLobeNoise: 0.1},
		[]string{"L-1"}, "processor-1", now,
	)
	if err != nil {
		t.Fatal(err)
	}
	commit := func(state *domain.SurveyAcceptance, expected int64, operation, key string) {
		t.Helper()
		_, commitErr := repository.Commit(context.Background(), application.CommitRequest{
			Acceptance: state, ExpectedVersion: expected, Operation: operation,
			IdempotencyKey: key, Result: json.RawMessage(`{"ok":true}`),
			Actor: "processor-1", OccurredAt: now.Add(time.Duration(state.Version) * time.Minute),
		})
		if commitErr != nil {
			t.Fatal(commitErr)
		}
	}
	commit(acceptance, 0, "create:ALIAS-001", "alias-create-001")

	acceptance, err = repository.Load(context.Background(), "acc-alias")
	if err != nil {
		t.Fatal(err)
	}
	revision := domain.SonarLineRevision{
		RevisionID: "rev-alias-1", AcceptanceID: acceptance.ID, LineID: "L-1", Sequence: 1,
		CoverageSamples: []domain.CoverageSample{{AlongTrackMeter: 0, Covered: true}, {AlongTrackMeter: 100, Covered: true}, {AlongTrackMeter: 200, Covered: true}},
		EchoGapRatio:    0.02, HeadingDeviation: 1, PositionConfidence: 0.98, SideLobeNoise: 0.02,
		CalibrationRef: "cal-alias-1", SubmittedBy: "processor-1", SubmittedAt: now.Add(time.Minute),
	}
	if err := acceptance.SubmitRevision(revision); err != nil {
		t.Fatal(err)
	}
	commit(acceptance, 1, "submit-revision:acc-alias", "alias-revision-001")

	acceptance, err = repository.Load(context.Background(), "acc-alias")
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]string{"L-1": revision.RevisionID}
	outcomes := domain.EvaluateRules(revision, acceptance.QualityThresholds)
	domain.SortOutcomes(outcomes)
	summaryHash, err := domain.AssessmentHash(acceptance.ID, refs, domain.CurrentRuleSetVersion, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	assessment := domain.QualityAssessment{
		AssessmentID: "asm-alias-1", AcceptanceID: acceptance.ID, RevisionRefs: refs,
		RuleSetVersion: domain.CurrentRuleSetVersion, RuleOutcomes: outcomes,
		BlockingCount: 0, SummaryHash: summaryHash, EvaluatedAt: now.Add(2 * time.Minute),
	}
	if err := acceptance.ApplyAssessment(assessment, func() string { return "unused-finding" }); err != nil {
		t.Fatal(err)
	}
	commit(acceptance, 2, "evaluate:acc-alias", "alias-evaluate-001")

	service := application.NewService(repository, nil, nil)
	first, err := service.Get(context.Background(), "acc-alias")
	if err != nil {
		t.Fatal(err)
	}
	originalDescription := first.LatestAssessment.RuleOutcomes[0].Description
	first.LatestRevisions[0].CoverageSamples[0].Covered = false
	first.LatestAssessment.RevisionRefs["L-1"] = "forged-revision"
	first.LatestAssessment.RuleOutcomes[0].Description = "forged-outcome"

	live, err := service.Get(context.Background(), "acc-alias")
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	recovered := application.NewService(reopened, nil, nil)
	afterRestart, err := recovered.Get(context.Background(), "acc-alias")
	if err != nil {
		t.Fatal(err)
	}

	livePolluted := !live.LatestRevisions[0].CoverageSamples[0].Covered ||
		live.LatestAssessment.RevisionRefs["L-1"] != revision.RevisionID ||
		live.LatestAssessment.RuleOutcomes[0].Description != originalDescription
	restartRestored := afterRestart.LatestRevisions[0].CoverageSamples[0].Covered &&
		afterRestart.LatestAssessment.RevisionRefs["L-1"] == revision.RevisionID &&
		afterRestart.LatestAssessment.RuleOutcomes[0].Description == originalDescription
	if livePolluted && restartRestored {
		t.Fatalf("查询结果修改污染了未提交的内存投影，但重启后又从持久化状态恢复")
	}
	if livePolluted {
		t.Fatalf("查询结果修改污染了后续查询返回的存储投影")
	}
}
