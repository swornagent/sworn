package store

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
)

func TestRecordsAndArtifactsAreContentAddressedAndImmutable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	control := openTestStore(t, filepath.Join(t.TempDir(), "control.db"))
	t.Cleanup(func() { _ = control.Close() })

	record := exampleExactPlan(t).Record()
	recordDigest, err := control.PutPlan(ctx, exampleExactPlan(t))
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, err := control.PutPlan(ctx, exampleExactPlan(t)); err != nil || duplicate != recordDigest {
		t.Fatalf("duplicate record = %q, %v", duplicate, err)
	}
	kind, storedRecord, err := control.Record(ctx, recordDigest)
	if err != nil || kind != record.Kind || !bytes.Equal(storedRecord, record.CanonicalJSON) {
		t.Fatalf("record = %q %q, %v", kind, storedRecord, err)
	}
	if _, err := control.db.Exec("UPDATE records SET kind = 'changed' WHERE digest = ?", recordDigest); err == nil {
		t.Fatal("record update bypassed immutability trigger")
	}

	artifact := []byte{0, 1, 2, 3, 255}
	artifactDigest, err := control.PutArtifact(ctx, "application/octet-stream", artifact)
	if err != nil {
		t.Fatal(err)
	}
	mediaType, storedArtifact, err := control.Artifact(ctx, artifactDigest)
	if err != nil || mediaType != "application/octet-stream" || !bytes.Equal(storedArtifact, artifact) {
		t.Fatalf("artifact = %q %v, %v", mediaType, storedArtifact, err)
	}
	if _, err := control.db.Exec("DELETE FROM artifacts WHERE digest = ?", artifactDigest); err == nil {
		t.Fatal("artifact delete bypassed immutability trigger")
	}

	if _, err := control.PutArtifact(ctx, "Application/JSON", []byte(`{}`)); err == nil {
		t.Fatal("non-canonical media type was accepted")
	}
	if _, err := control.PutArtifact(ctx, "application/json", []byte(`{"value":1,"value":2}`)); err == nil {
		t.Fatal("non-strict JSON artifact was accepted")
	}
	emptyDigest, err := control.PutArtifact(ctx, "application/octet-stream", nil)
	if err != nil {
		t.Fatalf("put empty artifact: %v", err)
	}
	_, empty, err := control.Artifact(ctx, emptyDigest)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty artifact = %x, %v", empty, err)
	}
}
