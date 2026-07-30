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
	EvalSchemaVersion = "sworn.eval/v1"
	EvalObserver      = "eval.core"
	maxStableReads    = 2
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
}

type UsageSummary struct {
	InputTokens   *int64      `json:"input_tokens"`
	OutputTokens  *int64      `json:"output_tokens"`
	Costs         []CostTotal `json:"costs"`
	TokenCoverage Ratio       `json:"token_coverage"`
	CostCoverage  Ratio       `json:"cost_coverage"`
}

type AttemptGroup struct {
	Role           string       `json:"role"`
	Responsibility string       `json:"responsibility"`
	Operation      string       `json:"operation"`
	Transport      string       `json:"transport"`
	Outcome        string       `json:"outcome"`
	Attempts       int64        `json:"attempts"`
	Retries        int64        `json:"retries"`
	DurationNS     Ratio        `json:"duration_ns"`
	Usage          UsageSummary `json:"usage"`
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
}

type groupAggregate struct {
	attempts int64
	retries  int64
	duration int64
	usage    usageAggregate
}

type usageAggregate struct {
	inputKnown int64
	costKnown  int64
	input      int64
	output     int64
	costs      map[string]int64
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
	case journal.EvaluationAttempt:
		a.addAttempt(fact)
	default:
		a.err = fail("INVALID_EVALUATION_FACT")
	}
}

func (a *aggregate) addRecovery(kind string) {
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
	}
	group := a.groups[key]
	if group == nil {
		group = &groupAggregate{}
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
	if err := a.usage.add(usage); err != nil {
		a.err = err
		return
	}
	a.err = group.usage.add(usage)
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
		u.costs[*receipt.Currency], err = safeAdd(
			u.costs[*receipt.Currency],
			*receipt.CostMicroUnits,
		)
		return err
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
			Role:           key.role,
			Responsibility: key.responsibility,
			Operation:      key.operation,
			Transport:      key.transport,
			Outcome:        key.outcome,
			Attempts:       group.attempts,
			Retries:        group.retries,
			DurationNS:     knownRatio(group.duration, group.attempts),
			Usage:          group.usage.summary(group.attempts),
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
		return l.Outcome < r.Outcome
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
	}
	if u.inputKnown != 0 {
		result.InputTokens = int64Pointer(u.input)
		result.OutputTokens = int64Pointer(u.output)
	}
	if u.costKnown != 0 {
		result.Costs = make([]CostTotal, 0, len(u.costs))
		for currency, total := range u.costs {
			result.Costs = append(result.Costs, CostTotal{
				Currency:   currency,
				MicroUnits: total,
			})
		}
		sort.Slice(result.Costs, func(left, right int) bool {
			return result.Costs[left].Currency < result.Costs[right].Currency
		})
	}
	return result
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
