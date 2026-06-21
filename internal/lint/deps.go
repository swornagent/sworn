// Package lint implements the `sworn lint` sub-targets that perform
// mechanical, pre-verification checks on release slices. Each target is
// fail-closed: exit 0 only when the check passes, non-zero on any violation.
//
// The deps target verifies that go.mod / go.sum changes in a slice's diff are
// declared in that slice's status.json planned_files.
package lint

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/state"
)

// depFiles is the set of Go dependency files that, if changed, must appear in
// a slice's planned_files.
var depFiles = map[string]bool{
	"go.mod": true,
	"go.sum": true,
}

// CheckDeps verifies that any changes to go.mod or go.sum since baseRef are
// declared in the slice's status.json planned_files. If baseRef is empty, it
// falls back to the slice's start_commit; if start_commit is also empty, it
// derives "release-wt/<release>" from the status.json release field.
//
// Returns nil if no dependency files changed, or if all changed dep files are
// declared in planned_files. Returns an error naming the undeclared file(s)
// otherwise.
func CheckDeps(sliceDir string, baseRef string) error {
	st, err := state.Read(sliceDir + "/status.json")
	if err != nil {
		return fmt.Errorf("lint deps: reading status.json: %w", err)
	}

	// Determine the base reference.
	if baseRef == "" {
		if st.StartCommit != "" {
			baseRef = st.StartCommit
		} else {
			baseRef = "release-wt/" + st.Release
		}
	}

	// Run git diff to list changed files. Use two-dot diff to capture exactly
	// the commits on the current branch since baseRef.
	cmd := exec.Command("git", "diff", "--name-only", baseRef+"..HEAD")
	cmd.Dir = sliceDir
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("lint deps: git diff %s..HEAD: %w", baseRef, err)
	}
	changed := strings.Fields(string(out))

	// Filter for dependency files.
	var depChanges []string
	for _, f := range changed {
		if depFiles[f] {
			depChanges = append(depChanges, f)
		}
	}
	if len(depChanges) == 0 {
		return nil
	}

	// Build a set of planned files for quick lookup.
	plannedSet := make(map[string]bool, len(st.PlannedFiles))
	for _, p := range st.PlannedFiles {
		plannedSet[p] = true
	}

	var undeclared []string
	for _, d := range depChanges {
		if !plannedSet[d] {
			undeclared = append(undeclared, d)
		}
	}
	if len(undeclared) > 0 {
		sort.Strings(undeclared)
		return fmt.Errorf("undeclared dependency file(s): %s", strings.Join(undeclared, ", "))
	}
	return nil
}
