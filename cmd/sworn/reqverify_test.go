package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// fakeVerifier returns a canned reply for model dispatch.
type fakeVerifier struct {
	reply string
	cost  float64
}

func (f fakeVerifier) Verify(context.Context, string, string) (string, float64, int64, int64, error) {
	return f.reply, f.cost, 0, 0, nil
}
// errVerifier returns an error on dispatch, simulating a model failure.
type errVerifier struct{}

func (errVerifier) Verify(context.Context, string, string) (string, float64, int64, int64, error) {
	return "", 0, 0, 0, context.Canceled
}
// writeReqverifyFixture creates a slice spec.md under a temp release directory.
func writeReqverifyFixture(t *testing.T, releaseDir, sliceID, spec string) {
	t.Helper()
	dir := filepath.Join(releaseDir, sliceID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestReqverifyCmd_MissingReleaseArg verifies that `sworn reqverify` without a
// release argument exits 64 (usage error).
func TestReqverifyCmd_MissingReleaseArg(t *testing.T) {
	exit := cmdReqverify([]string{})
	if exit != 64 {
		t.Errorf("expected exit 64 for missing release arg, got %d", exit)
	}
}

// TestReqverifyCmd_NonexistentRelease verifies that `sworn reqverify <nonexistent>`
// exits 2.
func TestReqverifyCmd_NonexistentRelease(t *testing.T) {
	exit := cmdReqverify([]string{"nonexistent-release-xyz"})
	if exit != 2 {
		t.Errorf("expected exit 2 for nonexistent release, got %d", exit)
	}
}

// TestReqverifyCmd_NoModelConfigured verifies that `sworn reqverify` with a valid
// release exits 2 when no model is configured.
func TestReqverifyCmd_NoModelConfigured(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0755)

	// Create an index.md so the release dir looks valid.
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte("---\ntitle: Test\n---"), 0644)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdReqverify([]string{"test-release"})
	// No model configured — should exit 2.
	if exit != 2 {
		t.Errorf("expected exit 2 when no model configured, got %d", exit)
	}
}

// TestReqverifyCmdWithVerifier_AllPass verifies that when all ACs pass the
// reqverify injectable path returns exit 0.
func TestReqverifyCmdWithVerifier_AllPass(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0755)

	writeReqverifyFixture(t, releaseDir, "S01-test", `## Acceptance checks

- [ ] THE SYSTEM SHALL do something.
- [ ] WHEN a user clicks save THE SYSTEM SHALL persist.
`)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	v := fakeVerifier{reply: `## RESULTS

AC 1 (S01-test): PASS
AC 2 (S01-test): PASS`}

	exit := cmdReqverifyWithVerifier("test-release", v)
	if exit != 0 {
		t.Errorf("expected exit 0 for all-pass, got %d", exit)
	}
}

// TestReqverifyCmdWithVerifier_Violations verifies that when a non-singular AC
// is detected, the reqverify injectable path returns exit 1.
func TestReqverifyCmdWithVerifier_Violations(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0755)

	writeReqverifyFixture(t, releaseDir, "S01-test", `## Acceptance checks

- [ ] THE SYSTEM SHALL do something.
- [ ] WHEN Y THE SYSTEM SHALL do Z and also do W.
`)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	v := fakeVerifier{reply: `## RESULTS

AC 1 (S01-test): PASS
AC 2 (S01-test): FAIL — singular [bundles two actions]`}

	exit := cmdReqverifyWithVerifier("test-release", v)
	if exit != 1 {
		t.Errorf("expected exit 1 for violations, got %d", exit)
	}
}

// TestReqverifyCmdWithVerifier_AmbiguousViolation verifies that when an ambiguous
// AC is detected, the reqverify injectable path returns exit 1.
func TestReqverifyCmdWithVerifier_AmbiguousViolation(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0755)

	writeReqverifyFixture(t, releaseDir, "S01-test", `## Acceptance checks

- [ ] THE SYSTEM SHALL do something.
- [ ] THE SYSTEM SHALL display the data appropriately.
`)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	v := fakeVerifier{reply: `## RESULTS

AC 1 (S01-test): PASS
AC 2 (S01-test): FAIL — ambiguous [could mean any format]`}

	exit := cmdReqverifyWithVerifier("test-release", v)
	if exit != 1 {
		t.Errorf("expected exit 1 for ambiguous violation, got %d", exit)
	}
}

// TestReqverifyCmdWithVerifier_IncompleteViolation verifies that when an incomplete
// AC is detected, the reqverify injectable path returns exit 1.
func TestReqverifyCmdWithVerifier_IncompleteViolation(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0755)

	writeReqverifyFixture(t, releaseDir, "S01-test", `## Acceptance checks

- [ ] THE SYSTEM SHALL display the dashboard.
- [ ] THE SYSTEM SHALL notify the user.
`)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	v := fakeVerifier{reply: `## RESULTS

AC 1 (S01-test): PASS
AC 2 (S01-test): FAIL — incomplete [lacks trigger condition]`}

	exit := cmdReqverifyWithVerifier("test-release", v)
	if exit != 1 {
		t.Errorf("expected exit 1 for incomplete violation, got %d", exit)
	}
}

// TestReqverifyCmdWithVerifier_ModelError verifies that a model dispatch error// through the injectable path returns exit 2.
func TestReqverifyCmdWithVerifier_ModelError(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0755)

	writeReqverifyFixture(t, releaseDir, "S01-test", `## Acceptance checks

- [ ] THE SYSTEM SHALL do something.
`)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(dir)

	exit := cmdReqverifyWithVerifier("test-release", errVerifier{})
	if exit != 2 {
		t.Errorf("expected exit 2 for model error, got %d", exit)
	}
}

// TestReqverifyCmdWithVerifier_NonexistentRelease verifies that the injectable
// path returns exit 2 for a release that doesn't exist.
func TestReqverifyCmdWithVerifier_NonexistentRelease(t *testing.T) {
	exit := cmdReqverifyWithVerifier("nonexistent-release-xyz", fakeVerifier{reply: ""})
	if exit != 2 {
		t.Errorf("expected exit 2 for nonexistent release, got %d", exit)
	}
}
