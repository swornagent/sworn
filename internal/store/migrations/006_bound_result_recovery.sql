-- Unknown effects may close only from their immutable bound result. Retrying
-- an unbound attempt requires a later kind-specific, attempt-bound witness;
-- free-form reconciliation no longer authorizes unknown -> pending or failed.
CREATE TABLE migration_006_retry_guard (marker INTEGER NOT NULL) STRICT;

CREATE TRIGGER migration_006_reject_manual_retry
BEFORE INSERT ON migration_006_retry_guard
BEGIN
    SELECT RAISE(ABORT, 'migration 006 refuses a previously manual-requeued effect');
END;

INSERT INTO migration_006_retry_guard (marker)
SELECT 1 WHERE EXISTS (
    SELECT 1 FROM effects WHERE state = 'pending' AND attempt > 0
);

DROP TRIGGER migration_006_reject_manual_retry;
DROP TABLE migration_006_retry_guard;

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
       AND NEW.last_error IS NULL)
  )
BEGIN
    SELECT RAISE(ABORT, 'invalid effect transition');
END;
