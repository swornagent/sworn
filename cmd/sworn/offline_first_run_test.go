package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestOfflineBuildAndFirstRunNeedsNoBatonInstallation is the acceptance-linked
// A1 proof: an ordinary offline build, with a PATH that contains no "baton"
// executable and network module fetches disabled, still builds Sworn and
// completes its first responsibilities (version and init) using only its own
// embedded, self-consistent role assets.
func TestOfflineBuildAndFirstRunNeedsNoBatonInstallation(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "sworn")
	command := exec.Command(
		"go", "build", "-mod=readonly", "-buildvcs=false", "-trimpath",
		"-o", binary, ".",
	)
	command.Env = cleanEnvironment(map[string]string{
		"CGO_ENABLED": "0",
		"GOCACHE":     t.TempDir(),
		"GOFLAGS":     "-buildvcs=false",
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
		"GOPROXY":     "off",
	})
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("offline build failed: %v: %s", err, output)
	}

	offlinePath := offlinePathWithoutBaton(t)

	var version bytes.Buffer
	versionCmd := exec.Command(binary, "version", "--json")
	versionCmd.Env = []string{"PATH=" + offlinePath, "HOME=" + t.TempDir()}
	versionCmd.Stdout = &version
	var versionStderr bytes.Buffer
	versionCmd.Stderr = &versionStderr
	if err := versionCmd.Run(); err != nil {
		t.Fatalf("offline version failed: %v: %s", err, versionStderr.String())
	}
	if !bytes.Contains(version.Bytes(), []byte(`"role_assets_version": "sworn.role-assets/v1"`)) {
		t.Fatalf("offline version output = %q, want Sworn-owned role-asset identity", version.String())
	}

	projectDir := t.TempDir()
	if err := exec.Command("git", "-C", projectDir, "init", "-q").Run(); err != nil {
		t.Fatal(err)
	}

	// init also requires a native coding-agent CLI (codex or claude) on PATH;
	// that is a driver concern independent of this slice. The offline,
	// Baton-free proof is that init never fails for a Baton reason even when
	// no such agent is present.
	var initOut, initErr bytes.Buffer
	initCmd := exec.Command(binary, "init", "--project", projectDir)
	initCmd.Env = []string{"PATH=" + offlinePath, "HOME=" + t.TempDir()}
	initCmd.Stdout = &initOut
	initCmd.Stderr = &initErr
	err := initCmd.Run()
	combined := strings.ToLower(initOut.String() + initErr.String())
	for _, phrase := range []string{
		"install baton", "restore baton", "upgrade baton", "certify baton", "baton package",
	} {
		if strings.Contains(combined, phrase) {
			t.Fatalf("offline init asked the operator about a separate Baton product: %q", combined)
		}
	}
	if err == nil && !strings.Contains(initOut.String(), "Project: ") {
		t.Fatalf("offline init output = %q", initOut.String())
	}
	if err != nil && !strings.Contains(combined, "no supported agent command was found on path") {
		t.Fatalf("offline init failed for an unexpected reason: %v: %s", err, combined)
	}
}

// offlinePathWithoutBaton returns a PATH containing only the real
// directories that hold git, sh, and core utilities Sworn's own commands may
// invoke, asserting none of them is named or contains an executable named
// "baton".
func offlinePathWithoutBaton(t *testing.T) string {
	t.Helper()
	seen := make(map[string]bool)
	var dirs []string
	for _, tool := range []string{"git", "sh", "env"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Fatalf("required tool %q is not available: %v", tool, err)
		}
		dir := filepath.Dir(path)
		if seen[dir] {
			continue
		}
		seen[dir] = true
		dirs = append(dirs, dir)
		if _, err := exec.LookPath(filepath.Join(dir, "baton")); err == nil {
			t.Fatalf("required tool directory %q also contains a baton executable", dir)
		}
		if entries, err := os.ReadDir(dir); err == nil {
			for _, entry := range entries {
				if entry.Name() == "baton" {
					t.Fatalf("tool directory %q contains a baton executable", dir)
				}
			}
		}
	}
	return strings.Join(dirs, string(os.PathListSeparator))
}
