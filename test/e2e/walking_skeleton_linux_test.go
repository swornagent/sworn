//go:build linux

package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

const e2eGit = "/usr/bin/git"

func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func cleanEnvironment(overrides map[string]string) []string {
	result := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, replaced := overrides[key]; replaced {
				continue
			}
		}
		result = append(result, entry)
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

func buildBinary(t *testing.T, output, source, ldflags string) {
	t.Helper()
	args := []string{
		"build", "-mod=readonly", "-buildvcs=false", "-trimpath",
		"-o", output,
	}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, source)
	command := exec.Command(filepath.Join(stdruntime.GOROOT(), "bin", "go"), args...)
	command.Dir = moduleRoot(t)
	command.Env = cleanEnvironment(map[string]string{
		"CGO_ENABLED": "0",
		"GOFLAGS":     "-buildvcs=false",
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
	})
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", source, err, output)
	}
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func runBinary(
	t *testing.T,
	binary string,
	wantExit int,
	args ...string,
) (string, string) {
	t.Helper()
	return runBinaryWithEnvironment(
		t,
		binary,
		wantExit,
		nil,
		args...,
	)
}

func runBinaryWithEnvironment(
	t *testing.T,
	binary string,
	wantExit int,
	environment map[string]string,
	args ...string,
) (string, string) {
	t.Helper()
	return runBinaryWithEnvironmentTimeout(
		t, binary, wantExit, environment, 120*time.Second, args...,
	)
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

func runGit(t *testing.T, repository string, args ...string) string {
	t.Helper()
	command := exec.Command(e2eGit, append([]string{"-C", repository}, args...)...)
	command.Env = cleanEnvironment(map[string]string{
		"LANG": "C", "LC_ALL": "C",
		"GIT_AUTHOR_NAME": "Fixture", "GIT_AUTHOR_EMAIL": "fixture@example.invalid",
		"GIT_COMMITTER_NAME": "Fixture", "GIT_COMMITTER_EMAIL": "fixture@example.invalid",
		"GIT_AUTHOR_DATE": "1700000000 +0000", "GIT_COMMITTER_DATE": "1700000000 +0000",
	})
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func newProductRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	command := exec.Command(
		e2eGit, "init", "--quiet", "--initial-branch=main", repository,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	if err := os.WriteFile(
		filepath.Join(repository, "base.txt"),
		[]byte("base\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "--", "base.txt")
	runGit(t, repository, "commit", "--quiet", "-m", "base")
	return repository
}

func e2ePlan(
	t *testing.T,
	release, repository string,
) ([]byte, baton.Plan) {
	t.Helper()
	slice := func(id, path string) baton.Slice {
		return baton.Slice{
			ID: id, Outcome: "Deliver " + id + ".",
			Scope:      baton.Scope{Include: []string{path}, Exclude: []string{}},
			Acceptance: []baton.Criterion{{ID: "A-" + id, Text: id + " is exact."}},
			Checks:     []string{"check " + id}, Constraints: []string{"deterministic"},
			DependsOn: []string{}, Consumes: []string{},
		}
	}
	metadata := baton.Metadata{
		SchemaVersion: baton.PlanVersion, Release: release, Revision: 1,
		PreviousPlan: nil, Repository: "acme-repo",
		TargetRef:   "refs/heads/main",
		ApprovalRef: "operator://" + release + "/1",
		Tracks: []baton.Track{
			{ID: "T1", DependsOn: []string{}, Slices: []baton.Slice{slice("S1", "one.txt")}},
			{ID: "T2", DependsOn: []string{}, Slices: []baton.Slice{slice("S2", "two.txt")}},
		},
	}
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(
		"```baton-plan-v2\n" + string(metadataBody) +
			"\n```\n\nReal-binary E2E plan for " + repository + ".\n",
	)
	plan, err := baton.ParsePlan(body)
	if err != nil {
		t.Fatal(err)
	}
	return body, plan
}

func encodedSubmission(t *testing.T, submission driver.Submission) string {
	t.Helper()
	body, err := driver.EncodeSubmission(submission)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(body)
}

func e2eManifest(
	t *testing.T,
	runID, repository, release string,
	fakeExecutable, fakeDigest, verifierModel string,
) ([]byte, []byte, baton.Plan) {
	t.Helper()
	planBytes, plan := e2ePlan(t, release, repository)
	var scripts []swornruntime.ScriptedAttempt
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
				Detail:         "Fresh bounded E2E evidence.",
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
			scripts = append(scripts, swornruntime.ScriptedAttempt{Slice: slice,
				Responsibility: responsibility, BatonAttempt: batonAttempt, Epoch: 1,
				Try: try, Behavior: "submit", Submission: encodedSubmission(t, submission)})
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
	manifest := swornruntime.Manifest{
		GitIdentity:   gitx.Identity{Name: "E2E Engine", Email: "engine@example.test"},
		SchemaVersion: swornruntime.ManifestVersion,
		RunID:         runID, Repository: repository, Release: release,
		TargetRef: "refs/heads/main", Intent: "Drive the exact approved E2E track.",
		MaxParallelTracks: 2,
		Authority: swornruntime.ProjectAuthority{
			Project: "acme-repo", ExternalAuthorizer: "operator",
		},
		Driver: &swornruntime.FakeDriverConfig{
			Executable: fakeExecutable, Digest: fakeDigest,
			AdapterKey: "e2e-fake", Profile: "e2e-fake",
		},
		Roles: driver.RoleSelections{
			Planner:     driver.RoleSelection{Profile: "e2e-fake", Model: "planner-model"},
			Implementer: driver.RoleSelection{Profile: "e2e-fake", Model: "implementer-model"},
			Captain:     driver.RoleSelection{Profile: "e2e-fake", Model: "captain-model"},
			Verifier:    driver.RoleSelection{Profile: "e2e-fake", Model: verifierModel},
		},
		Automation: &swornruntime.AutomationSelections{
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
	if _, err := swornruntime.ParseManifest(body); err != nil {
		t.Fatal(err)
	}
	return body, planBytes, plan
}

func authorizePlan(
	t *testing.T,
	journalPath, runID string,
	plan baton.Plan,
) {
	t.Helper()
	store, err := journal.Open(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	payload, err := json.Marshal(struct {
		Version    string `json:"version"`
		PlanDigest string `json:"plan_digest"`
	}{Version: "sworn.plan-authority/v1", PlanDigest: plan.Digest()})
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	if err := store.RecordCommand(context.Background(), journal.Command{
		RunID:     runID,
		ReplayKey: "plan-authority/" + strings.TrimPrefix(plan.Digest(), "sha256:"),
		Kind:      "plan_authority", Payload: payload, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

func inertResolver(
	request gitx.RecordRootRequest,
) (gitx.RecordRootDecision, error) {
	return gitx.RecordRootDecision{
		Kind: request.Kind, Repository: request.Repository,
		RecordRoot: request.RecordRoot, Commit: request.Commit,
		Decision: "inert",
	}, nil
}

func installAndPassComponent(
	t *testing.T,
	repositoryPath, release string,
	planBytes []byte,
) {
	t.Helper()
	installApprovedPlan(t, repositoryPath, planBytes)
	repository, err := gitx.Open(repositoryPath, e2eGit)
	if err != nil {
		t.Fatal(err)
	}
	gitRepository := baton.UseGitRepository(repository)
	actions, err := baton.NewActions(gitRepository, inertResolver, gitx.Identity{Name: "E2E Engine", Email: "engine@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []baton.AppendReceiptInput{
		{
			Release: release, Slice: "S2", Role: "implementer", Result: "designed",
			Summary: "Design disjoint component.", Detail: []byte("Component design."),
		},
		{
			Release: release, Slice: "S2", Role: "captain", Result: "proceed",
			Summary: "Proceed with disjoint component.", Detail: []byte("Distinct component Captain."),
		},
	} {
		if _, err := actions.AppendReceipt(input); err != nil {
			t.Fatal(err)
		}
	}
	workspaces, err := gitx.NewWorkspaces(
		repository,
		gitx.Identity{Name: "E2E Engine", Email: "engine@example.test"},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer workspaces.Close()
	key := gitx.TrackKey{Release: release, Track: "T2"}
	workspace, err := workspaces.OpenTrack(key, gitx.ImplementationView)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(workspace.Path(), "two.txt"),
		[]byte("component track\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	sealed, err := workspaces.SealTrack(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.Close(); err != nil {
		t.Fatal(err)
	}
	plan, err := baton.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := baton.ValidateSliceCandidateScope(
		gitRepository,
		inertResolver,
		plan,
		"S2",
		sealed.Before.String(),
		sealed.Candidate.String(),
	); err != nil {
		t.Fatal(err)
	}
	for _, input := range []baton.AppendReceiptInput{
		{
			Release: release, Slice: "S2", Role: "implementer", Result: "candidate",
			Summary: "Seal disjoint component.", Detail: []byte("Component implementation."),
			Candidate: sealed.Candidate.String(), CheckResults: []byte("component checks\n"),
		},
		{
			Release: release, Slice: "S2", Role: "verifier", Result: "pass",
			Summary: "Pass disjoint component.", Detail: []byte("Fresh component verification."),
			Candidate: sealed.Candidate.String(), CheckResults: []byte("fresh component checks\n"),
		},
	} {
		if _, err := actions.AppendReceipt(input); err != nil {
			t.Fatal(err)
		}
	}
}

func installApprovedPlan(
	t *testing.T,
	repositoryPath string,
	planBytes []byte,
) {
	t.Helper()
	repository, err := gitx.Open(repositoryPath, e2eGit)
	if err != nil {
		t.Fatal(err)
	}
	actions, err := baton.NewActions(
		baton.UseGitRepository(repository),
		inertResolver,
		gitx.Identity{Name: "E2E Engine", Email: "engine@example.test"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := actions.RecordPlanRevision(baton.RecordPlanRevisionInput{
		PlanBytes: planBytes,
		Summary:   "Install the externally published E2E approval.",
		Detail:    []byte("Test-only authority fixture after protected approval publication."),
	}); err != nil {
		t.Fatal(err)
	}
}

func readBatonState(t *testing.T, repositoryPath, release string) baton.State {
	t.Helper()
	repository, err := gitx.Open(repositoryPath, e2eGit)
	if err != nil {
		t.Fatal(err)
	}
	state, err := baton.ReadState(
		baton.UseGitRepository(repository),
		release,
		inertResolver,
	)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func writeManifest(t *testing.T, root string, body []byte) string {
	t.Helper()
	path := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertDispatchOrder(t *testing.T, journalPath, runID string) {
	t.Helper()
	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot, err := store.Snapshot(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, event := range snapshot.Events {
		if event.Kind == "dispatch_completed" {
			got = append(got, string(event.Body))
		}
	}
	want := []string{
		string(driver.PlannerProposal),
		string(driver.ImplementerDesign),
		string(driver.CaptainReview),
		string(driver.ImplementerImplementation),
		string(driver.WorkVerification),
		string(driver.AssemblyVerification),
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("dispatch order = %v, want %v", got, want)
	}
}

func runRealBinaryWalkingSkeletonRecoveryAndTransportTruth(t *testing.T) {
	buildRoot := t.TempDir()
	fakeBinary := filepath.Join(buildRoot, "e2e-fake")
	buildBinary(t, fakeBinary, "./test/e2e/testdata/fake", "")
	fakeDigest := fileDigest(t, fakeBinary)
	swornBinary := filepath.Join(buildRoot, "sworn")
	buildBinary(t, swornBinary, "./cmd/sworn", "")

	t.Run("rerun_replaces_stale_initial_proposal_at_same_revision", func(t *testing.T) {
		repository := newProductRepository(t)
		runRoot := t.TempDir()
		journalPath := filepath.Join(runRoot, "run.sqlite")
		const (
			runID   = "e2e-proposal-drift"
			release = "e2e-proposal-drift-release"
		)
		manifestBody, _, _ := e2eManifest(
			t,
			runID,
			repository,
			release,

			fakeBinary,
			fakeDigest,
			"verifier-model")

		manifestPath := writeManifest(t, runRoot, manifestBody)
		runBinary(
			t,
			swornBinary,
			0,
			"run",
			"--manifest",
			manifestPath,
			"--journal",
			journalPath,
		)
		if err := os.WriteFile(
			filepath.Join(repository, "proposal-drift.txt"),
			[]byte("new target authority\n"),
			0o644,
		); err != nil {
			t.Fatal(err)
		}
		runGit(t, repository, "add", "--", "proposal-drift.txt")
		runGit(t, repository, "commit", "--quiet", "-m", "move proposal target")
		stdout, _ := runBinary(
			t,
			swornBinary,
			0,
			"run",
			"--manifest",
			manifestPath,
			"--journal",
			journalPath,
		)
		if !strings.Contains(stdout, "  state: awaiting_approval") {
			t.Fatalf("replacement proposal status = %q", stdout)
		}
		store, err := journal.OpenReadOnly(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := store.Snapshot(context.Background(), runID)
		_ = store.Close()
		if err != nil {
			t.Fatal(err)
		}
		proposals := 0
		plannerEffects := make(map[string]struct{})
		installEffects := 0
		for _, command := range snapshot.Commands {
			if command.Kind == "planner_proposal" {
				proposals++
			}
		}
		for _, effect := range snapshot.Effects {
			if effect.Kind == "baton.install" {
				installEffects++
			}
			if effect.Kind != "driver.dispatch" ||
				effect.State != journal.Succeeded {
				continue
			}
			submission, decodeErr := driver.DecodeSubmission(effect.Result)
			if decodeErr == nil &&
				submission.Responsibility == driver.PlannerProposal {
				plannerEffects[effect.ID] = struct{}{}
			}
		}
		if proposals != 2 || len(plannerEffects) != 2 || installEffects != 0 {
			t.Fatalf(
				"replacement evidence: proposals=%d planners=%d installs=%d",
				proposals, len(plannerEffects), installEffects)
		}
	})

	t.Run("real_binary_cli_approval_replays_one_admission_install_and_continuation", func(t *testing.T) {
		repository := newProductRepository(t)
		runRoot := t.TempDir()
		journalPath := filepath.Join(runRoot, "run.sqlite")
		const (
			runID   = "e2e-cli-approval"
			release = "e2e-cli-approval-release"
		)
		manifestBody, _, _ := e2eManifest(
			t, runID, repository, release, fakeBinary, fakeDigest, "verifier-model",
		)
		manifestPath := writeManifest(t, runRoot, manifestBody)
		runBinary(t, swornBinary, 0, "run", "--manifest", manifestPath, "--journal", journalPath)
		statusBody, _ := runBinary(
			t, swornBinary, 0, "status", "--run", runID,
			"--journal", journalPath, "--json",
		)
		var status swornruntime.RunStatus
		if err := json.Unmarshal([]byte(statusBody), &status); err != nil || status.ApprovalOffer == nil {
			t.Fatalf("approval offer = %#v, err=%v, body=%s", status.ApprovalOffer, err, statusBody)
		}
		approval := status.ApprovalOffer.Command
		absent := func(value string) string {
			if value == "" {
				return "absent"
			}
			return value
		}
		approveArgs := []string{
			"approve", "--journal", journalPath,
			"--run", approval.RunID,
			"--manifest-digest", approval.ManifestDigest,
			"--project", approval.Project,
			"--release", approval.Release,
			"--release-ref", approval.ReleaseRef,
			"--release-head", absent(approval.ReleaseHead),
			"--proposal-replay-key", approval.ProposalReplayKey,
			"--plan-revision", fmt.Sprintf("%d", approval.PlanRevision),
			"--prior-plan", absent(approval.PriorPlan),
			"--plan-digest", approval.PlanDigest,
			"--target-ref", approval.TargetRef,
			"--target-head", approval.TargetHead,
			"--decision-class", approval.DecisionClass,
			"--decision", approval.Decision,
			"--actor-class", approval.ActorClass,
			"--actor-authority", approval.ActorAuthority,
		}
		first, firstErr := runBinary(t, swornBinary, 0, approveArgs...)
		second, secondErr := runBinary(t, swornBinary, 0, approveArgs...)
		if firstErr != "" || secondErr != "" || first != second {
			t.Fatalf("approval replay drift: first=%q/%q second=%q/%q", first, firstErr, second, secondErr)
		}
		store, err := journal.OpenReadOnly(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := store.Snapshot(context.Background(), runID)
		_ = store.Close()
		if err != nil {
			t.Fatal(err)
		}
		admissions, installs, continuations := 0, 0, 0
		for _, effect := range snapshot.Effects {
			if effect.Kind == "approval.admit" && effect.State == journal.Succeeded {
				admissions++
			}
			if effect.Kind == "baton.install" && effect.State == journal.Succeeded {
				installs++
			}
			if effect.Kind == "driver.dispatch" && effect.State == journal.Succeeded {
				submission, decodeErr := driver.DecodeSubmission(effect.Result)
				if decodeErr == nil && submission.Responsibility == driver.ImplementerDesign {
					continuations++
				}
			}
		}
		if admissions != 1 || installs != 1 || continuations != 1 {
			t.Fatalf("durable authority effects: admissions=%d installs=%d continuations=%d", admissions, installs, continuations)
		}
	})

	t.Run("complete_non_direct_flow", func(t *testing.T) {
		repository := newProductRepository(t)
		runRoot := t.TempDir()
		journalPath := filepath.Join(runRoot, "run.sqlite")
		const (
			runID   = "e2e-complete"
			release = "e2e-complete-release"
		)
		manifestBody, planBytes, plan := e2eManifest(
			t,
			runID,
			repository,
			release,

			fakeBinary,
			fakeDigest,
			"verifier-model")

		manifestPath := writeManifest(t, runRoot, manifestBody)
		targetBefore := runGit(t, repository, "rev-parse", "main")
		stdout, _ := runBinary(
			t,
			swornBinary,
			0,
			"run",
			"--manifest",
			manifestPath,
			"--journal",
			journalPath,
		)
		if !strings.Contains(stdout, "  state: awaiting_approval") {
			t.Fatalf("planner output = %q", stdout)
		}
		if targetAfter := runGit(t, repository, "rev-parse", "main"); targetAfter != targetBefore {
			t.Fatal("planner pause moved the target")
		}
		for _, ref := range []string{
			"refs/heads/release-wt/" + release,
			"refs/heads/track/" + release + "/T1",
			"refs/heads/track/" + release + "/T2",
		} {
			command := exec.Command(e2eGit, "-C", repository, "show-ref", "--verify", "--quiet", ref)
			if err := command.Run(); err == nil {
				t.Fatalf("planner pause created authority ref %s", ref)
			}
		}
		authorizePlan(t, journalPath, runID, plan)
		installAndPassComponent(t, repository, release, planBytes)
		stdout, stderr := runBinary(
			t,
			swornBinary,
			0,
			"resume",
			"--run",
			runID,
			"--journal",
			journalPath,
			"--command",
			"resume-1",
			"--generation",
			"0",
		)
		if stderr != "" || !strings.Contains(stdout, "  state: complete") {
			t.Fatalf("resume stdout = %q, stderr = %q", stdout, stderr)
		}
		state := readBatonState(t, repository, release)
		if state.Assembly.Outcome != "merged" ||
			state.Assembly.Candidate == nil ||
			state.Assembly.Pass == nil ||
			state.Assembly.ResultCommit == "" {
			t.Fatalf("assembly state = %#v", state.Assembly)
		}
		target := runGit(t, repository, "rev-parse", "main")
		if target != state.Assembly.ResultCommit {
			t.Fatalf("target = %s, ResultCommit = %s", target, state.Assembly.ResultCommit)
		}
		if got := runGit(t, repository, "show", "main:one.txt"); got != "active track" {
			t.Fatalf("active product = %q", got)
		}
		if got := runGit(t, repository, "show", "main:two.txt"); got != "component track" {
			t.Fatalf("component product = %q", got)
		}
		if state.Assembly.Candidate.OID == state.Assembly.Pass.OID {
			t.Fatal("assembly candidate and fresh PASS receipts collapsed")
		}
		assertDispatchOrder(t, journalPath, runID)
	})

	t.Run("post_merge_crash_reconstructs_all_new", func(t *testing.T) {
		crashBinary := filepath.Join(buildRoot, "sworn-crash")
		buildBinary(
			t,
			crashBinary,
			"./cmd/sworn",
			"-X=github.com/swornagent/sworn/internal/runtime.testCrashAfterEffect=baton.merge"+
				" -X=github.com/swornagent/sworn/internal/runtime.testOwnerLeaseMillis=1500",
		)
		repository := newProductRepository(t)
		runRoot := t.TempDir()
		journalPath := filepath.Join(runRoot, "run.sqlite")
		const (
			runID   = "e2e-crash"
			release = "e2e-crash-release"
		)
		manifestBody, planBytes, plan := e2eManifest(
			t, runID, repository, release,
			fakeBinary, fakeDigest, "verifier-model")

		manifestPath := writeManifest(t, runRoot, manifestBody)
		runBinary(
			t, crashBinary, 0, "run", "--manifest", manifestPath, "--journal", journalPath,
		)
		authorizePlan(t, journalPath, runID, plan)
		installAndPassComponent(t, repository, release, planBytes)
		runBinary(
			t, crashBinary, 86, "resume", "--run", runID, "--journal", journalPath,
			"--command", "resume-1", "--generation", "0",
		)
		stateAfterCrash := readBatonState(t, repository, release)
		if stateAfterCrash.Assembly.Outcome != "merged" ||
			runGit(t, repository, "rev-parse", "main") != stateAfterCrash.Assembly.ResultCommit {
			t.Fatal("post-effect crash did not leave exact all-new Git state")
		}
		store, err := journal.OpenReadOnly(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		snapshot, snapshotErr := store.Snapshot(context.Background(), runID)
		var mergeEffect journal.Effect
		for _, effect := range snapshot.Effects {
			if effect.Kind == "baton.merge" {
				mergeEffect = effect
			}
		}
		_ = store.Close()
		if snapshotErr != nil || mergeEffect.State != journal.Claimed {
			t.Fatalf("crash-cut merge effect = %#v, err = %v", mergeEffect, snapshotErr)
		}
		time.Sleep(1800 * time.Millisecond)
		stdout, _ := runBinary(
			t, crashBinary, 0, "takeover", "--run", runID, "--journal", journalPath,
			"--command", "takeover-1", "--generation", "1",
		)
		if !strings.Contains(stdout, "  state: complete") {
			t.Fatalf("recovered resume = %q", stdout)
		}
	})

	t.Run("transport_failure_creates_no_verdict", func(t *testing.T) {
		repository := newProductRepository(t)
		runRoot := t.TempDir()
		journalPath := filepath.Join(runRoot, "run.sqlite")
		const (
			runID   = "e2e-transport"
			release = "e2e-transport-release"
		)
		manifestBody, planBytes, plan := e2eManifest(
			t, runID, repository, release,
			fakeBinary, fakeDigest, "transport-fail")

		manifestPath := writeManifest(t, runRoot, manifestBody)
		runBinary(
			t, swornBinary, 0, "run", "--manifest", manifestPath, "--journal", journalPath,
		)
		authorizePlan(t, journalPath, runID, plan)
		installAndPassComponent(t, repository, release, planBytes)
		_, stderr := runBinary(
			t, swornBinary, 0, "resume", "--run", runID, "--journal", journalPath,
			"--command", "resume-1", "--generation", "0",
		)
		if stderr != "" {
			t.Fatalf("transport failure stderr = %q", stderr)
		}
		status, _ := runBinary(
			t,
			swornBinary,
			0,
			"status",
			"--run",
			runID,
			"--journal",
			journalPath,
			"--json",
		)
		if !strings.Contains(status, `"state": "parked"`) {
			t.Fatalf("transport status = %s", status)
		}
		state := readBatonState(t, repository, release)
		slice, ok := state.Slice("S1")
		if !ok || slice.Pass != nil || slice.CurrentReceipt == nil ||
			slice.CurrentReceipt.Receipt.Role != "implementer" ||
			slice.CurrentReceipt.Receipt.Result != "candidate" {
			t.Fatalf("transport failure created a Baton verdict: %#v", slice)
		}
		if state.Assembly.Candidate != nil || state.Assembly.Pass != nil {
			t.Fatalf("transport failure advanced assembly: %#v", state.Assembly)
		}
	})
}
