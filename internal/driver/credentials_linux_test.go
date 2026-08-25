//go:build linux

package driver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileCredentialLeaseAllowsOnlyBoundedInPlaceRefresh(t *testing.T) {
	workspace := t.TempDir()
	credentialRoot := t.TempDir()
	pathValue := filepath.Join(credentialRoot, "auth.json")
	if err := os.WriteFile(pathValue, []byte(`{"token":"first"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	lease, err := acquireFileCredential(pathValue, workspace, 4_096)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireFileCredential(pathValue, workspace, 4_096); !IsCode(err, "CREDENTIAL_NOT_CERTIFIED") {
		t.Fatalf("concurrent lease error = %v", err)
	}
	if err := os.WriteFile(pathValue, []byte(`{"token":"refreshed"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatalf("in-place refresh rejected: %v", err)
	}
	body, err := os.ReadFile(pathValue)
	if err != nil || string(body) != `{"token":"refreshed"}` {
		t.Fatalf("refreshed credential = %q, %v", body, err)
	}
}

func TestFileCredentialLeaseRejectsIdentityModeLinkAndWorkspaceDrift(t *testing.T) {
	t.Run("benign rotation", func(t *testing.T) {
		// A3: an atomic rename to a new 0600, owner-matched, single-link
		// replacement at the same path is ordinary host credential refresh
		// racing the dispatch, not tampering: the lease closes cleanly and
		// reports the rotation so the caller records it loudly.
		root := t.TempDir()
		pathValue := filepath.Join(root, "credential")
		if err := os.WriteFile(pathValue, []byte("first-value"), 0o600); err != nil {
			t.Fatal(err)
		}
		lease, err := acquireFileCredential(pathValue, t.TempDir(), 4_096)
		if err != nil {
			t.Fatal(err)
		}
		replacement := filepath.Join(root, "replacement")
		if err := os.WriteFile(
			replacement,
			[]byte("rotated-value"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(replacement, pathValue); err != nil {
			t.Fatal(err)
		}
		if err := lease.Close(); err != nil {
			t.Fatalf("benign rotation error = %v", err)
		}
		if !lease.benignRotation() {
			t.Fatal("benign rotation was not reported")
		}
		body, err := os.ReadFile(pathValue)
		if err != nil || string(body) != "rotated-value" {
			t.Fatalf("rotated credential = %q, %v", body, err)
		}
	})

	t.Run("unsafe rotation replacement", func(t *testing.T) {
		// A rotation whose replacement lacks the safe shape (0600, owner,
		// single link) is tampering, not benign rotation: the close keeps
		// failing exactly as before the A3 relaxation.
		root := t.TempDir()
		pathValue := filepath.Join(root, "credential")
		if err := os.WriteFile(pathValue, []byte("first-value"), 0o600); err != nil {
			t.Fatal(err)
		}
		lease, err := acquireFileCredential(pathValue, t.TempDir(), 4_096)
		if err != nil {
			t.Fatal(err)
		}
		replacement := filepath.Join(root, "replacement")
		if err := os.WriteFile(
			replacement,
			[]byte("loose-replacement"),
			0o640,
		); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(replacement, pathValue); err != nil {
			t.Fatal(err)
		}
		if err := lease.Close(); !IsCode(err, "CREDENTIAL_IDENTITY_CHANGED") {
			t.Fatalf("unsafe rotation error = %v", err)
		}
	})

	t.Run("owner drift", func(t *testing.T) {
		pathValue := filepath.Join(t.TempDir(), "credential")
		if err := os.WriteFile(pathValue, []byte("credential"), 0o600); err != nil {
			t.Fatal(err)
		}
		lease, err := acquireFileCredential(pathValue, t.TempDir(), 4_096)
		if err != nil {
			t.Fatal(err)
		}
		// Owner drift on the live path entry is tampering: the safe shape
		// check fails and the close refuses exactly as before the A3
		// relaxation. Chowning to another uid requires privilege; hosts
		// without it cannot synthesize this drift and skip the pin.
		if err := os.Chown(pathValue, 65534, 65534); err != nil {
			_ = lease.Close()
			t.Skipf("owner drift requires chown privilege: %v", err)
		}
		if err := lease.Close(); !IsCode(err, "CREDENTIAL_IDENTITY_CHANGED") {
			t.Fatalf("owner drift error = %v", err)
		}
	})

	t.Run("link drift", func(t *testing.T) {
		root := t.TempDir()
		pathValue := filepath.Join(root, "credential")
		if err := os.WriteFile(pathValue, []byte("credential"), 0o600); err != nil {
			t.Fatal(err)
		}
		lease, err := acquireFileCredential(pathValue, t.TempDir(), 4_096)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Link(pathValue, filepath.Join(root, "alias")); err != nil {
			_ = lease.Close()
			t.Fatalf("link drift setup failed: %v", err)
		}
		if err := lease.Close(); !IsCode(err, "CREDENTIAL_IDENTITY_CHANGED") {
			t.Fatalf("link drift error = %v", err)
		}
	})

	t.Run("missing path after rename", func(t *testing.T) {
		root := t.TempDir()
		pathValue := filepath.Join(root, "credential")
		if err := os.WriteFile(pathValue, []byte("credential"), 0o600); err != nil {
			t.Fatal(err)
		}
		lease, err := acquireFileCredential(pathValue, t.TempDir(), 4_096)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(pathValue, filepath.Join(root, "old")); err != nil {
			t.Fatal(err)
		}
		if err := lease.Close(); !IsCode(err, "CREDENTIAL_IDENTITY_CHANGED") {
			t.Fatalf("missing path error = %v", err)
		}
	})

	t.Run("mode drift", func(t *testing.T) {
		pathValue := filepath.Join(t.TempDir(), "credential")
		if err := os.WriteFile(pathValue, []byte("credential"), 0o600); err != nil {
			t.Fatal(err)
		}
		lease, err := acquireFileCredential(pathValue, t.TempDir(), 4_096)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(pathValue, 0o640); err != nil {
			t.Fatal(err)
		}
		if err := lease.Close(); !IsCode(err, "CREDENTIAL_IDENTITY_CHANGED") {
			t.Fatalf("mode drift error = %v", err)
		}
	})

	t.Run("hard link", func(t *testing.T) {
		root := t.TempDir()
		pathValue := filepath.Join(root, "credential")
		if err := os.WriteFile(pathValue, []byte("credential"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(pathValue, filepath.Join(root, "alias")); err != nil {
			t.Fatal(err)
		}
		if _, err := acquireFileCredential(pathValue, t.TempDir(), 4_096); !IsCode(err, "CREDENTIAL_NOT_CERTIFIED") {
			t.Fatalf("hard-link error = %v", err)
		}
	})

	t.Run("symlink", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "target")
		link := filepath.Join(root, "credential")
		if err := os.WriteFile(target, []byte("credential"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		if _, err := acquireFileCredential(link, t.TempDir(), 4_096); !IsCode(err, "CREDENTIAL_NOT_CERTIFIED") {
			t.Fatalf("symlink error = %v", err)
		}
	})

	t.Run("workspace path", func(t *testing.T) {
		workspace := t.TempDir()
		pathValue := filepath.Join(workspace, "credential")
		if err := os.WriteFile(pathValue, []byte("credential"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := acquireFileCredential(pathValue, workspace, 4_096); !IsCode(err, "CREDENTIAL_NOT_CERTIFIED") {
			t.Fatalf("workspace credential error = %v", err)
		}
	})
}
