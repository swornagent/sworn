package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/model"
)

// mockVerifier implements model.Verifier for testing.
type mockVerifier struct {
	reply string
	err   error
}

func (m *mockVerifier) Verify(ctx context.Context, systemPrompt, userPayload string) (string, float64, int64, int64, error) {
	return m.reply, 0, 0, 0, m.err
}

// ── Leaf helper tests (fast, no I/O) ──────────────────────────────────────

func TestTaskHasAcceptanceChecks(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "has ACs",
			content: "## Acceptance checks\n\n- [ ] do thing\n- [ ] do other thing\n",
			want:    true,
		},
		{
			name:    "no ACs",
			content: "## Acceptance checks\n\nNo checks defined.\n",
			want:    false,
		},
		{
			name:    "empty",
			content: "",
			want:    false,
		},
		{
			name:    "dash bracket space bracket but no space dash bracket",
			content: "Some text with - [x] checked but no unchecked",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasAcceptanceChecks(tt.content)
			if got != tt.want {
				t.Errorf("hasAcceptanceChecks() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaskExtractSpecFromReply(t *testing.T) {
	tests := []struct {
		name  string
		reply string
		want  string // substring that must appear in result
	}{
		{
			name: "bare frontmatter",
			reply: `---
title: 'S01 — test slice'
---

# Slice

- [ ] AC 1
`,
			want: "---",
		},
		{
			name: "markdown code block",
			reply: `Here is the spec:

` + "```markdown\n" + `---
title: 'S01 — test slice'
---

# Slice

- [ ] AC 1
` + "```" + `
`,
			want: "---",
		},
		{
			name: "generic code block with frontmatter",
			reply: `Here is the spec:

` + "```\n" + `---
title: 'S01 — test slice'
---

# Slice

- [ ] AC 1
` + "```" + `
`,
			want: "---",
		},
		{
			name:  "fallback — whole reply",
			reply: "# No spec here\nJust some text\n",
			want:  "# No spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSpecFromReply(tt.reply)
			if !strings.Contains(got, tt.want) {
				t.Errorf("extractSpecFromReply(): result does not contain %q\ngot: %s", tt.want, got)
			}
		})
	}
}

func TestTaskExtractSpecNoACs(t *testing.T) {
	// Planner output with no acceptance checks — used for AC3 validation.
	reply := `---
title: 'S01 — no ACs'
---

# Slice

## Acceptance checks

None.
`
	content := extractSpecFromReply(reply)
	if hasAcceptanceChecks(content) {
		t.Error("expected no acceptance checks in this content")
	}
}

// ── Integration tests (cmdRunTask with mock planner) ────────────────────

// setupMockPlanner replaces plannerFromEnv with a mock that returns the given
// reply and error. Returns a restore function.
func setupMockPlanner(reply string, err error) func() {
	orig := plannerFromEnv
	plannerFromEnv = func(modelID string) (model.Verifier, error) {
		if err != nil {
			return nil, err
		}
		return &mockVerifier{reply: reply}, nil
	}
	return func() { plannerFromEnv = orig }
}

// chdirTaskTemp changes the current working directory to a new temp dir and
// returns the temp dir path and a restore function.
func chdirTaskTemp(t *testing.T) (string, func()) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	return tmp, func() {
		if err := os.Chdir(orig); err != nil {
			t.Errorf("restore wd: %v", err)
		}
	}
}

func TestTaskRunIntegration_PlannerReturnsValidSpec(t *testing.T) {
	withModelConfig(t)
	tmpDir, restoreWd := chdirTaskTemp(t)
	defer restoreWd()

	validSpec := `---
title: 'S01 — test greeting endpoint'
---

# Slice: S01-task

## User outcome

Add a greeting endpoint to the demo server.

## Acceptance checks

- [ ] AC 1: GET /greet returns 200 with {"greeting": "hello"}
- [ ] AC 2: endpoint is registered in the router
`

	restorePlanner := setupMockPlanner(validSpec, nil)
	defer restorePlanner()

	exitCode := cmdRunTask("add a greeting endpoint", "", "", nil, 0, 0, false, nil)
	// cmdRunTask exits non-zero when run.RunSlice fails (expected in test env —
	// no real Go module or verifier model). AC5: on FAIL, exit non-zero and
	// keep spec+proof artefacts for inspection.
	if exitCode == 0 {
		t.Error("expected non-zero exit when RunSlice fails (AC5)")
	}
	// Verify spec.md was written in the task-runs directory.
	taskRunsDir := filepath.Join(tmpDir, ".sworn", "task-runs")
	entries, err := os.ReadDir(taskRunsDir)
	if err != nil {
		t.Fatalf("no task-runs dir at %s: %v", taskRunsDir, err)
	}
	if len(entries) == 0 {
		t.Fatal("no task-run directories created")
	}

	// Find the S01-task/spec.md inside the timestamp directory.
	var specPath string
	for _, e := range entries {
		candidate := filepath.Join(taskRunsDir, e.Name(), "S01-task", "spec.md")
		if _, err := os.Stat(candidate); err == nil {
			specPath = candidate
			break
		}
	}
	if specPath == "" {
		t.Fatal("spec.md not found in any task-run directory")
	}

	content, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read spec.md: %v", err)
	}
	if !strings.Contains(string(content), "greeting endpoint") {
		t.Errorf("spec.md does not contain expected content\ngot: %s", string(content))
	}
	if !hasAcceptanceChecks(string(content)) {
		t.Error("spec.md has no acceptance checks")
	}

	// Verify status.json was written.
	statusPath := filepath.Join(filepath.Dir(specPath), "status.json")
	if _, err := os.Stat(statusPath); err != nil {
		t.Errorf("status.json not found at %s: %v", statusPath, err)
	}
}

func TestTaskRunIntegration_PlannerReturnsNoACs(t *testing.T) {
	withModelConfig(t)
	tmpDir, restoreWd := chdirTaskTemp(t)
	defer restoreWd()

	noACsSpec := `---
title: 'S01 — no acceptance checks'
---

# Slice

No acceptance checks defined.
`

	restorePlanner := setupMockPlanner(noACsSpec, nil)
	defer restorePlanner()

	// Capture stderr to verify the exact error message.
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	exitCode := cmdRunTask("some task", "", "", nil, 0, 0, false, nil)

	w.Close()
	var stderrBuf strings.Builder
	// Drain the pipe (best-effort; test won't hang if short).
	buf := make([]byte, 1024)
	for {
		n, _ := r.Read(buf)
		if n == 0 {
			break
		}
		stderrBuf.Write(buf[:n])
	}
	os.Stderr = origStderr

	if exitCode != 2 {
		t.Errorf("expected exit code 2 for no-ACs planner output, got %d", exitCode)
	}
	stderrOut := stderrBuf.String()
	if !strings.Contains(stderrOut, "planner output contained no acceptance criteria") {
		t.Errorf("expected error message on stderr, got: %s", stderrOut)
	}

	// Verify planner output was saved for inspection.
	taskRunsDir := filepath.Join(tmpDir, ".sworn", "task-runs")
	entries, err := os.ReadDir(taskRunsDir)
	if err != nil {
		t.Fatalf("no task-runs dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no task-run directories created (should keep artefacts on failure)")
	}

	// Verify planner-output.txt exists.
	var plannerOutputPath string
	for _, e := range entries {
		candidate := filepath.Join(taskRunsDir, e.Name(), "S01-task", "planner-output.txt")
		if _, err := os.Stat(candidate); err == nil {
			plannerOutputPath = candidate
			break
		}
	}
	if plannerOutputPath == "" {
		t.Error("planner-output.txt not saved for inspection on no-ACs failure")
	}
}
func TestTaskRunIntegration_PlannerDispatchError(t *testing.T) {
	_, restoreWd := chdirTaskTemp(t)
	defer restoreWd()

	restorePlanner := setupMockPlanner("", fmt.Errorf("simulated planner error"))
	defer restorePlanner()

	exitCode := cmdRunTask("some task", "", "", nil, 0, 0, false, nil)
	if exitCode != 2 {
		t.Errorf("expected exit code 2 for planner model creation error, got %d", exitCode)
	}
	// No artefacts expected — planner model creation failure cleans up taskRoot.
}
func TestTaskDryRunFlagAccepted(t *testing.T) {
	withModelConfig(t)
	// Integration test: --dry-run exercises the code path, verifies exit 0
	// and the printed output confirms planner dispatch would be called.
	// Uses the real resolvePlannerModel (which falls back to openai/gpt-4o
	// when no config is available) but never calls the planner.
	exitCode := cmdRunTask("test task", "", "openai/gpt-4o", nil, 0, 0, true, nil)
	if exitCode != 0 {
		t.Errorf("dry-run exited %d, want 0", exitCode)
	}
}

func TestTaskRun_EmptyDescription(t *testing.T) {
	exitCode := cmdRunTask("", "", "", nil, 0, 0, false, nil)
	if exitCode != 64 {
		t.Errorf("expected exit 64 for empty --task, got %d", exitCode)
	}
}
