package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

// derivedWorks returns the set of work identities a git.seal command
// projects as prepared work. These are the nested works whose dispatch
// effects must never drive run-level park evaluation on their own, matching
// the t3 exhaustion scan's exclusion.
func derivedWorks(snapshot journal.Snapshot) map[string]struct{} {
	result := make(map[string]struct{})
	for _, command := range snapshot.Commands {
		if command.Kind != "git.seal" {
			continue
		}
		var probe struct {
			DispatchWork string `json:"dispatch_work"`
			PreparedWork string `json:"prepared_work"`
		}
		if json.Unmarshal(command.Payload, &probe) == nil {
			if probe.DispatchWork != "" {
				result[probe.DispatchWork] = struct{}{}
			}
			if probe.PreparedWork != "" {
				result[probe.PreparedWork] = struct{}{}
			}
		}
	}
	return result
}

// economyGuardsParked reports whether the journal currently crosses an
// economy or identical-failure guard for the given work. It is the
// between-tries admission check: dispatch try chains evaluate it after each
// durably failed try and stop before the next try burns when a guard crossed
// for this work, surfacing EFFECT_PARKED which the drive loop already treats
// as a benign park. work is the outer work identity the try chain is driving
// (a direct dispatch's own work, or the enclosing git.seal work for a nested
// implementer dispatch): a crossing on a different work never blocks this
// one's next try, matching the same lane-scoping the drive loop's own gate
// applies. The drive loop's own gate then writes the typed park event and
// returns.
func (s *Service) economyGuardsParked(
	ctx context.Context,
	manifest admittedManifest,
	runID string,
	work string,
) (bool, error) {
	snapshot, err := s.journal.Snapshot(ctx, runID)
	if err != nil {
		return false, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	control, err := s.journal.ControlProjection(ctx, runID)
	if err != nil {
		return false, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	for _, crossing := range economyParkCrossings(snapshot, control) {
		if ownerWorkForDispatch(snapshot, crossing.work) == work {
			return true, nil
		}
	}
	for _, crossing := range identicalFailureParkCrossings(
		manifest,
		snapshot,
		control,
		manifest.value.EffectiveIdenticalFailureParkAfter(),
	) {
		if ownerWorkForDispatch(snapshot, crossing.work) == work {
			// This gate can return directly out of the drive loop. Persist
			// the park here so notification consumers see the stop even
			// when there is no subsequent scheduler tick.
			body, err := identicalFailureParkEventBody(runID, work, crossing)
			if err != nil {
				return false, err
			}
			return true, s.appendParkEventOnce(ctx, runID, ParkCauseIdenticalFailure, body)
		}
	}
	return false, nil
}

// dispatchCycleOwner scans the journal's git.seal commands for the one
// whose payload names dispatchWork as its own cycle's dispatch work, and
// returns both the outer work identity that cycle was sealed under (the
// git.seal command's own before authority, recomputed deterministically)
// and the outer epoch that cycle was actually built under - recovered from
// the git.seal command's own ReplayKey, which implementSlice always sets to
// AttemptEffectID(workID, epoch, try) for the outer work's own attempt,
// regardless of which nested dispatch-work identity convention it chose for
// dispatchWork itself. found is false only for a direct dispatch (no git.seal
// cycle ever names it), in which case owner echoes dispatchWork unchanged.
// epochKnown is false only when a git.seal cycle names dispatchWork but its
// own ReplayKey fails to parse (a malformed or foreign journal entry);
// callers must not treat a zero outerEpoch as a real epoch in that case.
func dispatchCycleOwner(
	snapshot journal.Snapshot,
	dispatchWork string,
) (owner string, outerEpoch int64, found bool, epochKnown bool) {
	for _, command := range snapshot.Commands {
		if command.Kind != "git.seal" {
			continue
		}
		var cycle struct {
			Before       string `json:"before"`
			DispatchWork string `json:"dispatch_work"`
		}
		if json.Unmarshal(command.Payload, &cycle) != nil ||
			cycle.DispatchWork != dispatchWork {
			continue
		}
		owner = workIdentity(cycle.Before, "git.seal")
		if _, epoch, _, err := attemptCoordinates(command.ReplayKey); err == nil {
			outerEpoch = epoch
			epochKnown = true
		}
		return owner, outerEpoch, true, epochKnown
	}
	return dispatchWork, 0, false, false
}

// ownerWorkForDispatch maps a driver.dispatch work identity to the work
// identity a lane-scoped park gate is scoped to: the dispatch's own work
// identity when it is a direct dispatch, or the enclosing git.seal work when
// it is a nested implementer dispatch (readyLaneCandidates adds only the
// outer git.seal work to a track's implement-stage candidate set, never the
// inner dispatch work, matching the exhaustion scan's derived-work
// exclusion).
func ownerWorkForDispatch(snapshot journal.Snapshot, dispatchWork string) string {
	owner, _, found, _ := dispatchCycleOwner(snapshot, dispatchWork)
	if !found {
		return dispatchWork
	}
	return owner
}

// dispatchAttemptIsCurrentEpoch reports whether a driver.dispatch effect
// whose parsed identity is (work, epoch) is still built under the current
// retry epoch (S4-resumable-budget-stops V2): the same comparison every
// lane-scoped park/retry admission gate needs, generalized across every
// dispatch-work identity convention implementSlice can build.
//
// For a direct dispatch, work is its own RetryEpochs entry, exactly as
// Retry/Grant already key and advance it, unchanged from before this
// feature.
//
// For any nested git.seal-wrapped dispatch, work is never the right key:
// PinnedWork.WorkID - what the board names and what a Retry command's
// WorkID and a Grant's RetryWorkID both target (see
// ControlCommand.RetryWorkID and resolveLanePins) - is always the owner,
// never the dispatch work, so the owner's RetryEpochs entry is the only
// counter Retry or Grant ever actually advances for a nested dispatch; a
// nested dispatch's own RetryEpochs entry is never written by any caller.
// What "epoch" means for the comparison then splits by convention:
//
//   - The stable, epoch-independent identity the recovery-enabled
//     production path builds (workIdentity(workID,"driver.dispatch"),
//     constant across every epoch and try of that work) carries the outer
//     epoch in the dispatch effect's own parsed epoch field (childEpoch is
//     set to the outer epoch at build time), so epoch compares directly
//     against the owner's current RetryEpochs entry.
//   - The per-attempt, epoch/try-embedded identity the recovery-disabled or
//     scope-refusal-escaped path builds
//     (workIdentity(effectID,"driver.dispatch"), a fresh identity every
//     outer epoch and try) always sets childEpoch/childTry to 1: epoch is
//     structurally always 1 and carries no information about which outer
//     epoch actually built it, so it can never be the right value to
//     compare - comparing it anyway (as a prior revision of this function
//     did, by resolving to the same "owner" key and relying on the parsed
//     epoch field for both conventions) either wrongly treats a superseded
//     dispatchWork as permanently current (against a never-written
//     RetryEpochs[work] default) or wrongly treats a genuinely fresh one as
//     permanently stale (once the owner's epoch has advanced past 1),
//     because 1 never actually reflects the outer epoch this specific
//     dispatchWork was built under. The outer epoch that actually built it
//     is instead recovered structurally: dispatchCycleOwner reads it back
//     from the owning git.seal command's own ReplayKey (always
//     AttemptEffectID(workID, outerEpoch, try) for the outer work's own
//     attempt, regardless of which convention chose dispatchWork), and that
//     recovered value - not the dispatch effect's own parsed epoch - is
//     compared against the owner's current RetryEpochs entry.
//
// When the owning git.seal command's own ReplayKey fails to parse on the
// per-attempt convention, the outer epoch this dispatchWork was built under
// cannot be recovered at all. That is a malformed-journal condition, not
// evidence of staleness: treating it as epoch 0 would silently drop a real
// crossing from economyParkCrossings and let a bare Retry bypass the grant
// requirement it exists to enforce (fails open). This function instead
// fails closed - it reports the attempt current, which keeps the crossing
// parked - exactly as every other reader on this path (attemptCoordinates
// on the effect ID itself, validateDriverRecoveryCommand's CORRUPT_JOURNAL)
// already refuses rather than silently skips an unparseable identity.
func dispatchAttemptIsCurrentEpoch(
	snapshot journal.Snapshot,
	control journal.ControlProjection,
	work string,
	epoch int64,
) bool {
	owner, outerEpoch, nested, epochKnown := dispatchCycleOwner(snapshot, work)
	if !nested {
		current := control.RetryEpochs[work]
		if current == 0 {
			current = 1
		}
		return epoch == current
	}
	current := control.RetryEpochs[owner]
	if current == 0 {
		current = 1
	}
	if workIdentity(owner, "driver.dispatch") == work {
		return epoch == current
	}
	if !epochKnown {
		return true
	}
	return outerEpoch == current
}

// economyGuardCrossing names one current-epoch driver.dispatch effect whose
// failure code proves a per-work economy budget crossing (A1). The durable
// code is the authority; the spent figures are read back from that exact
// attempt's preserved usage receipt.
type economyGuardCrossing struct {
	work     string
	epoch    int64
	try      int64
	effectID string
	code     string
}

// economyParkCrossings scans every driver.dispatch effect, including the
// nested implementation dispatch works: that is exactly where the API
// conversation loop lives and where the economy codes land. It returns one
// crossing per distinct work (the highest-try current-epoch economy failure
// for that work), in first-encountered order, so a lane-scoped caller can
// evaluate every currently-crossed work uniformly instead of stopping at the
// first one found (A1).
func economyParkCrossings(
	snapshot journal.Snapshot,
	control journal.ControlProjection,
) []economyGuardCrossing {
	byWork := make(map[string]*economyGuardCrossing)
	var order []string
	for _, effect := range snapshot.Effects {
		if effect.Kind != "driver.dispatch" ||
			effect.State != journal.OperationalFailed {
			continue
		}
		if effect.ErrorCode != "ECONOMY_TURN_BUDGET_EXCEEDED" &&
			effect.ErrorCode != "ECONOMY_OUTPUT_BUDGET_EXCEEDED" {
			continue
		}
		work, epoch, try, coordErr := attemptCoordinates(effect.ID)
		if coordErr != nil {
			continue
		}
		if !dispatchAttemptIsCurrentEpoch(snapshot, control, work, epoch) {
			continue
		}
		existing, found := byWork[work]
		if !found {
			order = append(order, work)
		}
		if !found || try > existing.try {
			byWork[work] = &economyGuardCrossing{
				work:     work,
				epoch:    epoch,
				try:      try,
				effectID: effect.ID,
				code:     effect.ErrorCode,
			}
		}
	}
	result := make([]economyGuardCrossing, 0, len(order))
	for _, work := range order {
		result = append(result, *byWork[work])
	}
	return result
}

// economyParkFacts carries everything a park surface names for an economy
// crossing: the cause, the engine-counted spend, the effective budget the
// dispatch crossed, and the manifest knob that unblocks it.
type economyParkFacts struct {
	// work is the crossing's own dispatch-work identity (economyGuardCrossing.work):
	// the same identity a Grant targeting this crossing must name as its
	// ControlCommand.WorkID, distinct from the lane-scoped owner identity
	// callers key their own per-owner maps by (S4-resumable-budget-stops V3).
	work   string
	cause  string
	spent  int64
	budget int64
	knob   string
}

// economyParkFactsFor names the park facts for a crossing. Both the
// token-denominated conversation-loop crossing and the byte-denominated
// native output-stream crossing (S3-output-stream-economy A4) share the
// ECONOMY_OUTPUT_BUDGET_EXCEEDED top-level code, so diagnosticCode - the
// observation's Diagnostic.Code, read back by economySpent - is the
// disambiguating signal; the top-level code alone cannot tell them apart.
// limits is the exact per-work economy ceiling the crossing's own attempt
// was dispatched under - economyCrossingDispatchedLimits' result: that
// attempt's frozen EffectiveLimits when captureEffectiveLimits had already
// applied an admitted grant at its dispatch-build time, else the plain
// manifest limits. spent and budget must always share that one attempt's
// own basis (S4-resumable-budget-stops V1): a live, possibly-since-changed
// cumulative grant total is never comparable to spentTurns/spentTokens/
// spentBytes, which economySpent reads back from that exact attempt alone,
// because a later grant's headroom recomputation folds in every attempt's
// spend since the work's very first dispatch, not just this one's. Because
// the driver dispatch itself fails exactly when its own turn/token/byte
// count reaches the limits it was given (driver/provider.go), reporting
// that same limits value back as budget makes spent >= budget hold by
// construction on every crossing, first or later, and names the board the
// exact ceiling the dispatch actually ran under (A1, C6).
func economyParkFactsFor(
	crossing economyGuardCrossing,
	limits driver.Limits,
	spentTurns, spentTokens, spentBytes int64,
	diagnosticCode string,
) economyParkFacts {
	facts := economyParkFacts{work: crossing.work}
	switch {
	case crossing.code == "ECONOMY_TURN_BUDGET_EXCEEDED":
		facts.cause = ParkCauseEconomyTurns
		facts.spent = spentTurns
		facts.budget = limits.EffectiveMaxTurnsPerWork()
		facts.knob = EconomyTurnsUnblockKnob
	case crossing.code == "ECONOMY_OUTPUT_BUDGET_EXCEEDED" &&
		diagnosticCode == "economy_output_budget_bytes":
		facts.cause = ParkCauseEconomyOutputBytes
		facts.spent = spentBytes
		facts.budget = limits.EffectiveMaxNativeOutputStreamBytes()
		facts.knob = EconomyOutputBytesUnblockKnob
	case crossing.code == "ECONOMY_OUTPUT_BUDGET_EXCEEDED":
		facts.cause = ParkCauseEconomyOutputTokens
		facts.spent = spentTokens
		facts.budget = limits.EffectiveMaxOutputTokensPerWork()
		facts.knob = EconomyOutputTokensUnblockKnob
	}
	return facts
}

// economyCrossingDispatchedLimits returns the exact per-work economy limits
// the crossing's own attempt was dispatched under (S4-resumable-budget-stops
// V1): its frozen productionWorkContext.EffectiveLimits, read back from that
// exact attempt's own driver.dispatch command by replay key (the same
// dispatchContext seam economyWorkSpent's acknowledged-gap fallback already
// reads), when captureEffectiveLimits had already applied an admitted grant
// at that attempt's own dispatch-build time; else the manifest's own raw
// limits, exactly as an attempt dispatched before any grant existed. This is
// never a live re-derivation from the current, possibly-since-advanced
// grant total: it is always the one figure the crossing's own attempt was
// actually bound to.
func economyCrossingDispatchedLimits(
	snapshot journal.Snapshot,
	manifestLimits driver.Limits,
	crossing economyGuardCrossing,
) driver.Limits {
	for _, command := range snapshot.Commands {
		if command.ReplayKey != crossing.effectID || command.Kind != "driver.dispatch" {
			continue
		}
		var parsed productionDispatchCommand
		if json.Unmarshal(command.Payload, &parsed) == nil &&
			parsed.Context.EffectiveLimits != nil {
			return *parsed.Context.EffectiveLimits
		}
		break
	}
	return manifestLimits
}

// decodeAttemptUsageReceipt is the shared digest-verified decode seam for an
// attempt's observation body: the canonical inner usage receipt plus its
// diagnostic code, re-verified byte-identical against its own re-encoding.
// It refuses a not-stored or partial observation as corruption, matching
// the guarantee callers that only ever invoke it against a proven-crossing
// attempt already rely on.
func decodeAttemptUsageReceipt(
	observed journal.AttemptObservation,
) (driver.UsageReceipt, string, error) {
	if !observed.Stored || observed.Partial {
		return driver.UsageReceipt{}, "", runtimeFail("CORRUPT_JOURNAL", nil)
	}
	var body struct {
		Usage      json.RawMessage `json:"usage"`
		Diagnostic struct {
			Code string `json:"code"`
		} `json:"diagnostic"`
	}
	if err := json.Unmarshal(observed.Body, &body); err != nil ||
		len(body.Usage) == 0 {
		return driver.UsageReceipt{}, "", runtimeFail("CORRUPT_JOURNAL", nil)
	}
	var receipt driver.UsageReceipt
	decoder := json.NewDecoder(bytes.NewReader(body.Usage))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return driver.UsageReceipt{}, "", runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return driver.UsageReceipt{}, "", runtimeFail("CORRUPT_JOURNAL", nil)
	}
	canonical, err := driver.EncodeUsageReceipt(receipt)
	if err != nil || !bytes.Equal(canonical, body.Usage) {
		return driver.UsageReceipt{}, "", runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return receipt, body.Diagnostic.Code, nil
}

// economySpent reads the engine-counted spend back from the exact attempt
// that crossed the budget, through the targeted, digest-verified
// AttemptObservation seam (never the recency-windowed ReadObservation, whose
// 256-attempt window can silently un-park a long run). The body is the
// marshaled driver observation; its usage receipt must re-encode
// canonically. An attempt that cannot be read is corruption on a journal
// that claims a crossing, so the gate fails closed.
func (s *Service) economySpent(
	ctx context.Context,
	runID string,
	crossing economyGuardCrossing,
) (spentTurns, spentTokens, spentBytes int64, diagnosticCode string, err error) {
	observed, err := s.journal.AttemptObservation(
		ctx,
		runID,
		crossing.effectID,
		crossing.try,
	)
	if err != nil {
		return 0, 0, 0, "", runtimeFail("JOURNAL_READ_FAILED", err)
	}
	receipt, diagnosticCode, err := decodeAttemptUsageReceipt(observed)
	if err != nil {
		return 0, 0, 0, "", err
	}
	if receipt.Turns != nil {
		spentTurns = *receipt.Turns
	}
	if receipt.OutputTokens != nil {
		spentTokens = *receipt.OutputTokens
	}
	if receipt.NativeStreamBytes != nil {
		spentBytes = *receipt.NativeStreamBytes
	}
	return spentTurns, spentTokens, spentBytes, diagnosticCode, nil
}

// economyEffectiveOriginal names the manifest's own effective per-work
// ceiling for one Grant unit, before any admitted grant: the same
// Effective* accessor productionRequestForContextFreshness would otherwise
// feed the driver unchanged.
func economyEffectiveOriginal(limits driver.Limits, unit string) int64 {
	switch unit {
	case ParkCauseEconomyTurns:
		return limits.EffectiveMaxTurnsPerWork()
	case ParkCauseEconomyOutputTokens:
		return limits.EffectiveMaxOutputTokensPerWork()
	case ParkCauseEconomyOutputBytes:
		return limits.EffectiveMaxNativeOutputStreamBytes()
	default:
		return 0
	}
}

// economyHardCeiling names the absolute, manifest-independent hard ceiling
// for one Grant unit: the exact constants driver.ValidateRequest already
// enforces, never a value this feature invents.
func economyHardCeiling(unit string) int64 {
	switch unit {
	case ParkCauseEconomyTurns:
		return driver.MaxTurnsPerWorkLimit
	case ParkCauseEconomyOutputTokens:
		return driver.MaxOutputTokensPerWorkLimit
	case ParkCauseEconomyOutputBytes:
		return driver.MaxNativeOutputStreamBytesLimit
	default:
		return 0
	}
}

// saturatingAddInt64 adds two non-negative int64 values without wrapping.
// Every caller in this feature only ever adds non-negative amounts (a
// manifest ceiling, a cumulative granted total, or a validated positive
// Grant amount), so an amount large enough to overflow trivially exceeds
// any finite hard ceiling once saturated.
func saturatingAddInt64(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// economyWorkSpent folds a work's cumulative recorded economy spend across
// every driver.dispatch attempt whose owner work resolves to work (A3): the
// engine-counted turns, output tokens and native-stream bytes actually
// recorded, restart- and retry-safe because it is recomputed from durable
// journal history rather than cached. unknown is true only when a terminal
// (non-claimed, non-pending) attempt's usage cannot be read back and
// acknowledged is false; an acknowledged gap instead substitutes that
// attempt's own already-declared per-attempt ceiling (its frozen
// EffectiveLimits when it had one, else the raw manifest limits) — a real,
// bounded number, never zero and never an invented vendor total.
func economyWorkSpent(
	ctx context.Context,
	store *journal.Store,
	manifest admittedManifest,
	runID string,
	work string,
	acknowledged bool,
) (turns, tokens, nativeBytes int64, unknown bool, err error) {
	snapshot, snapErr := store.Snapshot(ctx, runID)
	if snapErr != nil {
		return 0, 0, 0, false, runtimeFail("JOURNAL_READ_FAILED", snapErr)
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		commands[command.ReplayKey] = command
	}
	for _, effect := range snapshot.Effects {
		if effect.Kind != "driver.dispatch" ||
			effect.State == journal.Pending || effect.State == journal.Claimed {
			continue
		}
		dispatchWork, _, try, coordErr := attemptCoordinates(effect.ID)
		if coordErr != nil || ownerWorkForDispatch(snapshot, dispatchWork) != work {
			continue
		}
		observed, obsErr := store.AttemptObservation(ctx, runID, effect.ID, try)
		if obsErr != nil && !journal.IsCode(obsErr, "ATTEMPT_NOT_FOUND") {
			return 0, 0, 0, false, runtimeFail("JOURNAL_READ_FAILED", obsErr)
		}
		if obsErr != nil || !observed.Stored || observed.Partial {
			if !acknowledged {
				unknown = true
				continue
			}
			ceiling := manifest.value.Limits
			if attemptContext, ok := dispatchContext(commands, effect.ReplayKey); ok &&
				attemptContext.EffectiveLimits != nil {
				ceiling = *attemptContext.EffectiveLimits
			}
			turns += ceiling.EffectiveMaxTurnsPerWork()
			tokens += ceiling.EffectiveMaxOutputTokensPerWork()
			nativeBytes += ceiling.EffectiveMaxNativeOutputStreamBytes()
			continue
		}
		receipt, _, decodeErr := decodeAttemptUsageReceipt(observed)
		if decodeErr != nil {
			return 0, 0, 0, false, decodeErr
		}
		if receipt.Turns != nil {
			turns += *receipt.Turns
		}
		if receipt.OutputTokens != nil {
			tokens += *receipt.OutputTokens
		}
		if receipt.NativeStreamBytes != nil {
			nativeBytes += *receipt.NativeStreamBytes
		}
	}
	return turns, tokens, nativeBytes, unknown, nil
}

// identicalFailureFacts carries everything a park surface names for an
// identical-failure crossing (A2): the shared error code, its durable
// refusal detail, the consecutive run length, and the effective threshold.
type identicalFailureFacts struct {
	work        string
	code        string
	detail      string
	consecutive int64
	threshold   int64
}

// identicalFailureParkCrossings scans journal history for every work whose
// current-epoch dispatch attempts end in N consecutive operational failures
// sharing one error code, where N reaches the manifest threshold before try
// exhaustion. Non-failed states, a differing code, and absent attempts break
// the consecutive suffix exactly as the contract's "consecutive operational
// failures with the same error code" reads. The durable refusal detail rides
// the last failing try's effect result (telemetry-foundations S5). Nested
// implementation dispatch works are included: they are where the API
// conversation failures actually land. It returns one crossing per
// qualifying work in first-encountered order so a lane-scoped caller can
// evaluate every currently-crossed work uniformly (A1), and before
// returning a work's crossing it applies the lineage-keyed freshness rule
// (A3): a later success sharing the streak's slice+responsibility lineage,
// at an attempt no lower than the streak's own attempt, suppresses that
// work's crossing because the streak is stale, not a live park cause.
func identicalFailureParkCrossings(
	manifest admittedManifest,
	snapshot journal.Snapshot,
	control journal.ControlProjection,
	threshold int64,
) []identicalFailureFacts {
	if threshold < 1 {
		return nil
	}
	byWork := make(map[string]map[int64]journal.Effect)
	var order []string
	for _, effect := range snapshot.Effects {
		if effect.Kind != "driver.dispatch" {
			continue
		}
		work, epoch, try, coordErr := attemptCoordinates(effect.ID)
		if coordErr != nil {
			continue
		}
		if !dispatchAttemptIsCurrentEpoch(snapshot, control, work, epoch) {
			continue
		}
		tries, ok := byWork[work]
		if !ok {
			tries = make(map[int64]journal.Effect)
			byWork[work] = tries
			order = append(order, work)
		}
		tries[try] = effect
	}
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		commands[command.ReplayKey] = command
	}
	var result []identicalFailureFacts
	for _, work := range order {
		var (
			runCode    string
			runLength  int64
			lastFailed journal.Effect
			failed     bool
		)
		for try := int64(1); try <= 3; try++ {
			effect, ok := byWork[work][try]
			if !ok {
				break
			}
			if effect.State != journal.OperationalFailed {
				break
			}
			if runCode != "" && effect.ErrorCode != runCode {
				runCode = effect.ErrorCode
				runLength = 1
				lastFailed = effect
				failed = true
				continue
			}
			if runCode == "" {
				runCode = effect.ErrorCode
			}
			runLength++
			lastFailed = effect
			failed = true
		}
		if !failed || runCode == "" || runLength < threshold {
			continue
		}
		if context, ok := dispatchContext(commands, lastFailed.ReplayKey); ok {
			lineageKey := dispatchLineageKey(manifest, context)
			if lineageHasLaterSuccess(
				manifest, snapshot, commands, lineageKey, work, context.Attempt,
			) {
				continue
			}
		}
		result = append(result, identicalFailureFacts{
			work:        work,
			code:        runCode,
			detail:      refusalDetail(lastFailed.Result, runCode),
			consecutive: runLength,
			threshold:   threshold,
		})
	}
	return result
}

// dispatchContext reads back the persisted work context of a driver.dispatch
// command by its replay key, without the full production-request-replay
// validation validateDriverRecoveryCommand performs: the lineage key this
// feeds is a read-time convenience derived from data already on the
// journal, not an authority boundary. A scripted fake dispatch's payload
// carries no context.responsibility and is correctly reported absent.
func dispatchContext(
	commands map[string]journal.Command,
	replayKey string,
) (productionWorkContext, bool) {
	command, ok := commands[replayKey]
	if !ok || command.Kind != "driver.dispatch" {
		return productionWorkContext{}, false
	}
	var parsed productionDispatchCommand
	if json.Unmarshal(command.Payload, &parsed) != nil ||
		parsed.Context.Responsibility == "" {
		return productionWorkContext{}, false
	}
	return parsed.Context, true
}

// dispatchLineageKey derives the coarser slice+responsibility lineage key a
// work's identical-failure streak is judged against: the same manifest,
// slice, and responsibility a fresh dispatch of the same stage always
// shares, with the attempt and before-authority churn that lane-scoped
// parking makes more common dropped entirely (A3).
func dispatchLineageKey(
	manifest admittedManifest,
	context productionWorkContext,
) string {
	return workIdentity(manifest.digest, context.Slice, context.Responsibility)
}

// lineageHasLaterSuccess reports whether the journal already carries a
// Succeeded driver.dispatch effect sharing lineageKey, for a work other than
// the streak's own, whose persisted attempt is no lower than the streak's
// attempt. Baton advances the attempt before the same responsibility
// dispatches again, so "no lower" can neither under-park an unbroken streak
// (which has no succeeded dispatch in its own lineage at or after its own
// attempt) nor over-break a genuine one.
func lineageHasLaterSuccess(
	manifest admittedManifest,
	snapshot journal.Snapshot,
	commands map[string]journal.Command,
	lineageKey string,
	streakWork string,
	streakAttempt int64,
) bool {
	for _, effect := range snapshot.Effects {
		if effect.Kind != "driver.dispatch" || effect.State != journal.Succeeded {
			continue
		}
		work, _, _, coordErr := attemptCoordinates(effect.ID)
		if coordErr != nil || work == streakWork {
			continue
		}
		context, ok := dispatchContext(commands, effect.ReplayKey)
		if !ok || context.Attempt < streakAttempt {
			continue
		}
		if dispatchLineageKey(manifest, context) == lineageKey {
			return true
		}
	}
	return false
}

// refusalDetail reads the durable refusal detail off a failed effect's
// result BLOB. Empty is honest absence for codes that carry no provider
// detail.
func refusalDetail(result []byte, code string) string {
	if len(result) == 0 {
		return ""
	}
	if code == "HOST_CHECK_FAILED" {
		var repair productionHostRepair
		if json.Unmarshal(result, &repair) == nil && validateHostRepair(repair, repair.Submission.InvocationID, repair.FailedCheck.Slice) == nil {
			return fmt.Sprintf("Host check %s for retained unverified candidate %s (exit %d). Inspect host_repair.failed_check before retrying.", repair.FailedCheck.Outcome, repair.FailedCheck.Candidate, repair.FailedCheck.ExitCode)
		}
	}
	var refusal productionRefusalBinding
	if err := json.Unmarshal(result, &refusal); err != nil {
		return ""
	}
	if refusal.Code != code || refusal.Detail == "" {
		return ""
	}
	if !validParkDetail(refusal.Detail) {
		return ""
	}
	return refusal.Detail
}

// economyParkEventBody builds the canonical typed park event body for an
// economy crossing, work-scoped by the crossing's own owning work.
func economyParkEventBody(runID, work string, facts economyParkFacts) ([]byte, error) {
	return canonicalDegradationParkEvent(DegradationParkEvent{
		SchemaVersion: ParkEventVersion,
		RunID:         runID,
		Cause:         facts.cause,
		Budget:        facts.budget,
		Spent:         facts.spent,
		UnblockKnob:   facts.knob,
		Work:          work,
	})
}

// identicalFailureParkEventBody builds the canonical typed park event body
// for an identical-failure crossing, work-scoped by the crossing's own
// owning work.
func identicalFailureParkEventBody(
	runID, work string,
	facts identicalFailureFacts,
) ([]byte, error) {
	return canonicalDegradationParkEvent(DegradationParkEvent{
		SchemaVersion: ParkEventVersion,
		RunID:         runID,
		Cause:         ParkCauseIdenticalFailure,
		Consecutive:   facts.consecutive,
		Threshold:     facts.threshold,
		FailureCode:   facts.code,
		FailureDetail: facts.detail,
		UnblockKnob:   IdenticalFailureUnblockKnob,
		Work:          work,
	})
}

// parkEventReplayKey derives the cause-scoped command replay key that makes
// park-event idempotence cause-scoped: each cause can be admitted exactly
// once, and a park event of one cause can never suppress a later park event
// of another cause sharing the journal kind.
func parkEventReplayKey(cause string, body []byte) string {
	return "park-event/" + cause + "/" +
		strings.TrimPrefix(sha256Digest(body), "sha256:")
}

// appendParkEventOnce writes one typed park event under the shared journal
// kind, exactly once per cause. The command replay key is the uniqueness
// boundary, so concurrent drive loops either admit the exact event once or
// observe the already-admitted fact without a check-then-append race.
func (s *Service) appendParkEventOnce(
	ctx context.Context,
	runID, cause string,
	body []byte,
) error {
	if err := s.journal.AppendEventOnce(ctx, journal.Command{
		RunID:     runID,
		ReplayKey: parkEventReplayKey(cause, body),
		Kind:      "park-event",
		Payload:   body,
		CreatedAt: s.now().UTC(),
	}, ParkEventKind, body, s.now().UTC()); err != nil {
		return runtimeFail("JOURNAL_WRITE_FAILED", err)
	}
	return nil
}

// hasParkEventForCause reports whether the journal already carries a typed
// park event of the shared kind for one cause, scoped to work when work is
// non-empty: two different works crossing the same cause must each get
// their own park event, which a cause-only match would wrongly treat as
// already admitted after the first. A same-kind event whose body cannot be
// parsed is treated as present (fail closed) so a park is never
// double-written behind an unreadable body. work is empty for the run-scoped
// causes (degradation, attention, human_authority), matching every event
// those causes ever produce.
func hasParkEventForCause(
	snapshot journal.Snapshot,
	cause string,
	work string,
) bool {
	for _, event := range snapshot.Events {
		if event.Kind != ParkEventKind {
			continue
		}
		parsed, err := ParseDegradationParkEvent(event.Body)
		if err != nil {
			return true
		}
		if parsed.Cause == cause && parsed.Work == work {
			return true
		}
	}
	return false
}

// exhaustedWorks returns every work whose current-epoch try budget is spent
// (a third try that failed operationally), with the durable refusal facts
// the exhausting effect carried. Both the status projection and the drive
// loop read exhaustion from here: the fact is journal-derived, so it is the
// same fact on both surfaces and it moves only when the journal moves.
//
// skipEffects names effects a deliberate recovery claim already owns; they
// are excluded exactly as the status effect walk excludes them, so an
// answered recovery turn is never also read as an exhaustion.
func exhaustedWorks(
	snapshot journal.Snapshot,
	control journal.ControlProjection,
	skipEffects map[string]journal.AttentionState,
) (map[string]struct{}, map[string]exhaustionRefusalFacts) {
	exhausted := make(map[string]struct{})
	refusals := make(map[string]exhaustionRefusalFacts)
	derived := derivedWorks(snapshot)
	for _, effect := range snapshot.Effects {
		if effect.ID == "runtime.owner" || effect.Kind == "runtime.control" {
			continue
		}
		if _, deliberate := skipEffects[effect.ID]; deliberate {
			continue
		}
		if effect.State != journal.OperationalFailed ||
			!strings.HasSuffix(effect.ID, "/t3") {
			continue
		}
		parts := strings.Split(effect.ID, "/")
		if len(parts) != 4 {
			continue
		}
		work := "sha256:" + parts[1]
		if _, isDerived := derived[work]; isDerived {
			continue
		}
		epoch, _ := strconv.ParseInt(strings.TrimPrefix(parts[2], "e"), 10, 64)
		// One epoch authority for every park gate: the same predicate the
		// economy crossings use, so a Retry or a Grant advancing the work's
		// epoch spends this park too, and a nested dispatch identity is
		// judged by the outer epoch that built it rather than by a
		// structurally constant one (S4-resumable-budget-stops V2).
		if !dispatchAttemptIsCurrentEpoch(snapshot, control, work, epoch) {
			continue
		}
		exhausted[work] = struct{}{}
		// The persisted effect's ErrorCode is the stable runtime-wrapper
		// code (e.g. CANDIDATE_SCOPE_FAILED); the more specific refusal
		// code (SLICE_OUTSIDE_SCOPE, RESERVED_RECORD_ROOT_CHANGED) rides
		// the effect's journaled productionRefusalBinding result alongside
		// the named paths.
		if effect.ErrorCode == "CANDIDATE_SCOPE_FAILED" {
			if detail := scopeExhaustionDetail(effect.Result); detail != "" {
				refusals[work] = exhaustionRefusalFacts{
					code: effect.ErrorCode, detail: detail,
				}
			}
		} else if effect.ErrorCode == "EMPTY_CANDIDATE" {
			// EMPTY_CANDIDATE is raised via a plain fail(...) with no
			// structured Result to render (unlike CANDIDATE_SCOPE_FAILED's
			// named paths), so the code alone already fully explains the
			// cause.
			refusals[work] = exhaustionRefusalFacts{
				code:   effect.ErrorCode,
				detail: "EMPTY_CANDIDATE: implementation produced no change to seal",
			}
		}
	}
	return exhausted, refusals
}

// exhaustionParkFacts names one standing exhaustion park: the exhausted work,
// the slice lineage the journal attributes it to (empty when no dispatch
// context names one), and the durable refusal code and detail.
type exhaustionParkFacts struct {
	work string
	// slice is the slice the work's dispatch context names; it is empty for
	// a release-lane work (a planner proposal, an assembly dispatch), which
	// attributed distinguishes from a work the journal attributes to no
	// dispatch context at all.
	slice      string
	attributed bool
	code       string
	detail     string
}

// exhaustionParkCrossings turns the currently-exhausted works into standing
// park facts, in stable work order. Each work is attributed to the slice its
// own dispatch context names - through ownerWorkForDispatch, so an enclosing
// git.seal work inherits the slice of the implementer dispatch nested inside
// it - and a work whose slice lineage already carries a later success is
// dropped: new journaled work for that slice spends the park, exactly as it
// spends an identical-failure streak (A3).
//
// The slice is what makes this park survivable. Every candidate work
// identity binds state.Refs.Target.Head, so a commit on the target branch
// moves the identity of work nobody has touched; the slice lineage does not
// move with it, and a park matched by lineage cannot be erased by a
// repository read (sworn#293).
func exhaustionParkCrossings(
	manifest admittedManifest,
	snapshot journal.Snapshot,
	exhausted map[string]struct{},
	refusals map[string]exhaustionRefusalFacts,
) []exhaustionParkFacts {
	if len(exhausted) == 0 {
		return nil
	}
	works := make([]string, 0, len(exhausted))
	for work := range exhausted {
		works = append(works, work)
	}
	sort.Strings(works)
	commands := make(map[string]journal.Command, len(snapshot.Commands))
	for _, command := range snapshot.Commands {
		commands[command.ReplayKey] = command
	}
	var result []exhaustionParkFacts
	for _, work := range works {
		context, hasContext := exhaustionDispatchContext(
			snapshot, commands, work,
		)
		if hasContext && lineageHasLaterSuccess(
			manifest, snapshot, commands,
			dispatchLineageKey(manifest, context), work, context.Attempt,
		) {
			continue
		}
		facts := exhaustionParkFacts{work: work}
		if hasContext {
			facts.slice, facts.attributed = context.Slice, true
		}
		if refusal, ok := refusals[work]; ok {
			facts.code, facts.detail = refusal.code, refusal.detail
		}
		result = append(result, facts)
	}
	return result
}

// exhaustionDispatchContext returns the dispatch context of the highest-try
// dispatch the exhausted work owns: the work's own dispatch when it is one,
// or the nested implementer dispatch when the work is its enclosing git.seal.
// Absent context is honest absence - a work with no journaled dispatch
// context names no slice.
func exhaustionDispatchContext(
	snapshot journal.Snapshot,
	commands map[string]journal.Command,
	work string,
) (productionWorkContext, bool) {
	var (
		found productionWorkContext
		best  int64
		ok    bool
	)
	for _, effect := range snapshot.Effects {
		if effect.Kind != "driver.dispatch" {
			continue
		}
		dispatchWork, _, try, coordErr := attemptCoordinates(effect.ID)
		if coordErr != nil ||
			ownerWorkForDispatch(snapshot, dispatchWork) != work {
			continue
		}
		context, hasContext := dispatchContext(commands, effect.ReplayKey)
		if !hasContext || try < best {
			continue
		}
		found, best, ok = context, try, true
	}
	return found, ok
}

// exhaustionParkLane names the candidate lane a standing exhaustion park
// pins: the track owning its slice, or the release pseudo-lane for a work
// with no slice of its own (a planner proposal, an assembly dispatch). A
// park the journal attributes to no dispatch context names no lane, and
// neither does a slice the current Baton state no longer carries: an
// unattributed park is not silently charged to the release lane.
func exhaustionParkLane(
	state baton.State,
	park exhaustionParkFacts,
) (string, bool) {
	if !park.attributed {
		return "", false
	}
	if park.slice == "" {
		return "release", true
	}
	owning, ok := state.Slice(park.slice)
	if !ok {
		return "", false
	}
	return owning.Location.Track.ID, true
}

// exhaustionParksByLane keys the standing exhaustion parks by the candidate
// lane each one pins, keeping the first park per lane so one lane is never
// pinned twice.
func exhaustionParksByLane(
	state baton.State,
	parks []exhaustionParkFacts,
) map[string]exhaustionParkFacts {
	byLane := make(map[string]exhaustionParkFacts, len(parks))
	for _, park := range parks {
		lane, ok := exhaustionParkLane(state, park)
		if !ok {
			continue
		}
		if _, exists := byLane[lane]; exists {
			continue
		}
		byLane[lane] = park
	}
	return byLane
}

// exhaustionParkEventBody renders the typed park event body for one standing
// exhaustion park, so exhaustion records its park in the journal exactly as
// the economy and identical-failure causes already do.
func exhaustionParkEventBody(
	runID string,
	park exhaustionParkFacts,
) ([]byte, error) {
	return canonicalDegradationParkEvent(DegradationParkEvent{
		SchemaVersion: ParkEventVersion,
		RunID:         runID,
		Cause:         ParkCauseExhaustion,
		FailureCode:   park.code,
		FailureDetail: park.detail,
		Work:          park.work,
	})
}
