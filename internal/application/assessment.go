package application

import (
	"context"

	"sonarqa/internal/domain"
)

func (s *Service) Evaluate(ctx context.Context, command EvaluateCommand) (MutationResult, error) {
	if err := command.Actor.Require(RoleProcessor); err != nil {
		return MutationResult{}, err
	}
	op := operation("evaluate", command.AcceptanceID)
	var replayed MutationResult
	if ok, err := s.replay(ctx, op, command.IdempotencyKey, &replayed); err != nil || ok {
		return replayed, err
	}
	ctx, finishEvaluation := s.beginEvaluation(ctx)
	defer finishEvaluation()
	acceptance, err := s.repository.Load(ctx, command.AcceptanceID)
	if err != nil {
		return MutationResult{}, err
	}
	if err := checkVersion(acceptance, command.ExpectedVersion); err != nil {
		return MutationResult{}, err
	}
	if err := acceptance.CanEvaluate(); err != nil {
		return MutationResult{}, err
	}
	refs := make(map[string]string, len(acceptance.PlannedLineIDs))
	outcomes := make([]domain.RuleOutcome, 0, len(acceptance.PlannedLineIDs)*5)
	for _, line := range acceptance.PlannedLineIDs {
		revision, _ := acceptance.LatestRevision(line)
		refs[line] = revision.RevisionID
		outcomes = append(outcomes, domain.EvaluateRules(revision, acceptance.QualityThresholds)...)
	}
	domain.SortOutcomes(outcomes)
	blocking := 0
	for _, outcome := range outcomes {
		if !outcome.Passed {
			blocking++
		}
	}
	hash, err := domain.AssessmentHash(acceptance.ID, refs, domain.CurrentRuleSetVersion, outcomes)
	if err != nil {
		return MutationResult{}, err
	}
	assessment := domain.QualityAssessment{
		AssessmentID: s.ids.NewID("asm"), AcceptanceID: acceptance.ID, RevisionRefs: refs,
		RuleSetVersion: domain.CurrentRuleSetVersion, RuleOutcomes: outcomes, BlockingCount: blocking,
		SummaryHash: hash, EvaluatedAt: s.clock.Now(),
	}
	if err := acceptance.ApplyAssessment(assessment, func() string { return s.ids.NewID("finding") }); err != nil {
		return MutationResult{}, err
	}
	result := MutationResult{AcceptanceID: acceptance.ID, Version: acceptance.Version, Status: acceptance.Status, ResourceID: assessment.AssessmentID}
	return s.commitMutation(ctx, acceptance, command.ExpectedVersion, op, command.IdempotencyKey, command.Actor, result)
}

func (s *Service) RemediateFinding(ctx context.Context, command RemediateFindingCommand) (MutationResult, error) {
	if err := command.Actor.Require(RoleProcessor); err != nil {
		return MutationResult{}, err
	}
	op := operation("remediate-finding:"+command.FindingID, command.AcceptanceID)
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
	if err := acceptance.RemediateFinding(command.FindingID, command.Cause, command.Remediation, command.EvidenceRevisionID, command.Actor.ID); err != nil {
		return MutationResult{}, err
	}
	result := MutationResult{AcceptanceID: acceptance.ID, Version: acceptance.Version, Status: acceptance.Status, ResourceID: command.FindingID}
	return s.commitMutation(ctx, acceptance, command.ExpectedVersion, op, command.IdempotencyKey, command.Actor, result)
}

type RemediationBatchCommand struct {
	AcceptanceID    string                       `json:"-"`
	ExpectedVersion int64                        `json:"expectedVersion"`
	Items           []domain.RemediationEvidence `json:"items"`
	Actor           Actor                        `json:"-"`
	IdempotencyKey  string                       `json:"-"`
}

func (s *Service) RemediateFindingsBatch(ctx context.Context, command RemediationBatchCommand) (MutationResult, error) {
	if err := command.Actor.Require(RoleProcessor); err != nil {
		return MutationResult{}, err
	}
	if len(command.Items) > 50 {
		return MutationResult{}, domain.FieldError("items", "批量条目不得超过 50 条")
	}
	op := operation("remediate-findings-batch", command.AcceptanceID)
	var replay MutationResult
	if ok, err := s.replay(ctx, op, command.IdempotencyKey, &replay); err != nil || ok {
		return replay, err
	}
	a, err := s.repository.Load(ctx, command.AcceptanceID)
	if err != nil {
		return MutationResult{}, err
	}
	if err := checkVersion(a, command.ExpectedVersion); err != nil {
		return MutationResult{}, err
	}
	if err := a.RemediateFindingsBatch(command.Items, command.Actor.ID); err != nil {
		return MutationResult{}, err
	}
	result := MutationResult{AcceptanceID: a.ID, Version: a.Version, Status: a.Status, ResourceID: s.ids.NewID("change")}
	return s.commitMutation(ctx, a, command.ExpectedVersion, op, command.IdempotencyKey, command.Actor, result)
}
