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
	t.Run("replacement", func(t *testing.T) {
		root := t.TempDir()
		pathValue := filepath.Join(root, "credential")
		if err := os.WriteFile(pathValue, []byte("first-value"), 0o600); err != nil {
			t.Fatal(err)
		}
		lease, err := acquireFileCredential(pathValue, t.TempDir(), 4_096)
		if err != nil {
			t.Fatal(err)
		}
		old := filepath.Join(root, "old")
		if err := os.Rename(pathValue, old); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pathValue, []byte("replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := lease.Close(); !IsCode(err, "CREDENTIAL_IDENTITY_CHANGED") {
			t.Fatalf("replacement error = %v", err)
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
