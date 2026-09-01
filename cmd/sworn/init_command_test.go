package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/observe"
)

// TestAgentCredentialSourceUsesResolvedCredentialsDir proves the A2 default
// for the credentials directory: the XDG-conformant default
// ($XDG_CONFIG_HOME/sworn) is effective even when SWORN_CREDENTIALS_DIR is
// unset, and is never bypassed in favour of the user home.
func TestAgentCredentialSourceUsesResolvedCredentialsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv(gitx.EnvCredentialsDir, "")
	configHome := filepath.Join(home, ".config")
	got := agentCredentialSource(driver.ProfileCodex)
	if want := filepath.Join(configHome, "sworn", ".codex", "auth.json"); got != want {
		t.Fatalf("codex credential = %q, want XDG default %q", got, want)
	}
	got = agentCredentialSource(driver.ProfileClaude)
	if want := filepath.Join(configHome, "sworn", ".claude", ".credentials.json"); got != want {
		t.Fatalf("claude credential = %q, want XDG default %q", got, want)
	}
	// An explicit override relocates the base; the override is honoured.
	t.Setenv(gitx.EnvCredentialsDir, filepath.Join(home, "custom"))
	got = agentCredentialSource(driver.ProfileCodex)
	if want := filepath.Join(home, "custom", ".codex", "auth.json"); got != want {
		t.Fatalf("overridden codex credential = %q, want %q", got, want)
	}
}

func initTestProject(t *testing.T) string {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	root := t.TempDir()
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "."},
		{"-c", "user.name=t", "-c", "user.email=t@example.com",
			"commit", "-q", "--allow-empty", "-m", "base"},
	} {
		command := exec.Command(git, args...)
		command.Dir = resolved
		command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	return resolved
}

func setupMockAgentAndEnvironment(t *testing.T) (string, string) {
	t.Helper()
	temp := t.TempDir()

	// Mock runtime files: the CANONICAL targets stay (native adapter
	// admission requires the four /etc names verbatim, and init now
	// round-trips its scaffold through that admission - sworn#267), while
	// resolution is redirected to hermetic fixture files so no host /etc
	// content leaks into the pinned digests.
	backing := make(map[string]string, len(initRuntimeTargets))
	for i, target := range initRuntimeTargets {
		path := filepath.Join(temp, "etc", filepath.Base(target))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(fmt.Sprintf("mock target %d\n", i)), 0o644); err != nil {
			t.Fatal(err)
		}
		backing[target] = path
	}
	oldResolve := initResolveRuntimePath
	initResolveRuntimePath = func(target string) (string, error) {
		resolved, found := backing[target]
		if !found {
			return "", fmt.Errorf("unexpected runtime target %s", target)
		}
		return resolved, nil
	}
	t.Cleanup(func() {
		initResolveRuntimePath = oldResolve
	})

	binDir := filepath.Join(temp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// The hermetic PATH set below has no host directories, so git must be
	// symlinked in explicitly - the same pattern TestInitNoAgentEmptyProject
	// already proves.
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	if err := os.Symlink(gitPath, filepath.Join(binDir, "git")); err != nil {
		t.Fatal(err)
	}

	// Mock claude CLI binary
	claudePath := filepath.Join(binDir, "claude")
	claudeScript := "#!/usr/bin/sh\necho '2.1.220 (Claude Code)'\n"
	if err := os.WriteFile(claudePath, []byte(claudeScript), 0o755); err != nil {
		t.Fatal(err)
	}

	// Mock credentials
	homeDir := filepath.Join(temp, "home")
	credDir := filepath.Join(homeDir, ".config", "sworn", ".claude")
	if err := os.MkdirAll(credDir, 0o755); err != nil {
		t.Fatal(err)
	}
	credFile := filepath.Join(credDir, ".credentials.json")
	if err := os.WriteFile(credFile, []byte(`{"token":"mock"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	// PATH carries only the mocked agent binaries and git, with no host
	// directories appended, so a real agent on the operator's own PATH
	// (sworn#228) cannot flip these fixtures.
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))
	t.Setenv(gitx.EnvCredentialsDir, "")

	return binDir, homeDir
}

// A1: In an empty project, init interactively confirms each artifact before writing
// (driver config from agent detection, operator config with the local listen default, the .sworn surface)
// and ends by naming the next command: pinned by cmd/sworn fixtures driving the prompt sequence over a scripted stdin.
func TestInitA1EmptyProjectInteractiveWalk(t *testing.T) {
	setupMockAgentAndEnvironment(t)
	root := initTestProject(t)

	stdin := strings.NewReader("y\ny\ny\n")
	var stdout, stderr bytes.Buffer
	code := runInitWithIO([]string{"--project", root}, stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runInitWithIO exit code = %d, want 0; stderr:\n%s", code, stderr.String())
	}

	out := stdout.String()
	// Confirms each artifact
	if !strings.Contains(out, "created "+filepath.Join(root, ".sworn")+"/") {
		t.Fatalf("surface directory not confirmed/created in stdout: %s", out)
	}
	if !strings.Contains(out, "created "+filepath.Join(root, ".sworn", "runs")+"/") {
		t.Fatalf("runs directory not confirmed/created in stdout: %s", out)
	}
	if !strings.Contains(out, "created "+filepath.Join(root, ".sworn", ".gitignore")) {
		t.Fatalf(".gitignore not confirmed/created in stdout: %s", out)
	}
	if !strings.Contains(out, "wrote "+filepath.Join(root, ".sworn", "drivers.json")) {
		t.Fatalf("driver config not confirmed/written in stdout: %s", out)
	}
	if !strings.Contains(out, "wrote "+filepath.Join(root, ".sworn", "operator.json")) {
		t.Fatalf("operator config not confirmed/written in stdout: %s", out)
	}
	// Ends by naming next command
	if !strings.Contains(out, "sworn driver doctor") {
		t.Fatalf("next command 'sworn driver doctor' not found in stdout: %s", out)
	}

	// Verify file permissions and content on disk
	ignBody, err := os.ReadFile(filepath.Join(root, ".sworn", ".gitignore"))
	if err != nil || string(ignBody) != canonicalGitignore {
		t.Fatalf(".gitignore content mismatch: %q, err=%v", ignBody, err)
	}

	driversInfo, err := os.Stat(filepath.Join(root, ".sworn", "drivers.json"))
	if err != nil || driversInfo.Mode().Perm() != 0o600 {
		t.Fatalf("drivers.json stat/perm error: info=%v, err=%v", driversInfo, err)
	}

	opInfo, err := os.Stat(filepath.Join(root, ".sworn", "operator.json"))
	if err != nil || opInfo.Mode().Perm() != 0o600 {
		t.Fatalf("operator.json stat/perm error: info=%v, err=%v", opInfo, err)
	}
	// The interactive walk's stdin holds no telemetry answers, so both
	// guided questions decline - and declines persist (sworn#269): the
	// private channel as "otel": null, share as an explicit enabled:false.
	opBody, err := os.ReadFile(filepath.Join(root, ".sworn", "operator.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(opBody, []byte("\"otel\": null")) {
		t.Fatalf("declined private telemetry not persisted as otel null:\n%s", opBody)
	}
	if !validOperatorFileInfo(opInfo) {
		t.Fatalf("validOperatorFileInfo failed on created operator.json")
	}
	settings, err := parseOperatorConfig(opBody)
	if err != nil {
		t.Fatalf("parseOperatorConfig failed on created operator.json: %v", err)
	}
	if settings.otel != nil {
		t.Fatalf("declined private telemetry parsed as configured: %#v", settings.otel)
	}
	if settings.share == nil || settings.share.Enabled {
		t.Fatalf("declined share not persisted as disabled: %#v", settings.share)
	}
}

// A2: Re-running init in a configured project reports what exists and what would change,
// and changes nothing without confirmation: pinned by an idempotence fixture asserting
// byte-identical files after a default-accepting re-run.
func TestInitA2IdempotenceAndDivergence(t *testing.T) {
	setupMockAgentAndEnvironment(t)
	root := initTestProject(t)

	// Step 1: Initial run with --yes to set up the project.
	var stdout1, stderr1 bytes.Buffer
	if code := runInit([]string{"--project", root, "--yes"}, &stdout1, &stderr1); code != 0 {
		t.Fatalf("initial run failed: %d, stderr=%s", code, stderr1.String())
	}

	files := []string{
		filepath.Join(root, ".sworn", ".gitignore"),
		filepath.Join(root, ".sworn", "drivers.json"),
		filepath.Join(root, ".sworn", "operator.json"),
	}
	hashesBefore := make(map[string]string)
	for _, f := range files {
		h, err := sha256Hex(f)
		if err != nil {
			t.Fatal(err)
		}
		hashesBefore[f] = h
	}

	// Step 2: Re-run with default-accepting input (empty line / EOF).
	var stdout2, stderr2 bytes.Buffer
	stdin2 := strings.NewReader("\n\n\n")
	if code := runInitWithIO([]string{"--project", root}, stdin2, &stdout2, &stderr2); code != 0 {
		t.Fatalf("re-run failed: %d, stderr=%s", code, stderr2.String())
	}

	out2 := stdout2.String()
	if !strings.Contains(out2, "Project surface already current") {
		t.Fatalf("re-run did not report surface current: %s", out2)
	}
	if !strings.Contains(out2, "AI connection file already current") {
		t.Fatalf("re-run did not report drivers.json current: %s", out2)
	}
	// The --yes scaffold never asked the telemetry questions, so the guided
	// re-run asks them; declining proposes persisting the declines
	// (sworn#269), and the confirm-guarded write defaults to keeping the
	// existing file untouched.
	if !strings.Contains(out2, "kept existing "+filepath.Join(root, ".sworn", "operator.json")) {
		t.Fatalf("re-run did not keep the operator config on declined replacement: %s", out2)
	}

	// Verify byte-identical files
	for _, f := range files {
		h, err := sha256Hex(f)
		if err != nil {
			t.Fatal(err)
		}
		if h != hashesBefore[f] {
			t.Fatalf("file %s was modified on re-run: before=%s after=%s", f, hashesBefore[f], h)
		}
	}

	// Step 3: Divergent files test. Modify drivers.json and operator.json.
	customDriver := []byte(`{"schema_version":"kept-custom"}`)
	if err := os.WriteFile(filepath.Join(root, ".sworn", "drivers.json"), customDriver, 0o600); err != nil {
		t.Fatal(err)
	}
	customOperator := []byte("{\n  \"schema_version\": \"sworn.operator-config/v1\",\n  \"local\": {\n    \"listen\": \"127.0.0.1:8888\"\n  }\n}\n")
	if err := os.WriteFile(filepath.Join(root, ".sworn", "operator.json"), customOperator, 0o600); err != nil {
		t.Fatal(err)
	}

	// Default-accepting re-run on divergent files: drivers.json defaults to
	// No [y/N], while the operator config runs the guided telemetry step
	// (S3-A5): the custom file never answered the telemetry questions, so
	// they are asked, the declines propose a persistence write (sworn#269) -
	// the proposal keeps every operator-authored field, never the bare
	// default (captain F6) - and the confirm-guarded write defaults to
	// keeping the file untouched.
	var stdout3, stderr3 bytes.Buffer
	stdin3 := strings.NewReader("\n\n")
	if code := runInitWithIO([]string{"--project", root}, stdin3, &stdout3, &stderr3); code != 0 {
		t.Fatalf("divergent re-run exit code = %d, want 0", code)
	}
	out3 := stdout3.String()
	if !strings.Contains(out3, "differs from proposed configuration") {
		t.Fatalf("diff summary not rendered for divergent config: %s", out3)
	}
	if !strings.Contains(out3, "kept existing "+filepath.Join(root, ".sworn", "drivers.json")) {
		t.Fatalf("drivers.json was not reported kept: %s", out3)
	}
	if !strings.Contains(out3, "kept existing "+filepath.Join(root, ".sworn", "operator.json")) {
		t.Fatalf("operator.json was not reported kept: %s", out3)
	}
	// The operator's own listen value must survive into the proposed body -
	// the diff may add telemetry state but never revert operator edits, so
	// the proposed (+) side must carry 8888 and never the bare default 7337.
	if !strings.Contains(out3, "+     \"listen\": \"127.0.0.1:8888\"") ||
		strings.Contains(out3, "+     \"listen\": \"127.0.0.1:7337\"") {
		t.Fatalf("proposed operator body did not keep the operator's listen value: %s", out3)
	}

	// Files must remain untouched
	currentDriver, _ := os.ReadFile(filepath.Join(root, ".sworn", "drivers.json"))
	if !bytes.Equal(currentDriver, customDriver) {
		t.Fatalf("divergent drivers.json was modified: %q", currentDriver)
	}
	currentOperator, _ := os.ReadFile(filepath.Join(root, ".sworn", "operator.json"))
	if !bytes.Equal(currentOperator, customOperator) {
		t.Fatalf("divergent operator.json was modified: %q", currentOperator)
	}

	// Step 4: Explicit replacement with --force replaces divergent files
	var stdout4, stderr4 bytes.Buffer
	if code := runInit([]string{"--project", root, "--force"}, &stdout4, &stderr4); code != 0 {
		t.Fatalf("--force run failed: %d, stderr=%s", code, stderr4.String())
	}
	out4 := stdout4.String()
	if !strings.Contains(out4, "replaced "+filepath.Join(root, ".sworn", "drivers.json")) {
		t.Fatalf("drivers.json was not reported replaced: %s", out4)
	}
	if !strings.Contains(out4, "replaced "+filepath.Join(root, ".sworn", "operator.json")) {
		t.Fatalf("operator.json was not reported replaced: %s", out4)
	}
}

// A3: --yes answers every prompt with its default and stays scriptable in CI:
// pinned by a non-interactive fixture producing the same artifacts as the interactive defaults path.
func TestInitA3YesFlagScriptability(t *testing.T) {
	setupMockAgentAndEnvironment(t)
	rootInteractive := initTestProject(t)
	rootYes := initTestProject(t)

	// Interactive defaults
	var stdoutI, stderrI bytes.Buffer
	stdinI := strings.NewReader("\n\n\n")
	if code := runInitWithIO([]string{"--project", rootInteractive}, stdinI, &stdoutI, &stderrI); code != 0 {
		t.Fatalf("interactive init failed: %d, stderr=%s", code, stderrI.String())
	}

	// Non-interactive --yes
	var stdoutY, stderrY bytes.Buffer
	if code := runInit([]string{"--project", rootYes, "--yes"}, &stdoutY, &stderrY); code != 0 {
		t.Fatalf("non-interactive --yes init failed: %d, stderr=%s", code, stderrY.String())
	}

	// Surface and driver artifacts are identical between the two paths.
	for _, rel := range []string{
		filepath.Join(".sworn", ".gitignore"),
		filepath.Join(".sworn", "drivers.json"),
	} {
		bodyI, err := os.ReadFile(filepath.Join(rootInteractive, rel))
		if err != nil {
			t.Fatal(err)
		}
		bodyY, err := os.ReadFile(filepath.Join(rootYes, rel))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(bodyI, bodyY) {
			t.Fatalf("artifact %s mismatch between interactive default and --yes:\ninteractive:\n%s\n--yes:\n%s", rel, bodyI, bodyY)
		}
	}
	// The operator config diverges by design: --yes never asks the
	// telemetry questions and writes the bare default (C4), while the
	// interactive walk asks them and persists the default declines
	// (sworn#269).
	bodyY, err := os.ReadFile(filepath.Join(rootYes, ".sworn", "operator.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bodyY, buildDefaultOperatorConfig()) {
		t.Fatalf("--yes operator config differs from the bare default:\n%s", bodyY)
	}
	bodyI, err := os.ReadFile(filepath.Join(rootInteractive, ".sworn", "operator.json"))
	if err != nil {
		t.Fatal(err)
	}
	settings, err := parseOperatorConfig(bodyI)
	if err != nil {
		t.Fatalf("interactive operator config did not parse: %v\n%s", err, bodyI)
	}
	if settings.otel != nil || settings.share == nil || settings.share.Enabled {
		t.Fatalf("interactive default declines were not persisted: %s", bodyI)
	}
}

// A4: init writes .sworn/operator.json with the conventional shape when absent,
// and never touches an existing one without confirmation: pinned by fixtures over absent,
// present-identical, and present-divergent cases.
func TestInitA4OperatorConfigScaffolding(t *testing.T) {
	setupMockAgentAndEnvironment(t)
	root := initTestProject(t)
	opPath := filepath.Join(root, ".sworn", "operator.json")

	// Absent case
	var stdout1, stderr1 bytes.Buffer
	stdin1 := strings.NewReader("y\ny\ny\n")
	if code := runInitWithIO([]string{"--project", root}, stdin1, &stdout1, &stderr1); code != 0 {
		t.Fatalf("absent init failed: %d, stderr=%s", code, stderr1.String())
	}
	if !strings.Contains(stdout1.String(), "wrote "+opPath) {
		t.Fatalf("did not report wrote operator.json: %s", stdout1.String())
	}
	// The scripted stdin declines both telemetry questions; the declines
	// persist (sworn#269) so the later "present-identical" re-run below has
	// nothing left to ask.
	body, err := os.ReadFile(opPath)
	if err != nil {
		t.Fatal(err)
	}
	scaffolded, err := parseOperatorConfig(body)
	if err != nil {
		t.Fatalf("scaffolded operator.json did not parse: %v\n%s", err, body)
	}
	if scaffolded.otel != nil || scaffolded.share == nil || scaffolded.share.Enabled {
		t.Fatalf("scaffolded declines were not persisted: %s", body)
	}
	info, err := os.Stat(opPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("operator.json mode is not 0600: %v", info)
	}

	// Present-identical case
	var stdout2, stderr2 bytes.Buffer
	stdin2 := strings.NewReader("")
	if code := runInitWithIO([]string{"--project", root}, stdin2, &stdout2, &stderr2); code != 0 {
		t.Fatalf("present-identical init failed: %d, stderr=%s", code, stderr2.String())
	}
	if !strings.Contains(stdout2.String(), "Operator configuration already current: "+opPath) {
		t.Fatalf("did not report operator.json already current: %s", stdout2.String())
	}

	// Present-divergent case 1: Content divergence. The custom file never
	// answered the telemetry questions, so the guided step asks them; the
	// declines propose a persistence write (sworn#269), and declining the
	// confirm-guarded replacement keeps the operator's file byte-untouched.
	customContent := []byte("{\n  \"schema_version\": \"sworn.operator-config/v1\",\n  \"local\": {\n    \"listen\": \"127.0.0.1:9090\"\n  }\n}\n")
	if err := os.WriteFile(opPath, customContent, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout3, stderr3 bytes.Buffer
	stdin3 := strings.NewReader("\n\n") // Decline endpoint, decline share; EOF declines the replacement
	if code := runInitWithIO([]string{"--project", root}, stdin3, &stdout3, &stderr3); code != 0 {
		t.Fatalf("divergent decline init failed: %d, stderr=%s", code, stderr3.String())
	}
	if !strings.Contains(stdout3.String(), "kept existing "+opPath) {
		t.Fatalf("did not report operator.json kept: %s", stdout3.String())
	}
	currentBody, _ := os.ReadFile(opPath)
	if !bytes.Equal(currentBody, customContent) {
		t.Fatalf("operator.json was modified despite declining: %s", currentBody)
	}

	// Present-divergent case 2: Non-0600 mode with identical content (Captain Correction 5)
	if err := os.WriteFile(opPath, buildDefaultOperatorConfig(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(opPath, 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout4, stderr4 bytes.Buffer
	stdin4 := strings.NewReader("\n\nn\n") // Decline endpoint, decline share, decline replacement
	if code := runInitWithIO([]string{"--project", root}, stdin4, &stdout4, &stderr4); code != 0 {
		t.Fatalf("mode-divergent decline failed: %d, stderr=%s", code, stderr4.String())
	}
	out4 := stdout4.String()
	if strings.Contains(out4, "Operator configuration already current") {
		t.Fatalf("mode 0644 was incorrectly reported as 'already current': %s", out4)
	}
	if !strings.Contains(out4, "differs from proposed configuration") || !strings.Contains(out4, "mode: 0644") {
		t.Fatalf("diff summary did not report mode 0644 divergence: %s", out4)
	}

	// Replacing mode-divergent file with 'y' resets mode to 0600. The two
	// guided telemetry questions come first and are declined with empty
	// answers, then the replacement prompt accepts.
	var stdout5, stderr5 bytes.Buffer
	stdin5 := strings.NewReader("\n\ny\n")
	if code := runInitWithIO([]string{"--project", root}, stdin5, &stdout5, &stderr5); code != 0 {
		t.Fatalf("mode-divergent replace failed: %d, stderr=%s", code, stderr5.String())
	}
	info5, err := os.Stat(opPath)
	if err != nil || info5.Mode().Perm() != 0o600 {
		t.Fatalf("replaced operator.json mode is not 0600: %v", info5)
	}
}

// Captain Correction 4: Handle no-agent empty project so the walk completes
// with driver config reported missing, operator config and surface created,
// and exit code 1.
func TestInitNoAgentEmptyProject(t *testing.T) {
	temp := t.TempDir()
	binDir := filepath.Join(temp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	if err := os.Symlink(gitPath, filepath.Join(binDir, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir) // PATH has only git, no codex or claude

	root := initTestProject(t)

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("y\ny\n")
	code := runInitWithIO([]string{"--project", root}, stdin, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("runInitWithIO exit code = %d, want 1 for missing driver config", code)
	}

	out := stdout.String()
	if !strings.Contains(out, "AI driver configuration: missing") {
		t.Fatalf("stdout did not report driver config missing: %s", out)
	}
	if !strings.Contains(out, "wrote "+filepath.Join(root, ".sworn", "operator.json")) {
		t.Fatalf("operator.json was not written in no-agent walk: %s", out)
	}
	if !strings.Contains(out, "created "+filepath.Join(root, ".sworn", ".gitignore")) {
		t.Fatalf(".gitignore was not created in no-agent walk: %s", out)
	}
	if !strings.Contains(out, "Sworn cannot start a run until an AI connection file exists at") {
		t.Fatalf("closing advice missing: %s", out)
	}
}

// S6-A1: on a non-Linux host, init states plainly that native dispatch
// requires Linux instead of tripping the Linux-only runtime-file preflight -
// even with no agent on PATH at all, since the statement must reach every
// non-Linux run regardless of agent presence (Captain Correction 1).
func TestInitDarwinStatesNativeDispatchRequiresLinux(t *testing.T) {
	oldHostOS := initHostOS
	initHostOS = "darwin"
	t.Cleanup(func() { initHostOS = oldHostOS })

	root := initTestProject(t)

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("y\ny\n")
	code := runInitWithIO([]string{"--project", root}, stdin, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("runInitWithIO exit code = %d, want 1 for a missing AI connection file on darwin", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "Native dispatch requires Linux") {
		t.Fatalf("stdout did not state the Linux requirement: %s", out)
	}
	// The old Linux-only runtime-file preflight message must never fire:
	// the gate gives every non-Linux host the plain statement instead
	// (Captain Correction 2 - a substring the mocked runtime targets in
	// setupMockAgentAndEnvironment cannot make vacuous, unlike "/etc/").
	if strings.Contains(out, "which the sandboxed agent needs") {
		t.Fatalf("darwin run tripped the Linux-only runtime-file preflight: %s", out)
	}
	if _, err := os.Stat(filepath.Join(root, ".sworn", "drivers.json")); !os.IsNotExist(err) {
		t.Fatalf("drivers.json was written on darwin: %v", err)
	}
}

// S6-A1: an existing connection file on a non-Linux host is kept and reported
// present, exactly as on Linux today.
func TestInitDarwinKeepsExistingConnectionFile(t *testing.T) {
	oldHostOS := initHostOS
	initHostOS = "darwin"
	t.Cleanup(func() { initHostOS = oldHostOS })

	root := initTestProject(t)
	home := filepath.Join(root, ".sworn")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "drivers.json")
	original := []byte(`{"schema_version":"kept"}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("y\ny\n")
	code := runInitWithIO([]string{"--project", root}, stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runInitWithIO exit code = %d, want 0 with an existing connection file; stderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "AI connection file present: "+path) {
		t.Fatalf("stdout did not report the existing connection file: %s", stdout.String())
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, original) {
		t.Fatalf("existing connection file was modified: %q", current)
	}
}

// S6-A2, Captain Correction 3: with a mock codex minted alongside the
// existing mock claude, the codex-first detection order is exercised
// directly by codex actually winning, not merely by codex's absence from
// PATH.
func TestInitDetectsCodexFirstOverAPresentClaude(t *testing.T) {
	binDir, homeDir := setupMockAgentAndEnvironment(t)
	root := initTestProject(t)

	codexPath := filepath.Join(binDir, "codex")
	if err := os.WriteFile(codexPath, []byte("#!/usr/bin/sh\necho 'codex-cli "+driver.CodexCLIVersion+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	codexCredDir := filepath.Join(homeDir, ".config", "sworn", ".codex")
	if err := os.MkdirAll(codexCredDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexCredDir, "auth.json"), []byte(`{"token":"mock"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := runInit([]string{"--project", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init failed: %d, stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "wrote "+filepath.Join(root, ".sworn", "drivers.json")+" (Codex ") {
		t.Fatalf("codex was not selected ahead of the present claude: %s", stdout.String())
	}
}

// Reconciled for sworn#265 (supersedes S6-A2 Captain Correction 3's
// no-fallback ruling): an installed agent with no readable credential no
// longer walls a signed-in later agent. The fallthrough is DISCLOSED, never
// silent - the skipped agent and the reason are reported before the write -
// which is what the original correction existed to protect.
func TestInitFallsThroughToSignedInClaudeWhenCodexIsSignedOut(t *testing.T) {
	binDir, _ := setupMockAgentAndEnvironment(t)
	root := initTestProject(t)

	codexPath := filepath.Join(binDir, "codex")
	if err := os.WriteFile(codexPath, []byte("#!/usr/bin/sh\necho 'codex-cli "+driver.CodexCLIVersion+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// No codex credential file is minted; claude's is (fixture default).

	var stdout, stderr bytes.Buffer
	if code := runInit([]string{"--project", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exit code = %d, want 0 via claude fallthrough; stdout=%s", code, stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "skipped Codex: not signed in") {
		t.Fatalf("the codex skip was not disclosed: %s", out)
	}
	if !strings.Contains(out, "wrote "+filepath.Join(root, ".sworn", "drivers.json")+" (Claude Code ") {
		t.Fatalf("claude was not selected after the codex skip: %s", out)
	}
}

// sworn#266: when the credential is missing at Sworn's credentials base but
// present at the agent's own standard home location, the skip reason names
// the remedy that actually works - SWORN_CREDENTIALS_DIR - instead of
// telling the operator to sign in again into a file the agent never writes.
func TestInitSkipReasonNamesCredentialsDirWhenStandardLocationIsSignedIn(t *testing.T) {
	binDir, homeDir := setupMockAgentAndEnvironment(t)
	root := initTestProject(t)

	codexPath := filepath.Join(binDir, "codex")
	if err := os.WriteFile(codexPath, []byte("#!/usr/bin/sh\necho 'codex-cli "+driver.CodexCLIVersion+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Codex is signed in at its STANDARD location ($HOME/.codex/auth.json),
	// not under the sworn credentials base the fixture points init at.
	standardDir := filepath.Join(homeDir, ".codex")
	if err := os.MkdirAll(standardDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(standardDir, "auth.json"), []byte(`{"token":"standard"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := runInit([]string{"--project", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exit code = %d, want 0 via claude fallthrough; stdout=%s", code, stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "skipped Codex: signed in at "+filepath.Join(standardDir, "auth.json")) ||
		!strings.Contains(out, "SWORN_CREDENTIALS_DIR="+homeDir) {
		t.Fatalf("the skip reason did not name the working remedy: %s", out)
	}
}

// The project directory holds absolute host paths, binary digests, and the run
// journal. None of it belongs in a repository other people clone, so init must
// exclude the whole directory except the records root, whose committed
// authority a fresh clone must carry.
func TestInitExcludesTheProjectDirectoryFromGit(t *testing.T) {
	root := initTestProject(t)
	var stdout, stderr bytes.Buffer
	runInit([]string{"--project", root}, &stdout, &stderr)

	body, err := os.ReadFile(filepath.Join(root, ".sworn", ".gitignore"))
	if err != nil {
		t.Fatalf("no .gitignore written: %v", err)
	}
	if strings.TrimSpace(string(body)) != "*\n!records/\n!records/**" {
		t.Fatalf("gitignore = %q, want the allowlist shape (records tracked, everything else excluded)", body)
	}
	for _, directory := range []string{".sworn", ".sworn/runs"} {
		info, err := os.Stat(filepath.Join(root, directory))
		if err != nil || !info.IsDir() {
			t.Fatalf("%s was not created as a directory: %v", directory, err)
		}
	}
}

// Reconciled for Captain Correction 1:
// Run definitions record the exact fingerprint of the connection file, so
// silently rewriting it would invalidate them. Re-running reports what exists
// and what would change, and defaults to keeping the existing file untouched.
func TestInitRefusesToReplaceAnExistingConnectionFile(t *testing.T) {
	setupMockAgentAndEnvironment(t)
	root := initTestProject(t)
	home := filepath.Join(root, ".sworn")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "drivers.json")
	original := []byte(`{"schema_version":"kept"}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("n\n") // Decline replacement
	code := runInitWithIO([]string{"--project", root}, stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init re-run exit code = %d, want 0; stderr:\n%s", code, stderr.String())
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, original) {
		t.Fatalf("existing connection file was modified: %q", current)
	}
	if !strings.Contains(stdout.String(), "kept existing "+path) {
		t.Fatalf("output did not report keeping existing file: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "differs from proposed configuration") {
		t.Fatalf("output did not report diff: %s", stdout.String())
	}
	// The closing advice must not claim the file is missing when it is present.
	if strings.Contains(stdout.String(), "until an AI connection file exists") {
		t.Fatalf("refusal contradicts itself: %s", stdout.String())
	}
}

// init is the first command run in a project, so it must work from wherever the
// operator happens to be standing.
func TestInitResolvesTheProjectFromASubdirectory(t *testing.T) {
	root := initTestProject(t)
	nested := filepath.Join(root, "deep", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(working)
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	runInit(nil, &stdout, &stderr)
	if !strings.Contains(stdout.String(), "Project: "+root) {
		t.Fatalf("did not resolve the project root: %s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".sworn", "runs")); err != nil {
		t.Fatalf("project was not prepared at the root: %v", err)
	}
}

func TestInitRefusesOutsideAGitProject(t *testing.T) {
	outside := t.TempDir()
	resolved, err := filepath.EvalSymlinks(outside)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runInit([]string{"--project", resolved}, &stdout, &stderr); code == 0 {
		t.Fatal("init reported success outside a Git project")
	}
	if _, err := os.Stat(filepath.Join(resolved, ".sworn")); !os.IsNotExist(err) {
		t.Fatal("init created a project directory outside a Git project")
	}
}

func TestInitRejectsRelativeProjectPaths(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runInit([]string{"--project", "relative/path"}, &stdout, &stderr); code == 0 {
		t.Fatal("init accepted a relative project path")
	}
}

// The launcher a package manager puts on PATH may be a script that executes a
// platform build elsewhere. Sworn must record the binary it will actually run.
func TestResolveCodexBinaryFindsThePlatformBuild(t *testing.T) {
	root := t.TempDir()
	launcher := filepath.Join(root, "bin", "codex.js")
	platform := filepath.Join(
		root, "node_modules", "@openai",
		"codex-linux-x64", "vendor", "x86_64-unknown-linux-musl", "bin",
	)
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(platform, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcher, []byte("#!/usr/bin/env node\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(platform, "codex")
	if err := os.WriteFile(binary, []byte("ELF"), 0o755); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveCodexBinary(launcher)
	if err != nil {
		t.Fatalf("platform build not found: %v", err)
	}
	if resolved != binary {
		t.Fatalf("resolved = %q, want %q", resolved, binary)
	}
}

func TestAgentReportedVersionReadsEachFamilyFormat(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		family driver.ProfileFamily
		output string
		want   string
		ok     bool
	}{
		{"codex", driver.ProfileCodex, "codex-cli 0.146.0\n", "0.146.0", true},
		{"claude", driver.ProfileClaude, "2.1.220 (Claude Code)\n", "2.1.220", true},
		{"codex unexpected", driver.ProfileCodex, "something else\n", "", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			value, ok := agentReportedVersion(testCase.family, testCase.output)
			if ok != testCase.ok || value != testCase.want {
				t.Fatalf("got (%q, %v), want (%q, %v)",
					value, ok, testCase.want, testCase.ok)
			}
		})
	}
}

func sha256Hex(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

// S3-A5: the interactive guided telemetry step writes the private otel block
// and the share opt-in block when answered, keeps mode 0600, and re-runs
// idempotently to "already current" with byte-identical content.
func TestInitTelemetryStepGuidedWalkWritesAndIsIdempotent(t *testing.T) {
	setupMockAgentAndEnvironment(t)
	root := initTestProject(t)
	opPath := filepath.Join(root, ".sworn", "operator.json")

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("y\ny\ny\nhttp://127.0.0.1:4318\ny\n")
	if code := runInitWithIO([]string{"--project", root}, stdin, &stdout, &stderr); code != 0 {
		t.Fatalf("guided init failed: %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "wrote "+opPath) {
		t.Fatalf("did not report wrote operator.json: %s", stdout.String())
	}
	info, err := os.Stat(opPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("operator.json stat/perm error: info=%v, err=%v", info, err)
	}
	body, err := os.ReadFile(opPath)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := parseOperatorConfig(body)
	if err != nil {
		t.Fatalf("guided operator config did not parse: %v\n%s", err, body)
	}
	if settings.otel == nil ||
		!strings.HasPrefix(settings.otel.Endpoint, "http://127.0.0.1:4318") {
		t.Fatalf("private telemetry block missing or wrong: %#v", settings.otel)
	}
	if settings.share == nil || !settings.share.Enabled ||
		settings.share.Endpoint != observe.ShareDefaultEndpoint {
		t.Fatalf("share block missing or wrong: %#v", settings.share)
	}

	// Idempotent re-run: no questions remain and the file is untouched.
	var stdout2, stderr2 bytes.Buffer
	if code := runInitWithIO([]string{"--project", root}, strings.NewReader(""), &stdout2, &stderr2); code != 0 {
		t.Fatalf("guided re-run failed: %d, stderr=%s", code, stderr2.String())
	}
	if !strings.Contains(stdout2.String(), "Operator configuration already current: "+opPath) {
		t.Fatalf("re-run did not report operator.json current: %s", stdout2.String())
	}
	after, err := os.ReadFile(opPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, body) {
		t.Fatalf("operator.json changed on idempotent re-run:\nbefore:\n%s\nafter:\n%s", body, after)
	}
}

// sworn#269 (supersedes the S3-A5 "decline keeps bare default" pin):
// declining both telemetry questions persists the declines - "otel": null
// and share enabled:false - so a guided re-run has nothing left to ask and
// the settled answers stay settled.
func TestInitTelemetryDeclinePersistsAndIsNotReAsked(t *testing.T) {
	setupMockAgentAndEnvironment(t)
	root := initTestProject(t)
	opPath := filepath.Join(root, ".sworn", "operator.json")

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("y\ny\ny\n\nn\n")
	if code := runInitWithIO([]string{"--project", root}, stdin, &stdout, &stderr); code != 0 {
		t.Fatalf("declined init failed: %d, stderr=%s", code, stderr.String())
	}
	body, err := os.ReadFile(opPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("\"otel\": null")) {
		t.Fatalf("declined private telemetry not persisted:\n%s", body)
	}
	settings, err := parseOperatorConfig(body)
	if err != nil {
		t.Fatalf("declined operator config did not parse: %v\n%s", err, body)
	}
	if settings.otel != nil || settings.share == nil || settings.share.Enabled {
		t.Fatalf("declines parsed as configured telemetry: %s", body)
	}

	// The guided re-run asks nothing and leaves the file byte-identical.
	var stdout2, stderr2 bytes.Buffer
	if code := runInitWithIO([]string{"--project", root}, strings.NewReader(""), &stdout2, &stderr2); code != 0 {
		t.Fatalf("guided re-run failed: %d, stderr=%s", code, stderr2.String())
	}
	out2 := stdout2.String()
	if strings.Contains(out2, "Private telemetry OTLP endpoint") ||
		strings.Contains(out2, "Share fleet telemetry") {
		t.Fatalf("a settled telemetry answer was re-asked: %s", out2)
	}
	if !strings.Contains(out2, "Operator configuration already current: "+opPath) {
		t.Fatalf("re-run did not report operator.json current: %s", out2)
	}
	after, err := os.ReadFile(opPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, body) {
		t.Fatalf("operator.json changed on quiet re-run:\nbefore:\n%s\nafter:\n%s", body, after)
	}
}

// S3-A5: a non-empty private endpoint must parse through the strict OTLP
// endpoint rules or it is reported and skipped, never written. A skip is
// not a decline (sworn#269): the mistyped question is asked again on the
// next guided run, while the answered share question is not.
func TestInitTelemetryStepRejectsInvalidEndpoint(t *testing.T) {
	setupMockAgentAndEnvironment(t)
	root := initTestProject(t)
	opPath := filepath.Join(root, ".sworn", "operator.json")

	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("y\ny\ny\nnot-an-endpoint\nn\n")
	if code := runInitWithIO([]string{"--project", root}, stdin, &stdout, &stderr); code != 0 {
		t.Fatalf("invalid-endpoint init failed: %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "skipped private telemetry") {
		t.Fatalf("invalid endpoint was not reported skipped: %s", stdout.String())
	}
	body, err := os.ReadFile(opPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("\"otel\"")) {
		t.Fatalf("invalid endpoint leaked an otel key:\n%s", body)
	}
	settings, err := parseOperatorConfig(body)
	if err != nil {
		t.Fatalf("operator config did not parse: %v\n%s", err, body)
	}
	if settings.otel != nil || settings.share == nil || settings.share.Enabled {
		t.Fatalf("unexpected telemetry state after skipped endpoint: %s", body)
	}

	// The next guided run re-asks only the unanswered endpoint question.
	var stdout2, stderr2 bytes.Buffer
	if code := runInitWithIO([]string{"--project", root}, strings.NewReader("\n\n"), &stdout2, &stderr2); code != 0 {
		t.Fatalf("guided re-run failed: %d, stderr=%s", code, stderr2.String())
	}
	out2 := stdout2.String()
	if !strings.Contains(out2, "Private telemetry OTLP endpoint") {
		t.Fatalf("the skipped endpoint question was not re-asked: %s", out2)
	}
	if strings.Contains(out2, "Share fleet telemetry") {
		t.Fatalf("the answered share question was re-asked: %s", out2)
	}
}

// S3-C4: the share opt-in (and the private-endpoint entry) is
// interactive-only. Under --yes and under --force no telemetry question is
// asked and the written body is byte-identical to today's
// buildDefaultOperatorConfig - even when the stdin would answer yes.
func TestInitTelemetryFlagsNeverAskOrOptIn(t *testing.T) {
	for _, flag := range []string{"--yes", "--force"} {
		t.Run(flag, func(t *testing.T) {
			setupMockAgentAndEnvironment(t)
			root := initTestProject(t)
			opPath := filepath.Join(root, ".sworn", "operator.json")

			var stdout, stderr bytes.Buffer
			stdin := strings.NewReader("y\ny\ny\nhttp://127.0.0.1:4318\ny\n")
			if code := runInitWithIO([]string{"--project", root, flag}, stdin, &stdout, &stderr); code != 0 {
				t.Fatalf("flag init failed: %d, stderr=%s", code, stderr.String())
			}
			if strings.Contains(stdout.String(), "Private telemetry OTLP endpoint") ||
				strings.Contains(stdout.String(), "Share fleet telemetry") {
				t.Fatalf("%s asked a telemetry question: %s", flag, stdout.String())
			}
			body, err := os.ReadFile(opPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(body, buildDefaultOperatorConfig()) {
				t.Fatalf("%s operator config differs from bare default:\n%s", flag, body)
			}
		})
	}
}

// sworn#267: a CLI that outran the certified pin but shares its major.minor
// is scaffolded with pin_mode "minor" and the delta disclosed, because init
// round-trips the proposed adapter through the engine's own admission. The
// fixture CLI reports a patch level the exact pin rejects.
func TestInitPinsByMinorWhenCLIOutranTheCertifiedPin(t *testing.T) {
	setupMockAgentAndEnvironment(t)
	root := initTestProject(t)

	var stdout, stderr bytes.Buffer
	if code := runInit([]string{"--project", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init failed: %d, stdout=%s", code, stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "pinned by major.minor (pin_mode \"minor\")") ||
		!strings.Contains(out, "2.1.220") ||
		!strings.Contains(out, driver.ClaudeCLIVersion) {
		t.Fatalf("the minor pin was not disclosed with both versions: %s", out)
	}
	body, err := os.ReadFile(filepath.Join(root, ".sworn", "drivers.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("\"pin_mode\":\"minor\"")) {
		t.Fatalf("scaffolded config does not carry the minor pin:\n%s", body)
	}
}

// sworn#267: a CLI outside the certified major.minor is refused at scaffold
// time, naming both versions - never written for the engine to reject two
// commands later.
func TestInitRefusesACLIOutsideTheCertifiedMinor(t *testing.T) {
	binDir, _ := setupMockAgentAndEnvironment(t)
	root := initTestProject(t)
	claudePath := filepath.Join(binDir, "claude")
	if err := os.WriteFile(claudePath, []byte("#!/usr/bin/sh\necho '3.0.0 (Claude Code)'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := runInit([]string{"--project", root, "--yes"}, &stdout, &stderr); code != 1 {
		t.Fatalf("init exit code = %d, want 1; stdout=%s", code, stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "3.0.0 does not match the version this Sworn build certifies ("+driver.ClaudeCLIVersion+")") {
		t.Fatalf("the refusal did not name both versions: %s", out)
	}
	if !strings.Contains(out, "AI driver configuration: unavailable") {
		t.Fatalf("the walk did not report the configuration unavailable: %s", out)
	}
	if _, err := os.Stat(filepath.Join(root, ".sworn", "drivers.json")); !os.IsNotExist(err) {
		t.Fatalf("an inadmissible config was written anyway: %v", err)
	}
}

// sworn#265, admission flavor: a signed-in agent the engine cannot admit
// (its version is outside the certified major.minor) is skipped with the
// named reason and the walk falls through to the next certifiable agent -
// an unusable earlier agent never walls a usable later one at ANY stage.
func TestInitFallsThroughWhenSignedInCodexIsNotCertifiable(t *testing.T) {
	binDir, homeDir := setupMockAgentAndEnvironment(t)
	root := initTestProject(t)

	codexPath := filepath.Join(binDir, "codex")
	if err := os.WriteFile(codexPath, []byte("#!/usr/bin/sh\necho 'codex-cli 1.2.3'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	codexCredDir := filepath.Join(homeDir, ".config", "sworn", ".codex")
	if err := os.MkdirAll(codexCredDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexCredDir, "auth.json"), []byte(`{"token":"mock"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := runInit([]string{"--project", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exit code = %d, want 0 via claude fallthrough; stdout=%s", code, stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "skipped Codex: Codex 1.2.3 does not match the version this Sworn build certifies ("+driver.CodexCLIVersion+")") {
		t.Fatalf("the admission skip was not disclosed with both versions: %s", out)
	}
	if !strings.Contains(out, "wrote "+filepath.Join(root, ".sworn", "drivers.json")+" (Claude Code ") {
		t.Fatalf("claude was not selected after the codex admission skip: %s", out)
	}
}

// sworn#268: the closing guidance prints a doctor invocation that works as
// pasted - config path, profile, certification model, and the mandatory
// --json - instead of a bare verb that exits on usage.
func TestInitNextStepPrintsAWorkingDoctorCommand(t *testing.T) {
	setupMockAgentAndEnvironment(t)
	root := initTestProject(t)

	var stdout, stderr bytes.Buffer
	if code := runInit([]string{"--project", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init failed: %d, stdout=%s", code, stdout.String())
	}
	want := "sworn driver doctor --config " +
		filepath.Join(root, ".sworn", "drivers.json") +
		" --profile claude --model claude-opus-4-6 --json"
	if !strings.Contains(stdout.String(), want) {
		t.Fatalf("next step does not print the working doctor command %q: %s", want, stdout.String())
	}
}
