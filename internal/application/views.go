package application

import (
	"time"

	"sonarqa/internal/domain"
)

type MutationResult struct {
	AcceptanceID string        `json:"acceptanceID"`
	Version      int64         `json:"version"`
	Status       domain.Status `json:"status"`
	ResourceID   string        `json:"resourceID,omitempty"`
	Replayed     bool          `json:"replayed,omitempty"`
}

type AcceptanceView struct {
	ID                  string                     `json:"id"`
	ProjectCode         string                     `json:"projectCode"`
	AreaBoundary        domain.AreaBoundary        `json:"areaBoundary"`
	CoordinateReference string                     `json:"coordinateReference"`
	QualityThresholds   domain.QualityThresholds   `json:"qualityThresholds"`
	PlannedLineIDs      []string                   `json:"plannedLineIDs"`
	Status              domain.Status              `json:"status"`
	Version             int64                      `json:"version"`
	CreatedBy           string                     `json:"createdBy"`
	CreatedAt           time.Time                  `json:"createdAt"`
	LatestRevisions     []domain.SonarLineRevision `json:"latestRevisions"`
	LatestAssessment    *domain.QualityAssessment  `json:"latestAssessment,omitempty"`
	Findings            []domain.QualityFinding    `json:"findings"`
	ReviewApproved      bool                       `json:"reviewApproved"`
	FinalReviewer       string                     `json:"finalReviewer,omitempty"`
	Manifest            *domain.FrozenManifest     `json:"manifest,omitempty"`
	Release             *domain.ArchiveRelease     `json:"release,omitempty"`
}

func BuildView(acceptance *domain.SurveyAcceptance) AcceptanceView {
	latest := make([]domain.SonarLineRevision, 0, len(acceptance.PlannedLineIDs))
	for _, line := range acceptance.PlannedLineIDs {
		if revision, exists := acceptance.LatestRevision(line); exists {
			latest = append(latest, revision)
		}
	}
	var assessment *domain.QualityAssessment
	if value := acceptance.LatestAssessment(); value != nil {
		copyValue := *value
		assessment = &copyValue
	}
	return AcceptanceView{
		ID: acceptance.ID, ProjectCode: acceptance.ProjectCode, AreaBoundary: acceptance.AreaBoundary,
		CoordinateReference: acceptance.CoordinateReference, QualityThresholds: acceptance.QualityThresholds,
		PlannedLineIDs: append([]string(nil), acceptance.PlannedLineIDs...), Status: acceptance.Status,
		Version: acceptance.Version, CreatedBy: acceptance.CreatedBy, CreatedAt: acceptance.CreatedAt,
		LatestRevisions: latest, LatestAssessment: assessment,
		Findings: append([]domain.QualityFinding(nil), acceptance.Findings...), ReviewApproved: acceptance.ReviewApproved,
		FinalReviewer: acceptance.FinalReviewer, Manifest: acceptance.Manifest, Release: acceptance.Release,
	}
}
