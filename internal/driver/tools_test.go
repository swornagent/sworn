package driver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreateInvocationScratchRootNamesItsSite is the A1/A2 proof for
// tools.go:131: called directly against a caller-chosen, already-resolved
// temp root (as newToolSession calls it once tempRoot() has already
// succeeded), a permission-denied root fails os.MkdirTemp with a real
// kernel EACCES, named at its own site with the kernel's cause and no host
// path. It is called directly rather than through newToolSession because
// StageInputProjection stages inputs under the same configured temp root
// moments earlier in newToolSession and would refuse first (as
// TestNewToolSessionRefusesUnavailableTempRoot already pins) if the whole
// root were made unavailable instead of just this call's own target.
func TestCreateInvocationScratchRootNamesItsSite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	_, err := createInvocationScratchRoot(root)
	if err == nil {
		t.Fatal("permission-denied temp root silently escaped")
	}
	contractErr, ok := err.(*ContractError)
	if !ok || contractErr.Code != "PROCESS_START_FAILED" {
		t.Fatalf("err = %#v", err)
	}
	var envelope sandboxStartDetail
	if jsonErr := json.Unmarshal([]byte(contractErr.Detail), &envelope); jsonErr != nil {
		t.Fatalf("detail = %q: %v", contractErr.Detail, jsonErr)
	}
	if envelope.Check != "sandbox_start.invocation_scratch_create" {
		t.Fatalf("check = %q", envelope.Check)
	}
	if envelope.Cause == "" {
		t.Fatal("no kernel cause carried for a real EACCES")
	}
	if strings.Contains(contractErr.Detail, root) {
		t.Fatalf("detail leaked the host temp root path: %q", contractErr.Detail)
	}
}

// TestCreateInvocationScratchSurfacesNamesItsSite is the A1/A2 proof for
// tools.go:137: called directly against a caller-chosen scratch directory
// (as newToolSession calls it once os.MkdirTemp has already succeeded), a
// pre-existing regular file named "home" makes the surface Mkdir fail with
// a real kernel EEXIST, named at its own distinct site.
func TestCreateInvocationScratchSurfacesNamesItsSite(t *testing.T) {
	t.Parallel()
	scratch := t.TempDir()
	if err := os.WriteFile(filepath.Join(scratch, "home"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := createInvocationScratchSurfaces(scratch)
	if err == nil {
		t.Fatal("colliding surface name silently escaped")
	}
	contractErr, ok := err.(*ContractError)
	if !ok || contractErr.Code != "PROCESS_START_FAILED" {
		t.Fatalf("err = %#v", err)
	}
	var envelope sandboxStartDetail
	if jsonErr := json.Unmarshal([]byte(contractErr.Detail), &envelope); jsonErr != nil {
		t.Fatalf("detail = %q: %v", contractErr.Detail, jsonErr)
	}
	if envelope.Check != "sandbox_start.home_tmp_surface_create" {
		t.Fatalf("check = %q", envelope.Check)
	}
	if envelope.Cause == "" {
		t.Fatal("no kernel cause carried for a real EEXIST")
	}
	if strings.Contains(contractErr.Detail, scratch) {
		t.Fatalf("detail leaked the host scratch path: %q", contractErr.Detail)
	}
}
