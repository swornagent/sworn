package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/swornagent/sworn/internal/protocol"
)

// PutPlan atomically persists an exact, fully parsed Baton plan as a canonical
// record. ExactPlan proves structure only; no storage-side projection grants
// it additional authority.
func (s *Store) PutPlan(ctx context.Context, plan protocol.ExactPlan) (string, error) {
	if s.readOnly {
		return "", errors.New("control store is read-only")
	}
	record, err := exactPlanRecord(plan)
	if err != nil {
		return "", err
	}
	transaction, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return "", fmt.Errorf("begin delivery plan storage: %w", err)
	}
	defer transaction.Rollback() //nolint:errcheck
	now := s.now().UTC().UnixMicro()
	if err := putPlanTransaction(ctx, transaction, plan, record, now); err != nil {
		return "", err
	}
	if err := transaction.Commit(); err != nil {
		return "", fmt.Errorf("commit delivery plan storage: %w", err)
	}
	return record.Digest, nil
}

func putPlanTransaction(
	ctx context.Context,
	transaction *sql.Tx,
	plan protocol.ExactPlan,
	record protocol.EncodedRecord,
	now int64,
) error {
	exact, err := exactPlanRecord(plan)
	if err != nil {
		return err
	}
	if exact.Kind != record.Kind || exact.Digest != record.Digest ||
		!bytes.Equal(exact.CanonicalJSON, record.CanonicalJSON) {
		return errors.New("delivery plan capability does not match its canonical record")
	}
	return putRecordTransaction(ctx, transaction, exact, now, "delivery plan")
}

// Plan restores a structural exact-plan capability from any canonical record
// with the delivery-plan kind that still passes the complete strict parser.
func (s *Store) Plan(ctx context.Context, planDigest string) (protocol.ExactPlan, error) {
	return loadExactPlan(ctx, s.db, planDigest)
}

func loadExactPlan(ctx context.Context, query rowQuerier, planDigest string) (protocol.ExactPlan, error) {
	kind, canonical, err := loadRecord(ctx, query, planDigest)
	if err != nil {
		return protocol.ExactPlan{}, err
	}
	if kind != protocol.DeliveryPlanSchemaVersion {
		return protocol.ExactPlan{}, fmt.Errorf("record %s is not a delivery plan", planDigest)
	}
	plan, err := protocol.ParseDeliveryPlan(canonical)
	if err != nil {
		return protocol.ExactPlan{}, fmt.Errorf("reparse stored delivery plan: %w", err)
	}
	record := plan.Record()
	if record.Kind != kind || record.Digest != planDigest || !bytes.Equal(record.CanonicalJSON, canonical) {
		return protocol.ExactPlan{}, errors.New("stored delivery plan record binding is invalid")
	}
	return plan, nil
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func exactPlanRecord(plan protocol.ExactPlan) (protocol.EncodedRecord, error) {
	record := plan.Record()
	if record.Kind != protocol.DeliveryPlanSchemaVersion || len(record.CanonicalJSON) == 0 ||
		record.Digest != protocol.RawDigest(record.CanonicalJSON) {
		return protocol.EncodedRecord{}, errors.New("invalid exact delivery plan capability")
	}
	return record, nil
}

func putRecordTransaction(
	ctx context.Context,
	transaction *sql.Tx,
	record protocol.EncodedRecord,
	now int64,
	label string,
) error {
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO records (digest, kind, canonical_json, size, created_at_us)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(digest) DO NOTHING`,
		record.Digest, record.Kind, record.CanonicalJSON, len(record.CanonicalJSON), now,
	); err != nil {
		return fmt.Errorf("put %s record %s: %w", label, record.Digest, err)
	}
	var storedKind string
	var storedJSON []byte
	if err := transaction.QueryRowContext(ctx,
		"SELECT kind, canonical_json FROM records WHERE digest = ?", record.Digest,
	).Scan(&storedKind, &storedJSON); err != nil {
		return fmt.Errorf("verify %s record %s: %w", label, record.Digest, err)
	}
	if storedKind != record.Kind || !bytes.Equal(storedJSON, record.CanonicalJSON) {
		return fmt.Errorf("%s record conflict for %s", label, record.Digest)
	}
	return nil
}
