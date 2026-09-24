package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/protocol"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
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

	// 3. protocol.RecordError with Msg
	var protocolBuf bytes.Buffer
	protocolErr := &protocol.RecordError{
		Code: "INVALID_FIELD",
		Msg:  "touchpoints[0] must be a string of 1-512 characters (got 600)",
	}
	writeCommandFailure(&protocolBuf, "test-cmd", "Plan validation failed.", protocolErr)
	protocolOut := protocolBuf.String()
	if !strings.Contains(protocolOut, "sworn test-cmd: Plan validation failed.\n") {
		t.Fatalf("missing fallback sentence: %q", protocolOut)
	}
	if !strings.Contains(protocolOut, "Technical code: INVALID_FIELD\n") {
		t.Fatalf("missing technical code: %q", protocolOut)
	}
	if !strings.Contains(protocolOut, "touchpoints[0] must be a string of 1-512 characters (got 600)\n") {
		t.Fatalf("missing protocol error detail: %q", protocolOut)
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
	// After S1 the revision is resolved once at the boundary through the
	// sanitized gitx resolver, so an unresolvable value is refused with
	// REVISION_NOT_FOUND before the protocol is reached. The refusal names
	// the flag and never echoes the value, a path, or raw git output.
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
	if !strings.Contains(stderrStr, "Technical code: REVISION_NOT_FOUND") {
		t.Fatalf("stderr missing REVISION_NOT_FOUND:\n%s", stderrStr)
	}
	if !strings.Contains(stderrStr, "--commit") {
		t.Fatalf("stderr does not name the flag:\n%s", stderrStr)
	}
	if strings.Contains(stderrStr, nonexistentCommit) {
		t.Fatalf("stderr echoed the revision value:\n%s", stderrStr)
	}
	if strings.Contains(stderrStr, root) {
		t.Fatalf("stderr echoed a path:\n%s", stderrStr)
	}
	if strings.Contains(stderrStr, "fatal:") || strings.Contains(stderrStr, "ls-tree") {
		t.Fatalf("stderr echoed raw git output:\n%s", stderrStr)
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
		version: "9.9",
		output:  "9.9 (Claude Code)",
	}

	_, _, err := buildDriverConfig(agent)
	if err == nil {
		t.Fatal("buildDriverConfig should fail on a malformed version")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "Technical code: INVALID_ADAPTER") {
		t.Fatalf("expected Technical code: INVALID_ADAPTER in error, got: %s", errStr)
	}
	if !strings.Contains(errStr, "version") {
		t.Fatalf("expected condition detail 'version' in error, got: %s", errStr)
	}
}

func refusalsPrettyManifest(t *testing.T, runID string) (canonical, pretty []byte) {
	t.Helper()
	canonical = operatorManifestBody(t, runID, "refusal hint")
	var decoded map[string]any
	if err := json.Unmarshal(canonical, &decoded); err != nil {
		t.Fatal(err)
	}
	indented, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	pretty = append(indented, '\n')
	if _, err := runtimepkg.ParseManifest(pretty); err == nil || !runtimepkg.IsCode(err, "NONCANONICAL_MANIFEST") {
		t.Fatalf("pretty err = %v, want NONCANONICAL_MANIFEST", err)
	}
	return canonical, pretty
}

func refusalsNoncanonicalDriver(t *testing.T) (canonical, noncanonical []byte) {
	t.Helper()
	canonical = manifestCanonicalDriverBytes(t)
	noncanonical = append(append([]byte(nil), canonical...), '\n')
	if _, err := driver.DecodeDriverConfig(noncanonical); !driver.IsCode(err, "NONCANONICAL_JSON") {
		t.Fatalf("noncanonical driver err = %v, want NONCANONICAL_JSON", err)
	}
	return canonical, noncanonical
}

func TestNoncanonicalManifestRefusalsNameCanonicalCommand(t *testing.T) {
	t.Parallel()
	_, pretty := refusalsPrettyManifest(t, "run-refusal-manifest")
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(manifestPath, pretty, 0o600); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(t.TempDir(), "run.sqlite")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"run", "--manifest", manifestPath, "--journal", journalPath}, &stdout, &stderr); code != 1 {
		t.Fatalf("run noncanonical = %d, want 1; stderr=%s", code, stderr.String())
	}
	out := stderr.String()
	if !strings.Contains(out, "Technical code: NONCANONICAL_MANIFEST") {
		t.Fatalf("run stderr missing NONCANONICAL_MANIFEST:\n%s", out)
	}
	if !strings.Contains(out, "sworn manifest canonical --manifest") {
		t.Fatalf("run stderr missing manifest canonical hint:\n%s", out)
	}
	if strings.Contains(out, manifestPath) || strings.Contains(out, journalPath) {
		t.Fatalf("run stderr echoed a path:\n%s", out)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"serve", "--run", "run-refusal-manifest", "--journal", journalPath, "--manifest", manifestPath}, &stdout, &stderr); code != 1 {
		t.Fatalf("serve noncanonical = %d, want 1; stderr=%s", code, stderr.String())
	}
	out = stderr.String()
	if !strings.Contains(out, "Technical code: NONCANONICAL_MANIFEST") {
		t.Fatalf("serve stderr missing NONCANONICAL_MANIFEST:\n%s", out)
	}
	if !strings.Contains(out, "manifest (--manifest)") {
		t.Fatalf("serve stderr missing manifest detail:\n%s", out)
	}
	if !strings.Contains(out, "sworn manifest canonical --manifest") {
		t.Fatalf("serve stderr missing manifest canonical hint:\n%s", out)
	}
	// Detail order: input kind first, hint second.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("serve stderr want 4 lines, got %d:\n%s", len(lines), out)
	}
	if lines[2] != "manifest (--manifest)" {
		t.Fatalf("serve line2 = %q, want manifest detail", lines[2])
	}
	if !strings.Contains(lines[3], "sworn manifest canonical --manifest") {
		t.Fatalf("serve line3 = %q, want hint", lines[3])
	}
	if strings.Contains(out, manifestPath) {
		t.Fatalf("serve stderr echoed a path:\n%s", out)
	}
}

func TestNoncanonicalDriverConfigRefusalsNameCanonicalCommand(t *testing.T) {
	t.Parallel()
	_, noncanonical := refusalsNoncanonicalDriver(t)
	configPath := filepath.Join(t.TempDir(), "drivers.json")
	if err := os.WriteFile(configPath, noncanonical, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"driver", "doctor", "--config", configPath, "--profile", "openai", "--model", "model-one", "--json"}, &stdout, &stderr); code != 1 {
		t.Fatalf("driver doctor noncanonical = %d, want 1; stderr=%s", code, stderr.String())
	}
	out := stderr.String()
	if !strings.Contains(out, "Technical code: NONCANONICAL_JSON") {
		t.Fatalf("driver stderr missing NONCANONICAL_JSON:\n%s", out)
	}
	if !strings.Contains(out, "sworn manifest canonical --driver-config") {
		t.Fatalf("driver stderr missing driver canonical hint:\n%s", out)
	}
	if strings.Contains(out, configPath) {
		t.Fatalf("driver stderr echoed a path:\n%s", out)
	}
	// run --config with a good manifest and a noncanonical driver config.
	canonicalManifest := operatorManifestBody(t, "run-refusal-driver", "driver hint")
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(manifestPath, canonicalManifest, 0o600); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(t.TempDir(), "run.sqlite")
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"run", "--manifest", manifestPath, "--journal", journalPath, "--config", configPath}, &stdout, &stderr); code != 1 {
		t.Fatalf("run --config noncanonical = %d, want 1; stderr=%s", code, stderr.String())
	}
	out = stderr.String()
	if !strings.Contains(out, "Technical code: NONCANONICAL_JSON") {
		t.Fatalf("run stderr missing NONCANONICAL_JSON:\n%s", out)
	}
	if !strings.Contains(out, "sworn manifest canonical --driver-config") {
		t.Fatalf("run stderr missing driver canonical hint:\n%s", out)
	}
	// serve --config with a good manifest and a noncanonical driver config.
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"serve", "--run", "run-refusal-driver", "--journal", journalPath, "--manifest", manifestPath, "--config", configPath}, &stdout, &stderr); code != 1 {
		t.Fatalf("serve --config noncanonical = %d, want 1; stderr=%s", code, stderr.String())
	}
	out = stderr.String()
	if !strings.Contains(out, "Technical code: NONCANONICAL_JSON") {
		t.Fatalf("serve stderr missing NONCANONICAL_JSON:\n%s", out)
	}
	if !strings.Contains(out, "driver config (--config)") {
		t.Fatalf("serve stderr missing driver detail:\n%s", out)
	}
	if !strings.Contains(out, "sworn manifest canonical --driver-config") {
		t.Fatalf("serve stderr missing driver canonical hint:\n%s", out)
	}
}

func TestDriverConfigHintCoversNonRunCommands(t *testing.T) {
	t.Parallel()
	_, noncanonical := refusalsNoncanonicalDriver(t)
	configPath := filepath.Join(t.TempDir(), "drivers.json")
	if err := os.WriteFile(configPath, noncanonical, 0o600); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(t.TempDir(), "run.sqlite")
	// answer loads --config through openRuntimeService like every control
	// command; a noncanonical driver config must name the fix there too.
	var stdout, stderr bytes.Buffer
	if code := run([]string{"answer", "--run", "run-1", "--journal", journalPath, "--attention", "sha256:" + strings.Repeat("a", 64), "--generation", "1", "--answer", "yes", "--config", configPath}, &stdout, &stderr); code != 1 {
		t.Fatalf("answer --config noncanonical = %d, want 1; stderr=%s", code, stderr.String())
	}
	out := stderr.String()
	if !strings.Contains(out, "Technical code: NONCANONICAL_JSON") {
		t.Fatalf("answer stderr missing NONCANONICAL_JSON:\n%s", out)
	}
	if !strings.Contains(out, "sworn manifest canonical --driver-config") {
		t.Fatalf("answer stderr missing driver canonical hint:\n%s", out)
	}
	if strings.Contains(out, configPath) {
		t.Fatalf("answer stderr echoed a path:\n%s", out)
	}
}

func TestDriverConfigHintRequiresConfigSource(t *testing.T) {
	t.Parallel()
	// A NONCANONICAL_JSON that did not come from the driver config (for
	// example a submission, seal, or request decode) must not name the
	// driver-config fix.
	var plain bytes.Buffer
	writeCommandFailure(&plain, "test-cmd", "Fallback.", &driver.ContractError{Code: "NONCANONICAL_JSON"})
	if strings.Contains(plain.String(), "sworn manifest canonical") {
		t.Fatalf("unmarked NONCANONICAL_JSON gained a hint:\n%s", plain.String())
	}
	if !strings.Contains(plain.String(), "Technical code: NONCANONICAL_JSON") {
		t.Fatalf("unmarked output missing code:\n%s", plain.String())
	}
	// A marked driver-config error keeps its true code and detail through
	// the wrapper.
	var marked bytes.Buffer
	writeCommandFailure(&marked, "test-cmd", "Fallback.", &driverConfigError{err: &driver.ContractError{Code: "NONCANONICAL_JSON"}})
	if !strings.Contains(marked.String(), "Technical code: NONCANONICAL_JSON") {
		t.Fatalf("marked output missing code:\n%s", marked.String())
	}
	if !strings.Contains(marked.String(), "sworn manifest canonical --driver-config") {
		t.Fatalf("marked output missing hint:\n%s", marked.String())
	}
	// For serve the hint also requires the driver-config input.
	var wrongInput bytes.Buffer
	writeCommandFailure(&wrongInput, "serve", "Fallback.", &serveInputError{input: serveInputJournal, err: &driverConfigError{err: &driver.ContractError{Code: "NONCANONICAL_JSON"}}})
	if strings.Contains(wrongInput.String(), "sworn manifest canonical") {
		t.Fatalf("serve journal input gained a driver hint:\n%s", wrongInput.String())
	}
	var unmarkedServe bytes.Buffer
	writeCommandFailure(&unmarkedServe, "serve", "Fallback.", &serveInputError{input: serveInputDriverConfig, err: &driver.ContractError{Code: "NONCANONICAL_JSON"}})
	if strings.Contains(unmarkedServe.String(), "sworn manifest canonical") {
		t.Fatalf("unmarked serve driver input gained a hint:\n%s", unmarkedServe.String())
	}
	var markedServe bytes.Buffer
	writeCommandFailure(&markedServe, "serve", "Fallback.", &serveInputError{input: serveInputDriverConfig, err: &driverConfigError{err: &driver.ContractError{Code: "NONCANONICAL_JSON"}}})
	if !strings.Contains(markedServe.String(), "sworn manifest canonical --driver-config") {
		t.Fatalf("marked serve driver input missing hint:\n%s", markedServe.String())
	}
}
