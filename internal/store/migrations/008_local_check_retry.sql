-- A witnessed local-check attempt may return to pending after controller-owned
-- reconciliation proves its deterministic executor and materialization roots
-- are quiescent and absent. "not_applied" means no typed journal result was
-- applied; content-addressed orphan artifacts may exist and are harmless.
-- A v7 local-check claim had no content-bound attempt witness. Pending checks
-- are safe because their next controlled claim creates one, and bound unknown
-- checks can finish from their typed result. An unbound running/unknown check
-- cannot be distinguished from a still-live legacy subprocess, so upgrading it
-- would create a Store that no v1 controller can recover safely.
CREATE TABLE migration_008_recovery_guard (marker INTEGER NOT NULL) STRICT;

CREATE TRIGGER migration_008_reject_legacy_check_recovery
BEFORE INSERT ON migration_008_recovery_guard
BEGIN
    SELECT RAISE(ABORT, 'migration 008 refuses legacy local-check recovery authority');
END;

INSERT INTO migration_008_recovery_guard (marker)
SELECT 1 WHERE EXISTS (
    SELECT 1 FROM effects
    WHERE kind = 'check.local'
      AND state IN ('running', 'unknown')
      AND receipt_json IS NULL
);

DROP TRIGGER migration_008_reject_legacy_check_recovery;
DROP TABLE migration_008_recovery_guard;

DROP TRIGGER effects_restrict_update;

CREATE TRIGGER effects_restrict_update
BEFORE UPDATE ON effects
WHEN NEW.effect_id != OLD.effect_id
  OR NEW.run_id != OLD.run_id
  OR NEW.command_id != OLD.command_id
  OR NEW.ordinal != OLD.ordinal
  OR NEW.kind != OLD.kind
  OR NEW.request_json != OLD.request_json
  OR NEW.created_at_us != OLD.created_at_us
  OR NOT (
      (OLD.state = 'pending' AND NEW.state = 'running'
       AND NEW.attempt = OLD.attempt + 1
       AND OLD.receipt_json IS NULL AND NEW.receipt_json IS NULL
       AND NEW.owner_id IS NOT NULL
       AND NEW.started_at_us IS NOT NULL
       AND NEW.completed_at_us IS NULL
       AND NEW.last_error IS NULL) OR
      (OLD.state = 'running' AND NEW.state = 'running'
       AND NEW.attempt = OLD.attempt
       AND OLD.receipt_json IS NULL AND NEW.receipt_json IS NOT NULL
       AND NEW.owner_id IS OLD.owner_id
       AND NEW.started_at_us IS OLD.started_at_us
       AND NEW.completed_at_us IS OLD.completed_at_us
       AND NEW.last_error IS OLD.last_error) OR
      (OLD.state = 'running' AND NEW.state = 'unknown'
       AND NEW.attempt = OLD.attempt
       AND NEW.receipt_json IS OLD.receipt_json
       AND NEW.owner_id IS OLD.owner_id
       AND NEW.started_at_us IS OLD.started_at_us
       AND NEW.completed_at_us IS NULL
       AND NEW.last_error IS NOT NULL) OR
      (OLD.state = 'running' AND NEW.state = 'succeeded'
       AND NEW.attempt = OLD.attempt
       AND OLD.receipt_json IS NOT NULL
       AND NEW.receipt_json IS OLD.receipt_json
       AND NEW.owner_id IS OLD.owner_id
       AND NEW.started_at_us IS OLD.started_at_us
       AND NEW.completed_at_us IS NOT NULL
       AND NEW.last_error IS NULL) OR
      (OLD.state = 'running' AND NEW.state = 'failed'
       AND NEW.attempt = OLD.attempt
       AND OLD.receipt_json IS NULL AND NEW.receipt_json IS NULL
       AND NEW.owner_id IS OLD.owner_id
       AND NEW.started_at_us IS OLD.started_at_us
       AND NEW.completed_at_us IS NOT NULL
       AND NEW.last_error IS NOT NULL) OR
      (OLD.state = 'unknown' AND NEW.state = 'succeeded'
       AND NEW.attempt = OLD.attempt
       AND OLD.receipt_json IS NOT NULL
       AND NEW.receipt_json IS OLD.receipt_json
       AND NEW.owner_id IS OLD.owner_id
       AND NEW.started_at_us IS OLD.started_at_us
       AND NEW.completed_at_us IS NOT NULL
       AND NEW.last_error IS NULL) OR
      (OLD.state = 'unknown' AND NEW.state = 'pending'
       AND OLD.kind IN ('runner.build', 'check.local')
       AND NEW.attempt = OLD.attempt
       AND OLD.receipt_json IS NULL AND NEW.receipt_json IS NULL
       AND NEW.owner_id IS NULL
       AND NEW.started_at_us IS NULL
       AND NEW.completed_at_us IS NULL
       AND NEW.last_error IS NULL
       AND EXISTS (
           SELECT 1
           FROM effect_observations AS claimed
           JOIN effect_observations AS reconciled
             ON reconciled.effect_id = claimed.effect_id
            AND reconciled.attempt = claimed.attempt
            AND reconciled.kind = 'not_applied'
           WHERE claimed.effect_id = OLD.effect_id
             AND claimed.attempt = OLD.attempt
             AND claimed.kind = 'claimed'
             AND claimed.receipt_json IS NOT NULL
             AND reconciled.receipt_json = claimed.receipt_json
       ))
  )
BEGIN
    SELECT RAISE(ABORT, 'invalid effect transition');
END;
