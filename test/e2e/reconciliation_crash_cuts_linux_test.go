//go:build linux

package e2e

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/journal"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

// A2. Interrupted-work reconciliation adds four crash cuts inside
// reconcileInterruptedWorkspaces/captureSalvageCheckpoint that exactlyOnceCuts
// cannot express: none of them is a journal effect kind (they are gitx
// on-disk marker cuts, cf. exactlyOnceCuts' own driver.dispatch exclusion
// note), so what is proven here is checkpoint and workspace state rather
// than effect cardinality. A fifth cut, workspace.cleanup, never touches an
// attributed tree at all (it only fires for unattributed debris) and is
// proven directly at the gitx level instead, in
// internal/gitx/workspaces_test.go.
//
// workspace.attribute fires before any driver call, so a fresh retry after
// it dispatches cleanly and the run reaches completion. The other three fire
// during reconciliation of a workspace whose driver dispatch already
// succeeded (left behind by a genuine implementation.handoff crash, the same
// pre-existing seam S1's own preservation journey relies on): once that
// dispatch has succeeded, the product's any-succeeded-try guard correctly
// refuses to open a second try-level dispatch claim for it under the
// scripted, non-production driver this harness uses (continuing a succeeded
// dispatch straight into sealing is a production-only path), so the run
// parks on retry exhaustion rather than completing. That park is itself part
// of what this test proves: every effect resolves to a terminal state with
// no duplicate transition and no unresolved "needs confirmation" - the
// crash cuts converge cleanly even though the outer implementation attempt
// cannot proceed, rather than leaving the run in the ambiguous
// effect-recovery refusal the contract's own known seam warns about.
func reconciliationCrashCuts() []string {
	return []string{
		"workspace.attribute",
		"checkpoint.prepare",
		"checkpoint.bind",
		"checkpoint.publish",
	}
}

func TestReconciliationCrashCutsRecoverExactlyOnceSalvagedCheckpoint(t *testing.T) {
	t.Parallel()
	buildRoot := t.TempDir()
	swornBinary := filepath.Join(buildRoot, "sworn")
	buildBinary(t, swornBinary, "./cmd/sworn", hookGateLDFlags+" "+uncontainedGateLDFlags)
	fakeBinary := filepath.Join(buildRoot, "e2e-fake")
	buildBinary(t, fakeBinary, "./test/e2e/testdata/fake", "")
	fakeDigest := fileDigest(t, fakeBinary)

	for _, seam := range reconciliationCrashCuts() {
		t.Run(strings.ReplaceAll(seam, ".", "_"), func(t *testing.T) {
			repository := newProductRepository(t)
			runRoot := t.TempDir()
			journalPath := filepath.Join(runRoot, "run.sqlite")
			runID := "e2e-reconcile-" + strings.ReplaceAll(seam, ".", "-")
			release := runID + "-release"
			manifestBody, planBytes, plan := e2eManifest(
				t, runID, repository, release, fakeBinary, fakeDigest, "verifier-model",
			)
			manifestPath := writeManifest(t, runRoot, manifestBody)

			runBinaryWithEnvironment(
				t, swornBinary, 0, exactlyOnceEnvironment,
				"run", "--manifest", manifestPath, "--journal", journalPath,
			)
			authorizePlan(t, journalPath, runID, plan)
			installAndPassComponent(t, repository, release, planBytes)
			seedHead := runGit(t, repository, "rev-parse", "main")

			seamCrashEnvironment := map[string]string{
				"SWORN_TEST_CRASH_AFTER_EFFECT":   seam,
				"SWORN_TEST_OWNER_LEASE_MILLIS":   testLeaseMillis,
				"SWORN_TEST_UNCONTAINED_DISPATCH": "1",
			}
			dispatchedFirst := seam != "workspace.attribute"
			finalGeneration := "1"
			if !dispatchedFirst {
				// Nothing has been dispatched yet: the seam itself is the
				// only cut, on the first resume.
				runBinaryWithEnvironment(
					t, swornBinary, 86, seamCrashEnvironment,
					"resume", "--run", runID, "--journal", journalPath,
					"--command", "reconcile-seam-cut", "--generation", "0",
				)
			} else {
				handoffCrashEnvironment := map[string]string{
					"SWORN_TEST_CRASH_AFTER_EFFECT":   "implementation.handoff",
					"SWORN_TEST_OWNER_LEASE_MILLIS":   testLeaseMillis,
					"SWORN_TEST_UNCONTAINED_DISPATCH": "1",
				}
				runBinaryWithEnvironment(
					t, swornBinary, 86, handoffCrashEnvironment,
					"resume", "--run", runID, "--journal", journalPath,
					"--command", "reconcile-handoff-cut", "--generation", "0",
				)
				leaseExpiryWait()
				runBinaryWithEnvironment(
					t, swornBinary, 86, seamCrashEnvironment,
					"takeover", "--run", runID, "--journal", journalPath,
					"--command", "reconcile-seam-cut", "--generation", "1",
				)
				finalGeneration = "2"
			}
			leaseExpiryWait()

			midCheckpoints := readUnverifiedCheckpoints(t, journalPath, runID)
			switch seam {
			case "workspace.attribute", "checkpoint.prepare", "checkpoint.bind":
				if len(midCheckpoints) != 0 {
					t.Fatalf(
						"%s: an unbound object was reported as a completed checkpoint: %#v",
						seam, midCheckpoints,
					)
				}
			case "checkpoint.publish":
				if len(midCheckpoints) != 1 || !midCheckpoints[0].Salvaged {
					t.Fatalf(
						"%s: bound checkpoint is not reachable right after its own cut: %#v",
						seam, midCheckpoints,
					)
				}
				runGit(t, repository, "rev-parse", "--verify", midCheckpoints[0].CheckpointRef)
			}

			takeoverStdout, takeoverStderr := runBinaryWithEnvironmentTimeout(
				t, swornBinary, 0, exactlyOnceRecoveryEnvironment, 600*time.Second,
				"takeover", "--run", runID, "--journal", journalPath,
				"--command", "reconcile-recovery-takeover", "--generation", finalGeneration,
			)
			if takeoverStderr != "" || !strings.Contains(takeoverStdout, "Sworn run "+runID) {
				t.Fatalf("%s: takeover stdout=%q stderr=%q", seam, takeoverStdout, takeoverStderr)
			}
			stdout, stderr := runBinaryWithEnvironmentTimeout(
				t, swornBinary, 0, exactlyOnceRecoveryEnvironment, 600*time.Second,
				"run", "--manifest", manifestPath, "--journal", journalPath,
			)
			wantState := "  state: complete"
			if dispatchedFirst {
				wantState = "  state: parked"
			}
			if stderr != "" || !strings.Contains(stdout, wantState) {
				t.Fatalf("%s: recovery stdout=%q stderr=%q", seam, stdout, stderr)
			}

			statusStdout, statusErr := runBinary(
				t, swornBinary, 0,
				"status", "--run", runID, "--journal", journalPath, "--json",
			)
			var status swornruntime.RunStatus
			if statusErr != "" || json.Unmarshal([]byte(statusStdout), &status) != nil {
				t.Fatalf("%s: recovered status = %q / %q", seam, statusStdout, statusErr)
			}

			if !dispatchedFirst {
				if status.State != "complete" || status.Outcome != "merged" {
					t.Fatalf("%s: recovered status = %#v", seam, status)
				}
				assertExactlyOnce(t, journalPath, runID, repository, release, seedHead)
			} else {
				// The dispatch this cut's pre-stage already completed can
				// never be retried under the scripted driver's
				// non-production path (a second try-level claim for an
				// already-succeeded work is refused, by design, rather than
				// silently repeating a role invocation): the run parks on
				// retry exhaustion for this one work, not on an ambiguous
				// "needs confirmation" recovery refusal. What must still
				// hold is that every effect the crash cuts touched resolved
				// to a definite terminal state exactly once.
				if status.State != "parked" || status.Park == nil || status.Park.Cause != "exhaustion" {
					t.Fatalf("%s: recovered status = %#v", seam, status)
				}
				assertNoNonterminalOrDuplicateEffect(t, journalPath, runID)
			}

			finalCheckpoints := readUnverifiedCheckpoints(t, journalPath, runID)
			if !dispatchedFirst {
				if len(finalCheckpoints) != 0 {
					t.Fatalf("%s: an unbound object was salvaged after all: %#v", seam, finalCheckpoints)
				}
			} else {
				salvaged := 0
				for _, cp := range finalCheckpoints {
					runGit(t, repository, "rev-parse", "--verify", cp.CheckpointRef)
					if cp.Salvaged {
						salvaged++
					}
				}
				if salvaged == 0 {
					t.Fatalf("%s: no salvaged checkpoint survived recovery: %#v", seam, finalCheckpoints)
				}
			}

			boardStdout, boardStderr := runBinary(
				t, swornBinary, 0, "board", "--run", runID, "--journal", journalPath,
			)
			if boardStderr != "" {
				t.Fatalf("%s: board stderr=%q", seam, boardStderr)
			}
			if strings.Contains(boardStdout, "Needs confirmation") {
				t.Fatalf(
					"%s: board still shows Needs confirmation after convergence:\n%s",
					seam, boardStdout,
				)
			}
		})
	}
}

// readUnverifiedCheckpoints is a small wrapper the reconciliation crash-cut
// scenarios use between phases: each sworn invocation has already exited by
// the time this reads, so a fresh read-only journal handle never contends
// with the product's own writer.
func readUnverifiedCheckpoints(
	t *testing.T, journalPath, runID string,
) []journal.UnverifiedCheckpoint {
	t.Helper()
	store, err := journal.OpenReadOnly(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	checkpoints, err := store.ListUnverifiedCheckpoints(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	return checkpoints
}

// assertNoNonterminalOrDuplicateEffect is the exactly-once claim for a run
// that parks rather than completes: nothing is left Claimed or Uncertain
// (the crash cuts and the retries they forced all resolved), and no replay
// key succeeded twice (no cut caused a transition to repeat).
func assertNoNonterminalOrDuplicateEffect(t *testing.T, journalPath, runID string) {
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
	succeededByReplayKey := map[string]string{}
	for _, effect := range snapshot.Effects {
		if effect.State == journal.Claimed || effect.State == journal.Uncertain {
			t.Fatalf("parked run left a nonterminal effect: %#v", effect)
		}
		if effect.State != journal.Succeeded {
			continue
		}
		if prior, duplicate := succeededByReplayKey[effect.ReplayKey]; duplicate {
			t.Fatalf(
				"replay key %q succeeded twice: %s and %s",
				effect.ReplayKey, prior, effect.ID,
			)
		}
		succeededByReplayKey[effect.ReplayKey] = effect.ID
	}
}
