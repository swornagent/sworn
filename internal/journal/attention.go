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
	"unicode/utf8"
)

const (
	MaxAttentionQuestionBytes = 16 * 1024
	MaxAttentionAnswerBytes   = 16 * 1024

	AttentionOpenedEvent   = "attention_opened"
	AttentionAnsweredEvent = "attention_answered"
	AttentionResolvedEvent = "attention_resolved"

	attentionCommandVersion = "sworn.attention-command/v1"
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
	ID       string          `json:"attention_id"`
	Ordinal  int64           `json:"ordinal"`
	Recovery RecoveryBinding `json:"recovery"`
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
)

type attentionCommandRecord struct {
	SchemaVersion      string           `json:"schema_version"`
	RunID              string           `json:"run_id"`
	Kind               attentionAction  `json:"kind"`
	Attention          AttentionBinding `json:"attention"`
	ExpectedGeneration int64            `json:"expected_generation"`
	Message            string           `json:"message,omitempty"`
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
	switch value.Kind {
	case attentionOpenAction:
		if value.ExpectedGeneration != 0 ||
			!validAttentionMessage(value.Message, MaxAttentionQuestionBytes) {
			return fail("INVALID_ATTENTION", nil)
		}
	case attentionAnswerAction:
		if value.ExpectedGeneration != 1 ||
			!validAttentionMessage(value.Message, MaxAttentionAnswerBytes) {
			return fail("INVALID_ATTENTION", nil)
		}
	case attentionResolveAction:
		if value.ExpectedGeneration != 2 || value.Message != "" {
			return fail("INVALID_ATTENTION", nil)
		}
	default:
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
			if command.Attention != projection.Attention ||
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
	default:
		return "", fail("INVALID_ATTENTION", nil)
	}
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
	body := marshalAttentionRecord(record)
	replayKey := attentionReplayKey(
		record.Attention.ID,
		record.ExpectedGeneration+1,
	)
	atValue, err := canonicalTime(at)
	if err != nil {
		return AttentionReceipt{}, err
	}
	var receipt AttentionReceipt
	err = s.immediate(ctx, func(conn *sql.Conn) error {
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
			return err
		}
		if replayed {
			if json.Unmarshal(result, &receipt) != nil ||
				receipt.Attention != record.Attention ||
				receipt.Generation != record.ExpectedGeneration+1 {
				return fail("CORRUPT_JOURNAL", nil)
			}
			return nil
		}
		if owner != nil {
			if err := checkOwner(ctx, conn, *owner, at); err != nil {
				return err
			}
		}
		control, err := projectionOnConnection(ctx, conn, record.RunID)
		if err != nil {
			return err
		}
		if control.Desired == "cancelled" ||
			(owner != nil && control.Desired != "running") {
			return fail("CONTROL_STOPPED", nil)
		}
		projections, err := attentionProjectionsOnConnection(
			ctx,
			conn,
			record.RunID,
			false,
		)
		if err != nil {
			return err
		}
		current, found := projections[record.Attention.ID]
		if !found {
			current = AttentionProjection{Attention: record.Attention}
		}
		if current.Attention != record.Attention ||
			current.Generation != record.ExpectedGeneration {
			return fail("STALE_ATTENTION_GENERATION", nil)
		}
		switch record.Kind {
		case attentionOpenAction:
			if found {
				return fail("STALE_ATTENTION_GENERATION", nil)
			}
			receipt.State = AttentionOpen
		case attentionAnswerAction:
			if !found || current.State != AttentionOpen {
				return fail("STALE_ATTENTION_GENERATION", nil)
			}
			receipt.State = AttentionAnswered
		case attentionResolveAction:
			if !found || current.State != AttentionAnswered {
				return fail("STALE_ATTENTION_GENERATION", nil)
			}
			receipt.State = AttentionResolved
		default:
			return fail("INVALID_ATTENTION", nil)
		}
		receipt.Attention = record.Attention
		receipt.Generation = record.ExpectedGeneration + 1
		result, _ = json.Marshal(receipt)
		result = append(result, '\n')
		eventKind, err := attentionEventKind(record.Kind)
		if err != nil {
			return err
		}
		return appendSucceededTransition(
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
		)
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
