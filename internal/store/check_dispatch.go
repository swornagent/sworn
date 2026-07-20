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

	plan, err := loadExactPlan(ctx, transaction, current.PlanDigest)
	if err != nil {
		return fmt.Errorf("load exact delivery plan: %w", err)
	}
	target := plan.Target()
	if plan.DeliveryID() != current.DeliveryID ||
		target.Repository != current.Repository || target.Ref != current.TargetRef {
		return errors.New("engine state does not match its exact delivery plan")
	}
	planWorkIDs := plan.WorkIDs()
	if len(current.Work) != len(planWorkIDs) {
		return errors.New("engine state work does not exactly match the delivery plan")
	}
	for index, workID := range planWorkIDs {
		if current.Work[index].ID != workID {
			return errors.New("engine state work does not exactly match the delivery plan")
		}
	}
	var workAttempt int64
	for _, work := range current.Work {
		if work.ID == payload.WorkID && work.State == engine.WorkActive {
			workAttempt = work.Attempt
		}
	}
	if workAttempt < 1 {
		return fmt.Errorf("work %q is not active for check dispatch", payload.WorkID)
	}

	resolver := journalResultResolver{query: transaction}
	selection, err := protocol.ResolveExactLocalChecks(ctx, resolver, plan, payload.WorkID)
	if err != nil {
		return fmt.Errorf("resolve exact local checks: %w", err)
	}
	requirements := selection.Requirements()
	if len(payload.Checks) != len(requirements) || len(decision.Effects) != len(requirements) {
		return errors.New("check dispatch does not exactly cover the plan-selected checks")
	}

	builder, err := resolver.SucceededEffect(ctx, payload.BuilderEffectID)
	if err != nil {
		return fmt.Errorf("resolve check dispatch builder: %w", err)
	}
	if builder.Kind != engine.EffectBuild || builder.DeliveryRunID != current.RunID {
		return errors.New("check dispatch builder belongs to a different delivery journal")
	}
	buildRequest, err := engine.ParseBuildEffectRequest(builder.Request)
	if err != nil {
		return fmt.Errorf("parse check dispatch builder request: %w", err)
	}
	buildResult, err := engine.ParseBuildEffectResult(builder.Result)
	if err != nil {
		return fmt.Errorf("parse check dispatch builder result: %w", err)
	}
	if buildRequest.DeliveryRunID != current.RunID || buildRequest.DeliveryID != current.DeliveryID ||
		buildRequest.WorkID != payload.WorkID || buildRequest.WorkAttempt != workAttempt ||
		buildRequest.DispatchDigest != selection.ContractDigest() {
		return errors.New("check dispatch builder does not match the current exact work attempt")
	}
	if buildResult.Candidate.RepositoryID != target.Repository || buildResult.Candidate.TargetRef != target.Ref {
		return errors.New("check dispatch builder candidate does not match the exact plan target")
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
			WorkID: payload.WorkID, WorkAttempt: workAttempt, BuilderEffectID: builder.ID,
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
	var authorityReceiptDigest string
	err = transaction.QueryRowContext(ctx, `
		SELECT receipt_digest FROM authority_approvals
		WHERE receipt_digest = ? AND plan_digest = ?`,
		current.AuthorityReceiptDigest, current.PlanDigest,
	).Scan(&authorityReceiptDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("active delivery lacks an immutable authority approval for its exact plan")
	}
	if err != nil {
		return fmt.Errorf("load check dispatch authority approval: %w", err)
	}
	receiptType, receiptBytes, err := resolver.Artifact(ctx, authorityReceiptDigest)
	if err != nil {
		return fmt.Errorf("resolve check dispatch authority receipt: %w", err)
	}
	if receiptType != "application/json" {
		return errors.New("check dispatch authority receipt is not application/json")
	}
	if err := protocol.ValidateAuthorityApprovalForBuilder(
		receiptBytes, plan, buildResult.Builder,
	); err != nil {
		return fmt.Errorf("validate check dispatch authority receipt: %w", err)
	}
	return nil
}
