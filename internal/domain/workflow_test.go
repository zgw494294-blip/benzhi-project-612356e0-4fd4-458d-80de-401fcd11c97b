package domain

import (
	"fmt"
	"testing"
	"time"
)

func testAcceptance(t *testing.T) *SurveyAcceptance {
	t.Helper()
	value, err := NewAcceptance(
		"acc-1", "AREA-001",
		AreaBoundary{Points: []Point{{Longitude: 120, Latitude: 30}, {Longitude: 120.1, Latitude: 30}, {Longitude: 120.1, Latitude: 30.1}}},
		"CGCS2000",
		QualityThresholds{MaxCoverageGapRatio: 0.2, MaxEchoGapRatio: 0.1, MaxHeadingDeviation: 5, MinPositionConfidence: 0.9, MaxSideLobeNoise: 0.1},
		[]string{"L-1"}, "processor-1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func testRevision(id string, sequence int, passing bool) SonarLineRevision {
	revision := SonarLineRevision{
		RevisionID: id, AcceptanceID: "acc-1", LineID: "L-1", Sequence: sequence,
		CoverageSamples: []CoverageSample{{AlongTrackMeter: 0, Covered: true}, {AlongTrackMeter: 100, Covered: true}},
		EchoGapRatio:    0.02, HeadingDeviation: 1, PositionConfidence: 0.98, SideLobeNoise: 0.02,
		CalibrationRef: "CAL-1", SubmittedBy: "processor-1", SubmittedAt: time.Date(2026, 1, sequence, 0, 0, 0, 0, time.UTC),
	}
	if !passing {
		revision.EchoGapRatio = 0.4
	}
	return revision
}

func assessmentFor(t *testing.T, acceptance *SurveyAcceptance, id string, at time.Time) QualityAssessment {
	t.Helper()
	revision, ok := acceptance.LatestRevision("L-1")
	if !ok {
		t.Fatal("missing revision")
	}
	outcomes := EvaluateRules(revision, acceptance.QualityThresholds)
	SortOutcomes(outcomes)
	blocking := 0
	for _, outcome := range outcomes {
		if !outcome.Passed {
			blocking++
		}
	}
	refs := map[string]string{"L-1": revision.RevisionID}
	hash, err := AssessmentHash(acceptance.ID, refs, CurrentRuleSetVersion, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	return QualityAssessment{AssessmentID: id, AcceptanceID: acceptance.ID, RevisionRefs: refs, RuleSetVersion: CurrentRuleSetVersion, RuleOutcomes: outcomes, BlockingCount: blocking, SummaryHash: hash, EvaluatedAt: at}
}

func TestFindingReassessmentReviewFreezeAndRelease(t *testing.T) {
	acceptance := testAcceptance(t)
	if err := acceptance.SubmitRevision(testRevision("rev-1", 1, false)); err != nil {
		t.Fatal(err)
	}
	ids := 0
	if err := acceptance.ApplyAssessment(assessmentFor(t, acceptance, "asm-1", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)), func() string { ids++; return fmt.Sprintf("finding-%d", ids) }); err != nil {
		t.Fatal(err)
	}
	if len(acceptance.Findings) != 1 || acceptance.Findings[0].Status != FindingOpen {
		t.Fatalf("unexpected findings: %#v", acceptance.Findings)
	}
	if err := acceptance.SubmitRevision(testRevision("rev-2", 2, true)); err != nil {
		t.Fatal(err)
	}
	if err := acceptance.RemediateFinding("finding-1", "采集姿态突变", "重采异常区段", "rev-2", "processor-1"); err != nil {
		t.Fatal(err)
	}
	if err := acceptance.ApplyAssessment(assessmentFor(t, acceptance, "asm-2", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)), func() string { ids++; return fmt.Sprintf("finding-%d", ids) }); err != nil {
		t.Fatal(err)
	}
	if acceptance.Findings[0].Status != FindingReadyForReview {
		t.Fatalf("finding not ready: %s", acceptance.Findings[0].Status)
	}
	if err := acceptance.ReviewFinding("finding-1", "processor-1", "同意", true, time.Now()); ErrorCode(err) != "INDEPENDENT_REVIEW_REQUIRED" {
		t.Fatalf("expected independent review error, got %v", err)
	}
	if err := acceptance.ReviewFinding("finding-1", "reviewer-1", "证据有效", true, time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if err := acceptance.DecideReview("reviewer-1", "批准冻结", true); err != nil {
		t.Fatal(err)
	}
	manifest, err := acceptance.Freeze("reviewer-1", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.ManifestHash) != 64 || acceptance.Status != StatusFrozen {
		t.Fatalf("invalid manifest: %#v", manifest)
	}
	release, err := acceptance.IssueRelease("release-1", "archivist-1", time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyRelease(*release); err != nil {
		t.Fatal(err)
	}
	if err := acceptance.ValidateIntegrity(); err != nil {
		t.Fatal(err)
	}
}
