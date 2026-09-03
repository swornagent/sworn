package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
)

func TestWriteCommandFailureRendersUnderlyingErrorDetail(t *testing.T) {
	t.Parallel()

	// 1. gitx.Error with args and stderr
	var gitxBuf bytes.Buffer
	gitxErr := &gitx.Error{
		Code: "CUSTOM_GIT_FAILED",
		Op:   "checkout -b test-branch",
		Err:  errors.New("fatal: a branch named 'test-branch' already exists"),
	}
	writeCommandFailure(&gitxBuf, "test-cmd", "Git operation failed.", gitxErr)
	gitxOut := gitxBuf.String()
	if !strings.Contains(gitxOut, "sworn test-cmd: Git operation failed.\n") {
		t.Fatalf("missing fallback sentence: %q", gitxOut)
	}
	if !strings.Contains(gitxOut, "Technical code: CUSTOM_GIT_FAILED\n") {
		t.Fatalf("missing technical code: %q", gitxOut)
	}
	if !strings.Contains(gitxOut, "checkout -b test-branch: fatal: a branch named 'test-branch' already exists\n") {
		t.Fatalf("missing gitx error detail: %q", gitxOut)
	}

	// 2. driver.ContractError with Detail
	var driverBuf bytes.Buffer
	driverErr := &driver.ContractError{
		Code:   "NATIVE_NOT_CERTIFIED",
		Detail: "toolchain_root",
	}
	writeCommandFailure(&driverBuf, "test-cmd", "Driver certification failed.", driverErr)
	driverOut := driverBuf.String()
	if !strings.Contains(driverOut, "sworn test-cmd: Driver certification failed.\n") {
		t.Fatalf("missing fallback sentence: %q", driverOut)
	}
	if !strings.Contains(driverOut, "Technical code: NATIVE_NOT_CERTIFIED\n") {
		t.Fatalf("missing technical code: %q", driverOut)
	}
	if !strings.Contains(driverOut, "toolchain_root\n") {
		t.Fatalf("missing driver error detail: %q", driverOut)
	}

	// 3. baton.RecordError with Msg
	var batonBuf bytes.Buffer
	batonErr := &baton.RecordError{
		Code: "INVALID_FIELD",
		Msg:  "touchpoints[0] must be a string of 1-512 characters (got 600)",
	}
	writeCommandFailure(&batonBuf, "test-cmd", "Plan validation failed.", batonErr)
	batonOut := batonBuf.String()
	if !strings.Contains(batonOut, "sworn test-cmd: Plan validation failed.\n") {
		t.Fatalf("missing fallback sentence: %q", batonOut)
	}
	if !strings.Contains(batonOut, "Technical code: INVALID_FIELD\n") {
		t.Fatalf("missing technical code: %q", batonOut)
	}
	if !strings.Contains(batonOut, "touchpoints[0] must be a string of 1-512 characters (got 600)\n") {
		t.Fatalf("missing baton error detail: %q", batonOut)
	}
}

func TestWriteCommandFailureBoundsAndSanitizesMultiLineOversizedDetail(t *testing.T) {
	t.Parallel()

	// Multi-line stderr with newlines, tabs, and excess characters (> 512 bytes)
	multiLineStderr := "error: line 1\n\terror: line 2\r\nerror: line 3\x00" + strings.Repeat(" padding", 100)
	gitxErr := &gitx.Error{
		Code: "GIT_EXECUTION_FAILED",
		Op:   "commit -m fixture",
		Err:  errors.New(multiLineStderr),
	}

	var buf bytes.Buffer
	writeCommandFailure(&buf, "git-op", "Fallback message.", gitxErr)
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected exactly 3 lines (message, code, detail), got %d: %q", len(lines), buf.String())
	}
	if lines[0] != "sworn git-op: Fallback message." {
		t.Fatalf("unexpected line 0: %q", lines[0])
	}
	if lines[1] != "Technical code: GIT_EXECUTION_FAILED" {
		t.Fatalf("unexpected line 1: %q", lines[1])
	}

	detailLine := lines[2]
	if len(detailLine) > maxCommandErrorDetailBytes {
		t.Fatalf("detail line exceeds max bound (%d bytes): got %d bytes", maxCommandErrorDetailBytes, len(detailLine))
	}
	if !strings.HasSuffix(detailLine, detailTruncationMarker) {
		t.Fatalf("detail line does not end with truncation marker %q: %q", detailTruncationMarker, detailLine)
	}
	if strings.Contains(detailLine, "\n") || strings.Contains(detailLine, "\r") || strings.Contains(detailLine, "\t") {
		t.Fatalf("detail line contains uncollapsed whitespace or newlines: %q", detailLine)
	}
	if !strings.HasPrefix(detailLine, "commit -m fixture: error: line 1 error: line 2 error: line 3") {
		t.Fatalf("detail line prefix unexpected: %q", detailLine)
	}
}

func TestPlanCommandsSurfaceRecordErrorMsg(t *testing.T) {
	t.Parallel()
	root := planTestRepo(t)

	// A fixture with a malformed plan fence that causes ParsePlan to fail with INVALID_PLAN_FENCE
	badManifestPath := filepath.Join(root, "bad_manifest.md")
	badManifestContent := []byte("```invalid-fence\n{}\n```\n")
	if err := os.WriteFile(badManifestPath, badManifestContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. plan pin
	var pinOut, pinErr bytes.Buffer
	code := runPlan([]string{"pin", "--manifest", badManifestPath, "--project", root}, &pinOut, &pinErr)
	if code == 0 {
		t.Fatal("plan pin should fail on bad fence")
	}
	if !strings.Contains(pinErr.String(), "Technical code: INVALID_PLAN_FENCE") {
		t.Fatalf("plan pin stderr missing code: %s", pinErr.String())
	}
	if !strings.Contains(pinErr.String(), "plan must begin at byte zero with a known schema fence") {
		t.Fatalf("plan pin stderr missing Msg: %s", pinErr.String())
	}

	// 2. plan lint
	var lintOut, lintErr bytes.Buffer
	code = runPlan([]string{"lint", "--manifest", badManifestPath, "--project", root}, &lintOut, &lintErr)
	if code == 0 {
		t.Fatal("plan lint should fail on bad fence")
	}
	if !strings.Contains(lintErr.String(), "Technical code: INVALID_PLAN_FENCE") {
		t.Fatalf("plan lint stderr missing code: %s", lintErr.String())
	}
	if !strings.Contains(lintErr.String(), "plan must begin at byte zero with a known schema fence") {
		t.Fatalf("plan lint stderr missing Msg: %s", lintErr.String())
	}

	// 3. plan record
	var recOut, recErr bytes.Buffer
	code = runPlan([]string{"record", "--manifest", badManifestPath, "--project", root, "--summary", "Test summary."}, &recOut, &recErr)
	if code == 0 {
		t.Fatal("plan record should fail on bad fence")
	}
	if !strings.Contains(recErr.String(), "Technical code: INVALID_PLAN_FENCE") {
		t.Fatalf("plan record stderr missing code: %s", recErr.String())
	}
	if !strings.Contains(recErr.String(), "plan must begin at byte zero with a known schema fence") {
		t.Fatalf("plan record stderr missing Msg: %s", recErr.String())
	}
}

func TestGitExecutionFailedShowsGitArgsAndStderr(t *testing.T) {
	t.Parallel()
	root := planTestRepo(t)

	// Create a valid manifest referencing a contract
	contractPath := "contracts/S1.json"
	contractRaw := planContractRaw(t, planContractBody("S1", "one/file.go"))
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(contractPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, contractPath), contractRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(root, "manifest.md")
	drifted := planManifestBytes(t, "git-fail-cli", contractPath, "one/file.go", "sha256:"+strings.Repeat("0", 64))
	if err := os.WriteFile(manifestPath, drifted, 0o644); err != nil {
		t.Fatal(err)
	}

	// Provide a valid-syntax 40-character hex commit OID that does not exist in the repository.
	// This drives baton.PinManifest -> readGitFileAt -> gitx.Repository.ListTree -> real git ls-tree execution,
	// which fails with GIT_EXECUTION_FAILED.
	nonexistentCommit := strings.Repeat("1", 40)
	var stdout, stderr bytes.Buffer
	code := runPlan([]string{
		"pin",
		"--manifest", manifestPath,
		"--project", root,
		"--commit", nonexistentCommit,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("runPlan exit = %d, want 1; stderr = %s", code, stderr.String())
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "Technical code: GIT_EXECUTION_FAILED") {
		t.Fatalf("stderr missing GIT_EXECUTION_FAILED:\n%s", stderrStr)
	}
	// Assert the executed git arguments (Op) are present in output
	if !strings.Contains(stderrStr, "ls-tree") || !strings.Contains(stderrStr, nonexistentCommit) {
		t.Fatalf("stderr missing git args (Op):\n%s", stderrStr)
	}
	// Assert the git stderr failure text (Err) is present in output
	if !strings.Contains(stderrStr, "fatal: not a tree object") {
		t.Fatalf("stderr missing git stderr text (Err):\n%s", stderrStr)
	}
}

func TestDriverDoctorSurfacesAdmissionConditionDetail(t *testing.T) {
	t.Parallel()

	// Construct a driver config fixture with an invalid adapter ID in native config
	bin := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(t.TempDir(), "driver.json")
	credKey := "fixture-cred"
	config := driver.DriverConfig{
		SchemaVersion: driver.DriverConfigSchemaVersion,
		Credentials: []driver.DriverCredentialSource{{
			Key:       credKey,
			Kind:      driver.CredentialFile,
			Reference: "/tmp/fake.json",
		}},
		Adapters: []driver.DriverAdapterConfig{{
			Native: &driver.NativeAdapterConfig{
				Key:                    "agent-adapter",
				ID:                     "sworn.claude",
				Version:                "1.0.0",
				Family:                 driver.ProfileClaude,
				CLI:                    driver.ExecutableIdentity{Path: bin, Digest: driver.ClaudeCLIDigest},
				CLIVersion:             "9.9.9",
				CredentialTarget:       driver.ClaudeCredentialTarget,
				CredentialRefs:         []string{credKey},
				VersionOutput:          driver.ClaudeCLIVersion + " (Claude Code)",
				MaxCredentialBytes:     1_048_576,
				RequiredRuntimeTargets: []string{"/etc/hosts"},
			},
		}},
		Profiles: []driver.DriverProfile{{
			Key:                 "claude-profile",
			Adapter:             "agent-adapter",
			Network:             driver.NetworkRequired,
			CredentialSource:    &credKey,
			CertificationModels: []string{"test-model"},
		}},
	}
	body, err := driver.EncodeDriverConfig(config)
	if err != nil {
		t.Fatalf("EncodeDriverConfig: %v", err)
	}
	if err := os.WriteFile(configPath, body, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"driver", "doctor",
		"--config", configPath,
		"--profile", "claude-profile",
		"--model", "test-model",
		"--json",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("driver doctor exit = %d, want 1; stderr = %s", code, stderr.String())
	}
	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "Technical code: INVALID_ADAPTER") {
		t.Fatalf("stderr missing Technical code: INVALID_ADAPTER:\n%s", stderrStr)
	}
	if !strings.Contains(stderrStr, "version") {
		t.Fatalf("stderr missing condition detail 'version':\n%s", stderrStr)
	}
}

func TestBuildDriverConfigSurfacesConditionDetail(t *testing.T) {
	backing := make(map[string]string, len(initRuntimeTargets))
	for i, target := range initRuntimeTargets {
		p := filepath.Join(t.TempDir(), fmt.Sprintf("target_%d", i))
		if err := os.WriteFile(p, []byte(fmt.Sprintf("mock %d\n", i)), 0o644); err != nil {
			t.Fatal(err)
		}
		backing[target] = p
	}
	oldResolve := initResolveRuntimePath
	initResolveRuntimePath = func(target string) (string, error) {
		resolved, found := backing[target]
		if !found {
			return "", fmt.Errorf("unexpected target %s", target)
		}
		return resolved, nil
	}
	t.Cleanup(func() {
		initResolveRuntimePath = oldResolve
	})

	credDir := t.TempDir()
	t.Setenv(gitx.EnvCredentialsDir, credDir)
	credPath := agentCredentialSource(driver.ProfileClaude)
	if err := os.MkdirAll(filepath.Dir(credPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	agent := detectedAgent{
		initAgent: initAgent{
			name: "Claude Code", family: driver.ProfileClaude, command: "claude",
			target: driver.ClaudeCredentialTarget,
		},
		binary:  bin,
		digest:  driver.ClaudeCLIDigest,
		version: "9.9.9",
		output:  "9.9.9 (Claude Code)",
	}

	_, _, err := buildDriverConfig(agent)
	if err == nil {
		t.Fatal("buildDriverConfig should fail on mismatched version")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "Technical code: INVALID_ADAPTER") {
		t.Fatalf("expected Technical code: INVALID_ADAPTER in error, got: %s", errStr)
	}
	if !strings.Contains(errStr, "version") {
		t.Fatalf("expected condition detail 'version' in error, got: %s", errStr)
	}
}
