//go:build linux

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

// A4. Every place this kernel can be interrupted must recover to exactly one
// authorized continuation and exactly one final mutation: no duplicate
// verdict, effect, receipt, commit, or ref movement.
//
// The seams below are the real effect boundaries the serial loop crosses --
// preparing a track base (target advance), dispatching role work, appending a
// Captain decision or a Verifier verdict, publishing a candidate in both of
// its halves, and Merge. Each one is cut with the product's own ldflags crash
// seam after the effect has really happened but before it was recorded, which
// is the hardest case: the world moved and the journal does not know it yet.
//
// The run is then restarted with an ordinary binary and must finish. What is
// asserted afterwards is cardinality, because that is what "exactly once"
// means: one succeeded effect per replay key, one receipt per role and
// attempt, one terminal per invocation, one movement of the target ref onto
// the merged commit, and one merge.

// exactlyOnceCut is one interruption point.
type exactlyOnceCut struct {
	// effect is the product's own effect kind, cut after it succeeded.
	effect string
	// phase names, for a human reading a failure, which part of the
	// lifecycle this seam belongs to.
	phase string
}

func exactlyOnceCuts() []exactlyOnceCut {
	return []exactlyOnceCut{
		{effect: "git.prepare_track_base", phase: "target advance"},
		{effect: "baton.append_receipt", phase: "decision and verdict records"},
		{effect: "git.seal.prepared", phase: "candidate publication (prepared)"},
		{effect: "git.seal", phase: "candidate publication"},
		{effect: "baton.merge", phase: "merge"},
	}
}

// driver.dispatch is deliberately absent from that table. A dispatch cut is
// the one seam whose correct recovery is *not* a continuation: the product
// leaves it quiescent and uncertain and never retries it, because a role
// invocation whose outcome is unknown must not be repeated. That behavior has
// its own coverage in topology_recovery_linux_test.go; asserting completion
// for it here would assert the opposite of the contract.

// exactlyOnceEnvironment keeps the crash and recovery processes in the same
// short owner-lease regime the recovery journeys run under, so a takeover
// only has to wait out leaseExpiryWait rather than a production lease.
var exactlyOnceEnvironment = map[string]string{
	"SWORN_TEST_OWNER_LEASE_MILLIS":   testLeaseMillis,
	"SWORN_TEST_UNCONTAINED_DISPATCH": "1",
}

// exactlyOnceRecoveryEnvironment gives the restart phase a lease long enough
// that a control verb's cancelled in-process drive can still release its own
// claim: ReleaseOwner refuses an expired lease, so under the short crash
// lease a cancelled recovery that outlives it would strand the owner
// claimed-expired and only another takeover could clear it.
var exactlyOnceRecoveryEnvironment = map[string]string{
	"SWORN_TEST_OWNER_LEASE_MILLIS":   "5000",
	"SWORN_TEST_UNCONTAINED_DISPATCH": "1",
}

// assertExactlyOnce is the whole cardinality claim, read out of the real
// journal and the real repository after recovery.
func assertExactlyOnce(
	t *testing.T,
	journalPath, runID, repository, release string,
	seedHead string,
) {
	t.Helper()
	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(context.Background(), runID)
	_ = store.Close()
	if err != nil {
		t.Fatal(err)
	}

	// One succeeded effect per replay key, and nothing left unresolved.
	succeededByReplayKey := map[string]string{}
	merges, seals := 0, 0
	for _, effect := range snapshot.Effects {
		switch effect.State {
		case journal.Succeeded:
			if prior, duplicate := succeededByReplayKey[effect.ReplayKey]; duplicate {
				t.Fatalf(
					"replay key %q succeeded twice: %s and %s",
					effect.ReplayKey, prior, effect.ID,
				)
			}
			succeededByReplayKey[effect.ReplayKey] = effect.ID
			switch effect.Kind {
			case "baton.merge":
				merges++
			case "git.seal":
				seals++
			}
		case journal.Uncertain, journal.Claimed:
			t.Fatalf(
				"recovery left effect %s (%s) in state %s",
				effect.ID, effect.Kind, effect.State,
			)
		}
	}
	if merges != 1 {
		t.Fatalf("succeeded merge effects = %d, want exactly one", merges)
	}
	if seals == 0 {
		t.Fatal("no candidate was published")
	}

	// One terminal per dispatched invocation.
	terminals := map[string]string{}
	for _, effect := range snapshot.Effects {
		if effect.Kind != "driver.dispatch" || effect.State != journal.Succeeded {
			continue
		}
		submission, decodeErr := driver.DecodeSubmission(effect.Result)
		if decodeErr != nil {
			continue
		}
		if prior, duplicate := terminals[submission.InvocationID]; duplicate {
			t.Fatalf(
				"invocation %s produced two terminals: %s and %s",
				submission.InvocationID, prior, effect.ID,
			)
		}
		terminals[submission.InvocationID] = effect.ID
	}
	if len(terminals) == 0 {
		t.Fatal("no role work was dispatched")
	}

	// One receipt per slice, role and attempt.
	state := readBatonState(t, repository, release)
	if state.Assembly.Outcome != "merged" || state.Assembly.ResultCommit == "" {
		t.Fatalf("recovered assembly = %#v", state.Assembly)
	}
	seen := map[string]string{}
	for _, slice := range state.Slices {
		for _, entry := range slice.History.Entries {
			attempt := int64(0)
			if entry.Receipt.Attempt != nil {
				attempt = *entry.Receipt.Attempt
			}
			key := fmt.Sprintf(
				"%s/%s/%s/%d",
				slice.Location.Slice.ID, entry.Receipt.Role,
				entry.Receipt.Result, attempt,
			)
			if prior, duplicate := seen[key]; duplicate {
				t.Fatalf("receipt %s recorded twice: %s and %s", key, prior, entry.OID)
			}
			seen[key] = entry.OID
		}
	}

	// One final mutation: the target is the merge result, and the target ref
	// arrived at that commit exactly once.
	target := runGit(t, repository, "rev-parse", "main")
	if target != state.Assembly.ResultCommit {
		t.Fatalf("target %s != assembly result %s", target, state.Assembly.ResultCommit)
	}
	if target == seedHead {
		t.Fatal("the recovered run merged nothing")
	}
	arrivals := 0
	for _, line := range strings.Split(
		runGit(t, repository, "reflog", "show", "--format=%H", "refs/heads/main"),
		"\n",
	) {
		if strings.TrimSpace(line) == target {
			arrivals++
		}
	}
	if arrivals != 1 {
		t.Fatalf("the target ref arrived at %s %d times, want exactly one", target, arrivals)
	}
}

// TestProductionCrashCutsRecoverToExactlyOneContinuation is A4.
func TestProductionCrashCutsRecoverToExactlyOneContinuation(t *testing.T) {
	t.Parallel()
	buildRoot := t.TempDir()
	fakeBinary := filepath.Join(buildRoot, "e2e-fake")
	buildBinary(t, fakeBinary, "./test/e2e/testdata/fake", "")
	fakeDigest := fileDigest(t, fakeBinary)
	// The recovery binary is an ordinary Sworn: no crash seam at all. Only the
	// owner lease is pinned, to the value the existing recovery journeys use.
	recoveryBinary := filepath.Join(buildRoot, "sworn-recovery")
	buildBinary(t, recoveryBinary, "./cmd/sworn", uncontainedDispatchLDFlags())

	for _, cut := range exactlyOnceCuts() {
		t.Run(strings.ReplaceAll(cut.effect, ".", "_"), func(t *testing.T) {
			crashBinary := filepath.Join(
				buildRoot, "sworn-cut-"+strings.ReplaceAll(cut.effect, ".", "-"),
			)
			buildBinary(t, crashBinary, "./cmd/sworn", uncontainedDispatchLDFlags())
			crashEnvironment := map[string]string{
				"SWORN_TEST_CRASH_AFTER_EFFECT":   cut.effect,
				"SWORN_TEST_OWNER_LEASE_MILLIS":   testLeaseMillis,
				"SWORN_TEST_UNCONTAINED_DISPATCH": "1",
			}

			repository := newProductRepository(t)
			runRoot := t.TempDir()
			journalPath := filepath.Join(runRoot, "run.sqlite")
			runID := "e2e-exactly-once-" + strings.ReplaceAll(cut.effect, ".", "-")
			release := runID + "-release"
			manifestBody, planBytes, plan := e2eManifest(
				t, runID, repository, release, fakeBinary, fakeDigest, "verifier-model",
			)
			manifestPath := writeManifest(t, runRoot, manifestBody)

			// The proposal phase runs on an ordinary binary. Every seam in
			// the table belongs to the authorized part of the lifecycle,
			// which begins at the resume below, so the cut lands there.
			runBinaryWithEnvironment(
				t, recoveryBinary, 0, exactlyOnceEnvironment,
				"run", "--manifest", manifestPath, "--journal", journalPath,
			)
			authorizePlan(t, journalPath, runID, plan)
			installAndPassComponent(t, repository, release, planBytes)
			seedHead := runGit(t, repository, "rev-parse", "main")

			// The cut. Durable resume hosts the drive and exits 86 at the seam.
			runBinaryWithEnvironment(
				t, crashBinary, 86, crashEnvironment,
				"resume", "--run", runID, "--journal", journalPath,
				"--command", "cut-resume-1", "--generation", "0",
			)
			cutSnapshot := recordedEffectStates(t, journalPath, runID)
			if cutSnapshot[cut.effect] == 0 {
				t.Fatalf(
					"%s (%s) never reached its seam: %#v",
					cut.effect, cut.phase, cutSnapshot,
				)
			}

			// The restart. An ordinary binary, after the dead owner's lease
			// has expired, performs a durable takeover which hosts its drive
			// to completion.
			leaseExpiryWait()
			takeoverStdout, takeoverStderr := runBinaryWithEnvironmentTimeout(
				t, recoveryBinary, 0, exactlyOnceRecoveryEnvironment, 600*time.Second,
				"takeover", "--run", runID, "--journal", journalPath,
				"--command", "cut-takeover-1", "--generation", "1",
			)
			if takeoverStderr != "" || !strings.Contains(takeoverStdout, "Sworn run "+runID) {
				t.Fatalf("%s (%s) takeover stdout=%q stderr=%q", cut.effect, cut.phase, takeoverStdout, takeoverStderr)
			}
			stdout, stderr := runBinaryWithEnvironmentTimeout(
				t, recoveryBinary, 0, exactlyOnceRecoveryEnvironment, 600*time.Second,
				"run", "--manifest", manifestPath, "--journal", journalPath,
			)
			if stderr != "" || !strings.Contains(stdout, "  state: complete") {
				t.Fatalf(
					"%s (%s) recovery stdout=%q stderr=%q",
					cut.effect, cut.phase, stdout, stderr,
				)
			}

			assertExactlyOnce(t, journalPath, runID, repository, release, seedHead)

			// The recovered state reads the same through the command line as
			// it does in the journal, which is what the conformance profile's
			// restart case promises on the CLI surface.
			statusBody, statusErr := runBinary(
				t, recoveryBinary, 0,
				"status", "--run", runID, "--journal", journalPath, "--json",
			)
			var status swornruntime.RunStatus
			if statusErr != "" ||
				json.Unmarshal([]byte(statusBody), &status) != nil {
				t.Fatalf("recovered status = %q / %q", statusBody, statusErr)
			}
			if status.RunID != runID || status.State != "complete" ||
				status.Outcome != "merged" ||
				status.TargetHead != runGit(t, repository, "rev-parse", "main") {
				t.Fatalf("recovered status = %#v", status)
			}
			recordSwornConformance(
				t, caseRestartRecovery, surfaceCLI,
				"exactly-once/restart/"+cut.effect+"/cli",
			)
			recordSwornConformance(
				t, caseRestartRecovery, surfaceConfiguredDriver,
				"exactly-once/restart/"+cut.effect+"/configured-driver",
			)
		})
	}
}

// recordedEffectStates counts every recorded effect by kind, whatever state it
// is in. It exists so a cut can prove it actually happened before recovery is
// judged: a seam that never fired would make the recovery below vacuous.
func recordedEffectStates(t *testing.T, journalPath, runID string) map[string]int {
	t.Helper()
	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(context.Background(), runID)
	_ = store.Close()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, effect := range snapshot.Effects {
		counts[effect.Kind]++
	}
	return counts
}

// assertNoDuplicateVerdicts is a small shared helper the repair journey uses
// to state the same exactly-once claim about a direct repair after a Verifier
// FAIL: the failed attempt and the repaired attempt are each recorded once,
// and the passed attempt is the one that merged.
func assertNoDuplicateVerdicts(t *testing.T, state baton.State, slice string) {
	t.Helper()
	record, ok := state.Slice(slice)
	if !ok {
		t.Fatalf("slice %s is absent", slice)
	}
	verdicts := map[int64]string{}
	for _, entry := range record.History.Entries {
		if entry.Receipt.Role != "verifier" || entry.Receipt.Attempt == nil {
			continue
		}
		attempt := *entry.Receipt.Attempt
		if prior, duplicate := verdicts[attempt]; duplicate {
			t.Fatalf(
				"slice %s attempt %d has two verdicts: %s and %s",
				slice, attempt, prior, entry.Receipt.Result,
			)
		}
		verdicts[attempt] = entry.Receipt.Result
	}
	if len(verdicts) < 2 {
		t.Fatalf("slice %s recorded %d verdicts, want a FAIL and its repair",
			slice, len(verdicts))
	}
	if verdicts[1] != "fail" || verdicts[2] != "pass" {
		t.Fatalf("slice %s verdicts = %#v", slice, verdicts)
	}
	if record.Pass == nil || record.Pass.Receipt.Attempt == nil ||
		*record.Pass.Receipt.Attempt != 2 {
		t.Fatalf("slice %s passed on the wrong attempt: %#v", slice, record.Pass)
	}
}
