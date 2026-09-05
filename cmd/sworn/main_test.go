package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

func TestRunDetachedRefusesWithCodedErrorAndLeavesNoClaim(t *testing.T) {
	manifestPath, journalPath := runManifestFixture(t, "run-detached-1")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"run",
		"--manifest", manifestPath,
		"--journal", journalPath,
		"--detached",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("sworn run --detached exit = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("sworn run --detached stdout = %q, want empty", stdout.String())
	}
	stderrStr := stderr.String()
	wantMessage := "sworn run: sworn run --detached is not supported; use sworn serve to host background runs\n"
	wantCode := "Technical code: DETACHED_UNSUPPORTED\n"
	if !strings.Contains(stderrStr, wantMessage) || !strings.Contains(stderrStr, wantCode) {
		t.Fatalf("sworn run --detached stderr = %q, want %q and %q", stderrStr, wantMessage, wantCode)
	}

	// Verify no owner claim or registered run was created in the journal.
	if _, err := os.Stat(journalPath); err == nil {
		store, err := journal.OpenReadOnly(context.Background(), journalPath)
		if err == nil {
			defer store.Close()
			owner, present, err := store.CurrentOwner(context.Background(), "run-detached-1")
			if err == nil && present {
				t.Fatalf("claimed owner lease remained after detached command refusal: %#v", owner)
			}
		}
	}
}

func buildFakeDriverBinary(t *testing.T) (string, string) {
	t.Helper()
	fakeBinary := filepath.Join(t.TempDir(), "fake-driver")
	cmd := exec.Command("go", "build", "-mod=readonly", "-buildvcs=false", "-trimpath", "-o", fakeBinary, "./test/e2e/testdata/fake")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = cleanEnvironment(map[string]string{
		"CGO_ENABLED": "0",
		"GOFLAGS":     "-buildvcs=false",
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
	})
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build fake driver: %v: %s", err, output)
	}
	body, err := os.ReadFile(fakeBinary)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	return fakeBinary, fmt.Sprintf("sha256:%x", sum)
}

func encodeSubmission(t *testing.T, submission driver.Submission) string {
	t.Helper()
	body, err := driver.EncodeSubmission(submission)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(body)
}

func hostedDrivePlanFixture(t *testing.T, release, repository string) ([]byte, baton.Plan) {
	t.Helper()
	planBytes := []byte(fmt.Sprintf("```baton-plan-v2\n{\n  \"schema_version\": \"baton.plan/v2\",\n  \"release\": %q,\n  \"revision\": 1,\n  \"previous_plan\": null,\n  \"repository\": \"acme-repo\",\n  \"target_ref\": \"refs/heads/main\",\n  \"approval_ref\": \"operator://%s/1\",\n  \"tracks\": [\n    {\n      \"id\": \"T1\",\n      \"depends_on\": [],\n      \"slices\": [\n        {\n          \"id\": \"S1\",\n          \"outcome\": \"Deliver S1.\",\n          \"scope\": {\n            \"include\": [\"one.txt\"],\n            \"exclude\": []\n          },\n          \"acceptance\": [\n            {\n              \"id\": \"A1\",\n              \"text\": \"S1 is complete.\"\n            }\n          ],\n          \"checks\": [\"true\"],\n          \"constraints\": [\"deterministic\"],\n          \"depends_on\": [],\n          \"consumes\": []\n        }\n      ]\n    }\n  ]\n}\n```\n\nFixture plan.\n", release, release))
	plan, err := baton.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	return planBytes, plan
}

func writeHostedDriveManifest(
	t *testing.T,
	directory, repository, release, runID, fakeExecutable, fakeDigest string,
	failImplementation bool,
) (string, baton.Plan) {
	t.Helper()
	planBytes, plan := hostedDrivePlanFixture(t, release, repository)
	var scripts []runtimepkg.ScriptedAttempt
	add := func(slice string, responsibility driver.Responsibility, batonAttempt int64) {
		for try := int64(1); try <= 3; try++ {
			work := slice
			if work == "" {
				work = "release"
			}
			submission := driver.Submission{
				SchemaVersion: driver.SubmissionSchemaVersion,
				InvocationID: fmt.Sprintf("%s/%s/%s/%d/1/%d", runID, work,
					responsibility, batonAttempt, try),
				Responsibility: responsibility,
				Summary:        "Exact " + string(responsibility) + ".",
				Detail:         "Fresh bounded test evidence.",
			}
			switch responsibility {
			case driver.PlannerProposal:
				submission.Plan, _ = driver.NewPlanBytes(planBytes)
			case driver.CaptainReview:
				submission.Decision, _ = driver.NewDecision(driver.DecisionProceed)
			case driver.ImplementerImplementation:
				submission.Checks, _ = driver.NewCheckBytes([]byte("implementation checks\n"))
			case driver.WorkVerification:
				submission.Checks, _ = driver.NewCheckBytes([]byte("fresh work checks\n"))
				submission.Decision, _ = driver.NewDecision(driver.DecisionPass)
			case driver.AssemblyVerification:
				submission.Checks, _ = driver.NewCheckBytes([]byte("fresh assembly checks\n"))
				submission.Decision, _ = driver.NewDecision(driver.DecisionPass)
			}
			behavior := "submit"
			subStr := encodeSubmission(t, submission)
			if responsibility == driver.ImplementerImplementation && failImplementation {
				behavior = "none"
				subStr = ""
			}
			scripts = append(scripts, runtimepkg.ScriptedAttempt{
				Slice:          slice,
				Responsibility: responsibility,
				BatonAttempt:   batonAttempt,
				Epoch:          1,
				Try:            try,
				Behavior:       behavior,
				Submission:     subStr,
			})
		}
	}
	add("", driver.PlannerProposal, 1)
	add("S1", driver.ImplementerDesign, 1)
	add("S1", driver.CaptainReview, 1)
	add("S1", driver.ImplementerImplementation, 1)
	add("S1", driver.WorkVerification, 1)
	add("", driver.AssemblyVerification, 1)
	sort.Slice(scripts, func(i, j int) bool {
		left := fmt.Sprintf("%s/%s/%020d/%020d/%d", scripts[i].Responsibility,
			scripts[i].Slice, scripts[i].BatonAttempt, scripts[i].Epoch, scripts[i].Try)
		right := fmt.Sprintf("%s/%s/%020d/%020d/%d", scripts[j].Responsibility,
			scripts[j].Slice, scripts[j].BatonAttempt, scripts[j].Epoch, scripts[j].Try)
		return left < right
	})
	manifest := runtimepkg.Manifest{
		GitIdentity:       gitx.Identity{Name: "CLI Test Engine", Email: "engine@example.test"},
		SchemaVersion:     runtimepkg.ManifestVersion,
		RunID:             runID,
		Repository:        repository,
		Release:           release,
		TargetRef:         "refs/heads/main",
		Intent:            "Drive the hosted CLI test track.",
		MaxParallelTracks: 1,
		Authority: runtimepkg.ProjectAuthority{
			Project: "acme-repo", ExternalAuthorizer: "operator",
		},
		Driver: &runtimepkg.FakeDriverConfig{
			Executable: fakeExecutable, Digest: fakeDigest,
			AdapterKey: "e2e-fake", Profile: "e2e-fake",
		},
		Roles: driver.RoleSelections{
			Planner:     driver.RoleSelection{Profile: "e2e-fake", Model: "planner-model"},
			Implementer: driver.RoleSelection{Profile: "e2e-fake", Model: "implementer-model"},
			Captain:     driver.RoleSelection{Profile: "e2e-fake", Model: "captain-model"},
			Verifier:    driver.RoleSelection{Profile: "e2e-fake", Model: "verifier-model"},
		},
		Automation: &runtimepkg.AutomationSelections{
			Recovery: driver.RoleSelection{Profile: "e2e-fake", Model: "recovery-model"},
		},
		Limits:  driver.Limits{TimeoutMillis: 30_000, OutputBytes: 65_536},
		Scripts: scripts,
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if _, err := runtimepkg.ParseManifest(body); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(directory, release+".json")
	if err := os.WriteFile(manifestPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return manifestPath, plan
}

func approveOfferToArgs(journalPath string, offer *runtimepkg.ApprovalOffer) []string {
	cmd := offer.Command
	releaseHead := cmd.ReleaseHead
	if releaseHead == "" {
		releaseHead = "absent"
	}
	priorPlan := cmd.PriorPlan
	if priorPlan == "" {
		priorPlan = "absent"
	}
	return []string{
		"approve",
		"--journal", journalPath,
		"--run", cmd.RunID,
		"--manifest-digest", cmd.ManifestDigest,
		"--project", cmd.Project,
		"--release", cmd.Release,
		"--release-ref", cmd.ReleaseRef,
		"--release-head", releaseHead,
		"--proposal-replay-key", cmd.ProposalReplayKey,
		"--plan-revision", fmt.Sprint(cmd.PlanRevision),
		"--prior-plan", priorPlan,
		"--plan-digest", cmd.PlanDigest,
		"--target-ref", cmd.TargetRef,
		"--target-head", cmd.TargetHead,
		"--decision-class", cmd.DecisionClass,
		"--decision", cmd.Decision,
		"--actor-class", cmd.ActorClass,
		"--actor-authority", cmd.ActorAuthority,
	}
}

func buildTestBinary(t *testing.T, output, source, ldflags string) {
	t.Helper()
	args := []string{
		"build", "-mod=readonly", "-buildvcs=false", "-trimpath",
	}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, "-o", output, source)
	command := exec.Command("go", args...)
	command.Dir = filepath.Join("..", "..")
	command.Env = cleanEnvironment(map[string]string{
		"CGO_ENABLED": "0",
		"GOFLAGS":     "-buildvcs=false",
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
	})
	outputBytes, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build binary %s: %v: %s", source, err, outputBytes)
	}
}

func runBinaryWithEnvironmentTimeout(
	t *testing.T,
	binary string,
	wantExit int,
	environment map[string]string,
	timeout time.Duration,
	args ...string,
) (string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...)
	overrides := map[string]string{}
	for key, value := range environment {
		overrides[key] = value
	}
	command.Env = cleanEnvironment(overrides)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf(
			"Sworn binary timed out\nstdout:\n%s\nstderr:\n%s",
			stdout.String(),
			stderr.String(),
		)
	}
	exit := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run Sworn: %v", err)
		}
		exit = exitErr.ExitCode()
	}
	if exit != wantExit {
		t.Fatalf(
			"sworn %v exit = %d, want %d\nstdout:\n%s\nstderr:\n%s",
			args,
			exit,
			wantExit,
			stdout.String(),
			stderr.String(),
		)
	}
	return stdout.String(), stderr.String()
}

func TestHostedDriveResumeAndTakeover(t *testing.T) {
	fakeBinary, fakeDigest := buildFakeDriverBinary(t)
	swornBinary := filepath.Join(t.TempDir(), "sworn")
	buildTestBinary(t, swornBinary, "./cmd/sworn", "-X=github.com/swornagent/sworn/internal/driver.testUncontainedDispatch=1 -X=github.com/swornagent/sworn/internal/runtime.testHooksFromEnv=1")
	env := map[string]string{
		"SWORN_TEST_UNCONTAINED_DISPATCH": "1",
	}

	// Test 1: Resume hosts drive to completion, writes durable status, exits 0, and all effects are journaled.
	t.Run("resume hosts to completion", func(t *testing.T) {
		root, _ := projectRepositoryFixture(t)
		manifestDir := filepath.Join(root, ".sworn", "runs")
		if err := os.MkdirAll(manifestDir, 0o700); err != nil {
			t.Fatal(err)
		}
		runID := "run-hosted-resume"
		release := "delivery-resume"
		manifestPath, _ := writeHostedDriveManifest(t, manifestDir, root, release, runID, fakeBinary, fakeDigest, false)
		journalPath := filepath.Join(root, ".sworn", "run.sqlite")

		// Step 1: Start run -> proposes plan, reaches awaiting_approval
		startOut, startErr := runBinaryWithEnvironmentTimeout(t, swornBinary, 0, env, 30*time.Second,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		if startErr != "" || !strings.Contains(startOut, "  state: awaiting_approval") {
			t.Fatalf("run start stdout = %q, stderr = %q", startOut, startErr)
		}

		// Step 2: Approve the plan
		statusReader, err := runtimepkg.OpenStatusService(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		status, err := statusReader.Status(context.Background(), runID)
		_ = statusReader.Close()
		if err != nil || status.ApprovalOffer == nil {
			t.Fatalf("status = %#v, err = %v", status, err)
		}
		approveArgs := approveOfferToArgs(journalPath, status.ApprovalOffer)
		approveOut, approveErr := runBinaryWithEnvironmentTimeout(t, swornBinary, 0, env, 30*time.Second, approveArgs...)
		if approveErr != "" {
			t.Fatalf("approve stderr = %q, stdout = %q", approveErr, approveOut)
		}

		// Step 3: Resume -> hosts background drive to completion and exits 0
		resumeOut, resumeErr := runBinaryWithEnvironmentTimeout(t, swornBinary, 0, env, 60*time.Second,
			"resume",
			"--run", runID,
			"--journal", journalPath,
			"--command", "resume-cmd-1",
			"--generation", "0",
		)
		if resumeErr != "" {
			t.Fatalf("resume stderr = %q", resumeErr)
		}

		// Verify stdout contains durable status
		if !strings.Contains(resumeOut, "Sworn run "+runID) || !strings.Contains(resumeOut, "Status:") {
			t.Fatalf("resume stdout missing durable status: %q", resumeOut)
		}

		// Verify all effects were journaled before process exited and state is complete
		store, err := journal.OpenReadOnly(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		snapshot, err := store.Snapshot(context.Background(), runID)
		if err != nil {
			t.Fatal(err)
		}

		hasMerge := false
		for _, effect := range snapshot.Effects {
			if effect.Kind == "baton.merge" && effect.State == journal.Succeeded {
				hasMerge = true
			}
		}
		if !hasMerge {
			t.Fatal("resume process exited before merge effect was journaled")
		}

		// Verify owner was cleanly released
		owner, present, err := store.CurrentOwner(context.Background(), runID)
		if err != nil {
			t.Fatal(err)
		}
		if present {
			t.Fatalf("claimed owner lease remained after completion: %#v", owner)
		}
	})

	// Test 2: Drive ending in run-level refusal still exits 0 with refusal left readable in run state
	t.Run("drive ending in refusal exits 0", func(t *testing.T) {
		root, _ := projectRepositoryFixture(t)
		manifestDir := filepath.Join(root, ".sworn", "runs")
		if err := os.MkdirAll(manifestDir, 0o700); err != nil {
			t.Fatal(err)
		}
		runID := "run-hosted-refusal"
		release := "delivery-refusal"
		manifestPath, _ := writeHostedDriveManifest(t, manifestDir, root, release, runID, fakeBinary, fakeDigest, true)
		journalPath := filepath.Join(root, ".sworn", "run.sqlite")

		// Start run
		startOut, startErr := runBinaryWithEnvironmentTimeout(t, swornBinary, 0, env, 30*time.Second,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		if startErr != "" || !strings.Contains(startOut, "  state: awaiting_approval") {
			t.Fatalf("run start stdout = %q, stderr = %q", startOut, startErr)
		}

		// Approve plan
		statusReader, err := runtimepkg.OpenStatusService(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		status, err := statusReader.Status(context.Background(), runID)
		_ = statusReader.Close()
		if err != nil || status.ApprovalOffer == nil {
			t.Fatalf("status = %#v, err = %v", status, err)
		}
		approveArgs := approveOfferToArgs(journalPath, status.ApprovalOffer)
		approveOut, approveErr := runBinaryWithEnvironmentTimeout(t, swornBinary, 0, env, 30*time.Second, approveArgs...)
		if approveErr != "" {
			t.Fatalf("approve stderr = %q, stdout = %q", approveErr, approveOut)
		}

		// Resume -> implementation fails and parks; command admission still exits 0!
		resumeOut, resumeErr := runBinaryWithEnvironmentTimeout(t, swornBinary, 0, env, 60*time.Second,
			"resume",
			"--run", runID,
			"--journal", journalPath,
			"--command", "resume-refusal-1",
			"--generation", "0",
		)
		if resumeErr != "" {
			t.Fatalf("resume with run-level refusal stderr = %q", resumeErr)
		}
		if !strings.Contains(resumeOut, "Sworn run "+runID) {
			t.Fatalf("resume stdout missing durable status: %q", resumeOut)
		}

		// Verify the refusal / parked state is left readable in status
		statusOut, statusErr := runBinaryWithEnvironmentTimeout(t, swornBinary, 0, env, 30*time.Second,
			"status", "--run", runID, "--journal", journalPath, "--json")
		if statusErr != "" {
			t.Fatalf("status stderr = %q", statusErr)
		}
		var finalStatus runtimepkg.RunStatus
		if err := json.Unmarshal([]byte(statusOut), &finalStatus); err != nil {
			t.Fatal(err)
		}
		if finalStatus.State != "parked" {
			t.Fatalf("finalStatus.State = %q, want 'parked'", finalStatus.State)
		}
	})

	// Test 3: Takeover hosts drive to completion
	t.Run("takeover hosts to completion", func(t *testing.T) {
		root, _ := projectRepositoryFixture(t)
		manifestDir := filepath.Join(root, ".sworn", "runs")
		if err := os.MkdirAll(manifestDir, 0o700); err != nil {
			t.Fatal(err)
		}
		runID := "run-hosted-takeover"
		release := "delivery-takeover"
		manifestPath, _ := writeHostedDriveManifest(t, manifestDir, root, release, runID, fakeBinary, fakeDigest, false)
		journalPath := filepath.Join(root, ".sworn", "run.sqlite")

		// Start run
		startOut, startErr := runBinaryWithEnvironmentTimeout(t, swornBinary, 0, env, 30*time.Second,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		if startErr != "" || !strings.Contains(startOut, "  state: awaiting_approval") {
			t.Fatalf("run start stdout = %q, stderr = %q", startOut, startErr)
		}

		// Approve plan
		statusReader, err := runtimepkg.OpenStatusService(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		status, err := statusReader.Status(context.Background(), runID)
		_ = statusReader.Close()
		if err != nil || status.ApprovalOffer == nil {
			t.Fatalf("status = %#v, err = %v", status, err)
		}
		approveArgs := approveOfferToArgs(journalPath, status.ApprovalOffer)
		approveOut, approveErr := runBinaryWithEnvironmentTimeout(t, swornBinary, 0, env, 30*time.Second, approveArgs...)
		if approveErr != "" {
			t.Fatalf("approve stderr = %q, stdout = %q", approveErr, approveOut)
		}

		// Simulate an expired owner lease from a previous dead host
		store, err := journal.Open(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		expiredTime := time.Now().UTC().Add(-10 * time.Second)
		if _, err := store.AcquireOwner(context.Background(), runID, expiredTime, 2*time.Second, false); err != nil {
			_ = store.Close()
			t.Fatal(err)
		}
		_ = store.Close()

		// Takeover -> hosts drive to completion and exits 0
		takeoverOut, takeoverErr := runBinaryWithEnvironmentTimeout(t, swornBinary, 0, env, 60*time.Second,
			"takeover",
			"--run", runID,
			"--journal", journalPath,
			"--command", "takeover-cmd-1",
			"--generation", "0",
		)
		if takeoverErr != "" {
			t.Fatalf("takeover stderr = %q", takeoverErr)
		}

		if !strings.Contains(takeoverOut, "Sworn run "+runID) {
			t.Fatalf("takeover stdout missing durable status: %q", takeoverOut)
		}

		// Verify merge effect is succeeded in journal
		store, err = journal.OpenReadOnly(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		snapshot, err := store.Snapshot(context.Background(), runID)
		if err != nil {
			t.Fatal(err)
		}
		hasMerge := false
		for _, effect := range snapshot.Effects {
			if effect.Kind == "baton.merge" && effect.State == journal.Succeeded {
				hasMerge = true
			}
		}
		if !hasMerge {
			t.Fatal("takeover process exited before merge effect was journaled")
		}
	})
}

func TestKilledHostMidDriveRecoversCleanly(t *testing.T) {
	fakeBinary, fakeDigest := buildFakeDriverBinary(t)
	swornBinary := filepath.Join(t.TempDir(), "sworn")
	buildTestBinary(t, swornBinary, "./cmd/sworn", "-X=github.com/swornagent/sworn/internal/driver.testUncontainedDispatch=1 -X=github.com/swornagent/sworn/internal/runtime.testHooksFromEnv=1")
	env := map[string]string{
		"SWORN_TEST_UNCONTAINED_DISPATCH": "1",
	}

	root, _ := projectRepositoryFixture(t)
	manifestDir := filepath.Join(root, ".sworn", "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	runID := "run-killed-host"
	release := "delivery-killed"
	manifestPath, _ := writeHostedDriveManifest(t, manifestDir, root, release, runID, fakeBinary, fakeDigest, false)
	journalPath := filepath.Join(root, ".sworn", "run.sqlite")

	// Start run -> proposes plan, reaches awaiting_approval
	startOut, startErr := runBinaryWithEnvironmentTimeout(t, swornBinary, 0, env, 30*time.Second,
		"run", "--manifest", manifestPath, "--journal", journalPath)
	if startErr != "" || !strings.Contains(startOut, "  state: awaiting_approval") {
		t.Fatalf("run start stdout = %q, stderr = %q", startOut, startErr)
	}

	// Pause run so approve does not automatically wake and drive to completion before our crash test
	pauseOut, pauseErr := runBinaryWithEnvironmentTimeout(t, swornBinary, 0, env, 30*time.Second,
		"pause", "--run", runID, "--journal", journalPath, "--command", "pause-1", "--generation", "0")
	if pauseErr != "" {
		t.Fatalf("pause stderr = %q, stdout = %q", pauseErr, pauseOut)
	}

	// Approve plan
	statusReader, err := runtimepkg.OpenStatusService(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	status, err := statusReader.Status(context.Background(), runID)
	_ = statusReader.Close()
	if err != nil || status.ApprovalOffer == nil {
		t.Fatalf("status = %#v, err = %v", status, err)
	}
	approveArgs := approveOfferToArgs(journalPath, status.ApprovalOffer)
	approveOut, approveErr := runBinaryWithEnvironmentTimeout(t, swornBinary, 0, env, 30*time.Second, approveArgs...)
	if approveErr != "" {
		t.Fatalf("approve stderr = %q, stdout = %q", approveErr, approveOut)
	}

	// Simulate host death: start resume with short lease and crash cut after baton.append_receipt
	crashEnv := map[string]string{
		"SWORN_TEST_UNCONTAINED_DISPATCH": "1",
		"SWORN_TEST_CRASH_AFTER_EFFECT":   "baton.append_receipt",
		"SWORN_TEST_OWNER_LEASE_MILLIS":   "500",
	}
	runBinaryWithEnvironmentTimeout(t, swornBinary, 86, crashEnv, 30*time.Second,
		"resume",
		"--run", runID,
		"--journal", journalPath,
		"--command", "resume-crash-1",
		"--generation", "1",
	)

	// Wait out lease expiration
	time.Sleep(2000 * time.Millisecond)

	// Verify takeover can recover the run cleanly and host it to completion
	takeoverOut, takeoverErr := runBinaryWithEnvironmentTimeout(t, swornBinary, 0, env, 60*time.Second,
		"takeover",
		"--run", runID,
		"--journal", journalPath,
		"--command", "takeover-recover-1",
		"--generation", "2",
	)
	if takeoverErr != "" {
		t.Fatalf("takeover recovery stderr = %q, stdout = %q", takeoverErr, takeoverOut)
	}

	// Verify run reached completion and effect cardinalities are exact
	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot, err := store.Snapshot(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}

	// Assert exactly one merge effect succeeded
	merges := 0
	for _, effect := range snapshot.Effects {
		if effect.Kind == "baton.merge" && effect.State == journal.Succeeded {
			merges++
		}
	}
	if merges != 1 {
		t.Fatalf("succeeded merge effects = %d, want 1", merges)
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

func TestTakeoverDuringUnexpiredLeaseExitsNonZeroWithActionableWait(t *testing.T) {
	journalPath := boardJournalFixture(t)
	store, err := journal.Open(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := store.AcquireOwner(context.Background(), "run-1", now, 30*time.Second, false); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"takeover",
		"--run", "run-1",
		"--journal", journalPath,
		"--command", "takeover-1",
		"--generation", "0",
	}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("takeover exit = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout was not empty: %q", stdout.String())
	}
	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "sworn takeover: The previous Sworn process has not released its owner lease yet. Wait ") {
		t.Fatalf("stderr missing actionable wait message: %q", stderrStr)
	}
	if !strings.Contains(stderrStr, "Technical code: OWNER_TRANSITION_PENDING") {
		t.Fatalf("stderr missing technical code: %q", stderrStr)
	}
}
