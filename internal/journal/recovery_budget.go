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
	MaxRecoveryCorrectionsPerTurn   = int64(2)
	MaxRecoveryNudgesPerTurn        = int64(1)
	MaxRecoveryAdvisoriesPerCycle   = int64(1)
	MaxRecoveryDecisionsPerProgress = int64(2)
	MaxRecoveryAutomaticPerCycle    = int64(4)

	RecoveryStepReservedEvent = "turn_recovery_step_reserved"
	RecoveryParkedEvent       = "turn_recovery_parked"

	recoveryStepVersion = "sworn.turn-recovery-step/v1"
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

type RecoveryStepCommand struct {
	RunID   string           `json:"run_id"`
	ID      string           `json:"step_id"`
	Binding RecoveryBinding  `json:"binding"`
	Ordinal int64            `json:"ordinal"`
	Kind    RecoveryStepKind `json:"kind"`
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
	Binding          RecoveryBinding `json:"binding"`
	NextOrdinal      int64           `json:"next_ordinal"`
	AutomaticActions int64           `json:"automatic_actions"`
	Corrections      int64           `json:"corrections"`
	Nudges           int64           `json:"nudges"`
	Advisories       int64           `json:"advisories"`
	SameProgress     int64           `json:"same_progress"`
	Parked           bool            `json:"parked"`
	LastStepID       string          `json:"last_step_id,omitempty"`
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
	return nil
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
		if step.Ordinal != result.NextOrdinal || result.Parked {
			return RecoveryBudgetProjection{}, fail("CORRUPT_JOURNAL", nil)
		}
		result.NextOrdinal++
		result.LastStepID = step.ID
		if recoveryAutomatic(step.Kind) {
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
		if recoveryDecision(step.Kind) &&
			step.Binding.ProgressID == binding.ProgressID {
			result.SameProgress++
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
	if recoveryAutomatic(kind) {
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
	if recoveryDecision(kind) {
		projection.SameProgress++
	}
}

func recoveryBudgetAllows(
	projection RecoveryBudgetProjection,
	kind RecoveryStepKind,
) error {
	if projection.Parked {
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

func (s *Store) ReserveRecoveryStep(
	ctx context.Context,
	owner OwnerLease,
	command RecoveryStepCommand,
	at time.Time,
) (RecoveryStepReceipt, error) {
	if err := validateRecoveryStep(command); err != nil {
		return RecoveryStepReceipt{}, err
	}
	record := recoveryStepRecord{
		SchemaVersion: recoveryStepVersion,
		Step:          command,
	}
	body := marshalRecoveryStep(record)
	replayKey := recoveryReplayKey(command.ID)
	atValue, err := canonicalTime(at)
	if err != nil {
		return RecoveryStepReceipt{}, err
	}
	var receipt RecoveryStepReceipt
	err = s.immediate(ctx, func(conn *sql.Conn) error {
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
			return err
		}
		if replayed {
			if json.Unmarshal(result, &receipt) != nil ||
				receipt.Step != command {
				return fail("CORRUPT_JOURNAL", nil)
			}
			return nil
		}
		if err := checkOwner(ctx, conn, owner, at); err != nil {
			return err
		}
		control, err := projectionOnConnection(ctx, conn, command.RunID)
		if err != nil {
			return err
		}
		if control.Desired != "running" {
			return fail("CONTROL_STOPPED", nil)
		}
		projection, err := recoveryBudgetOnConnection(
			ctx,
			conn,
			command.RunID,
			command.Binding,
		)
		if err != nil {
			return err
		}
		if command.Ordinal != projection.NextOrdinal {
			return fail("STALE_RECOVERY_ORDINAL", nil)
		}
		if err := recoveryBudgetAllows(projection, command.Kind); err != nil {
			return err
		}
		applyRecoveryCount(&projection, command.Kind)
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
		if command.Kind == RecoveryParkTrack {
			eventKind = RecoveryParkedEvent
		}
		return appendSucceededTransition(
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
		)
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
