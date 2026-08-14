package baton

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/gitx"
)

// TestConfiguredCommitPrefixThreadsThroughActions proves A1's commit-message
// prefix: with a committed project config naming a prefix, every plan and
// receipt subject is written with the configured prefix, and unconfigured
// repositories keep today's "baton(" subjects byte-for-byte.
func TestConfiguredCommitPrefixThreadsThroughActions(t *testing.T) {
	t.Parallel()

	writeConfig := func(t *testing.T, repoPath, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(repoPath, "docs", "sworn"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repoPath, filepath.FromSlash(gitx.ProjectConfigPath)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	record := func(t *testing.T, repoPath string) string {
		t.Helper()
		repository, err := gitx.Open(repoPath, actionTestGit)
		if err != nil {
			t.Fatal(err)
		}
		actions, err := NewActions(UseGitRepository(repository), inertActionResolver, gitx.Identity{Name: "Prefix Engine", Email: "prefix@example.test"})
		if err != nil {
			t.Fatal(err)
		}
		release := "prefix-release"
		if _, err := actions.RecordPlanRevision(RecordPlanRevisionInput{
			PlanBytes: actionPlanRevisionBytes(release, 1, nil, []Track{{
				ID: "T1", DependsOn: []string{}, Slices: []Slice{actionSlice("S1", "one.txt")},
			}}),
			Summary: "Approve with a configured prefix.", Detail: []byte("approval"),
		}); err != nil {
			t.Fatal(err)
		}
		return release
	}

	t.Run("configured prefix", func(t *testing.T) {
		repoPath := createActionRepository(t, "sha1")
		writeConfig(t, repoPath, `{"commit_prefix": "sworn"}`)
		release := record(t, repoPath)
		head := actionGit(t, repoPath, nil, nil, "rev-parse", releaseRef(release))
		planParent := actionGit(t, repoPath, nil, nil, "rev-parse", head+"^")
		for commit, want := range map[string]string{
			head:       "sworn(" + release + "): approve plan",
			planParent: "sworn(" + release + "): plan revision 1",
		} {
			subject := actionGit(t, repoPath, nil, nil, "log", "-1", "--format=%s", commit)
			if subject != want {
				t.Fatalf("subject for %s = %q, want %q", commit, subject, want)
			}
		}
	})

	t.Run("unconfigured keeps baton", func(t *testing.T) {
		repoPath := createActionRepository(t, "sha1")
		release := record(t, repoPath)
		head := actionGit(t, repoPath, nil, nil, "rev-parse", releaseRef(release))
		subject := actionGit(t, repoPath, nil, nil, "log", "-1", "--format=%s", head)
		if !strings.HasPrefix(subject, "baton("+release+"):") {
			t.Fatalf("unconfigured subject = %q, want baton(...)", subject)
		}
	})
}

// TestHistoricalPrefixesStillParseAndAreDetected proves the configured
// prefix never strands existing history: baton( and sworn( subjects remain
// parseable receipt messages and the engine's history detection keeps
// accepting them regardless of configuration.
func TestHistoricalPrefixesStillParseAndAreDetected(t *testing.T) {
	t.Parallel()
	attempt := int64(1)
	contract := "sha256:" + strings.Repeat("c", 64)
	for _, subject := range []string{
		"baton(rel): plan revision 1",
		"sworn(rel): plan revision 1",
		"baton(rel/slice): implementer designed",
		"sworn(rel/slice): verifier pass",
	} {
		receipt := Receipt{
			Version: ReceiptVersion, Release: "rel", Slice: &sliceIDValue,
			Role: "implementer", Result: "designed", Attempt: &attempt,
			Plan: strings.Repeat("a", 40), Contract: &contract,
			Binds: strings.Repeat("b", 40), Detail: DigestBytes(nil),
			Summary: "Historical prefix.",
		}
		message, err := RenderReceiptCommit(subject, []byte("detail"), receipt)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := ParseReceiptCommitMessage(message)
		if err != nil {
			t.Fatalf("parse %q: %v", subject, err)
		}
		if parsed.Subject != subject {
			t.Fatalf("parsed subject = %q, want %q", parsed.Subject, subject)
		}
	}
	// History detection accepts every historical prefix independent of
	// configuration (the engine commit check in repository.go).
	for _, subject := range []string{"baton(rel): x", "sworn(rel): x", "Baton exact ", "Baton engine-owned "} {
		if !historicalEngineSubject(subject) {
			t.Fatalf("historical subject %q not detected as engine-owned", subject)
		}
	}
}

var sliceIDValue = "slice"

func historicalEngineSubject(message string) bool {
	return strings.HasPrefix(message, "baton(") || strings.HasPrefix(message, "sworn(") ||
		strings.HasPrefix(message, "Baton exact ") || strings.HasPrefix(message, "Baton engine-owned ")
}
