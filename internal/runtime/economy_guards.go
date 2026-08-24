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
// economy or identical-failure guard. It is the between-tries admission
// check: dispatch try chains evaluate it after each durably failed try and
// stop before the next try burns when a guard crossed, surfacing
// EFFECT_PARKED which the drive loop already treats as a benign park. The
// drive loop's own gate then writes the typed park event and returns.
func (s *Service) economyGuardsParked(
	ctx context.Context,
	manifest admittedManifest,
	runID string,
) (bool, error) {
	snapshot, err := s.journal.Snapshot(ctx, runID)
	if err != nil {
		return false, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	control, err := s.journal.ControlProjection(ctx, runID)
	if err != nil {
		return false, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if economyParkCrossing(snapshot, control) != nil {
		return true, nil
	}
	if identicalFailureParkCrossing(
		snapshot,
		control,
		manifest.value.EffectiveIdenticalFailureParkAfter(),
	) != nil {
		return true, nil
	}
	return false, nil
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

// economyParkCrossing scans every driver.dispatch effect, including the
// nested implementation dispatch works: that is exactly where the API
// conversation loop lives and where the economy codes land.
func economyParkCrossing(
	snapshot journal.Snapshot,
	control journal.ControlProjection,
) *economyGuardCrossing {
	var result *economyGuardCrossing
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
		if result == nil ||
			(result.work == work && try > result.try) {
			result = &economyGuardCrossing{
				work:     work,
				epoch:    epoch,
				try:      try,
				effectID: effect.ID,
				code:     effect.ErrorCode,
			}
		}
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

func economyParkFactsFor(
	crossing economyGuardCrossing,
	limits driver.Limits,
	spentTurns, spentTokens int64,
) economyParkFacts {
	facts := economyParkFacts{}
	switch crossing.code {
	case "ECONOMY_TURN_BUDGET_EXCEEDED":
		facts.cause = ParkCauseEconomyTurns
		facts.spent = spentTurns
		facts.budget = limits.EffectiveMaxTurnsPerWork()
		facts.knob = EconomyTurnsUnblockKnob
	case "ECONOMY_OUTPUT_BUDGET_EXCEEDED":
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
) (spentTurns, spentTokens int64, err error) {
	observed, err := s.journal.AttemptObservation(
		ctx,
		runID,
		crossing.effectID,
		crossing.try,
	)
	if err != nil {
		return 0, 0, runtimeFail("JOURNAL_READ_FAILED", err)
	}
	if !observed.Stored || observed.Partial {
		return 0, 0, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	var body struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(observed.Body, &body); err != nil ||
		len(body.Usage) == 0 {
		return 0, 0, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	var receipt driver.UsageReceipt
	decoder := json.NewDecoder(bytes.NewReader(body.Usage))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return 0, 0, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return 0, 0, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	canonical, err := driver.EncodeUsageReceipt(receipt)
	if err != nil || !bytes.Equal(canonical, body.Usage) {
		return 0, 0, runtimeFail("CORRUPT_JOURNAL", nil)
	}
	if receipt.Turns != nil {
		spentTurns = *receipt.Turns
	}
	if receipt.OutputTokens != nil {
		spentTokens = *receipt.OutputTokens
	}
	return spentTurns, spentTokens, nil
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

// identicalFailureParkCrossing scans journal history for a work whose
// current-epoch dispatch attempts end in N consecutive operational failures
// sharing one error code, where N reaches the manifest threshold before try
// exhaustion. Non-failed states, a differing code, and absent attempts break
// the consecutive suffix exactly as the contract's "consecutive operational
// failures with the same error code" reads. The durable refusal detail rides
// the last failing try's effect result (telemetry-foundations S5). Nested
// implementation dispatch works are included: they are where the API
// conversation failures actually land.
func identicalFailureParkCrossing(
	snapshot journal.Snapshot,
	control journal.ControlProjection,
	threshold int64,
) *identicalFailureFacts {
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
		return &identicalFailureFacts{
			work:        work,
			code:        runCode,
			detail:      refusalDetail(lastFailed.Result, runCode),
			consecutive: runLength,
			threshold:   threshold,
		}
	}
	return nil
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
// economy crossing.
func economyParkEventBody(runID string, facts economyParkFacts) ([]byte, error) {
	return canonicalDegradationParkEvent(DegradationParkEvent{
		SchemaVersion: ParkEventVersion,
		RunID:         runID,
		Cause:         facts.cause,
		Budget:        facts.budget,
		Spent:         facts.spent,
		UnblockKnob:   facts.knob,
	})
}

// identicalFailureParkEventBody builds the canonical typed park event body
// for an identical-failure crossing.
func identicalFailureParkEventBody(
	runID string,
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
// park event of the shared kind for one cause. A same-kind event whose body
// cannot be parsed is treated as present (fail closed) so a park is never
// double-written behind an unreadable body.
func hasParkEventForCause(
	snapshot journal.Snapshot,
	cause string,
) bool {
	for _, event := range snapshot.Events {
		if event.Kind != ParkEventKind {
			continue
		}
		parsed, err := ParseDegradationParkEvent(event.Body)
		if err != nil {
			return true
		}
		if parsed.Cause == cause {
			return true
		}
	}
	return false
}
