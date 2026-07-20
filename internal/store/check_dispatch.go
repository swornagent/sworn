package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/protocol"
)

func (s *Store) validateCheckDispatchDecision(
	ctx context.Context,
	transaction *sql.Tx,
	current engine.State,
	command engine.Command,
	decision engine.Decision,
) error {
	if !engine.ValidDigest(s.localCheckRuntimeManifestDigest) {
		return errors.New("local-check runtime is not configured for this control process")
	}
	var payload engine.DispatchChecksPayload
	if err := json.Unmarshal(command.Payload, &payload); err != nil {
		return fmt.Errorf("decode check dispatch payload: %w", err)
	}
	if payload.RuntimeManifestDigest != s.localCheckRuntimeManifestDigest {
		return errors.New("check dispatch runtime does not match immutable process configuration")
	}

	attempt, err := loadExactAttempt(ctx, transaction, current, payload.WorkID, engine.WorkActive)
	if err != nil {
		return err
	}
	requirements := attempt.checks.Requirements()
	if len(payload.Checks) != len(requirements) || len(decision.Effects) != len(requirements) {
		return errors.New("check dispatch does not exactly cover the plan-selected checks")
	}
	resolver := journalResultResolver{query: transaction}
	builder, build, err := resolveAttemptBuilder(ctx, resolver, current, attempt, payload.BuilderEffectID)
	if err != nil {
		return err
	}

	for index, requirement := range requirements {
		selected := payload.Checks[index]
		if selected.CheckID != requirement.CheckID ||
			selected.DefinitionDigest != requirement.Definition.Digest {
			return fmt.Errorf("check dispatch selection %d differs from the exact policy order", index)
		}
		effect := decision.Effects[index]
		expected, err := engine.EncodeLocalCheckEffectRequest(engine.LocalCheckEffectRequest{
			SchemaVersion: engine.LocalCheckEffectRequestSchemaVersion,
			DeliveryRunID: current.RunID, DeliveryID: current.DeliveryID,
			WorkID: payload.WorkID, WorkAttempt: attempt.number, BuilderEffectID: builder.ID,
			CheckID: requirement.CheckID, DefinitionDigest: requirement.Definition.Digest,
			RuntimeManifestDigest: s.localCheckRuntimeManifestDigest,
		})
		if err != nil {
			return fmt.Errorf("encode exact check dispatch effect %d: %w", index, err)
		}
		if effect.Kind != engine.EffectLocalCheck || !bytes.Equal(effect.Request, expected) {
			return fmt.Errorf("check dispatch effect %d does not match its exact derived request", index)
		}
	}
	_, err = loadHistoricalAuthority(ctx, transaction, current, attempt.plan, build.Builder)
	return err
}

type exactAttempt struct {
	plan   protocol.ExactPlan
	checks protocol.ExactLocalChecks
	workID string
	number int64
}

func loadExactAttempt(
	ctx context.Context,
	transaction *sql.Tx,
	current engine.State,
	workID string,
	want engine.WorkState,
) (exactAttempt, error) {
	plan, err := loadExactPlan(ctx, transaction, current.PlanDigest)
	if err != nil {
		return exactAttempt{}, fmt.Errorf("load exact delivery plan: %w", err)
	}
	target, workIDs := plan.Target(), plan.WorkIDs()
	if plan.DeliveryID() != current.DeliveryID || target.Repository != current.Repository ||
		target.Ref != current.TargetRef || len(current.Work) != len(workIDs) {
		return exactAttempt{}, errors.New("engine state does not match its exact delivery plan")
	}
	var number int64
	for index, plannedID := range workIDs {
		if current.Work[index].ID != plannedID {
			return exactAttempt{}, errors.New("engine state work does not exactly match the delivery plan")
		}
		if plannedID == workID && current.Work[index].State == want {
			number = current.Work[index].Attempt
		}
	}
	if number < 1 {
		return exactAttempt{}, fmt.Errorf("work %q is not %s at a positive attempt", workID, want)
	}
	checks, err := protocol.ResolveExactLocalChecks(ctx, journalResultResolver{query: transaction}, plan, workID)
	if err != nil {
		return exactAttempt{}, fmt.Errorf("resolve exact local checks: %w", err)
	}
	return exactAttempt{plan: plan, checks: checks, workID: workID, number: number}, nil
}

func resolveAttemptBuilder(
	ctx context.Context,
	resolver journalResultResolver,
	current engine.State,
	attempt exactAttempt,
	effectID string,
) (engine.JournalEffect, engine.BuildEffectResult, error) {
	builder, err := resolver.SucceededEffect(ctx, effectID)
	if err != nil {
		return engine.JournalEffect{}, engine.BuildEffectResult{}, fmt.Errorf("resolve attempt builder: %w", err)
	}
	request, requestErr := engine.ParseBuildEffectRequest(builder.Request)
	result, resultErr := engine.ParseBuildEffectResult(builder.Result)
	if requestErr != nil || resultErr != nil || builder.Kind != engine.EffectBuild ||
		builder.DeliveryRunID != current.RunID || request.DeliveryRunID != current.RunID ||
		request.DeliveryID != current.DeliveryID || request.WorkID != attempt.workID ||
		request.WorkAttempt != attempt.number || request.DispatchDigest != attempt.checks.ContractDigest() {
		return engine.JournalEffect{}, engine.BuildEffectResult{}, errors.New("builder does not match the current exact work attempt")
	}
	target := attempt.plan.Target()
	if result.Candidate.RepositoryID != target.Repository || result.Candidate.TargetRef != target.Ref {
		return engine.JournalEffect{}, engine.BuildEffectResult{}, errors.New("builder candidate does not match the exact plan target")
	}
	return builder, result, nil
}

func loadHistoricalAuthority(
	ctx context.Context,
	transaction *sql.Tx,
	current engine.State,
	plan protocol.ExactPlan,
	builder protocol.BuilderRun,
) (protocol.Artifact, error) {
	authority, target := plan.Authority(), plan.Target()
	var receiptDigest string
	err := transaction.QueryRowContext(ctx, `
		SELECT approval.receipt_digest
		FROM authority_approvals AS approval
		JOIN authority_source_snapshots AS source
		  ON source.source_ref = approval.source_ref
		 AND source.source_version = approval.source_version
		 AND source.source_digest = approval.source_digest
		JOIN authority_source_authentications AS authentication
		  ON authentication.source_ref = approval.source_ref
		 AND authentication.source_version = approval.source_version
		 AND authentication.source_digest = approval.source_digest
		 AND authentication.source_artifact_digest = approval.source_artifact_digest
		 AND authentication.proof_digest = approval.proof_digest
		 AND authentication.proof_canonical_digest = approval.proof_canonical_digest
		 AND authentication.plan_digest = approval.plan_digest
		 AND authentication.authority_digest = approval.authority_digest
		 AND authentication.root_key_id = approval.root_key_id
		 AND authentication.approved_at = approval.approved_at
		WHERE approval.receipt_digest = ? AND approval.plan_digest = ?
		  AND approval.authority_digest = ? AND approval.source_ref = ?
		  AND source.status = 'active' AND source.repository_id = ? AND source.target_ref = ?`,
		current.AuthorityReceiptDigest, current.PlanDigest, authority.Digest, authority.SourceRef,
		target.Repository, target.Ref,
	).Scan(&receiptDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return protocol.Artifact{}, errors.New("delivery lacks an immutable authenticated approval for its exact plan")
	}
	if err != nil {
		return protocol.Artifact{}, fmt.Errorf("load historical authority approval: %w", err)
	}
	pointer := protocol.Artifact{Ref: receiptDigest, MediaType: "application/json", Digest: receiptDigest}
	mediaType, contents, err := (journalResultResolver{query: transaction}).Artifact(ctx, receiptDigest)
	if err != nil || mediaType != pointer.MediaType {
		return protocol.Artifact{}, errors.New("historical authority receipt artifact is unavailable or invalid")
	}
	if err := protocol.ValidateAuthorityApprovalForBuilder(contents, plan, builder); err != nil {
		return protocol.Artifact{}, fmt.Errorf("validate historical authority receipt: %w", err)
	}
	return pointer, nil
}
