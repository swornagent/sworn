package run

// loop_sit_test.go is S10-conformance-sit's cold-board System Integration Test
// (AC-03/AC-04/AC-05/AC-06). It boots the assembled parallel loop — the real
// RunParallel → RunTrack → RunSlice path, NOT a mocked leaf — over a hermetic
// fixture release from a COLD board (no pre-made release or track worktrees),
// with every role dispatch served by the offline reference StubDriver from
// internal/driver/drivertest (design D4: the SIT's dispatched driver IS the
// conformance-certified stub, so it can never silently diverge from what the
// conformance suite in AC-01/AC-02 certified).
//
// It is the release's own Rule-1 reachability artefact for the whole seam: the
// three-model dogfood (2026-06-28) shipped DOA on unit-green because nothing
// ever booted the assembled loop from a cold board — the nil-factory SIGSEGV
// and cold-start crash were invisible to per-slice verification over mocked
// leaves. This test boots it for real.
//
// AC-05: no network, no paid dispatch — the StubDriver has no transport.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/db"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/driver/drivertest"
	"github.com/swornagent/sworn/internal/driver/registry"
	"github.com/swornagent/sworn/internal/state"
)

const (
	sitRelease     = "sit-fixture"
	sitTrackID     = "T1-sit"
	sitSliceID     = "S01-sit-slice"
	sitTrackBranch = "track/sit-fixture/T1-sit"

	// sitDeadline bounds the whole cold-board boot. With the verified-path
	// commit present (internal/run/slice.go PASS branch), the loop terminates in
	// well under a second — one slice, an offline driver, then a merge-track
	// pause. The ceiling exists only so the sworn#93 regression — reverting that
	// commit makes the committed track ref never advance to `verified`, so the
	// production router re-reads `implemented` and re-dispatches the verify leg
	// forever — fails to a BOUNDED deadline (AC-04) instead of hanging CI.
	sitDeadline = 30 * time.Second
)

// sitDesignText is the StubDriver's captain-leg output. It serves BOTH captain
// dispatches RunSlice makes: design.Generate (which requires all six §1–§6
// section markers, else it errors) and captain.Review (which halts the run on
// any `N. [escalate]` pin line — this text has none, so the review is a clean
// PROCEED). Pin 2 (Coach disposition): the stub scripts the captain leg
// (design TL;DR + zero-escalate review).
const sitDesignText = `# Design TL;DR — S01-sit-slice

§1 Approach — reference slice; exercise the assembled loop end to end over the fixture release.
§2 Data model — none; the fixture carries no schema surface.
§3 Surface — the parallel boot path RunParallel -> RunTrack -> RunSlice; no production affordance.
§4 Risks — none material; the offline reference driver is deterministic and never touches the network.
§5 Tests — TestLoopSIT is the reachability artefact for this fixture.
§6 Rollback — delete the fixture; there is no production surface to revert.

No blocking findings. PROCEED.
`

// sitStub builds the offline reference driver for the SIT: the exported
// drivertest.StubDriver (design D4) with per-role handlers scripting the
// captain (design + review), implement, and verify legs. Its Dispatch fires the
// Rule-11 worktree guard before any handler (drivertest/stub.go), so the SIT
// also exercises AssertWorktree on the real materialised track worktree.
func sitStub() *drivertest.StubDriver {
	return &drivertest.StubDriver{
		DriverName: "sit-reference",
		Handlers: map[driver.Role]func(driver.DispatchInput) (driver.Result, error){
			driver.RoleCaptain: func(in driver.DispatchInput) (driver.Result, error) {
				// The captain handler serves THREE captain-family dispatches,
				// distinguished by their system prompt (pin 2 — the stub scripts
				// the whole captain leg):
				//   - reqverify DoR grading (implement.Run's Definition-of-Ready
				//     gate) → a `## RESULTS` section grading every AC PASS, so the
				//     gate passes on the first pass rather than being retry-bypassed.
				//   - design TL;DR / captain review → the six-section, zero-escalate
				//     -pin text (design.Generate requires §1–§6; captain.Review
				//     halts on any escalate pin line — this has none).
				text := sitDesignText
				if strings.Contains(in.SystemPrompt, "requirements quality gate") {
					text = sitReqverifyResults(in.Payload)
				}
				return driver.Result{
					Status:     driver.StatusOK,
					ResultText: text,
					ModelID:    in.ModelID,
					CostSource: driver.CostSourceUnknown,
				}, nil
			},
			driver.RoleImplementer: func(in driver.DispatchInput) (driver.Result, error) {
				// implement.Run itself writes proof.md and transitions the slice
				// to `implemented` (pin 2: proof.md is written by implement.Run,
				// not pre-baked). The stub only has to return a well-formed OK.
				return driver.Result{
					Status:     driver.StatusOK,
					ResultText: "sit-reference: implementation complete",
					ModelID:    in.ModelID,
					CostSource: driver.CostSourceUnknown,
				}, nil
			},
			driver.RoleVerifier: func(in driver.DispatchInput) (driver.Result, error) {
				// structuredVerdictReply (run_test.go) emits a schema-valid
				// verifier-verdict-v1 PASS; the ENGINE validates it fail-closed
				// inside verify.RunAgentic after Dispatch returns.
				return driver.Result{
					Status:         driver.StatusOK,
					StructuredJSON: json.RawMessage(structuredVerdictReply("PASS")),
					ModelID:        in.ModelID,
					CostSource:     driver.CostSourceUnknown,
				}, nil
			},
		},
	}
}

// sitReqverifyResults builds the reqverify DoR grading reply: a `## RESULTS`
// section grading every acceptance criterion in the dispatched payload PASS. It
// scans the payload reqverify.Run assembled (### Slice: <id> + `AC <n>: …`
// lines) so it stays correct if the fixture spec's AC count changes.
func sitReqverifyResults(payload string) string {
	slice := sitSliceID
	if m := sitSliceHeaderRe.FindStringSubmatch(payload); m != nil {
		slice = m[1]
	}
	var b strings.Builder
	b.WriteString("Every acceptance criterion is singular, unambiguous, complete, " +
		"consistent, feasible, verifiable, and necessary.\n\n## RESULTS\n")
	for _, m := range sitACLineRe.FindAllStringSubmatch(payload, -1) {
		fmt.Fprintf(&b, "AC %s (%s): PASS\n", m[1], slice)
	}
	return b.String()
}

var (
	sitSliceHeaderRe = regexp.MustCompile(`(?m)^###\s+Slice:\s*(\S+)`)
	sitACLineRe      = regexp.MustCompile(`(?m)^AC\s+(\d+):`)
)

// TestLoopSIT is the cold-board System Integration Test (AC-03/04/05/06).
func TestLoopSIT(t *testing.T) {
	absRoot, releaseWT, trackWT := setupSITFixture(t)

	// Stub driver registered via the S05 registry under the "stub" prefix; the
	// model IDs below resolve to it for every role (RoleImplementer/Verifier/
	// Captain are all declared by StubDriver.Roles()).
	stub := sitStub()
	reg := registry.New()
	reg.Register(registry.Entry{Driver: stub, Prefixes: []string{"stub"}})

	// RunSliceFn is the REAL RunSlice wired with the stub registry — exactly how
	// cmd/sworn/run.go wires production, but with an offline registry. This is
	// the "not a mocked leaf" contract of AC-03: only the model transport is
	// stubbed; the implement→verify state machine, the design/captain gates, the
	// verdict validation, and the git commits are all the real thing.
	runSliceFn := func(ctx context.Context, worktreeRoot, specPath, statusPath string) error {
		return RunSlice(ctx, worktreeRoot, specPath, statusPath, RunSliceOptions{
			ImplementerModel: "stub/impl",
			VerifierModel:    "stub/verify",
			EscalationModels: []string{"stub/impl"},
			ImplementTimeout: -1, // no per-attempt timeout; the outer ctx bounds the whole run
			Registry:         reg,
		})
	}

	database, err := db.Open(filepath.Join(absRoot, db.DefaultDir, "sit.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), sitDeadline)
	defer cancel()

	// Boot the assembled parallel loop over the fixture from a COLD board:
	//   - Router nil        → RunParallel auto-constructs the PRODUCTION router,
	//                          which re-reads COMMITTED track-ref state via git
	//                          (the exact reader whose blindness sworn#93 exploited).
	//   - MergeTrackFn nil  → D7: AC-03's boundary is "at least one slice reaches
	//                          verified", not "track merged"; the router's
	//                          merge-track decision pauses the track (terminal for
	//                          this run) rather than auto-merging.
	runErr := RunParallel(ctx, ParallelOptions{
		ReleaseName:   sitRelease,
		WorkspaceRoot: absRoot,
		DB:            database,
		RunSliceFn:    runSliceFn,
		ProjectDir:    filepath.Base(absRoot),
	})

	// AC-04: a stalled loop must fail with the board state dumped — never hang.
	// Hitting the ctx deadline means the loop never reached a terminal/pause
	// decision: the sworn#93 re-dispatch spin.
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("AC-04: SIT loop STALLED — RunParallel did not return within %s. "+
			"The committed track ref never advanced to a terminal state, so the router "+
			"re-dispatched forever (sworn#93 verified-commit regression).\n%s",
			sitDeadline, sitBoardDump(t, absRoot))
	}

	// With the fix present the loop terminates by PAUSING at merge-track
	// (MergeTrackFn nil), so RunParallel returns a "paused" outcome. That is the
	// EXPECTED, side-effect-free success shape (D7) — not a failure. The
	// committed-state assertion below is the source of truth.
	t.Logf("RunParallel returned (a merge-track pause is the expected success shape): %v", runErr)

	// AC-03: the release and track worktrees materialised from the cold board.
	if !dirExists(releaseWT) {
		t.Errorf("AC-03: release worktree did not materialise at %s", releaseWT)
	}
	if !dirExists(trackWT) {
		t.Errorf("AC-03: track worktree did not materialise at %s", trackWT)
	}

	// AC-03: the implement, verify, AND captain legs all fired through the
	// registry (proof the dispatch actually reached the driver, not a bypass).
	counts := stub.RoleCounts()
	if counts[driver.RoleImplementer] == 0 {
		t.Error("AC-03: implement leg never dispatched through the registry")
	}
	if counts[driver.RoleVerifier] == 0 {
		t.Error("AC-03: verify leg never dispatched through the registry")
	}
	if counts[driver.RoleCaptain] == 0 {
		t.Error("AC-03: captain leg never dispatched through the registry")
	}

	// AC-03 + AC-06 (the regression assertion for sworn#93): the verdict was
	// consumed by the state machine AND the `verified` transition is COMMITTED to
	// the track ref — not merely written to the worktree. We read the status.json
	// committed on the track branch (what the production router re-reads), never
	// the worktree file (which is `verified` in BOTH the fixed and the buggy case
	// — that is precisely why reading the worktree would be tautological).
	committed := sitCommittedState(t, absRoot)
	if committed != string(state.Verified) {
		t.Fatalf("AC-06: committed track-ref state = %q, want %q. The verified "+
			"transition was written to the worktree but NOT committed to %s, so a "+
			"router re-read observes the pre-verified state and re-dispatches verify. "+
			"This is the sworn#93 regression: revert the verified-path repo.Stage/"+
			"repo.Commit in internal/run/slice.go and this assertion is what fails.\n%s",
			committed, state.Verified, sitTrackBranch, sitBoardDump(t, absRoot))
	}

	// AC-06: because the committed state is terminal (`verified`), the router
	// does NOT re-dispatch the verify leg. One cold-board pass reaches verified in
	// a single implement+verify cycle; the sworn#93 spin shows many verify
	// dispatches. Bound generously to stay robust while catching a runaway.
	if vc := counts[driver.RoleVerifier]; vc > 3 {
		t.Errorf("AC-06: verify leg dispatched %d times — the router re-dispatched "+
			"verify (sworn#93 spin). With the verified-path commit it fires once.", vc)
	}
}

// setupSITFixture builds a hermetic git repo carrying the cold-board fixture
// release and returns the repo root plus the conventional release/track worktree
// paths (which do NOT yet exist — RunParallel/RunTrack materialise them).
func setupSITFixture(t *testing.T) (absRoot, releaseWT, trackWT string) {
	t.Helper()
	tmpRoot := t.TempDir()
	absRoot = filepath.Join(tmpRoot, "repo")
	if err := os.MkdirAll(absRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	// wtParent must exist before `git worktree add` targets a child of it.
	wtParent := filepath.Join(tmpRoot, "wt")
	if err := os.MkdirAll(wtParent, 0o755); err != nil {
		t.Fatal(err)
	}
	releaseWT = filepath.Join(wtParent, "release-"+sitRelease)
	trackWT = filepath.Join(wtParent, "release-"+sitRelease+"-"+sitTrackID)

	runCmd(t, absRoot, "git", "init", "-b", "main")
	runCmd(t, absRoot, "git", "config", "user.email", "sit@swornagent.dev")
	runCmd(t, absRoot, "git", "config", "user.name", "sworn sit")

	// The loop writes runtime logs/DB under .sworn/; keep it out of the worktree
	// commits RunSlice makes (repo.Stage(".") would otherwise sweep it in).
	if err := os.WriteFile(filepath.Join(absRoot, ".gitignore"), []byte("/.sworn/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Copy the static fixture tree into docs/release/<release>/, substituting the
	// release-worktree-path token in board.json.
	relDir := filepath.Join(absRoot, "docs", "release", sitRelease)
	copySITFixture(t, relDir, releaseWT)

	// status.json is generated (not static): start_commit is left empty so
	// RunSlice self-bootstraps it to the track-branch HEAD on the first dispatch
	// (the cold-start path), meaning the fixture needs no baked commit SHA. The
	// slice starts at `implemented` — a slice a prior /implement-slice finished,
	// awaiting verify — which is exactly the sworn#93 shape: the router routes
	// `implemented` → verify.
	statusPath := filepath.Join(relDir, sitSliceID, "status.json")
	st := &state.Status{
		Schema:        "https://baton.sawy3r.net/schemas/slice-status-v1.json",
		SliceID:       sitSliceID,
		Release:       sitRelease,
		Track:         sitTrackID,
		State:         state.Implemented,
		Owner:         "sworn-run",
		LastUpdatedBy: "sit-fixture",
		LastUpdatedAt: time.Now().UTC().Format(time.RFC3339),
		StartCommit:   "",
		SpecPath:      filepath.Join("docs", "release", sitRelease, sitSliceID, "spec.md"),
		ProofPath:     filepath.Join("docs", "release", sitRelease, sitSliceID, "proof.md"),
		JournalPath:   filepath.Join("docs", "release", sitRelease, sitSliceID, "journal.md"),
		CoversNeeds:   []string{"N-01"},
		PlannedFiles:  []string{},
		TestCommands:  []string{"go test ./internal/run/ -run TestLoopSIT"},
		Verification:  state.Verification{Result: "pending"},
		// Human-ratified validation record (pin 2): the fixture carries a
		// reqvalidate record so implement.Run's Definition-of-Ready gate passes.
		Validation: state.ValidationRecord{
			HumanRatified:     true,
			RatifiedBy:        "sit-fixture (test author)",
			RatifiedAt:        "2026-07-11T00:00:00Z",
			PositiveScenarios: []string{"Cold board with the verified-path commit present: the loop drives S01-sit-slice to a committed verified state and terminates."},
			NegativeScenarios: []string{"Verified-path commit reverted: the committed track ref never advances to verified and the router re-dispatches the verify leg to the bounded deadline."},
			BenefitHypothesis: "Booting the assembled loop over a real cold-board fixture catches dead loop wiring in CI instead of shipping a DOA release.",
		},
	}
	if err := state.Write(statusPath, st); err != nil {
		t.Fatal(err)
	}

	runCmd(t, absRoot, "git", "add", "-A")
	runCmd(t, absRoot, "git", "commit", "-m", "sit: hermetic fixture release (cold board)")
	return absRoot, releaseWT, trackWT
}

// copySITFixture copies internal/run/testdata/sit-fixture/* into destDir,
// substituting the __RELEASE_WT__ token in board.json with releaseWT.
func copySITFixture(t *testing.T, destDir, releaseWT string) {
	t.Helper()
	srcRoot := filepath.Join("testdata", "sit-fixture")
	err := filepath.Walk(srcRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(destDir, rel)
		if info.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if filepath.Base(path) == "board.json" {
			data = []byte(strings.ReplaceAll(string(data), "__RELEASE_WT__", releaseWT))
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy SIT fixture: %v", err)
	}
}

// sitCommittedState returns the `state` field of the status.json COMMITTED on
// the track ref — the authoritative, router-visible state. It deliberately does
// NOT read the worktree file: the worktree status.json is `verified` in both the
// fixed and the buggy case, so only the committed ref distinguishes them.
func sitCommittedState(t *testing.T, absRoot string) string {
	t.Helper()
	ref := sitTrackBranch + ":docs/release/" + sitRelease + "/" + sitSliceID + "/status.json"
	cmd := exec.Command("git", "show", ref)
	cmd.Dir = absRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("read committed status.json from %q: %v\n%s", ref, err, out)
	}
	var st struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(out, &st); err != nil {
		t.Fatalf("parse committed status.json: %v\n%s", err, out)
	}
	return st.State
}

// sitBoardDump renders the track branch's recent history and committed
// status.json for AC-04's on-stall / on-failure diagnostic.
func sitBoardDump(t *testing.T, absRoot string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("── SIT board dump ──\n")
	for _, args := range [][]string{
		{"log", "--oneline", "-n", "25", sitTrackBranch},
		{"show", sitTrackBranch + ":docs/release/" + sitRelease + "/" + sitSliceID + "/status.json"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = absRoot
		out, _ := cmd.CombinedOutput()
		b.WriteString("$ git " + strings.Join(args, " ") + "\n")
		b.Write(out)
		b.WriteString("\n")
	}
	return b.String()
}
