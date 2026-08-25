package domain

import "time"

type Status string

const (
	StatusDraft      Status = "Draft"
	StatusCollecting Status = "Collecting"
	StatusReviewing  Status = "Reviewing"
	StatusRework     Status = "Rework"
	StatusFrozen     Status = "Frozen"
	StatusReleased   Status = "Released"
)

type Point struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
}

type AreaBoundary struct {
	Points []Point `json:"points"`
}

type QualityThresholds struct {
	MaxCoverageGapRatio   float64 `json:"maxCoverageGapRatio"`
	MaxEchoGapRatio       float64 `json:"maxEchoGapRatio"`
	MaxHeadingDeviation   float64 `json:"maxHeadingDeviation"`
	MinPositionConfidence float64 `json:"minPositionConfidence"`
	MaxSideLobeNoise      float64 `json:"maxSideLobeNoise"`
}

type CoverageSample struct {
	AlongTrackMeter float64 `json:"alongTrackMeter"`
	Covered         bool    `json:"covered"`
}

type SonarLineRevision struct {
	RevisionID         string           `json:"revisionID"`
	AcceptanceID       string           `json:"acceptanceID"`
	LineID             string           `json:"lineID"`
	Sequence           int              `json:"sequence"`
	CoverageSamples    []CoverageSample `json:"coverageSamples"`
	EchoGapRatio       float64          `json:"echoGapRatio"`
	HeadingDeviation   float64          `json:"headingDeviation"`
	PositionConfidence float64          `json:"positionConfidence"`
	SideLobeNoise      float64          `json:"sideLobeNoise"`
	CalibrationRef     string           `json:"calibrationRef"`
	SubmittedBy        string           `json:"submittedBy"`
	SubmittedAt        time.Time        `json:"submittedAt"`
}

type RuleOutcome struct {
	LineID      string  `json:"lineID"`
	RevisionID  string  `json:"revisionID"`
	RuleCode    string  `json:"ruleCode"`
	Passed      bool    `json:"passed"`
	Observed    float64 `json:"observed"`
	Threshold   float64 `json:"threshold"`
	Comparator  string  `json:"comparator"`
	Description string  `json:"description"`
}

type QualityAssessment struct {
	AssessmentID   string            `json:"assessmentID"`
	AcceptanceID   string            `json:"acceptanceID"`
	RevisionRefs   map[string]string `json:"revisionRefs"`
	RuleSetVersion string            `json:"ruleSetVersion"`
	RuleOutcomes   []RuleOutcome     `json:"ruleOutcomes"`
	BlockingCount  int               `json:"blockingCount"`
	SummaryHash    string            `json:"summaryHash"`
	EvaluatedAt    time.Time         `json:"evaluatedAt"`
}

type FindingStatus string

const (
	FindingOpen              FindingStatus = "Open"
	FindingEvidenceSubmitted FindingStatus = "EvidenceSubmitted"
	FindingReadyForReview    FindingStatus = "ReadyForReview"
	FindingApproved          FindingStatus = "Approved"
	FindingRejected          FindingStatus = "Rejected"
	FindingSuperseded        FindingStatus = "Superseded"
)

type QualityFinding struct {
	FindingID          string        `json:"findingID"`
	AcceptanceID       string        `json:"acceptanceID"`
	LineID             string        `json:"lineID"`
	RuleCode           string        `json:"ruleCode"`
	Severity           string        `json:"severity"`
	Status             FindingStatus `json:"status"`
	Cause              string        `json:"cause,omitempty"`
	Remediation        string        `json:"remediation,omitempty"`
	EvidenceRevisionID string        `json:"evidenceRevisionID,omitempty"`
	RemediatedBy       string        `json:"remediatedBy,omitempty"`
	Reviewer           string        `json:"reviewer,omitempty"`
	ReviewNote         string        `json:"reviewNote,omitempty"`
	ReviewedAt         *time.Time    `json:"reviewedAt,omitempty"`
	CreatedAt          time.Time     `json:"createdAt"`
}

type FrozenLine struct {
	LineID     string `json:"lineID"`
	RevisionID string `json:"revisionID"`
	Sequence   int    `json:"sequence"`
}

type FrozenManifest struct {
	AcceptanceID     string       `json:"acceptanceID"`
	ProjectCode      string       `json:"projectCode"`
	Lines            []FrozenLine `json:"lines"`
	AssessmentID     string       `json:"assessmentID"`
	AssessmentHash   string       `json:"assessmentHash"`
	ApprovedFindings []string     `json:"approvedFindings"`
	FrozenBy         string       `json:"frozenBy"`
	FrozenAt         time.Time    `json:"frozenAt"`
	ManifestHash     string       `json:"manifestHash"`
}

type ArchiveRelease struct {
	ReleaseID          string    `json:"releaseID"`
	AcceptanceID       string    `json:"acceptanceID"`
	ProjectCode        string    `json:"projectCode"`
	ManifestHash       string    `json:"manifestHash"`
	IssuedBy           string    `json:"issuedBy"`
	IssuedAt           time.Time `json:"issuedAt"`
	VerificationDigest string    `json:"verificationDigest"`
}

type SurveyAcceptance struct {
	ID                  string              `json:"id"`
	ProjectCode         string              `json:"projectCode"`
	AreaBoundary        AreaBoundary        `json:"areaBoundary"`
	CoordinateReference string              `json:"coordinateReference"`
	QualityThresholds   QualityThresholds   `json:"qualityThresholds"`
	PlannedLineIDs      []string            `json:"plannedLineIDs"`
	Status              Status              `json:"status"`
	Version             int64               `json:"version"`
	CreatedBy           string              `json:"createdBy"`
	CreatedAt           time.Time           `json:"createdAt"`
	Revisions           []SonarLineRevision `json:"revisions"`
	Assessments         []QualityAssessment `json:"assessments"`
	Findings            []QualityFinding    `json:"findings"`
	ReviewApproved      bool                `json:"reviewApproved"`
	FinalReviewer       string              `json:"finalReviewer,omitempty"`
	Manifest            *FrozenManifest     `json:"manifest,omitempty"`
	Release             *ArchiveRelease     `json:"release,omitempty"`
}
