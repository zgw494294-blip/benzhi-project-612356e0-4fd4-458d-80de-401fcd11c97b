package application

import (
	"context"

	"sonarqa/internal/domain"
)

func (s *Service) ReviewFinding(ctx context.Context, command ReviewFindingCommand) (MutationResult, error) {
	if err := command.Actor.Require(RoleReviewer); err != nil {
		return MutationResult{}, err
	}
	op := operation("review-finding:"+command.FindingID, command.AcceptanceID)
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
	if err := acceptance.ReviewFinding(command.FindingID, command.Actor.ID, command.Note, command.Approved, s.clock.Now()); err != nil {
		return MutationResult{}, err
	}
	result := MutationResult{AcceptanceID: acceptance.ID, Version: acceptance.Version, Status: acceptance.Status, ResourceID: command.FindingID}
	return s.commitMutation(ctx, acceptance, command.ExpectedVersion, op, command.IdempotencyKey, command.Actor, result)
}

func (s *Service) DecideReview(ctx context.Context, command DecideReviewCommand) (MutationResult, error) {
	if err := command.Actor.Require(RoleReviewer); err != nil {
		return MutationResult{}, err
	}
	op := operation("review-decision", command.AcceptanceID)
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
	if err := acceptance.DecideReview(command.Actor.ID, command.Note, command.Approved); err != nil {
		return MutationResult{}, err
	}
	result := MutationResult{AcceptanceID: acceptance.ID, Version: acceptance.Version, Status: acceptance.Status}
	return s.commitMutation(ctx, acceptance, command.ExpectedVersion, op, command.IdempotencyKey, command.Actor, result)
}

func (s *Service) Freeze(ctx context.Context, command FreezeCommand) (MutationResult, error) {
	if err := command.Actor.Require(RoleReviewer); err != nil {
		return MutationResult{}, err
	}
	op := operation("freeze", command.AcceptanceID)
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
	manifest, err := acceptance.Freeze(command.Actor.ID, s.clock.Now())
	if err != nil {
		return MutationResult{}, err
	}
	result := MutationResult{AcceptanceID: acceptance.ID, Version: acceptance.Version, Status: acceptance.Status, ResourceID: manifest.ManifestHash}
	return s.commitMutation(ctx, acceptance, command.ExpectedVersion, op, command.IdempotencyKey, command.Actor, result)
}

func (s *Service) Release(ctx context.Context, command ReleaseCommand) (MutationResult, error) {
	if err := command.Actor.Require(RoleArchivist); err != nil {
		return MutationResult{}, err
	}
	op := operation("release", command.AcceptanceID)
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
	release, err := acceptance.IssueRelease(s.ids.NewID("release"), command.Actor.ID, s.clock.Now())
	if err != nil {
		return MutationResult{}, err
	}
	result := MutationResult{AcceptanceID: acceptance.ID, Version: acceptance.Version, Status: acceptance.Status, ResourceID: release.ReleaseID}
	return s.commitMutation(ctx, acceptance, command.ExpectedVersion, op, command.IdempotencyKey, command.Actor, result)
}

func (s *Service) Manifest(ctx context.Context, id string) (*domain.FrozenManifest, error) {
	acceptance, err := s.repository.Load(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := acceptance.VerifyManifest(); err != nil {
		return nil, err
	}
	copyValue := *acceptance.Manifest
	return &copyValue, nil
}

func (s *Service) ReleaseCredential(ctx context.Context, id string) (*domain.ArchiveRelease, error) {
	acceptance, err := s.repository.Load(ctx, id)
	if err != nil {
		return nil, err
	}
	if acceptance.Release == nil {
		return nil, domain.NewError("RELEASE_NOT_FOUND", "放行凭据不存在")
	}
	if err := domain.VerifyRelease(*acceptance.Release); err != nil {
		return nil, err
	}
	copyValue := *acceptance.Release
	return &copyValue, nil
}
