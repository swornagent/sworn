// Package bench runs the sworn bench model benchmark: iterate a set of
// verifier models against a set of spec + diff tasks, record pass-rate, cost,
// and hosting jurisdiction, and pick the safe-hosted default model from data.
//
// Pin 3 (SWORN_OPENAI_MODEL override): the benchmark constructs OAI clients
// directly with explicit model IDs, bypassing model.FromEnv's env-override
// logic. If SWORN_OPENAI_MODEL is set, the benchmark still uses the explicit
// model ID for each iteration.
//
// Pin 4 (safe-hosted filter): SelectDefault filters to provider==openai with
// the standard https://api.openai.com/v1 base URL before comparing pass-rates.
//
// Stdlib only — zero runtime dependencies beyond internal packages.
package bench

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/verdict"
	"github.com/swornagent/sworn/internal/verify"
)

// Task is a single benchmark task: a spec + a known-good diff that should PASS.
type Task struct {
	Name     string // e.g. "S01-verifier-core"
	SpecPath string // absolute path to spec.md
	DiffPath string // absolute path to a known-good unified diff
}

// ModelEntry is a verifier model under test. Provider and Model are parsed
// from the "provider/model" ID at construction time.
type ModelEntry struct {
	ModelID  string // e.g. "openai/gpt-4.1"
	Provider string // e.g. "openai"
	Model    string // e.g. "gpt-4.1"
}

// CellResult records one model × task benchmark cell.
type CellResult struct {
	ModelID   string          `json:"model_id"`
	TaskName  string          `json:"task_name"`
	Verdict   verdict.Verdict `json:"verdict"`
	CostUSD   float64         `json:"cost_usd"`
	Error     string          `json:"error,omitempty"`
	Rationale string          `json:"rationale,omitempty"`
}

// Run executes the benchmark across models × tasks. Each cell is a single
// RunFirstPass call with a freshly-constructed OAI client (Pin 3). Caller// provides the API key directly.
func Run(ctx context.Context, models []ModelEntry, tasks []Task, apiKey string) ([]CellResult, error) {
	var results []CellResult
	for _, m := range models {
		v := &model.OAI{
			BaseURL: "https://api.openai.com/v1",
			Model:   m.Model,
			APIKey:  apiKey,
		}
		for _, t := range tasks {
			res := verify.RunFirstPass(ctx, verify.Input{				SpecPath:  t.SpecPath,
				DiffPath:  t.DiffPath,
				ProofPath: "",
				Model:     m.ModelID,
				Verifier:  v,
			})
			cr := CellResult{
				ModelID:   m.ModelID,
				TaskName:  t.Name,
				Verdict:   res.Verdict,
				CostUSD:   res.CostUSD,
				Rationale: res.Rationale,
			}
			if res.Verdict == verdict.Blocked && res.FailedGate != "" {
				cr.Error = fmt.Sprintf("%s: %s", res.FailedGate, res.Rationale)
			}
			results = append(results, cr)
		}
	}
	return results, nil
}

// MakeKnownGoodDiff generates a trivial known-good unified diff for a spec
// file by prepending a benchmark comment line. The resulting diff should PASS
// verification for any correctly-functioning model (Pin 1: diff strategy).
//
// Returns the path to the generated diff file. The caller should clean it up.
func MakeKnownGoodDiff(specPath string, workDir string) (string, error) {
	orig, err := os.ReadFile(specPath)
	if err != nil {
		return "", fmt.Errorf("bench: read spec %s: %w", specPath, err)
	}

	// Prepend a benchmark comment that the verifier should ignore.
	modified := append([]byte("<!-- benchmark: trivial known-good diff for model evaluation -->\n"), orig...)

	// Write the modified spec to a temp file.
	tmpFile, err := os.CreateTemp(workDir, "bench-spec-*.md")
	if err != nil {
		return "", fmt.Errorf("bench: create temp spec: %w", err)
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(modified); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("bench: write temp spec: %w", err)
	}
	tmpFile.Close()

	// Generate unified diff: diff -u orig modified.
	diffFile, err := os.CreateTemp(workDir, "bench-diff-*.patch")
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("bench: create temp diff: %w", err)
	}
	diffPath := diffFile.Name()

	cmd := exec.Command("diff", "-u", specPath, tmpPath)
	var diffOut bytes.Buffer
	cmd.Stdout = &diffOut
	_ = cmd.Run() // diff exits 1 when files differ (expected)

	if _, err := diffFile.Write(diffOut.Bytes()); err != nil {
		diffFile.Close()
		os.Remove(tmpPath)
		os.Remove(diffPath)
		return "", fmt.Errorf("bench: write diff: %w", err)
	}
	diffFile.Close()
	os.Remove(tmpPath) // cleanup the temp modified spec

	return diffPath, nil
}

// ResolveTaskSet walks a release directory and returns a Task for every slice
// directory that contains a spec.md. Tasks are sorted by slice ID.
func ResolveTaskSet(releaseDir string, workDir string) ([]Task, error) {
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		return nil, fmt.Errorf("bench: read release dir %s: %w", releaseDir, err)
	}

	var tasks []Task
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		specPath := filepath.Join(releaseDir, e.Name(), "spec.md")
		if _, err := os.Stat(specPath); err != nil {
			continue
		}
		diffPath, err := MakeKnownGoodDiff(specPath, workDir)
		if err != nil {
			return nil, fmt.Errorf("bench: make diff for %s: %w", e.Name(), err)
		}
		tasks = append(tasks, Task{
			Name:     e.Name(),
			SpecPath: specPath,
			DiffPath: diffPath,
		})
	}
	return tasks, nil
}
