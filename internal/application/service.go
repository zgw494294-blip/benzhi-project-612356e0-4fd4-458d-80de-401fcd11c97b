package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"sonarqa/internal/domain"
)

type Service struct {
	repository  Repository
	clock       Clock
	ids         IDGenerator
	viewMu      sync.RWMutex
	stableViews map[string]AcceptanceView
}

func NewService(repository Repository, clock Clock, ids IDGenerator) *Service {
	return &Service{
		repository:  repository,
		clock:       clock,
		ids:         ids,
		stableViews: make(map[string]AcceptanceView),
	}
}

func validateIdempotency(key string) error {
	key = strings.TrimSpace(key)
	if len(key) < 8 || len(key) > 128 {
		return domain.FieldError("Idempotency-Key", "幂等键长度必须在 8 到 128 个字符之间")
	}
	return nil
}

func operation(name, acceptanceID string) string {
	return name + ":" + acceptanceID
}

func (s *Service) replay(ctx context.Context, operation, key string, target any) (bool, error) {
	if err := validateIdempotency(key); err != nil {
		return false, err
	}
	raw, exists, err := s.repository.FindIdempotent(ctx, operation, key)
	if err != nil || !exists {
		return false, err
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return false, fmt.Errorf("解析幂等结果: %w", err)
	}
	if result, ok := target.(*MutationResult); ok {
		result.Replayed = true
	}
	return true, nil
}

func encodeResult(value any) (json.RawMessage, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("编码操作结果: %w", err)
	}
	return data, nil
}

func checkVersion(acceptance *domain.SurveyAcceptance, expected int64) error {
	if expected <= 0 {
		return domain.FieldError("expectedVersion", "expectedVersion 必须为正整数")
	}
	if acceptance.Version != expected {
		return domain.ErrVersionConflict
	}
	return nil
}

func (s *Service) commitMutation(ctx context.Context, aggregate *domain.SurveyAcceptance, expected int64, operationName, key string, actor Actor, result MutationResult) (MutationResult, error) {
	raw, err := encodeResult(result)
	if err != nil {
		return MutationResult{}, err
	}
	commit, err := s.repository.Commit(ctx, CommitRequest{
		Acceptance: aggregate, ExpectedVersion: expected, Operation: operationName,
		IdempotencyKey: key, Result: raw, Actor: actor.ID, OccurredAt: s.clock.Now(),
	})
	if err != nil {
		return MutationResult{}, err
	}
	if commit.Replayed {
		if err := json.Unmarshal(commit.Result, &result); err != nil {
			return MutationResult{}, fmt.Errorf("解析并发幂等结果: %w", err)
		}
		result.Replayed = true
	}
	s.refreshStableView(aggregate)
	return result, nil
}

// refreshStableView keeps the Frozen detail cache in sync with persisted state
// after a mutation commits. For Frozen tasks the cached projection is refreshed
// with the committed version; for Released tasks the committed view is kept so a
// concurrent Get that loaded the prior Frozen aggregate cannot overwrite the
// newer Released view (the version guard in Get compares against this entry).
// Other statuses are not cached here; Get only caches Frozen projections, so
// removing the entry lets subsequent reads load fresh state from the repository.
func (s *Service) refreshStableView(aggregate *domain.SurveyAcceptance) {
	s.viewMu.Lock()
	defer s.viewMu.Unlock()
	switch aggregate.Status {
	case domain.StatusFrozen, domain.StatusReleased:
		fresh := BuildView(aggregate)
		if cached, exists := s.stableViews[aggregate.ID]; exists && cached.Version > fresh.Version {
			return
		}
		s.stableViews[aggregate.ID] = fresh
	default:
		delete(s.stableViews, aggregate.ID)
	}
}

func (s *Service) Get(ctx context.Context, id string) (AcceptanceView, error) {
	s.viewMu.RLock()
	cached, exists := s.stableViews[id]
	s.viewMu.RUnlock()
	if exists {
		return cached, nil
	}
	acceptance, err := s.repository.Load(ctx, id)
	if err != nil {
		return AcceptanceView{}, err
	}
	view := BuildView(acceptance)
	if view.Status == domain.StatusFrozen {
		s.viewMu.Lock()
		// Avoid writing back a stale Frozen projection when a concurrent
		// mutation already committed a newer version for this acceptance.
		if existing, ok := s.stableViews[id]; !ok || existing.Version <= view.Version {
			s.stableViews[id] = view
		}
		s.viewMu.Unlock()
	}
	return view, nil
}

func (s *Service) List(ctx context.Context) ([]AcceptanceView, error) {
	items, err := s.repository.List(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]AcceptanceView, 0, len(items))
	for _, item := range items {
		views = append(views, BuildView(item))
	}
	return views, nil
}
