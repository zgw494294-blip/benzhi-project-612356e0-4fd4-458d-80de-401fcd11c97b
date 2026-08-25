package application

import (
	"sonarqa/internal/domain"
	"time"
)

type CreateAcceptanceCommand struct {
	ProjectCode         string                   `json:"projectCode"`
	AreaBoundary        domain.AreaBoundary      `json:"areaBoundary"`
	CoordinateReference string                   `json:"coordinateReference"`
	QualityThresholds   domain.QualityThresholds `json:"qualityThresholds"`
	PlannedLineIDs      []string                 `json:"plannedLineIDs"`
	Actor               Actor                    `json:"-"`
	IdempotencyKey      string                   `json:"-"`
}

type SubmitRevisionCommand struct {
	AcceptanceID       string                  `json:"-"`
	ExpectedVersion    int64                   `json:"expectedVersion"`
	LineID             string                  `json:"lineID"`
	CoverageSamples    []domain.CoverageSample `json:"coverageSamples"`
	EchoGapRatio       float64                 `json:"echoGapRatio"`
	HeadingDeviation   float64                 `json:"headingDeviation"`
	PositionConfidence float64                 `json:"positionConfidence"`
	SideLobeNoise      float64                 `json:"sideLobeNoise"`
	CalibrationRef     string                  `json:"calibrationRef"`
	Actor              Actor                   `json:"-"`
	IdempotencyKey     string                  `json:"-"`
}

type EvaluateCommand struct {
	AcceptanceID    string `json:"-"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Actor           Actor  `json:"-"`
	IdempotencyKey  string `json:"-"`
}

type RemediateFindingCommand struct {
	AcceptanceID       string `json:"-"`
	FindingID          string `json:"-"`
	ExpectedVersion    int64  `json:"expectedVersion"`
	Cause              string `json:"cause"`
	Remediation        string `json:"remediation"`
	EvidenceRevisionID string `json:"evidenceRevisionID"`
	Actor              Actor  `json:"-"`
	IdempotencyKey     string `json:"-"`
}

type ReviewFindingCommand struct {
	AcceptanceID    string `json:"-"`
	FindingID       string `json:"-"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Approved        bool   `json:"approved"`
	Note            string `json:"note"`
	Actor           Actor  `json:"-"`
	IdempotencyKey  string `json:"-"`
}

type DecideReviewCommand struct {
	AcceptanceID    string `json:"-"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Approved        bool   `json:"approved"`
	Note            string `json:"note"`
	Actor           Actor  `json:"-"`
	IdempotencyKey  string `json:"-"`
}

type FreezeCommand struct {
	AcceptanceID    string `json:"-"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Actor           Actor  `json:"-"`
	IdempotencyKey  string `json:"-"`
}

type ReleaseCommand struct {
	AcceptanceID    string `json:"-"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Actor           Actor  `json:"-"`
	IdempotencyKey  string `json:"-"`
}

type ListQuery struct {
	Status      string
	ProjectCode string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Page        int
	PageSize    int
}
type ListItem struct {
	AcceptanceView
	LatestAssessmentBlocking int  `json:"latestAssessmentBlocking"`
	OpenFindings             int  `json:"openFindings"`
	Released                 bool `json:"released"`
}
type ListResult struct {
	Items        []ListItem            `json:"items"`
	Total        int                   `json:"total"`
	HasNext      bool                  `json:"hasNext"`
	StatusCounts map[domain.Status]int `json:"statusCounts"`
}
