package replaycache_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"sonarqa/internal/application"
	"sonarqa/internal/domain"
)

type gatedRepository struct {
	arrived chan struct{}
	release chan struct{}
	result  json.RawMessage
}

func (r *gatedRepository) FindIdempotent(context.Context, string, string) (json.RawMessage, bool, error) {
	r.arrived <- struct{}{}
	<-r.release
	return append(json.RawMessage(nil), r.result...), true, nil
}

func (*gatedRepository) Load(context.Context, string) (*domain.SurveyAcceptance, error) {
	return nil, errors.New("不应加载聚合")
}

func (*gatedRepository) Commit(context.Context, application.CommitRequest) (application.CommitResult, error) {
	return application.CommitResult{}, errors.New("不应提交聚合")
}

func (*gatedRepository) List(context.Context) ([]*domain.SurveyAcceptance, error) {
	return nil, errors.New("不应查询列表")
}

func TestConcurrentReplayCacheIsRaceFree(t *testing.T) {
	const callers = 2
	repository := &gatedRepository{
		arrived: make(chan struct{}, callers),
		release: make(chan struct{}),
		result:  json.RawMessage(`{"acceptanceID":"acc-race","version":2,"status":"Collecting","resourceID":"rev-one"}`),
	}
	service := application.NewService(repository, nil, nil)
	command := application.SubmitRevisionCommand{
		AcceptanceID:    "acc-race",
		ExpectedVersion: 1,
		Actor:           application.Actor{ID: "processor-one", Role: application.RoleProcessor},
		IdempotencyKey:  "same-retry-key",
	}

	results := make(chan application.MutationResult, callers)
	errorsSeen := make(chan error, callers)
	var workers sync.WaitGroup
	workers.Add(callers)
	for index := 0; index < callers; index++ {
		go func() {
			defer workers.Done()
			result, err := service.SubmitRevision(context.Background(), command)
			results <- result
			errorsSeen <- err
		}()
	}

	for index := 0; index < callers; index++ {
		<-repository.arrived
	}
	close(repository.release)
	workers.Wait()
	close(results)
	close(errorsSeen)

	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("并发幂等重放失败: %v", err)
		}
	}
	for result := range results {
		if !result.Replayed || result.ResourceID != "rev-one" {
			t.Fatalf("幂等重放结果不稳定: %+v", result)
		}
	}
}
