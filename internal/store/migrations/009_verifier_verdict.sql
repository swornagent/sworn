-- Independent verification remains inside the one effect journal. Dispatch
-- and verdict relationship rows are immutable indexes over canonical records,
-- raw CAS artifacts, commands, events, and effects; none is a second workflow
-- or mutable "current verdict" table.
CREATE TABLE verifier_dispatch_records (
    dispatch_id TEXT PRIMARY KEY,
    digest TEXT NOT NULL UNIQUE REFERENCES records(digest),
    artifact_digest TEXT NOT NULL UNIQUE REFERENCES artifacts(digest),
    submission_id TEXT NOT NULL REFERENCES submission_records(submission_id),
    submission_digest TEXT NOT NULL REFERENCES records(digest),
    effect_id TEXT NOT NULL UNIQUE REFERENCES effects(effect_id),
    profile_digest TEXT NOT NULL,
    run_id TEXT NOT NULL,
    command_id TEXT NOT NULL UNIQUE,
    review_epoch INTEGER NOT NULL CHECK (review_epoch >= 1 AND review_epoch <= 3),
    created_at_us INTEGER NOT NULL,
    CHECK (dispatch_id = effect_id),
    CHECK (digest = artifact_digest),
    UNIQUE (submission_id, review_epoch),
    FOREIGN KEY (command_id, run_id) REFERENCES commands(command_id, run_id)
) STRICT;

CREATE TRIGGER verifier_dispatch_records_require_dispatch
BEFORE INSERT ON verifier_dispatch_records
WHEN NOT EXISTS (
    SELECT 1
    FROM commands AS command
    JOIN effects AS effect
      ON effect.command_id = command.command_id
     AND effect.run_id = command.run_id
    JOIN submission_records AS submission
      ON submission.submission_id = NEW.submission_id
     AND submission.digest = NEW.submission_digest
     AND submission.run_id = NEW.run_id
    WHERE command.command_id = NEW.command_id
      AND command.run_id = NEW.run_id
      AND command.kind = 'verifier.dispatch'
      AND command.outcome = 'applied'
      AND effect.effect_id = NEW.effect_id
      AND effect.kind = 'runner.verifier'
      AND effect.ordinal = 0
) BEGIN
    SELECT RAISE(ABORT, 'verifier dispatch identity requires its exact applied command, effect, and submission');
END;

CREATE TRIGGER verifier_dispatch_records_no_update
BEFORE UPDATE ON verifier_dispatch_records BEGIN
    SELECT RAISE(ABORT, 'verifier dispatch identities are immutable');
END;

CREATE TRIGGER verifier_dispatch_records_no_delete
BEFORE DELETE ON verifier_dispatch_records BEGIN
    SELECT RAISE(ABORT, 'verifier dispatch identities are immutable');
END;

CREATE TABLE verdict_records (
    verdict_id TEXT PRIMARY KEY,
    digest TEXT NOT NULL UNIQUE REFERENCES records(digest),
    submission_id TEXT NOT NULL REFERENCES submission_records(submission_id),
    submission_digest TEXT NOT NULL REFERENCES records(digest),
    dispatch_id TEXT NOT NULL UNIQUE REFERENCES verifier_dispatch_records(dispatch_id),
    verifier_effect_id TEXT NOT NULL UNIQUE REFERENCES effects(effect_id),
    assessment_digest TEXT NOT NULL REFERENCES records(digest),
    outcome TEXT NOT NULL CHECK (outcome IN ('PASS', 'FAIL', 'SPEC_BLOCK', 'INCONCLUSIVE')),
    run_id TEXT NOT NULL,
    command_id TEXT NOT NULL UNIQUE,
    event_id TEXT NOT NULL UNIQUE REFERENCES events(event_id),
    event_revision INTEGER NOT NULL CHECK (event_revision >= 0 AND event_revision <= 9007199254740991),
    review_epoch INTEGER NOT NULL CHECK (review_epoch >= 1 AND review_epoch <= 3),
    created_at_us INTEGER NOT NULL,
    CHECK (dispatch_id = verifier_effect_id),
    FOREIGN KEY (command_id, run_id) REFERENCES commands(command_id, run_id)
) STRICT;

CREATE TRIGGER verdict_records_require_admission
BEFORE INSERT ON verdict_records
WHEN NOT EXISTS (
    SELECT 1
    FROM commands AS command
    JOIN events AS event
      ON event.command_id = command.command_id
     AND event.run_id = command.run_id
    JOIN submission_records AS submission
      ON submission.submission_id = NEW.submission_id
     AND submission.digest = NEW.submission_digest
     AND submission.run_id = NEW.run_id
    JOIN verifier_dispatch_records AS dispatch
      ON dispatch.dispatch_id = NEW.dispatch_id
     AND dispatch.effect_id = NEW.verifier_effect_id
     AND dispatch.submission_id = NEW.submission_id
     AND dispatch.submission_digest = NEW.submission_digest
     AND dispatch.run_id = NEW.run_id
     AND dispatch.review_epoch = NEW.review_epoch
    WHERE command.command_id = NEW.command_id
      AND command.run_id = NEW.run_id
      AND command.kind = 'verdict.admit'
      AND command.outcome = 'applied'
      AND event.event_id = NEW.event_id
      AND event.kind = 'verdict.admitted'
      AND event.revision = NEW.event_revision
) BEGIN
    SELECT RAISE(ABORT, 'verdict identity requires its exact applied admission event and dispatch');
END;

CREATE TRIGGER verdict_records_no_update
BEFORE UPDATE ON verdict_records BEGIN
    SELECT RAISE(ABORT, 'verdict identities are immutable');
END;

CREATE TRIGGER verdict_records_no_delete
BEFORE DELETE ON verdict_records BEGIN
    SELECT RAISE(ABORT, 'verdict identities are immutable');
END;

-- A claimed verifier may return directly to pending only while its process-
-- local capability proves that no worker entry occurred. Store writes an exact
-- not_applied witness matching the durable claimed attempt identity before the
-- transition. Unknown verifier attempts remain permanently non-retryable.
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
       )) OR
      (OLD.state = 'running' AND NEW.state = 'pending'
       AND OLD.kind = 'runner.verifier'
       AND NEW.attempt = OLD.attempt
       AND OLD.receipt_json IS NULL AND NEW.receipt_json IS NULL
       AND NEW.owner_id IS NULL
       AND NEW.started_at_us IS NULL
       AND NEW.completed_at_us IS NULL
       AND NEW.last_error IS NULL
       AND EXISTS (
           SELECT 1
           FROM verifier_dispatch_records AS dispatch
           JOIN effect_observations AS claimed
             ON claimed.effect_id = dispatch.effect_id
            AND claimed.attempt = OLD.attempt
            AND claimed.kind = 'claimed'
           JOIN effect_observations AS not_applied
             ON not_applied.effect_id = claimed.effect_id
            AND not_applied.attempt = claimed.attempt
            AND not_applied.kind = 'not_applied'
           WHERE dispatch.dispatch_id = OLD.effect_id
             AND dispatch.effect_id = OLD.effect_id
             AND claimed.receipt_json IS NOT NULL
             AND not_applied.receipt_json = claimed.receipt_json
       ))
  )
BEGIN
    SELECT RAISE(ABORT, 'invalid effect transition');
END;
