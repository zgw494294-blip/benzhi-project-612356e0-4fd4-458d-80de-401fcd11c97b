package concurrent_evaluation_base_alias_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"sonarqa/internal/application"
	"sonarqa/internal/domain"
)

type repository struct {
	acceptance *domain.SurveyAcceptance
}

func (r *repository) Load(context.Context, string) (*domain.SurveyAcceptance, error) {
	return r.acceptance, nil
}

func (*repository) FindIdempotent(context.Context, string, string) (json.RawMessage, bool, error) {
	return nil, false, nil
}

func (*repository) Commit(context.Context, application.CommitRequest) (application.CommitResult, error) {
	return application.CommitResult{Result: json.RawMessage(`{}`)}, nil
}

func (r *repository) List(context.Context) ([]*domain.SurveyAcceptance, error) {
	return []*domain.SurveyAcceptance{r.acceptance}, nil
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC) }

type barrierIDs struct {
	mu      sync.Mutex
	armed   bool
	waiting int
	gate    chan struct{}
}

func (g *barrierIDs) arm() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.armed = true
	g.gate = make(chan struct{})
}

func (g *barrierIDs) NewID(prefix string) string {
	if prefix != "asm" {
		return prefix + "-fixed"
	}
	g.mu.Lock()
	if !g.armed {
		g.mu.Unlock()
		return "asm-warmup"
	}
	g.waiting++
	gate := g.gate
	if g.waiting == 2 {
		close(g.gate)
	}
	g.mu.Unlock()
	<-gate
	return "asm-concurrent"
}

func TestConcurrentEvaluationsDoNotShareMutableAggregate(t *testing.T) {
	now := fixedClock{}.Now()
	acceptance, err := domain.NewAcceptance(
		"acc-race", "RACE-001",
		domain.AreaBoundary{Points: []domain.Point{{Longitude: 120, Latitude: 30}, {Longitude: 121, Latitude: 30}, {Longitude: 121, Latitude: 31}, {Longitude: 120, Latitude: 31}}},
		"CGCS2000",
		domain.QualityThresholds{MaxCoverageGapRatio: 0.2, MaxEchoGapRatio: 0.1, MaxHeadingDeviation: 5, MinPositionConfidence: 0.9, MaxSideLobeNoise: 0.1},
		[]string{"L-001"}, "processor", now,
	)
	if err != nil {
		t.Fatal(err)
	}
	err = acceptance.SubmitRevision(domain.SonarLineRevision{
		RevisionID: "rev-001", AcceptanceID: acceptance.ID, LineID: "L-001", Sequence: 1,
		CoverageSamples: []domain.CoverageSample{{AlongTrackMeter: 0, Covered: true}, {AlongTrackMeter: 100, Covered: true}},
		EchoGapRatio:    0.01, HeadingDeviation: 1, PositionConfidence: 0.99, SideLobeNoise: 0.01,
		CalibrationRef: "cal-001", SubmittedBy: "processor", SubmittedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	ids := &barrierIDs{}
	service := application.NewService(&repository{acceptance: acceptance}, fixedClock{}, ids)
	ids.arm()
	start := make(chan struct{})
	errors := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, key := range []string{"evaluation-race-001", "evaluation-race-002"} {
		go func(idempotencyKey string) {
			ready.Done()
			<-start
			_, evaluateErr := service.Evaluate(context.Background(), application.EvaluateCommand{
				AcceptanceID: acceptance.ID, ExpectedVersion: acceptance.Version,
				Actor:          application.Actor{ID: "processor", Role: application.RoleProcessor},
				IdempotencyKey: idempotencyKey,
			})
			errors <- evaluateErr
		}(key)
	}
	ready.Wait()
	close(start)
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatalf("并发评估返回错误: %v", err)
		}
	}
}
