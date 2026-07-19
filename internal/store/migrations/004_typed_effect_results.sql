DROP TRIGGER protocol_identities_no_delete;
DROP TRIGGER protocol_identities_no_update;
DROP TABLE protocol_identities;

-- receipt_json becomes the effect row's single typed-result slot. It may move
-- from NULL to exact JSON once under a running lease, then remains byte-for-byte
-- immutable through interruption and terminal success.
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
      (OLD.state = 'unknown' AND NEW.state = 'pending'
       AND NEW.attempt = OLD.attempt
       AND OLD.receipt_json IS NULL AND NEW.receipt_json IS NULL
       AND NEW.owner_id IS NULL
       AND NEW.started_at_us IS NULL
       AND NEW.completed_at_us IS NULL
       AND NEW.last_error IS NULL) OR
      (OLD.state = 'unknown' AND NEW.state = 'succeeded'
       AND NEW.attempt = OLD.attempt
       AND OLD.receipt_json IS NOT NULL
       AND NEW.receipt_json IS OLD.receipt_json
       AND NEW.owner_id IS OLD.owner_id
       AND NEW.started_at_us IS OLD.started_at_us
       AND NEW.completed_at_us IS NOT NULL
       AND NEW.last_error IS NULL) OR
      (OLD.state = 'unknown' AND NEW.state = 'failed'
       AND NEW.attempt = OLD.attempt
       AND OLD.receipt_json IS NULL AND NEW.receipt_json IS NULL
       AND NEW.owner_id IS OLD.owner_id
       AND NEW.started_at_us IS OLD.started_at_us
       AND NEW.completed_at_us IS NOT NULL
       AND NEW.last_error IS NOT NULL)
  )
BEGIN
    SELECT RAISE(ABORT, 'invalid effect transition');
END;
