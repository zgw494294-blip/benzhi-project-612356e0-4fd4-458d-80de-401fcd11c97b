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
	repository Repository
	clock      Clock
	ids        IDGenerator
	listMu     sync.RWMutex
	listCache  map[string]json.RawMessage
}

func NewService(repository Repository, clock Clock, ids IDGenerator) *Service {
	return &Service{repository: repository, clock: clock, ids: ids, listCache: make(map[string]json.RawMessage)}
}

func (s *Service) cachedList(status string) (ListResult, bool, error) {
	s.listMu.RLock()
	raw, exists := s.listCache[status]
	raw = append(json.RawMessage(nil), raw...)
	s.listMu.RUnlock()
	if !exists {
		return ListResult{}, false, nil
	}
	var result ListResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return ListResult{}, false, fmt.Errorf("解析列表缓存: %w", err)
	}
	return result, true, nil
}

func (s *Service) cacheList(status string, result ListResult) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("编码列表缓存: %w", err)
	}
	s.listMu.Lock()
	s.listCache[status] = append(json.RawMessage(nil), raw...)
	s.listMu.Unlock()
	return nil
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
	return result, nil
}

func (s *Service) Get(ctx context.Context, id string) (AcceptanceView, error) {
	acceptance, err := s.repository.Load(ctx, id)
	if err != nil {
		return AcceptanceView{}, err
	}
	return BuildView(acceptance), nil
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
