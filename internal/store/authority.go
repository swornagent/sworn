package store

import (
	"bytes"
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
		ctx, transaction, plan, planRecord, facts, closure, nil, s.now().UTC().UnixMicro(),
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
		ctx, transaction, plan, planRecord, sourceFacts, sourceClosure, &approvalFacts, now,
	); err != nil {
		return err
	}
	if err := putArtifactTransaction(
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

// AuthorityApproval restores historical authenticated approval from its exact
// archived closure. It does not issue a current gate permit or re-resolve the
// live authority source.
func (s *Store) AuthorityApproval(
	ctx context.Context,
	receiptDigest string,
	root policy.TrustRoot,
) (policy.HistoricalApproval, error) {
	var sourceFacts policy.SourceFacts
	var approvalFacts policy.ApprovalFacts
	var sourceRecordKind string
	var sourceRecordCanonical []byte
	var sourceType, proofType, receiptType string
	var sourceRaw, proofRaw, receiptRaw []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT
			a.receipt_id, a.receipt_digest, a.plan_digest, a.authority_digest,
			a.source_ref, source.source_id, a.source_version, source.status,
			a.source_digest, a.source_artifact_digest,
			source.repository_id, source.target_ref, a.authorizer_ref,
			source.valid_from, source.valid_until, a.proof_digest,
			a.proof_canonical_digest, a.root_key_id, a.approved_at,
			source_record.kind, source_record.canonical_json,
			source_artifact.media_type, source_artifact.content,
			proof_artifact.media_type, proof_artifact.content,
			receipt_artifact.media_type, receipt_artifact.content
		FROM authority_approvals AS a
		JOIN authority_source_snapshots AS source
		  ON source.source_ref = a.source_ref
		 AND source.source_version = a.source_version
		 AND source.source_digest = a.source_digest
		JOIN authority_source_authentications AS authentication
		  ON authentication.source_ref = a.source_ref
		 AND authentication.source_version = a.source_version
		 AND authentication.source_digest = a.source_digest
		 AND authentication.source_artifact_digest = a.source_artifact_digest
		 AND authentication.proof_digest = a.proof_digest
		 AND authentication.proof_canonical_digest = a.proof_canonical_digest
		 AND authentication.plan_digest = a.plan_digest
		 AND authentication.authority_digest = a.authority_digest
		 AND authentication.root_key_id = a.root_key_id
		 AND authentication.approved_at = a.approved_at
		JOIN records AS source_record ON source_record.digest = a.source_digest
		JOIN artifacts AS source_artifact ON source_artifact.digest = a.source_artifact_digest
		JOIN artifacts AS proof_artifact ON proof_artifact.digest = a.proof_digest
		JOIN artifacts AS receipt_artifact ON receipt_artifact.digest = a.receipt_digest
		WHERE a.receipt_digest = ?`, receiptDigest,
	).Scan(
		&approvalFacts.ReceiptID, &approvalFacts.ReceiptDigest,
		&sourceFacts.PlanDigest, &sourceFacts.AuthorityDigest,
		&sourceFacts.SourceRef, &sourceFacts.SourceID, &sourceFacts.SourceVersion, &sourceFacts.SourceStatus,
		&sourceFacts.SourceCanonicalDigest, &sourceFacts.SourceRawDigest,
		&sourceFacts.Repository, &sourceFacts.TargetRef, &sourceFacts.AuthorizerRef,
		&sourceFacts.ValidFrom, &sourceFacts.ValidUntil, &sourceFacts.ProofRawDigest,
		&sourceFacts.ProofCanonicalDigest, &sourceFacts.RootKeyID, &sourceFacts.ApprovedAt,
		&sourceRecordKind, &sourceRecordCanonical,
		&sourceType, &sourceRaw, &proofType, &proofRaw, &receiptType, &receiptRaw,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return policy.HistoricalApproval{}, fmt.Errorf("authority approval %s: %w", receiptDigest, sql.ErrNoRows)
	}
	if err != nil {
		return policy.HistoricalApproval{}, fmt.Errorf("read authority approval %s: %w", receiptDigest, err)
	}
	if sourceType != authoritySourceMediaType || proofType != authorityProofMediaType || receiptType != "application/json" ||
		protocol.RawDigest(sourceRaw) != sourceFacts.SourceRawDigest ||
		protocol.RawDigest(proofRaw) != sourceFacts.ProofRawDigest ||
		protocol.RawDigest(receiptRaw) != approvalFacts.ReceiptDigest {
		return policy.HistoricalApproval{}, errors.New("stored authority artifact binding is invalid")
	}
	sourceCanonical, err := protocol.CanonicalizeJSON(sourceRaw)
	if err != nil || sourceRecordKind != authoritySourceRecordKind ||
		!bytes.Equal(sourceCanonical, sourceRecordCanonical) ||
		protocol.CanonicalDigest(sourceCanonical) != sourceFacts.SourceCanonicalDigest {
		return policy.HistoricalApproval{}, errors.New("stored authority source canonical binding is invalid")
	}
	proofCanonical, err := protocol.CanonicalizeJSON(proofRaw)
	if err != nil || protocol.CanonicalDigest(proofCanonical) != sourceFacts.ProofCanonicalDigest {
		return policy.HistoricalApproval{}, errors.New("stored authority proof canonical binding is invalid")
	}
	plan, err := s.Plan(ctx, sourceFacts.PlanDigest)
	if err != nil {
		return policy.HistoricalApproval{}, err
	}
	historical, err := policy.RestoreHistoricalApproval(
		plan, root,
		policy.SourceClosure{
			SourceRaw: sourceRaw, SourceCanonical: sourceCanonical,
			ProofRaw: proofRaw, ProofCanonical: proofCanonical,
		},
		protocol.EncodedRecord{
			Kind:          protocol.ControlReceiptSchemaVersion,
			CanonicalJSON: receiptRaw,
			Digest:        approvalFacts.ReceiptDigest,
		},
	)
	if err != nil {
		return policy.HistoricalApproval{}, fmt.Errorf("restore authority approval: %w", err)
	}
	if historical.SourceFacts() != sourceFacts || historical.Facts() != approvalFacts {
		return policy.HistoricalApproval{}, errors.New("stored authority approval projection is invalid")
	}
	return historical, nil
}

func putAuthoritySourceTransaction(
	ctx context.Context,
	transaction *sql.Tx,
	plan protocol.ExactPlan,
	planRecord protocol.EncodedRecord,
	facts policy.SourceFacts,
	closure policy.SourceClosure,
	approval *policy.ApprovalFacts,
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
	if err := putArtifactTransaction(
		ctx, transaction, facts.SourceRawDigest, authoritySourceMediaType, closure.SourceRaw, now,
	); err != nil {
		return err
	}
	if err := putArtifactTransaction(
		ctx, transaction, facts.ProofRawDigest, authorityProofMediaType, closure.ProofRaw, now,
	); err != nil {
		return err
	}
	if err := enforceSourceMonotonicity(ctx, transaction, facts, approval); err != nil {
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

func putArtifactTransaction(
	ctx context.Context,
	transaction *sql.Tx,
	digest, mediaType string,
	content []byte,
	now int64,
) error {
	if protocol.RawDigest(content) != digest {
		return fmt.Errorf("authority artifact digest mismatch for %s", digest)
	}
	if err := protocol.ValidateArtifactContent(mediaType, content); err != nil {
		return err
	}
	content = append([]byte{}, content...)
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO artifacts (digest, media_type, content, size, created_at_us)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(digest) DO NOTHING`, digest, mediaType, content, len(content), now,
	); err != nil {
		return fmt.Errorf("put authority artifact %s: %w", digest, err)
	}
	var storedType string
	var stored []byte
	if err := transaction.QueryRowContext(ctx,
		"SELECT media_type, content FROM artifacts WHERE digest = ?", digest,
	).Scan(&storedType, &stored); err != nil {
		return fmt.Errorf("verify authority artifact %s: %w", digest, err)
	}
	if storedType != mediaType || !bytes.Equal(stored, content) {
		return fmt.Errorf("authority artifact conflict for %s", digest)
	}
	return nil
}

func enforceSourceMonotonicity(
	ctx context.Context,
	transaction *sql.Tx,
	facts policy.SourceFacts,
	approval *policy.ApprovalFacts,
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

func verifySourceSnapshot(ctx context.Context, source queryRower, facts policy.SourceFacts) error {
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

func verifySourceAuthentication(ctx context.Context, source queryRower, facts policy.SourceFacts) error {
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
	source queryRower,
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
