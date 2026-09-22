package runtime

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
)

// assemblyHostEvidenceFixture drives a release whose slices declare host
// checks all the way to a prepared assembly candidate, through the real
// host runner and the real seal, so the assembly verification's projected
// roll-up (#343) is proven against candidate receipts the engine wrote.
type assemblyHostEvidenceFixture struct {
	ctx      context.Context
	manifest admittedManifest
	store    *journal.Store
	owner    journal.OwnerLease
	service  *Service
	engine   *engine
	plan     protocol.Plan
}

// newAssemblyHostEvidenceFixture installs a plan with the given track shape
// (each inner list is one track's serial slices). Every slice's scope is
// its own file (<slice>.txt) and declares hostChecks as host_checks.
func newAssemblyHostEvidenceFixture(
	t *testing.T,
	tracks [][]string,
	hostChecks []string,
) *assemblyHostEvidenceFixture {
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
	now := time.Date(2026, 9, 22, 5, 6, 7, 0, time.UTC)
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
	planBytes := assemblyHostChecksPlanBytes(t, manifest, tracks, hostChecks)
	plan, err := protocol.ParsePlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.actions.RecordPlanRevision(protocol.RecordPlanRevisionInput{
		PlanBytes: planBytes,
		Summary:   "Install the assembly host-evidence fixture plan.",
		Detail:    []byte("Assembly host-evidence fixture."),
	}); err != nil {
		t.Fatal(err)
	}
	return &assemblyHostEvidenceFixture{
		ctx: ctx, manifest: manifest, store: store, owner: owner,
		service: service, engine: engine, plan: plan,
	}
}

func assemblyHostChecksPlanBytes(
	t *testing.T,
	manifest admittedManifest,
	tracks [][]string,
	hostChecks []string,
) []byte {
	t.Helper()
	checks := append(append([]string(nil), hostChecks...), "worker check")
	planTracks := make([]protocol.Track, 0, len(tracks))
	for index, sliceIDs := range tracks {
		track := protocol.Track{
			ID: "T" + string(rune('1'+index)), DependsOn: []string{},
		}
		for _, sliceID := range sliceIDs {
			track.Slices = append(track.Slices, protocol.Slice{
				ID: sliceID, Outcome: "Deliver host-checked " + sliceID + ".",
				Scope: protocol.Scope{
					Include: []string{assemblyFixtureSliceFile(sliceID)},
					Exclude: []string{},
				},
				Acceptance:  []protocol.Criterion{{ID: "A-" + sliceID, Text: sliceID + " is exact."}},
				Checks:      checks,
				HostChecks:  hostChecks,
				Constraints: []string{"deterministic"},
				DependsOn:   []string{}, Consumes: []string{},
			})
		}
		planTracks = append(planTracks, track)
	}
	metadata := protocol.Metadata{
		SchemaVersion: protocol.PlanVersion,
		Release:       manifest.value.Release,
		Revision:      1,
		PreviousPlan:  nil,
		Repository:    manifest.value.Authority.Project,
		TargetRef:     manifest.value.TargetRef,
		ApprovalRef:   "operator://" + manifest.value.Release + "/1",
		Tracks:        planTracks,
	}
	body, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return []byte("```protocol-plan-v2\n" + string(body) + "\n```\n\nAssembly host-evidence fixture plan.\n")
}

func assemblyFixtureSliceFile(sliceID string) string {
	return strings.ToLower(sliceID) + ".txt"
}

func (f *assemblyHostEvidenceFixture) readState(t *testing.T) protocol.State {
	t.Helper()
	state, err := protocol.ReadState(f.engine.git, f.manifest.value.Release, f.engine.inertness)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

// designAndProceed appends the design and Lead proceed receipts that put a
// slice into the implement stage.
func (f *assemblyHostEvidenceFixture) designAndProceed(t *testing.T, sliceID string) {
	t.Helper()
	for _, receipt := range []protocol.AppendReceiptInput{
		{
			Release: f.manifest.value.Release, Slice: sliceID,
			Role: "implementer", Result: "designed",
			Summary: "Design " + sliceID + ".",
			Detail:  []byte("Exact design."),
		},
		{
			Release: f.manifest.value.Release, Slice: sliceID,
			Role: "lead", Result: "proceed",
			Summary: "Proceed with " + sliceID + ".",
			Detail:  []byte("Exact review."),
		},
	} {
		if _, err := f.engine.actions.AppendReceipt(receipt); err != nil {
			t.Fatal(err)
		}
	}
}

// sealThroughHostRunner implements a slice through the real seal path: the
// fixture driver writes the slice's scoped file, the engine runs the declared
// host checks against the sealed candidate, journals them and binds the
// engine-built manifest as the candidate receipt's checks digest.
func (f *assemblyHostEvidenceFixture) sealThroughHostRunner(t *testing.T, sliceID string) {
	t.Helper()
	f.designAndProceed(t, sliceID)
	f.service.dispatcher = fixtureDriver(func(_ context.Context, invocation driver.Invocation) (driver.Observation, error) {
		path := filepath.Join(invocation.HostWorkspace, assemblyFixtureSliceFile(sliceID))
		if err := os.WriteFile(path, []byte(sliceID+" delivered\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return productionImplementationObservation(t, invocation), nil
	})
	state := f.readState(t)
	slice, ok := state.Slice(sliceID)
	if !ok {
		t.Fatalf("%s missing from Protocol state", sliceID)
	}
	if err := f.service.implementSlice(f.ctx, f.engine, f.owner, state, slice); err != nil {
		t.Fatalf("implement %s: %v", sliceID, err)
	}
	current, _ := f.readState(t).Slice(sliceID)
	if current.Candidate == nil || current.Candidate.Receipt.Checks == nil ||
		current.NextRole != "verifier" {
		t.Fatalf("%s did not seal a host-checked candidate: %#v", sliceID, current)
	}
}

// sealByHand appends a candidate receipt for a slice without running the
// host runner, so the slice has a candidate receipt but no journaled
// check.host evidence and no git.seal record.
func (f *assemblyHostEvidenceFixture) sealByHand(t *testing.T, sliceID, trackID string) {
	t.Helper()
	f.designAndProceed(t, sliceID)
	repository := f.manifest.value.Repository
	ref := "refs/heads/track/" + f.manifest.value.Release + "/" + trackID
	parent := strings.TrimSpace(runRuntimeGit(t, repository, "rev-parse", "--verify", ref))
	// The candidate adds the slice's scoped file on top of the track head,
	// so it changes the product exactly as a sealed candidate would.
	scoped := filepath.Join(t.TempDir(), assemblyFixtureSliceFile(sliceID))
	if err := os.WriteFile(scoped, []byte(sliceID+" delivered by hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	blob := strings.TrimSpace(runRuntimeGit(t, repository, "hash-object", "-w", scoped))
	listing := strings.TrimSpace(runRuntimeGit(t, repository, "ls-tree", parent+"^{tree}"))
	listing += "\n100644 blob " + blob + "\t" + assemblyFixtureSliceFile(sliceID) + "\n"
	mktree := exec.Command("git", "-C", repository, "mktree")
	mktree.Stdin = strings.NewReader(listing)
	treeBytes, err := mktree.CombinedOutput()
	if err != nil {
		t.Fatalf("git mktree: %v\n%s", err, treeBytes)
	}
	tree := strings.TrimSpace(string(treeBytes))
	candidate := strings.TrimSpace(runRuntimeGitIdentity(
		t, repository, "commit-tree", tree, "-p", parent, "-m", "candidate "+sliceID))
	runRuntimeGit(t, repository, "update-ref", ref, candidate, parent)
	if _, err := f.engine.actions.AppendReceipt(protocol.AppendReceiptInput{
		Release: f.manifest.value.Release, Slice: sliceID,
		Role: "implementer", Result: "candidate",
		Summary: "Candidate " + sliceID + ".", Detail: []byte("candidate detail"),
		Candidate: candidate, CheckResults: []byte("{}"),
	}); err != nil {
		t.Fatal(err)
	}
}

// passByVerifier appends the independent PASS for the slice's current
// candidate with opaque verifier checks bytes.
func (f *assemblyHostEvidenceFixture) passByVerifier(t *testing.T, sliceID string) {
	t.Helper()
	slice, ok := f.readState(t).Slice(sliceID)
	if !ok || slice.Candidate == nil || slice.Candidate.Receipt.Candidate == nil {
		t.Fatalf("%s has no candidate to pass", sliceID)
	}
	if _, err := f.engine.actions.AppendReceipt(protocol.AppendReceiptInput{
		Release: f.manifest.value.Release, Slice: sliceID,
		Role: "verifier", Result: "pass",
		Summary: "Pass " + sliceID + ".", Detail: []byte("pass detail"),
		Candidate:    *slice.Candidate.Receipt.Candidate,
		CheckResults: []byte("verifier checks\n"),
	}); err != nil {
		t.Fatal(err)
	}
}

// prepareAssembly composes the assembly candidate and returns the state
// that authorises its verification.
func (f *assemblyHostEvidenceFixture) prepareAssembly(t *testing.T) protocol.State {
	t.Helper()
	if err := f.service.prepareAssembly(f.ctx, f.engine, f.owner, f.readState(t)); err != nil {
		t.Fatalf("prepareAssembly: %v", err)
	}
	state := f.readState(t)
	if state.Assembly.NextRole != "verifier" || state.Assembly.Candidate == nil ||
		state.Assembly.Candidate.Receipt.Candidate == nil {
		t.Fatalf("assembly not awaiting verification: %#v", state.Assembly)
	}
	return state
}

// captureAssemblyContext derives the assembly verification work context
// exactly as verifyAssembly dispatches it.
func (f *assemblyHostEvidenceFixture) captureAssemblyContext(
	t *testing.T,
	state protocol.State,
) productionWorkContext {
	t.Helper()
	candidate := *state.Assembly.Candidate.Receipt.Candidate
	before := workIdentity(state.Plan.OID, state.Refs.Release.Head, state.Refs.Target.Head, candidate)
	coordinates := dispatchCoordinates{
		Responsibility:  driver.AssemblyVerification,
		ProtocolAttempt: state.Plan.Metadata.Revision,
		Epoch:           1, Try: 1,
	}
	workContext, _, err := captureProductionWorkContext(
		f.ctx, f.engine, coordinates, before, driver.ReadOnly)
	if err != nil {
		t.Fatalf("capture assembly work context: %v", err)
	}
	if err := validateProductionWorkContext(f.manifest, workContext); err != nil {
		t.Fatalf("assembly work context refused: %v", err)
	}
	return workContext
}

// decodeHostEvidenceInput returns the projected host-evidence.json body.
func decodeHostEvidenceInput(
	t *testing.T,
	workContext productionWorkContext,
) productionAssemblyHostEvidence {
	t.Helper()
	contents, err := productionInputContents(workContext, mustJSON(workContext))
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range contents {
		if content.Input.Path != productionHostEvidencePath {
			continue
		}
		if content.Input.Name != "host-evidence" ||
			driver.Digest(content.Bytes) != content.Input.Digest {
			t.Fatalf("host evidence input binding = %#v", content.Input)
		}
		var body productionAssemblyHostEvidence
		if err := json.Unmarshal(content.Bytes, &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	t.Fatal("host evidence input was not projected")
	return productionAssemblyHostEvidence{}
}

// #343 (a): a serial single-track release whose slices all declare host
// checks reaches assembly with a roll-up that proves every evidence pin
// against its candidate receipt, and the assembly candidate's tree is the
// tree of the last verified slice candidate.
func TestAssemblyWorkContextProjectsProvenHostEvidenceRollup(t *testing.T) {
	hostCheck := "printf 'host ok\\n'"
	f := newAssemblyHostEvidenceFixture(t, [][]string{{"S1", "S2"}}, []string{hostCheck})
	for _, sliceID := range []string{"S1", "S2"} {
		f.sealThroughHostRunner(t, sliceID)
		f.passByVerifier(t, sliceID)
	}
	state := f.prepareAssembly(t)
	workContext := f.captureAssemblyContext(t, state)

	evidence := workContext.HostEvidence
	if evidence == nil || evidence.Assembly == nil ||
		evidence.SchemaVersion != productionAssemblyHostEvidenceVersion ||
		evidence.Slice != "" || evidence.ContractDigest != "" ||
		len(evidence.Results) != 0 ||
		evidence.Candidate != *state.Assembly.Candidate.Receipt.Candidate ||
		evidence.ManifestDigest != *state.Assembly.Candidate.Receipt.Checks {
		t.Fatalf("assembly host evidence = %#v", evidence)
	}
	body := decodeHostEvidenceInput(t, workContext)
	if body.SchemaVersion != productionAssemblyHostEvidenceVersion ||
		body.Candidate != evidence.Candidate ||
		!body.TreeMatchesLastVerifiedCandidate || body.MatchingSlice != "S2" ||
		len(body.Slices) != 1 {
		t.Fatalf("roll-up body = %#v", body)
	}
	// Exactly one pin: the track's final passed slice. Its proof binds the
	// candidate receipt the engine sealed and the contract the plan admits.
	entry := body.Slices[0]
	slice, _ := state.Slice("S2")
	contractDigest, _ := f.plan.Contract("S2")
	if entry.Slice != "S2" || entry.Evidence != assemblyHostEvidenceProven ||
		entry.Reason != "" ||
		entry.CandidateReceipt != slice.Candidate.OID ||
		entry.Candidate != *slice.Candidate.Receipt.Candidate ||
		entry.ContractDigest != contractDigest ||
		entry.ManifestDigest != *slice.Candidate.Receipt.Checks ||
		entry.CandidateTree != body.AssemblyTree ||
		len(entry.Checks) != 1 ||
		entry.Checks[0].Check != hostCheck ||
		entry.Checks[0].Outcome != protocol.CheckOutcomePass ||
		entry.Checks[0].ExitCode != 0 ||
		!runtimeDigestPattern.MatchString(entry.Checks[0].OutputDigest) ||
		entry.Checks[0].HostEffect == "" {
		t.Fatalf("slice roll-up entry = %#v", entry)
	}
	// The cited effect is the journaled check.host effect for exactly this
	// slice candidate, and the manifest it rebuilds is the receipt's.
	effect, err := f.store.Effect(f.ctx, f.manifest.value.RunID, entry.Checks[0].HostEffect)
	if err != nil || effect.Kind != "check.host" || effect.State != journal.Succeeded {
		t.Fatalf("cited host effect = %#v, %v", effect, err)
	}
	results, err := readJournaledHostResults(
		f.ctx, f.engine, "S2", entry.Candidate, contractDigest, []string{hostCheck})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.store.Snapshot(f.ctx, f.owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	record, found := journaledSealedRecords(snapshot)[sealedRecordKey("S2", entry.Candidate)]
	if !found {
		t.Fatal("sealed record for S2 absent from the journal")
	}
	if reason := proveSliceHostManifest(
		state.Release, "S2", entry.Candidate, contractDigest,
		entry.ManifestDigest, results, record.Receipt.CheckResults,
	); reason != "" {
		t.Fatalf("independent proof of the projected entry = %q", reason)
	}
	// The request carries the input, and a retry re-derives identical bytes.
	request, err := productionRequestForContext(f.manifest, workContext)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, input := range request.Inputs {
		found = found || input == evidence.Input
	}
	if !found {
		t.Fatal("host evidence input is absent from the assembly request")
	}
	again := f.captureAssemblyContext(t, state)
	if again.HostEvidence == nil || again.HostEvidence.Input != evidence.Input {
		t.Fatalf("re-derived host evidence = %#v", again.HostEvidence)
	}
}

// #343 (b): a pin whose candidate has no journaled host evidence is
// projected as missing with a reason, never as a pass, while the other pin
// is still proven and the dispatch still prepares. Two tracks compose a
// merge, so the assembly tree matches no verified slice candidate.
func TestAssemblyWorkContextFailsClosedPerSliceWhenHostEvidenceIsAbsent(t *testing.T) {
	hostCheck := "printf 'host ok\\n'"
	f := newAssemblyHostEvidenceFixture(t, [][]string{{"S1"}, {"S2"}}, []string{hostCheck})
	f.sealThroughHostRunner(t, "S1")
	f.passByVerifier(t, "S1")
	f.sealByHand(t, "S2", "T2")
	f.passByVerifier(t, "S2")
	state := f.prepareAssembly(t)
	workContext := f.captureAssemblyContext(t, state)

	if workContext.HostEvidence == nil || workContext.HostEvidence.Assembly == nil {
		t.Fatalf("assembly host evidence = %#v", workContext.HostEvidence)
	}
	body := decodeHostEvidenceInput(t, workContext)
	if body.TreeMatchesLastVerifiedCandidate || body.MatchingSlice != "" ||
		len(body.Slices) != 2 {
		t.Fatalf("roll-up body = %#v", body)
	}
	proven, missing := body.Slices[0], body.Slices[1]
	if proven.Slice != "S1" || proven.Evidence != assemblyHostEvidenceProven ||
		len(proven.Checks) != 1 || proven.Checks[0].Outcome != protocol.CheckOutcomePass {
		t.Fatalf("proven entry = %#v", proven)
	}
	slice, _ := state.Slice("S2")
	if missing.Slice != "S2" || missing.Evidence != assemblyHostEvidenceMissing ||
		missing.Reason != "HOST_CHECK_EVIDENCE_MISSING" ||
		missing.Candidate != *slice.Candidate.Receipt.Candidate ||
		missing.ManifestDigest != "" || len(missing.Checks) != 0 {
		t.Fatalf("missing entry = %#v", missing)
	}
}

// #343: the proof refuses a sealed manifest whose bytes are not what the
// receipt's checks digest covers, or that the journaled results do not
// rebuild, and never reports such a slice as proven.
func TestProveSliceHostManifestFailsClosedOnMismatch(t *testing.T) {
	t.Parallel()

	candidate := strings.Repeat("7", 40)
	contract := "sha256:" + strings.Repeat("a", 64)
	results := []hostCheckResult{{
		Slice: "S1", Candidate: candidate, ContractDigest: contract,
		Check: "printf ok", Outcome: protocol.CheckOutcomePass, ExitCode: 0,
		Output: "ok\n", OutputDigest: protocol.DigestBytes([]byte("ok\n")),
		EffectID: "attempt/host/1/1",
	}}
	roleDigest := protocol.DigestBytes([]byte("role checks\n"))
	manifest, err := buildHostCheckResultsManifest(
		"release", "S1", 1, candidate, contract, results, roleDigest)
	if err != nil {
		t.Fatal(err)
	}
	checksDigest := protocol.DigestBytes(manifest)
	if reason := proveSliceHostManifest(
		"release", "S1", candidate, contract, checksDigest, results, manifest,
	); reason != "" {
		t.Fatalf("exact manifest = %q", reason)
	}

	// The receipt digest covers different bytes.
	if reason := proveSliceHostManifest(
		"release", "S1", candidate, contract,
		"sha256:"+strings.Repeat("b", 64), results, manifest,
	); reason != "CHECKS_DIGEST_MISMATCH" {
		t.Fatalf("foreign checks digest = %q", reason)
	}
	// The journal records a different outcome than the sealed manifest.
	failed := append([]hostCheckResult(nil), results...)
	failed[0].Outcome = protocol.CheckOutcomeFail
	failed[0].ExitCode = 1
	if reason := proveSliceHostManifest(
		"release", "S1", candidate, contract, checksDigest, failed, manifest,
	); reason != "CHECKS_DIGEST_MISMATCH" {
		t.Fatalf("outcome drift = %q", reason)
	}
	// The sealed manifest binds another candidate.
	other, err := buildHostCheckResultsManifest(
		"release", "S1", 1, strings.Repeat("8", 40), contract, results, roleDigest)
	if err != nil {
		t.Fatal(err)
	}
	if reason := proveSliceHostManifest(
		"release", "S1", candidate, contract,
		protocol.DigestBytes(other), results, other,
	); reason != "CORRUPT_JOURNAL" {
		t.Fatalf("candidate drift = %q", reason)
	}
	// No sealed manifest at all.
	if reason := proveSliceHostManifest(
		"release", "S1", candidate, contract, checksDigest, results, nil,
	); reason != "CHECKS_DIGEST_MISMATCH" {
		t.Fatalf("absent manifest = %q", reason)
	}
}

// assemblyHostEvidenceWorkContext builds an AssemblyVerification production
// work context that carries the given host-evidence roll-up.
func assemblyHostEvidenceWorkContext(
	t *testing.T,
	manifest admittedManifest,
	coordinates dispatchCoordinates,
	evidence *productionHostEvidence,
) productionWorkContext {
	t.Helper()
	value := hostEvidenceWorkContext(t, manifest, coordinates, nil)
	value.Role = driver.RoleVerifier
	value.Track = ""
	value.Authority.TrackRef = ""
	value.Authority.TrackHead = ""
	value.HostEvidence = evidence
	return value
}

func assemblyRollupEvidence(candidate string) *productionHostEvidence {
	rollup := &productionAssemblyHostEvidence{
		SchemaVersion:                    productionAssemblyHostEvidenceVersion,
		Candidate:                        candidate,
		AssemblyTree:                     strings.Repeat("9", 40),
		MatchingSlice:                    "S2",
		TreeMatchesLastVerifiedCandidate: true,
		Slices: []productionAssemblySliceEvidence{
			{
				Slice: "S1", CandidateReceipt: strings.Repeat("a", 40),
				Candidate: strings.Repeat("b", 40), CandidateTree: strings.Repeat("c", 40),
				ContractDigest: "sha256:" + strings.Repeat("d", 64),
				Evidence:       assemblyHostEvidenceMissing,
				Reason:         "HOST_CHECK_EVIDENCE_MISSING",
			},
			{
				Slice: "S2", CandidateReceipt: strings.Repeat("e", 40),
				Candidate: strings.Repeat("f", 40), CandidateTree: strings.Repeat("9", 40),
				ContractDigest: "sha256:" + strings.Repeat("1", 64),
				ManifestDigest: "sha256:" + strings.Repeat("2", 64),
				Evidence:       assemblyHostEvidenceProven,
				Checks: []productionAssemblyHostCheck{{
					Check: "go test ./...", Outcome: protocol.CheckOutcomePass,
					ExitCode: 0, OutputDigest: "sha256:" + strings.Repeat("3", 64),
					HostEffect: "attempt/host/1/1",
				}},
			},
		},
	}
	body := mustJSON(rollup)
	return &productionHostEvidence{
		SchemaVersion:  productionAssemblyHostEvidenceVersion,
		Candidate:      candidate,
		ManifestDigest: "sha256:" + strings.Repeat("4", 64),
		Assembly:       rollup,
		Input: driver.Input{
			Name:   "host-evidence",
			Path:   productionHostEvidencePath,
			Digest: driver.Digest(body),
		},
		body: body,
	}
}

// #343 (c): the work-context guard admits the roll-up on
// AssemblyVerification with an empty slice, and still refuses host evidence
// on WorkVerification with an empty slice and on every other responsibility.
func TestAssemblyHostEvidenceGuardAdmitsOnlyAssemblyVerification(t *testing.T) {
	t.Parallel()

	repository := productionRepository(t)
	config := productionConfig(t)
	manifest := productionManifest(t, repository, config)
	coordinates := dispatchCoordinates{
		Responsibility:  driver.AssemblyVerification,
		ProtocolAttempt: 1, Epoch: 1, Try: 1,
	}
	candidate := strings.Repeat("7", 40)
	value := assemblyHostEvidenceWorkContext(
		t, manifest, coordinates, assemblyRollupEvidence(candidate))
	if err := validateProductionWorkContext(manifest, value); err != nil {
		t.Fatal(err)
	}
	contents, err := productionInputContents(value, mustJSON(value))
	if err != nil {
		t.Fatal(err)
	}
	projected := false
	for _, content := range contents {
		projected = projected || content.Input == value.HostEvidence.Input
	}
	if !projected {
		t.Fatal("assembly roll-up was not projected as an input")
	}

	// WorkVerification with an empty slice stays refused, whichever schema
	// the evidence claims.
	for name, evidence := range map[string]*productionHostEvidence{
		"roll-up": assemblyRollupEvidence(candidate),
		"slice manifest": {
			SchemaVersion:  productionHostEvidenceVersion,
			Candidate:      candidate,
			ContractDigest: "sha256:" + strings.Repeat("a", 64),
			ManifestDigest: "sha256:" + strings.Repeat("b", 64),
			Results: []productionHostCheckResult{{
				Check: "go test ./...", Outcome: "pass",
				OutputDigest: "sha256:" + strings.Repeat("c", 64),
				HostEffect:   "attempt/host/1/1",
			}},
			Input: driver.Input{
				Name: "host-evidence", Path: productionHostEvidencePath,
				Digest: "sha256:" + strings.Repeat("d", 64),
			},
		},
	} {
		t.Run("work verification empty slice "+name, func(t *testing.T) {
			slice := coordinates
			slice.Responsibility = driver.WorkVerification
			broken := assemblyHostEvidenceWorkContext(t, manifest, slice, evidence)
			broken.Track = "T1"
			broken.Authority.TrackRef = "refs/heads/track/" + manifest.value.Release + "/T1"
			broken.Authority.TrackHead = strings.Repeat("4", 40)
			if err := validateProductionWorkContext(
				manifest, broken,
			); !IsCode(err, "CORRUPT_JOURNAL") {
				t.Fatalf("work verification with empty slice = %v", err)
			}
		})
	}

	// Every non-verification responsibility refuses host evidence.
	for _, responsibility := range []driver.Responsibility{
		driver.ImplementerDesign, driver.LeadReview, driver.ImplementerImplementation,
	} {
		t.Run(string(responsibility), func(t *testing.T) {
			other := dispatchCoordinates{
				Slice: "S1", Responsibility: responsibility,
				ProtocolAttempt: 1, Epoch: 1, Try: 1,
			}
			broken := hostEvidenceWorkContext(t, manifest, other, assemblyRollupEvidence(candidate))
			broken.Role, _ = roleForResponsibility(responsibility)
			broken.Candidate = nil
			if responsibility == driver.ImplementerImplementation {
				broken.WorkspaceAccess = driver.ReadWrite
			}
			if err := validateProductionWorkContext(
				manifest, broken,
			); !IsCode(err, "CORRUPT_JOURNAL") {
				t.Fatalf("%s host evidence = %v", responsibility, err)
			}
		})
	}

	// A malformed roll-up is refused on the assembly responsibility too.
	for name, mutate := range map[string]func(*productionAssemblyHostEvidence){
		"empty slices": func(value *productionAssemblyHostEvidence) {
			value.Slices = nil
		},
		"missing without reason": func(value *productionAssemblyHostEvidence) {
			value.Slices[0].Reason = ""
		},
		"missing claiming a manifest": func(value *productionAssemblyHostEvidence) {
			value.Slices[0].ManifestDigest = "sha256:" + strings.Repeat("5", 64)
		},
		"proven without checks": func(value *productionAssemblyHostEvidence) {
			value.Slices[1].Checks = nil
		},
		"tree flag disagrees": func(value *productionAssemblyHostEvidence) {
			value.TreeMatchesLastVerifiedCandidate = false
		},
		"matching slice tree differs": func(value *productionAssemblyHostEvidence) {
			value.Slices[1].CandidateTree = strings.Repeat("8", 40)
		},
		"duplicate slice": func(value *productionAssemblyHostEvidence) {
			value.Slices[0].Slice = "S2"
		},
		"foreign candidate": func(value *productionAssemblyHostEvidence) {
			value.Candidate = strings.Repeat("6", 40)
		},
	} {
		t.Run(name, func(t *testing.T) {
			evidence := assemblyRollupEvidence(candidate)
			mutate(evidence.Assembly)
			broken := assemblyHostEvidenceWorkContext(t, manifest, coordinates, evidence)
			if err := validateProductionWorkContext(
				manifest, broken,
			); !IsCode(err, "CORRUPT_JOURNAL") {
				t.Fatalf("mutated roll-up = %v", err)
			}
		})
	}
}
