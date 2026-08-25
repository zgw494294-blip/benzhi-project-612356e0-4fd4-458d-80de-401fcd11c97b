package application

import (
	"context"
	"encoding/json"

	"sonarqa/internal/domain"
)

func (s *Service) Create(ctx context.Context, command CreateAcceptanceCommand) (MutationResult, error) {
	if err := command.Actor.Require(RoleProcessor); err != nil {
		return MutationResult{}, err
	}
	op := operation("create", command.ProjectCode)
	var replayed MutationResult
	if ok, err := s.replay(ctx, op, command.IdempotencyKey, &replayed); err != nil || ok {
		return replayed, err
	}
	id := s.ids.NewID("acc")
	acceptance, err := domain.NewAcceptance(id, command.ProjectCode, command.AreaBoundary, command.CoordinateReference, command.QualityThresholds, command.PlannedLineIDs, command.Actor.ID, s.clock.Now())
	if err != nil {
		return MutationResult{}, err
	}
	result := MutationResult{AcceptanceID: id, Version: acceptance.Version, Status: acceptance.Status, ResourceID: id}
	raw, err := json.Marshal(result)
	if err != nil {
		return MutationResult{}, err
	}
	commit, err := s.repository.Commit(ctx, CommitRequest{
		Acceptance: acceptance, ExpectedVersion: 0, Operation: op, IdempotencyKey: command.IdempotencyKey,
		Result: raw, Actor: command.Actor.ID, OccurredAt: s.clock.Now(),
	})
	if err != nil {
		return MutationResult{}, err
	}
	if commit.Replayed {
		if err := json.Unmarshal(commit.Result, &result); err != nil {
			return MutationResult{}, err
		}
		result.Replayed = true
	}
	return result, nil
}

func (s *Service) SubmitRevision(ctx context.Context, command SubmitRevisionCommand) (MutationResult, error) {
	if err := command.Actor.Require(RoleProcessor); err != nil {
		return MutationResult{}, err
	}
	op := operation("submit-revision", command.AcceptanceID)
	var replayed MutationResult
	if ok, err := s.replay(ctx, op, command.IdempotencyKey, &replayed); err != nil || ok {
		return replayed, err
	}
	acceptance, err := s.repository.Load(ctx, command.AcceptanceID)
	if err != nil {
		return MutationResult{}, err
	}
	if err := checkVersion(acceptance, command.ExpectedVersion); err != nil {
		return MutationResult{}, err
	}
	sequence := 1
	if latest, exists := acceptance.LatestRevision(command.LineID); exists {
		sequence = latest.Sequence + 1
	}
	revision := domain.SonarLineRevision{
		RevisionID: s.ids.NewID("rev"), AcceptanceID: acceptance.ID, LineID: command.LineID,
		Sequence: sequence, CoverageSamples: command.CoverageSamples, EchoGapRatio: command.EchoGapRatio,
		HeadingDeviation: command.HeadingDeviation, PositionConfidence: command.PositionConfidence,
		SideLobeNoise: command.SideLobeNoise, CalibrationRef: command.CalibrationRef,
		SubmittedBy: command.Actor.ID, SubmittedAt: s.clock.Now(),
	}
	if err := acceptance.SubmitRevision(revision); err != nil {
		return MutationResult{}, err
	}
	result := MutationResult{AcceptanceID: acceptance.ID, Version: acceptance.Version, Status: acceptance.Status, ResourceID: revision.RevisionID}
	return s.commitMutation(ctx, acceptance, command.ExpectedVersion, op, command.IdempotencyKey, command.Actor, result)
}
