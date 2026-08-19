package baton

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/gitx"
)

func runBatonTestGit(t *testing.T, repository string, args ...string) string {
	t.Helper()
	cmd := exec.Command("/usr/bin/git", append([]string{"-C", repository}, args...)...)
	cmd.Env = append(os.Environ(),
		"LANG=C", "LC_ALL=C",
		"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func TestValidateSliceCandidateScopeRefusalsCarryBoundedPaths(t *testing.T) {
	tempDir := t.TempDir()
	runBatonTestGit(t, tempDir, "init", "--initial-branch=main")
	runBatonTestGit(t, tempDir, "config", "user.name", "Fixture")
	runBatonTestGit(t, tempDir, "config", "user.email", "fixture@example.invalid")

	// Commit initial file
	initialFile := filepath.Join(tempDir, "base.txt")
	if err := os.WriteFile(initialFile, []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runBatonTestGit(t, tempDir, "add", "base.txt")
	runBatonTestGit(t, tempDir, "commit", "-m", "initial")
	baseOID := runBatonTestGit(t, tempDir, "rev-parse", "HEAD")

	repo, err := gitx.Open(tempDir, "/usr/bin/git")
	if err != nil {
		t.Fatal(err)
	}

	planMarkdown := "```sworn-release-manifest-v1\n" +
		"{\n" +
		`  "schema_version": "sworn.release-manifest/v1",` + "\n" +
		`  "release": "rel-1",` + "\n" +
		`  "revision": 1,` + "\n" +
		`  "previous_plan": null,` + "\n" +
		`  "repository": "sworn",` + "\n" +
		`  "target_ref": "refs/heads/main",` + "\n" +
		`  "approval_ref": "operator://rel-1/1",` + "\n" +
		`  "tracks": [` + "\n" +
		`    {` + "\n" +
		`      "id": "T1",` + "\n" +
		`      "depends_on": [],` + "\n" +
		`      "slices": [` + "\n" +
		`        {` + "\n" +
		`          "id": "S1",` + "\n" +
		`          "outcome": "Slice 1",` + "\n" +
		`          "contract_path": "contracts/rel-1/S1.json",` + "\n" +
		`          "digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000",` + "\n" +
		`          "depends_on": [],` + "\n" +
		`          "consumes": [],` + "\n" +
		`          "touchpoints": ["pkg/allowed"]` + "\n" +
		`        }` + "\n" +
		`      ]` + "\n" +
		`    }` + "\n" +
		`  ]` + "\n" +
		"}\n```\n# Plan\n"

	plan, err := ParsePlan([]byte(planMarkdown))
	if err != nil {
		t.Fatal(err)
	}

	inertResolver := func(r gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
		return gitx.RecordRootDecision{
			Kind:       r.Kind,
			Repository: r.Repository,
			RecordRoot: r.RecordRoot,
			Commit:     r.Commit,
			Decision:   "inert",
		}, nil
	}

	t.Run("out-of-scope files bounded to 20", func(t *testing.T) {
		runBatonTestGit(t, tempDir, "checkout", "-B", "out-of-scope-branch", baseOID)
		for i := 0; i < 25; i++ {
			p := filepath.Join(tempDir, "pkg", "disallowed", fmt.Sprintf("file_%02d.txt", i))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("forbidden\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runBatonTestGit(t, tempDir, "add", filepath.ToSlash(filepath.Join("pkg", "disallowed", fmt.Sprintf("file_%02d.txt", i))))
		}
		runBatonTestGit(t, tempDir, "commit", "-m", "25 out of scope changes")
		candidateOID := runBatonTestGit(t, tempDir, "rev-parse", "HEAD")

		err := ValidateSliceCandidateScope(
			UseGitRepository(repo),
			inertResolver,
			plan,
			"S1",
			baseOID,
			candidateOID,
		)
		if err == nil {
			t.Fatal("expected SLICE_OUTSIDE_SCOPE, got nil")
		}
		var recordErr *RecordError
		if !errors.As(err, &recordErr) {
			t.Fatalf("expected *RecordError, got %T: %v", err, err)
		}
		if recordErr.Code != "SLICE_OUTSIDE_SCOPE" {
			t.Fatalf("expected code SLICE_OUTSIDE_SCOPE, got %q", recordErr.Code)
		}
		if len(recordErr.Paths) != 20 {
			t.Fatalf("expected 20 bounded paths, got %d", len(recordErr.Paths))
		}
		if recordErr.TotalPaths != 25 {
			t.Fatalf("expected 25 total paths, got %d", recordErr.TotalPaths)
		}
		for i := 1; i < len(recordErr.Paths); i++ {
			if recordErr.Paths[i-1] >= recordErr.Paths[i] {
				t.Fatalf("paths not strictly sorted: %v", recordErr.Paths)
			}
		}
	})

	t.Run("reserved-root files bounded to 20", func(t *testing.T) {
		runBatonTestGit(t, tempDir, "checkout", "-B", "reserved-root-branch", baseOID)
		for i := 0; i < 25; i++ {
			p := filepath.Join(tempDir, filepath.FromSlash(gitx.DefaultRecordsRoot), "rel-1", fmt.Sprintf("escape_%02d.md", i))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("reserved\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runBatonTestGit(t, tempDir, "add", filepath.ToSlash(filepath.Join(gitx.DefaultRecordsRoot, "rel-1", fmt.Sprintf("escape_%02d.md", i))))
		}
		runBatonTestGit(t, tempDir, "commit", "-m", "25 reserved root changes")
		candidateOID := runBatonTestGit(t, tempDir, "rev-parse", "HEAD")

		err := ValidateSliceCandidateScope(
			UseGitRepository(repo),
			inertResolver,
			plan,
			"S1",
			baseOID,
			candidateOID,
		)
		if err == nil {
			t.Fatal("expected RESERVED_RECORD_ROOT_CHANGED, got nil")
		}
		var recordErr *RecordError
		if !errors.As(err, &recordErr) {
			t.Fatalf("expected *RecordError, got %T: %v", err, err)
		}
		if recordErr.Code != "RESERVED_RECORD_ROOT_CHANGED" {
			t.Fatalf("expected code RESERVED_RECORD_ROOT_CHANGED, got %q", recordErr.Code)
		}
		if len(recordErr.Paths) != 20 {
			t.Fatalf("expected 20 bounded paths, got %d", len(recordErr.Paths))
		}
		if recordErr.TotalPaths != 25 {
			t.Fatalf("expected 25 total paths, got %d", recordErr.TotalPaths)
		}
		for i := 1; i < len(recordErr.Paths); i++ {
			if recordErr.Paths[i-1] >= recordErr.Paths[i] {
				t.Fatalf("paths not strictly sorted: %v", recordErr.Paths)
			}
		}
	})

	t.Run("in-scope change passes", func(t *testing.T) {
		runBatonTestGit(t, tempDir, "checkout", "-B", "valid-branch", baseOID)
		p := filepath.Join(tempDir, "pkg", "allowed", "feature.txt")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("in scope\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runBatonTestGit(t, tempDir, "add", "pkg/allowed/feature.txt")
		runBatonTestGit(t, tempDir, "commit", "-m", "valid change")
		candidateOID := runBatonTestGit(t, tempDir, "rev-parse", "HEAD")

		err := ValidateSliceCandidateScope(
			UseGitRepository(repo),
			inertResolver,
			plan,
			"S1",
			baseOID,
			candidateOID,
		)
		if err != nil {
			t.Fatalf("expected valid candidate to pass, got: %v", err)
		}
	})
}
