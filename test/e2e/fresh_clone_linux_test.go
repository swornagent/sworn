//go:build linux

package e2e

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Release-completion gate: a fresh clone of this repository carries the
// exact post-migration shape and no legacy .baton surface.
//
// This test is DECLARED, NOT EXECUTED by a role (ADR 0010). It asserts that
// the reserved records root lives at .sworn/records and no legacy .baton
// surface is tracked; it is the host-boundary release-completion assertion.
func TestFreshCloneCarriesMigratedRecordsAndNoLegacySurface(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)

	// Clone the repository fresh so the assertion is about what a new adopter
	// sees, never about this checkout's working files.
	clone := t.TempDir()
	command := exec.Command(e2eGit, "clone", "--quiet", root, filepath.Join(clone, "repo"))
	command.Env = cleanEnvironment(map[string]string{"LANG": "C", "LC_ALL": "C"})
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v: %s", err, output)
	}
	repo := filepath.Join(clone, "repo")
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(e2eGit, append([]string{"-C", repo}, args...)...)
		cmd.Env = cleanEnvironment(map[string]string{"LANG": "C", "LC_ALL": "C"})
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}

	// The working tree of a fresh clone is clean.
	if status := run("status", "--porcelain=v1", "--untracked-files=all"); status != "" {
		t.Fatalf("fresh clone is not clean:\n%s", status)
	}

	// No .baton path anywhere in the tracked tree.
	tracked := run("ls-files")
	if strings.Contains(tracked, ".baton") {
		t.Fatalf("fresh clone tracks a .baton path:\n%s", tracked)
	}

	// Every release recorded at HEAD lives under .sworn/records and its
	// frozen plan is readable. The inventory comes from the records tree
	// itself: a fresh clone maps branches to remote-tracking refs, and
	// historical release-wt refs predating the record scheme carry no
	// record at HEAD, so ref inventory is the wrong source of truth here.
	recordEntries := run("ls-tree", "--name-only", "HEAD", ".sworn/records/")
	var releases []string
	for _, entry := range strings.Split(recordEntries, "\n") {
		if entry = strings.TrimSpace(entry); entry != "" {
			releases = append(releases, strings.TrimPrefix(entry, ".sworn/records/"))
		}
	}
	if len(releases) == 0 {
		t.Fatal("no recorded releases under .sworn/records in the fresh clone")
	}
	for _, release := range releases {
		body := run("show", "HEAD:"+".sworn/records/"+release+"/plan.md")
		if !strings.HasPrefix(body, "```") && !strings.HasPrefix(body, "{\n") {
			t.Fatalf("release %s has no recorded plan under .sworn/records", release)
		}
	}

	// The committed project configuration exists.
	if run("show", "HEAD:docs/sworn/sworn.json") == "" {
		t.Fatal("committed docs/sworn/sworn.json is missing")
	}

	// Authored plan and slice contracts are present under the documents root.
	authoredPlans := run("ls-tree", "-r", "--name-only", "HEAD", "--", "docs/sworn/")
	if !strings.Contains(authoredPlans, "/plan.md") ||
		!strings.Contains(authoredPlans, "/contracts/") {
		t.Fatalf("fresh clone lacks authored plan/contracts under docs/sworn:\n%s", authoredPlans)
	}

	// No journals or working files are tracked under .sworn except records.
	for _, path := range strings.Split(tracked, "\n") {
		path = strings.TrimSpace(path)
		if path == "" || path == ".sworn/records" || strings.HasPrefix(path, ".sworn/records/") {
			continue
		}
		if path == ".sworn" || strings.HasPrefix(path, ".sworn/") {
			t.Fatalf("run state path %q is tracked in the fresh clone", path)
		}
	}
}
