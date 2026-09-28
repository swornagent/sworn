package runtime

import (
	"encoding/json"

	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
)

// HostEnvironmentFactSchemaVersion versions the host-check environment
// fact shape (S1-host-check-environment-failures A5), served on
// EffectStatus and PinnedWork beside - never replacing - the pre-existing
// HostCheckFailureFact/sworn.host-check-failure-fact/v1 shape, which stays
// exactly as it is: a real check failure and a host environment failure
// are distinct facts about distinct effect kinds.
const HostEnvironmentFactSchemaVersion = "sworn.host-check-environment-fact/v1"

// HostEnvironmentFact is one bounded, versioned projection of a currently
// active host-check environment park, derived purely at Status time from
// an already-journaled check.host.environment record and the current
// state of the check.host effect it names - never by re-running the
// classifier. It is evidence, never authority: nothing in the engine reads
// it to decide a retry, a park, or a repair. Slice is empty for an
// assembly-scoped check.
type HostEnvironmentFact struct {
	SchemaVersion  string `json:"schema_version"`
	Check          string `json:"check"`
	MissingCommand string `json:"missing_command"`
	HostEffect     string `json:"host_effect"`
	Candidate      string `json:"candidate"`
	ContractDigest string `json:"contract_digest"`
	Slice          string `json:"slice"`
}

// hostEnvironmentCrossing names one check.host work currently parked on a
// host environment failure: the journaled classification, unpacked, plus
// the check.host effect id it concerns.
type hostEnvironmentCrossing struct {
	Slice          string
	Candidate      string
	ContractDigest string
	Check          string
	MissingCommand string
	HostEffect     string
}

// hostEnvironmentParkCrossings is a pure function of snapshot: it scans
// for Succeeded check.host.environment records and includes one crossing
// per record whose named check.host effect is CURRENTLY still
// journal.Claimed in this exact snapshot. The moment that effect completes
// (the environment was fixed and the host runner actually re-ran it), the
// crossing silently disappears from this function's output on the very
// next call - true self-clearing, journal-only, with no shell call and no
// engine, exactly like economyParkCrossings/identicalFailureParkCrossings.
// Both pinCrossingLanes (the writer/pinning side, run by the engine) and
// Status (the reader side) call this same function, so they can never
// disagree.
func hostEnvironmentParkCrossings(snapshot journal.Snapshot) []hostEnvironmentCrossing {
	effectsByID := make(map[string]journal.Effect, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		effectsByID[effect.ID] = effect
	}
	var crossings []hostEnvironmentCrossing
	for _, effect := range snapshot.Effects {
		if effect.Kind != "check.host.environment" || effect.State != journal.Succeeded {
			continue
		}
		crossing, ok := parseHostEnvironmentCrossing(effect, effectsByID)
		if !ok {
			continue
		}
		crossings = append(crossings, crossing)
	}
	return crossings
}

// parseHostEnvironmentCrossing validates one check.host.environment effect
// against its own binding and the current state of the check.host effect
// it names, reporting false when anything fails to bind - unparsable,
// mismatched, or the named check.host effect is no longer Claimed
// (self-cleared) - never corrupt.
func parseHostEnvironmentCrossing(
	effect journal.Effect,
	effectsByID map[string]journal.Effect,
) (hostEnvironmentCrossing, bool) {
	var record hostEnvironmentClassification
	if json.Unmarshal(effect.Result, &record) != nil ||
		!bytesEqualCanonicalJSON(effect.Result, record) ||
		record.SchemaVersion != hostEnvironmentClassificationSchemaVersion {
		return hostEnvironmentCrossing{}, false
	}
	checkHostWork := hostCheckWork(record.Slice, record.Candidate, record.ContractDigest, record.Check)
	// #296: the classification's own bound work is either the check.host
	// work's first execution or its one re-execution (host_checks.go's
	// hostCheckBoundWork, the same helper validateHostCheckEvidenceProof
	// already uses for this exact first-vs-rerun ambiguity) - never
	// assumed to be the first execution alone. A rerun's own
	// journalHostEnvironmentClassification call journals under the rerun
	// identity, so recomputing against the first execution only would
	// reject every rerun classification as unparsable.
	boundWork, ok := hostCheckBoundWork(record.HostEffect, checkHostWork)
	if !ok {
		return hostEnvironmentCrossing{}, false
	}
	classificationWork := hostEnvironmentClassificationWork(boundWork)
	if effect.BeforeDigest != classificationWork ||
		effect.ID != journal.AttemptEffectID(classificationWork, 1, 1) {
		return hostEnvironmentCrossing{}, false
	}
	hostEffect, found := effectsByID[record.HostEffect]
	if !found || hostEffect.Kind != "check.host" ||
		hostEffect.BeforeDigest != boundWork ||
		hostEffect.State != journal.Claimed {
		return hostEnvironmentCrossing{}, false
	}
	return hostEnvironmentCrossing{
		Slice: record.Slice, Candidate: record.Candidate,
		ContractDigest: record.ContractDigest, Check: record.Check,
		MissingCommand: record.MissingCommand, HostEffect: record.HostEffect,
	}, true
}

// hostEnvironmentCrossingOwner names the lane-candidate work identity a
// crossing pins: a slice-scoped crossing (Slice != "") is owned by the
// slice's current git.seal work, exactly as implementSlice derives it; an
// assembly-scoped crossing (Slice == "") is owned by the assembly's
// prepare work, exactly as prepareAssembly derives it. Both identities
// already appear in readyLaneCandidates' own work sets while their state
// applies, so resolveLanePins/pinCrossingLanes need no separate by-lane
// side channel the way a standing exhaustion park does.
func hostEnvironmentCrossingOwner(state protocol.State, crossing hostEnvironmentCrossing) string {
	if crossing.Slice != "" {
		return workIdentity(sliceFingerprint(state, crossing.Slice), "git.seal")
	}
	return assemblyPrepareWork(state)
}

// hostEnvironmentCrossingDetail renders one crossing's bounded park
// detail, honestly absent (never guessed) when it fails validParkDetail's
// bound.
func hostEnvironmentCrossingDetail(crossing hostEnvironmentCrossing) string {
	detail := crossing.Check + ": command not found: " + crossing.MissingCommand
	if !validParkDetail(detail) {
		return ""
	}
	return detail
}

// hostEnvironmentParkFacts carries everything a park surface names for a
// work-scoped host-environment crossing: the owning work, the stable
// failure code, the rendered detail, and the ready-built A5 fact for that
// same crossing (so PinnedWork.HostEnvironmentFailure is the identical
// value EffectStatus.HostEnvironmentFailure would carry for the same
// check.host effect, never independently re-derived).
type hostEnvironmentParkFacts struct {
	work   string
	code   string
	detail string
	fact   HostEnvironmentFact
}

// hostEnvironmentFactFor builds the additive A5 fact for one crossing.
func hostEnvironmentFactFor(crossing hostEnvironmentCrossing) HostEnvironmentFact {
	return HostEnvironmentFact{
		SchemaVersion: HostEnvironmentFactSchemaVersion,
		Check:         crossing.Check, MissingCommand: crossing.MissingCommand,
		HostEffect: crossing.HostEffect, Candidate: crossing.Candidate,
		ContractDigest: crossing.ContractDigest, Slice: crossing.Slice,
	}
}

// hostEnvironmentParkFactsByOwner builds resolveLanePins' input map from
// the currently-active crossings, keyed by owner work identity, first-
// encountered order winning any owner collision (there should be none: a
// distinct check.host work always binds a distinct owner unless two
// checks of the identical slice/candidate/contract are both parked, which
// this map simply reports as one park, matching every other cause's
// single-crossing-per-owner precedent).
func hostEnvironmentParkFactsByOwner(
	state protocol.State,
	snapshot journal.Snapshot,
) map[string]hostEnvironmentParkFacts {
	result := make(map[string]hostEnvironmentParkFacts)
	for _, crossing := range hostEnvironmentParkCrossings(snapshot) {
		owner := hostEnvironmentCrossingOwner(state, crossing)
		if owner == "" {
			continue
		}
		if _, exists := result[owner]; exists {
			continue
		}
		result[owner] = hostEnvironmentParkFacts{
			work: owner, code: "HOST_CHECK_ENVIRONMENT",
			detail: hostEnvironmentCrossingDetail(crossing),
			fact:   hostEnvironmentFactFor(crossing),
		}
	}
	return result
}

// hostEnvironmentExcludedEffects names every effect ID that Status must not
// fold into "active" or "uncertain" work because it belongs to a current
// host-environment crossing (S6-host-environment-park-projection A1): the
// crossing's own check.host effect always (needs no state), and, only when
// state is available, the one enclosing git.seal or protocol.prepare_assembly
// effect whose BeforeDigest equals hostEnvironmentCrossingOwner(state,
// crossing) - the only other effect Claimed while such a crossing stands.
// implementSlice keeps its git.seal effect Claimed across
// claimPreparedImplementation's host-check block (which runs after the
// nested driver.dispatch effect has already Succeeded and before
// git.seal.prepared is ever journaled); prepareAssembly keeps
// protocol.prepare_assembly Claimed across runAssemblyHostChecks the same
// way. A verifier-stage host check (advanceSlice's NextRole=="verifier"
// branch) can never itself be the Claimed effect a crossing names: it
// resolves the identical (slice, candidate, contract digest, check) work
// the implement-stage seal already ran, so admitHostCheckEffect always finds
// it already journal.Succeeded and only ever replays the recorded result.
// The owner exclusion fails closed (omitted) when stateErr != nil, exactly
// like every other state-dependent park fact in Status.
func hostEnvironmentExcludedEffects(
	snapshot journal.Snapshot,
	state protocol.State,
	stateErr error,
) map[string]bool {
	excluded := make(map[string]bool)
	crossings := hostEnvironmentParkCrossings(snapshot)
	for _, crossing := range crossings {
		excluded[crossing.HostEffect] = true
	}
	if stateErr != nil {
		return excluded
	}
	for _, crossing := range crossings {
		owner := hostEnvironmentCrossingOwner(state, crossing)
		if owner == "" {
			continue
		}
		for _, effect := range snapshot.Effects {
			if effect.BeforeDigest == owner && effect.State == journal.Claimed {
				excluded[effect.ID] = true
			}
		}
	}
	return excluded
}

// attachHostEnvironmentFacts derives the A5 fact for every currently-
// active crossing in snapshot and attaches it to result's EffectStatus
// entries by the check.host effect id it concerns, clearing any prior
// attachment for check.host effects. It never fails; unparsable or
// missing evidence, or a check.host effect that is no longer Claimed,
// reports absent.
func attachHostEnvironmentFacts(result *RunStatus, snapshot journal.Snapshot) {
	if result == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	byHostEffect := make(map[string]HostEnvironmentFact)
	for _, crossing := range hostEnvironmentParkCrossings(snapshot) {
		byHostEffect[crossing.HostEffect] = hostEnvironmentFactFor(crossing)
	}
	for index := range result.Effects {
		if result.Effects[index].Kind != "check.host" {
			continue
		}
		if fact, ok := byHostEffect[result.Effects[index].ID]; ok {
			built := fact
			result.Effects[index].HostEnvironmentFailure = &built
		} else {
			result.Effects[index].HostEnvironmentFailure = nil
		}
	}
}
