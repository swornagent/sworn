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

	// Mock runtime targets
	var targets []string
	for i, name := range []string{"hosts", "nsswitch.conf", "resolv.conf", "ca-certificates.crt"} {
		path := filepath.Join(temp, "etc", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(fmt.Sprintf("mock target %d\n", i)), 0o644); err != nil {
			t.Fatal(err)
		}
		targets = append(targets, path)
	}
	oldTargets := initRuntimeTargets
	initRuntimeTargets = targets
	t.Cleanup(func() {
		initRuntimeTargets = oldTargets
	})

	// Mock claude CLI binary
	binDir := filepath.Join(temp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
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

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
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
	opBody, err := os.ReadFile(filepath.Join(root, ".sworn", "operator.json"))
	if err != nil || !bytes.Equal(opBody, buildDefaultOperatorConfig()) {
		t.Fatalf("operator.json content mismatch: %s", opBody)
	}
	if !validOperatorFileInfo(opInfo) {
		t.Fatalf("validOperatorFileInfo failed on created operator.json")
	}
	if _, err := parseOperatorConfig(opBody); err != nil {
		t.Fatalf("parseOperatorConfig failed on created operator.json: %v", err)
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
	if !strings.Contains(out2, "Operator configuration already current") {
		t.Fatalf("re-run did not report operator.json current: %s", out2)
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
	// No [y/N], while the operator config now runs the guided telemetry step
	// (S3-A5): both telemetry questions default to declining, so the
	// proposed body equals the existing bytes exactly and the operator's own
	// config is reported current instead of offered replacement with the
	// bare default (captain F6).
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
	if !strings.Contains(out3, "Operator configuration already current: "+filepath.Join(root, ".sworn", "operator.json")) {
		t.Fatalf("operator.json was not reported current: %s", out3)
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

	// Compare artifacts
	for _, rel := range []string{
		filepath.Join(".sworn", ".gitignore"),
		filepath.Join(".sworn", "drivers.json"),
		filepath.Join(".sworn", "operator.json"),
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
	body, err := os.ReadFile(opPath)
	if err != nil || !bytes.Equal(body, buildDefaultOperatorConfig()) {
		t.Fatalf("operator.json content incorrect: %s", body)
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

	// Present-divergent case 1: Content divergence. The guided telemetry
	// step declines both questions (S3-A5), so the proposed body equals the
	// existing bytes and the operator's own config is reported current
	// rather than offered replacement with the bare default.
	customContent := []byte("{\n  \"schema_version\": \"sworn.operator-config/v1\",\n  \"local\": {\n    \"listen\": \"127.0.0.1:9090\"\n  }\n}\n")
	if err := os.WriteFile(opPath, customContent, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout3, stderr3 bytes.Buffer
	stdin3 := strings.NewReader("\n\n") // Decline endpoint, decline share
	if code := runInitWithIO([]string{"--project", root}, stdin3, &stdout3, &stderr3); code != 0 {
		t.Fatalf("divergent decline init failed: %d, stderr=%s", code, stderr3.String())
	}
	if !strings.Contains(stdout3.String(), "Operator configuration already current: "+opPath) {
		t.Fatalf("did not report operator.json current: %s", stdout3.String())
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
	stdin4 := strings.NewReader("n\n") // Decline
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

// S3-A5: declining both telemetry questions leaves the exact bare scaffold
// byte-identical to today's buildDefaultOperatorConfig.
func TestInitTelemetryStepDeclineKeepsBareDefault(t *testing.T) {
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
	if !bytes.Equal(body, buildDefaultOperatorConfig()) {
		t.Fatalf("declined operator config differs from bare default:\n%s", body)
	}
	if bytes.Contains(body, []byte("otel")) || bytes.Contains(body, []byte("share")) {
		t.Fatalf("declined operator config carries telemetry blocks:\n%s", body)
	}
}

// S3-A5: a non-empty private endpoint must parse through the strict OTLP
// endpoint rules or it is reported and skipped, never written.
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
	if !bytes.Equal(body, buildDefaultOperatorConfig()) {
		t.Fatalf("invalid endpoint leaked a telemetry block:\n%s", body)
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
