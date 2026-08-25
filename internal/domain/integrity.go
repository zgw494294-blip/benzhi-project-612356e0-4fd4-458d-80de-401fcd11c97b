package domain

import (
	"fmt"
	"sort"
)

// ValidateIntegrity checks cross-entity invariants when a projection is committed or recovered.
func (a *SurveyAcceptance) ValidateIntegrity() error {
	if a == nil {
		return fmt.Errorf("任务投影为空")
	}
	if a.ID == "" || a.ProjectCode == "" || a.CreatedBy == "" || a.CreatedAt.IsZero() {
		return fmt.Errorf("任务核心标识不完整")
	}
	if a.Version < 1 {
		return fmt.Errorf("任务版本必须为正整数")
	}
	if err := ValidateCreate(a.ProjectCode, a.AreaBoundary, a.CoordinateReference, a.QualityThresholds, a.PlannedLineIDs, a.CreatedBy); err != nil {
		return fmt.Errorf("任务创建字段无效: %w", err)
	}
	if !validStatus(a.Status) {
		return fmt.Errorf("未知任务状态 %q", a.Status)
	}
	if err := a.validateRevisionHistory(); err != nil {
		return err
	}
	if err := a.validateAssessments(); err != nil {
		return err
	}
	if err := a.validateFindings(); err != nil {
		return err
	}
	if err := a.validateLifecycleArtifacts(); err != nil {
		return err
	}
	return nil
}

func validStatus(status Status) bool {
	switch status {
	case StatusDraft, StatusCollecting, StatusReviewing, StatusRework, StatusFrozen, StatusReleased:
		return true
	default:
		return false
	}
}

func (a *SurveyAcceptance) validateRevisionHistory() error {
	ids := make(map[string]struct{}, len(a.Revisions))
	sequences := make(map[string][]int, len(a.PlannedLineIDs))
	for _, revision := range a.Revisions {
		if revision.AcceptanceID != a.ID {
			return fmt.Errorf("修订 %s 指向其他任务", revision.RevisionID)
		}
		if !a.hasLine(revision.LineID) {
			return fmt.Errorf("修订 %s 指向未计划测线", revision.RevisionID)
		}
		if _, duplicate := ids[revision.RevisionID]; duplicate {
			return fmt.Errorf("修订编号 %s 重复", revision.RevisionID)
		}
		ids[revision.RevisionID] = struct{}{}
		if err := ValidateRevision(revision); err != nil {
			return fmt.Errorf("修订 %s 无效: %w", revision.RevisionID, err)
		}
		sequences[revision.LineID] = append(sequences[revision.LineID], revision.Sequence)
	}
	for lineID, values := range sequences {
		sort.Ints(values)
		for index, sequence := range values {
			if sequence != index+1 {
				return fmt.Errorf("测线 %s 的修订序号不连续", lineID)
			}
		}
	}
	return nil
}

func (a *SurveyAcceptance) validateAssessments() error {
	ids := make(map[string]struct{}, len(a.Assessments))
	for _, assessment := range a.Assessments {
		if assessment.AssessmentID == "" || assessment.AcceptanceID != a.ID || assessment.EvaluatedAt.IsZero() {
			return fmt.Errorf("评估记录核心字段不完整")
		}
		if _, duplicate := ids[assessment.AssessmentID]; duplicate {
			return fmt.Errorf("评估编号 %s 重复", assessment.AssessmentID)
		}
		ids[assessment.AssessmentID] = struct{}{}
		if assessment.RuleSetVersion != CurrentRuleSetVersion {
			return fmt.Errorf("评估 %s 使用未知规则版本", assessment.AssessmentID)
		}
		if len(assessment.RevisionRefs) != len(a.PlannedLineIDs) {
			return fmt.Errorf("评估 %s 未覆盖全部计划测线", assessment.AssessmentID)
		}
		for _, lineID := range a.PlannedLineIDs {
			revisionID, exists := assessment.RevisionRefs[lineID]
			if !exists {
				return fmt.Errorf("评估 %s 缺少测线 %s", assessment.AssessmentID, lineID)
			}
			revision, found := a.revisionByID(revisionID)
			if !found || revision.LineID != lineID {
				return fmt.Errorf("评估 %s 的修订引用无效", assessment.AssessmentID)
			}
		}
		blocking := 0
		for _, outcome := range assessment.RuleOutcomes {
			if outcome.LineID == "" || outcome.RevisionID != assessment.RevisionRefs[outcome.LineID] || outcome.RuleCode == "" {
				return fmt.Errorf("评估 %s 包含无效规则结论", assessment.AssessmentID)
			}
			if !outcome.Passed {
				blocking++
			}
		}
		if blocking != assessment.BlockingCount {
			return fmt.Errorf("评估 %s 的阻断数量不一致", assessment.AssessmentID)
		}
		hash, err := AssessmentHash(a.ID, assessment.RevisionRefs, assessment.RuleSetVersion, assessment.RuleOutcomes)
		if err != nil {
			return err
		}
		if hash != assessment.SummaryHash {
			return fmt.Errorf("评估 %s 的摘要哈希不匹配", assessment.AssessmentID)
		}
	}
	return nil
}

func (a *SurveyAcceptance) validateFindings() error {
	ids := make(map[string]struct{}, len(a.Findings))
	for _, finding := range a.Findings {
		if finding.FindingID == "" || finding.AcceptanceID != a.ID || finding.LineID == "" || finding.RuleCode == "" || finding.CreatedAt.IsZero() {
			return fmt.Errorf("异常记录核心字段不完整")
		}
		if _, duplicate := ids[finding.FindingID]; duplicate {
			return fmt.Errorf("异常编号 %s 重复", finding.FindingID)
		}
		ids[finding.FindingID] = struct{}{}
		if !a.hasLine(finding.LineID) || finding.Severity != "Blocking" || !validFindingStatus(finding.Status) {
			return fmt.Errorf("异常 %s 的分类字段无效", finding.FindingID)
		}
		if finding.EvidenceRevisionID != "" {
			revision, exists := a.revisionByID(finding.EvidenceRevisionID)
			if !exists || revision.LineID != finding.LineID || finding.RemediatedBy == "" || finding.Cause == "" || finding.Remediation == "" {
				return fmt.Errorf("异常 %s 的处置证据无效", finding.FindingID)
			}
		}
		if finding.Status == FindingApproved {
			if finding.Reviewer == "" || finding.ReviewedAt == nil || finding.ReviewNote == "" {
				return fmt.Errorf("已批准异常 %s 缺少复核记录", finding.FindingID)
			}
			if finding.Reviewer == finding.RemediatedBy || finding.Reviewer == a.CreatedBy {
				return fmt.Errorf("异常 %s 未满足独立复核约束", finding.FindingID)
			}
		}
	}
	return nil
}

func validFindingStatus(status FindingStatus) bool {
	switch status {
	case FindingOpen, FindingEvidenceSubmitted, FindingReadyForReview, FindingApproved, FindingRejected, FindingSuperseded:
		return true
	default:
		return false
	}
}

func (a *SurveyAcceptance) validateLifecycleArtifacts() error {
	if a.Status == StatusFrozen || a.Status == StatusReleased {
		if a.Manifest == nil {
			return fmt.Errorf("%s 状态缺少冻结清单", a.Status)
		}
		if err := a.VerifyManifest(); err != nil {
			return err
		}
	}
	if a.Manifest != nil {
		if a.Status != StatusFrozen && a.Status != StatusReleased {
			return fmt.Errorf("未冻结任务不能包含冻结清单")
		}
		if a.Manifest.AcceptanceID != a.ID || a.Manifest.ProjectCode != a.ProjectCode || len(a.Manifest.Lines) != len(a.PlannedLineIDs) {
			return fmt.Errorf("冻结清单与任务不一致")
		}
		assessment := a.LatestAssessment()
		if assessment == nil || assessment.AssessmentID != a.Manifest.AssessmentID || assessment.SummaryHash != a.Manifest.AssessmentHash {
			return fmt.Errorf("冻结清单未引用最新评估")
		}
	}
	if a.Status == StatusReleased {
		if a.Release == nil {
			return fmt.Errorf("Released 状态缺少放行凭据")
		}
		if err := VerifyRelease(*a.Release); err != nil {
			return err
		}
	}
	if a.Release != nil {
		if a.Status != StatusReleased || a.Manifest == nil || a.Release.AcceptanceID != a.ID || a.Release.ProjectCode != a.ProjectCode || a.Release.ManifestHash != a.Manifest.ManifestHash {
			return fmt.Errorf("放行凭据与任务或冻结清单不一致")
		}
	}
	return nil
}
