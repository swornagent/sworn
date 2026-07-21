package protocol

import (
	"io/fs"
	"testing"
)

func TestSnapshotIsCompleteAndAuthentic(t *testing.T) {
	t.Parallel()

	if err := VerifySnapshot(); err != nil {
		t.Fatalf("VerifySnapshot() error = %v", err)
	}
	digest, err := SnapshotDigest()
	if err != nil {
		t.Fatalf("SnapshotDigest() error = %v", err)
	}
	if len(digest) != 64 {
		t.Fatalf("SnapshotDigest() length = %d, want 64", len(digest))
	}

	snapshot, err := SnapshotFS()
	if err != nil {
		t.Fatalf("SnapshotFS() error = %v", err)
	}
	for _, required := range []string{
		"schemas/delivery-plan-v1.json",
		"schemas/submission-v1.json",
		"schemas/delivery-verdict-v1.json",
		"schemas/delivery-board-v1.json",
		"schemas/assurance-policy-v1.json",
		"schemas/control-receipt-v1.json",
		"conformance/manifest.json",
	} {
		if _, err := fs.Stat(snapshot, required); err != nil {
			t.Errorf("required snapshot file %q: %v", required, err)
		}
	}
}
