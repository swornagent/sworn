package runtime

import (
	"context"
	"encoding/json"
	"errors"
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

// A1 (S1-seal-time-gates): declared quick checks run before any long suite,
// and a failing quick check blocks the seal before a single long-suite
// check.host effect is ever journaled for that candidate, regardless of the
// checks' declared order.
func TestHostCheckExecutionRunsQuickChecksBeforeLongSuitesAndBlocksOnQuickFailure(t *testing.T) {
	longCheck := "echo 'go test ./...' && false"
	quickCheck := "false"
	fixture := newHostCheckFixture(t, []string{longCheck, quickCheck})
	_, err := fixture.service.runHostChecks(
		fixture.ctx, fixture.engine, fixture.owner, fixture.plan,
		"S1", fixture.candidate, fixture.targetHead, fixture.releaseHead)
	if !IsCode(err, "HOST_CHECK_FAILED") {
		t.Fatalf("expected HOST_CHECK_FAILED, got %v", err)
	}
	var failure *hostCheckFailure
	if !errors.As(err, &failure) || failure.result.Check != quickCheck {
		t.Fatalf("expected the quick check to fail first, got %#v", failure)
	}
	// No check.host effect may exist for the long-suite check: the quick
	// check's failure returned before phaseOrderedHostChecks's long-suite
	// tail was ever reached.
	if _, err := fixture.store.Effect(fixture.ctx, fixture.manifest.value.RunID,
		hostCheckEffectID(hostCheckWork("S1", fixture.candidate, fixture.contractDgst, longCheck))); err == nil {
		t.Fatal("a long-suite check ran before its quick sibling was proven")
	}
}

// A1: reordering into quick-then-long changes only iteration order, never a
// check's own identity, so the same check reused across the implementer seal
// call site and a later verifier dispatch call site still creates exactly
// one check.host effect.
func TestPhaseOrderedHostChecksPreservesEachGroupsDeclaredOrder(t *testing.T) {
	declared := []string{
		"GOFLAGS=-buildvcs=false go test -count=1 ./...",
		"go vet ./...",
		"GOFLAGS=-buildvcs=false go test -count=1 -race ./...",
		"test -z \"$(gofmt -l .)\"",
	}
	ordered := phaseOrderedHostChecks(declared)
	want := []string{
		"go vet ./...",
		"test -z \"$(gofmt -l .)\"",
		"GOFLAGS=-buildvcs=false go test -count=1 ./...",
		"GOFLAGS=-buildvcs=false go test -count=1 -race ./...",
	}
	if len(ordered) != len(want) {
		t.Fatalf("phaseOrderedHostChecks(%v) = %v, want %v", declared, ordered, want)
	}
	for i := range want {
		if ordered[i] != want[i] {
			t.Fatalf("phaseOrderedHostChecks(%v) = %v, want %v", declared, ordered, want)
		}
	}
}

func TestIsLongSuiteHostCheckMatchesOnlyGoTestInvocations(t *testing.T) {
	tests := []struct {
		check string
		long  bool
	}{
		{"GOFLAGS=-buildvcs=false go test -count=1 ./cmd/sworn", true},
		{"GOFLAGS=-buildvcs=false go test -count=1 -parallel=1 -timeout=60m ./test/e2e", true},
		{"GOFLAGS=-buildvcs=false go test -count=1 -race -timeout=20m ./internal/...", true},
		{"GOFLAGS=-buildvcs=false go vet ./...", false},
		{"test -z \"$(gofmt -l ./cmd ./internal ./tools)\"", false},
		{"go mod tidy -diff", false},
		{"git diff --check", false},
		{"GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 GOFLAGS=-buildvcs=false go build ./...", false},
	}
	for _, tc := range tests {
		if got := isLongSuiteHostCheck(tc.check); got != tc.long {
			t.Fatalf("isLongSuiteHostCheck(%q) = %v, want %v", tc.check, got, tc.long)
		}
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

func recordFixtureManifestCommand(t *testing.T, fixture *hostCheckFixture) {
	t.Helper()
	if err := fixture.store.RecordCommand(fixture.ctx, journal.Command{
		RunID: fixture.manifest.value.RunID, ReplayKey: "manifest", Kind: "start",
		Payload: fixture.manifest.raw, CreatedAt: fixture.service.now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

func buildRepairFromHostResult(t *testing.T, fixture *hostCheckFixture, result hostCheckResult, epoch, try int64) productionHostRepair {
	t.Helper()
	invocationID := fixture.manifest.value.RunID + "/S1/implementer_implementation/1/" + fmt.Sprintf("%d", epoch) + "/" + fmt.Sprintf("%d", try)
	checks, err := driver.NewCheckBytes([]byte("test checks\n"))
	if err != nil {
		t.Fatal(err)
	}
	repair := productionHostRepair{
		SchemaVersion: hostRepairVersion,
		Before:        "sha256:" + strings.Repeat("a", 64),
		Plan:          fixture.state.Plan.OID,
		PreparedBase:  fixture.candidate,
		ProductTree:   "sha256:" + strings.Repeat("b", 64),
		SourceEpoch:   epoch, SourceTry: try,
		Submission: driver.Submission{
			SchemaVersion:  driver.SubmissionSchemaVersion,
			InvocationID:   invocationID,
			Responsibility: driver.ImplementerImplementation,
			Summary:        "Test submission.",
			Detail:         "Test detail.",
			Checks:         checks,
		},
		FailedCheck: result,
	}
	if err := validateHostRepair(repair, invocationID, "S1"); err != nil {
		t.Fatalf("test repair does not validate: %v", err)
	}
	return repair
}

func journalHostFailedDispatchForFact(t *testing.T, fixture *hostCheckFixture, work string, epoch, try int64, repair productionHostRepair) string {
	t.Helper()
	effectID := journal.AttemptEffectID(work, epoch, try)
	payload := mustJSON(map[string]string{"work": work})
	if err := fixture.store.RecordCommandEffect(fixture.ctx, journal.Command{
		RunID: fixture.manifest.value.RunID, ReplayKey: effectID, Kind: "driver.dispatch",
		Payload: payload, CreatedAt: fixture.service.now().UTC(),
	}, journal.Effect{
		RunID: fixture.manifest.value.RunID, ID: effectID, ReplayKey: effectID,
		Kind: "driver.dispatch", State: journal.Pending,
		BeforeDigest: sha256Digest(payload), ExpectedDigest: "sha256:" + strings.Repeat("d", 64),
		UpdatedAt: fixture.service.now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	claim, err := fixture.store.Claim(fixture.ctx, fixture.manifest.value.RunID, effectID, fixture.service.now().UTC(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.Complete(fixture.ctx, journal.Completion{
		RunID: fixture.manifest.value.RunID, EffectID: effectID, Token: claim.Token,
		State: journal.OperationalFailed, ErrorCode: "HOST_CHECK_FAILED",
		Result:    mustJSON(repair),
		EventKind: "dispatch_operational_failure", EventBody: []byte("{}"), At: fixture.service.now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	return effectID
}

// S3-host-check-failure-facts A1: for the latest failed host check of a
// work, the status projection carries one bounded fact with the declared
// command, outcome and exit, rerun linkage, phase-ordered not-run checks
// and the excerpt from the stored result.
func TestHostCheckFailureFactDerivesCommandExitRerunNotRunExcerpt(t *testing.T) {
	first := "printf 'first ok\\n'"
	failing := "echo 'one.txt needs repair'; exit 7"
	third := "printf 'third\\n'"
	fixture := newHostCheckFixture(t, []string{first, failing, third})
	_, err := fixture.service.runHostChecks(
		fixture.ctx, fixture.engine, fixture.owner, fixture.plan,
		"S1", fixture.candidate, fixture.targetHead, fixture.releaseHead)
	var failure *hostCheckFailure
	if !errors.As(err, &failure) || failure.result.Check != failing {
		t.Fatalf("expected failure on %q, got %#v %v", failing, failure, err)
	}
	work := "sha256:" + strings.Repeat("c", 64)
	repair := buildRepairFromHostResult(t, fixture, failure.result, 1, 1)
	dispatchID := journalHostFailedDispatchForFact(t, fixture, work, 1, 1, repair)
	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	resolver := func(sliceID string) ([]string, string, bool) {
		if sliceID != "S1" {
			return nil, "", false
		}
		return fixture.hostChecks, fixture.contractDgst, true
	}
	facts := hostCheckFailureFactsForSnapshot(snapshot, resolver)
	fact := facts[dispatchID]
	if fact == nil {
		t.Fatalf("no fact for %s (facts %v)", dispatchID, facts)
	}
	if fact.SchemaVersion != HostCheckFailureFactSchemaVersion {
		t.Fatalf("schema = %q", fact.SchemaVersion)
	}
	if fact.Check != failing {
		t.Fatalf("check = %q, want %q", fact.Check, failing)
	}
	if fact.Outcome != protocol.CheckOutcomeFail || fact.ExitCode != 7 {
		t.Fatalf("outcome/exit = %q/%d", fact.Outcome, fact.ExitCode)
	}
	if fact.Reran {
		t.Fatalf("reran = true, want false (no re-execution yet)")
	}
	if fact.RerunOutcome != "" || fact.RerunExitCode != nil || fact.RerunEffect != "" {
		t.Fatalf("rerun fields set without re-execution: %#v", fact)
	}
	if fact.NotRunUnknown || len(fact.NotRun) != 1 || fact.NotRun[0] != third {
		t.Fatalf("not_run = %v unknown=%v, want [%q]", fact.NotRun, fact.NotRunUnknown, third)
	}
	wantExcerpt, wantTruncated := hostOutputExcerpt(failure.result.Output, failure.result.Truncated)
	if fact.Excerpt != wantExcerpt || fact.ExcerptTruncated != wantTruncated {
		t.Fatalf("excerpt = %q/%v, want %q/%v", fact.Excerpt, fact.ExcerptTruncated, wantExcerpt, wantTruncated)
	}
	if !strings.Contains(fact.Excerpt, "one.txt needs repair") {
		t.Fatalf("excerpt does not carry stored output: %q", fact.Excerpt)
	}
	if fact.OutputDigest != failure.result.OutputDigest {
		t.Fatalf("output_digest = %q, want %q", fact.OutputDigest, failure.result.OutputDigest)
	}
	if fact.HostEffect != failure.result.EffectID {
		t.Fatalf("host_effect = %q, want %q", fact.HostEffect, failure.result.EffectID)
	}
	if fact.Candidate != fixture.candidate || fact.ContractDigest != fixture.contractDgst {
		t.Fatalf("candidate/contract = %q/%q", fact.Candidate, fact.ContractDigest)
	}
	body, _ := json.Marshal(fact)
	if len(body) > HostCheckFailureFactMaxBytes {
		t.Fatalf("fact len %d exceeds %d", len(body), HostCheckFailureFactMaxBytes)
	}
	// Old journals (no repair, unparsable) report absent, never corrupt.
	emptyWork := "sha256:" + strings.Repeat("e", 64)
	emptyID := journal.AttemptEffectID(emptyWork, 1, 1)
	payload := mustJSON(map[string]string{"work": emptyWork})
	if err := fixture.store.RecordCommandEffect(fixture.ctx, journal.Command{
		RunID: fixture.manifest.value.RunID, ReplayKey: emptyID, Kind: "driver.dispatch",
		Payload: payload, CreatedAt: fixture.service.now().UTC(),
	}, journal.Effect{
		RunID: fixture.manifest.value.RunID, ID: emptyID, ReplayKey: emptyID,
		Kind: "driver.dispatch", State: journal.Pending,
		BeforeDigest: sha256Digest(payload), ExpectedDigest: "sha256:" + strings.Repeat("d", 64),
		UpdatedAt: fixture.service.now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	claim, err := fixture.store.Claim(fixture.ctx, fixture.manifest.value.RunID, emptyID, fixture.service.now().UTC(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.Complete(fixture.ctx, journal.Completion{
		RunID: fixture.manifest.value.RunID, EffectID: emptyID, Token: claim.Token,
		State: journal.OperationalFailed, ErrorCode: "HOST_CHECK_FAILED",
		EventKind: "dispatch_operational_failure", EventBody: []byte("{}"), At: fixture.service.now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	facts = hostCheckFailureFactsForSnapshot(snapshot, resolver)
	if _, found := facts[emptyID]; found {
		t.Fatal("empty repair produced a fact, want absent")
	}
}

// S3 A1 re-execution direction, both ways: a stored FailedCheck that is
// itself the re-execution result reads the first execution through RerunOf,
// and a first-execution FailedCheck with a succeeded re-execution in the
// snapshot reports it, without confusing the two outcomes.
func TestHostCheckFailureFactRerunBothDirections(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "runs")
	check := countingHostCheck(counter, 0, "")
	fixture := newHostCheckFixture(t, []string{check})
	run := func() *hostCheckFailure {
		_, err := fixture.service.runHostChecks(
			fixture.ctx, fixture.engine, fixture.owner, fixture.plan,
			"S1", fixture.candidate, fixture.targetHead, fixture.releaseHead)
		var failure *hostCheckFailure
		if !errors.As(err, &failure) {
			t.Fatalf("expected HOST_CHECK_FAILED, got %v", err)
		}
		return failure
	}
	firstFailure := run()
	if firstFailure.result.RerunOf != "" {
		t.Fatalf("first failure carries RerunOf: %#v", firstFailure.result)
	}
	rerunFailure := run()
	if rerunFailure.result.RerunOf == "" {
		t.Fatalf("second failure is not a re-execution: %#v", rerunFailure.result)
	}
	if rerunFailure.result.RerunOf != firstFailure.result.EffectID {
		t.Fatalf("rerun RerunOf %q != first effect %q", rerunFailure.result.RerunOf, firstFailure.result.EffectID)
	}
	resolver := func(sliceID string) ([]string, string, bool) {
		if sliceID != "S1" {
			return nil, "", false
		}
		return fixture.hostChecks, fixture.contractDgst, true
	}
	// FailedCheck is the re-execution result.
	rerunWork := "sha256:" + strings.Repeat("c", 64)
	rerunRepair := buildRepairFromHostResult(t, fixture, rerunFailure.result, 1, 2)
	rerunDispatchID := journalHostFailedDispatchForFact(t, fixture, rerunWork, 1, 2, rerunRepair)
	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	facts := hostCheckFailureFactsForSnapshot(snapshot, resolver)
	fact := facts[rerunDispatchID]
	if fact == nil {
		t.Fatalf("no fact for rerun dispatch %s", rerunDispatchID)
	}
	if !fact.Reran {
		t.Fatal("reran = false for stored re-execution, want true")
	}
	if fact.Outcome != firstFailure.result.Outcome || fact.ExitCode != firstFailure.result.ExitCode {
		t.Fatalf("first outcome/exit = %q/%d, want %q/%d", fact.Outcome, fact.ExitCode, firstFailure.result.Outcome, firstFailure.result.ExitCode)
	}
	if fact.RerunOutcome != rerunFailure.result.Outcome || fact.RerunExitCode == nil || *fact.RerunExitCode != rerunFailure.result.ExitCode {
		t.Fatalf("rerun outcome/exit = %q/%v, want %q/%d", fact.RerunOutcome, fact.RerunExitCode, rerunFailure.result.Outcome, rerunFailure.result.ExitCode)
	}
	if fact.HostEffect != firstFailure.result.EffectID || fact.RerunEffect != rerunFailure.result.EffectID {
		t.Fatalf("host/rerun effects = %q/%q", fact.HostEffect, fact.RerunEffect)
	}
	wantExcerpt, _ := hostOutputExcerpt(rerunFailure.result.Output, rerunFailure.result.Truncated)
	if fact.Excerpt != wantExcerpt {
		t.Fatal("excerpt is not from the stored re-execution result")
	}
	// FailedCheck is the first execution, with the succeeded re-execution
	// already in the snapshot.
	firstWork := "sha256:" + strings.Repeat("d", 64)
	firstRepair := buildRepairFromHostResult(t, fixture, firstFailure.result, 1, 1)
	firstDispatchID := journalHostFailedDispatchForFact(t, fixture, firstWork, 1, 1, firstRepair)
	snapshot, err = fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	facts = hostCheckFailureFactsForSnapshot(snapshot, resolver)
	firstFact := facts[firstDispatchID]
	if firstFact == nil {
		t.Fatalf("no fact for first dispatch %s", firstDispatchID)
	}
	if !firstFact.Reran {
		t.Fatal("reran = false for first with succeeded re-execution in snapshot, want true")
	}
	if firstFact.Outcome != firstFailure.result.Outcome {
		t.Fatalf("first outcome = %q", firstFact.Outcome)
	}
	if firstFact.RerunOutcome != rerunFailure.result.Outcome {
		t.Fatalf("rerun outcome = %q, want %q", firstFact.RerunOutcome, rerunFailure.result.Outcome)
	}
}

// S3 A1 not-run derivation and contract binding: not_run is the checks
// after the failed check's position in the phase order the engine used,
// resolved only when the plan contract equals the stored digest and
// contains the failed check; otherwise it is absent and marked unknown,
// and a resolution error never fails Status.
func TestHostCheckFailureFactNotRunPhaseOrderAndContractBinding(t *testing.T) {
	quickPass := "printf 'quick pass\\n'"
	quickFail := "false"
	quickAfter := "printf 'quick after\\n'"
	longSuite := "GOFLAGS=-buildvcs=false go test -count=1 ./..."
	fixture := newHostCheckFixture(t, []string{longSuite, quickPass, quickFail, quickAfter})
	_, err := fixture.service.runHostChecks(
		fixture.ctx, fixture.engine, fixture.owner, fixture.plan,
		"S1", fixture.candidate, fixture.targetHead, fixture.releaseHead)
	var failure *hostCheckFailure
	if !errors.As(err, &failure) || failure.result.Check != quickFail {
		t.Fatalf("expected failure on quick check, got %#v %v", failure, err)
	}
	work := "sha256:" + strings.Repeat("c", 64)
	repair := buildRepairFromHostResult(t, fixture, failure.result, 1, 1)
	dispatchID := journalHostFailedDispatchForFact(t, fixture, work, 1, 1, repair)
	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	phaseResolver := func(sliceID string) ([]string, string, bool) {
		if sliceID != "S1" {
			return nil, "", false
		}
		return fixture.hostChecks, fixture.contractDgst, true
	}
	facts := hostCheckFailureFactsForSnapshot(snapshot, phaseResolver)
	fact := facts[dispatchID]
	if fact == nil {
		t.Fatal("no fact for phase-ordered failure")
	}
	wantNotRun := []string{quickAfter, longSuite}
	if fact.NotRunUnknown || len(fact.NotRun) != len(wantNotRun) {
		t.Fatalf("not_run = %v unknown=%v, want %v", fact.NotRun, fact.NotRunUnknown, wantNotRun)
	}
	for index := range wantNotRun {
		if fact.NotRun[index] != wantNotRun[index] {
			t.Fatalf("not_run = %v, want %v", fact.NotRun, wantNotRun)
		}
	}
	// A different contract digest never guesses.
	wrongDigest := func(sliceID string) ([]string, string, bool) {
		return fixture.hostChecks, "sha256:" + strings.Repeat("9", 64), true
	}
	facts = hostCheckFailureFactsForSnapshot(snapshot, wrongDigest)
	fact = facts[dispatchID]
	if fact == nil || !fact.NotRunUnknown || len(fact.NotRun) != 0 {
		t.Fatalf("wrong-digest not_run = %v unknown=%v, want unknown", fact.NotRun, fact.NotRunUnknown)
	}
	// A contract that does not contain the failed check never guesses.
	missingCheck := func(sliceID string) ([]string, string, bool) {
		return []string{quickPass, quickAfter}, fixture.contractDgst, true
	}
	facts = hostCheckFailureFactsForSnapshot(snapshot, missingCheck)
	fact = facts[dispatchID]
	if fact == nil || !fact.NotRunUnknown {
		t.Fatalf("missing-check not_run = %v unknown=%v, want unknown", fact.NotRun, fact.NotRunUnknown)
	}
	// A resolution error reports unknown, never corrupt and never fails.
	failingResolver := func(sliceID string) ([]string, string, bool) {
		return nil, "", false
	}
	facts = hostCheckFailureFactsForSnapshot(snapshot, failingResolver)
	fact = facts[dispatchID]
	if fact == nil || !fact.NotRunUnknown {
		t.Fatalf("unresolved not_run = %v unknown=%v, want unknown", fact.NotRun, fact.NotRunUnknown)
	}
	if fact.Check != quickFail || fact.Outcome != protocol.CheckOutcomeFail {
		t.Fatalf("unresolved fact lost check/outcome: %#v", fact)
	}
}

// S3 A3: each check.host effect in the projection reports the check's
// outcome beside the effect state, so an executed-and-failed check no
// longer reads only as succeeded. Journal effect states are unchanged.
func TestCheckHostEffectReportsOutcomeBesideState(t *testing.T) {
	passing := "printf 'pass\\n'"
	failing := "exit 7"
	fixture := newHostCheckFixture(t, []string{passing, failing})
	_, err := fixture.service.runHostChecks(
		fixture.ctx, fixture.engine, fixture.owner, fixture.plan,
		"S1", fixture.candidate, fixture.targetHead, fixture.releaseHead)
	if !IsCode(err, "HOST_CHECK_FAILED") {
		t.Fatalf("expected HOST_CHECK_FAILED, got %v", err)
	}
	recordFixtureManifestCommand(t, fixture)
	status, err := fixture.service.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]EffectStatus, len(status.Effects))
	for _, effect := range status.Effects {
		byID[effect.ID] = effect
	}
	passWork := hostCheckWork("S1", fixture.candidate, fixture.contractDgst, passing)
	failWork := hostCheckWork("S1", fixture.candidate, fixture.contractDgst, failing)
	passStatus, ok := byID[hostCheckEffectID(passWork)]
	if !ok {
		t.Fatalf("no status for passing check.host effect (effects %v)", status.Effects)
	}
	if passStatus.State != string(journal.Succeeded) || passStatus.CheckOutcome != protocol.CheckOutcomePass {
		t.Fatalf("passing check status = %#v, want state succeeded + outcome pass", passStatus)
	}
	failStatus, ok := byID[hostCheckEffectID(failWork)]
	if !ok {
		t.Fatalf("no status for failing check.host effect")
	}
	if failStatus.State != string(journal.Succeeded) || failStatus.CheckOutcome != protocol.CheckOutcomeFail {
		t.Fatalf("failing check status = %#v, want state succeeded + outcome fail", failStatus)
	}
	snapshot, err := fixture.store.Snapshot(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	for _, effect := range snapshot.Effects {
		if effect.Kind != "check.host" {
			continue
		}
		if effect.State != journal.Succeeded {
			t.Fatalf("journal check.host state changed: %#v", effect)
		}
	}
	// Timeout and overflow variants, plus an assembly (slice "") effect,
	// bind through the journaled command payload the same way.
	timeoutWork := hostCheckWork("S1", fixture.candidate, fixture.contractDgst, "timeout-check")
	timeoutCommand := hostCheckCommand{
		SchemaVersion: hostCheckSchemaVersion, Slice: "S1",
		Candidate: fixture.candidate, ContractDigest: fixture.contractDgst,
		Check: "timeout-check", OutputBytes: hostCheckOutputBytes, TimeoutMillis: 1000,
	}
	timeoutResult := hostCheckResult{
		Slice: "S1", Candidate: fixture.candidate, ContractDigest: fixture.contractDgst,
		Check: "timeout-check", Outcome: protocol.CheckOutcomeTimeout, ExitCode: -1,
		Output: "timed out", OutputDigest: protocol.DigestBytes([]byte("timed out")),
		EffectID: hostCheckEffectID(timeoutWork),
	}
	journalDirectHostEffect(t, fixture, timeoutWork, timeoutCommand, timeoutResult)
	overflowWork := hostCheckWork("S1", fixture.candidate, fixture.contractDgst, "overflow-check")
	overflowCommand := hostCheckCommand{
		SchemaVersion: hostCheckSchemaVersion, Slice: "S1",
		Candidate: fixture.candidate, ContractDigest: fixture.contractDgst,
		Check: "overflow-check", OutputBytes: hostCheckOutputBytes, TimeoutMillis: 1000,
	}
	overflowResult := hostCheckResult{
		Slice: "S1", Candidate: fixture.candidate, ContractDigest: fixture.contractDgst,
		Check: "overflow-check", Outcome: protocol.CheckOutcomeOverflow, ExitCode: 0,
		Output: "overflowed", OutputDigest: protocol.DigestBytes([]byte("overflowed")),
		EffectID: hostCheckEffectID(overflowWork),
	}
	journalDirectHostEffect(t, fixture, overflowWork, overflowCommand, overflowResult)
	assemblyWork := assemblyHostCheckWork(fixture.candidate, fixture.contractDgst, "assembly-check")
	assemblyCommand := hostCheckCommand{
		SchemaVersion: hostCheckSchemaVersion, Slice: "",
		Candidate: fixture.candidate, ContractDigest: fixture.contractDgst,
		Check: "assembly-check", OutputBytes: hostCheckOutputBytes, TimeoutMillis: 1000,
	}
	assemblyResult := hostCheckResult{
		Slice: "", Candidate: fixture.candidate, ContractDigest: fixture.contractDgst,
		Check: "assembly-check", Outcome: protocol.CheckOutcomeFail, ExitCode: 3,
		Output: "assembly failed", OutputDigest: protocol.DigestBytes([]byte("assembly failed")),
		EffectID: hostCheckEffectID(assemblyWork),
	}
	journalDirectHostEffect(t, fixture, assemblyWork, assemblyCommand, assemblyResult)
	status, err = fixture.service.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	byID = make(map[string]EffectStatus, len(status.Effects))
	for _, effect := range status.Effects {
		byID[effect.ID] = effect
	}
	if got := byID[hostCheckEffectID(timeoutWork)].CheckOutcome; got != protocol.CheckOutcomeTimeout {
		t.Fatalf("timeout check outcome = %q", got)
	}
	if got := byID[hostCheckEffectID(overflowWork)].CheckOutcome; got != protocol.CheckOutcomeOverflow {
		t.Fatalf("overflow check outcome = %q", got)
	}
	if got := byID[hostCheckEffectID(assemblyWork)].CheckOutcome; got != protocol.CheckOutcomeFail {
		t.Fatalf("assembly check outcome = %q, want fail", got)
	}
	// A binding mismatch leaves the outcome absent, never corrupt.
	mismatchWork := hostCheckWork("S1", fixture.candidate, fixture.contractDgst, "mismatch-check")
	mismatchCommand := hostCheckCommand{
		SchemaVersion: hostCheckSchemaVersion, Slice: "S1",
		Candidate: fixture.candidate, ContractDigest: fixture.contractDgst,
		Check: "other-check", OutputBytes: hostCheckOutputBytes, TimeoutMillis: 1000,
	}
	mismatchResult := hostCheckResult{
		Slice: "S1", Candidate: fixture.candidate, ContractDigest: fixture.contractDgst,
		Check: "mismatch-check", Outcome: protocol.CheckOutcomeFail, ExitCode: 1,
		Output: "mismatch", OutputDigest: protocol.DigestBytes([]byte("mismatch")),
		EffectID: hostCheckEffectID(mismatchWork),
	}
	journalDirectHostEffect(t, fixture, mismatchWork, mismatchCommand, mismatchResult)
	status, err = fixture.service.Status(fixture.ctx, fixture.manifest.value.RunID)
	if err != nil {
		t.Fatal(err)
	}
	byID = make(map[string]EffectStatus, len(status.Effects))
	for _, effect := range status.Effects {
		byID[effect.ID] = effect
	}
	if got := byID[hostCheckEffectID(mismatchWork)].CheckOutcome; got != "" {
		t.Fatalf("mismatched check outcome = %q, want absent", got)
	}
}

func journalDirectHostEffect(t *testing.T, fixture *hostCheckFixture, work string, command hostCheckCommand, result hostCheckResult) {
	t.Helper()
	effectID := hostCheckEffectID(work)
	payload := mustJSON(command)
	body := mustJSON(result)
	now := fixture.service.now().UTC()
	if err := fixture.store.EnsureAttempt(fixture.ctx,
		journal.Command{RunID: fixture.manifest.value.RunID, ReplayKey: effectID,
			Kind: "check.host", Payload: payload, CreatedAt: now},
		journal.Effect{RunID: fixture.manifest.value.RunID, ID: effectID, ReplayKey: effectID,
			Kind: "check.host", BeforeDigest: work,
			ExpectedDigest: sha256Digest(payload), UpdatedAt: now},
		journal.EffectAttempt{WorkID: work, Epoch: 1, Try: 1}); err != nil {
		t.Fatal(err)
	}
	effect, err := fixture.store.Effect(fixture.ctx, fixture.manifest.value.RunID, effectID)
	if err != nil {
		t.Fatal(err)
	}
	if effect.State == journal.Succeeded {
		return
	}
	claim, err := fixture.store.ClaimOwned(fixture.ctx, fixture.owner, effectID, now, effectLease)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.CompleteOwned(fixture.ctx, fixture.owner, journal.Completion{
		RunID: fixture.manifest.value.RunID, EffectID: effectID, Token: claim.Token,
		State: journal.Succeeded, Result: body,
		Receipts:  []journal.Receipt{{Kind: "host_check_result", Body: body}},
		EventKind: "host_check_completed",
		EventBody: MarshalAssociation(EventAssociation{EffectID: effectID, WorkID: work, Slice: command.Slice}),
		At:        fixture.service.now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}
