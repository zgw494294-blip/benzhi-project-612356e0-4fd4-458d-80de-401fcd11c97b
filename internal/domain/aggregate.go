package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func NewAcceptance(id, projectCode string, boundary AreaBoundary, coordinate string, thresholds QualityThresholds, lines []string, actor string, now time.Time) (*SurveyAcceptance, error) {
	if err := ValidateCreate(projectCode, boundary, coordinate, thresholds, lines, actor); err != nil {
		return nil, err
	}
	if strings.TrimSpace(id) == "" {
		return nil, FieldError("id", "验收任务 ID 不能为空")
	}
	copyLines := append([]string(nil), lines...)
	return &SurveyAcceptance{
		ID: id, ProjectCode: strings.TrimSpace(projectCode), AreaBoundary: boundary,
		CoordinateReference: strings.TrimSpace(coordinate), QualityThresholds: thresholds,
		PlannedLineIDs: copyLines, Status: StatusCollecting, Version: 1,
		CreatedBy: strings.TrimSpace(actor), CreatedAt: now.UTC(), Revisions: []SonarLineRevision{},
		Assessments: []QualityAssessment{}, Findings: []QualityFinding{},
	}, nil
}

func (a *SurveyAcceptance) ensureMutable() error {
	if a.Status == StatusFrozen || a.Status == StatusReleased {
		return ErrFrozen
	}
	return nil
}

func (a *SurveyAcceptance) hasLine(lineID string) bool {
	for _, planned := range a.PlannedLineIDs {
		if planned == lineID {
			return true
		}
	}
	return false
}

func (a *SurveyAcceptance) LatestRevision(lineID string) (SonarLineRevision, bool) {
	var latest SonarLineRevision
	found := false
	for _, revision := range a.Revisions {
		if revision.LineID == lineID && (!found || revision.Sequence > latest.Sequence) {
			latest, found = revision, true
		}
	}
	return latest, found
}

func (a *SurveyAcceptance) revisionByID(id string) (SonarLineRevision, bool) {
	for _, revision := range a.Revisions {
		if revision.RevisionID == id {
			return revision, true
		}
	}
	return SonarLineRevision{}, false
}

func (a *SurveyAcceptance) SubmitRevision(revision SonarLineRevision) error {
	if err := a.ensureMutable(); err != nil {
		return err
	}
	if a.Status != StatusCollecting && a.Status != StatusRework && a.Status != StatusReviewing {
		return ErrStateConflict
	}
	if !a.hasLine(revision.LineID) {
		return FieldError("lineID", "测线不在任务计划中")
	}
	if err := ValidateRevision(revision); err != nil {
		return err
	}
	if revision.AcceptanceID != a.ID {
		return FieldError("acceptanceID", "修订不属于当前验收任务")
	}
	for _, existing := range a.Revisions {
		if existing.RevisionID == revision.RevisionID {
			return NewError("REVISION_EXISTS", "修订编号已存在且不可覆盖")
		}
	}
	latest, exists := a.LatestRevision(revision.LineID)
	expectedSequence := 1
	if exists {
		expectedSequence = latest.Sequence + 1
	}
	if revision.Sequence != expectedSequence {
		return FieldError("sequence", fmt.Sprintf("修订序号应为 %d", expectedSequence))
	}
	a.Revisions = append(a.Revisions, revision)
	a.ReviewApproved = false
	a.FinalReviewer = ""
	a.Version++
	return nil
}

func (a *SurveyAcceptance) CanEvaluate() error {
	if err := a.ensureMutable(); err != nil {
		return err
	}
	if a.Status != StatusCollecting && a.Status != StatusReviewing && a.Status != StatusRework {
		return ErrStateConflict
	}
	for _, line := range a.PlannedLineIDs {
		if _, ok := a.LatestRevision(line); !ok {
			return NewError("LINES_INCOMPLETE", "所有计划测线均提交修订后才能评估")
		}
	}
	if a.Status == StatusReviewing && len(a.activeFindings()) > 0 {
		for _, finding := range a.activeFindings() {
			if finding.EvidenceRevisionID == "" {
				return NewError("EVIDENCE_INCOMPLETE", "所有阻断异常均提交新证据后才能复验")
			}
		}
	}
	return nil
}

func (a *SurveyAcceptance) ApplyAssessment(assessment QualityAssessment, idFactory func() string) error {
	if err := a.CanEvaluate(); err != nil {
		return err
	}
	if assessment.AcceptanceID != a.ID || assessment.RuleSetVersion != CurrentRuleSetVersion {
		return FieldError("assessment", "评估标识或规则版本无效")
	}
	a.Assessments = append(a.Assessments, assessment)
	failures := make(map[string]RuleOutcome)
	for _, outcome := range assessment.RuleOutcomes {
		if !outcome.Passed {
			failures[outcome.LineID+"\x00"+outcome.RuleCode] = outcome
		}
	}
	for index := range a.Findings {
		finding := &a.Findings[index]
		if finding.Status == FindingApproved || finding.Status == FindingSuperseded {
			continue
		}
		key := finding.LineID + "\x00" + finding.RuleCode
		if _, stillFails := failures[key]; stillFails {
			if finding.EvidenceRevisionID != "" {
				finding.Status = FindingRejected
			}
			delete(failures, key)
			continue
		}
		if finding.EvidenceRevisionID != "" {
			finding.Status = FindingReadyForReview
		} else {
			finding.Status = FindingSuperseded
		}
	}
	keys := make([]string, 0, len(failures))
	for key := range failures {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		outcome := failures[key]
		a.Findings = append(a.Findings, QualityFinding{
			FindingID: idFactory(), AcceptanceID: a.ID, LineID: outcome.LineID,
			RuleCode: outcome.RuleCode, Severity: "Blocking", Status: FindingOpen,
			CreatedAt: assessment.EvaluatedAt,
		})
	}
	a.Status = StatusReviewing
	a.ReviewApproved = false
	a.FinalReviewer = ""
	a.Version++
	return nil
}

func (a *SurveyAcceptance) activeFindings() []QualityFinding {
	result := make([]QualityFinding, 0)
	for _, finding := range a.Findings {
		if finding.Status != FindingApproved && finding.Status != FindingSuperseded {
			result = append(result, finding)
		}
	}
	return result
}

func (a *SurveyAcceptance) RemediateFinding(findingID, cause, remediation, evidenceRevisionID, actor string) error {
	if err := a.ensureMutable(); err != nil {
		return err
	}
	if a.Status != StatusReviewing && a.Status != StatusRework {
		return ErrStateConflict
	}
	if strings.TrimSpace(cause) == "" || strings.TrimSpace(remediation) == "" || strings.TrimSpace(actor) == "" {
		return FieldError("remediation", "原因、处置说明和处理员不能为空")
	}
	revision, exists := a.revisionByID(evidenceRevisionID)
	if !exists {
		return FieldError("evidenceRevisionID", "证据修订不存在")
	}
	for index := range a.Findings {
		finding := &a.Findings[index]
		if finding.FindingID != findingID {
			continue
		}
		if finding.Status == FindingApproved || finding.Status == FindingSuperseded {
			return ErrStateConflict
		}
		if revision.LineID != finding.LineID {
			return FieldError("evidenceRevisionID", "证据修订必须属于异常测线")
		}
		oldRevisionID := ""
		if len(a.Assessments) > 0 {
			oldRevisionID = a.Assessments[len(a.Assessments)-1].RevisionRefs[finding.LineID]
		}
		if oldRevision, ok := a.revisionByID(oldRevisionID); ok && revision.Sequence <= oldRevision.Sequence {
			return FieldError("evidenceRevisionID", "异常证据必须引用更新的修订")
		}
		finding.Cause = strings.TrimSpace(cause)
		finding.Remediation = strings.TrimSpace(remediation)
		finding.EvidenceRevisionID = evidenceRevisionID
		finding.RemediatedBy = strings.TrimSpace(actor)
		finding.Status = FindingEvidenceSubmitted
		finding.Reviewer, finding.ReviewNote, finding.ReviewedAt = "", "", nil
		a.Version++
		return nil
	}
	return NewError("FINDING_NOT_FOUND", "质量异常不存在")
}

func (a *SurveyAcceptance) ReviewFinding(findingID, reviewer, note string, approved bool, now time.Time) error {
	if a.Status != StatusReviewing {
		return ErrStateConflict
	}
	if strings.TrimSpace(reviewer) == "" || strings.TrimSpace(note) == "" {
		return FieldError("review", "复核员和复核意见不能为空")
	}
	for index := range a.Findings {
		finding := &a.Findings[index]
		if finding.FindingID != findingID {
			continue
		}
		if finding.Status != FindingReadyForReview {
			return NewError("FINDING_NOT_READY", "异常尚未通过复验，不能复核")
		}
		if reviewer == finding.RemediatedBy || reviewer == a.CreatedBy {
			return NewError("INDEPENDENT_REVIEW_REQUIRED", "创建者或处置人不能复核自己的处置")
		}
		finding.Reviewer, finding.ReviewNote = reviewer, strings.TrimSpace(note)
		at := now.UTC()
		finding.ReviewedAt = &at
		if approved {
			finding.Status = FindingApproved
		} else {
			finding.Status = FindingRejected
			a.Status = StatusRework
		}
		a.ReviewApproved = false
		a.Version++
		return nil
	}
	return NewError("FINDING_NOT_FOUND", "质量异常不存在")
}

func (a *SurveyAcceptance) DecideReview(reviewer, note string, approved bool) error {
	if a.Status != StatusReviewing {
		return ErrStateConflict
	}
	if strings.TrimSpace(reviewer) == "" || strings.TrimSpace(note) == "" {
		return FieldError("review", "复核员和决定说明不能为空")
	}
	if reviewer == a.CreatedBy {
		return NewError("INDEPENDENT_REVIEW_REQUIRED", "任务创建者不能执行最终复核")
	}
	latest := a.LatestAssessment()
	if latest == nil {
		return NewError("ASSESSMENT_REQUIRED", "尚无质量评估结果")
	}
	if approved {
		if latest.BlockingCount != 0 {
			return NewError("QUALITY_BLOCKED", "当前评估仍有阻断规则")
		}
		for _, finding := range a.Findings {
			if finding.Status != FindingApproved && finding.Status != FindingSuperseded {
				return NewError("FINDINGS_OPEN", "仍有异常未完成独立复核")
			}
		}
		a.ReviewApproved = true
		a.FinalReviewer = reviewer
	} else {
		a.Status = StatusRework
		a.ReviewApproved = false
		a.FinalReviewer = reviewer
	}
	a.Version++
	return nil
}

func (a *SurveyAcceptance) LatestAssessment() *QualityAssessment {
	if len(a.Assessments) == 0 {
		return nil
	}
	return &a.Assessments[len(a.Assessments)-1]
}
