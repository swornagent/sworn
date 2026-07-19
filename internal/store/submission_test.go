package store

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/protocol"
)

func TestPutSubmissionReservesGlobalAndAttemptIdentities(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	submission := admittedExampleSubmission(t, ctx, control)
	digest, err := putTestSubmission(ctx, control, submission)
	if err != nil {
		t.Fatal(err)
	}
	if repeated, err := putTestSubmission(ctx, control, submission); err != nil || repeated != digest {
		t.Fatalf("idempotent submission = %q, %v; want %q", repeated, err, digest)
	}
	storedDigest, canonical, err := control.SubmissionRecord(ctx, submission.SubmissionID)
	if err != nil || storedDigest != digest || protocol.RawDigest(canonical) != digest {
		t.Fatalf("stored submission = %q %q, %v", storedDigest, canonical, err)
	}

	changed := submission
	changed.CreatedAt = "2026-07-19T00:06:00Z"
	if _, err := putTestSubmission(ctx, control, changed); err == nil {
		t.Fatal("global submission id was rebound")
	}
	changed = submission
	changed.SubmissionID = "different-submission-id"
	if _, err := putTestSubmission(ctx, control, changed); err == nil {
		t.Fatal("work attempt was rebound")
	}
	assertCount(t, control, "records", 1)
	assertCount(t, control, "submission_records", 1)
	assertCount(t, control, "protocol_identities", 2)
}

func TestPutSubmissionRequiresResolvableArtifacts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	submission := exampleSubmissionFromSnapshot(t)
	if _, err := putTestSubmission(ctx, control, submission); err == nil {
		t.Fatal("submission with missing artifacts was stored")
	}
	assertCount(t, control, "records", 0)
}

func TestSubmissionProtocolIdentitiesAreWriteOnceAcrossRecords(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	first := admittedExampleSubmission(t, ctx, control)
	if _, err := putTestSubmission(ctx, control, first); err != nil {
		t.Fatal(err)
	}

	// The same exact approval receipt is intentionally reusable.
	second := distinctSubmission(first, "second")
	if _, err := putTestSubmission(ctx, control, second); err != nil {
		t.Fatalf("reuse exact authority receipt: %v", err)
	}

	reusedBuilder := distinctSubmission(first, "reused-builder")
	reusedBuilder.Builder.RunID = first.Builder.RunID
	if _, err := putTestSubmission(ctx, control, reusedBuilder); err == nil || !strings.Contains(err.Error(), "builder_run") {
		t.Fatalf("reused builder identity error = %v", err)
	}

	// Preserve the exact Check and Evidence bytes while changing submission
	// context. A producer run is still globally one-record-only.
	reusedProducer := first
	reusedProducer.SubmissionID = "producer-reuse-submission"
	reusedProducer.WorkID = "producer-reuse-work"
	reusedProducer.Builder.RunID = "producer-reuse-builder"
	if _, err := putTestSubmission(ctx, control, reusedProducer); err == nil || !strings.Contains(err.Error(), "producer_run") {
		t.Fatalf("reused producer identity error = %v", err)
	}

	// Work-attempt identity is delivery-scoped, so common work IDs remain valid
	// in independent deliveries.
	laterDelivery := distinctSubmission(first, "later-delivery")
	laterDelivery.DeliveryID = "another-delivery"
	laterDelivery.WorkID = first.WorkID
	if _, err := putTestSubmission(ctx, control, laterDelivery); err != nil {
		t.Fatalf("delivery-scoped work attempt was globally blocked: %v", err)
	}

	assertCount(t, control, "submission_records", 3)
	assertCount(t, control, "protocol_identities", 6)
}

func TestSubmissionPersistenceRejectsFalseCASAndMediaAliases(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })
	base := admittedExampleSubmission(t, ctx, control)

	falseRef := base
	falseRef.AuthorityReceipt.Ref = "artifact:false"
	if _, err := putTestSubmission(ctx, control, falseRef); err == nil || !strings.Contains(err.Error(), "CAS reference") {
		t.Fatalf("false artifact ref error = %v", err)
	}

	mediaAlias := base
	mediaAlias.Checks[0].Receipt = protocol.Artifact{
		Ref: base.AuthorityReceipt.Digest, MediaType: "text/plain", Digest: base.AuthorityReceipt.Digest,
	}
	if _, err := putTestSubmission(ctx, control, mediaAlias); err == nil || !strings.Contains(err.Error(), "conflicting media") {
		t.Fatalf("conflicting media alias error = %v", err)
	}

	if _, err := control.PutSubmission(ctx, protocol.PreparedSubmission{}); err == nil {
		t.Fatal("zero or independently constructible submission capability was admitted")
	}
	assertCount(t, control, "submission_records", 0)
}

func admittedExampleSubmission(t *testing.T, ctx context.Context, control *Store) protocol.Submission {
	t.Helper()
	submission := exampleSubmissionFromSnapshot(t)
	snapshot, err := protocol.SnapshotFS()
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range []struct {
		path      string
		mediaType string
	}{
		{"examples/artifacts/authority/plan-approval.json", "application/json"},
		{"examples/artifacts/checks/test.log", "text/plain"},
		{"examples/artifacts/evidence/health-smoke.json", "application/json"},
	} {
		contents, err := fs.ReadFile(snapshot, artifact.path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := control.PutArtifact(ctx, artifact.mediaType, contents); err != nil {
			t.Fatal(err)
		}
	}
	submission.AuthorityReceipt.Ref = submission.AuthorityReceipt.Digest
	for index := range submission.Checks {
		submission.Checks[index].Receipt.Ref = submission.Checks[index].Receipt.Digest
	}
	for index := range submission.Evidence {
		submission.Evidence[index].Artifact.Ref = submission.Evidence[index].Artifact.Digest
	}
	return submission
}

func putTestSubmission(ctx context.Context, control *Store, submission protocol.Submission) (string, error) {
	record, err := protocol.EncodeSubmission(submission)
	if err != nil {
		return "", err
	}
	dependencies, err := submissionArtifacts(submission)
	if err != nil {
		return "", err
	}
	return control.putPreparedSubmission(ctx, submission, record, dependencies)
}

func distinctSubmission(base protocol.Submission, suffix string) protocol.Submission {
	value := base
	value.SubmissionID = "submission-" + suffix
	value.WorkID = "work-" + suffix
	value.Builder.RunID = "builder-" + suffix
	value.Checks = append([]protocol.Check(nil), base.Checks...)
	value.Checks[0].RunID = "producer-" + suffix
	value.Evidence = append([]protocol.Evidence(nil), base.Evidence...)
	value.Evidence[0].ProducerRunID = value.Checks[0].RunID
	return value
}

func exampleSubmissionFromSnapshot(t *testing.T) protocol.Submission {
	t.Helper()
	snapshot, err := protocol.SnapshotFS()
	if err != nil {
		t.Fatal(err)
	}
	contents, err := fs.ReadFile(snapshot, "examples/standard-submission.json")
	if err != nil {
		t.Fatal(err)
	}
	var submission protocol.Submission
	if err := json.Unmarshal(contents, &submission); err != nil {
		t.Fatal(err)
	}
	return submission
}
