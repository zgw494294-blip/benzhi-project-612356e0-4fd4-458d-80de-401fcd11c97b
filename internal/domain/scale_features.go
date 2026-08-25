package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type RevisionMetricDelta struct {
	Metric string  `json:"metric"`
	Delta  float64 `json:"delta"`
	Trend  string  `json:"trend"`
}

type RevisionHistoryItem struct {
	SonarLineRevision
	Deltas []RevisionMetricDelta `json:"deltas,omitempty"`
}

func metricTrend(delta float64, lowerIsBetter bool) string {
	if delta == 0 {
		return "unchanged"
	}
	if lowerIsBetter {
		if delta < 0 {
			return "improved"
		}
		return "worsened"
	}
	if delta > 0 {
		return "improved"
	}
	return "worsened"
}

func (a *SurveyAcceptance) RevisionHistory(lineID, revisionID string) ([]RevisionHistoryItem, error) {
	if !a.hasLine(lineID) {
		return nil, NewError("LINE_NOT_FOUND", "测线不存在")
	}
	items := make([]SonarLineRevision, 0)
	for _, revision := range a.Revisions {
		if revision.LineID == lineID {
			items = append(items, revision)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Sequence < items[j].Sequence })
	for i, item := range items {
		if item.Sequence != i+1 {
			return nil, NewError("REVISION_HISTORY_INVALID", "测线修订序号不连续")
		}
	}
	result := make([]RevisionHistoryItem, 0, len(items))
	for i, revision := range items {
		if revisionID != "" && revision.RevisionID != revisionID {
			continue
		}
		item := RevisionHistoryItem{SonarLineRevision: revision}
		if i > 0 {
			prev := items[i-1]
			item.Deltas = []RevisionMetricDelta{
				{Metric: "coverageGapRatio", Delta: coverageGap(revision) - coverageGap(prev), Trend: metricTrend(coverageGap(revision)-coverageGap(prev), true)},
				{Metric: "echoGapRatio", Delta: revision.EchoGapRatio - prev.EchoGapRatio, Trend: metricTrend(revision.EchoGapRatio-prev.EchoGapRatio, true)},
				{Metric: "headingDeviation", Delta: revision.HeadingDeviation - prev.HeadingDeviation, Trend: metricTrend(revision.HeadingDeviation-prev.HeadingDeviation, true)},
				{Metric: "positionConfidence", Delta: revision.PositionConfidence - prev.PositionConfidence, Trend: metricTrend(revision.PositionConfidence-prev.PositionConfidence, false)},
				{Metric: "sideLobeNoise", Delta: revision.SideLobeNoise - prev.SideLobeNoise, Trend: metricTrend(revision.SideLobeNoise-prev.SideLobeNoise, true)},
			}
		}
		result = append(result, item)
	}
	if revisionID != "" && len(result) == 0 {
		return nil, NewError("REVISION_NOT_FOUND", "修订不存在")
	}
	return result, nil
}

func coverageGap(revision SonarLineRevision) float64 {
	if len(revision.CoverageSamples) == 0 {
		return 0
	}
	missing := 0
	for _, sample := range revision.CoverageSamples {
		if !sample.Covered {
			missing++
		}
	}
	return float64(missing) / float64(len(revision.CoverageSamples))
}

type RuleSummary struct {
	RuleCode   string    `json:"ruleCode"`
	Passed     int       `json:"passed"`
	Blocking   int       `json:"blocking"`
	Observed   []float64 `json:"observed"`
	Thresholds []float64 `json:"thresholds"`
	LineIDs    []string  `json:"lineIDs"`
}
type AssessmentSummary struct {
	AssessmentID   string        `json:"assessmentID"`
	RuleSetVersion string        `json:"ruleSetVersion"`
	Rules          []RuleSummary `json:"rules"`
	BlockingCount  int           `json:"blockingCount"`
	SummaryHash    string        `json:"summaryHash"`
	Verification   string        `json:"verification"`
}

func (a *SurveyAcceptance) AssessmentSummary(id, lineID string, blockingOnly bool) (AssessmentSummary, error) {
	var assessment *QualityAssessment
	if id == "" {
		assessment = a.LatestAssessment()
	} else {
		for i := range a.Assessments {
			if a.Assessments[i].AssessmentID == id {
				assessment = &a.Assessments[i]
				break
			}
		}
	}
	if assessment == nil {
		return AssessmentSummary{}, NewError("ASSESSMENT_NOT_FOUND", "评估不存在")
	}
	if lineID != "" {
		if _, ok := assessment.RevisionRefs[lineID]; !ok {
			return AssessmentSummary{}, NewError("LINE_NOT_FOUND", "评估中不存在指定测线")
		}
	}
	groups := map[string]*RuleSummary{}
	for _, outcome := range assessment.RuleOutcomes {
		if lineID != "" && outcome.LineID != lineID {
			continue
		}
		if blockingOnly && outcome.Passed {
			continue
		}
		g := groups[outcome.RuleCode]
		if g == nil {
			g = &RuleSummary{RuleCode: outcome.RuleCode}
			groups[outcome.RuleCode] = g
		}
		g.Observed = append(g.Observed, outcome.Observed)
		g.Thresholds = append(g.Thresholds, outcome.Threshold)
		g.LineIDs = append(g.LineIDs, outcome.LineID)
		if outcome.Passed {
			g.Passed++
		} else {
			g.Blocking++
		}
	}
	codes := make([]string, 0, len(groups))
	for code := range groups {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	rules := make([]RuleSummary, 0, len(codes))
	for _, code := range codes {
		rules = append(rules, *groups[code])
	}
	return AssessmentSummary{AssessmentID: assessment.AssessmentID, RuleSetVersion: assessment.RuleSetVersion, Rules: rules, BlockingCount: assessment.BlockingCount, SummaryHash: assessment.SummaryHash, Verification: "consistent"}, nil
}

func (a *SurveyAcceptance) VerifyAssessment(id string) (string, error) {
	var assessment *QualityAssessment
	if id == "" {
		assessment = a.LatestAssessment()
	} else {
		for i := range a.Assessments {
			if a.Assessments[i].AssessmentID == id {
				assessment = &a.Assessments[i]
				break
			}
		}
	}
	if assessment == nil {
		return "", NewError("ASSESSMENT_NOT_FOUND", "评估不存在")
	}
	if assessment.RuleSetVersion != CurrentRuleSetVersion {
		return "rule_version_mismatch", NewError("ASSESSMENT_RULE_VERSION_MISMATCH", "评估规则版本不匹配")
	}
	for line, ref := range assessment.RevisionRefs {
		revision, ok := a.revisionByID(ref)
		if !ok || revision.LineID != line {
			return "stale_reference", NewError("ASSESSMENT_STALE", "评估修订引用已过期")
		}
		if latest, exists := a.LatestRevision(line); !exists || latest.RevisionID != ref {
			return "stale_reference", NewError("ASSESSMENT_STALE", "评估修订引用已过期")
		}
	}
	hash, err := AssessmentHash(a.ID, assessment.RevisionRefs, assessment.RuleSetVersion, assessment.RuleOutcomes)
	if err != nil {
		return "", err
	}
	if hash != assessment.SummaryHash {
		return "inconsistent", NewError("ASSESSMENT_INCONSISTENT", "评估摘要哈希不一致")
	}
	return "consistent", nil
}

type RemediationEvidence struct {
	FindingID          string `json:"findingID"`
	Cause              string `json:"cause"`
	Remediation        string `json:"remediation"`
	EvidenceRevisionID string `json:"evidenceRevisionID"`
}

func (a *SurveyAcceptance) RemediateFindingsBatch(items []RemediationEvidence, actor string) error {
	if err := a.ensureMutable(); err != nil {
		return err
	}
	if a.Status != StatusReviewing && a.Status != StatusRework {
		return ErrStateConflict
	}
	if len(items) == 0 {
		return FieldError("items", "整改条目不能为空")
	}
	if strings.TrimSpace(actor) == "" {
		return FieldError("actor", "处理员不能为空")
	}
	seen := map[string]bool{}
	for i, item := range items {
		if seen[item.FindingID] {
			return FieldError(fmt.Sprintf("items[%d].findingID", i), "异常编号不得重复")
		}
		seen[item.FindingID] = true
		var finding *QualityFinding
		for j := range a.Findings {
			if a.Findings[j].FindingID == item.FindingID {
				finding = &a.Findings[j]
				break
			}
		}
		if finding == nil {
			return &Error{Code: "FINDING_NOT_FOUND", Message: "质量异常不存在", Field: fmt.Sprintf("items[%d].findingID", i)}
		}
		if strings.TrimSpace(item.Cause) == "" || strings.TrimSpace(item.Remediation) == "" {
			return FieldError(fmt.Sprintf("items[%d]", i), "原因和处置说明不能为空")
		}
		rev, ok := a.revisionByID(item.EvidenceRevisionID)
		if !ok {
			return FieldError(fmt.Sprintf("items[%d].evidenceRevisionID", i), "证据修订不存在")
		}
		if rev.LineID != finding.LineID {
			return FieldError(fmt.Sprintf("items[%d].evidenceRevisionID", i), "证据修订必须属于异常测线")
		}
		oldID := a.findingAssessmentRevision(*finding)
		if oldID == "" {
			return &Error{Code: "DATA_INVALID", Message: "异常缺少原评估修订引用", Field: fmt.Sprintf("items[%d].findingID", i)}
		}
		if old, ok := a.revisionByID(oldID); ok && rev.Sequence <= old.Sequence {
			return FieldError(fmt.Sprintf("items[%d].evidenceRevisionID", i), "异常证据必须引用更新的修订")
		}
		if finding.Status == FindingApproved || finding.Status == FindingSuperseded {
			return ErrStateConflict
		}
	}
	for _, item := range items {
		for i := range a.Findings {
			if a.Findings[i].FindingID == item.FindingID {
				f := &a.Findings[i]
				f.Cause = strings.TrimSpace(item.Cause)
				f.Remediation = strings.TrimSpace(item.Remediation)
				f.EvidenceRevisionID = item.EvidenceRevisionID
				f.RemediatedBy = strings.TrimSpace(actor)
				f.Status = FindingEvidenceSubmitted
				f.Reviewer = ""
				f.ReviewNote = ""
				f.ReviewedAt = nil
			}
		}
	}
	a.Version++
	return nil
}

func (a *SurveyAcceptance) findingAssessmentRevision(finding QualityFinding) string {
	for _, assessment := range a.Assessments {
		if !assessment.EvaluatedAt.Equal(finding.CreatedAt) {
			continue
		}
		for _, outcome := range assessment.RuleOutcomes {
			if !outcome.Passed && outcome.LineID == finding.LineID && outcome.RuleCode == finding.RuleCode {
				return assessment.RevisionRefs[finding.LineID]
			}
		}
	}
	return ""
}

type ReviewWorkbenchItem struct {
	QualityFinding
	EvidenceRevision *SonarLineRevision `json:"evidenceRevision,omitempty"`
}
type ReviewWorkbench struct {
	CreatedBy      string                `json:"createdBy"`
	ReadyForReview []ReviewWorkbenchItem `json:"readyForReview"`
	Approved       []ReviewWorkbenchItem `json:"approved"`
	Rejected       []ReviewWorkbenchItem `json:"rejected"`
	Other          []ReviewWorkbenchItem `json:"other"`
	Completed      int                   `json:"completed"`
	Remaining      int                   `json:"remaining"`
	Eligible       bool                  `json:"eligible"`
}

func (a *SurveyAcceptance) BuildReviewWorkbench() (ReviewWorkbench, error) {
	if a.Status != StatusReviewing {
		return ReviewWorkbench{}, ErrStateConflict
	}
	w := ReviewWorkbench{CreatedBy: a.CreatedBy}
	for _, f := range a.Findings {
		item := ReviewWorkbenchItem{QualityFinding: f}
		if f.EvidenceRevisionID != "" {
			if rev, ok := a.revisionByID(f.EvidenceRevisionID); ok {
				r := rev
				item.EvidenceRevision = &r
			}
		}
		switch f.Status {
		case FindingReadyForReview:
			w.ReadyForReview = append(w.ReadyForReview, item)
		case FindingApproved:
			w.Approved = append(w.Approved, item)
		case FindingRejected:
			w.Rejected = append(w.Rejected, item)
		default:
			w.Other = append(w.Other, item)
		}
	}
	sortItems := func(items []ReviewWorkbenchItem) {
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].CreatedAt.Equal(items[j].CreatedAt) {
				if items[i].LineID == items[j].LineID {
					return items[i].FindingID < items[j].FindingID
				}
				return items[i].LineID < items[j].LineID
			}
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		})
	}
	sortItems(w.ReadyForReview)
	sortItems(w.Approved)
	sortItems(w.Rejected)
	sortItems(w.Other)
	w.Completed = len(w.Approved) + len(w.Rejected)
	w.Remaining = len(w.ReadyForReview)
	w.Eligible = w.Remaining == 0 && len(w.Other) == 0 && len(w.Rejected) == 0
	if latest := a.LatestAssessment(); latest == nil || latest.BlockingCount != 0 {
		w.Eligible = false
	}
	return w, nil
}

type AuditEvent struct {
	Sequence           int64     `json:"sequence"`
	Operation          string    `json:"operation"`
	Actor              string    `json:"actor"`
	OccurredAt         time.Time `json:"occurredAt"`
	AggregateVersion   int64     `json:"aggregateVersion"`
	PreviousHash       string    `json:"previousHash"`
	Hash               string    `json:"hash"`
	ManifestHash       string    `json:"manifestHash,omitempty"`
	VerificationDigest string    `json:"verificationDigest,omitempty"`
	FrozenBoundary     bool      `json:"frozenBoundary,omitempty"`
	ReleasedBoundary   bool      `json:"releasedBoundary,omitempty"`
}
