package journal

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// Recovery budgets are runaway guards at turn-budget scale, not
	// per-type allowances: a weak or local model may need many nudges and
	// corrections to complete, and each reserved step is durable eval data
	// (nudges-per-completion is a model quality metric, not a failure).
	// The invocation turn budget and timeout are the real bounds.
	MaxRecoveryCorrectionsPerTurn   = int64(1_000)
	MaxRecoveryNudgesPerTurn        = int64(1_000)
	MaxRecoveryAdvisoriesPerCycle   = int64(100)
	MaxRecoveryDecisionsPerProgress = int64(100)
	MaxRecoveryAutomaticPerCycle    = int64(10_000)

	RecoveryStepReservedEvent     = "turn_recovery_step_reserved"
	RecoveryResumeWorkerEvent     = "turn_recovery.action.resume_worker"
	RecoveryAskCaptainEvent       = "turn_recovery.action.ask_captain"
	RecoveryRetryOperationalEvent = "turn_recovery.action.retry_operationally"
	RecoveryParkedEvent           = "turn_recovery.action.pause_track_for_human"

	recoveryStepVersion  = "sworn.turn-recovery-step/v1"
	recoveryMaximumValue = int64(9_007_199_254_740_991)
)

// RecoveryBinding deliberately has no content, dispatch try, retry epoch, or
// process identity. Callers must retain these stable IDs across those changes.
type RecoveryBinding struct {
	LaneID     string `json:"lane_id"`
	CycleID    string `json:"cycle_id"`
	TurnID     string `json:"turn_id"`
	ProgressID string `json:"progress_id"`
}

type RecoveryStepKind string

const (
	RecoveryMalformedCorrection RecoveryStepKind = "malformed_correction"
	RecoveryProseNudge          RecoveryStepKind = "prose_nudge"
	RecoveryResumeWorker        RecoveryStepKind = "resume_worker"
	RecoveryAskCaptain          RecoveryStepKind = "ask_captain"
	RecoveryRetryOperationally  RecoveryStepKind = "retry_operationally"
	RecoveryParkTrack           RecoveryStepKind = "park_track"
)

// RecoveryAccounting is a cumulative, content-free cost snapshot. It rides
// the existing replay-keyed recovery step so a human park does not lose work
// already paid for.
type RecoveryAccounting struct {
	DurationMillis int64   `json:"duration_ms"`
	TokenStatus    string  `json:"token_status"`
	InputTokens    *int64  `json:"input_tokens"`
	OutputTokens   *int64  `json:"output_tokens"`
	CostStatus     string  `json:"cost_status"`
	CostMicroUnits *int64  `json:"cost_micro_units"`
	Currency       *string `json:"currency"`
	Source         *string `json:"source"`
}

type RecoveryStepCommand struct {
	RunID      string              `json:"run_id"`
	ID         string              `json:"step_id"`
	Binding    RecoveryBinding     `json:"binding"`
	Ordinal    int64               `json:"ordinal"`
	Kind       RecoveryStepKind    `json:"kind"`
	Accounting *RecoveryAccounting `json:"accounting,omitempty"`
}

type RecoveryStepReceipt struct {
	Step             RecoveryStepCommand `json:"step"`
	AutomaticActions int64               `json:"automatic_actions"`
	Corrections      int64               `json:"corrections"`
	Nudges           int64               `json:"nudges"`
	Advisories       int64               `json:"advisories"`
	SameProgress     int64               `json:"same_progress"`
	Parked           bool                `json:"parked"`
}

type RecoveryBudgetProjection struct {
	Binding          RecoveryBinding     `json:"binding"`
	NextOrdinal      int64               `json:"next_ordinal"`
	AutomaticActions int64               `json:"automatic_actions"`
	Corrections      int64               `json:"corrections"`
	Nudges           int64               `json:"nudges"`
	Advisories       int64               `json:"advisories"`
	SameProgress     int64               `json:"same_progress"`
	Parked           bool                `json:"parked"`
	LastStepID       string              `json:"last_step_id,omitempty"`
	Accounting       *RecoveryAccounting `json:"accounting,omitempty"`
}

type recoveryStepRecord struct {
	SchemaVersion string              `json:"schema_version"`
	Step          RecoveryStepCommand `json:"step"`
}

func validateRecoveryBinding(value RecoveryBinding) error {
	if err := validateIdentity(value.LaneID, "lane"); err != nil {
		return err
	}
	if validateDigest(value.CycleID) != nil ||
		validateDigest(value.TurnID) != nil ||
		validateDigest(value.ProgressID) != nil {
		return fail("INVALID_RECOVERY_BINDING", nil)
	}
	return nil
}

// RecoveryStepID excludes the current turn, progress, action, content,
// dispatch try, and retry epoch. The cycle ordinal therefore has one stable
// replay identity across restarts and operational retries.
func RecoveryStepID(binding RecoveryBinding, ordinal int64) string {
	body, _ := json.Marshal(struct {
		SchemaVersion string `json:"schema_version"`
		LaneID        string `json:"lane_id"`
		CycleID       string `json:"cycle_id"`
		Ordinal       int64  `json:"ordinal"`
	}{
		SchemaVersion: "sworn.turn-recovery-step-identity/v1",
		LaneID:        binding.LaneID,
		CycleID:       binding.CycleID,
		Ordinal:       ordinal,
	})
	return digest(body)
}

func validRecoveryKind(value RecoveryStepKind) bool {
	switch value {
	case RecoveryMalformedCorrection,
		RecoveryProseNudge,
		RecoveryResumeWorker,
		RecoveryAskCaptain,
		RecoveryRetryOperationally,
		RecoveryParkTrack:
		return true
	default:
		return false
	}
}

func recoveryAutomatic(value RecoveryStepKind) bool {
	return value != RecoveryParkTrack && validRecoveryKind(value)
}

func recoveryDecision(value RecoveryStepKind) bool {
	switch value {
	case RecoveryResumeWorker, RecoveryAskCaptain,
		RecoveryRetryOperationally:
		return true
	default:
		return false
	}
}

func validateRecoveryStep(value RecoveryStepCommand) error {
	if err := validateIdentity(value.RunID, "run"); err != nil {
		return err
	}
	if err := validateRecoveryBinding(value.Binding); err != nil {
		return err
	}
	if value.Ordinal < 1 ||
		validateDigest(value.ID) != nil ||
		value.ID != RecoveryStepID(value.Binding, value.Ordinal) ||
		!validRecoveryKind(value.Kind) {
		return fail("INVALID_RECOVERY_STEP", nil)
	}
	if value.Accounting != nil &&
		!validRecoveryAccounting(*value.Accounting) {
		return fail("INVALID_RECOVERY_STEP", nil)
	}
	return nil
}

func validRecoveryAccounting(value RecoveryAccounting) bool {
	if value.DurationMillis < 0 ||
		value.DurationMillis > recoveryMaximumValue {
		return false
	}
	switch value.TokenStatus {
	case "reported":
		if value.InputTokens == nil || value.OutputTokens == nil ||
			*value.InputTokens < 0 ||
			*value.InputTokens > recoveryMaximumValue ||
			*value.OutputTokens < 0 ||
			*value.OutputTokens > recoveryMaximumValue {
			return false
		}
	case "unavailable":
		if value.InputTokens != nil || value.OutputTokens != nil {
			return false
		}
	default:
		return false
	}
	switch value.CostStatus {
	case "reported":
		if value.CostMicroUnits == nil ||
			value.Currency == nil ||
			value.Source == nil ||
			*value.CostMicroUnits < 0 ||
			*value.CostMicroUnits > recoveryMaximumValue ||
			len(*value.Currency) != 3 ||
			*value.Source != "provider_reported" {
			return false
		}
		for _, character := range []byte(*value.Currency) {
			if character < 'A' || character > 'Z' {
				return false
			}
		}
	case "unavailable":
		if value.CostMicroUnits != nil ||
			value.Currency != nil ||
			value.Source != nil {
			return false
		}
	default:
		return false
	}
	return true
}

func cloneRecoveryAccounting(
	value *RecoveryAccounting,
) *RecoveryAccounting {
	if value == nil {
		return nil
	}
	result := *value
	if value.InputTokens != nil {
		input, output := *value.InputTokens, *value.OutputTokens
		result.InputTokens = &input
		result.OutputTokens = &output
	}
	if value.CostMicroUnits != nil {
		cost := *value.CostMicroUnits
		currency, source := *value.Currency, *value.Source
		result.CostMicroUnits = &cost
		result.Currency = &currency
		result.Source = &source
	}
	return &result
}

func marshalRecoveryStep(value recoveryStepRecord) []byte {
	body, _ := json.Marshal(value)
	return append(body, '\n')
}

func parseRecoveryStep(
	runID, commandKind string,
	body []byte,
) (recoveryStepRecord, error) {
	var value recoveryStepRecord
	if json.Unmarshal(body, &value) != nil ||
		value.SchemaVersion != recoveryStepVersion ||
		value.Step.RunID != runID ||
		commandKind != "turn_recovery.step" ||
		!bytes.Equal(body, marshalRecoveryStep(value)) {
		return recoveryStepRecord{}, fail("CORRUPT_JOURNAL", nil)
	}
	if err := validateRecoveryStep(value.Step); err != nil {
		return recoveryStepRecord{}, fail("CORRUPT_JOURNAL", err)
	}
	return value, nil
}

func recoveryRecordsOnConnection(
	ctx context.Context,
	conn *sql.Conn,
	runID string,
) ([]recoveryStepRecord, error) {
	rows, err := conn.QueryContext(
		ctx,
		`SELECT kind,payload FROM commands
		  WHERE run_id=? AND kind='turn_recovery.step'`,
		runID,
	)
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()
	var result []recoveryStepRecord
	for rows.Next() {
		var kind string
		var body []byte
		if err := rows.Scan(&kind, &body); err != nil {
			return nil, dbError(err)
		}
		value, err := parseRecoveryStep(runID, kind, body)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, dbError(err)
	}
	return result, nil
}

func recoveryBudgetOnConnection(
	ctx context.Context,
	conn *sql.Conn,
	runID string,
	binding RecoveryBinding,
) (RecoveryBudgetProjection, error) {
	records, err := recoveryRecordsOnConnection(ctx, conn, runID)
	if err != nil {
		return RecoveryBudgetProjection{}, err
	}
	cycle := make([]RecoveryStepCommand, 0, len(records))
	for _, record := range records {
		if record.Step.Binding.CycleID == binding.CycleID {
			cycle = append(cycle, record.Step)
		}
	}
	var persistedLane string
	for _, step := range cycle {
		if persistedLane == "" {
			persistedLane = step.Binding.LaneID
		} else if step.Binding.LaneID != persistedLane {
			return RecoveryBudgetProjection{}, fail("CORRUPT_JOURNAL", nil)
		}
	}
	if persistedLane != "" && persistedLane != binding.LaneID {
		return RecoveryBudgetProjection{},
			fail("RECOVERY_BINDING_CONFLICT", nil)
	}
	sort.Slice(cycle, func(left, right int) bool {
		return cycle[left].Ordinal < cycle[right].Ordinal
	})
	result := RecoveryBudgetProjection{
		Binding:     binding,
		NextOrdinal: 1,
	}
	for _, step := range cycle {
		if step.Ordinal != result.NextOrdinal ||
			(result.Parked && step.Kind != RecoveryResumeWorker) {
			return RecoveryBudgetProjection{}, fail("CORRUPT_JOURNAL", nil)
		}
		humanResume := result.Parked &&
			step.Kind == RecoveryResumeWorker
		if humanResume {
			result.Parked = false
		}
		result.NextOrdinal++
		result.LastStepID = step.ID
		if recoveryAutomatic(step.Kind) && !humanResume {
			result.AutomaticActions++
		}
		switch step.Kind {
		case RecoveryMalformedCorrection:
			if step.Binding.TurnID == binding.TurnID {
				result.Corrections++
			}
		case RecoveryProseNudge:
			if step.Binding.TurnID == binding.TurnID {
				result.Nudges++
			}
		case RecoveryAskCaptain:
			result.Advisories++
		case RecoveryParkTrack:
			result.Parked = true
		}
		if recoveryDecision(step.Kind) && !humanResume &&
			step.Binding.ProgressID == binding.ProgressID {
			result.SameProgress++
		}
		if step.Accounting != nil {
			result.Accounting = cloneRecoveryAccounting(
				step.Accounting,
			)
		}
	}
	return result, nil
}

func recoveryReplayKey(stepID string) string {
	return "turn-recovery/" + strings.TrimPrefix(stepID, "sha256:")
}

func applyRecoveryCount(
	projection *RecoveryBudgetProjection,
	kind RecoveryStepKind,
) {
	humanResume := projection.Parked && kind == RecoveryResumeWorker
	if humanResume {
		projection.Parked = false
	}
	if recoveryAutomatic(kind) && !humanResume {
		projection.AutomaticActions++
	}
	switch kind {
	case RecoveryMalformedCorrection:
		projection.Corrections++
	case RecoveryProseNudge:
		projection.Nudges++
	case RecoveryAskCaptain:
		projection.Advisories++
	case RecoveryParkTrack:
		projection.Parked = true
	}
	if recoveryDecision(kind) && !humanResume {
		projection.SameProgress++
	}
}

func recoveryBudgetAllows(
	projection RecoveryBudgetProjection,
	kind RecoveryStepKind,
) error {
	if projection.Parked {
		if kind == RecoveryResumeWorker {
			// A durable human answer may restart a parked lane. Admission of
			// that exact answered attention happens in the same transaction.
			return nil
		}
		return fail("RECOVERY_PARKED", nil)
	}
	if kind == RecoveryParkTrack {
		return nil
	}
	if projection.AutomaticActions >= MaxRecoveryAutomaticPerCycle {
		return fail("RECOVERY_BUDGET_EXHAUSTED", nil)
	}
	switch kind {
	case RecoveryMalformedCorrection:
		if projection.Corrections >= MaxRecoveryCorrectionsPerTurn {
			return fail("RECOVERY_BUDGET_EXHAUSTED", nil)
		}
	case RecoveryProseNudge:
		if projection.Nudges >= MaxRecoveryNudgesPerTurn {
			return fail("RECOVERY_BUDGET_EXHAUSTED", nil)
		}
	case RecoveryAskCaptain:
		if projection.Advisories >= MaxRecoveryAdvisoriesPerCycle {
			return fail("RECOVERY_BUDGET_EXHAUSTED", nil)
		}
		fallthrough
	case RecoveryResumeWorker, RecoveryRetryOperationally:
		if projection.SameProgress >= MaxRecoveryDecisionsPerProgress {
			return fail("RECOVERY_BUDGET_EXHAUSTED", nil)
		}
	default:
		return fail("INVALID_RECOVERY_STEP", nil)
	}
	return nil
}

func reserveRecoveryStepOnConnection(
	ctx context.Context,
	conn *sql.Conn,
	owner OwnerLease,
	command RecoveryStepCommand,
	at time.Time,
	atValue string,
) (RecoveryStepReceipt, error) {
	record := recoveryStepRecord{
		SchemaVersion: recoveryStepVersion,
		Step:          command,
	}
	body := marshalRecoveryStep(record)
	replayKey := recoveryReplayKey(command.ID)
	var receipt RecoveryStepReceipt
	result, replayed, err := replayedTransition(
		ctx,
		conn,
		command.RunID,
		replayKey,
		"turn_recovery.step",
		"runtime.turn_recovery",
		body,
	)
	if err != nil {
		return RecoveryStepReceipt{}, err
	}
	if replayed {
		if json.Unmarshal(result, &receipt) != nil ||
			!bytes.Equal(
				marshalRecoveryStep(recoveryStepRecord{
					SchemaVersion: recoveryStepVersion,
					Step:          receipt.Step,
				}),
				body,
			) {
			return RecoveryStepReceipt{}, fail("CORRUPT_JOURNAL", nil)
		}
		return receipt, nil
	}
	if err := checkOwner(ctx, conn, owner, at); err != nil {
		return RecoveryStepReceipt{}, err
	}
	control, err := projectionOnConnection(ctx, conn, command.RunID)
	if err != nil {
		return RecoveryStepReceipt{}, err
	}
	if control.Desired != "running" {
		return RecoveryStepReceipt{}, fail("CONTROL_STOPPED", nil)
	}
	projection, err := recoveryBudgetOnConnection(
		ctx,
		conn,
		command.RunID,
		command.Binding,
	)
	if err != nil {
		return RecoveryStepReceipt{}, err
	}
	if command.Ordinal != projection.NextOrdinal {
		return RecoveryStepReceipt{},
			fail("STALE_RECOVERY_ORDINAL", nil)
	}
	if projection.Parked && command.Kind == RecoveryResumeWorker {
		attentions, attentionErr := attentionProjectionsOnConnection(
			ctx,
			conn,
			command.RunID,
			true,
		)
		if attentionErr != nil {
			return RecoveryStepReceipt{}, attentionErr
		}
		answered := 0
		for _, attention := range attentions {
			if attention.State == AttentionAnswered &&
				attention.Attention.Recovery.CycleID ==
					command.Binding.CycleID &&
				attention.Attention.Recovery.LaneID ==
					command.Binding.LaneID {
				answered++
			}
		}
		if answered != 1 {
			return RecoveryStepReceipt{},
				fail("RECOVERY_PARKED", nil)
		}
	}
	if err := recoveryBudgetAllows(projection, command.Kind); err != nil {
		return RecoveryStepReceipt{}, err
	}
	applyRecoveryCount(&projection, command.Kind)
	if command.Accounting != nil {
		projection.Accounting = cloneRecoveryAccounting(
			command.Accounting,
		)
	}
	projection.NextOrdinal++
	projection.LastStepID = command.ID
	receipt = RecoveryStepReceipt{
		Step:             command,
		AutomaticActions: projection.AutomaticActions,
		Corrections:      projection.Corrections,
		Nudges:           projection.Nudges,
		Advisories:       projection.Advisories,
		SameProgress:     projection.SameProgress,
		Parked:           projection.Parked,
	}
	result, _ = json.Marshal(receipt)
	result = append(result, '\n')
	eventKind := RecoveryStepReservedEvent
	switch command.Kind {
	case RecoveryResumeWorker:
		eventKind = RecoveryResumeWorkerEvent
	case RecoveryAskCaptain:
		eventKind = RecoveryAskCaptainEvent
	case RecoveryRetryOperationally:
		eventKind = RecoveryRetryOperationalEvent
	case RecoveryParkTrack:
		eventKind = RecoveryParkedEvent
	}
	if err := appendSucceededTransition(
		ctx,
		conn,
		succeededTransition{
			runID:        command.RunID,
			replayKey:    replayKey,
			commandKind:  "turn_recovery.step",
			effectKind:   "runtime.turn_recovery",
			body:         body,
			beforeDigest: digest([]byte(strconv.FormatInt(command.Ordinal-1, 10))),
			result:       result,
			eventKind:    eventKind,
			eventBody:    result,
			at:           atValue,
		},
	); err != nil {
		return RecoveryStepReceipt{}, err
	}
	return receipt, nil
}

func (s *Store) ReserveRecoveryStep(
	ctx context.Context,
	owner OwnerLease,
	command RecoveryStepCommand,
	at time.Time,
) (RecoveryStepReceipt, error) {
	if err := validateRecoveryStep(command); err != nil {
		return RecoveryStepReceipt{}, err
	}
	atValue, err := canonicalTime(at)
	if err != nil {
		return RecoveryStepReceipt{}, err
	}
	var receipt RecoveryStepReceipt
	err = s.immediate(ctx, func(conn *sql.Conn) error {
		var reserveErr error
		receipt, reserveErr = reserveRecoveryStepOnConnection(
			ctx,
			conn,
			owner,
			command,
			at,
			atValue,
		)
		return reserveErr
	})
	return receipt, err
}

func (s *Store) RecoveryBudget(
	ctx context.Context,
	runID string,
	binding RecoveryBinding,
) (RecoveryBudgetProjection, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return RecoveryBudgetProjection{}, err
	}
	if err := validateRecoveryBinding(binding); err != nil {
		return RecoveryBudgetProjection{}, err
	}
	var result RecoveryBudgetProjection
	err := s.readTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		result, err = recoveryBudgetOnConnection(
			ctx,
			conn,
			runID,
			binding,
		)
		return err
	})
	return result, err
}
