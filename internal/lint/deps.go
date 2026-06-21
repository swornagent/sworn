package lint

import (
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
)

type Status struct {
    SliceID      string   `json:"slice_id"`
    Release      string   `json:"release"`
    PlannedFiles []string `json:"planned_files"`
    StartCommit  *string  `json:"start_commit"`
}

// readStatus reads the status.json for the given slice directory.
func readStatus(sliceDir string) (*Status, error) {
    data, err := os.ReadFile(filepath.Join(sliceDir, "status.json"))
    if err != nil {
        return nil, err
    }
    var s Status
    if err := json.Unmarshal(data, &s); err != nil {
        return nil, err
    }
    return &s, nil
}

// CheckDeps verifies that any changes to go.mod or go.sum since baseRef are declared in planned_files.
// If baseRef is empty, it falls back to the slice's start_commit; if that is nil, it uses "release-wt/<release>".
func CheckDeps(sliceDir string, baseRef string) error {
    status, err := readStatus(sliceDir)
    if err != nil {
        return fmt.Errorf("reading status.json: %w", err)
    }

    // Determine the base reference.
    if baseRef == "" {
        if status.StartCommit != nil && *status.StartCommit != "" {
            baseRef = *status.StartCommit
        } else {
            baseRef = fmt.Sprintf("release-wt/%s", status.Release)
        }
    }

    // Run git diff to list changed files.
    cmd := exec.Command("git", "diff", "--name-only", baseRef+"...HEAD")
    cmd.Dir = sliceDir // run from slice directory (repo root is parent of docs)
    out, err := cmd.Output()
    if err != nil {
        // git diff returns non-zero if there is no diff? Actually it returns 0.
        // If command fails, propagate.
        return fmt.Errorf("git diff failed: %w", err)
    }
    changed := strings.Fields(string(out))

    // Determine if go.mod or go.sum changed.
    var depChanges []string
    for _, f := range changed {
        if f == "go.mod" || f == "go.sum" {
            depChanges = append(depChanges, f)
        }
    }
    if len(depChanges) == 0 {
        // No dependency changes – pass.
        return nil
    }

    // Build a set of planned files for quick lookup.
    plannedSet := make(map[string]struct{}, len(status.PlannedFiles))
    for _, p := range status.PlannedFiles {
        plannedSet[p] = struct{}{}
    }

    var undeclared []string
    for _, d := range depChanges {
        if _, ok := plannedSet[d]; !ok {
            undeclared = append(undeclared, d)
        }
    }
    if len(undeclared) > 0 {
        return errors.New("undeclared dependency file(s): " + strings.Join(undeclared, ", "))
    }
    return nil
}