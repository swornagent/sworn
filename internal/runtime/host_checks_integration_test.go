package runtime

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
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
	state        baton.State
	plan         baton.Plan
	targetHead   string
	releaseHead  string
	candidate    string
	hostChecks   []string
	contractDgst string
}

func newHostCheckFixture(t *testing.T, hostChecks []string) *hostCheckFixture {
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
	plan, err := baton.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.actions.RecordPlanRevision(baton.RecordPlanRevisionInput{
		PlanBytes: planBytes,
		Summary:   "Install the host-checks fixture plan.",
		Detail:    []byte("Host-checks fixture."),
	}); err != nil {
		t.Fatal(err)
	}
	for _, receipt := range []baton.AppendReceiptInput{
		{
			Release: manifest.value.Release, Slice: "S1",
			Role: "implementer", Result: "designed",
			Summary: "Design the host-checks fixture.",
			Detail:  []byte("Exact design."),
		},
		{
			Release: manifest.value.Release, Slice: "S1",
			Role: "captain", Result: "proceed",
			Summary: "Proceed with the host-checks fixture.",
			Detail:  []byte("Exact review."),
		},
	} {
		if _, err := engine.actions.AppendReceipt(receipt); err != nil {
			t.Fatal(err)
		}
	}
	state, err := baton.ReadState(engine.git, manifest.value.Release, engine.inertness)
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
	metadata := baton.Metadata{
		SchemaVersion: baton.PlanVersion,
		Release:       manifest.value.Release,
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    manifest.value.Authority.Project,
		TargetRef:     manifest.value.TargetRef,
		ApprovalRef:   "operator://" + manifest.value.Release + "/1",
		Tracks: []baton.Track{{
			ID: "T1", DependsOn: []string{},
			Slices: []baton.Slice{{
				ID: "S1", Outcome: "Deliver host-checked S1.",
				Scope:       baton.Scope{Include: []string{"one.txt"}, Exclude: []string{}},
				Acceptance:  []baton.Criterion{{ID: "A1", Text: "S1 is exact."}},
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
	return []byte("```baton-plan-v2\n" + string(body) + "\n```\n\nHost-checks fixture plan.\n")
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
	if result.Outcome != baton.CheckOutcomePass || result.ExitCode != 0 {
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
	parsed, err := baton.ParseCheckResults(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Entries[0].Provenance != baton.CheckProvenanceHost {
		t.Fatalf("host entry provenance = %q", parsed.Entries[0].Provenance)
	}
	// A host entry relabelled as role evidence must fail closed.
	substituted := strings.Replace(
		string(manifest),
		`"provenance":"host_boundary"`,
		`"provenance":"role"`,
		1,
	)
	if _, err := baton.ParseCheckResults([]byte(substituted)); err == nil {
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
	if recorded.Outcome != baton.CheckOutcomeOverflow ||
		!strings.Contains(recorded.Output, baton.HostCheckTruncationPrefix) {
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
	if recorded.Outcome != baton.CheckOutcomePass ||
		!strings.Contains(recorded.Output, "recovered") {
		t.Fatalf("recovered result = %#v", recorded)
	}
}
