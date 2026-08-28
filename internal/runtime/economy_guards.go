package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

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
			return true, nil
		}
	}
	return false, nil
}

// ownerWorkForDispatch maps a driver.dispatch work identity to the work
// identity a lane-scoped park gate is scoped to: the dispatch's own work
// identity when it is a direct dispatch, or the enclosing git.seal work when
// it is a nested implementer dispatch (readyLaneCandidates adds only the
// outer git.seal work to a track's implement-stage candidate set, never the
// inner dispatch work, matching the exhaustion scan's derived-work
// exclusion). The git.seal command payload names its own before authority,
// from which the outer work identity is recomputed deterministically.
func ownerWorkForDispatch(snapshot journal.Snapshot, dispatchWork string) string {
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
		return workIdentity(cycle.Before, "git.seal")
	}
	return dispatchWork
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
		current := control.RetryEpochs[work]
		if current == 0 {
			current = 1
		}
		if epoch != current {
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
func economyParkFactsFor(
	crossing economyGuardCrossing,
	limits driver.Limits,
	spentTurns, spentTokens, spentBytes int64,
	diagnosticCode string,
) economyParkFacts {
	facts := economyParkFacts{}
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
	if !observed.Stored || observed.Partial {
		return 0, 0, 0, "", runtimeFail("CORRUPT_JOURNAL", nil)
	}
	var body struct {
		Usage      json.RawMessage `json:"usage"`
		Diagnostic struct {
			Code string `json:"code"`
		} `json:"diagnostic"`
	}
	if err := json.Unmarshal(observed.Body, &body); err != nil ||
		len(body.Usage) == 0 {
		return 0, 0, 0, "", runtimeFail("CORRUPT_JOURNAL", nil)
	}
	var receipt driver.UsageReceipt
	decoder := json.NewDecoder(bytes.NewReader(body.Usage))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return 0, 0, 0, "", runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return 0, 0, 0, "", runtimeFail("CORRUPT_JOURNAL", nil)
	}
	canonical, err := driver.EncodeUsageReceipt(receipt)
	if err != nil || !bytes.Equal(canonical, body.Usage) {
		return 0, 0, 0, "", runtimeFail("CORRUPT_JOURNAL", nil)
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
	return spentTurns, spentTokens, spentBytes, body.Diagnostic.Code, nil
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
		current := control.RetryEpochs[work]
		if current == 0 {
			current = 1
		}
		if epoch != current {
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
