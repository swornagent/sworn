package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

func TestVersionJSONReportsExactBatonAdmission(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"version", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr = %q", code, stderr.String())
	}
	var got versionInfo
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != swornVersion || got.State != swornState {
		t.Fatalf("version identity = %#v", got)
	}
	if got.Baton.PackageVersion != baton.PackageVersion ||
		got.Baton.TagObject != baton.TagObject ||
		got.Baton.Commit != baton.Commit ||
		got.Baton.Tree != baton.Tree ||
		got.Baton.SupportPackageSHA256 != baton.SupportPackageSHA256 ||
		got.Baton.ManifestSHA256 != baton.ManifestSHA256 ||
		got.Baton.AssetCount != baton.AssetCount ||
		got.Baton.AssetBytes != baton.AssetBytes {
		t.Fatalf("Baton identity = %#v", got.Baton)
	}
	if strings.Contains(stdout.String(), `"commit":"unknown"`) {
		t.Fatalf("version output reintroduced Sworn commit stamping: %s", stdout.String())
	}
}

func TestVersionTextIsSmallAndExplicit(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr = %q", code, stderr.String())
	}
	want := "Sworn 1.0.0-rc.2-dev\nBaton 1.0.0-rc.14\n\n" +
		"Technical details:\n" +
		"  state: baton-rc14-admitted\n" +
		"  baton commit: " + baton.Commit + "\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestHelpIsTheOnlyArgumentFreeCommand(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{nil, {"help"}, {"--help"}, {"-h"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("run(%v) = %d, stderr = %q", args, code, stderr.String())
		}
		if stdout.String() != usage || stderr.Len() != 0 {
			t.Fatalf("run(%v) stdout = %q, stderr = %q", args, stdout.String(), stderr.String())
		}
	}
}

func TestRetiredAndUnknownCommandsShareOneClosedPath(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"__executor-shim", "--marker", "/unwritable"},
		{"deliver"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Fatalf("run(%v) = %d, want 2", args, code)
		}
		if stdout.Len() != 0 {
			t.Fatalf("run(%v) stdout = %q", args, stdout.String())
		}
		if !strings.Contains(stderr.String(), "unknown command") ||
			!strings.Contains(stderr.String(), `Run "sworn help"`) {
			t.Fatalf("run(%v) stderr = %q", args, stderr.String())
		}
		if strings.Contains(stderr.String(), "/unreadable") || strings.Contains(stderr.String(), "/unwritable") {
			t.Fatalf("run(%v) inspected or echoed a retired path: %q", args, stderr.String())
		}
	}
}

func TestBoardTerminalAndJSONRenderOneReadOnlySnapshot(t *testing.T) {
	journalPath := boardJournalFixture(t)
	before, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}

	var jsonOut, jsonErr bytes.Buffer
	if code := run(
		[]string{"board", "--run", "run-1", "--journal", journalPath, "--json"},
		&jsonOut,
		&jsonErr,
	); code != 0 {
		t.Fatalf("board JSON = %d, stderr = %q", code, jsonErr.String())
	}
	var snapshot cockpit.Snapshot
	if err := json.Unmarshal(jsonOut.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(jsonOut.Bytes(), []byte("\n  \"schema_version\"")) {
		t.Fatalf("board JSON is not pretty printed: %q", jsonOut.String())
	}
	if snapshot.SchemaVersion != cockpit.SnapshotSchemaVersion ||
		snapshot.Run.ID != "run-1" ||
		snapshot.Run.Release != "release-1" ||
		snapshot.Run.TargetRef != "refs/heads/main" ||
		snapshot.Run.DesiredState != "running" {
		t.Fatalf("snapshot facts = %#v", snapshot)
	}
	if len(snapshot.Diagnostics) != 1 ||
		snapshot.Diagnostics[0].Code != "BATON_UNAVAILABLE" {
		t.Fatalf("snapshot diagnostics = %#v", snapshot.Diagnostics)
	}

	var terminalOut, terminalErr bytes.Buffer
	if code := run(
		[]string{"board", "--run", "run-1", "--journal", journalPath},
		&terminalOut,
		&terminalErr,
	); code != 0 {
		t.Fatalf("board terminal = %d, stderr = %q", code, terminalErr.String())
	}
	if terminalOut.String() != cockpit.RenderTerminal(snapshot) {
		t.Fatalf(
			"terminal did not render JSON snapshot facts:\n%s\nwant:\n%s",
			terminalOut.String(),
			cockpit.RenderTerminal(snapshot),
		)
	}
	after, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("board mutated the journal")
	}
}

func TestBoardRejectsInvalidArgumentsPathsAndRunsWithoutExposure(t *testing.T) {
	journalPath := boardJournalFixture(t)
	unavailable := filepath.Join(t.TempDir(), "TOP-SECRET.sqlite")
	tests := []struct {
		name   string
		args   []string
		code   int
		stderr string
	}{
		{
			name:   "missing run",
			args:   []string{"board", "--journal", journalPath},
			code:   2,
			stderr: "usage: sworn board --run ID --journal PATH [--json]\n",
		},
		{
			name: "duplicate JSON switch",
			args: []string{
				"board", "--run", "run-1", "--journal", journalPath,
				"--json", "--json",
			},
			code:   2,
			stderr: "usage: sworn board --run ID --journal PATH [--json]\n",
		},
		{
			name: "unknown switch",
			args: []string{
				"board", "--run", "run-1", "--journal", journalPath, "--write",
			},
			code:   2,
			stderr: "usage: sworn board --run ID --journal PATH [--json]\n",
		},
		{
			name: "switch consumed as value",
			args: []string{
				"board", "--run", "--json", "--journal", journalPath,
			},
			code:   2,
			stderr: "usage: sworn board --run ID --journal PATH [--json]\n",
		},
		{
			name: "relative journal",
			args: []string{
				"board", "--run", "run-1", "--journal", "TOP-SECRET.sqlite",
			},
			code: 1,
			stderr: "sworn board: Could not open the saved run record. " +
				"Check the journal path and file permissions.\n" +
				"Technical code: JOURNAL_UNAVAILABLE\n",
		},
		{
			name: "missing journal",
			args: []string{
				"board", "--run", "run-1", "--journal", unavailable,
			},
			code: 1,
			stderr: "sworn board: Could not open the saved run record. " +
				"Check the journal path and file permissions.\n" +
				"Technical code: JOURNAL_UNAVAILABLE\n",
		},
		{
			name: "unknown run",
			args: []string{
				"board", "--run", "TOP-SECRET", "--journal", journalPath,
			},
			code: 1,
			stderr: "sworn board: Could not build the delivery board " +
				"from the saved run and Git state.\n" +
				"Technical code: JOURNAL_UNAVAILABLE\n",
		},
		{
			name: "malformed run",
			args: []string{
				"board", "--run", "TOP SECRET", "--journal", journalPath,
			},
			code: 1,
			stderr: "sworn board: Could not build the delivery board " +
				"from the saved run and Git state.\n" +
				"Technical code: JOURNAL_UNAVAILABLE\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(test.args, &stdout, &stderr); code != test.code {
				t.Fatalf("run() = %d, want %d", code, test.code)
			}
			if stdout.Len() != 0 || stderr.String() != test.stderr {
				t.Fatalf(
					"stdout = %q, stderr = %q, want stderr %q",
					stdout.String(),
					stderr.String(),
					test.stderr,
				)
			}
			if strings.Contains(stderr.String(), "TOP-SECRET") ||
				strings.Contains(stderr.String(), "TOP SECRET") ||
				strings.Contains(stderr.String(), journalPath) {
				t.Fatalf("board exposed rejected input: %q", stderr.String())
			}
		})
	}
	if _, err := os.Lstat(unavailable); !os.IsNotExist(err) {
		t.Fatalf("board created unavailable journal path: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if code := run(
		[]string{"status", "--run", "run-1", "--journal", journalPath},
		&stdout,
		&stderr,
	); code != 2 {
		t.Fatalf("status without required --json = %d, want 2", code)
	}
	if stderr.String() != "usage: sworn status --run ID --journal PATH --json\n" {
		t.Fatalf("status stderr = %q", stderr.String())
	}
}

func TestBoardFailsClosedWhenGitIsUnavailable(t *testing.T) {
	t.Setenv("PATH", "")
	var stdout, stderr bytes.Buffer
	if code := run(
		[]string{"board", "--run", "run-1", "--journal", "/not-consumed.sqlite"},
		&stdout,
		&stderr,
	); code != 1 {
		t.Fatalf("run() = %d, want 1", code)
	}
	want := "sworn board: Could not find Git. Install Git or make it " +
		"available on PATH.\nTechnical code: GIT_UNAVAILABLE\n"
	if stdout.Len() != 0 || stderr.String() != want {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRuntimeCommandsRejectEveryOpenOrAmbiguousShapeBeforeIO(t *testing.T) {
	t.Parallel()

	tests := []struct {
		args []string
		want string
	}{
		{[]string{"run", "--manifest", "/blocking"}, "usage: sworn run"},
		{[]string{"run", "--manifest", "/blocking", "--journal", "/journal", "--journal", "/other"}, "usage: sworn run"},
		{[]string{"resume", "--run", "r1"}, "usage: sworn resume"},
		{[]string{"status", "--run", "r1", "--journal", "/blocking"}, "usage: sworn status"},
	}
	for _, test := range tests {
		var stdout, stderr bytes.Buffer
		if code := run(test.args, &stdout, &stderr); code != 2 {
			t.Fatalf("run(%v) = %d, want 2", test.args, code)
		}
		if stdout.Len() != 0 || !strings.Contains(stderr.String(), test.want) {
			t.Fatalf("run(%v) stdout = %q, stderr = %q", test.args, stdout.String(), stderr.String())
		}
		if strings.Contains(stderr.String(), "/blocking") {
			t.Fatalf("run(%v) consumed or exposed ignored path: %q", test.args, stderr.String())
		}
	}
}

func boardJournalFixture(t *testing.T) string {
	t.Helper()
	repository := filepath.Join(t.TempDir(), "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "init", "--quiet", "--initial-branch=main")
	readme := filepath.Join(repository, "README.md")
	if err := os.WriteFile(readme, []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "--", "README.md")
	runGit(
		t,
		repository,
		"-c", "user.name=Sworn Board Test",
		"-c", "user.email=sworn-board@example.invalid",
		"commit", "--quiet", "-m", "fixture",
	)

	profile := driver.RoleSelection{
		Profile: "fixture",
		Model:   "fixture-model",
	}
	manifest := runtimepkg.Manifest{
		GitIdentity:       gitx.Identity{Name: "CLI Test Engine", Email: "engine@example.test"},
		SchemaVersion:     runtimepkg.ManifestVersion,
		RunID:             "run-1",
		Repository:        repository,
		Release:           "release-1",
		TargetRef:         "refs/heads/main",
		Intent:            "Project one read-only board fixture.",
		MaxParallelTracks: 1,
		Authority: runtimepkg.ProjectAuthority{
			Project: "acme-repo", ExternalAuthorizer: "operator",
		},
		Driver: &runtimepkg.FakeDriverConfig{
			Executable: "/bin/true",
			Digest: "sha256:" +
				strings.Repeat("a", 64),
			AdapterKey: "fixture",
			Profile:    "fixture",
		},
		Roles: driver.RoleSelections{
			Planner:     profile,
			Implementer: profile,
			Captain:     profile,
			Verifier:    profile,
		},
		Automation: &runtimepkg.AutomationSelections{
			Recovery: profile,
		},
		Limits: driver.Limits{
			TimeoutMillis: 1,
			OutputBytes:   1,
		},
		Scripts: []runtimepkg.ScriptedAttempt{{
			Responsibility: driver.PlannerProposal,
			BatonAttempt:   1,
			Epoch:          1,
			Try:            1,
			Behavior:       "none",
		}},
	}
	manifestBody, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestBody = append(manifestBody, '\n')
	if _, err := runtimepkg.ParseManifest(manifestBody); err != nil {
		t.Fatalf("fixture manifest: %v", err)
	}
	sum := sha256.Sum256(manifestBody)
	manifestDigest := fmt.Sprintf("sha256:%x", sum)
	journalPath := filepath.Join(t.TempDir(), "run.sqlite")
	store, err := journal.Open(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 123).UTC()
	if err := store.RegisterRun(context.Background(), journal.Run{
		ID:             manifest.RunID,
		ManifestDigest: manifestDigest,
		Repository:     manifest.Repository,
		Release:        manifest.Release,
		TargetRef:      manifest.TargetRef,
		CreatedAt:      now,
	}); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.RecordCommand(context.Background(), journal.Command{
		RunID:     manifest.RunID,
		ReplayKey: "manifest",
		Kind:      "start",
		Payload:   manifestBody,
		CreatedAt: now,
	}); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return journalPath
}

func TestVersionRejectsEveryOtherShape(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"version", "--json", "--json"}, {"version", "--text"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Fatalf("run(%v) = %d, want 2", args, code)
		}
		if stdout.Len() != 0 || stderr.String() != "usage: sworn version [--json]\n" {
			t.Fatalf("run(%v) stdout = %q, stderr = %q", args, stdout.String(), stderr.String())
		}
	}
}
