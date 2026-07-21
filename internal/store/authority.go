package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/swornagent/sworn/internal/policy"
	"github.com/swornagent/sworn/internal/protocol"
)

const (
	authoritySourceRecordKind = "sworn-authority-source-v1"
	authoritySourceMediaType  = "application/vnd.sworn.authority-source+json"
	authorityProofMediaType   = "application/vnd.sworn.authority-proof+json"
)

// PutAuthoritySource records an authenticated source observation even when it
// is revoked or no longer current. It never mints an approval receipt.
func (s *Store) PutAuthoritySource(ctx context.Context, prepared policy.PreparedSource) error {
	return s.putAuthoritySource(ctx, prepared, false)
}

// PutCurrentAuthoritySource records an authenticated source observation only
// when it is the current durable head for its source reference. Unlike
// PutAuthoritySource, an exact historical replay is rejected: this operation
// is the ledger half of policy's current-authority permit boundary.
func (s *Store) PutCurrentAuthoritySource(ctx context.Context, prepared policy.PreparedSource) error {
	return s.putAuthoritySource(ctx, prepared, true)
}

func (s *Store) putAuthoritySource(
	ctx context.Context,
	prepared policy.PreparedSource,
	requireCurrentHead bool,
) error {
	if s.readOnly {
		return errors.New("control store is read-only")
	}
	plan := prepared.Plan()
	planRecord := plan.Record()
	facts := prepared.Facts()
	closure := prepared.Closure()
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin authority source storage: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	if err := putAuthoritySourceTransaction(
		ctx, transaction, plan, planRecord, facts, closure, nil, requireCurrentHead,
		s.now().UTC().UnixMicro(),
	); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit authority source storage: %w", err)
	}
	return nil
}

// PutAuthorityApproval atomically persists the authenticated source closure,
// exact plan, Baton receipt, and write-once receipt identity. Only policy's
// engine-owned Authority service can mint the opaque input capability.
func (s *Store) PutAuthorityApproval(ctx context.Context, prepared policy.PreparedApproval) error {
	if s.readOnly {
		return errors.New("control store is read-only")
	}
	source := prepared.Source()
	plan := source.Plan()
	planRecord := plan.Record()
	sourceFacts := source.Facts()
	sourceClosure := source.Closure()
	approvalFacts := prepared.Facts()
	receipt := prepared.Receipt()
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin authority approval storage: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	now := s.now().UTC().UnixMicro()
	if err := putAuthoritySourceTransaction(
		ctx, transaction, plan, planRecord, sourceFacts, sourceClosure, &approvalFacts, false, now,
	); err != nil {
		return err
	}
	if err := putArtifact(
		ctx, transaction, approvalFacts.ReceiptDigest, "application/json",
		receipt.CanonicalJSON, now,
	); err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO authority_approvals (
			receipt_id, receipt_digest, plan_digest, authority_digest,
			source_ref, source_version, source_digest, source_artifact_digest,
			proof_digest, proof_canonical_digest, root_key_id, authorizer_ref,
			approved_at, recorded_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`,
		approvalFacts.ReceiptID, approvalFacts.ReceiptDigest,
		sourceFacts.PlanDigest, sourceFacts.AuthorityDigest,
		sourceFacts.SourceRef, sourceFacts.SourceVersion, sourceFacts.SourceCanonicalDigest,
		sourceFacts.SourceRawDigest, sourceFacts.ProofRawDigest, sourceFacts.ProofCanonicalDigest,
		sourceFacts.RootKeyID, sourceFacts.AuthorizerRef, sourceFacts.ApprovedAt, now,
	); err != nil {
		return fmt.Errorf("put authority approval: %w", err)
	}
	if err := verifyApprovalProjection(ctx, transaction, sourceFacts, approvalFacts); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit authority approval storage: %w", err)
	}
	return nil
}

func putAuthoritySourceTransaction(
	ctx context.Context,
	transaction *sql.Tx,
	plan protocol.ExactPlan,
	planRecord protocol.EncodedRecord,
	facts policy.SourceFacts,
	closure policy.SourceClosure,
	approval *policy.ApprovalFacts,
	requireCurrentHead bool,
	now int64,
) error {
	if err := putPlanTransaction(ctx, transaction, plan, planRecord, now); err != nil {
		return err
	}
	if err := putRecordTransaction(ctx, transaction, protocol.EncodedRecord{
		Kind: authoritySourceRecordKind, CanonicalJSON: closure.SourceCanonical,
		Digest: facts.SourceCanonicalDigest,
	}, now, "authority source"); err != nil {
		return err
	}
	if err := putArtifact(
		ctx, transaction, facts.SourceRawDigest, authoritySourceMediaType, closure.SourceRaw, now,
	); err != nil {
		return err
	}
	if err := putArtifact(
		ctx, transaction, facts.ProofRawDigest, authorityProofMediaType, closure.ProofRaw, now,
	); err != nil {
		return err
	}
	if err := enforceSourceMonotonicity(ctx, transaction, facts, approval, requireCurrentHead); err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO authority_source_snapshots (
			source_ref, source_version, source_id, source_digest, status,
			repository_id, target_ref, authorizer_ref, valid_from, valid_until,
			authenticated_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`,
		facts.SourceRef, facts.SourceVersion, facts.SourceID, facts.SourceCanonicalDigest,
		facts.SourceStatus, facts.Repository, facts.TargetRef, facts.AuthorizerRef,
		facts.ValidFrom, facts.ValidUntil, now,
	); err != nil {
		return fmt.Errorf("put authority source snapshot: %w", err)
	}
	if err := verifySourceSnapshot(ctx, transaction, facts); err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO authority_source_authentications (
			source_ref, source_version, source_digest, source_artifact_digest,
			proof_digest, proof_canonical_digest, plan_digest, authority_digest,
			root_key_id, approved_at, authenticated_at_us
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`,
		facts.SourceRef, facts.SourceVersion, facts.SourceCanonicalDigest, facts.SourceRawDigest,
		facts.ProofRawDigest, facts.ProofCanonicalDigest, facts.PlanDigest, facts.AuthorityDigest,
		facts.RootKeyID, facts.ApprovedAt, now,
	); err != nil {
		return fmt.Errorf("put authority source authentication: %w", err)
	}
	return verifySourceAuthentication(ctx, transaction, facts)
}

func enforceSourceMonotonicity(
	ctx context.Context,
	transaction *sql.Tx,
	facts policy.SourceFacts,
	approval *policy.ApprovalFacts,
	requireCurrentHead bool,
) error {
	var digest string
	var version int64
	err := transaction.QueryRowContext(ctx, `
		SELECT source_version, source_digest
		FROM authority_source_snapshots
		WHERE source_ref = ? ORDER BY source_version DESC LIMIT 1`, facts.SourceRef,
	).Scan(&version, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		// A fresh ledger may first encounter an already-advanced source. Treat
		// that authenticated version as the high-water mark; insisting on v1
		// would discard a signed revocation and permit a later stale v1 replay.
		return nil
	}
	if err != nil {
		return fmt.Errorf("read authority source head: %w", err)
	}
	if facts.SourceVersion < version {
		if requireCurrentHead {
			return errors.New("authority source version rollback")
		}
		if approval != nil {
			var existingDigest string
			existingErr := transaction.QueryRowContext(ctx,
				"SELECT receipt_digest FROM authority_approvals WHERE receipt_id = ?", approval.ReceiptID,
			).Scan(&existingDigest)
			if existingErr == nil && existingDigest == approval.ReceiptDigest {
				return nil
			}
			if existingErr != nil && !errors.Is(existingErr, sql.ErrNoRows) {
				return fmt.Errorf("check historical authority approval replay: %w", existingErr)
			}
		} else {
			var exact int
			exactErr := transaction.QueryRowContext(ctx, `
				SELECT 1 FROM authority_source_authentications
				WHERE source_ref = ? AND source_version = ? AND source_digest = ?
				  AND source_artifact_digest = ? AND proof_digest = ?
				  AND proof_canonical_digest = ? AND plan_digest = ?
				  AND authority_digest = ? AND root_key_id = ? AND approved_at = ?`,
				facts.SourceRef, facts.SourceVersion, facts.SourceCanonicalDigest,
				facts.SourceRawDigest, facts.ProofRawDigest, facts.ProofCanonicalDigest,
				facts.PlanDigest, facts.AuthorityDigest, facts.RootKeyID, facts.ApprovedAt,
			).Scan(&exact)
			if exactErr == nil {
				return nil
			}
			if !errors.Is(exactErr, sql.ErrNoRows) {
				return fmt.Errorf("check historical authority source replay: %w", exactErr)
			}
		}
		return errors.New("authority source version rollback")
	}
	if facts.SourceVersion == version && facts.SourceCanonicalDigest != digest {
		return errors.New("authority source version fork")
	}
	return nil
}

func verifySourceSnapshot(ctx context.Context, source rowQuerier, facts policy.SourceFacts) error {
	var sourceID, sourceDigest, status, repositoryID, targetRef, authorizerRef, validFrom, validUntil string
	err := source.QueryRowContext(ctx, `
		SELECT source_id, source_digest, status, repository_id, target_ref,
		       authorizer_ref, valid_from, valid_until
		FROM authority_source_snapshots WHERE source_ref = ? AND source_version = ?`,
		facts.SourceRef, facts.SourceVersion,
	).Scan(&sourceID, &sourceDigest, &status, &repositoryID, &targetRef, &authorizerRef, &validFrom, &validUntil)
	if err != nil {
		return fmt.Errorf("verify authority source snapshot: %w", err)
	}
	if sourceID != facts.SourceID || sourceDigest != facts.SourceCanonicalDigest || status != facts.SourceStatus ||
		repositoryID != facts.Repository || targetRef != facts.TargetRef || authorizerRef != facts.AuthorizerRef ||
		validFrom != facts.ValidFrom || validUntil != facts.ValidUntil {
		return errors.New("authority source snapshot conflicts with authenticated facts")
	}
	return nil
}

func verifySourceAuthentication(ctx context.Context, source rowQuerier, facts policy.SourceFacts) error {
	var proofCanonicalDigest, planDigest, authorityDigest, rootKeyID, approvedAt string
	err := source.QueryRowContext(ctx, `
		SELECT proof_canonical_digest, plan_digest, authority_digest, root_key_id, approved_at
		FROM authority_source_authentications
		WHERE source_ref = ? AND source_version = ?
		  AND source_artifact_digest = ? AND proof_digest = ?`,
		facts.SourceRef, facts.SourceVersion, facts.SourceRawDigest, facts.ProofRawDigest,
	).Scan(&proofCanonicalDigest, &planDigest, &authorityDigest, &rootKeyID, &approvedAt)
	if err != nil {
		return fmt.Errorf("verify authority source authentication: %w", err)
	}
	if proofCanonicalDigest != facts.ProofCanonicalDigest || planDigest != facts.PlanDigest ||
		authorityDigest != facts.AuthorityDigest || rootKeyID != facts.RootKeyID || approvedAt != facts.ApprovedAt {
		return errors.New("authority source authentication conflicts with verified proof")
	}
	return nil
}

func verifyApprovalProjection(
	ctx context.Context,
	source rowQuerier,
	sourceFacts policy.SourceFacts,
	approvalFacts policy.ApprovalFacts,
) error {
	var receiptDigest, planDigest, authorityDigest, sourceRef, sourceDigest string
	var proofCanonicalDigest, rootKeyID, authorizerRef, approvedAt string
	var sourceVersion int64
	err := source.QueryRowContext(ctx, `
		SELECT receipt_digest, plan_digest, authority_digest, source_ref, source_version,
		       source_digest, proof_canonical_digest, root_key_id, authorizer_ref, approved_at
		FROM authority_approvals WHERE receipt_id = ?`, approvalFacts.ReceiptID,
	).Scan(&receiptDigest, &planDigest, &authorityDigest, &sourceRef, &sourceVersion,
		&sourceDigest, &proofCanonicalDigest, &rootKeyID, &authorizerRef, &approvedAt)
	if err != nil {
		return fmt.Errorf("verify authority approval projection: %w", err)
	}
	if receiptDigest != approvalFacts.ReceiptDigest || planDigest != sourceFacts.PlanDigest ||
		authorityDigest != sourceFacts.AuthorityDigest || sourceRef != sourceFacts.SourceRef ||
		sourceVersion != sourceFacts.SourceVersion || sourceDigest != sourceFacts.SourceCanonicalDigest ||
		proofCanonicalDigest != sourceFacts.ProofCanonicalDigest || rootKeyID != sourceFacts.RootKeyID ||
		authorizerRef != sourceFacts.AuthorizerRef || approvedAt != sourceFacts.ApprovedAt {
		return errors.New("authority approval conflicts with authenticated facts")
	}
	return nil
}
