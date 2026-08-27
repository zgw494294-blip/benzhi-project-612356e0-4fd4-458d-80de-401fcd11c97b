package evaluationcancel_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"sonarqa/internal/application"
	"sonarqa/internal/domain"
)

type controlledRepository struct {
	mu               sync.Mutex
	aggregates       map[string]*domain.SurveyAcceptance
	blockedID        string
	loadStarted      chan struct{}
	releaseBlocked   chan struct{}
	releaseBlockedDo sync.Once
}

func (r *controlledRepository) Load(ctx context.Context, id string) (*domain.SurveyAcceptance, error) {
	if id == r.blockedID {
		close(r.loadStarted)
		<-r.releaseBlocked
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	} else {
		r.releaseBlockedDo.Do(func() { close(r.releaseBlocked) })
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.aggregates[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copyValue := *value
	copyValue.Revisions = append([]domain.SonarLineRevision(nil), value.Revisions...)
	return &copyValue, nil
}

func (r *controlledRepository) FindIdempotent(context.Context, string, string) (json.RawMessage, bool, error) {
	return nil, false, nil
}

func (r *controlledRepository) Commit(_ context.Context, request application.CommitRequest) (application.CommitResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.aggregates[request.Acceptance.ID] = request.Acceptance
	return application.CommitResult{Result: request.Result}, nil
}

func (r *controlledRepository) List(context.Context) ([]*domain.SurveyAcceptance, error) {
	return nil, nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type sequentialIDs struct {
	mu   sync.Mutex
	next int
}

func (g *sequentialIDs) NewID(prefix string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	return fmt.Sprintf("%s-%d", prefix, g.next)
}

func readyAcceptance(t *testing.T, id string, now time.Time) *domain.SurveyAcceptance {
	t.Helper()
	acceptance, err := domain.NewAcceptance(
		id,
		"PROJECT-"+id,
		domain.AreaBoundary{Points: []domain.Point{{Longitude: 120, Latitude: 30}, {Longitude: 121, Latitude: 30}, {Longitude: 121, Latitude: 31}}},
		"CGCS2000",
		domain.QualityThresholds{MaxCoverageGapRatio: 0.2, MaxEchoGapRatio: 0.1, MaxHeadingDeviation: 5, MinPositionConfidence: 0.9, MaxSideLobeNoise: 0.1},
		[]string{"L-1"},
		"processor-1",
		now,
	)
	if err != nil {
		t.Fatalf("创建测试任务失败: %v", err)
	}
	err = acceptance.SubmitRevision(domain.SonarLineRevision{
		RevisionID: "rev-" + id, AcceptanceID: id, LineID: "L-1", Sequence: 1,
		CoverageSamples: []domain.CoverageSample{{AlongTrackMeter: 0, Covered: true}, {AlongTrackMeter: 100, Covered: true}},
		EchoGapRatio:    0.01, HeadingDeviation: 1, PositionConfidence: 0.99, SideLobeNoise: 0.01,
		CalibrationRef: "cal-1", SubmittedBy: "processor-1", SubmittedAt: now,
	})
	if err != nil {
		t.Fatalf("准备测试修订失败: %v", err)
	}
	return acceptance
}

func TestIndependentEvaluationsKeepRequestContexts(t *testing.T) {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	first := readyAcceptance(t, "acc-first", now)
	second := readyAcceptance(t, "acc-second", now)
	repository := &controlledRepository{
		aggregates:     map[string]*domain.SurveyAcceptance{first.ID: first, second.ID: second},
		blockedID:      first.ID,
		loadStarted:    make(chan struct{}),
		releaseBlocked: make(chan struct{}),
	}
	service := application.NewService(repository, fixedClock{now: now}, &sequentialIDs{})
	actor := application.Actor{ID: "processor-1", Role: application.RoleProcessor}

	firstResult := make(chan error, 1)
	go func() {
		_, err := service.Evaluate(context.Background(), application.EvaluateCommand{
			AcceptanceID: first.ID, ExpectedVersion: first.Version, Actor: actor, IdempotencyKey: "evaluate-first-001",
		})
		firstResult <- err
	}()
	<-repository.loadStarted

	_, secondErr := service.Evaluate(context.Background(), application.EvaluateCommand{
		AcceptanceID: second.ID, ExpectedVersion: second.Version, Actor: actor, IdempotencyKey: "evaluate-second-001",
	})
	if secondErr != nil {
		t.Fatalf("第二个独立评估失败: %v", secondErr)
	}
	if err := <-firstResult; err != nil {
		t.Fatalf("独立评估被其他任务取消: %v", err)
	}
}
