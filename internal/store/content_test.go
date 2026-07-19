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

	record := []byte(`{"schema_version":"example-v1","value":1}`)
	recordDigest, err := control.PutRecord(ctx, "example-v1", record)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, err := control.PutRecord(ctx, "example-v1", record); err != nil || duplicate != recordDigest {
		t.Fatalf("duplicate record = %q, %v", duplicate, err)
	}
	kind, storedRecord, err := control.Record(ctx, recordDigest)
	if err != nil || kind != "example-v1" || !bytes.Equal(storedRecord, record) {
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
}
