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
	if got.RoleAssets.RoleAssetsVersion != baton.RoleAssetsVersion ||
		got.RoleAssets.LegacyBatonVersion != baton.LegacyBatonVersion ||
		got.RoleAssets.ManifestSHA256 != baton.ManifestSHA256 ||
		got.RoleAssets.AssetCount != baton.AssetCount ||
		got.RoleAssets.AssetBytes != baton.AssetBytes {
		t.Fatalf("role-asset identity = %#v", got.RoleAssets)
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
	want := "Sworn 1.0.0-rc.2-dev\n\n" +
		"Technical details:\n" +
		"  state: role-assets-admitted\n" +
		"  role assets: " + baton.RoleAssetsVersion + "\n" +
		"  legacy Baton content: " + baton.LegacyBatonVersion + "\n"
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

func TestCommandErrorCodeResolvesBatonRecordErrorsAndParity(t *testing.T) {
	t.Parallel()

	// A3: CLI error code resolution covers baton record errors and parity across types
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "baton record error",
			err:  &baton.RecordError{Code: "TARGET_DIVERGED", Msg: "target diverged"},
			want: "TARGET_DIVERGED",
		},
		{
			name: "journal error",
			err:  &journal.Error{Code: "STALE_RETRY_EPOCH"},
			want: "STALE_RETRY_EPOCH",
		},
		{
			name: "runtime error",
			err:  &runtimepkg.Error{Code: "INVALID_RUN"},
			want: "INVALID_RUN",
		},
		{
			name: "gitx error",
			err:  &gitx.Error{Code: "AUTHORITY_MOVED"},
			want: "AUTHORITY_MOVED",
		},
		{
			name: "driver contract error",
			err:  &driver.ContractError{Code: "UNCONTAINED_DISPATCH_REFUSED"},
			want: "UNCONTAINED_DISPATCH_REFUSED",
		},
		{
			name: "cockpit error",
			err:  &cockpit.Error{Code: "BOARD_FAILED"},
			want: "BOARD_FAILED",
		},
		{
			name: "uncoded error",
			err:  fmt.Errorf("generic operational error"),
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := commandErrorCode(tc.err)
			if got != tc.want {
				t.Fatalf("commandErrorCode(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}

	// Test writeCommandFailure prints technical code for baton record error
	var out bytes.Buffer
	writeCommandFailure(&out, "status", "Could not find that run in the saved record.", &baton.RecordError{Code: "TARGET_DIVERGED"})
	outStr := out.String()
	if !strings.Contains(outStr, "Technical code: TARGET_DIVERGED") {
		t.Fatalf("writeCommandFailure output missing technical code: %q", outStr)
	}
}

func runManifestFixture(t *testing.T, runID string) (string, string) {
	t.Helper()
	root, _ := projectRepositoryFixture(t)
	installProjectPlan(t, root, "delivery")

	stateDir := filepath.Join(root, ".sworn")
	manifestDir := filepath.Join(stateDir, "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projectWriteManifest(t, manifestDir, "delivery.json", root, "delivery", runID)
	manifestPath := filepath.Join(manifestDir, "delivery.json")
	journalPath := filepath.Join(stateDir, "run.sqlite")
	return manifestPath, journalPath
}

func TestRunDetachedPrintsWatchGuidanceAndExitsZero(t *testing.T) {
	manifestPath, journalPath := runManifestFixture(t, "run-detached-1")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"run",
		"--manifest", manifestPath,
		"--journal", journalPath,
		"--detached",
	}, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("sworn run --detached exit = %d, stderr = %q", code, stderr.String())
	}
	wantGuidance := fmt.Sprintf(
		"Sworn run run-detached-1 started detached.\n\n"+
			"Watch progress:\n"+
			"  sworn board --run run-detached-1 --journal %s\n"+
			"  sworn tui\n",
		journalPath,
	)
	if stdout.String() != wantGuidance {
		t.Fatalf("sworn run --detached stdout = %q, want %q", stdout.String(), wantGuidance)
	}

	// Verify the run was registered and left in a clean resumable state.
	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	binding, err := store.RunBinding(context.Background(), "run-detached-1")
	if err != nil || binding.ID != "run-detached-1" {
		t.Fatalf("registered binding = %#v, %v", binding, err)
	}
	owner, present, err := store.CurrentOwner(context.Background(), "run-detached-1")
	if err != nil {
		t.Fatal(err)
	}
	if present {
		t.Fatalf("claimed owner lease remained after detached command exit: %#v", owner)
	}
}

func TestRunForegroundExecutesAndPrintsStatus(t *testing.T) {
	manifestPath, journalPath := runManifestFixture(t, "run-fg-1")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"run",
		"--manifest", manifestPath,
		"--journal", journalPath,
	}, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("sworn run exit = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Sworn run run-fg-1\nStatus:") {
		t.Fatalf("sworn run stdout = %q", stdout.String())
	}
}

func TestRunDetachedRejectsDuplicateAndInvalidShapes(t *testing.T) {
	for _, args := range [][]string{
		{"run", "--detached", "--detached"},
		{"run", "--detached", "--unknown"},
		{"run", "--manifest", "/manifest.json", "--detached", "--detached"},
	} {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("run(%v) = %d, want 2", args, code)
		}
		if !strings.Contains(stderr.String(), "usage: sworn run") || !strings.Contains(stderr.String(), "[--detached]") {
			t.Fatalf("run(%v) stderr = %q", args, stderr.String())
		}
	}
}
