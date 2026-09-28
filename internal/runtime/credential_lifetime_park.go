package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
)

// CredentialLifetimeParkFactSchemaVersion versions the credential-lifetime
// admission-refusal fact shape (S3-credential-lifetime A2).
const CredentialLifetimeParkFactSchemaVersion = "sworn.credential-lifetime-park-fact/v1"

// credentialLifetimeParkFact is the canonical journal body for one
// credential-liveness admission refusal: a bounded, non-authoritative audit
// record. Nothing in the engine reads it to decide a retry, a park, or a
// repair - it feeds only Status()'s display path (resolveLanePins/
// parkStatusFor), never pinCrossingLanes. Detail is duration-only (A1/A2):
// no credential byte, no absolute timestamp.
type credentialLifetimeParkFact struct {
	SchemaVersion  string `json:"schema_version"`
	RunID          string `json:"run_id"`
	WorkID         string `json:"work_id"`
	Slice          string `json:"slice"`
	Responsibility string `json:"responsibility"`
	Before         string `json:"before"`
	Epoch          int64  `json:"epoch"`
	Try            int64  `json:"try"`
	Code           string `json:"code"`
	Detail         string `json:"detail,omitempty"`
}

// shortContentDigest is a bounded, content-addressed suffix (16 hex
// characters, 64 bits) for a replay key whose base identity can legitimately
// carry more than one distinct body over time: an unchanged body always
// produces an unchanged suffix (true idempotence), while a legitimately
// different body gets its own row instead of AppendEventOnce's
// content-equality check reporting REPLAY_CONFLICT.
func shortContentDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])[:16]
}

// recordCredentialLifetimeParkFact journals one credential-lifetime
// admission refusal (S3-credential-lifetime A2), idempotently and
// content-addressed so a later, distinct refusal for the identical
// (workID, epoch) - a different remaining/required duration, for instance -
// gets its own row rather than conflicting. The refusal error itself, and
// its zero-try-burn shape, are entirely unaffected by this call: it is a
// side, non-authoritative audit write.
func (s *Service) recordCredentialLifetimeParkFact(
	ctx context.Context,
	engine *engine,
	coordinates dispatchCoordinates,
	before string,
	code string,
	detail string,
) error {
	// A nested implementer_implementation dispatch journals its attempt
	// under coordinates.DispatchWork (cycle.DispatchWork), an identity
	// driverWorkIdentity alone can never reconstruct - every call site that
	// can reach this probe already sets it (scheduler.go, dispatch.go), so
	// preferring it here, with the pre-existing computation kept only as
	// the fallback for callers that predate this field, is the same
	// fallback contract captureEffectiveLimits already uses
	// (production_dispatch.go).
	workID := coordinates.DispatchWork
	if workID == "" {
		workID = driverWorkIdentity(
			engine.manifest.digest, coordinates.Slice, coordinates.Responsibility,
			coordinates.ProtocolAttempt, before,
		)
	}
	fact := credentialLifetimeParkFact{
		SchemaVersion:  CredentialLifetimeParkFactSchemaVersion,
		RunID:          engine.manifest.value.RunID,
		WorkID:         workID,
		Slice:          coordinates.Slice,
		Responsibility: string(coordinates.Responsibility),
		Before:         before,
		Epoch:          coordinates.Epoch,
		Try:            coordinates.Try,
		Code:           code,
		Detail:         detail,
	}
	body, err := json.Marshal(fact)
	if err != nil {
		return err
	}
	replayKey := "credential-lifetime-park/" + workID + "/" +
		strconv.FormatInt(coordinates.Epoch, 10) + "/" + shortContentDigest(body)
	return s.journal.AppendEventOnce(ctx, journal.Command{
		RunID:     engine.manifest.value.RunID,
		ReplayKey: replayKey,
		Kind:      "credential-lifetime-park",
		Payload:   body,
		CreatedAt: s.now().UTC(),
	}, "credential_lifetime_park", body, s.now().UTC())
}

// credentialLifetimeCrossing names one currently-active credential-lifetime
// admission refusal, unpacked from its journaled fact.
type credentialLifetimeCrossing struct {
	WorkID         string
	Slice          string
	Responsibility driver.Responsibility
	Before         string
	Epoch          int64
	Try            int64
	Code           string
	Detail         string
}

// credentialLifetimeParkCrossings is a pure function of state and snapshot,
// exactly like hostEnvironmentParkCrossings: it scans the journaled
// credential-lifetime-park facts, keeps the latest one per WorkID
// (CreatedAt order, matching snapshot.Events' own append order), keeps only
// crossings that are still current (dispatchAuthorityCurrent - the identical
// predicate dispatchRoleWithScope itself uses to refuse a stale dispatch, so
// the write and read sides can never disagree about what "current" means),
// and excludes any crossing whose WorkID already has a driver.dispatch
// attempt effect at a position (epoch, try) at or after the fact's own
// (Epoch, Try): a real attempt at or after that position means admission
// passed at least once since. Both pinCrossingLanes (which never reads this
// function - S3's own deliberate divergence from every other ParkCause) and
// Status (the reader side) never disagree because there is only one reader.
func credentialLifetimeParkCrossings(
	state protocol.State,
	snapshot journal.Snapshot,
) []credentialLifetimeCrossing {
	latest := make(map[string]credentialLifetimeCrossing)
	var order []string
	for _, event := range snapshot.Events {
		if event.Kind != "credential_lifetime_park" {
			continue
		}
		var fact credentialLifetimeParkFact
		if json.Unmarshal(event.Body, &fact) != nil ||
			fact.SchemaVersion != CredentialLifetimeParkFactSchemaVersion ||
			fact.WorkID == "" || fact.Code == "" {
			continue
		}
		if _, exists := latest[fact.WorkID]; !exists {
			order = append(order, fact.WorkID)
		}
		latest[fact.WorkID] = credentialLifetimeCrossing{
			WorkID:         fact.WorkID,
			Slice:          fact.Slice,
			Responsibility: driver.Responsibility(fact.Responsibility),
			Before:         fact.Before,
			Epoch:          fact.Epoch,
			Try:            fact.Try,
			Code:           fact.Code,
			Detail:         fact.Detail,
		}
	}
	type attemptPosition struct{ epoch, try int64 }
	attempted := make(map[string][]attemptPosition, len(snapshot.Effects))
	for _, effect := range snapshot.Effects {
		if effect.Kind != "driver.dispatch" {
			continue
		}
		work, epoch, try, err := attemptCoordinates(effect.ID)
		if err != nil {
			continue
		}
		attempted[work] = append(attempted[work], attemptPosition{epoch: epoch, try: try})
	}
	var crossings []credentialLifetimeCrossing
	for _, workID := range order {
		crossing := latest[workID]
		if !dispatchAuthorityCurrent(
			state, crossing.Slice, crossing.Responsibility, crossing.Before,
		) {
			continue
		}
		cleared := false
		for _, position := range attempted[crossing.WorkID] {
			if position.epoch > crossing.Epoch ||
				(position.epoch == crossing.Epoch && position.try >= crossing.Try) {
				cleared = true
				break
			}
		}
		if cleared {
			continue
		}
		crossings = append(crossings, crossing)
	}
	return crossings
}

// credentialLifetimeCrossingOwner maps a crossing to the lane-visible work
// identity readyLaneCandidates itself would add for that exact
// responsibility (status.go:1016-1036): a nested implementer_implementation
// dispatch is owned by its enclosing git.seal work, while
// implementer_design, lead_review, work_verification and
// assembly_verification are each already, themselves, the lane candidate
// work readyLaneCandidates adds - no remapping needed. planner_proposal and
// lead_plan_review fail dispatchAuthorityCurrent's own default (false) and
// never reach here as a live crossing; they are named only for completeness.
func credentialLifetimeCrossingOwner(crossing credentialLifetimeCrossing) string {
	switch crossing.Responsibility {
	case driver.ImplementerImplementation:
		return workIdentity(crossing.Before, "git.seal")
	case driver.ImplementerDesign, driver.LeadReview,
		driver.WorkVerification, driver.AssemblyVerification:
		return crossing.WorkID
	default:
		return ""
	}
}

// credentialLifetimeParkFacts carries everything a park surface names for a
// work-scoped credential-lifetime crossing: the owning work, the stable
// failure code, and the rendered (duration-only) detail.
type credentialLifetimeParkFacts struct {
	work   string
	code   string
	detail string
}

// credentialLifetimeParkFactsByOwner builds resolveLanePins' input map from
// the currently-active crossings, keyed by owner work identity, mirroring
// hostEnvironmentParkFactsByOwner's first-encountered-wins dedup.
func credentialLifetimeParkFactsByOwner(
	state protocol.State,
	snapshot journal.Snapshot,
) map[string]credentialLifetimeParkFacts {
	result := make(map[string]credentialLifetimeParkFacts)
	for _, crossing := range credentialLifetimeParkCrossings(state, snapshot) {
		owner := credentialLifetimeCrossingOwner(crossing)
		if owner == "" {
			continue
		}
		if _, exists := result[owner]; exists {
			continue
		}
		result[owner] = credentialLifetimeParkFacts{
			work: owner, code: crossing.Code, detail: crossing.Detail,
		}
	}
	return result
}
