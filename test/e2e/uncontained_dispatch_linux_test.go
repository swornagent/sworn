//go:build linux

package e2e

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

// workerSandboxArguments returns the /usr/bin/bwrap command line that creates
// the outer worker sandbox for the orchestration subset. It mirrors the
// production containment surface: unshared namespaces, --disable-userns so
// nested containment is impossible inside, dropped capabilities, a cleared
// environment, and ro-binds for the system surface. Only the paths the journey
// needs are bound in; the sandboxed Sworn binary then dispatches the fake
// driver through the test-only uncontained path instead of requiring nested
// bwrap (which would read /usr/bin/bwrap as uid 65534 and refuse).
func workerSandboxArguments(
	swornBinary string,
	fakeBinary string,
	repository string,
	runRoot string,
	manifestPath string,
	environment map[string]string,
) []string {
	arguments := []string{
		"/usr/bin/bwrap",
		"--die-with-parent", "--new-session",
		"--unshare-all", "--unshare-user", "--disable-userns",
		"--cap-drop", "ALL",
		"--clearenv",
		"--tmpfs", "/proc", "--remount-ro", "/proc",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--dir", "/home", "--dir", "/home/sworn",
		"--ro-bind", "/usr", "/usr",
	}
	for _, systemPath := range []string{
		"/lib", "/lib64", "/etc/ssl/certs", "/etc/passwd", "/etc/group",
	} {
		if _, err := os.Stat(systemPath); err == nil {
			arguments = append(arguments, "--ro-bind", systemPath, systemPath)
		}
	}
	arguments = append(arguments,
		"--ro-bind", swornBinary, swornBinary,
		"--ro-bind", fakeBinary, fakeBinary,
		"--bind", repository, repository,
		"--bind", runRoot, runRoot,
		"--ro-bind", manifestPath, manifestPath,
		"--chdir", repository,
		"--setenv", "HOME", "/home/sworn",
		"--setenv", "TMPDIR", "/tmp",
		"--setenv", "LANG", "C.UTF-8",
		"--setenv", "LC_ALL", "C.UTF-8",
		"--setenv", "TZ", "UTC",
		"--setenv", "PATH", "/usr/bin:/bin",
		"--setenv", "PWD", repository,
		"--setenv", "SWORN_TEST_UNCONTAINED_DISPATCH", "1",
	)
	for key, value := range environment {
		arguments = append(arguments, "--setenv", key, value)
	}
	return arguments
}

// runSwornInWorkerSandbox runs one Sworn command inside the outer worker
// sandbox with the uncontained dispatch request set, and asserts the process
// exits with the wanted status.
func runSwornInWorkerSandbox(
	t *testing.T,
	binary string,
	fakeBinary string,
	repository string,
	runRoot string,
	manifestPath string,
	journalPath string,
	wantExit int,
	environment map[string]string,
	args ...string,
) (string, string) {
	t.Helper()
	arguments := workerSandboxArguments(
		binary, fakeBinary, repository, runRoot, manifestPath, environment,
	)
	arguments = append(arguments, binary)
	arguments = append(arguments, args...)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, arguments[0], arguments[1:]...)
	command.Env = cleanEnvironment(nil)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf(
			"Sworn binary in worker sandbox timed out\nstdout:\n%s\nstderr:\n%s",
			stdout.String(),
			stderr.String(),
		)
	}
	exit := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run Sworn in worker sandbox: %v", err)
		}
		exit = exitErr.ExitCode()
	}
	if exit != wantExit {
		t.Fatalf(
			"sworn %v in worker sandbox exit = %d, want %d\nstdout:\n%s\nstderr:\n%s",
			args,
			exit,
			wantExit,
			stdout.String(),
			stderr.String(),
		)
	}
	return stdout.String(), stderr.String()
}

// assertUncontainedRerunReplacesStaleProposal proves the exactly-once cut of
// the orchestration subset inside a worker sandbox: once the proposal target
// moves, a rerun of the same manifest replaces the stale proposal and installs
// nothing. It owns its repository and journal so the replacement dispatch does
// not leak into a caller's dispatch-order evidence.
func assertUncontainedRerunReplacesStaleProposal(
	t *testing.T,
	swornBinary, fakeBinary, fakeDigest string,
) {
	t.Helper()
	repository := newProductRepository(t)
	runRoot := t.TempDir()
	journalPath := filepath.Join(runRoot, "run.sqlite")
	const (
		runID   = "e2e-uncontained-drift"
		release = "e2e-uncontained-drift-release"
	)
	manifestBody, _, _ := e2eManifest(
		t,
		runID,
		repository,
		release,
		fakeBinary,
		fakeDigest,
		"verifier-model",
	)
	manifestPath := writeManifest(t, runRoot, manifestBody)
	run := func() string {
		stdout, _ := runSwornInWorkerSandbox(
			t,
			swornBinary,
			fakeBinary,
			repository,
			runRoot,
			manifestPath,
			journalPath,
			0,
			nil,
			"run", "--manifest", manifestPath, "--journal", journalPath,
		)
		return stdout
	}
	run()
	if err := os.WriteFile(
		filepath.Join(repository, "proposal-drift.txt"),
		[]byte("new target authority\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "--", "proposal-drift.txt")
	runGit(t, repository, "commit", "--quiet", "-m", "move proposal target")
	if stdout := run(); !strings.Contains(stdout, "  state: awaiting_approval") {
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
	proposals, plannerEffects, installs := 0, 0, 0
	for _, command := range snapshot.Commands {
		if command.Kind == "planner_proposal" {
			proposals++
		}
	}
	for _, effect := range snapshot.Effects {
		if effect.Kind == "baton.install" && effect.State == journal.Succeeded {
			installs++
		}
		if effect.Kind != "driver.dispatch" ||
			effect.State != journal.Succeeded {
			continue
		}
		submission, decodeErr := driver.DecodeSubmission(effect.Result)
		if decodeErr == nil &&
			submission.Responsibility == driver.PlannerProposal {
			plannerEffects++
		}
	}
	if proposals != 2 || plannerEffects != 2 || installs != 0 {
		t.Fatalf(
			"exactly-once evidence: proposals=%d planners=%d installs=%d",
			proposals, plannerEffects, installs,
		)
	}
}

// TestUncontainedOrchestrationSubsetRunsInsideWorkerSandbox is the A2
// end-to-end check, declared per ADR 0010 and executed at the host boundary
// (it requires bwrap and git). It proves the orchestration subset —
// scheduling, journal semantics, exactly-once and crash recovery — passes
// inside a worker sandbox where nested containment is impossible, because the
// fake driver dispatches through the test-only uncontained path.
func TestUncontainedOrchestrationSubsetRunsInsideWorkerSandbox(t *testing.T) {
	buildRoot := t.TempDir()
	fakeBinary := filepath.Join(buildRoot, "e2e-fake")
	buildBinary(t, fakeBinary, "./test/e2e/testdata/fake", uncontainedGateLDFlags)
	fakeDigest := fileDigest(t, fakeBinary)
	swornBinary := filepath.Join(buildRoot, "sworn")
	// uncontainedDispatchLDFlags() is the harness helper the orchestration
	// subset uses: it combines the uncontained gate with the runtime hook gate
	// exactly when the run asks for uncontained dispatch, so a host can run
	// the subset in-worker with one helper.
	buildBinary(t, swornBinary, "./cmd/sworn", uncontainedDispatchLDFlags())
	// The crash-recovery cut needs both gates unconditionally: the uncontained
	// gate for the worker dispatch and the hook gate for the crash hook.
	crashBinary := filepath.Join(buildRoot, "sworn-crash")
	buildBinary(t, crashBinary, "./cmd/sworn", hookGateLDFlags+" "+uncontainedGateLDFlags)

	t.Run("scheduling_journal_and_exactly_once", func(t *testing.T) {
		repository := newProductRepository(t)
		runRoot := t.TempDir()
		journalPath := filepath.Join(runRoot, "run.sqlite")
		const (
			runID   = "e2e-uncontained"
			release = "e2e-uncontained-release"
		)
		manifestBody, planBytes, plan := e2eManifest(
			t,
			runID,
			repository,
			release,
			fakeBinary,
			fakeDigest,
			"verifier-model",
		)
		manifestPath := writeManifest(t, runRoot, manifestBody)

		// Scheduling: start the run inside the worker sandbox; the fake
		// dispatches run uncontained and the run pauses awaiting approval
		// without creating any authority refs.
		stdout, _ := runSwornInWorkerSandbox(
			t,
			swornBinary,
			fakeBinary,
			repository,
			runRoot,
			manifestPath,
			journalPath,
			0,
			nil,
			"run", "--manifest", manifestPath, "--journal", journalPath,
		)
		if !strings.Contains(stdout, "  state: awaiting_approval") {
			t.Fatalf("uncontained planner output = %q", stdout)
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

		// Exactly-once: a rerun after the proposal target moves replaces the
		// stale proposal rather than duplicating authority. The drift is what
		// makes the first proposal stale — without it a rerun is correctly a
		// no-op, since declining to re-propose at an unchanged revision is
		// the same invariant. This cut runs against its own repository and
		// journal: a replacement proposal is a second planner dispatch, and
		// leaving it in the shared journal would falsify the dispatch order
		// asserted below.
		assertUncontainedRerunReplacesStaleProposal(
			t, swornBinary, fakeBinary, fakeDigest,
		)

		// Journal semantics + scheduling: authorize and install the disjoint
		// component at the host boundary, then resume inside the sandbox and
		// drive the serial track to a merged assembly in dispatch order.
		authorizePlan(t, journalPath, runID, plan)
		installAndPassComponent(t, repository, release, planBytes)
		stdout, stderr := runSwornInWorkerSandbox(
			t,
			swornBinary,
			fakeBinary,
			repository,
			runRoot,
			manifestPath,
			journalPath,
			0,
			nil,
			"resume", "--run", runID, "--journal", journalPath,
			"--command", "resume-1", "--generation", "0",
		)
		if stderr != "" {
			t.Fatalf("resume stderr = %q", stderr)
		}
		stdout, stderr = runSwornInWorkerSandbox(
			t,
			swornBinary,
			fakeBinary,
			repository,
			runRoot,
			manifestPath,
			journalPath,
			0,
			nil,
			"run", "--manifest", manifestPath, "--journal", journalPath,
		)
		if stderr != "" || !strings.Contains(stdout, "  state: complete") {
			t.Fatalf("resume stdout = %q, stderr = %q", stdout, stderr)
		}
		assertDispatchOrder(t, journalPath, runID)
		state := readBatonState(t, repository, release)
		if state.Assembly.Outcome != "merged" ||
			state.Assembly.Candidate == nil ||
			state.Assembly.Pass == nil ||
			state.Assembly.ResultCommit == "" {
			t.Fatalf("assembly state = %#v", state.Assembly)
		}
		if got := runGit(t, repository, "show", "main:one.txt"); got != "active track" {
			t.Fatalf("active product = %q", got)
		}
		if got := runGit(t, repository, "show", "main:two.txt"); got != "component track" {
			t.Fatalf("component product = %q", got)
		}
	})

	t.Run("crash_recovery", func(t *testing.T) {
		repository := newProductRepository(t)
		runRoot := t.TempDir()
		journalPath := filepath.Join(runRoot, "run.sqlite")
		const (
			runID   = "e2e-uncontained-crash"
			release = "e2e-uncontained-crash-release"
		)
		manifestBody, planBytes, plan := e2eManifest(
			t,
			runID,
			repository,
			release,
			fakeBinary,
			fakeDigest,
			"verifier-model",
		)
		manifestPath := writeManifest(t, runRoot, manifestBody)
		runSwornInWorkerSandbox(
			t,
			swornBinary,
			fakeBinary,
			repository,
			runRoot,
			manifestPath,
			journalPath,
			0,
			nil,
			"run", "--manifest", manifestPath, "--journal", journalPath,
		)
		authorizePlan(t, journalPath, runID, plan)
		installAndPassComponent(t, repository, release, planBytes)
		crashEnvironment := map[string]string{
			"SWORN_TEST_CRASH_AFTER_EFFECT": "baton.merge",
			"SWORN_TEST_OWNER_LEASE_MILLIS": testLeaseMillis,
		}
		runSwornInWorkerSandbox(
			t,
			swornBinary,
			fakeBinary,
			repository,
			runRoot,
			manifestPath,
			journalPath,
			0,
			nil,
			"resume", "--run", runID, "--journal", journalPath,
			"--command", "resume-1", "--generation", "0",
		)
		runSwornInWorkerSandbox(
			t,
			crashBinary,
			fakeBinary,
			repository,
			runRoot,
			manifestPath,
			journalPath,
			86,
			crashEnvironment,
			"run", "--manifest", manifestPath, "--journal", journalPath,
		)
		stateAfterCrash := readBatonState(t, repository, release)
		if stateAfterCrash.Assembly.Outcome != "merged" ||
			runGit(t, repository, "rev-parse", "main") !=
				stateAfterCrash.Assembly.ResultCommit {
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
		leaseExpiryWait()
		runSwornInWorkerSandbox(
			t,
			swornBinary,
			fakeBinary,
			repository,
			runRoot,
			manifestPath,
			journalPath,
			0,
			nil,
			"takeover", "--run", runID, "--journal", journalPath,
			"--command", "takeover-1", "--generation", "1",
		)
		stdout, _ := runSwornInWorkerSandbox(
			t,
			swornBinary,
			fakeBinary,
			repository,
			runRoot,
			manifestPath,
			journalPath,
			0,
			nil,
			"run", "--manifest", manifestPath, "--journal", journalPath,
		)
		if !strings.Contains(stdout, "  state: complete") {
			t.Fatalf("recovered resume = %q", stdout)
		}
		state := readBatonState(t, repository, release)
		if state.Assembly.Outcome != "merged" || state.Assembly.ResultCommit == "" {
			t.Fatalf("recovered assembly = %#v", state.Assembly)
		}
		assertDispatchOrder(t, journalPath, runID)
	})
}

// TestProductionBinaryRefusesUncontainedDispatch is the A4 end-to-end check,
// declared per ADR 0010 and executed at the host boundary. A production
// binary (no gate linked) refuses the uncontained dispatch request and reports
// it through the journaled dispatch_operational_failure path; the run cannot
// complete.
func TestProductionBinaryRefusesUncontainedDispatch(t *testing.T) {
	buildRoot := t.TempDir()
	fakeBinary := filepath.Join(buildRoot, "e2e-fake")
	buildBinary(t, fakeBinary, "./test/e2e/testdata/fake", "")
	fakeDigest := fileDigest(t, fakeBinary)
	swornBinary := filepath.Join(buildRoot, "sworn")
	// Production link flags: no hook gate, no uncontained gate.
	buildBinary(t, swornBinary, "./cmd/sworn", "")

	repository := newProductRepository(t)
	runRoot := t.TempDir()
	journalPath := filepath.Join(runRoot, "run.sqlite")
	const (
		runID   = "e2e-uncontained-refusal"
		release = "e2e-uncontained-refusal-release"
	)
	manifestBody, _, _ := e2eManifest(
		t,
		runID,
		repository,
		release,
		fakeBinary,
		fakeDigest,
		"verifier-model",
	)
	manifestPath := writeManifest(t, runRoot, manifestBody)
	// The refusal is an operational failure on the first dispatch: the run
	// parks (exit 0) and never reaches a complete assembly.
	stdout, stderr := runBinaryWithEnvironment(
		t,
		swornBinary,
		0,
		map[string]string{"SWORN_TEST_UNCONTAINED_DISPATCH": "1"},
		"run", "--manifest", manifestPath, "--journal", journalPath,
	)
	if stderr != "" || !strings.Contains(stdout, "  state: parked") {
		t.Fatalf("refusal run stdout = %q, stderr = %q", stdout, stderr)
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
	refused := 0
	for _, effect := range snapshot.Effects {
		if effect.Kind == "driver.dispatch" &&
			effect.State == journal.OperationalFailed &&
			effect.ErrorCode == "UNCONTAINED_DISPATCH_REFUSED" {
			refused++
		}
	}
	if refused == 0 {
		t.Fatal("no journaled UNCONTAINED_DISPATCH_REFUSED operational failure")
	}
	// A run refused at its first dispatch parks before any baton authority
	// effect, so it must leave no authority refs behind. Assert that directly
	// with git (mirroring the A2 planner-pause pattern) rather than reading
	// baton state, which REF_NOT_FOUNDs because the release-wt ref was never
	// created.
	for _, ref := range []string{
		"refs/heads/release-wt/" + release,
		"refs/heads/track/" + release + "/T1",
		"refs/heads/track/" + release + "/T2",
	} {
		command := exec.Command(e2eGit, "-C", repository, "show-ref", "--verify", "--quiet", ref)
		if err := command.Run(); err == nil {
			t.Fatalf("refused run created authority ref %s", ref)
		}
	}
}
