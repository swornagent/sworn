package journal

import (
	"context"
	"encoding/json"
	"time"
)

const (
	UnverifiedCheckpointSchemaVersion = "sworn.unverified-checkpoint/v1"
	UnverifiedCheckpointEventKind     = "sworn.unverified-checkpoint/v1"
)

type UnverifiedCheckpoint struct {
	SchemaVersion  string `json:"schema_version"`
	Repository     string `json:"repository"`
	RunID          string `json:"run_id"`
	Release        string `json:"release"`
	Track          string `json:"track"`
	Slice          string `json:"slice"`
	PlanOID        string `json:"plan_oid"`
	PlanDigest     string `json:"plan_digest"`
	ContractPath   string `json:"contract_path"`
	ContractDigest string `json:"contract_digest"`
	PreparedBase   string `json:"prepared_base"`
	DispatchWork   string `json:"dispatch_work"`
	Epoch          int64  `json:"epoch"`
	Try            int64  `json:"try"`
	CheckpointRef  string `json:"checkpoint_ref"`
	CommitOID      string `json:"commit_oid"`
	TreeOID        string `json:"tree_oid"`
	TreeDigest     string `json:"tree_digest"`
	StagedBytes    int64  `json:"staged_bytes"`
	FileCount      int    `json:"file_count"`
	CreatedAt      string `json:"created_at"`
}

// RecordUnverifiedCheckpoint persists an unverified checkpoint event into the events table.
func (s *Store) RecordUnverifiedCheckpoint(
	ctx context.Context,
	checkpoint UnverifiedCheckpoint,
	at time.Time,
) error {
	if checkpoint.SchemaVersion == "" {
		checkpoint.SchemaVersion = UnverifiedCheckpointSchemaVersion
	}
	if checkpoint.CreatedAt == "" {
		checkpoint.CreatedAt = at.UTC().Format(time.RFC3339Nano)
	}
	body, err := json.Marshal(checkpoint)
	if err != nil {
		return fail("INVALID_CHECKPOINT", err)
	}
	return s.AppendEvent(ctx, checkpoint.RunID, UnverifiedCheckpointEventKind, body, at)
}

// LatestUnverifiedCheckpoint retrieves the latest unverified checkpoint for slice in runID.
func (s *Store) LatestUnverifiedCheckpoint(
	ctx context.Context,
	runID, slice string,
) (*UnverifiedCheckpoint, error) {
	if s.db == nil || s.conn == nil {
		return nil, fail("CLOSED", nil)
	}
	if err := validateIdentity(runID, "run"); err != nil {
		return nil, err
	}
	rows, err := s.conn.QueryContext(
		ctx,
		`SELECT body FROM events
		 WHERE run_id = ? AND kind = ?
		 ORDER BY event_offset DESC`,
		runID,
		UnverifiedCheckpointEventKind,
	)
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()
	for rows.Next() {
		var body []byte
		if err := rows.Scan(&body); err != nil {
			return nil, dbError(err)
		}
		var cp UnverifiedCheckpoint
		if err := json.Unmarshal(body, &cp); err == nil {
			if cp.Slice == slice {
				return &cp, nil
			}
		}
	}
	return nil, nil
}

// ListUnverifiedCheckpoints returns all unverified checkpoints recorded for runID.
func (s *Store) ListUnverifiedCheckpoints(
	ctx context.Context,
	runID string,
) ([]UnverifiedCheckpoint, error) {
	if s.db == nil || s.conn == nil {
		return nil, fail("CLOSED", nil)
	}
	if err := validateIdentity(runID, "run"); err != nil {
		return nil, err
	}
	rows, err := s.conn.QueryContext(
		ctx,
		`SELECT body FROM events
		 WHERE run_id = ? AND kind = ?
		 ORDER BY event_offset ASC`,
		runID,
		UnverifiedCheckpointEventKind,
	)
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()
	var result []UnverifiedCheckpoint
	for rows.Next() {
		var body []byte
		if err := rows.Scan(&body); err != nil {
			return nil, dbError(err)
		}
		var cp UnverifiedCheckpoint
		if err := json.Unmarshal(body, &cp); err == nil {
			result = append(result, cp)
		}
	}
	return result, nil
}

const (
	CheckpointRestoredSchemaVersion = "sworn.checkpoint-restored/v1"
	CheckpointRestoredEventKind     = "sworn.checkpoint-restored/v1"
)

type CheckpointRestored struct {
	SchemaVersion string `json:"schema_version"`
	RunID         string `json:"run_id"`
	Release       string `json:"release"`
	Track         string `json:"track"`
	Slice         string `json:"slice"`
	CheckpointRef string `json:"checkpoint_ref"`
	TreeOID       string `json:"tree_oid"`
	RestoredAt    string `json:"restored_at"`
}

// RecordCheckpointRestored persists a checkpoint restored event into the events table.
func (s *Store) RecordCheckpointRestored(
	ctx context.Context,
	restored CheckpointRestored,
	at time.Time,
) error {
	if restored.SchemaVersion == "" {
		restored.SchemaVersion = CheckpointRestoredSchemaVersion
	}
	if restored.RestoredAt == "" {
		restored.RestoredAt = at.UTC().Format(time.RFC3339Nano)
	}
	body, err := json.Marshal(restored)
	if err != nil {
		return fail("INVALID_CHECKPOINT", err)
	}
	return s.AppendEvent(ctx, restored.RunID, CheckpointRestoredEventKind, body, at)
}

// ListRestoredCheckpoints returns all checkpoint restored events recorded for runID.
func (s *Store) ListRestoredCheckpoints(
	ctx context.Context,
	runID string,
) ([]CheckpointRestored, error) {
	if s.db == nil || s.conn == nil {
		return nil, fail("CLOSED", nil)
	}
	if err := validateIdentity(runID, "run"); err != nil {
		return nil, err
	}
	rows, err := s.conn.QueryContext(
		ctx,
		`SELECT body FROM events
		 WHERE run_id = ? AND kind = ?
		 ORDER BY event_offset ASC`,
		runID,
		CheckpointRestoredEventKind,
	)
	if err != nil {
		return nil, dbError(err)
	}
	defer rows.Close()
	var result []CheckpointRestored
	for rows.Next() {
		var body []byte
		if err := rows.Scan(&body); err != nil {
			return nil, dbError(err)
		}
		var cr CheckpointRestored
		if err := json.Unmarshal(body, &cr); err == nil {
			result = append(result, cr)
		}
	}
	return result, nil
}
