package domain

import (
	"testing"
	"time"
)

func TestRevisionHistoryAndAssessmentSummary(t *testing.T) {
	a := testAcceptance(t)
	first := testRevision("rev-1", 1, false)
	if err := a.SubmitRevision(first); err != nil {
		t.Fatal(err)
	}
	assessment := assessmentFor(t, a, "asm-1", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	if err := a.ApplyAssessment(assessment, func() string { return "finding-1" }); err != nil {
		t.Fatal(err)
	}
	summary, err := a.AssessmentSummary("asm-1", "L-1", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Rules) != 1 || summary.Rules[0].RuleCode != "ECHO_GAP" || summary.BlockingCount != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if result, err := a.VerifyAssessment("asm-1"); err != nil || result != "consistent" {
		t.Fatalf("verification = %q, %v", result, err)
	}
	second := testRevision("rev-2", 2, true)
	if err := a.SubmitRevision(second); err != nil {
		t.Fatal(err)
	}
	history, err := a.RevisionHistory("L-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || len(history[1].Deltas) != 5 || history[1].Deltas[1].Trend != "improved" {
		t.Fatalf("unexpected history: %#v", history)
	}
	if result, err := a.VerifyAssessment("asm-1"); ErrorCode(err) != "ASSESSMENT_STALE" || result != "stale_reference" {
		t.Fatalf("stale verification = %q, %v", result, err)
	}
}

func TestRemediateFindingsBatchIsAtomic(t *testing.T) {
	a := testAcceptance(t)
	first := testRevision("rev-1", 1, false)
	first.SideLobeNoise = 0.4
	if err := a.SubmitRevision(first); err != nil {
		t.Fatal(err)
	}
	ids := []string{"finding-echo", "finding-noise"}
	index := 0
	if err := a.ApplyAssessment(assessmentFor(t, a, "asm-1", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)), func() string { value := ids[index]; index++; return value }); err != nil {
		t.Fatal(err)
	}
	second := testRevision("rev-2", 2, true)
	if err := a.SubmitRevision(second); err != nil {
		t.Fatal(err)
	}
	version := a.Version
	bad := []RemediationEvidence{{FindingID: ids[0], Cause: "原因", Remediation: "重采", EvidenceRevisionID: "rev-2"}, {FindingID: ids[1], Cause: "原因", Remediation: "重采", EvidenceRevisionID: "missing"}}
	if err := a.RemediateFindingsBatch(bad, "processor-1"); err == nil {
		t.Fatal("expected batch validation error")
	}
	if a.Version != version || a.Findings[0].Status != FindingOpen || a.Findings[1].Status != FindingOpen {
		t.Fatalf("failed batch changed aggregate: %#v", a.Findings)
	}
	bad[1].EvidenceRevisionID = "rev-2"
	if err := a.RemediateFindingsBatch(bad, "processor-1"); err != nil {
		t.Fatal(err)
	}
	if a.Version != version+1 || a.Findings[0].Status != FindingEvidenceSubmitted || a.Findings[1].Status != FindingEvidenceSubmitted {
		t.Fatalf("successful batch not atomic: %#v", a.Findings)
	}
}
