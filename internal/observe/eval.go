package observe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

const (
	EvalSchemaVersion = "sworn.eval/v3"
	EvalObserver      = "eval.core"
	maxStableReads    = 2
	// maxUnreportedSurfaces bounds the A3 naming list so a hostile or
	// pathological receipt population cannot inflate the record.
	maxUnreportedSurfaces = 64
)

type Error struct {
	Code string
}

func (e *Error) Error() string { return "observe: " + e.Code }

func IsCode(err error, code string) bool {
	var observeError *Error
	return errors.As(err, &observeError) && observeError.Code == code
}

func fail(code string) error { return &Error{Code: code} }

type EvaluationJournal interface {
	ObserverCursor(context.Context, string, string) (int64, error)
	VisitEvaluation(
		context.Context,
		string,
		func(journal.EvaluationFact),
	) (journal.EvaluationWindow, error)
	AdvanceObserver(context.Context, journal.ObserverAdvance) error
}

type SnapshotProjector interface {
	Snapshot(context.Context, string) (cockpit.Snapshot, error)
}

type Evaluator struct {
	journal EvaluationJournal
	cockpit SnapshotProjector
	version string
}

func NewEvaluator(
	journalStore EvaluationJournal,
	projector SnapshotProjector,
	version string,
) (*Evaluator, error) {
	if journalStore == nil || projector == nil || !validVersion(version) {
		return nil, fail("INVALID_EVALUATOR")
	}
	return &Evaluator{
		journal: journalStore,
		cockpit: projector,
		version: version,
	}, nil
}

type Ratio struct {
	Numerator   *int64 `json:"numerator"`
	Denominator *int64 `json:"denominator"`
}

type Quality struct {
	Name        string `json:"name"`
	Numerator   *int64 `json:"numerator"`
	Denominator *int64 `json:"denominator"`
}

type CostTotal struct {
	Currency   string `json:"currency"`
	MicroUnits int64  `json:"micro_units"`
	// Source distinguishes a provider-reported figure from any nominal
	// computed figure. Only typed provider reports are admitted.
	Source string `json:"source"`
}

type UsageSummary struct {
	InputTokens   *int64      `json:"input_tokens"`
	OutputTokens  *int64      `json:"output_tokens"`
	Costs         []CostTotal `json:"costs"`
	TokenCoverage Ratio       `json:"token_coverage"`
	CostCoverage  Ratio       `json:"cost_coverage"`
	// CacheReadTokens and CacheWriteTokens are the summed canonical cache
	// pair. A vocabulary that reports only reads (Gemini, the Responses API)
	// leaves CacheWriteTokens nil rather than turning absence into zero.
	CacheReadTokens  *int64 `json:"cache_read_tokens"`
	CacheWriteTokens *int64 `json:"cache_write_tokens"`
	CacheCoverage    Ratio  `json:"cache_coverage"`
	// ReasoningTokens is the additive optional reasoning side, omitempty and
	// strictly nil-absent so legacy eval records re-encode byte-identically.
	ReasoningTokens *int64  `json:"reasoning_tokens,omitempty"`
	EffortRequested *string `json:"effort_requested"`
	EffortReported  *string `json:"effort_reported"`
	FinishReason    *string `json:"finish_reason"`
	Truncated       *bool   `json:"truncated"`
	// UnreportedSurfaces names, sorted and de-duplicated, the surfaces whose
	// attempts could not report tokens. A legacy silent blob contributes the
	// honest literal "unknown". Coverage plus this naming ride in-band
	// wherever a total goes, so a partial sum is never rendered as a total.
	UnreportedSurfaces []string `json:"unreported_surfaces,omitempty"`
}

type AttemptGroup struct {
	Role           string `json:"role"`
	Responsibility string `json:"responsibility"`
	Operation      string `json:"operation"`
	Transport      string `json:"transport"`
	Outcome        string `json:"outcome"`
	Attempts       int64  `json:"attempts"`
	Retries        int64  `json:"retries"`
	// DurationNS is the event-derived duration kept for continuity;
	// ObservationDurationNS is the attempt-observed wall-clock duration
	// stamped on receipts at the runtime's single attempt-write seam.
	DurationNS            Ratio `json:"duration_ns"`
	ObservationDurationNS Ratio `json:"observation_duration_ns"`
	// Profile and Model are the profile and certified model id actually
	// dispatched, per the A4 identity facts; empty on legacy receipts that
	// predate the stamp.
	Profile string       `json:"profile"`
	Model   string       `json:"model"`
	Usage   UsageSummary `json:"usage"`
	// TurnEconomics is the A5 turn-economics aggregate: turns, tool calls,
	// tool calls per turn, and the per-name call mix, attributable per role.
	TurnEconomics TurnEconomics `json:"turn_economics"`
}

// TurnEconomics aggregates the engine-counted turn facts across the
// attempts of one group. Each ratio's denominator is the attempt count; a
// numerator sums only attempts that reported the fact.
type TurnEconomics struct {
	Turns            *int64          `json:"turns"`
	ToolCalls        *int64          `json:"tool_calls"`
	ToolCallsPerTurn Ratio           `json:"tool_calls_per_turn"`
	ToolCallMix      []ToolCallCount `json:"tool_call_mix,omitempty"`
}

type ToolCallCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type RecoverySummary struct {
	Uncertain  int64 `json:"uncertain"`
	Reconciled int64 `json:"reconciled"`
	Recovered  int64 `json:"recovered"`
	RolledBack int64 `json:"rolled_back"`
}

type Record struct {
	SchemaVersion string              `json:"schema_version"`
	ID            string              `json:"id"`
	SwornVersion  string              `json:"sworn_version"`
	RunID         string              `json:"run_id"`
	Release       string              `json:"release"`
	RunState      string              `json:"run_state"`
	Outcome       string              `json:"outcome"`
	ThroughOffset int64               `json:"through_offset"`
	StartedAt     time.Time           `json:"started_at"`
	ObservedAt    time.Time           `json:"observed_at"`
	ElapsedNS     int64               `json:"elapsed_ns"`
	Events        int64               `json:"events"`
	Attempts      int64               `json:"attempts"`
	Retries       int64               `json:"retries"`
	Recovery      RecoverySummary     `json:"recovery"`
	Continuation  ContinuationSummary `json:"continuation"`
	TurnRecovery  TurnRecoverySummary `json:"turn_recovery"`
	DurationNS    Ratio               `json:"duration_ns"`
	Usage         UsageSummary        `json:"usage"`
	Groups        []AttemptGroup      `json:"groups"`
	Quality       []Quality           `json:"quality"`
}

// Advance persists one cumulative, canonical evaluation record at a stable
// journal/cockpit high-water mark. It does not emit telemetry or alter runtime
// delivery. The bool is false when there is no new durable event.
func (e *Evaluator) Advance(
	ctx context.Context,
	runID string,
) (Record, bool, error) {
	if e == nil || ctx == nil || runID == "" {
		return Record{}, false, fail("INVALID_EVALUATION")
	}
	cursor, err := e.journal.ObserverCursor(ctx, runID, EvalObserver)
	if err != nil {
		return Record{}, false, fail("JOURNAL_UNAVAILABLE")
	}
	for read := 0; read < maxStableReads; read++ {
		aggregate := newAggregate()
		window, err := e.journal.VisitEvaluation(
			ctx,
			runID,
			func(fact journal.EvaluationFact) {
				if aggregate.err == nil {
					aggregate.add(fact)
				}
			},
		)
		if err != nil {
			return Record{}, false, fail("JOURNAL_UNAVAILABLE")
		}
		if aggregate.err != nil {
			return Record{}, false, aggregate.err
		}
		if window.ThroughOffset <= cursor {
			return Record{}, false, nil
		}
		snapshot, err := e.cockpit.Snapshot(ctx, runID)
		if err != nil {
			return Record{}, false, fail("COCKPIT_UNAVAILABLE")
		}
		if snapshot.ThroughOffset != window.ThroughOffset {
			continue
		}
		record, err := aggregate.record(
			e.version,
			window,
			snapshot,
		)
		if err != nil {
			return Record{}, false, err
		}
		body, err := json.Marshal(record)
		if err != nil || len(body) > journal.MaxObserverBodyBytes {
			return Record{}, false, fail("EVAL_RECORD_LIMIT")
		}
		if err := e.journal.AdvanceObserver(ctx, journal.ObserverAdvance{
			RunID:          runID,
			Observer:       EvalObserver,
			ExpectedOffset: cursor,
			ThroughOffset:  window.ThroughOffset,
			Eval: []journal.EvalDraft{{
				SourceEventOffset: window.ThroughOffset,
				ID:                record.ID,
				Body:              body,
			}},
			At: window.ObservedAt,
		}); err != nil {
			return Record{}, false, fail("JOURNAL_UNAVAILABLE")
		}
		return record, true, nil
	}
	return Record{}, false, fail("SNAPSHOT_UNSTABLE")
}

type aggregate struct {
	events       int64
	attempts     int64
	retries      int64
	recovery     RecoverySummary
	continuation continuationAggregate
	turnRecovery turnRecoveryAggregate
	duration     int64
	usage        usageAggregate
	groups       map[groupKey]*groupAggregate
	err          error
}

type groupKey struct {
	role           string
	responsibility string
	operation      string
	transport      string
	outcome        string
	profile        string
	model          string
}

type groupAggregate struct {
	attempts int64
	retries  int64
	duration int64
	usage    usageAggregate
	// observationDuration sums the receipt-stamped wall-clock durations
	// (milliseconds) of attempts that reported them.
	observationDuration int64
	// turns/toolCalls/toolCallMix aggregate the A5 turn economics.
	turns          int64
	turnsKnown     int64
	toolCalls      int64
	toolCallsKnown int64
	toolCallMix    map[string]int64
}

type usageAggregate struct {
	inputKnown int64
	costKnown  int64
	input      int64
	output     int64
	costs      map[string]int64

	cacheKnown      int64
	cacheReadKnown  int64
	cacheWriteKnown int64
	cacheRead       int64
	cacheWrite      int64
	reasoningKnown  int64
	reasoning       int64
	effortRequested *string
	effortReported  *string
	finishReason    *string
	truncated       *bool
	// unreported names, per surface, the attempts whose receipts could not
	// report tokens.
	unreported map[string]int64
}

func newAggregate() *aggregate {
	return &aggregate{groups: make(map[groupKey]*groupAggregate)}
}

func (a *aggregate) add(fact journal.EvaluationFact) {
	switch fact.Kind {
	case journal.EvaluationEvent:
		a.events, a.err = safeAdd(a.events, 1)
		if a.err == nil {
			a.addRecovery(fact.EventKind)
		}
		if a.err == nil {
			a.err = a.continuation.add(fact.EventKind)
		}
		if a.err == nil {
			a.err = a.turnRecovery.add(fact.EventKind)
		}
	case journal.EvaluationAttempt:
		a.addAttempt(fact)
	default:
		a.err = fail("INVALID_EVALUATION_FACT")
	}
}

func (a *aggregate) addRecovery(kind string) {
	if strings.HasPrefix(kind, "turn_recovery.") {
		return
	}
	var target *int64
	switch {
	case strings.Contains(kind, "uncertain"):
		target = &a.recovery.Uncertain
	case strings.Contains(kind, "reconciled"):
		target = &a.recovery.Reconciled
	case strings.Contains(kind, "recovered"):
		target = &a.recovery.Recovered
	case strings.Contains(kind, "rolled_back"):
		target = &a.recovery.RolledBack
	default:
		return
	}
	*target, a.err = safeAdd(*target, 1)
}

func (a *aggregate) addAttempt(fact journal.EvaluationFact) {
	if fact.Attempt < 1 {
		a.err = fail("INVALID_EVALUATION_FACT")
		return
	}
	duration := fact.FinishedAt.Sub(fact.StartedAt).Nanoseconds()
	if duration < 0 {
		a.err = fail("INVALID_EVALUATION_FACT")
		return
	}
	usage, err := decodeUsage(fact.Usage)
	if err != nil {
		a.err = err
		return
	}
	key := groupKey{
		role:           roleForResponsibility(fact.Responsibility),
		responsibility: boundedResponsibility(fact.Responsibility),
		operation:      boundedOperation(fact.EffectKind),
		transport:      boundedTransport(fact.Transport),
		outcome:        boundedEffectState(fact.EffectState),
		profile:        usageProfile(usage),
		model:          usageModel(usage),
	}
	group := a.groups[key]
	if group == nil {
		group = &groupAggregate{
			toolCallMix: make(map[string]int64),
		}
		a.groups[key] = group
	}
	retry := int64(0)
	if fact.Attempt > 1 {
		retry = 1
	}
	for _, target := range []*int64{
		&a.attempts,
		&group.attempts,
	} {
		*target, a.err = safeAdd(*target, 1)
		if a.err != nil {
			return
		}
	}
	for _, target := range []*int64{&a.retries, &group.retries} {
		*target, a.err = safeAdd(*target, retry)
		if a.err != nil {
			return
		}
	}
	for _, target := range []*int64{&a.duration, &group.duration} {
		*target, a.err = safeAdd(*target, duration)
		if a.err != nil {
			return
		}
	}
	if usage.DurationMillis != nil {
		group.observationDuration, a.err = safeAdd(
			group.observationDuration,
			*usage.DurationMillis*1_000_000,
		)
		if a.err != nil {
			return
		}
	}
	if usage.Turns != nil {
		group.turns, a.err = safeAdd(group.turns, *usage.Turns)
		if a.err != nil {
			return
		}
		group.turnsKnown, a.err = safeAdd(group.turnsKnown, 1)
		if a.err != nil {
			return
		}
	}
	if usage.ToolCalls != nil {
		group.toolCalls, a.err = safeAdd(group.toolCalls, *usage.ToolCalls)
		if a.err != nil {
			return
		}
		group.toolCallsKnown, a.err = safeAdd(group.toolCallsKnown, 1)
		if a.err != nil {
			return
		}
	}
	for _, item := range usage.ToolCallsByName {
		group.toolCallMix[item.Name], a.err = safeAdd(
			group.toolCallMix[item.Name],
			item.Count,
		)
		if a.err != nil {
			return
		}
	}
	if err := a.usage.add(usage); err != nil {
		a.err = err
		return
	}
	a.err = group.usage.add(usage)
}

func usageProfile(receipt driver.UsageReceipt) string {
	if receipt.Profile == nil {
		return ""
	}
	return *receipt.Profile
}

func usageModel(receipt driver.UsageReceipt) string {
	if receipt.Model == nil {
		return ""
	}
	return *receipt.Model
}

// turnEconomics aggregates the A5 facts of one group. Turns and tool calls
// are summed over the attempts that reported them (nil is honest absence);
// tool_calls_per_turn rides as a ratio over the summed turns, and the mix is
// the sorted per-name total over the closed tool vocabulary.
func (g *groupAggregate) turnEconomics() TurnEconomics {
	result := TurnEconomics{
		ToolCallsPerTurn: knownRatio(g.toolCalls, g.turns),
	}
	if g.turnsKnown != 0 {
		result.Turns = int64Pointer(g.turns)
	}
	if g.toolCallsKnown != 0 {
		result.ToolCalls = int64Pointer(g.toolCalls)
	}
	if len(g.toolCallMix) != 0 {
		mix := make([]ToolCallCount, 0, len(g.toolCallMix))
		for name, count := range g.toolCallMix {
			if count > 0 {
				mix = append(mix, ToolCallCount{Name: name, Count: count})
			}
		}
		sort.Slice(mix, func(left, right int) bool {
			return mix[left].Name < mix[right].Name
		})
		if len(mix) != 0 {
			result.ToolCallMix = mix
		}
	}
	return result
}

func (u *usageAggregate) add(receipt driver.UsageReceipt) error {
	if receipt.TokenStatus == driver.UsageReported {
		var err error
		u.inputKnown, err = safeAdd(u.inputKnown, 1)
		if err != nil {
			return err
		}
		u.input, err = safeAdd(u.input, *receipt.InputTokens)
		if err != nil {
			return err
		}
		u.output, err = safeAdd(u.output, *receipt.OutputTokens)
		if err != nil {
			return err
		}
	} else {
		// A3: name the non-reporting surface. A loud v2 receipt supplies the
		// adapter id; a legacy silent blob contributes the honest literal
		// "unknown".
		var err error
		surface := receipt.Surface
		if surface == "" {
			surface = "unknown"
		}
		if u.unreported == nil {
			u.unreported = make(map[string]int64)
		}
		u.unreported[surface], err = safeAdd(u.unreported[surface], 1)
		if err != nil {
			return err
		}
	}
	if receipt.CostStatus == driver.UsageReported {
		var err error
		u.costKnown, err = safeAdd(u.costKnown, 1)
		if err != nil {
			return err
		}
		if u.costs == nil {
			u.costs = make(map[string]int64)
		}
		// The currency+source pair is the cost identity; the source keeps a
		// provider-reported figure distinguished from any nominal computed
		// figure.
		u.costs[*receipt.Currency+"\x00"+*receipt.Source], err = safeAdd(
			u.costs[*receipt.Currency+"\x00"+*receipt.Source],
			*receipt.CostMicroUnits,
		)
		if err != nil {
			return err
		}
	}
	if receipt.CacheStatus == driver.UsageReported {
		var err error
		u.cacheKnown, err = safeAdd(u.cacheKnown, 1)
		if err != nil {
			return err
		}
		if receipt.CacheReadTokens != nil {
			u.cacheReadKnown, err = safeAdd(u.cacheReadKnown, 1)
			if err != nil {
				return err
			}
			u.cacheRead, err = safeAdd(u.cacheRead, *receipt.CacheReadTokens)
			if err != nil {
				return err
			}
		}
		if receipt.CacheWriteTokens != nil {
			u.cacheWriteKnown, err = safeAdd(u.cacheWriteKnown, 1)
			if err != nil {
				return err
			}
			u.cacheWrite, err = safeAdd(u.cacheWrite, *receipt.CacheWriteTokens)
			if err != nil {
				return err
			}
		}
	}
	if receipt.ReasoningTokens != nil {
		var err error
		u.reasoningKnown, err = safeAdd(u.reasoningKnown, 1)
		if err != nil {
			return err
		}
		u.reasoning, err = safeAdd(u.reasoning, *receipt.ReasoningTokens)
		if err != nil {
			return err
		}
	}
	// Group-level aggregation is deterministic last-reported-wins: attempts
	// arrive in journal order, so the last non-nil value is the most recent
	// reported fact. Truncation is carried as any-non-nil (the only non-nil
	// value in practice is true).
	if receipt.EffortRequested != nil {
		value := *receipt.EffortRequested
		u.effortRequested = &value
	}
	if receipt.EffortReported != nil {
		value := *receipt.EffortReported
		u.effortReported = &value
	}
	if receipt.FinishReason != nil {
		value := *receipt.FinishReason
		u.finishReason = &value
	}
	if receipt.Truncated != nil {
		value := *receipt.Truncated
		u.truncated = &value
	}
	return nil
}

func (a *aggregate) record(
	version string,
	window journal.EvaluationWindow,
	snapshot cockpit.Snapshot,
) (Record, error) {
	elapsed := window.ObservedAt.Sub(window.Run.CreatedAt).Nanoseconds()
	if elapsed < 0 {
		return Record{}, fail("INVALID_EVALUATION_FACT")
	}
	groups := make([]AttemptGroup, 0, len(a.groups))
	for key, group := range a.groups {
		groups = append(groups, AttemptGroup{
			Role:                  key.role,
			Responsibility:        key.responsibility,
			Operation:             key.operation,
			Transport:             key.transport,
			Outcome:               key.outcome,
			Attempts:              group.attempts,
			Retries:               group.retries,
			DurationNS:            knownRatio(group.duration, group.attempts),
			ObservationDurationNS: knownRatio(group.observationDuration, group.attempts),
			Profile:               key.profile,
			Model:                 key.model,
			Usage:                 group.usage.summary(group.attempts),
			TurnEconomics:         group.turnEconomics(),
		})
	}
	sort.Slice(groups, func(left, right int) bool {
		l, r := groups[left], groups[right]
		if l.Role != r.Role {
			return l.Role < r.Role
		}
		if l.Responsibility != r.Responsibility {
			return l.Responsibility < r.Responsibility
		}
		if l.Operation != r.Operation {
			return l.Operation < r.Operation
		}
		if l.Transport != r.Transport {
			return l.Transport < r.Transport
		}
		if l.Outcome != r.Outcome {
			return l.Outcome < r.Outcome
		}
		if l.Profile != r.Profile {
			return l.Profile < r.Profile
		}
		return l.Model < r.Model
	})
	if snapshot.Run.ID != window.Run.ID ||
		snapshot.Run.Release != window.Run.Release {
		return Record{}, fail("SNAPSHOT_MISMATCH")
	}
	return Record{
		SchemaVersion: EvalSchemaVersion,
		ID:            recordID(window.Run.ID, window.ThroughOffset),
		SwornVersion:  version,
		RunID:         window.Run.ID,
		Release:       window.Run.Release,
		RunState:      boundedRunState(snapshot.Run.State),
		Outcome:       boundedRunOutcome(snapshot.Run.Outcome),
		ThroughOffset: window.ThroughOffset,
		StartedAt:     window.Run.CreatedAt,
		ObservedAt:    window.ObservedAt,
		ElapsedNS:     elapsed,
		Events:        a.events,
		Attempts:      a.attempts,
		Retries:       a.retries,
		Recovery:      a.recovery,
		Continuation:  a.continuation.summary(),
		TurnRecovery:  a.turnRecovery.summary(),
		DurationNS:    knownRatio(a.duration, a.attempts),
		Usage:         a.usage.summary(a.attempts),
		Groups:        groups,
		Quality:       graphQuality(snapshot.Graph),
	}, nil
}

func (u usageAggregate) summary(denominator int64) UsageSummary {
	result := UsageSummary{
		TokenCoverage: knownRatio(u.inputKnown, denominator),
		CostCoverage:  knownRatio(u.costKnown, denominator),
		CacheCoverage: knownRatio(u.cacheKnown, denominator),
	}
	if u.inputKnown != 0 {
		result.InputTokens = int64Pointer(u.input)
		result.OutputTokens = int64Pointer(u.output)
	}
	if u.costKnown != 0 {
		result.Costs = make([]CostTotal, 0, len(u.costs))
		for key, total := range u.costs {
			currency, source := splitCostKey(key)
			result.Costs = append(result.Costs, CostTotal{
				Currency:   currency,
				MicroUnits: total,
				Source:     source,
			})
		}
		sort.Slice(result.Costs, func(left, right int) bool {
			if result.Costs[left].Currency != result.Costs[right].Currency {
				return result.Costs[left].Currency <
					result.Costs[right].Currency
			}
			return result.Costs[left].Source <
				result.Costs[right].Source
		})
	}
	if u.cacheKnown != 0 {
		if u.cacheReadKnown != 0 {
			result.CacheReadTokens = int64Pointer(u.cacheRead)
		}
		if u.cacheWriteKnown != 0 {
			result.CacheWriteTokens = int64Pointer(u.cacheWrite)
		}
	}
	if u.reasoningKnown != 0 {
		result.ReasoningTokens = int64Pointer(u.reasoning)
	}
	if u.effortRequested != nil {
		value := *u.effortRequested
		result.EffortRequested = &value
	}
	if u.effortReported != nil {
		value := *u.effortReported
		result.EffortReported = &value
	}
	if u.finishReason != nil {
		value := *u.finishReason
		result.FinishReason = &value
	}
	if u.truncated != nil {
		value := *u.truncated
		result.Truncated = &value
	}
	if len(u.unreported) != 0 {
		surfaces := make([]string, 0, len(u.unreported))
		for surface := range u.unreported {
			surfaces = append(surfaces, surface)
		}
		sort.Strings(surfaces)
		if len(surfaces) > maxUnreportedSurfaces {
			surfaces = surfaces[:maxUnreportedSurfaces]
		}
		result.UnreportedSurfaces = surfaces
	}
	return result
}

func splitCostKey(key string) (string, string) {
	if index := strings.IndexByte(key, '\x00'); index >= 0 {
		return key[:index], key[index+1:]
	}
	return key, driver.CostSourceProviderReported
}

func decodeUsage(body []byte) (driver.UsageReceipt, error) {
	var result driver.UsageReceipt
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return driver.UsageReceipt{}, fail("INVALID_USAGE")
	}
	if decoder.Decode(&struct{}{}) == nil {
		return driver.UsageReceipt{}, fail("INVALID_USAGE")
	}
	canonical, err := driver.EncodeUsageReceipt(result)
	if err != nil || !bytes.Equal(canonical, body) {
		return driver.UsageReceipt{}, fail("INVALID_USAGE")
	}
	return result, nil
}

func knownRatio(numerator, denominator int64) Ratio {
	return Ratio{
		Numerator:   int64Pointer(numerator),
		Denominator: int64Pointer(denominator),
	}
}

func int64Pointer(value int64) *int64 { return &value }

func safeAdd(left, right int64) (int64, error) {
	if right < 0 || left > int64(^uint64(0)>>1)-right {
		return 0, fail("AGGREGATE_OVERFLOW")
	}
	return left + right, nil
}

func recordID(runID string, through int64) string {
	body, _ := json.Marshal(struct {
		Schema  string `json:"schema"`
		Run     string `json:"run"`
		Through int64  `json:"through"`
	}{EvalSchemaVersion, runID, through})
	sum := sha256.Sum256(body)
	return "eval-" + hex.EncodeToString(sum[:])
}

func validVersion(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if !((character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune(".+-", character)) {
			return false
		}
	}
	return true
}

func boundedResponsibility(value string) string {
	switch driver.Responsibility(value) {
	case driver.PlannerProposal, driver.ImplementerDesign,
		driver.ImplementerImplementation, driver.CaptainReview,
		driver.WorkVerification, driver.AssemblyVerification:
		return value
	default:
		return "other"
	}
}

func roleForResponsibility(value string) string {
	switch driver.Responsibility(value) {
	case driver.PlannerProposal:
		return string(driver.RolePlanner)
	case driver.ImplementerDesign, driver.ImplementerImplementation:
		return string(driver.RoleImplementer)
	case driver.CaptainReview:
		return string(driver.RoleCaptain)
	case driver.WorkVerification, driver.AssemblyVerification:
		return string(driver.RoleVerifier)
	default:
		return "other"
	}
}

func boundedOperation(value string) string {
	if value == "driver.dispatch" {
		return value
	}
	return "other"
}

func boundedTransport(value string) string {
	switch driver.TransportStatus(value) {
	case driver.Completed, driver.TransportError, driver.TimedOut,
		driver.Cancelled, driver.RunnerError:
		return value
	default:
		return "other"
	}
}

func boundedEffectState(value journal.EffectState) string {
	switch value {
	case journal.Pending, journal.Claimed, journal.Succeeded,
		journal.OperationalFailed, journal.Uncertain:
		return string(value)
	default:
		return "other"
	}
}

func boundedRunState(value string) string {
	switch value {
	case "new", "running", "pausing", "paused", "cancelling", "cancelled",
		"parked", "uncertain", "takeover_required", "awaiting_approval",
		"complete":
		return value
	default:
		return "unknown"
	}
}

func boundedRunOutcome(value string) string {
	switch value {
	case "", "pending", "proceed", "pass", "fail", "blocked", "merged":
		if value == "" {
			return "unknown"
		}
		return value
	default:
		return "unknown"
	}
}

func graphQuality(graph cockpit.Graph) []Quality {
	var slices, passedSlices, terminalSlices int64
	var assemblyNumerator, assemblyDenominator int64
	for _, node := range graph.Nodes {
		switch node.Kind {
		case "slice":
			slices++
			switch node.Outcome {
			case "pass":
				passedSlices++
				terminalSlices++
			case "fail", "blocked":
				terminalSlices++
			}
		case "assembly":
			switch node.Outcome {
			case "pass", "merged":
				assemblyNumerator = 1
				assemblyDenominator = 1
			case "fail", "blocked":
				assemblyDenominator = 1
			}
		}
	}
	return []Quality{
		{
			Name:        "delivery",
			Numerator:   int64Pointer(passedSlices),
			Denominator: int64Pointer(slices),
		},
		{
			Name:        "integration",
			Numerator:   int64Pointer(assemblyNumerator),
			Denominator: int64Pointer(assemblyDenominator),
		},
		{
			// No exact requirements oracle is present in the runtime journal.
			Name: "requirements",
		},
		{
			Name:        "verification",
			Numerator:   int64Pointer(passedSlices),
			Denominator: int64Pointer(terminalSlices),
		},
	}
}
