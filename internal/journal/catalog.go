package journal

import (
	"context"
	"database/sql"
)

// MaxRunBindings bounds one complete journal catalog projection. Returning an
// error instead of a partial catalog keeps callers from mistaking truncation
// for the complete set of recorded runs.
const MaxRunBindings = 1024

// RunBindings returns every run recorded in this journal, oldest first. Runs
// with the same creation time are ordered by ID.
func (s *Store) RunBindings(ctx context.Context) ([]Run, error) {
	result := make([]Run, 0)
	err := s.readTransaction(ctx, func(conn *sql.Conn) error {
		rows, err := conn.QueryContext(
			ctx,
			`SELECT run_id, manifest_digest, repository, release_id,
			        target_ref, created_at
			 FROM runs
			 ORDER BY created_at, run_id
			 LIMIT ?`,
			MaxRunBindings+1,
		)
		if err != nil {
			return dbError(err)
		}
		defer rows.Close()

		for rows.Next() {
			if len(result) == MaxRunBindings {
				return fail("RESOURCE_LIMIT", nil)
			}
			var run Run
			var createdAt string
			if err := rows.Scan(
				&run.ID,
				&run.ManifestDigest,
				&run.Repository,
				&run.Release,
				&run.TargetRef,
				&createdAt,
			); err != nil {
				return dbError(err)
			}
			run.CreatedAt, err = parseTime(createdAt)
			if err != nil {
				return err
			}
			result = append(result, run)
		}
		if err := rows.Err(); err != nil {
			return dbError(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
