-- A build claim now records its exact attempt identity in the immutable
-- claimed observation before any native builder can start. Only those new
-- attempts may use the kind-specific machine-proved retry path. Legacy claims
-- have a NULL receipt and remain stopped.
CREATE TABLE migration_007_recovery_guard (marker INTEGER NOT NULL) STRICT;

CREATE TRIGGER migration_007_reject_legacy_recovery
BEFORE INSERT ON migration_007_recovery_guard
BEGIN
    SELECT RAISE(ABORT, 'migration 007 refuses legacy builder recovery authority');
END;

INSERT INTO migration_007_recovery_guard (marker)
SELECT 1 WHERE EXISTS (
    SELECT 1 FROM effect_observations
    WHERE kind = 'not_applied'
       OR (kind = 'claimed' AND receipt_json IS NOT NULL)
) OR EXISTS (
    SELECT 1 FROM effects
    WHERE kind = 'runner.build' AND state IN ('pending', 'running', 'unknown')
);

DROP TRIGGER migration_007_reject_legacy_recovery;
DROP TABLE migration_007_recovery_guard;

CREATE UNIQUE INDEX one_claim_observation_per_attempt
    ON effect_observations(effect_id, attempt)
    WHERE kind = 'claimed';

CREATE UNIQUE INDEX one_retry_observation_per_attempt
    ON effect_observations(effect_id, attempt)
    WHERE kind = 'not_applied';

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
       AND OLD.kind = 'runner.build'
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
