package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

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
	if value == nil {
		return &domain.SurveyAcceptance{}, nil
	}
	result := *value
	result.AreaBoundary.Points = append([]domain.Point(nil), value.AreaBoundary.Points...)
	result.PlannedLineIDs = append([]string(nil), value.PlannedLineIDs...)
	result.Revisions = append([]domain.SonarLineRevision(nil), value.Revisions...)
	for index := range result.Revisions {
		result.Revisions[index].CoverageSamples = append([]domain.CoverageSample(nil), value.Revisions[index].CoverageSamples...)
	}
	result.Assessments = append([]domain.QualityAssessment(nil), value.Assessments...)
	for index := range result.Assessments {
		result.Assessments[index].RuleOutcomes = append([]domain.RuleOutcome(nil), value.Assessments[index].RuleOutcomes...)
		revisionRefs := make(map[string]string, len(value.Assessments[index].RevisionRefs))
		for key, ref := range value.Assessments[index].RevisionRefs {
			revisionRefs[key] = ref
		}
		result.Assessments[index].RevisionRefs = revisionRefs
	}
	result.Findings = append([]domain.QualityFinding(nil), value.Findings...)
	for index := range result.Findings {
		if value.Findings[index].ReviewedAt != nil {
			reviewedAt := *value.Findings[index].ReviewedAt
			result.Findings[index].ReviewedAt = &reviewedAt
		}
	}
	if value.Manifest != nil {
		manifest := *value.Manifest
		manifest.Lines = append([]domain.FrozenLine(nil), value.Manifest.Lines...)
		manifest.ApprovedFindings = append([]string(nil), value.Manifest.ApprovedFindings...)
		result.Manifest = &manifest
	}
	if value.Release != nil {
		release := *value.Release
		result.Release = &release
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
