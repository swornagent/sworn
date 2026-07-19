package store

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/protocol"
)

func TestPutPlanPersistsAndRestoresExactCapability(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	plan := exampleExactPlan(t)

	digest, err := control.PutPlan(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if repeated, err := control.PutPlan(ctx, plan); err != nil || repeated != digest {
		t.Fatalf("idempotent PutPlan = %q, %v; want %q", repeated, err, digest)
	}
	restored, err := control.Plan(ctx, digest)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Record().Digest != plan.Record().Digest || restored.Authority().Digest != plan.Authority().Digest {
		t.Fatal("restored plan lost its exact canonical bindings")
	}
	assertCount(t, control, "records", 1)
}

func TestPutPlanAllowsCanonicalRevisionButRejectsZeroCapability(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	plan := exampleExactPlan(t)
	if _, err := control.PutPlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	revisedBytes := strings.Replace(
		string(plan.Record().CanonicalJSON),
		`"outcome":"Expose a health endpoint that reports the assembled service as ready."`,
		`"outcome":"Expose a revised health endpoint that reports the service as ready."`,
		1,
	)
	revised, err := protocol.ParseDeliveryPlan([]byte(revisedBytes))
	if err != nil {
		t.Fatal(err)
	}
	if revised.DeliveryID() != plan.DeliveryID() || revised.Record().Digest == plan.Record().Digest {
		t.Fatal("fixture revision did not retain delivery identity and change digest")
	}
	if _, err := control.PutPlan(ctx, revised); err != nil {
		t.Fatalf("put authorized plan revision shape: %v", err)
	}
	if _, err := control.PutPlan(ctx, protocol.ExactPlan{}); err == nil {
		t.Fatal("zero exact-plan capability was stored")
	}
	assertCount(t, control, "records", 2)
}

func TestGenericRecordsRestoreOnlyWhenKindAndPlanAreValid(t *testing.T) {
	t.Parallel()

	t.Run("valid plan", func(t *testing.T) {
		ctx := context.Background()
		control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
		t.Cleanup(func() { _ = control.Close() })
		record := exampleExactPlan(t).Record()
		digest, err := putTestRecord(ctx, control, record)
		if err != nil {
			t.Fatal(err)
		}
		if restored, err := control.Plan(ctx, digest); err != nil || restored.Record().Digest != digest {
			t.Fatalf("Plan() = %q, %v; want structural capability", restored.Record().Digest, err)
		}
	})

	t.Run("invalid plan", func(t *testing.T) {
		ctx := context.Background()
		control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
		t.Cleanup(func() { _ = control.Close() })
		canonical := []byte(`{"schema_version":"delivery-plan-v1"}`)
		digest, err := putTestRecord(ctx, control, protocol.EncodedRecord{
			Kind: protocol.DeliveryPlanSchemaVersion, CanonicalJSON: canonical, Digest: protocol.RawDigest(canonical),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := control.Plan(ctx, digest); err == nil || !strings.Contains(err.Error(), "reparse stored delivery plan") {
			t.Fatalf("invalid plan restore error = %v", err)
		}
	})

	t.Run("wrong kind", func(t *testing.T) {
		ctx := context.Background()
		control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
		t.Cleanup(func() { _ = control.Close() })
		record := exampleExactPlan(t).Record()
		record.Kind = "not-a-delivery-plan"
		digest, err := putTestRecord(ctx, control, record)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := control.Plan(ctx, digest); err == nil || !strings.Contains(err.Error(), "not a delivery plan") {
			t.Fatalf("wrong-kind restore error = %v", err)
		}
	})
}

func putTestRecord(
	ctx context.Context,
	control *Store,
	record protocol.EncodedRecord,
) (string, error) {
	transaction, err := control.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return "", err
	}
	defer transaction.Rollback() //nolint:errcheck
	if err := putRecordTransaction(ctx, transaction, record, control.now().UTC().UnixMicro(), "test"); err != nil {
		return "", err
	}
	if err := transaction.Commit(); err != nil {
		return "", err
	}
	return record.Digest, nil
}

func exampleExactPlan(t *testing.T) protocol.ExactPlan {
	t.Helper()
	snapshot, err := protocol.SnapshotFS()
	if err != nil {
		t.Fatal(err)
	}
	contents, err := fs.ReadFile(snapshot, "examples/standard-plan.json")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := protocol.ParseDeliveryPlan(contents)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
