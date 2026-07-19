package store

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/swornagent/sworn/internal/protocol"
)

// PutSubmission atomically persists the opaque construction capability minted
// by protocol.BuildSubmission. Raw, independently constructible Baton records
// are deliberately not accepted by this control boundary.
func (s *Store) PutSubmission(ctx context.Context, prepared protocol.PreparedSubmission) (string, error) {
	if s.readOnly {
		return "", errors.New("control store is read-only")
	}
	submission := prepared.Submission()
	record := prepared.Record()
	dependencies := prepared.Dependencies()
	reencoded, err := protocol.EncodeSubmission(submission)
	if err != nil {
		return "", err
	}
	if record.Kind != reencoded.Kind || record.Digest != reencoded.Digest ||
		!bytes.Equal(record.CanonicalJSON, reencoded.CanonicalJSON) {
		return "", errors.New("submission capability does not match its canonical record")
	}
	return s.putPreparedSubmission(ctx, submission, record, dependencies)
}

func (s *Store) putPreparedSubmission(
	ctx context.Context,
	submission protocol.Submission,
	record protocol.EncodedRecord,
	dependencies []protocol.Artifact,
) (string, error) {
	artifacts, err := completeSubmissionArtifacts(submission, dependencies)
	if err != nil {
		return "", err
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return "", fmt.Errorf("begin submission storage: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	var authorityBytes []byte
	for _, artifact := range artifacts {
		contents, err := verifySubmissionArtifact(ctx, transaction, artifact)
		if err != nil {
			return "", err
		}
		if artifact == submission.AuthorityReceipt {
			authorityBytes = contents
		}
	}
	now := s.now().UTC().UnixMicro()
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO records (digest, kind, canonical_json, size, created_at_us)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(digest) DO NOTHING`,
		record.Digest, record.Kind, record.CanonicalJSON, len(record.CanonicalJSON), now,
	); err != nil {
		return "", fmt.Errorf("put submission record %s: %w", record.Digest, err)
	}
	var storedKind string
	var storedJSON []byte
	if err := transaction.QueryRowContext(ctx,
		"SELECT kind, canonical_json FROM records WHERE digest = ?", record.Digest,
	).Scan(&storedKind, &storedJSON); err != nil {
		return "", fmt.Errorf("verify submission record %s: %w", record.Digest, err)
	}
	if storedKind != record.Kind || !bytes.Equal(storedJSON, record.CanonicalJSON) {
		return "", fmt.Errorf("submission record conflict for %s", record.Digest)
	}
	reservations, err := submissionIdentityReservations(submission, authorityBytes, record.Digest)
	if err != nil {
		return "", err
	}
	for _, reservation := range reservations {
		if err := reserveProtocolIdentity(ctx, transaction, reservation); err != nil {
			return "", err
		}
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO submission_records (submission_id, delivery_id, work_id, attempt, digest)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`,
		submission.SubmissionID, submission.DeliveryID, submission.WorkID, submission.Attempt, record.Digest,
	); err != nil {
		return "", fmt.Errorf("reserve submission identity: %w", err)
	}
	var deliveryID, workID, digest string
	var attempt int64
	err = transaction.QueryRowContext(ctx, `
		SELECT delivery_id, work_id, attempt, digest
		FROM submission_records WHERE submission_id = ?`, submission.SubmissionID,
	).Scan(&deliveryID, &workID, &attempt, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		var occupiedID string
		pairErr := transaction.QueryRowContext(ctx, `
			SELECT submission_id FROM submission_records
			WHERE delivery_id = ? AND work_id = ? AND attempt = ?`,
			submission.DeliveryID, submission.WorkID, submission.Attempt,
		).Scan(&occupiedID)
		if pairErr == nil {
			return "", fmt.Errorf("work attempt is already bound to submission %q", occupiedID)
		}
		return "", errors.New("submission identity reservation was not recorded")
	}
	if err != nil {
		return "", fmt.Errorf("verify submission identity: %w", err)
	}
	if deliveryID != submission.DeliveryID || workID != submission.WorkID ||
		attempt != submission.Attempt || digest != record.Digest {
		return "", fmt.Errorf("submission id %q is already bound to different canonical bytes", submission.SubmissionID)
	}
	if err := transaction.Commit(); err != nil {
		return "", fmt.Errorf("commit submission storage: %w", err)
	}
	return record.Digest, nil
}

// SubmissionRecord reads the immutable canonical record bound to one global
// submission ID.
func (s *Store) SubmissionRecord(ctx context.Context, submissionID string) (string, []byte, error) {
	var digest string
	var kind string
	var canonical []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT identities.digest, records.kind, records.canonical_json
		FROM submission_records AS identities
		JOIN records ON records.digest = identities.digest
		WHERE identities.submission_id = ?`, submissionID,
	).Scan(&digest, &kind, &canonical)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, fmt.Errorf("submission %q: %w", submissionID, sql.ErrNoRows)
	}
	if err != nil {
		return "", nil, fmt.Errorf("read submission %q: %w", submissionID, err)
	}
	if kind != protocol.SubmissionSchemaVersion || protocol.RawDigest(canonical) != digest {
		return "", nil, errors.New("stored submission record binding is invalid")
	}
	return digest, canonical, nil
}

func submissionArtifacts(submission protocol.Submission) ([]protocol.Artifact, error) {
	artifacts := []protocol.Artifact{submission.AuthorityReceipt}
	seen := map[string]string{submission.AuthorityReceipt.Digest: submission.AuthorityReceipt.MediaType}
	for _, check := range submission.Checks {
		if mediaType, exists := seen[check.Receipt.Digest]; exists && mediaType != check.Receipt.MediaType {
			return nil, fmt.Errorf("submission reuses artifact %s with conflicting media types", check.Receipt.Digest)
		} else if !exists {
			seen[check.Receipt.Digest] = check.Receipt.MediaType
			artifacts = append(artifacts, check.Receipt)
		}
	}
	for _, evidence := range submission.Evidence {
		if mediaType, exists := seen[evidence.Artifact.Digest]; exists && mediaType != evidence.Artifact.MediaType {
			return nil, fmt.Errorf("submission reuses artifact %s with conflicting media types", evidence.Artifact.Digest)
		} else if !exists {
			seen[evidence.Artifact.Digest] = evidence.Artifact.MediaType
			artifacts = append(artifacts, evidence.Artifact)
		}
	}
	return artifacts, nil
}

func completeSubmissionArtifacts(
	submission protocol.Submission,
	dependencies []protocol.Artifact,
) ([]protocol.Artifact, error) {
	if len(dependencies) == 0 {
		return nil, errors.New("prepared submission lacks its resolved artifact closure")
	}
	all := make(map[string]protocol.Artifact, len(dependencies))
	for _, pointer := range dependencies {
		if existing, exists := all[pointer.Digest]; exists {
			if existing != pointer {
				return nil, fmt.Errorf("prepared artifact %s has conflicting pointers", pointer.Digest)
			}
			continue
		}
		all[pointer.Digest] = pointer
	}
	direct, err := submissionArtifacts(submission)
	if err != nil {
		return nil, err
	}
	for _, pointer := range direct {
		if dependency, exists := all[pointer.Digest]; !exists || dependency != pointer {
			return nil, fmt.Errorf("prepared submission omits direct artifact %s", pointer.Digest)
		}
	}
	ordered := make([]protocol.Artifact, 0, len(all))
	for _, pointer := range all {
		ordered = append(ordered, pointer)
	}
	slices.SortFunc(ordered, func(left, right protocol.Artifact) int {
		return cmp.Compare(left.Digest, right.Digest)
	})
	return ordered, nil
}

func verifySubmissionArtifact(ctx context.Context, transaction *sql.Tx, pointer protocol.Artifact) ([]byte, error) {
	if pointer.Ref != pointer.Digest {
		return nil, fmt.Errorf("submission artifact %s is not a CAS reference", pointer.Digest)
	}
	var mediaType string
	var contents []byte
	err := transaction.QueryRowContext(ctx,
		"SELECT media_type, content FROM artifacts WHERE digest = ?", pointer.Digest,
	).Scan(&mediaType, &contents)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("submission artifact %s is unavailable", pointer.Digest)
	}
	if err != nil {
		return nil, fmt.Errorf("read submission artifact %s: %w", pointer.Digest, err)
	}
	if mediaType != pointer.MediaType || protocol.RawDigest(contents) != pointer.Digest {
		return nil, fmt.Errorf("submission artifact %s does not match its pointer", pointer.Digest)
	}
	return contents, nil
}

type protocolIdentityReservation struct {
	kind          string
	id            string
	bindingDigest string
}

func submissionIdentityReservations(
	submission protocol.Submission,
	authorityBytes []byte,
	recordDigest string,
) ([]protocolIdentityReservation, error) {
	authorityID, err := protocol.AuthorityApprovalReceiptID(authorityBytes)
	if err != nil {
		return nil, fmt.Errorf("identify submission authority receipt: %w", err)
	}
	reservations := []protocolIdentityReservation{{
		kind: "authority_approval", id: authorityID, bindingDigest: submission.AuthorityReceipt.Digest,
	}}
	reservations = append(reservations, protocolIdentityReservation{
		kind: "builder_run", id: submission.Builder.RunID, bindingDigest: recordDigest,
	})
	for _, check := range submission.Checks {
		reservations = append(reservations, protocolIdentityReservation{
			kind: "producer_run", id: check.RunID, bindingDigest: recordDigest,
		})
	}
	return reservations, nil
}

func reserveProtocolIdentity(
	ctx context.Context,
	transaction *sql.Tx,
	reservation protocolIdentityReservation,
) error {
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO protocol_identities (identity_kind, identity_id, binding_digest)
		VALUES (?, ?, ?)
		ON CONFLICT DO NOTHING`,
		reservation.kind, reservation.id, reservation.bindingDigest,
	); err != nil {
		return fmt.Errorf("reserve %s %q: %w", reservation.kind, reservation.id, err)
	}
	var bindingDigest string
	if err := transaction.QueryRowContext(ctx, `
		SELECT binding_digest FROM protocol_identities
		WHERE identity_kind = ? AND identity_id = ?`,
		reservation.kind, reservation.id,
	).Scan(&bindingDigest); err != nil {
		return fmt.Errorf("verify %s %q: %w", reservation.kind, reservation.id, err)
	}
	if bindingDigest != reservation.bindingDigest {
		return fmt.Errorf("%s %q is already bound to different bytes or effects", reservation.kind, reservation.id)
	}
	return nil
}
