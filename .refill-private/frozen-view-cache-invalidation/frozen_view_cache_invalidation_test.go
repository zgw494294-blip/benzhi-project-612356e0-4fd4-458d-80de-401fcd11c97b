package frozenviewcacheinvalidation

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sonarqa/internal/application"
	"sonarqa/internal/domain"
	"sonarqa/internal/httpapi"
	"sonarqa/internal/store"
)

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Unix(1700000000, 0).UTC() }

type sequentialIDs struct{ next int }

func (g *sequentialIDs) NewID(prefix string) string {
	g.next++
	return prefix + "-fixed-" + string(rune('a'+g.next))
}

func requireMutation(t *testing.T, operation func() (application.MutationResult, error)) application.MutationResult {
	t.Helper()
	result, err := operation()
	if err != nil {
		t.Fatalf("prepare frozen acceptance: %v", err)
	}
	return result
}

func prepareFrozen(t *testing.T, service *application.Service) application.MutationResult {
	t.Helper()
	ctx := context.Background()
	processor := application.Actor{ID: "processor-1", Role: application.RoleProcessor}
	reviewer := application.Actor{ID: "reviewer-1", Role: application.RoleReviewer}
	created := requireMutation(t, func() (application.MutationResult, error) {
		return service.Create(ctx, application.CreateAcceptanceCommand{
			ProjectCode: "SURVEY-CACHE-01",
			AreaBoundary: domain.AreaBoundary{Points: []domain.Point{
				{Longitude: 120, Latitude: 30}, {Longitude: 121, Latitude: 30}, {Longitude: 120, Latitude: 31},
			}},
			CoordinateReference: "CGCS2000",
			QualityThresholds: domain.QualityThresholds{
				MaxCoverageGapRatio: 0.2, MaxEchoGapRatio: 0.2, MaxHeadingDeviation: 10,
				MinPositionConfidence: 0.8, MaxSideLobeNoise: 0.2,
			},
			PlannedLineIDs: []string{"L-01"}, Actor: processor, IdempotencyKey: "create-cache-01",
		})
	})
	revised := requireMutation(t, func() (application.MutationResult, error) {
		return service.SubmitRevision(ctx, application.SubmitRevisionCommand{
			AcceptanceID: created.AcceptanceID, ExpectedVersion: created.Version, LineID: "L-01",
			CoverageSamples: []domain.CoverageSample{{AlongTrackMeter: 0, Covered: true}, {AlongTrackMeter: 10, Covered: true}},
			EchoGapRatio:    0.05, HeadingDeviation: 2, PositionConfidence: 0.95, SideLobeNoise: 0.05,
			CalibrationRef: "cal-01", Actor: processor, IdempotencyKey: "submit-cache-01",
		})
	})
	evaluated := requireMutation(t, func() (application.MutationResult, error) {
		return service.Evaluate(ctx, application.EvaluateCommand{
			AcceptanceID: created.AcceptanceID, ExpectedVersion: revised.Version,
			Actor: processor, IdempotencyKey: "evaluate-cache-01",
		})
	})
	decided := requireMutation(t, func() (application.MutationResult, error) {
		return service.DecideReview(ctx, application.DecideReviewCommand{
			AcceptanceID: created.AcceptanceID, ExpectedVersion: evaluated.Version, Approved: true,
			Note: "质量结论满足冻结要求", Actor: reviewer, IdempotencyKey: "decision-cache-01",
		})
	})
	return requireMutation(t, func() (application.MutationResult, error) {
		return service.Freeze(ctx, application.FreezeCommand{
			AcceptanceID: created.AcceptanceID, ExpectedVersion: decided.Version,
			Actor: reviewer, IdempotencyKey: "freeze-cache-01",
		})
	})
}

func TestReleasedAcceptanceInvalidatesFrozenViewCache(t *testing.T) {
	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	service := application.NewService(repository, fixedClock{}, &sequentialIDs{})
	frozen := prepareFrozen(t, service)
	handler := httpapi.NewServer(service, nil).Handler()

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1/acceptances/"+frozen.AcceptanceID, nil))
	if first.Code != http.StatusOK {
		t.Fatalf("prime frozen view cache: status %d", first.Code)
	}

	body, err := json.Marshal(map[string]any{"expectedVersion": frozen.Version})
	if err != nil {
		t.Fatalf("encode release request: %v", err)
	}
	releaseRequest := httptest.NewRequest(http.MethodPost, "/api/v1/acceptances/"+frozen.AcceptanceID+"/release", bytes.NewReader(body))
	releaseRequest.Header.Set("Content-Type", "application/json")
	releaseRequest.Header.Set("X-Actor-ID", "archivist-1")
	releaseRequest.Header.Set("X-Actor-Role", application.RoleArchivist)
	releaseRequest.Header.Set("Idempotency-Key", "release-cache-01")
	released := httptest.NewRecorder()
	handler.ServeHTTP(released, releaseRequest)
	if released.Code != http.StatusCreated {
		t.Fatalf("release acceptance: status %d body %s", released.Code, released.Body.String())
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/api/v1/acceptances/"+frozen.AcceptanceID, nil))
	var view application.AcceptanceView
	if err := json.Unmarshal(second.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode acceptance detail: %v", err)
	}
	if view.Status != domain.StatusReleased || view.Version != frozen.Version+1 || view.Release == nil {
		t.Fatalf("TestReleasedAcceptanceInvalidatesFrozenViewCache: detail stayed status=%s version=%d release=%v", view.Status, view.Version, view.Release)
	}
}
