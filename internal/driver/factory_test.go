package driver

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/gitx"
)

// TestReapCertificationRootsRequiresOwnershipMatch pins the ownership guard
// staleCertificationRoot applies before an aged root is ever considered for
// removal: the selfUID parameter lets this be proven directly, since the
// sandbox has no root to construct a genuinely foreign-owned file.
func TestReapCertificationRootsRequiresOwnershipMatch(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, certificationRootPrefix+"aged")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	aged := time.Now().Add(-certificationRootLifetime - time.Hour)
	if err := os.Chtimes(root, aged, aged); err != nil {
		t.Fatal(err)
	}
	if staleCertificationRoot(root, uint32(os.Getuid())+1) {
		t.Fatal("foreign-owned uid mismatch reported stale")
	}
	if !staleCertificationRoot(root, uint32(os.Getuid())) {
		t.Fatal("own aged non-symlink root did not report stale")
	}
}

// TestProductionDriverFactoryReapsStaleCertificationRootsAtConstruction is
// the A1 consumer proof: construction sweeps a stale prior certification
// root while leaving a fresh same-prefix root, a differently-prefixed root,
// and a same-prefix symlink alone.
func TestProductionDriverFactoryReapsStaleCertificationRootsAtConstruction(t *testing.T) {
	temp := t.TempDir()
	t.Setenv(gitx.EnvTempRoot, temp)

	stale := filepath.Join(temp, certificationRootPrefix+"stale")
	if err := os.Mkdir(stale, 0o700); err != nil {
		t.Fatal(err)
	}
	aged := time.Now().Add(-certificationRootLifetime - time.Hour)
	if err := os.Chtimes(stale, aged, aged); err != nil {
		t.Fatal(err)
	}

	fresh := filepath.Join(temp, certificationRootPrefix+"fresh")
	if err := os.Mkdir(fresh, 0o700); err != nil {
		t.Fatal(err)
	}

	foreignPrefix := filepath.Join(temp, "other-prefix-root")
	if err := os.Mkdir(foreignPrefix, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(foreignPrefix, aged, aged); err != nil {
		t.Fatal(err)
	}

	symlinkTarget := filepath.Join(temp, certificationRootPrefix+"symlink-target")
	if err := os.Mkdir(symlinkTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(temp, certificationRootPrefix+"symlink")
	if err := os.Symlink(symlinkTarget, symlink); err != nil {
		t.Fatal(err)
	}

	config := completeDriverConfigFixture(t)
	body, err := EncodeDriverConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := DecodeDriverConfig(body)
	if err != nil {
		t.Fatal(err)
	}
	factory, err := NewProductionDriverFactory(loaded)
	if err != nil {
		t.Fatal(err)
	}
	defer factory.Close()

	if _, err := os.Lstat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale certification root survived construction: err=%v", err)
	}
	if _, err := os.Lstat(fresh); err != nil {
		t.Fatalf("fresh certification root was swept: %v", err)
	}
	if _, err := os.Lstat(foreignPrefix); err != nil {
		t.Fatalf("differently-prefixed root was swept: %v", err)
	}
	if _, err := os.Lstat(symlink); err != nil {
		t.Fatalf("same-prefix symlink was swept: %v", err)
	}
}
