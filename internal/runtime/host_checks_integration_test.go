package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
)

// hostCheckFixture builds a production fixture whose slice contract declares
// host_checks, with the slice advanced to the implement stage. It exercises
// the real host runner: approved contract commands run through the fixed sh -c
// surface against a real candidate commit and are journaled exactly-once.
type hostCheckFixture struct {
	ctx          context.Context
	manifest     admittedManifest
	store        *journal.Store
	owner        journal.OwnerLease
	service      *Service
	engine       *engine
	state        protocol.State
	plan         protocol.Plan
	targetHead   string
	releaseHead  string
	candidate    string
	hostChecks   []string
	contractDgst string
}

func newHostCheckFixture(t *testing.T, hostChecks []string, manifestPlan ...bool) *hostCheckFixture {
	t.Helper()
	ctx := context.Background()
	repository := productionRepository(t)
	config := productionConfig(t)
	manifest := productionManifest(t, repository, config)
	production, err := newProductionDriverRuntime(config, driver.DriverFactoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 11, 5, 6, 7, 0, time.UTC)
	store, err := journal.Open(ctx, filepath.Join(t.TempDir(), "journal.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.RegisterRun(ctx, journal.Run{
		ID: manifest.value.RunID, ManifestDigest: manifest.digest,
		Repository: manifest.value.Repository,
		Release:    manifest.value.Release, TargetRef: manifest.value.TargetRef,
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	owner, err := store.AcquireOwner(ctx, manifest.value.RunID, now, time.Minute, false)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{journal: store, production: production,
		gitExecutable: gitExecutable, now: func() time.Time { return now }}
	engine, err := service.openEngine(manifest)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	planBytes := hostChecksPlanBytes(t, manifest, hostChecks)
	var overlay map[string][]byte
	if len(manifestPlan) > 0 && manifestPlan[0] {
		inline, err := protocol.ParsePlan(planBytes)
		if err != nil {
			t.Fatal(err)
		}
		metadata := inline.Metadata()
		slice := metadata.Tracks[0].Slices[0]
		contract := mustJSON(map[string]any{
			"outcome": slice.Outcome, "scope": slice.Scope, "acceptance": slice.Acceptance,
			"checks": slice.Checks, "host_checks": slice.HostChecks, "constraints": slice.Constraints,
			"depends_on": slice.DependsOn, "consumes": slice.Consumes,
		})
		_, digest, err := protocol.ParseSliceContract(contract, "S1", "T1")
		if err != nil {
			t.Fatal(err)
		}
		overlay = map[string][]byte{"contracts/S1.json": contract}
		body := mustJSON(map[string]any{
			"schema_version": "sworn.release-manifest/v1", "release": metadata.Release,
			"revision": 1, "previous_plan": nil, "repository": metadata.Repository,
			"target_ref": metadata.TargetRef, "approval_ref": metadata.ApprovalRef,
			"tracks": []any{map[string]any{"id": "T1", "depends_on": []string{}, "slices": []any{
				map[string]any{"id": "S1", "outcome": slice.Outcome, "contract_path": "contracts/S1.json", "digest": digest, "depends_on": []string{}, "consumes": []string{}, "touchpoints": []string{"one.txt"}},
			}}},
		})
		planBytes = []byte("```sworn-release-manifest-v1\n" + string(body) + "```\n\nHost-check fixture.\n")
	}
	plan, err := protocol.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.actions.RecordPlanRevision(protocol.RecordPlanRevisionInput{
		PlanBytes:       planBytes,
		ContractOverlay: overlay,
		Summary:         "Install the host-checks fixture plan.",
		Detail:          []byte("Host-checks fixture."),
	}); err != nil {
		t.Fatal(err)
	}
	for _, receipt := range []protocol.AppendReceiptInput{
		{
			Release: manifest.value.Release, Slice: "S1",
			Role: "implementer", Result: "designed",
			Summary: "Design the host-checks fixture.",
			Detail:  []byte("Exact design."),
		},
		{
			Release: manifest.value.Release, Slice: "S1",
			Role: "lead", Result: "proceed",
			Summary: "Proceed with the host-checks fixture.",
			Detail:  []byte("Exact review."),
		},
	} {
		if _, err := engine.actions.AppendReceipt(receipt); err != nil {
			t.Fatal(err)
		}
	}
	state, err := protocol.ReadState(engine.git, manifest.value.Release, engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	slice, sliceOK := state.Slice("S1")
	track, trackOK := state.Track("T1")
	if !sliceOK || !trackOK || slice.CurrentReceipt == nil ||
		slice.Stage != "implement" || slice.NextRole != "implementer" {
		t.Fatalf("implementation authority = %#v", state)
	}
	resolved, err := plan.ResolveSliceContractAtHead(engine.git, "S1", state.Refs.Release.Head, state.Refs.Target.Head)
	if err != nil {
		t.Fatal(err)
	}
	contractDigest, ok := plan.Contract("S1")
	if !ok {
		t.Fatal("contract digest absent")
	}
	return &hostCheckFixture{
		ctx: ctx, manifest: manifest, store: store, owner: owner,
		service: service, engine: engine, state: state, plan: plan,
		targetHead: state.Refs.Target.Head, releaseHead: state.Refs.Release.Head,
		candidate:  track.Head,
		hostChecks: resolved.HostChecks, contractDgst: contractDigest,
	}
}

func hostChecksPlanBytes(t *testing.T, manifest admittedManifest, hostChecks []string) []byte {
	t.Helper()
	// The contract's checks list must contain every declared host check plus
	// one worker-runnable check, so the fixture exercises the mixed case.
	checks := append(append([]string(nil), hostChecks...), "worker check")
	metadata := protocol.Metadata{
		SchemaVersion: protocol.PlanVersion,
		Release:       manifest.value.Release,
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    manifest.value.Authority.Project,
		TargetRef:     manifest.value.TargetRef,
		ApprovalRef:   "operator://" + manifest.value.Release + "/1",
		Tracks: []protocol.Track{{
			ID: "T1", DependsOn: []string{},
			Slices: []protocol.Slice{{
				ID: "S1", Outcome: "Deliver host-checked S1.",
				Scope:       protocol.Scope{Include: []string{"one.txt"}, Exclude: []string{}},
				Acceptance:  []protocol.Criterion{{ID: "A1", Text: "S1 is exact."}},
				Checks:      checks,
				HostChecks:  hostChecks,
				Constraints: []string{"deterministic"},
				DependsOn:   []string{}, Consumes: []string{},
			}},
		}},
	}
	body, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return []byte("```protocol-plan-v2\n" + string(body) + "\n```\n\nHost-checks fixture plan.\n")
}

// A1: the engine executes a declared host check against the exact candidate,
// journals its exit code and bounded output as a durable effect bound to the
// exact slice, candidate and contract digest, and a second invocation reuses
// the succeeded effect exactly-once.
func TestHostCheckExecutionJournalsAndBindsExactlyOnce(t *testing.T) {
	fixture := newHostCheckFixture(t, []string{"printf 'host ok\\n'"})
	results, err := fixture.service.runHostChecks(
		fixture.ctx, fixture.engine, fixture.owner, fixture.plan,
		"S1", fixture.candidate, fixture.targetHead, fixture.releaseHead)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	result := results[0]
	if result.Outcome != protocol.CheckOutcomePass || result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(result.Output, "host ok") {
		t.Fatalf("output = %q", result.Output)
	}
	if result.Slice != "S1" || result.Candidate != fixture.candidate ||
		result.ContractDigest != fixture.contractDgst {
		t.Fatalf("binding = %#v", result)
	}
	work := hostCheckWork("S1", fixture.candidate, fixture.contractDgst, "printf 'host ok\\n'")
	effectID := hostCheckEffectID(work)
	effect, err := fixture.store.Effect(fixture.ctx, fixture.manifest.value.RunID, effectID)
	if err != nil {
		t.Fatal(err)
	}
	if effect.Kind != "check.host" || effect.State != journal.Succeeded {
		t.Fatalf("effect = %#v", effect)
	}
	// Exactly-once: a second invocation of the same candidate reuses the
	// succeeded effect and returns identical evidence without a new run.
	again, err := fixture.service.runHostChecks(
		fixture.ctx, fixture.engine, fixture.owner, fixture.plan,
		"S1", fixture.candidate, fixture.targetHead, fixture.releaseHead)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 || again[0].EffectID != effectID ||
		again[0].OutputDigest != result.OutputDigest {
		t.Fatalf("reused result = %#v", again)
	}
	effectAfter, err := fixture.store.Effect(fixture.ctx, fixture.manifest.value.RunID, effectID)
	if err != nil {
		t.Fatal(err)
	}
	if effectAfter.State != journal.Succeeded || effectAfter.CurrentClaim != "" {
		t.Fatalf("effect was re-run: %#v", effectAfter)
	}
}

// A2: the engine-built receipt manifest carries explicit host_boundary
// provenance that cannot be mistaken for role evidence, and a verifier receipt
// cannot bind a manifest whose host entry was substituted.
func TestHostCheckManifestProvenanceCannotBeSubstituted(t *testing.T) {
	fixture := newHostCheckFixture(t, []string{"printf 'host ok\\n'"})
	results, err := fixture.service.runHostChecks(
		fixture.ctx, fixture.engine, fixture.owner, fixture.plan,
		"S1", fixture.candidate, fixture.targetHead, fixture.releaseHead)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := buildHostCheckResultsManifest(
		fixture.manifest.value.Release, "S1", 1, fixture.candidate,
		fixture.contractDgst, results, "sha256:"+strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := protocol.ParseCheckResults(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Entries[0].Provenance != protocol.CheckProvenanceHost {
		t.Fatalf("host entry provenance = %q", parsed.Entries[0].Provenance)
	}
	// A host entry relabelled as role evidence must fail closed.
	substituted := strings.Replace(
		string(manifest),
		`"provenance":"host_boundary"`,
		`"provenance":"role"`,
		1,
	)
	if _, err := protocol.ParseCheckResults([]byte(substituted)); err == nil {
		t.Fatal("relabelled host evidence was accepted")
	}
}

// A3: the runner's public surface takes a check identity, never a command; an
// identity the approved contract did not declare is refused and the refusal is
// journaled as a durable effect with no command executed.
func TestHostCheckRunnerRefusesUndeclaredIdentityAndJournalsRefusal(t *testing.T) {
	fixture := newHostCheckFixture(t, []string{"printf 'host ok\\n'"})
	_, err := fixture.service.runOneHostCheck(
		fixture.ctx, fixture.engine, fixture.owner, fixture.plan,
		"S1", fixture.candidate, fixture.targetHead, fixture.releaseHead, "rm -rf /")
	if !IsCode(err, "HOST_CHECK_NOT_DECLARED") {
		t.Fatalf("undeclared check error = %v", err)
	}
	refusalWork := hostCheckRefusalWork(
		"S1", fixture.candidate, "rm -rf /",
		"check is not declared as a containment-requiring check in the approved contract")
	effect, err := fixture.store.Effect(
		fixture.ctx, fixture.manifest.value.RunID, hostCheckEffectID(refusalWork))
	if err != nil {
		t.Fatal(err)
	}
	if effect.Kind != "check.refused" || effect.State != journal.Succeeded {
		t.Fatalf("refusal effect = %#v", effect)
	}
	// No check.host effect may exist for the refused identity.
	if _, err := fixture.store.Effect(fixture.ctx, fixture.manifest.value.RunID,
		hostCheckEffectID(hostCheckWork("S1", fixture.candidate, fixture.contractDgst, "rm -rf /"))); err == nil {
		t.Fatal("refused identity was executed")
	}
}

// A4: a host check that times out or overflows is recorded as a failure with
// its diagnostic, never as a pass or as absent, and blocks the seal.
func TestHostCheckRunnerRecordsOverflowAsFailure(t *testing.T) {
	fixture := newHostCheckFixture(t, []string{"yes x | head -c 1000000"})
	_, err := fixture.service.runHostChecks(
		fixture.ctx, fixture.engine, fixture.owner, fixture.plan,
		"S1", fixture.candidate, fixture.targetHead, fixture.releaseHead)
	if !IsCode(err, "HOST_CHECK_FAILED") {
		t.Fatalf("overflow error = %v", err)
	}
	work := hostCheckWork("S1", fixture.candidate, fixture.contractDgst, "yes x | head -c 1000000")
	effect, err := fixture.store.Effect(fixture.ctx, fixture.manifest.value.RunID, hostCheckEffectID(work))
	if err != nil {
		t.Fatal(err)
	}
	if effect.State != journal.Succeeded {
		t.Fatalf("effect state = %q", effect.State)
	}
	var recorded hostCheckResult
	if err := json.Unmarshal(effect.Result, &recorded); err != nil {
		t.Fatal(err)
	}
	if recorded.Outcome != protocol.CheckOutcomeOverflow ||
		!strings.Contains(recorded.Output, protocol.HostCheckTruncationPrefix) {
		t.Fatalf("recorded = %#v", recorded)
	}
}

// Recovery: a claimed check.host effect left by a crash is re-run and
// completed by the recovery sweep instead of stranding the seal.
func TestHostCheckRecoveryCompletesClaimedEffect(t *testing.T) {
	fixture := newHostCheckFixture(t, []string{"printf 'recovered\\n'"})
	work := hostCheckWork("S1", fixture.candidate, fixture.contractDgst, "printf 'recovered\\n'")
	effectID := hostCheckEffectID(work)
	command := hostCheckCommand{
		SchemaVersion: hostCheckSchemaVersion, Slice: "S1",
		Candidate: fixture.candidate, ContractDigest: fixture.contractDgst,
		Check: "printf 'recovered\\n'", OutputBytes: hostCheckOutputBytes,
		TimeoutMillis: 30_000,
	}
	payload := mustJSON(command)
	if err := fixture.store.EnsureAttempt(fixture.ctx,
		journal.Command{RunID: fixture.owner.RunID, ReplayKey: effectID,
			Kind: "check.host", Payload: payload, CreatedAt: fixture.service.now().UTC()},
		journal.Effect{RunID: fixture.owner.RunID, ID: effectID, ReplayKey: effectID,
			Kind: "check.host", BeforeDigest: work,
			ExpectedDigest: sha256Digest(payload), UpdatedAt: fixture.service.now().UTC()},
		journal.EffectAttempt{WorkID: work, Epoch: 1, Try: 1}); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash between claim and completion: the effect is Claimed and
	// no result exists yet.
	claim, err := fixture.store.ClaimOwned(
		fixture.ctx, fixture.owner, effectID, fixture.service.now().UTC(), effectLease)
	if err != nil {
		t.Fatal(err)
	}
	effect, err := fixture.store.Effect(fixture.ctx, fixture.owner.RunID, effectID)
	if err != nil {
		t.Fatal(err)
	}
	if effect.State != journal.Claimed || effect.CurrentClaim != claim.Token {
		t.Fatalf("claim = %#v", effect)
	}
	// The recovery sweep re-runs the exact approved command and completes the
	// claimed effect.
	if _, err := fixture.service.executeHostCheckFromRecovery(
		fixture.ctx, fixture.engine, fixture.owner, effect, command); err != nil {
		t.Fatal(err)
	}
	after, err := fixture.store.Effect(fixture.ctx, fixture.owner.RunID, effectID)
	if err != nil {
		t.Fatal(err)
	}
	if after.State != journal.Succeeded {
		t.Fatalf("recovered effect = %#v", after)
	}
	var recorded hostCheckResult
	if err := json.Unmarshal(after.Result, &recorded); err != nil {
		t.Fatal(err)
	}
	if recorded.Outcome != protocol.CheckOutcomePass ||
		!strings.Contains(recorded.Output, "recovered") {
		t.Fatalf("recovered result = %#v", recorded)
	}
}

// countingHostCheck returns a shell check that records how many times it has
// executed in counter and exits 0 only from the passAfter-th execution on
// (never, when passAfter is 0). The counter lives outside the candidate
// snapshot, so every execution of the same candidate sees it.
func countingHostCheck(counter string, passAfter int, prelude string) string {
	return fmt.Sprintf(
		"n=$(cat %s 2>/dev/null || echo 0); n=$((n+1)); echo $n > %s; %s [ %d -gt 0 ] && [ $n -ge %d ]",
		counter, counter, prelude, passAfter, passAfter,
	)
}

func hostCheckExecutions(t *testing.T, counter string) int {
	t.Helper()
	body, err := os.ReadFile(counter)
	if err != nil {
		return 0
	}
	var value int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(body)), "%d", &value); err != nil {
		t.Fatal(err)
	}
	return value
}

// #296: an unchanged candidate whose recorded host-check failure carries no
// deterministic signature is re-executed exactly once; the re-execution is
// journaled under its own identity naming the record it replaced, and every
// reader of host evidence sees the re-execution's outcome.
func TestHostCheckPlainFailureIsReExecutedOnceForTheSameCandidate(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "runs")
	check := countingHostCheck(counter, 2, "")
	fixture := newHostCheckFixture(t, []string{check})
	run := func() ([]hostCheckResult, error) {
		return fixture.service.runHostChecks(
			fixture.ctx, fixture.engine, fixture.owner, fixture.plan,
			"S1", fixture.candidate, fixture.targetHead, fixture.releaseHead)
	}
	if _, err := run(); !IsCode(err, "HOST_CHECK_FAILED") {
		t.Fatalf("first execution = %v, want HOST_CHECK_FAILED", err)
	}
	if got := hostCheckExecutions(t, counter); got != 1 {
		t.Fatalf("executions after the first seal = %d", got)
	}
	work := hostCheckWork("S1", fixture.candidate, fixture.contractDgst, check)

	results, err := run()
	if err != nil || len(results) != 1 {
		t.Fatalf("re-execution = %#v, %v", results, err)
	}
	rerun := results[0]
	if rerun.Outcome != protocol.CheckOutcomePass ||
		rerun.EffectID != hostCheckRerunEffectID(work) ||
		rerun.RerunOf != hostCheckEffectID(work) {
		t.Fatalf("re-execution result = %#v", rerun)
	}
	if got := hostCheckExecutions(t, counter); got != 2 {
		t.Fatalf("executions after the re-execution = %d", got)
	}
	effect, err := fixture.store.Effect(fixture.ctx, fixture.manifest.value.RunID, hostCheckRerunEffectID(work))
	if err != nil || effect.State != journal.Succeeded || effect.BeforeDigest != hostCheckRerunWork(work) {
		t.Fatalf("re-execution effect = %#v, %v", effect, err)
	}
	var command hostCheckCommand
	if err := json.Unmarshal(fixture.commandPayload(t, hostCheckRerunEffectID(work)), &command); err != nil ||
		command.RerunOf != hostCheckEffectID(work) {
		t.Fatalf("re-execution command = %#v, %v", command, err)
	}
	first, err := fixture.store.Effect(fixture.ctx, fixture.manifest.value.RunID, hostCheckEffectID(work))
	if err != nil || first.State != journal.Succeeded {
		t.Fatalf("first record must stay intact: %#v, %v", first, err)
	}

	// A further identical resubmission replays the re-execution; nothing
	// runs a third time.
	again, err := run()
	if err != nil || len(again) != 1 || again[0].EffectID != rerun.EffectID {
		t.Fatalf("replay = %#v, %v", again, err)
	}
	if got := hostCheckExecutions(t, counter); got != 2 {
		t.Fatalf("executions after the replay = %d", got)
	}

	// The verifier's evidence reader and the receipt manifest both speak
	// for the re-execution.
	journaled, err := readJournaledHostResults(
		fixture.ctx, fixture.engine, "S1", fixture.candidate, fixture.contractDgst, []string{check})
	if err != nil || len(journaled) != 1 || journaled[0].EffectID != rerun.EffectID ||
		journaled[0].Outcome != protocol.CheckOutcomePass {
		t.Fatalf("journaled host results = %#v, %v", journaled, err)
	}
}

// #296: a deterministic failure is re-executed once and then stands; a
// third identical resubmission replays the re-execution's failure without
// running the check.
func TestHostCheckDeterministicFailureStandsAfterOneReExecution(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "runs")
	check := countingHostCheck(counter, 0, "")
	fixture := newHostCheckFixture(t, []string{check})
	run := func() error {
		_, err := fixture.service.runHostChecks(
			fixture.ctx, fixture.engine, fixture.owner, fixture.plan,
			"S1", fixture.candidate, fixture.targetHead, fixture.releaseHead)
		return err
	}
	for i, want := range []int{1, 2, 2} {
		if err := run(); !IsCode(err, "HOST_CHECK_FAILED") {
			t.Fatalf("seal %d = %v, want HOST_CHECK_FAILED", i+1, err)
		}
		if got := hostCheckExecutions(t, counter); got != want {
			t.Fatalf("executions after seal %d = %d, want %d", i+1, got, want)
		}
	}
}

// #296: a recorded failure carrying a deterministic signature (a data
// race, a build failure) is never re-executed, and neither is a timeout or
// an overflow.
func TestHostCheckSignedFailureIsNeverReExecuted(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "runs")
	check := countingHostCheck(counter, 2, "echo 'WARNING: DATA RACE';")
	fixture := newHostCheckFixture(t, []string{check})
	for i := 0; i < 2; i++ {
		_, err := fixture.service.runHostChecks(
			fixture.ctx, fixture.engine, fixture.owner, fixture.plan,
			"S1", fixture.candidate, fixture.targetHead, fixture.releaseHead)
		if !IsCode(err, "HOST_CHECK_FAILED") {
			t.Fatalf("seal %d = %v, want HOST_CHECK_FAILED", i+1, err)
		}
	}
	if got := hostCheckExecutions(t, counter); got != 1 {
		t.Fatalf("a race-signed failure was re-executed: %d executions", got)
	}
	for _, outcome := range []string{protocol.CheckOutcomeTimeout, protocol.CheckOutcomeOverflow} {
		if hostCheckRerunEligible(hostCheckResult{Outcome: outcome}) {
			t.Fatalf("%s is re-execution eligible", outcome)
		}
	}
	if hostCheckRerunEligible(hostCheckResult{Outcome: protocol.CheckOutcomeFail, Output: "FAIL\tpkg [build failed]"}) {
		t.Fatal("a build failure is re-execution eligible")
	}
	if hostCheckRerunEligible(hostCheckResult{Outcome: protocol.CheckOutcomeFail, RerunOf: "x"}) {
		t.Fatal("a re-execution is itself re-execution eligible")
	}
	if !hostCheckRerunEligible(hostCheckResult{Outcome: protocol.CheckOutcomeFail, Output: "--- FAIL: TestFlaky (0.01s)"}) {
		t.Fatal("a plain failure is not re-execution eligible")
	}
}

func (fixture *hostCheckFixture) commandPayload(t *testing.T, effectID string) []byte {
	t.Helper()
	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range snapshot.Commands {
		if command.ReplayKey == effectID && command.Kind == "check.host" {
			return command.Payload
		}
	}
	t.Fatalf("no check.host command journaled for %s", effectID)
	return nil
}
