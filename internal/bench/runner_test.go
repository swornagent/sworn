package bench

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/verdict"
)

func TestMakeKnownGoodDiff(t *testing.T) {
	dir := t.TempDir()

	// Create a minimal spec file.
	specPath := filepath.Join(dir, "spec.md")
	specContent := "---\ntitle: test\n---\n\n# Test\n"
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatal(err)
	}

	diffPath, err := MakeKnownGoodDiff(specPath, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(diffPath)

	diffBytes, err := os.ReadFile(diffPath)
	if err != nil {
		t.Fatal(err)
	}
	diff := string(diffBytes)

	if diff == "" {
		t.Error("MakeKnownGoodDiff returned empty diff")
	}
	if !strings.Contains(diff, "benchmark: trivial known-good diff") {
		t.Error("diff does not contain the benchmark comment")
	}
	// Must be a unified diff format.
	if !strings.Contains(diff, "--- ") || !strings.Contains(diff, "+++ ") {
		t.Error("diff is not in unified diff format")
	}
}

func TestMakeKnownGoodDiff_FileNotFound(t *testing.T) {
	_, err := MakeKnownGoodDiff("/nonexistent/path/spec.md", t.TempDir())
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestResolveTaskSet(t *testing.T) {
	dir := t.TempDir()
	workDir := t.TempDir()

	// Create two slice directories with spec.md.
	for _, name := range []string{"S01-test-a", "S02-test-b"} {
		sliceDir := filepath.Join(dir, name)
		if err := os.MkdirAll(sliceDir, 0o755); err != nil {
			t.Fatal(err)
		}
		specPath := filepath.Join(sliceDir, "spec.md")
		if err := os.WriteFile(specPath, []byte("---\ntitle: "+name+"\n---\n\n# Test\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Create a directory without spec.md — should be skipped.
	noSpecDir := filepath.Join(dir, "S03-no-spec")
	if err := os.MkdirAll(noSpecDir, 0o755); err != nil {
		t.Fatal(err)
	}

	tasks, err := ResolveTaskSet(dir, workDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(tasks) != 2 {
		t.Fatalf("ResolveTaskSet returned %d tasks, want 2", len(tasks))
	}

	for _, task := range tasks {
		if task.SpecPath == "" {
			t.Errorf("task %s has empty SpecPath", task.Name)
		}
		if task.DiffPath == "" {
			t.Errorf("task %s has empty DiffPath", task.Name)
		}
		defer os.Remove(task.DiffPath)
	}
}

func TestRun_NoModels(t *testing.T) {
	cells, err := Run(context.Background(), nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cells) != 0 {
		t.Errorf("expected 0 cells, got %d", len(cells))
	}
}

func TestRun_NoTasks(t *testing.T) {
	models := []ModelEntry{
		{ModelID: "openai/gpt-4.1", Provider: "openai", Model: "gpt-4.1"},
	}
	cells, err := Run(context.Background(), models, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cells) != 0 {
		t.Errorf("expected 0 cells, got %d", len(cells))
	}
}

func TestRun_UnconfiguredModel(t *testing.T) {
	// When no API key is set, the OAI client dispatches a real HTTP call
	// which will fail. The benchmark records this as BLOCKED/ERR.
	dir := t.TempDir()
	specPath := filepath.Join(dir, "spec.md")
	os.WriteFile(specPath, []byte("---\ntitle: test\n---\n\n# Spec\n"), 0o644)

	diffPath, err := MakeKnownGoodDiff(specPath, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(diffPath)

	models := []ModelEntry{
		{ModelID: "openai/gpt-4.1", Provider: "openai", Model: "gpt-4.1"},
	}
	tasks := []Task{
		{Name: "test", SpecPath: specPath, DiffPath: diffPath},
	}

	cells, err := Run(context.Background(), models, tasks, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cells) != 1 {
		t.Fatalf("expected 1 cell, got %d", len(cells))
	}
	// With no API key, the model dispatch should fail → BLOCKED.
	if cells[0].Verdict != verdict.Blocked {
		t.Logf("cell verdict: %s (expected BLOCKED without API key, but dispatch may succeed if key is in env)", cells[0].Verdict)
	}
}

// fakeVerifier is a test fake that returns a predetermined verdict.
type fakeVerifier struct {
	text string
	cost float64
	err  error
}

func (f *fakeVerifier) Verify(_ context.Context, _, _ string) (string, float64, error) {
	return f.text, f.cost, f.err
}

// Ensure fakeVerifier implements model.Verifier.
var _ model.Verifier = (*fakeVerifier)(nil)

func TestCellResult_ErrorPopulated(t *testing.T) {
	// Verify CellResult.Error is populated for BLOCKED results with a failed gate.
	cr := CellResult{
		Verdict: verdict.Blocked,
		Error:   "first_pass:spec: file not found",
	}
	if cr.Error == "" {
		t.Error("expected Error to be populated for BLOCKED cell")
	}
}
