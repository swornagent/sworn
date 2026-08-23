package journal

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxAttentionQuestionBytes = 16 * 1024
	MaxAttentionAnswerBytes   = 16 * 1024

	AttentionOpenedEvent   = "attention_opened"
	AttentionAnsweredEvent = "attention_answered"
	AttentionResolvedEvent = "attention_resolved"
	AttentionRetiredEvent  = "attention_retired"

	attentionCommandVersion = "sworn.attention-command/v1"
	HumanTurnBindingVersion = "sworn.human-turn-binding/v1"
)

type AttentionState string

const (
	AttentionOpen      AttentionState = "open"
	AttentionAnswered  AttentionState = "answered"
	AttentionResolved  AttentionState = "resolved"
	AttentionCancelled AttentionState = "cancelled"
)

// AttentionBinding makes one operator question specific to a stable recovery
// cycle, turn, progress point, and lane. Ordinal is supplied by the runtime;
// neither content nor a process-local counter contributes to the identity.
type AttentionBinding struct {
	ID        string            `json:"attention_id"`
	Ordinal   int64             `json:"ordinal"`
	Recovery  RecoveryBinding   `json:"recovery"`
	HumanTurn *HumanTurnBinding `json:"human_turn,omitempty"`
}

// HumanTurnBinding classifies an attention as a human-only turn and binds it
// to one exact production dispatch. It deliberately contains no answer or
// authority-bearing payload.
type HumanTurnBinding struct {
	SchemaVersion         string `json:"schema_version"`
	Kind                  string `json:"kind"`
	RunID                 string `json:"run_id"`
	Track                 string `json:"track"`
	Slice                 string `json:"slice"`
	Role                  string `json:"role"`
	Responsibility        string `json:"responsibility"`
	InvocationID          string `json:"invocation_id"`
	BatonAttempt          int64  `json:"baton_attempt"`
	PlanAuthorityDigest   string `json:"plan_authority_digest"`
	TargetAuthorityDigest string `json:"target_authority_digest"`
	WorkIdentity          string `json:"work_identity"`
	CycleID               string `json:"cycle_id"`
	TurnID                string `json:"turn_id"`
	Ordinal               int64  `json:"ordinal"`
	OpenGeneration        int64  `json:"open_generation"`
}

func equalAttentionBinding(left, right AttentionBinding) bool {
	if left.ID != right.ID || left.Ordinal != right.Ordinal ||
		left.Recovery != right.Recovery ||
		(left.HumanTurn == nil) != (right.HumanTurn == nil) {
		return false
	}
	return left.HumanTurn == nil || *left.HumanTurn == *right.HumanTurn
}

func validHumanTurnBinding(
	value *HumanTurnBinding,
	attention AttentionBinding,
	runID string,
) error {
	if value == nil {
		return nil
	}
	validRole := value.Role == "planner" || value.Role == "implementer" ||
		value.Role == "captain" || value.Role == "verifier"
	validResponsibility := value.Responsibility == "planner_proposal" ||
		value.Responsibility == "implementer_design" ||
		value.Responsibility == "implementer_implementation" ||
		value.Responsibility == "captain_review" ||
		value.Responsibility == "captain_plan_review" ||
		value.Responsibility == "work_verification" ||
		value.Responsibility == "assembly_verification"
	if value.SchemaVersion != HumanTurnBindingVersion ||
		(value.Kind != "human_choice" &&
			value.Kind != "human_confirmation") ||
		value.RunID != runID || value.RunID == "" ||
		value.Track == "" || value.Slice == "" ||
		!validRole || !validResponsibility ||
		strings.TrimSpace(value.InvocationID) == "" ||
		strings.ContainsRune(value.InvocationID, 0) ||
		value.BatonAttempt < 1 ||
		validateDigest(value.PlanAuthorityDigest) != nil ||
		validateDigest(value.TargetAuthorityDigest) != nil ||
		validateDigest(value.WorkIdentity) != nil ||
		value.WorkIdentity != attention.Recovery.ProgressID ||
		value.CycleID != attention.Recovery.CycleID ||
		value.TurnID != attention.Recovery.TurnID ||
		value.Ordinal != attention.Ordinal ||
		value.OpenGeneration != 1 {
		return fail("INVALID_ATTENTION_BINDING", nil)
	}
	return nil
}

type OpenAttentionCommand struct {
	RunID              string           `json:"run_id"`
	Attention          AttentionBinding `json:"attention"`
	ExpectedGeneration int64            `json:"expected_generation"`
	Question           string           `json:"question"`
}

type AnswerAttentionCommand struct {
	RunID              string           `json:"run_id"`
	Attention          AttentionBinding `json:"attention"`
	ExpectedGeneration int64            `json:"expected_generation"`
	Answer             string           `json:"answer"`
}

type ResolveAttentionCommand struct {
	RunID              string           `json:"run_id"`
	Attention          AttentionBinding `json:"attention"`
	ExpectedGeneration int64            `json:"expected_generation"`
}

type AttentionReceipt struct {
	Attention  AttentionBinding `json:"attention"`
	Generation int64            `json:"generation"`
	State      AttentionState   `json:"state"`
}

type AttentionProjection struct {
	Attention  AttentionBinding `json:"attention"`
	Generation int64            `json:"generation"`
	State      AttentionState   `json:"state"`
	Question   string           `json:"question"`
	Answer     string           `json:"answer,omitempty"`
}

type attentionAction string

const (
	attentionOpenAction    attentionAction = "open"
	attentionAnswerAction  attentionAction = "answer"
	attentionResolveAction attentionAction = "resolve"
	attentionRetireAction  attentionAction = "retire"
)

type attentionRetireEffect struct {
	ID            string      `json:"effect_id"`
	ExpectedState EffectState `json:"expected_state"`
	ClaimToken    string      `json:"claim_token,omitempty"`
}

type attentionCommandRecord struct {
	SchemaVersion      string                  `json:"schema_version"`
	RunID              string                  `json:"run_id"`
	Kind               attentionAction         `json:"kind"`
	Attention          AttentionBinding        `json:"attention"`
	ExpectedGeneration int64                   `json:"expected_generation"`
	Message            string                  `json:"message,omitempty"`
	RetireEffects      []attentionRetireEffect `json:"retire_effects,omitempty"`
	ErrorCode          string                  `json:"error_code,omitempty"`
}

// AttentionID returns the only admitted identity for an attention ordinal.
// Turn and progress remain exact command bindings, but do not let a retry,
// restart, or wording change create a second attention ordinal.
func AttentionID(binding RecoveryBinding, ordinal int64) string {
	body, _ := json.Marshal(struct {
		SchemaVersion string `json:"schema_version"`
		LaneID        string `json:"lane_id"`
		CycleID       string `json:"cycle_id"`
		Ordinal       int64  `json:"ordinal"`
	}{
		SchemaVersion: "sworn.attention-identity/v1",
		LaneID:        binding.LaneID,
		CycleID:       binding.CycleID,
		Ordinal:       ordinal,
	})
	return digest(body)
}

func validAttentionBinding(value AttentionBinding) error {
	if err := validateRecoveryBinding(value.Recovery); err != nil {
		return err
	}
	if value.Ordinal < 1 ||
		validateDigest(value.ID) != nil ||
		value.ID != AttentionID(value.Recovery, value.Ordinal) {
		return fail("INVALID_ATTENTION_BINDING", nil)
	}
	return nil
}

func validAttentionMessage(value string, limit int) bool {
	return len(value) > 0 &&
		len(value) <= limit &&
		utf8.ValidString(value) &&
		!strings.ContainsRune(value, 0) &&
		!strings.ContainsRune(value, '\r') &&
		strings.TrimSpace(value) != ""
}

func validateAttentionRecord(value attentionCommandRecord) error {
	if value.SchemaVersion != attentionCommandVersion {
		return fail("INVALID_ATTENTION", nil)
	}
	if err := validateIdentity(value.RunID, "run"); err != nil {
		return err
	}
	if err := validAttentionBinding(value.Attention); err != nil {
		return err
	}
	if err := validHumanTurnBinding(
		value.Attention.HumanTurn,
		value.Attention,
		value.RunID,
	); err != nil {
		return err
	}
	switch value.Kind {
	case attentionOpenAction:
		if value.ExpectedGeneration != 0 ||
			!validAttentionMessage(value.Message, MaxAttentionQuestionBytes) {
			return fail("INVALID_ATTENTION", nil)
		}
	case attentionAnswerAction:
		if value.ExpectedGeneration != 1 {
			return fail("INVALID_ATTENTION", nil)
		}
		if len(value.Message) > MaxAttentionAnswerBytes {
			// The oversize bound is named in the detail so an operator can
			// see why the answer was refused, before the generic
			// INVALID_ATTENTION that shares this arm.
			return fail(
				"ATTENTION_ANSWER_OVERSIZE",
				fmt.Errorf(
					"answer exceeds the %d-byte attention answer limit",
					MaxAttentionAnswerBytes,
				),
			)
		}
		if !validAttentionMessage(value.Message, MaxAttentionAnswerBytes) {
			return fail("INVALID_ATTENTION", nil)
		}
	case attentionResolveAction:
		if value.ExpectedGeneration != 2 || value.Message != "" ||
			len(value.RetireEffects) != 0 || value.ErrorCode != "" {
			return fail("INVALID_ATTENTION", nil)
		}
	case attentionRetireAction:
		if (value.ExpectedGeneration != 1 &&
			value.ExpectedGeneration != 2) ||
			value.Message != "" ||
			len(value.RetireEffects) == 0 ||
			len(value.RetireEffects) > 8 ||
			validateIdentity(value.ErrorCode, "error_code") != nil {
			return fail("INVALID_ATTENTION", nil)
		}
		seen := make(map[string]struct{}, len(value.RetireEffects))
		for index, effect := range value.RetireEffects {
			if validateIdentity(effect.ID, "effect") != nil {
				return fail("INVALID_ATTENTION", nil)
			}
			if index > 0 &&
				value.RetireEffects[index-1].ID >= effect.ID {
				return fail("INVALID_ATTENTION", nil)
			}
			if _, duplicate := seen[effect.ID]; duplicate {
				return fail("INVALID_ATTENTION", nil)
			}
			seen[effect.ID] = struct{}{}
			switch effect.ExpectedState {
			case Claimed:
				if len(effect.ClaimToken) != 64 {
					return fail("INVALID_CLAIM_TOKEN", nil)
				}
			case Succeeded, OperationalFailed:
				if effect.ClaimToken != "" {
					return fail("INVALID_CLAIM_TOKEN", nil)
				}
			default:
				return fail("INVALID_ATTENTION", nil)
			}
		}
	default:
		return fail("INVALID_ATTENTION", nil)
	}
	if value.Kind != attentionRetireAction &&
		(len(value.RetireEffects) != 0 || value.ErrorCode != "") {
		return fail("INVALID_ATTENTION", nil)
	}
	return nil
}

func marshalAttentionRecord(value attentionCommandRecord) []byte {
	body, _ := json.Marshal(value)
	return append(body, '\n')
}

func parseAttentionRecord(
	runID, commandKind string,
	body []byte,
) (attentionCommandRecord, error) {
	var value attentionCommandRecord
	if json.Unmarshal(body, &value) != nil ||
		value.RunID != runID ||
		commandKind != "attention."+string(value.Kind) ||
		!bytes.Equal(body, marshalAttentionRecord(value)) {
		return attentionCommandRecord{}, fail("CORRUPT_JOURNAL", nil)
	}
	if err := validateAttentionRecord(value); err != nil {
		return attentionCommandRecord{}, fail("CORRUPT_JOURNAL", err)
	}
	return value, nil
}

func attentionRecordsOnConnection(
	ctx context.Context,
	conn *sql.Conn,
	runID string,
) ([]attentionCommandRecord, error) {
	rows, err := conn.QueryContext(
		ctx,
		`SELECT kind,payload FROM commands
		  WHERE run_id=? AND kind LIKE 'attention.%'`,
		runID,
	)
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()
	var result []attentionCommandRecord
	for rows.Next() {
		var kind string
		var body []byte
		if err := rows.Scan(&kind, &body); err != nil {
			return nil, dbError(err)
		}
		record, err := parseAttentionRecord(runID, kind, body)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return nil, dbError(err)
	}
	return result, nil
}

func foldAttentionRecords(
	records []attentionCommandRecord,
) (map[string]AttentionProjection, error) {
	grouped := make(map[string][]attentionCommandRecord)
	for _, record := range records {
		id := record.Attention.ID
		grouped[id] = append(grouped[id], record)
	}
	result := make(map[string]AttentionProjection, len(grouped))
	for id, commands := range grouped {
		sort.Slice(commands, func(left, right int) bool {
			return commands[left].ExpectedGeneration <
				commands[right].ExpectedGeneration
		})
		projection := AttentionProjection{Attention: commands[0].Attention}
		for _, command := range commands {
			if !equalAttentionBinding(
				command.Attention,
				projection.Attention,
			) ||
				command.ExpectedGeneration != projection.Generation {
				return nil, fail("CORRUPT_JOURNAL", nil)
			}
			switch {
			case projection.Generation == 0 &&
				command.Kind == attentionOpenAction:
				projection.Question = command.Message
				projection.State = AttentionOpen
			case projection.Generation == 1 &&
				command.Kind == attentionAnswerAction:
				projection.Answer = command.Message
				projection.State = AttentionAnswered
			case projection.Generation == 2 &&
				command.Kind == attentionResolveAction:
				projection.State = AttentionResolved
			case (projection.Generation == 1 ||
				projection.Generation == 2) &&
				command.Kind == attentionRetireAction:
				projection.State = AttentionCancelled
			default:
				return nil, fail("CORRUPT_JOURNAL", nil)
			}
			projection.Generation++
		}
		result[id] = projection
	}
	return result, nil
}

func attentionProjectionsOnConnection(
	ctx context.Context,
	conn *sql.Conn,
	runID string,
	effectiveCancel bool,
) (map[string]AttentionProjection, error) {
	records, err := attentionRecordsOnConnection(ctx, conn, runID)
	if err != nil {
		return nil, err
	}
	result, err := foldAttentionRecords(records)
	if err != nil || !effectiveCancel {
		return result, err
	}
	control, err := projectionOnConnection(ctx, conn, runID)
	if err != nil {
		return nil, err
	}
	if control.Desired == "cancelled" {
		for id, value := range result {
			if value.State != AttentionResolved {
				value.State = AttentionCancelled
				result[id] = value
			}
		}
	}
	return result, nil
}

func attentionReplayKey(attentionID string, generation int64) string {
	return "attention/" + strings.TrimPrefix(attentionID, "sha256:") +
		"/g" + strconv.FormatInt(generation, 10)
}

func attentionEventKind(action attentionAction) (string, error) {
	switch action {
	case attentionOpenAction:
		return AttentionOpenedEvent, nil
	case attentionAnswerAction:
		return AttentionAnsweredEvent, nil
	case attentionResolveAction:
		return AttentionResolvedEvent, nil
	case attentionRetireAction:
		return AttentionRetiredEvent, nil
	default:
		return "", fail("INVALID_ATTENTION", nil)
	}
}

func applyAttentionOnConnection(
	ctx context.Context,
	conn *sql.Conn,
	owner *OwnerLease,
	record attentionCommandRecord,
	at time.Time,
	atValue string,
) (AttentionReceipt, error) {
	body := marshalAttentionRecord(record)
	replayKey := attentionReplayKey(
		record.Attention.ID,
		record.ExpectedGeneration+1,
	)
	var receipt AttentionReceipt
	result, replayed, err := replayedTransition(
		ctx,
		conn,
		record.RunID,
		replayKey,
		"attention."+string(record.Kind),
		"runtime.attention",
		body,
	)
	if err != nil {
		return AttentionReceipt{}, err
	}
	if replayed {
		if json.Unmarshal(result, &receipt) != nil ||
			!equalAttentionBinding(receipt.Attention, record.Attention) ||
			receipt.Generation != record.ExpectedGeneration+1 {
			return AttentionReceipt{}, fail("CORRUPT_JOURNAL", nil)
		}
		return receipt, nil
	}
	if owner != nil {
		if err := checkOwner(ctx, conn, *owner, at); err != nil {
			return AttentionReceipt{}, err
		}
	}
	control, err := projectionOnConnection(ctx, conn, record.RunID)
	if err != nil {
		return AttentionReceipt{}, err
	}
	if control.Desired == "cancelled" ||
		(owner != nil && control.Desired != "running") {
		return AttentionReceipt{}, fail("CONTROL_STOPPED", nil)
	}
	projections, err := attentionProjectionsOnConnection(
		ctx,
		conn,
		record.RunID,
		false,
	)
	if err != nil {
		return AttentionReceipt{}, err
	}
	current, found := projections[record.Attention.ID]
	if !found {
		current = AttentionProjection{Attention: record.Attention}
	}
	if !equalAttentionBinding(current.Attention, record.Attention) ||
		current.Generation != record.ExpectedGeneration {
		return AttentionReceipt{},
			fail("STALE_ATTENTION_GENERATION", nil)
	}
	switch record.Kind {
	case attentionOpenAction:
		if found {
			return AttentionReceipt{},
				fail("STALE_ATTENTION_GENERATION", nil)
		}
		receipt.State = AttentionOpen
	case attentionAnswerAction:
		if !found || current.State != AttentionOpen {
			return AttentionReceipt{},
				fail("STALE_ATTENTION_GENERATION", nil)
		}
		receipt.State = AttentionAnswered
	case attentionResolveAction:
		if !found || current.State != AttentionAnswered {
			return AttentionReceipt{},
				fail("STALE_ATTENTION_GENERATION", nil)
		}
		receipt.State = AttentionResolved
	case attentionRetireAction:
		if !found ||
			(current.State != AttentionOpen &&
				current.State != AttentionAnswered) {
			return AttentionReceipt{},
				fail("STALE_ATTENTION_GENERATION", nil)
		}
		receipt.State = AttentionCancelled
	default:
		return AttentionReceipt{}, fail("INVALID_ATTENTION", nil)
	}
	receipt.Attention = record.Attention
	receipt.Generation = record.ExpectedGeneration + 1
	result, _ = json.Marshal(receipt)
	result = append(result, '\n')
	eventKind, err := attentionEventKind(record.Kind)
	if err != nil {
		return AttentionReceipt{}, err
	}
	if err := appendSucceededTransition(
		ctx,
		conn,
		succeededTransition{
			runID:        record.RunID,
			replayKey:    replayKey,
			commandKind:  "attention." + string(record.Kind),
			effectKind:   "runtime.attention",
			body:         body,
			beforeDigest: digest([]byte(strconv.FormatInt(current.Generation, 10))),
			result:       result,
			eventKind:    eventKind,
			eventBody:    result,
			at:           atValue,
		},
	); err != nil {
		return AttentionReceipt{}, err
	}
	return receipt, nil
}

func (s *Store) applyAttention(
	ctx context.Context,
	owner *OwnerLease,
	record attentionCommandRecord,
	at time.Time,
) (AttentionReceipt, error) {
	if err := validateAttentionRecord(record); err != nil {
		return AttentionReceipt{}, err
	}
	atValue, err := canonicalTime(at)
	if err != nil {
		return AttentionReceipt{}, err
	}
	var receipt AttentionReceipt
	err = s.immediate(ctx, func(conn *sql.Conn) error {
		var applyErr error
		receipt, applyErr = applyAttentionOnConnection(
			ctx,
			conn,
			owner,
			record,
			at,
			atValue,
		)
		return applyErr
	})
	return receipt, err
}

func (s *Store) OpenAttention(
	ctx context.Context,
	owner OwnerLease,
	command OpenAttentionCommand,
	at time.Time,
) (AttentionReceipt, error) {
	return s.applyAttention(
		ctx,
		&owner,
		attentionCommandRecord{
			SchemaVersion:      attentionCommandVersion,
			RunID:              command.RunID,
			Kind:               attentionOpenAction,
			Attention:          command.Attention,
			ExpectedGeneration: command.ExpectedGeneration,
			Message:            command.Question,
		},
		at,
	)
}

func (s *Store) AnswerAttention(
	ctx context.Context,
	command AnswerAttentionCommand,
	at time.Time,
) (AttentionReceipt, error) {
	return s.applyAttention(
		ctx,
		nil,
		attentionCommandRecord{
			SchemaVersion:      attentionCommandVersion,
			RunID:              command.RunID,
			Kind:               attentionAnswerAction,
			Attention:          command.Attention,
			ExpectedGeneration: command.ExpectedGeneration,
			Message:            command.Answer,
		},
		at,
	)
}

func (s *Store) ResolveAttention(
	ctx context.Context,
	owner OwnerLease,
	command ResolveAttentionCommand,
	at time.Time,
) (AttentionReceipt, error) {
	return s.applyAttention(
		ctx,
		&owner,
		attentionCommandRecord{
			SchemaVersion:      attentionCommandVersion,
			RunID:              command.RunID,
			Kind:               attentionResolveAction,
			Attention:          command.Attention,
			ExpectedGeneration: command.ExpectedGeneration,
		},
		at,
	)
}

func (s *Store) Attention(
	ctx context.Context,
	runID, attentionID string,
) (AttentionProjection, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return AttentionProjection{}, err
	}
	if err := validateDigest(attentionID); err != nil {
		return AttentionProjection{}, fail("INVALID_ATTENTION_BINDING", nil)
	}
	var result AttentionProjection
	err := s.readTransaction(ctx, func(conn *sql.Conn) error {
		projections, err := attentionProjectionsOnConnection(
			ctx,
			conn,
			runID,
			true,
		)
		if err != nil {
			return err
		}
		var found bool
		result, found = projections[attentionID]
		if !found {
			return fail("ATTENTION_NOT_FOUND", nil)
		}
		return nil
	})
	return result, err
}

func (s *Store) Attentions(
	ctx context.Context,
	runID string,
) ([]AttentionProjection, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return nil, err
	}
	var result []AttentionProjection
	err := s.readTransaction(ctx, func(conn *sql.Conn) error {
		projections, err := attentionProjectionsOnConnection(
			ctx,
			conn,
			runID,
			true,
		)
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(projections))
		for id := range projections {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			result = append(result, projections[id])
		}
		return nil
	})
	return result, err
}
