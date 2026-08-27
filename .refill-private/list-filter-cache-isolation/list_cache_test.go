package listfiltercacheisolation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"sonarqa/internal/application"
	"sonarqa/internal/domain"
)

type listRepository struct {
	items []*domain.SurveyAcceptance
	calls int
}

func (r *listRepository) Load(context.Context, string) (*domain.SurveyAcceptance, error) {
	return nil, domain.ErrNotFound
}

func (r *listRepository) FindIdempotent(context.Context, string, string) (json.RawMessage, bool, error) {
	return nil, false, nil
}

func (r *listRepository) Commit(context.Context, application.CommitRequest) (application.CommitResult, error) {
	return application.CommitResult{}, nil
}

func (r *listRepository) List(context.Context) ([]*domain.SurveyAcceptance, error) {
	r.calls++
	return r.items, nil
}

type fixedIDs struct{}

func (fixedIDs) NewID(prefix string) string { return prefix + "-private" }

func acceptance(t *testing.T, id, projectCode string) *domain.SurveyAcceptance {
	t.Helper()
	value, err := domain.NewAcceptance(
		id, projectCode,
		domain.AreaBoundary{Points: []domain.Point{{Longitude: 120, Latitude: 30}, {Longitude: 121, Latitude: 30}, {Longitude: 121, Latitude: 31}}},
		"CGCS2000",
		domain.QualityThresholds{MaxCoverageGapRatio: 0.2, MaxEchoGapRatio: 0.1, MaxHeadingDeviation: 5, MinPositionConfidence: 0.9, MaxSideLobeNoise: 0.1},
		[]string{"L-1"}, "processor", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestListFiltersDoNotReuseAnotherRequestCache(t *testing.T) {
	repository := &listRepository{items: []*domain.SurveyAcceptance{
		acceptance(t, "acc-area-a", "AREA-A-001"),
		acceptance(t, "acc-area-b", "AREA-B-001"),
	}}
	service := application.NewService(repository, application.SystemClock{}, fixedIDs{})
	first, err := service.ListQuery(context.Background(), application.ListQuery{ProjectCode: "AREA-A", Page: 1, PageSize: 20})
	if err != nil || first.Total != 1 || first.Items[0].ProjectCode != "AREA-A-001" {
		t.Fatalf("首个列表查询结果异常: %#v, %v", first, err)
	}
	second, err := service.ListQuery(context.Background(), application.ListQuery{ProjectCode: "AREA-B", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if second.Total != 1 || len(second.Items) != 1 || second.Items[0].ProjectCode != "AREA-B-001" {
		t.Fatalf("第二个过滤请求复用了其他请求的缓存: %#v", second)
	}
	if repository.calls != 2 {
		t.Fatalf("第二个不同过滤条件未重新查询 repository: calls=%d", repository.calls)
	}
}
