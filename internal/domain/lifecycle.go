package domain

import (
	"sort"
	"strings"
	"time"
)

func (a *SurveyAcceptance) Freeze(actor string, now time.Time) (*FrozenManifest, error) {
	if a.Status != StatusReviewing || !a.ReviewApproved {
		return nil, NewError("REVIEW_REQUIRED", "独立复核通过后才能冻结")
	}
	if strings.TrimSpace(actor) == "" {
		return nil, FieldError("actor", "冻结操作人不能为空")
	}
	latest := a.LatestAssessment()
	if latest == nil || latest.BlockingCount != 0 {
		return nil, NewError("QUALITY_BLOCKED", "存在阻断结论，不能冻结")
	}
	lines := make([]FrozenLine, 0, len(a.PlannedLineIDs))
	for _, lineID := range a.PlannedLineIDs {
		revision, exists := a.LatestRevision(lineID)
		if !exists || latest.RevisionRefs[lineID] != revision.RevisionID {
			return nil, NewError("ASSESSMENT_STALE", "最新修订尚未纳入质量评估")
		}
		lines = append(lines, FrozenLine{LineID: lineID, RevisionID: revision.RevisionID, Sequence: revision.Sequence})
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].LineID < lines[j].LineID })
	approved := make([]string, 0)
	for _, finding := range a.Findings {
		if finding.Status == FindingApproved {
			approved = append(approved, finding.FindingID)
		} else if finding.Status != FindingSuperseded {
			return nil, NewError("FINDINGS_OPEN", "仍有异常未闭环")
		}
	}
	sort.Strings(approved)
	manifest := FrozenManifest{
		AcceptanceID: a.ID, ProjectCode: a.ProjectCode, Lines: lines,
		AssessmentID: latest.AssessmentID, AssessmentHash: latest.SummaryHash,
		ApprovedFindings: approved, FrozenBy: strings.TrimSpace(actor), FrozenAt: now.UTC(),
	}
	hash, err := ComputeManifestHash(manifest)
	if err != nil {
		return nil, err
	}
	manifest.ManifestHash = hash
	a.Manifest = &manifest
	a.Status = StatusFrozen
	a.Version++
	return &manifest, nil
}

func (a *SurveyAcceptance) VerifyManifest() error {
	if a.Manifest == nil {
		return NewError("MANIFEST_NOT_FOUND", "冻结清单不存在")
	}
	hash, err := ComputeManifestHash(*a.Manifest)
	if err != nil {
		return err
	}
	if hash != a.Manifest.ManifestHash {
		return NewError("MANIFEST_TAMPERED", "冻结清单哈希校验失败")
	}
	return nil
}

func (a *SurveyAcceptance) IssueRelease(releaseID, actor string, now time.Time) (*ArchiveRelease, error) {
	if a.Status == StatusReleased || a.Release != nil {
		return nil, NewError("RELEASE_EXISTS", "归档放行凭据已经签发")
	}
	if a.Status != StatusFrozen {
		return nil, NewError("FROZEN_REQUIRED", "仅 Frozen 任务可以签发放行凭据")
	}
	if strings.TrimSpace(releaseID) == "" || strings.TrimSpace(actor) == "" {
		return nil, FieldError("release", "放行编号和签发人不能为空")
	}
	if err := a.VerifyManifest(); err != nil {
		return nil, err
	}
	release := ArchiveRelease{
		ReleaseID: releaseID, AcceptanceID: a.ID, ProjectCode: a.ProjectCode,
		ManifestHash: a.Manifest.ManifestHash, IssuedBy: strings.TrimSpace(actor), IssuedAt: now.UTC(),
	}
	digest, err := ComputeReleaseDigest(release)
	if err != nil {
		return nil, err
	}
	release.VerificationDigest = digest
	a.Release = &release
	a.Status = StatusReleased
	a.Version++
	return &release, nil
}

func VerifyRelease(release ArchiveRelease) error {
	digest, err := ComputeReleaseDigest(release)
	if err != nil {
		return err
	}
	if digest != release.VerificationDigest {
		return NewError("RELEASE_TAMPERED", "放行凭据校验摘要不匹配")
	}
	return nil
}
