package implement

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/spec"
	"github.com/swornagent/sworn/internal/state"
)

// proofRecord is the JSON shape written to proof.json.
type proofRecord struct {
	Schema                string          `json:"$schema"`
	SchemaVersion         int             `json:"schema_version"`
	SliceID               string          `json:"slice_id"`
	Release               string          `json:"release"`
	Scope                 string          `json:"scope"`
	FilesChanged          []string        `json:"files_changed"`
	TestResults           []testResultRec `json:"test_results"`
	ReachabilityArtifacts []string        `json:"reachability_artifacts"`
	Delivered             []string        `json:"delivered"`
	NotDelivered          []string        `json:"not_delivered"`
	Divergence            []string        `json:"divergence"`
}

type testResultRec struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
}

// WriteProofRecord generates proof.json from live repo state and writes it
// to the slice directory. It uses git diff --name-only <start_commit>..HEAD
// for files_changed, derives delivered from spec.md acceptance criteria,
// not_delivered from status.json open_deferrals, and divergence from
// comparing planned_files to actual files.
func WriteProofRecord(workspaceRoot, specPath, statusPath, sliceDir string) error {
	st, err := state.Read(statusPath)
	if err != nil {
		return fmt.Errorf("proof_record: read status: %w", err)
	}

	// Spec content: spec.json preferred (authoritative), spec.md legacy fallback.
	specRec, specMD, loadErr := spec.LoadSpec(sliceDir)
	if loadErr != nil {
		return fmt.Errorf("proof_record: read spec: %w", loadErr)
	}

	rec := proofRecord{
		Schema:        baton.ProofSchemaURI,
		SchemaVersion: 1,
		SliceID:       st.SliceID,
		Release:       st.Release,
	}
	if specRec != nil {
		rec.Scope = specRec.UserOutcome
	} else {
		rec.Scope = extractScope(specMD)
	}

	// files_changed: use git diff --name-only <start_commit>..HEAD
	rec.FilesChanged = filesChangedFromGit(workspaceRoot, st.StartCommit)

	// test_results: run the test commands from status.json.
	rec.TestResults = runTestCommands(workspaceRoot, st.TestCommands)

	// reachability_artifacts: from status.json.
	rec.ReachabilityArtifacts = st.ReachabilityArtifacts

	// delivered: from spec.json acceptance criteria when present (each AC is a
	// delivered item — spec-v1 has no per-AC checkbox), else the checked
	// spec.md acceptance criteria (legacy fallback).
	if specRec != nil {
		rec.Delivered = deliveredFromRecord(specRec)
	} else {
		rec.Delivered = deliveredFromSpec(specMD)
	}

	// not_delivered: from status.json open_deferrals.
	rec.NotDelivered = notDeliveredFromDeferrals(st.DeferralStrings())

	// divergence: compare planned_files to actual git diff.
	rec.Divergence = divergenceFromPlan(st.PlannedFiles, rec.FilesChanged)

	// Marshal and validate.
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("proof_record: marshal: %w", err)
	}
	if err := baton.Validate("proof-v1", data); err != nil {
		return fmt.Errorf("proof_record: validation failed: %w", err)
	}

	proofJSONPath := filepath.Join(sliceDir, "proof.json")
	if err := os.WriteFile(proofJSONPath, data, 0o644); err != nil {
		return fmt.Errorf("proof_record: write: %w", err)
	}
	return nil
}

// filesChangedFromGit returns the list of files changed between startCommit
// and HEAD. Falls back to git diff --name-only HEAD~1..HEAD if startCommit is
// empty, or git status --porcelain as a last resort.
func filesChangedFromGit(workspaceRoot, startCommit string) []string {
	var files []string
	var out string
	var err error

	if startCommit != "" {
		out, err = runGitCmdOut(workspaceRoot, "diff", "--name-only", startCommit+"..HEAD")
		if err == nil && out != "" {
			for _, f := range strings.Split(out, "\n") {
				f = strings.TrimSpace(f)
				if f != "" {
					files = append(files, f)
				}
			}
			return files
		}
	}

	// Fallback: diff against HEAD~1.
	out, err = runGitCmdOut(workspaceRoot, "diff", "--name-only", "HEAD~1..HEAD")
	if err == nil && out != "" {
		for _, f := range strings.Split(out, "\n") {
			f = strings.TrimSpace(f)
			if f != "" {
				files = append(files, f)
			}
		}
		return files
	}

	// Last resort: git status --porcelain.
	out, err = runGitCmdOut(workspaceRoot, "status", "--porcelain")
	if err == nil && out != "" {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if len(line) > 3 {
				files = append(files, line[3:])
			}
		}
	}
	return files
}

// runTestCommands runs each test command and captures its output + exit code.
func runTestCommands(workspaceRoot string, commands []string) []testResultRec {
	var results []testResultRec
	for _, cmd := range commands {
		parts := strings.Fields(cmd)
		if len(parts) == 0 {
			continue
		}
		c := exec.Command(parts[0], parts[1:]...)
		c.Dir = workspaceRoot
		out, err := c.CombinedOutput()
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
		results = append(results, testResultRec{
			Command:  cmd,
			ExitCode: exitCode,
			Output:   string(out),
		})
	}
	return results
}

// deliveredFromRecord lists a spec.json record's acceptance criteria as
// delivered items ("AC-NN: text"). spec-v1 has no per-AC checkbox, so every
// acceptance criterion in the authoritative record is a delivered item.
func deliveredFromRecord(rec *spec.Record) []string {
	if rec == nil {
		return nil
	}
	var delivered []string
	for _, ac := range rec.AcceptanceCriteria {
		if ac.ID != "" {
			delivered = append(delivered, ac.ID+": "+ac.Text)
		} else {
			delivered = append(delivered, ac.Text)
		}
	}
	return delivered
}

// deliveredFromSpec extracts checked acceptance criteria from spec.md.
func deliveredFromSpec(spec string) []string {
	var delivered []string
	inSection := false
	for _, line := range strings.Split(spec, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			inSection = strings.Contains(strings.ToLower(trimmed), "acceptance check")
			continue
		}
		if !inSection {
			continue
		}
		if m := reACLine.FindStringSubmatch(line); m != nil {
			text := strings.TrimSpace(m[1])
			if strings.HasPrefix(strings.ToUpper(text), "NOTE:") {
				continue
			}
			// Only include checked items ([x]).
			if strings.Contains(line, "[x]") || strings.Contains(line, "[X]") {
				delivered = append(delivered, text)
			}
		}
	}
	return delivered
}

// notDeliveredFromDeferrals converts open_deferrals to a string array.
// Each deferral is represented as "<item>: <why> (tracked: <tracking>)"
func notDeliveredFromDeferrals(deferrals []string) []string {
	if len(deferrals) == 0 {
		return nil
	}
	return deferrals
}

// divergenceFromPlan compares planned files to actual files and reports
// any unexpected files (in actual but not planned) or missing files
// (in planned but not actual).
func divergenceFromPlan(planned, actual []string) []string {
	plannedSet := make(map[string]bool)
	for _, f := range planned {
		plannedSet[f] = true
	}
	actualSet := make(map[string]bool)
	for _, f := range actual {
		actualSet[f] = true
	}

	var divergences []string
	for _, f := range actual {
		if !plannedSet[f] {
			divergences = append(divergences, "unexpected file: "+f)
		}
	}
	for _, f := range planned {
		if !actualSet[f] {
			divergences = append(divergences, "planned but not changed: "+f)
		}
	}
	return divergences
}

// runGitCmdOut runs a git command and returns trimmed stdout.
func runGitCmdOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
