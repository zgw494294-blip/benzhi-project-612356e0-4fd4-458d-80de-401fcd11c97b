package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"sonarqa/internal/domain"
)

func checksum(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func calculateEventHash(event eventRecord) (string, error) {
	return checksum(eventHashInput{
		Sequence: event.Sequence, PreviousHash: event.PreviousHash, Operation: event.Operation,
		AcceptanceID: event.AcceptanceID, AggregateVersion: event.AggregateVersion,
		Actor: event.Actor, OccurredAt: event.OccurredAt, IdempotencyKey: event.IdempotencyKey,
		Result: event.Result, State: event.State,
	})
}

func calculateSnapshotHash(snapshot snapshotFile) (string, error) {
	return checksum(snapshotHashInput{
		SchemaVersion: snapshot.SchemaVersion, EventSequence: snapshot.EventSequence,
		EventHash: snapshot.EventHash, Aggregates: snapshot.Aggregates,
		Idempotency: snapshot.Idempotency, SavedAt: snapshot.SavedAt,
	})
}

func cloneAcceptance(value *domain.SurveyAcceptance) (*domain.SurveyAcceptance, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("编码任务投影: %w", err)
	}
	var result domain.SurveyAcceptance
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, fmt.Errorf("复制任务投影: %w", err)
	}
	return &result, nil
}

func cloneMap(values map[string]*domain.SurveyAcceptance) (map[string]*domain.SurveyAcceptance, error) {
	result := make(map[string]*domain.SurveyAcceptance, len(values))
	for key, value := range values {
		copyValue, err := cloneAcceptance(value)
		if err != nil {
			return nil, err
		}
		result[key] = copyValue
	}
	return result, nil
}
