package runtime

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
)

// assemblyHostCheckEffects returns, per check, the succeeded check.host
// effect that speaks for an assembly candidate (an empty slice): the one
// bounded re-execution when it succeeded, otherwise the first execution,
// exactly as latestJournaledHostCheck prefers. The snapshot orders effects
// by content-addressed id, so a last-writer-wins map would pick either at
// random from run to run.
func (f *assemblyHostEvidenceFixture) assemblyHostCheckEffects(t *testing.T) map[string]journal.Effect {
	t.Helper()
	snapshot, err := f.store.Snapshot(f.ctx, f.owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	effects := make(map[string]journal.Effect)
	for _, effect := range snapshot.Effects {
		if effect.Kind != "check.host" || effect.State != journal.Succeeded {
			continue
		}
		var result hostCheckResult
		if err := json.Unmarshal(effect.Result, &result); err != nil {
			t.Fatal(err)
		}
		if result.Slice != "" {
			continue
		}
		work := assemblyHostCheckWork(result.Candidate, result.ContractDigest, result.Check)
		latest, latestID, err := latestJournaledHostCheck(f.ctx, f.engine, work)
		if err != nil || latestID != effect.ID {
			continue
		}
		effects[result.Check] = latest
	}
	return effects
}

func (f *assemblyHostEvidenceFixture) countHostCheckEffects(t *testing.T) int {
	t.Helper()
	snapshot, err := f.store.Snapshot(f.ctx, f.owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, effect := range snapshot.Effects {
		if effect.Kind == "check.host" {
			count++
		}
	}
	return count
}

// rebuildAssemblyManifest replays the assembly host checks for the prepared
// candidate through the runner, which by identity executes nothing new, and
// returns the manifest it rebuilds.
func (f *assemblyHostEvidenceFixture) rebuildAssemblyManifest(t *testing.T, state protocol.State) []byte {
	t.Helper()
	manifest, err := f.service.runAssemblyHostChecks(
		f.ctx, f.engine, f.owner, state, f.plan, *state.Assembly.Candidate.Receipt.Candidate)
	if err != nil {
		t.Fatalf("rebuild assembly manifest: %v", err)
	}
	return manifest
}

// #343 (a): two serial slices verified in this run through the host runner
// reach an assembly whose tree is the last slice candidate's tree, so the
// assembly checks reuse that candidate's recorded pass by identity instead
// of executing; the receipt's checks digest is the assembly manifest's digest
// and the roll-up's assembly section is proven citing the reused effect.
func TestAssemblyPreparationReusesSliceHostEvidenceForIdenticalTree(t *testing.T) {
	hostCheck := "printf 'host ok\\n'"
	f := newAssemblyHostEvidenceFixture(t, [][]string{{"S1", "S2"}}, []string{hostCheck})
	for _, sliceID := range []string{"S1", "S2"} {
		f.sealThroughHostRunner(t, sliceID)
		f.passByVerifier(t, sliceID)
	}
	before := f.countHostCheckEffects(t)
	state := f.prepareAssembly(t)

	// Reuse: no check.host effect is keyed by the assembly candidate, and the
	// journal grew by nothing.
	if effects := f.assemblyHostCheckEffects(t); len(effects) != 0 {
		t.Fatalf("assembly-keyed check.host effects = %#v, want reuse", effects)
	}
	if after := f.countHostCheckEffects(t); after != before {
		t.Fatalf("check.host effects %d -> %d, want no execution", before, after)
	}
	// The receipt binds the assembly manifest rebuilt from the journal.
	manifest := f.rebuildAssemblyManifest(t, state)
	if digest := protocol.DigestBytes(manifest); digest != *state.Assembly.Candidate.Receipt.Checks {
		t.Fatalf("receipt checks digest = %s, manifest digest = %s",
			*state.Assembly.Candidate.Receipt.Checks, digest)
	}
	parsed, err := protocol.ParseCheckResults(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Slice != "" || parsed.Candidate != *state.Assembly.Candidate.Receipt.Candidate ||
		parsed.Attempt != state.Plan.Metadata.Revision || len(parsed.Entries) != 1 ||
		parsed.Entries[0].Provenance != protocol.CheckProvenanceHost {
		t.Fatalf("assembly manifest = %#v", parsed)
	}

	workContext := f.captureAssemblyContext(t, state)
	body := decodeHostEvidenceInput(t, workContext)
	section := body.Assembly
	if section == nil || section.Evidence != assemblyHostEvidenceProven ||
		section.Reason != "" || section.Candidate != body.Candidate ||
		section.Tree != body.AssemblyTree || !section.ReceiptBindsManifest ||
		section.ManifestDigest != *state.Assembly.Candidate.Receipt.Checks ||
		section.ContractDigest != parsed.ContractDigest ||
		len(section.Checks) != 1 || section.Checks[0].Check != hostCheck ||
		section.Checks[0].Outcome != protocol.CheckOutcomePass ||
		section.Checks[0].ReusedFrom != "S2" {
		t.Fatalf("assembly section = %#v", section)
	}
	// The cited effect is S2's own journaled check.host effect.
	slice, _ := state.Slice("S2")
	contractDigest, _ := f.plan.Contract("S2")
	_, expectedID, err := latestJournaledHostCheck(f.ctx, f.engine,
		hostCheckWork("S2", *slice.Candidate.Receipt.Candidate, contractDigest, hostCheck))
	if err != nil || section.Checks[0].HostEffect != expectedID {
		t.Fatalf("cited effect = %s, want S2's %s (%v)", section.Checks[0].HostEffect, expectedID, err)
	}
	// The per-slice section is unchanged supporting context.
	if len(body.Slices) != 1 || body.Slices[0].Evidence != assemblyHostEvidenceProven {
		t.Fatalf("per-slice section = %#v", body.Slices)
	}
}

// #343 (b): a relaunched run adopts slice passes as records with no journaled
// check.host evidence; the assembly checks execute against the assembled
// tree, are journaled keyed by the assembly candidate, and the roll-up's
// assembly section is proven while the per-slice section reports missing.
func TestAssemblyPreparationExecutesHostChecksForAdoptedPasses(t *testing.T) {
	hostCheck := "printf 'host ok\\n'"
	f := newAssemblyHostEvidenceFixture(t, [][]string{{"S1", "S2"}}, []string{hostCheck})
	for _, sliceID := range []string{"S1", "S2"} {
		f.sealByHand(t, sliceID, "T1")
		f.passByVerifier(t, sliceID)
	}
	if count := f.countHostCheckEffects(t); count != 0 {
		t.Fatalf("adopted passes journaled %d check.host effects", count)
	}
	state := f.prepareAssembly(t)
	candidate := *state.Assembly.Candidate.Receipt.Candidate

	effects := f.assemblyHostCheckEffects(t)
	effect, executed := effects[hostCheck]
	if !executed || len(effects) != 1 {
		t.Fatalf("assembly-keyed check.host effects = %#v, want one execution", effects)
	}
	manifest := f.rebuildAssemblyManifest(t, state)
	parsed, err := protocol.ParseCheckResults(manifest)
	if err != nil {
		t.Fatal(err)
	}
	// The effect identity is the assembly identity: empty slice, the
	// assembly candidate, the union contract digest and the check.
	expectedID := hostCheckEffectID(assemblyHostCheckWork(candidate, parsed.ContractDigest, hostCheck))
	if effect.ID != expectedID || effect.BeforeDigest != assemblyHostCheckWork(candidate, parsed.ContractDigest, hostCheck) {
		t.Fatalf("assembly effect id = %s bound %s, want %s", effect.ID, effect.BeforeDigest, expectedID)
	}
	var result hostCheckResult
	if err := json.Unmarshal(effect.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Slice != "" || result.Candidate != candidate ||
		result.ContractDigest != parsed.ContractDigest ||
		result.Outcome != protocol.CheckOutcomePass || result.RerunOf != "" {
		t.Fatalf("assembly check result = %#v", result)
	}
	if digest := protocol.DigestBytes(manifest); digest != *state.Assembly.Candidate.Receipt.Checks {
		t.Fatalf("receipt checks digest = %s, manifest digest = %s",
			*state.Assembly.Candidate.Receipt.Checks, digest)
	}

	workContext := f.captureAssemblyContext(t, state)
	body := decodeHostEvidenceInput(t, workContext)
	section := body.Assembly
	if section == nil || section.Evidence != assemblyHostEvidenceProven ||
		!section.ReceiptBindsManifest || len(section.Checks) != 1 ||
		section.Checks[0].HostEffect != expectedID ||
		section.Checks[0].ReusedFrom != "" ||
		section.Checks[0].Outcome != protocol.CheckOutcomePass {
		t.Fatalf("assembly section = %#v", section)
	}
	if len(body.Slices) != 1 || body.Slices[0].Slice != "S2" ||
		body.Slices[0].Evidence != assemblyHostEvidenceMissing ||
		body.Slices[0].Reason != "HOST_CHECK_EVIDENCE_MISSING" {
		t.Fatalf("per-slice section = %#v", body.Slices)
	}
	// The guard admits the projected shape and the request carries it.
	if err := validateProductionWorkContext(f.manifest, workContext); err != nil {
		t.Fatal(err)
	}
}

// #343 (c): a declared check that fails on the assembled tree refuses the
// preparation under the existing HOST_CHECK_FAILED path: the prepare_assembly
// action fails operationally on every try, the exhaustion park names the
// check, no assembly candidate exists, and nothing can project proven.
func TestAssemblyPreparationRefusesOnFailingHostCheck(t *testing.T) {
	// Each slice candidate carries only its own file, so the check passes at
	// every slice seal and fails only on the composed tree carrying both.
	hostCheck := "test ! \\( -f s1.txt -a -f s2.txt \\)"
	f := newAssemblyHostEvidenceFixture(t, [][]string{{"S1"}, {"S2"}}, []string{hostCheck})
	for _, sliceID := range []string{"S1", "S2"} {
		f.sealThroughHostRunner(t, sliceID)
		f.passByVerifier(t, sliceID)
	}
	state := f.readState(t)
	if state.Assembly.NextRole != "merge" {
		t.Fatalf("assembly not ready to prepare: %#v", state.Assembly)
	}
	err := f.service.prepareAssembly(f.ctx, f.engine, f.owner, state)
	if !IsCode(err, "EFFECT_PARKED") {
		t.Fatalf("prepareAssembly = %v, want EFFECT_PARKED after the try budget", err)
	}
	after := f.readState(t)
	if after.Assembly.Candidate != nil || after.Assembly.NextRole != "merge" {
		t.Fatalf("a failing host check minted an assembly candidate: %#v", after.Assembly)
	}
	// Exactly one execution plus its one bounded re-execution (#296); the
	// third try replayed the re-execution's record.
	effects := f.assemblyHostCheckEffects(t)
	if len(effects) != 1 {
		t.Fatalf("assembly-keyed check.host effects = %#v", effects)
	}
	var result hostCheckResult
	if err := json.Unmarshal(effects[hostCheck].Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Outcome != protocol.CheckOutcomeFail || result.RerunOf == "" {
		t.Fatalf("assembly check result = %#v, want the re-executed fail", result)
	}
	// The reader the preparation and the roll-up share resolves the same
	// re-execution for the assembly identity, and its record names the first
	// execution it replaced.
	assemblyWork := assemblyHostCheckWork(result.Candidate, result.ContractDigest, hostCheck)
	latest, latestID, err := latestJournaledHostCheck(f.ctx, f.engine, assemblyWork)
	if err != nil || latestID != hostCheckRerunEffectID(assemblyWork) ||
		latest.ID != effects[hostCheck].ID || result.RerunOf != hostCheckEffectID(assemblyWork) {
		t.Fatalf("latest assembly effect = %s (%v), want rerun %s of %s",
			latestID, err, hostCheckRerunEffectID(assemblyWork), hostCheckEffectID(assemblyWork))
	}
	snapshot, err := f.store.Snapshot(f.ctx, f.owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	executions := 0
	for _, effect := range snapshot.Effects {
		if effect.Kind == "check.host" && strings.Contains(string(effect.Result), `"slice":""`) {
			executions++
		}
	}
	if executions != 2 {
		t.Fatalf("assembly check executions = %d, want first run plus one re-execution", executions)
	}
	// The prepare work's third try failed HOST_CHECK_FAILED with the check
	// named, and exhaustion reads it as the park's typed reason.
	before := workIdentity(state.Plan.OID, state.Refs.Release.Head, state.Refs.Target.Head,
		state.Assembly.Outcome, state.Assembly.InputPins)
	work := workIdentity(before, "prepare")
	third, err := f.store.Effect(f.ctx, f.owner.RunID, journal.AttemptEffectID(work, 1, 3))
	if err != nil || third.State != journal.OperationalFailed || third.ErrorCode != "HOST_CHECK_FAILED" {
		t.Fatalf("third prepare try = %#v, %v", third, err)
	}
	control, err := f.store.ControlProjection(f.ctx, f.owner.RunID)
	if err != nil {
		t.Fatal(err)
	}
	exhausted, refusals := exhaustedWorks(snapshot, control, nil)
	if _, ok := exhausted[work]; !ok {
		t.Fatalf("prepare work %s is not exhausted: %#v", work, exhausted)
	}
	facts, ok := refusals[work]
	if !ok || facts.code != "HOST_CHECK_FAILED" ||
		!strings.Contains(facts.detail, hostCheck) ||
		!strings.Contains(facts.detail, "recorded fail") {
		t.Fatalf("exhaustion refusal = %#v", facts)
	}
}

// #343 (d): running the assembly preparation again executes nothing: the
// check.host effects replay by identity and the manifest is byte-identical.
func TestAssemblyHostChecksReplayByIdentity(t *testing.T) {
	hostCheck := "printf 'host ok\\n'"
	f := newAssemblyHostEvidenceFixture(t, [][]string{{"S1"}, {"S2"}}, []string{hostCheck})
	f.sealThroughHostRunner(t, "S1")
	f.passByVerifier(t, "S1")
	f.sealByHand(t, "S2", "T2")
	f.passByVerifier(t, "S2")
	ready := f.readState(t)
	state := f.prepareAssembly(t)
	// Two tracks compose a merge: the tree matches no slice candidate, so
	// the check executed once against the assembly candidate.
	if effects := f.assemblyHostCheckEffects(t); len(effects) != 1 {
		t.Fatalf("assembly-keyed check.host effects = %#v", effects)
	}
	count := f.countHostCheckEffects(t)
	first := f.rebuildAssemblyManifest(t, state)

	// The same preparation replays the succeeded action and its checks.
	if err := f.service.prepareAssembly(f.ctx, f.engine, f.owner, ready); err != nil {
		t.Fatalf("replayed prepareAssembly: %v", err)
	}
	second := f.rebuildAssemblyManifest(t, f.readState(t))
	if string(first) != string(second) {
		t.Fatalf("replayed manifest differs:\n%s\n%s", first, second)
	}
	if again := f.countHostCheckEffects(t); again != count {
		t.Fatalf("check.host effects %d -> %d after replay", count, again)
	}
	if digest := protocol.DigestBytes(second); digest != *state.Assembly.Candidate.Receipt.Checks {
		t.Fatalf("receipt checks digest = %s, replayed manifest digest = %s",
			*state.Assembly.Candidate.Receipt.Checks, digest)
	}
	body := decodeHostEvidenceInput(t, f.captureAssemblyContext(t, state))
	if body.Assembly == nil || body.Assembly.Evidence != assemblyHostEvidenceProven ||
		!body.Assembly.ReceiptBindsManifest || body.TreeMatchesLastVerifiedCandidate {
		t.Fatalf("assembly section = %#v (tree match %v)", body.Assembly, body.TreeMatchesLastVerifiedCandidate)
	}
}

// #343: the guard admits a well-formed assembly section and refuses one that
// contradicts the roll-up or the receipt.
func TestAssemblyChecksEvidenceGuard(t *testing.T) {
	t.Parallel()

	repository := productionRepository(t)
	config := productionConfig(t)
	manifest := productionManifest(t, repository, config)
	coordinates := dispatchCoordinates{
		Responsibility:  driver.AssemblyVerification,
		ProtocolAttempt: 1, Epoch: 1, Try: 1,
	}
	candidate := strings.Repeat("7", 40)
	withSection := func(mutate func(*productionAssemblyChecksEvidence)) *productionHostEvidence {
		evidence := assemblyRollupEvidence(candidate)
		section := &productionAssemblyChecksEvidence{
			Candidate: candidate, Tree: evidence.Assembly.AssemblyTree,
			ContractDigest:       "sha256:" + strings.Repeat("6", 64),
			ManifestDigest:       evidence.ManifestDigest,
			ReceiptBindsManifest: true,
			Evidence:             assemblyHostEvidenceProven,
			Checks: []productionAssemblyHostCheck{{
				Check: "go test ./...", Outcome: protocol.CheckOutcomePass,
				ExitCode: 0, OutputDigest: "sha256:" + strings.Repeat("3", 64),
				HostEffect: "attempt/host/1/1", ReusedFrom: "S2",
			}},
		}
		if mutate != nil {
			mutate(section)
		}
		evidence.Assembly.Assembly = section
		body := mustJSON(evidence.Assembly)
		evidence.Input.Digest = driver.Digest(body)
		evidence.body = body
		return evidence
	}
	if err := validateProductionWorkContext(manifest,
		assemblyHostEvidenceWorkContext(t, manifest, coordinates, withSection(nil))); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*productionAssemblyChecksEvidence){
		"foreign candidate": func(s *productionAssemblyChecksEvidence) { s.Candidate = strings.Repeat("8", 40) },
		"foreign tree":      func(s *productionAssemblyChecksEvidence) { s.Tree = strings.Repeat("8", 40) },
		"binding claim against another digest": func(s *productionAssemblyChecksEvidence) {
			s.ManifestDigest = "sha256:" + strings.Repeat("5", 64)
		},
		"proven with a failed check": func(s *productionAssemblyChecksEvidence) {
			s.Checks[0].Outcome = protocol.CheckOutcomeFail
		},
		"proven without checks": func(s *productionAssemblyChecksEvidence) { s.Checks = nil },
		"missing without reason": func(s *productionAssemblyChecksEvidence) {
			s.Evidence, s.ManifestDigest, s.ReceiptBindsManifest = assemblyHostEvidenceMissing, "", false
		},
		"missing claiming a binding": func(s *productionAssemblyChecksEvidence) {
			s.Evidence, s.Reason, s.ManifestDigest = assemblyHostEvidenceMissing, "HOST_CHECK_EVIDENCE_MISSING", ""
		},
		"none declared with checks": func(s *productionAssemblyChecksEvidence) {
			s.Evidence, s.ContractDigest, s.ManifestDigest, s.ReceiptBindsManifest = assemblyHostEvidenceNone, "", "", false
		},
		"unknown evidence": func(s *productionAssemblyChecksEvidence) { s.Evidence = "verified" },
	} {
		t.Run(name, func(t *testing.T) {
			value := assemblyHostEvidenceWorkContext(t, manifest, coordinates, withSection(mutate))
			if err := validateProductionWorkContext(manifest, value); !IsCode(err, "CORRUPT_JOURNAL") {
				t.Fatalf("mutated assembly section = %v", err)
			}
		})
	}
	// A missing section with a reason and partial checks is admitted.
	partial := withSection(func(s *productionAssemblyChecksEvidence) {
		s.Evidence, s.Reason = assemblyHostEvidenceMissing, "HOST_CHECK_EVIDENCE_MISSING"
		s.ManifestDigest, s.ReceiptBindsManifest = "", false
	})
	if err := validateProductionWorkContext(manifest,
		assemblyHostEvidenceWorkContext(t, manifest, coordinates, partial)); err != nil {
		t.Fatal(err)
	}
}
