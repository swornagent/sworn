package journal

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ControlKind string

const (
	Pause    ControlKind = "pause"
	Resume   ControlKind = "resume"
	Cancel   ControlKind = "cancel"
	Retry    ControlKind = "retry"
	Takeover ControlKind = "takeover"
)

type ControlCommand struct {
	RunID              string      `json:"run_id"`
	ID                 string      `json:"command_id"`
	Kind               ControlKind `json:"kind"`
	ExpectedGeneration int64       `json:"expected_generation"`
	WorkID             string      `json:"work_id,omitempty"`
	ExpectedEpoch      int64       `json:"expected_epoch,omitempty"`
}

type ControlReceipt struct {
	CommandID  string      `json:"command_id"`
	Kind       ControlKind `json:"kind"`
	Generation int64       `json:"generation"`
	Epoch      int64       `json:"epoch,omitempty"`
}

type ControlProjection struct {
	Generation  int64
	Desired     string
	RetryEpochs map[string]int64
}

type OwnerLease struct {
	RunID      string
	Token      string
	Generation int64
	ExpiresAt  time.Time
}

type EffectAttempt struct {
	WorkID string
	Epoch  int64
	Try    int64
}

func AttemptEffectID(workID string, epoch, try int64) string {
	return fmt.Sprintf("attempt/%s/e%d/t%d", strings.TrimPrefix(workID, "sha256:"), epoch, try)
}

func validControl(command ControlCommand) error {
	if err := validateIdentity(command.RunID, "run"); err != nil {
		return err
	}
	if err := validateIdentity(command.ID, "command"); err != nil {
		return err
	}
	if command.ExpectedGeneration < 0 {
		return fail("INVALID_GENERATION", nil)
	}
	switch command.Kind {
	case Pause, Resume, Cancel, Takeover:
		if command.WorkID != "" || command.ExpectedEpoch != 0 {
			return fail("INVALID_CONTROL", nil)
		}
	case Retry:
		if err := validateDigest(command.WorkID); err != nil || command.ExpectedEpoch < 1 {
			return fail("INVALID_CONTROL", nil)
		}
	default:
		return fail("INVALID_CONTROL", nil)
	}
	return nil
}

func projectionOnConnection(ctx context.Context, conn *sql.Conn, runID string) (ControlProjection, error) {
	result := ControlProjection{Desired: "running", RetryEpochs: map[string]int64{}}
	rows, err := conn.QueryContext(ctx,
		`SELECT payload FROM commands WHERE run_id = ? AND kind LIKE 'control.%'`, runID)
	if err != nil {
		return result, dbError(err)
	}
	var commands []ControlCommand
	for rows.Next() {
		var body []byte
		if err := rows.Scan(&body); err != nil {
			return result, dbError(err)
		}
		var command ControlCommand
		if err := json.Unmarshal(body, &command); err != nil || command.RunID != runID {
			_ = rows.Close()
			return result, fail("CORRUPT_JOURNAL", nil)
		}
		commands = append(commands, command)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return result, dbError(err)
	}
	if err := rows.Close(); err != nil {
		return result, dbError(err)
	}
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].ExpectedGeneration < commands[j].ExpectedGeneration
	})
	for _, command := range commands {
		if command.ExpectedGeneration != result.Generation {
			return result, fail("CORRUPT_JOURNAL", nil)
		}
		result.Generation++
		switch command.Kind {
		case Pause:
			result.Desired = "paused"
		case Resume:
			if result.Desired == "cancelled" {
				return result, fail("CORRUPT_JOURNAL", nil)
			}
			result.Desired = "running"
		case Cancel:
			result.Desired = "cancelled"
		case Retry:
			result.RetryEpochs[command.WorkID] = command.ExpectedEpoch + 1
		}
	}
	return result, nil
}

func (s *Store) ControlProjection(ctx context.Context, runID string) (ControlProjection, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return ControlProjection{}, err
	}
	if s == nil {
		return ControlProjection{}, fail("CLOSED", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return ControlProjection{}, fail("CLOSED", nil)
	}
	return projectionOnConnection(ctx, s.conn, runID)
}

// CurrentOwner returns the exact live-or-expired owner claim without changing it.
func (s *Store) CurrentOwner(ctx context.Context, runID string) (OwnerLease, bool, error) {
	if err := validateIdentity(runID, "run"); err != nil {
		return OwnerLease{}, false, err
	}
	if s == nil {
		return OwnerLease{}, false, fail("CLOSED", nil)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return OwnerLease{}, false, fail("CLOSED", nil)
	}
	return currentOwnerOnConnection(ctx, s.conn, runID)
}

func currentOwnerOnConnection(
	ctx context.Context,
	conn *sql.Conn,
	runID string,
) (OwnerLease, bool, error) {
	var state, token, expires, completed string
	var generation int64
	err := conn.QueryRowContext(
		ctx,
		`SELECT e.state,COALESCE(e.current_claim,''),COALESCE(c.expires_at,''),
		        COALESCE(c.completed_at,''),
		        (SELECT count(*) FROM claims
		          WHERE run_id=e.run_id AND effect_id='runtime.owner')
		   FROM effects e
		   LEFT JOIN claims c ON c.run_id=e.run_id AND c.effect_id=e.effect_id
		    AND c.token=e.current_claim
		  WHERE e.run_id=? AND e.effect_id='runtime.owner'`,
		runID,
	).Scan(&state, &token, &expires, &completed, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		return OwnerLease{}, false, nil
	}
	if err != nil {
		return OwnerLease{}, false, dbError(err)
	}
	if state == string(Pending) && token == "" {
		return OwnerLease{}, false, nil
	}
	expiry, err := parseTime(expires)
	if state != string(Claimed) || len(token) != 64 || completed != "" ||
		generation < 1 || err != nil {
		return OwnerLease{}, false, fail("CORRUPT_JOURNAL", err)
	}
	return OwnerLease{
		RunID: runID, Token: token, Generation: generation, ExpiresAt: expiry,
	}, true, nil
}

func (s *Store) ApplyControl(ctx context.Context, command ControlCommand, at time.Time) (ControlReceipt, error) {
	if err := validControl(command); err != nil {
		return ControlReceipt{}, err
	}
	body, err := json.Marshal(command)
	if err != nil {
		return ControlReceipt{}, fail("INVALID_CONTROL", nil)
	}
	body = append(body, '\n')
	replayKey := "control/" + command.ID
	var receipt ControlReceipt
	err = s.immediate(ctx, func(conn *sql.Conn) error {
		var observed []byte
		err := conn.QueryRowContext(ctx,
			`SELECT payload FROM commands WHERE run_id = ? AND replay_key = ?`,
			command.RunID, replayKey).Scan(&observed)
		if err == nil {
			if !bytes.Equal(observed, body) {
				return fail("REPLAY_CONFLICT", nil)
			}
			var result []byte
			if err := conn.QueryRowContext(ctx,
				`SELECT result FROM effects WHERE run_id = ? AND effect_id = ?`,
				command.RunID, replayKey).Scan(&result); err != nil ||
				json.Unmarshal(result, &receipt) != nil {
				return fail("CORRUPT_JOURNAL", nil)
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return dbError(err)
		}
		projection, err := projectionOnConnection(ctx, conn, command.RunID)
		if err != nil {
			return err
		}
		if projection.Generation != command.ExpectedGeneration ||
			(projection.Desired == "cancelled" && command.Kind != Cancel) {
			return fail("STALE_CONTROL_GENERATION", nil)
		}
		if command.Kind == Takeover {
			var expires string
			if err := conn.QueryRowContext(ctx,
				`SELECT c.expires_at FROM effects e JOIN claims c
				  ON c.run_id=e.run_id AND c.effect_id=e.effect_id AND c.token=e.current_claim
				 WHERE e.run_id=? AND e.effect_id='runtime.owner' AND e.state='claimed'`,
				command.RunID).Scan(&expires); err != nil {
				return fail("OWNER_NOT_TAKEOVERABLE", nil)
			}
			expiry, err := parseTime(expires)
			if err != nil || expiry.After(at) {
				return fail("OWNER_ACTIVE", nil)
			}
		}
		if command.Kind == Retry {
			epoch := projection.RetryEpochs[command.WorkID]
			if epoch == 0 {
				epoch = 1
			}
			if epoch != command.ExpectedEpoch {
				return fail("STALE_RETRY_EPOCH", nil)
			}
			var state string
			if err := conn.QueryRowContext(ctx,
				`SELECT state FROM effects WHERE run_id = ? AND effect_id = ?`,
				command.RunID, AttemptEffectID(command.WorkID, epoch, 3)).Scan(&state); err != nil || state != string(OperationalFailed) {
				return fail("WORK_NOT_EXHAUSTED", nil)
			}
			receipt.Epoch = epoch + 1
		}
		receipt.CommandID, receipt.Kind = command.ID, command.Kind
		receipt.Generation = projection.Generation + 1
		result, _ := json.Marshal(receipt)
		result = append(result, '\n')
		now, err := canonicalTime(at)
		if err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO commands(run_id,replay_key,kind,payload_digest,payload,created_at)
			 VALUES(?,?,?,?,?,?)`,
			command.RunID, replayKey, "control."+string(command.Kind), digest(body), body, now); err != nil {
			return dbError(err)
		}
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO effects(run_id,effect_id,replay_key,kind,state,before_digest,
			 expected_digest,result_digest,result,updated_at)
			 VALUES(?,?,?,'runtime.control','succeeded',?,?,?,?,?)`,
			command.RunID, replayKey, replayKey,
			digest([]byte(strconv.FormatInt(projection.Generation, 10))),
			digest(body), digest(result), result, now); err != nil {
			return dbError(err)
		}
		return appendEvent(ctx, conn, command.RunID, "control_accepted", result, now)
	})
	return receipt, err
}

func ensureOwnerEffect(ctx context.Context, conn *sql.Conn, runID, at string) error {
	body := []byte("{}\n")
	_, err := conn.ExecContext(ctx,
		`INSERT OR IGNORE INTO commands(run_id,replay_key,kind,payload_digest,payload,created_at)
		 VALUES(?,'runtime.owner','runtime.owner',?,?,?)`, runID, digest(body), body, at)
	if err != nil {
		return dbError(err)
	}
	_, err = conn.ExecContext(ctx,
		`INSERT OR IGNORE INTO effects(run_id,effect_id,replay_key,kind,state,
		 before_digest,expected_digest,updated_at)
		 VALUES(?,'runtime.owner','runtime.owner','runtime.owner','pending',?,?,?)`,
		runID, digest([]byte(runID)), digest(body), at)
	return dbError(err)
}

func (s *Store) AcquireOwner(ctx context.Context, runID string, now time.Time, duration time.Duration, takeover bool) (OwnerLease, error) {
	if err := validateIdentity(runID, "run"); err != nil || duration <= 0 || duration > MaxLease {
		return OwnerLease{}, fail("INVALID_OWNER_LEASE", nil)
	}
	var lease OwnerLease
	err := s.immediate(ctx, func(conn *sql.Conn) error {
		at, err := canonicalTime(now)
		if err != nil {
			return err
		}
		if err := ensureOwnerEffect(ctx, conn, runID, at); err != nil {
			return err
		}
		var state string
		var current sql.NullString
		if err := conn.QueryRowContext(ctx,
			`SELECT state,current_claim FROM effects
			 WHERE run_id=? AND effect_id='runtime.owner'`, runID).Scan(&state, &current); err != nil {
			return dbError(err)
		}
		if state == string(Claimed) {
			var expires string
			if err := conn.QueryRowContext(ctx,
				`SELECT expires_at FROM claims WHERE run_id=? AND effect_id='runtime.owner' AND token=?`,
				runID, current.String).Scan(&expires); err != nil {
				return dbError(err)
			}
			expiry, err := parseTime(expires)
			if err != nil {
				return err
			}
			if expiry.After(now) {
				return fail("OWNER_ACTIVE", nil)
			}
			if !takeover {
				return fail("TAKEOVER_REQUIRED", nil)
			}
			if _, err := conn.ExecContext(ctx,
				`UPDATE claims SET completed_at=?,outcome='expired'
				 WHERE run_id=? AND effect_id='runtime.owner' AND token=? AND completed_at IS NULL`,
				at, runID, current.String); err != nil {
				return dbError(err)
			}
		} else if state != string(Pending) || takeover {
			return fail("OWNER_NOT_TAKEOVERABLE", nil)
		}
		token, err := randomToken()
		if err != nil {
			return err
		}
		expiresAt := now.Add(duration).UTC()
		expires, _ := canonicalTime(expiresAt)
		var generation int64
		if err := conn.QueryRowContext(ctx,
			`SELECT count(*)+1 FROM claims WHERE run_id=? AND effect_id='runtime.owner'`,
			runID).Scan(&generation); err != nil {
			return dbError(err)
		}
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO claims(run_id,effect_id,token,acquired_at,expires_at)
			 VALUES(?,'runtime.owner',?,?,?)`, runID, token, at, expires); err != nil {
			return dbError(err)
		}
		if _, err := conn.ExecContext(ctx,
			`UPDATE effects SET state='claimed',current_claim=?,updated_at=?
			 WHERE run_id=? AND effect_id='runtime.owner'`, token, at, runID); err != nil {
			return dbError(err)
		}
		lease = OwnerLease{RunID: runID, Token: token, Generation: generation, ExpiresAt: expiresAt}
		return appendEvent(ctx, conn, runID, "owner_acquired",
			[]byte(strconv.FormatInt(generation, 10)), at)
	})
	return lease, err
}

func checkOwner(ctx context.Context, conn *sql.Conn, lease OwnerLease, now time.Time) error {
	var token, expires string
	var generation int64
	if err := conn.QueryRowContext(ctx,
		`SELECT e.current_claim,c.expires_at,
		        (SELECT count(*) FROM claims WHERE run_id=e.run_id AND effect_id='runtime.owner')
		 FROM effects e JOIN claims c ON c.run_id=e.run_id AND c.effect_id=e.effect_id
		  AND c.token=e.current_claim
		 WHERE e.run_id=? AND e.effect_id='runtime.owner' AND e.state='claimed'`,
		lease.RunID).Scan(&token, &expires, &generation); err != nil {
		return fail("OWNER_FENCED", nil)
	}
	expiry, err := parseTime(expires)
	if err != nil || token != lease.Token || generation != lease.Generation || !expiry.After(now) {
		return fail("OWNER_FENCED", nil)
	}
	return nil
}

func (s *Store) RenewOwner(ctx context.Context, lease OwnerLease, now time.Time, duration time.Duration) (OwnerLease, error) {
	if duration <= 0 || duration > MaxLease {
		return OwnerLease{}, fail("INVALID_OWNER_LEASE", nil)
	}
	err := s.immediate(ctx, func(conn *sql.Conn) error {
		if err := checkOwner(ctx, conn, lease, now); err != nil {
			return err
		}
		expiresAt := now.Add(duration).UTC()
		expires, _ := canonicalTime(expiresAt)
		result, err := conn.ExecContext(ctx,
			`UPDATE claims SET expires_at=? WHERE run_id=? AND effect_id='runtime.owner'
			 AND token=? AND completed_at IS NULL`, expires, lease.RunID, lease.Token)
		if err != nil {
			return dbError(err)
		}
		if err := requireRows(result, "OWNER_FENCED"); err != nil {
			return err
		}
		lease.ExpiresAt = expiresAt
		return nil
	})
	return lease, err
}

func (s *Store) ReleaseOwner(ctx context.Context, lease OwnerLease, now time.Time) error {
	return s.immediate(ctx, func(conn *sql.Conn) error {
		if err := checkOwner(ctx, conn, lease, now); err != nil {
			return err
		}
		return releaseOwnerOnConnection(ctx, conn, lease, now)
	})
}

// ReleaseOwnerIfIdle closes the final answer-vs-owner race. The answered
// attention and owner release are checked under the same write transaction:
// either the current owner remains responsible for the wake, or a later
// answer observes no owner and can acquire it.
func (s *Store) ReleaseOwnerIfIdle(
	ctx context.Context,
	lease OwnerLease,
	now time.Time,
) (bool, error) {
	released := false
	err := s.immediate(ctx, func(conn *sql.Conn) error {
		if err := checkOwner(ctx, conn, lease, now); err != nil {
			return err
		}
		control, err := projectionOnConnection(ctx, conn, lease.RunID)
		if err != nil {
			return err
		}
		if control.Desired != "running" {
			if err := releaseOwnerOnConnection(
				ctx,
				conn,
				lease,
				now,
			); err != nil {
				return err
			}
			released = true
			return nil
		}
		attentions, err := attentionProjectionsOnConnection(
			ctx,
			conn,
			lease.RunID,
			true,
		)
		if err != nil {
			return err
		}
		for _, attention := range attentions {
			if attention.State == AttentionAnswered {
				return nil
			}
		}
		if err := releaseOwnerOnConnection(
			ctx,
			conn,
			lease,
			now,
		); err != nil {
			return err
		}
		released = true
		return nil
	})
	return released, err
}

func releaseOwnerOnConnection(
	ctx context.Context,
	conn *sql.Conn,
	lease OwnerLease,
	now time.Time,
) error {
	at, _ := canonicalTime(now)
	if _, err := conn.ExecContext(ctx,
		`UPDATE claims SET completed_at=?,outcome='released'
		 WHERE run_id=? AND effect_id='runtime.owner' AND token=?`,
		at, lease.RunID, lease.Token); err != nil {
		return dbError(err)
	}
	if _, err := conn.ExecContext(ctx,
		`UPDATE effects SET state='pending',current_claim=NULL,updated_at=?
		 WHERE run_id=? AND effect_id='runtime.owner' AND current_claim=?`,
		at, lease.RunID, lease.Token); err != nil {
		return dbError(err)
	}
	return appendEvent(
		ctx,
		conn,
		lease.RunID,
		"owner_released",
		[]byte(strconv.FormatInt(lease.Generation, 10)),
		at,
	)
}

func (s *Store) EnsureAttempt(ctx context.Context, command Command, effect Effect, attempt EffectAttempt) error {
	if attempt.Try < 1 || attempt.Try > 3 || attempt.Epoch < 1 ||
		validateDigest(attempt.WorkID) != nil ||
		effect.ID != AttemptEffectID(attempt.WorkID, attempt.Epoch, attempt.Try) ||
		command.RunID != effect.RunID || command.ReplayKey != effect.ReplayKey ||
		command.ReplayKey != effect.ID || command.Kind != effect.Kind {
		return fail("INVALID_EFFECT_ATTEMPT", nil)
	}
	prepared, err := prepareCommand(command)
	if err != nil {
		return err
	}
	updatedAt, err := prepareEffect(effect)
	if err != nil {
		return err
	}
	return s.immediate(ctx, func(conn *sql.Conn) error {
		projection, err := projectionOnConnection(ctx, conn, command.RunID)
		if err != nil {
			return err
		}
		epoch := projection.RetryEpochs[attempt.WorkID]
		if epoch == 0 {
			epoch = 1
		}
		if epoch != attempt.Epoch {
			return fail("STALE_RETRY_EPOCH", nil)
		}
		if attempt.Try > 1 {
			previous, err := effectOnConnection(ctx, conn, command.RunID,
				AttemptEffectID(attempt.WorkID, attempt.Epoch, attempt.Try-1))
			if err != nil || previous.State != OperationalFailed {
				return fail("PREVIOUS_ATTEMPT_NOT_RETRYABLE", nil)
			}
		}
		if err := recordCommandOnConnection(ctx, conn, command, prepared); err != nil {
			return err
		}
		return ensureEffectOnConnection(ctx, conn, effect, updatedAt)
	})
}
