package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"sonarqa/internal/application"
	"sonarqa/internal/domain"
)

type Store struct {
	mu           sync.RWMutex
	directory    string
	eventPath    string
	snapshotPath string
	aggregates   map[string]*domain.SurveyAcceptance
	idempotency  map[string]json.RawMessage
	sequence     int64
	lastHash     string
}

func Open(directory string) (*Store, error) {
	if directory == "" {
		return nil, fmt.Errorf("存储目录不能为空")
	}
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, fmt.Errorf("创建存储目录: %w", err)
	}
	store := &Store{
		directory: directory, eventPath: filepath.Join(directory, "events.jsonl"),
		snapshotPath: filepath.Join(directory, "snapshot.json"), aggregates: make(map[string]*domain.SurveyAcceptance),
		idempotency: make(map[string]json.RawMessage),
	}
	if err := store.recover(); err != nil {
		return nil, err
	}
	return store, nil
}

func idemMapKey(operation, key string) string { return operation + "\x00" + key }

func (s *Store) recover() error {
	events, err := readEvents(s.eventPath)
	if err != nil {
		return err
	}
	snapshot, err := readSnapshot(s.snapshotPath)
	if err != nil {
		return err
	}
	if err := validateSnapshotAnchor(snapshot, events); err != nil {
		return err
	}
	start := int64(0)
	if snapshot != nil {
		s.aggregates, err = cloneMap(snapshot.Aggregates)
		if err != nil {
			return err
		}
		for _, value := range snapshot.Idempotency {
			s.idempotency[idemMapKey(value.Operation, value.Key)] = append(json.RawMessage(nil), value.Result...)
		}
		start = snapshot.EventSequence
		s.sequence, s.lastHash = snapshot.EventSequence, snapshot.EventHash
	}
	for _, event := range events[start:] {
		if err := s.applyEvent(event); err != nil {
			return fmt.Errorf("重放事件 %d: %w", event.Sequence, err)
		}
	}
	return nil
}

func (s *Store) applyEvent(event eventRecord) error {
	copyValue, err := cloneAcceptance(event.State)
	if err != nil {
		return err
	}
	s.aggregates[event.AcceptanceID] = copyValue
	s.idempotency[idemMapKey(event.Operation, event.IdempotencyKey)] = append(json.RawMessage(nil), event.Result...)
	s.sequence, s.lastHash = event.Sequence, event.Hash
	return nil
}

func (s *Store) Load(_ context.Context, id string) (*domain.SurveyAcceptance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, exists := s.aggregates[id]
	if !exists {
		return nil, domain.ErrNotFound
	}
	return cloneAcceptance(value)
}

func (s *Store) List(_ context.Context) ([]*domain.SurveyAcceptance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.aggregates))
	for id := range s.aggregates {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]*domain.SurveyAcceptance, 0, len(ids))
	for _, id := range ids {
		copyValue, err := cloneAcceptance(s.aggregates[id])
		if err != nil {
			return nil, err
		}
		result = append(result, copyValue)
	}
	return result, nil
}

func (s *Store) Audit(_ context.Context, acceptanceID string, cursor int64, limit int, operation string) ([]domain.AuditEvent, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, exists := s.aggregates[acceptanceID]; !exists {
		return nil, 0, domain.ErrNotFound
	}
	events, err := readEvents(s.eventPath)
	if err != nil {
		return nil, 0, domain.NewError("AUDIT_CHAIN_INVALID", err.Error())
	}
	result := make([]domain.AuditEvent, 0, limit)
	next := int64(0)
	expectedVersion := int64(1)
	for _, event := range events {
		if event.AcceptanceID == acceptanceID {
			if event.AggregateVersion != expectedVersion {
				return nil, 0, domain.NewError("AUDIT_CHAIN_INVALID", "任务聚合版本不连续")
			}
			expectedVersion++
		}
		matchesOperation := operation == "" || event.Operation == operation || strings.HasPrefix(event.Operation, operation+":")
		if event.AcceptanceID != acceptanceID || event.Sequence <= cursor || !matchesOperation {
			continue
		}
		view := domain.AuditEvent{Sequence: event.Sequence, Operation: event.Operation, Actor: event.Actor, OccurredAt: event.OccurredAt, AggregateVersion: event.AggregateVersion, PreviousHash: event.PreviousHash, Hash: event.Hash}
		if event.Operation == operationName("freeze", acceptanceID) && event.State.Manifest != nil {
			if current := s.aggregates[acceptanceID].Manifest; current == nil || current.ManifestHash != event.State.Manifest.ManifestHash {
				return nil, 0, domain.NewError("AUDIT_CHAIN_INVALID", "冻结事件与当前清单不一致")
			}
			view.ManifestHash = event.State.Manifest.ManifestHash
			view.FrozenBoundary = true
		}
		if event.Operation == operationName("release", acceptanceID) && event.State.Release != nil {
			if current := s.aggregates[acceptanceID].Release; current == nil || current.VerificationDigest != event.State.Release.VerificationDigest {
				return nil, 0, domain.NewError("AUDIT_CHAIN_INVALID", "放行事件与当前凭据不一致")
			}
			view.VerificationDigest = event.State.Release.VerificationDigest
			view.ReleasedBoundary = true
		}
		if len(result) == limit {
			next = result[len(result)-1].Sequence
			break
		}
		result = append(result, view)
	}
	return result, next, nil
}

func operationName(name, id string) string { return name + ":" + id }

func (s *Store) FindIdempotent(_ context.Context, operation, key string) (json.RawMessage, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, exists := s.idempotency[idemMapKey(operation, key)]
	return append(json.RawMessage(nil), value...), exists, nil
}

func (s *Store) Commit(_ context.Context, request application.CommitRequest) (application.CommitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mapKey := idemMapKey(request.Operation, request.IdempotencyKey)
	if result, exists := s.idempotency[mapKey]; exists {
		return application.CommitResult{Result: append(json.RawMessage(nil), result...), Replayed: true}, nil
	}
	if request.Acceptance == nil || request.Acceptance.ID == "" {
		return application.CommitResult{}, fmt.Errorf("提交缺少任务投影")
	}
	current, exists := s.aggregates[request.Acceptance.ID]
	if request.ExpectedVersion == 0 {
		if exists {
			return application.CommitResult{}, domain.ErrVersionConflict
		}
		for _, aggregate := range s.aggregates {
			if aggregate.ProjectCode == request.Acceptance.ProjectCode {
				return application.CommitResult{}, domain.NewError("PROJECT_CODE_EXISTS", "任务编号已经存在")
			}
		}
		if request.Acceptance.Version != 1 {
			return application.CommitResult{}, fmt.Errorf("新建任务版本必须为 1")
		}
	} else {
		if !exists || current.Version != request.ExpectedVersion {
			return application.CommitResult{}, domain.ErrVersionConflict
		}
		if request.Acceptance.Version != request.ExpectedVersion+1 {
			return application.CommitResult{}, fmt.Errorf("提交版本必须恰好递增一次")
		}
	}
	state, err := cloneAcceptance(request.Acceptance)
	if err != nil {
		return application.CommitResult{}, err
	}
	if err := state.ValidateIntegrity(); err != nil {
		return application.CommitResult{}, fmt.Errorf("拒绝提交无效业务投影: %w", err)
	}
	event := eventRecord{
		Sequence: s.sequence + 1, PreviousHash: s.lastHash, Operation: request.Operation,
		AcceptanceID: state.ID, AggregateVersion: state.Version, Actor: request.Actor,
		OccurredAt: request.OccurredAt.UTC(), IdempotencyKey: request.IdempotencyKey,
		Result: append(json.RawMessage(nil), request.Result...), State: state,
	}
	event.Hash, err = calculateEventHash(event)
	if err != nil {
		return application.CommitResult{}, err
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return application.CommitResult{}, fmt.Errorf("编码事件: %w", err)
	}
	encoded = append(encoded, '\n')
	file, err := os.OpenFile(s.eventPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return application.CommitResult{}, fmt.Errorf("打开事件日志: %w", err)
	}
	_, writeErr := file.Write(encoded)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return application.CommitResult{}, fmt.Errorf("持久化事件: %w", err)
	}
	if err := s.applyEvent(event); err != nil {
		return application.CommitResult{}, err
	}
	if err := s.writeSnapshot(request.OccurredAt); err != nil {
		return application.CommitResult{}, err
	}
	return application.CommitResult{Result: append(json.RawMessage(nil), request.Result...)}, nil
}

func (s *Store) writeSnapshot(now time.Time) error {
	aggregates, err := cloneMap(s.aggregates)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(s.idempotency))
	for key := range s.idempotency {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]idempotentValue, 0, len(keys))
	for _, combined := range keys {
		operation, key, ok := splitIdemKey(combined)
		if !ok {
			return fmt.Errorf("内存幂等索引键损坏")
		}
		values = append(values, idempotentValue{Operation: operation, Key: key, Result: append(json.RawMessage(nil), s.idempotency[combined]...)})
	}
	snapshot := snapshotFile{
		SchemaVersion: snapshotSchemaVersion, EventSequence: s.sequence, EventHash: s.lastHash,
		Aggregates: aggregates, Idempotency: values, SavedAt: now.UTC(),
	}
	snapshot.Checksum, err = calculateSnapshotHash(snapshot)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("编码投影快照: %w", err)
	}
	temporary, err := os.CreateTemp(s.directory, ".snapshot-*.tmp")
	if err != nil {
		return fmt.Errorf("创建快照临时文件: %w", err)
	}
	temporaryName := temporary.Name()
	cleanup := func() { _ = os.Remove(temporaryName) }
	if err := temporary.Chmod(0o640); err != nil {
		_ = temporary.Close()
		cleanup()
		return err
	}
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		cleanup()
		return fmt.Errorf("写入投影快照: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		cleanup()
		return fmt.Errorf("同步投影快照: %w", err)
	}
	if err := temporary.Close(); err != nil {
		cleanup()
		return fmt.Errorf("关闭投影快照: %w", err)
	}
	if err := os.Rename(temporaryName, s.snapshotPath); err != nil {
		cleanup()
		return fmt.Errorf("原子替换投影快照: %w", err)
	}
	directory, err := os.Open(s.directory)
	if err != nil {
		return fmt.Errorf("打开存储目录: %w", err)
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return fmt.Errorf("同步存储目录: %w", err)
	}
	return nil
}

func splitIdemKey(value string) (string, string, bool) {
	for index := 0; index < len(value); index++ {
		if value[index] == 0 {
			return value[:index], value[index+1:], true
		}
	}
	return "", "", false
}

var _ application.Repository = (*Store)(nil)
