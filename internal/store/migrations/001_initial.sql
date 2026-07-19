CREATE TABLE runs (
    run_id TEXT PRIMARY KEY,
    delivery_id TEXT NOT NULL UNIQUE,
    repository_id TEXT NOT NULL,
    target_ref TEXT NOT NULL,
    plan_digest TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK (revision >= 0),
    phase TEXT NOT NULL CHECK (phase IN ('planned', 'active')),
    terminal INTEGER NOT NULL DEFAULT 0 CHECK (terminal IN (0, 1)),
    state_json BLOB NOT NULL CHECK (json_valid(state_json)),
    created_at_us INTEGER NOT NULL,
    updated_at_us INTEGER NOT NULL
) STRICT;

CREATE UNIQUE INDEX one_active_run_per_target
    ON runs(repository_id, target_ref)
    WHERE terminal = 0;

CREATE TABLE commands (
    command_id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    expected_revision INTEGER NOT NULL,
    request_digest TEXT NOT NULL,
    request_json BLOB NOT NULL CHECK (json_valid(request_json)),
    outcome TEXT NOT NULL CHECK (outcome IN ('applied', 'rejected')),
    result_json BLOB NOT NULL CHECK (json_valid(result_json)),
    error_code TEXT,
    error_message TEXT,
    recorded_at_us INTEGER NOT NULL,
    CHECK (
        (outcome = 'applied' AND error_code IS NULL AND error_message IS NULL) OR
        (outcome = 'rejected' AND error_code IS NOT NULL AND error_message IS NOT NULL)
    )
) STRICT;

CREATE INDEX commands_by_run ON commands(run_id, recorded_at_us, command_id);

CREATE TABLE events (
    event_id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES runs(run_id),
    command_id TEXT NOT NULL REFERENCES commands(command_id),
    revision INTEGER NOT NULL CHECK (revision >= 0),
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    kind TEXT NOT NULL,
    data_json BLOB NOT NULL CHECK (json_valid(data_json)),
    recorded_at_us INTEGER NOT NULL,
    UNIQUE (run_id, revision, ordinal)
) STRICT;

CREATE TABLE effects (
    effect_id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES runs(run_id),
    command_id TEXT NOT NULL REFERENCES commands(command_id),
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    kind TEXT NOT NULL,
    request_json BLOB NOT NULL CHECK (json_valid(request_json)),
    state TEXT NOT NULL CHECK (state IN ('pending', 'running', 'unknown', 'succeeded', 'failed')),
    attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    owner_id TEXT,
    receipt_json BLOB CHECK (receipt_json IS NULL OR json_valid(receipt_json)),
    last_error TEXT,
    created_at_us INTEGER NOT NULL,
    started_at_us INTEGER,
    completed_at_us INTEGER,
    UNIQUE (command_id, ordinal),
    CHECK (
        (state = 'pending' AND owner_id IS NULL AND receipt_json IS NULL AND completed_at_us IS NULL) OR
        (state = 'running' AND owner_id IS NOT NULL AND started_at_us IS NOT NULL AND completed_at_us IS NULL) OR
        (state = 'unknown' AND owner_id IS NOT NULL AND started_at_us IS NOT NULL AND last_error IS NOT NULL AND completed_at_us IS NULL) OR
        (state = 'succeeded' AND receipt_json IS NOT NULL AND completed_at_us IS NOT NULL) OR
        (state = 'failed' AND last_error IS NOT NULL AND completed_at_us IS NOT NULL)
    )
) STRICT;

CREATE INDEX effects_pending ON effects(state, created_at_us, effect_id);

CREATE TABLE effect_observations (
    observation_id INTEGER PRIMARY KEY AUTOINCREMENT,
    effect_id TEXT NOT NULL REFERENCES effects(effect_id),
    attempt INTEGER NOT NULL CHECK (attempt > 0),
    kind TEXT NOT NULL CHECK (kind IN ('claimed', 'unknown', 'not_applied', 'succeeded', 'failed')),
    owner_id TEXT,
    receipt_json BLOB CHECK (receipt_json IS NULL OR json_valid(receipt_json)),
    detail TEXT,
    recorded_at_us INTEGER NOT NULL
) STRICT;

CREATE TABLE records (
    digest TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    canonical_json BLOB NOT NULL CHECK (json_valid(canonical_json)),
    size INTEGER NOT NULL CHECK (size >= 0),
    created_at_us INTEGER NOT NULL
) STRICT;

CREATE TABLE artifacts (
    digest TEXT PRIMARY KEY,
    media_type TEXT NOT NULL,
    content BLOB NOT NULL,
    size INTEGER NOT NULL CHECK (size >= 0),
    created_at_us INTEGER NOT NULL
) STRICT;

CREATE TRIGGER commands_no_update
BEFORE UPDATE ON commands BEGIN
    SELECT RAISE(ABORT, 'commands are immutable');
END;

CREATE TRIGGER runs_no_delete
BEFORE DELETE ON runs BEGIN
    SELECT RAISE(ABORT, 'runs are durable history');
END;

CREATE TRIGGER runs_restrict_update
BEFORE UPDATE ON runs
WHEN NEW.run_id != OLD.run_id
  OR NEW.delivery_id != OLD.delivery_id
  OR NEW.repository_id != OLD.repository_id
  OR NEW.target_ref != OLD.target_ref
  OR NEW.plan_digest != OLD.plan_digest
  OR NEW.created_at_us != OLD.created_at_us
  OR NEW.revision != OLD.revision + 1
BEGIN
    SELECT RAISE(ABORT, 'invalid run snapshot update');
END;

CREATE TRIGGER commands_no_delete
BEFORE DELETE ON commands BEGIN
    SELECT RAISE(ABORT, 'commands are immutable');
END;

CREATE TRIGGER events_no_update
BEFORE UPDATE ON events BEGIN
    SELECT RAISE(ABORT, 'events are immutable');
END;

CREATE TRIGGER events_no_delete
BEFORE DELETE ON events BEGIN
    SELECT RAISE(ABORT, 'events are immutable');
END;

CREATE TRIGGER observations_no_update
BEFORE UPDATE ON effect_observations BEGIN
    SELECT RAISE(ABORT, 'effect observations are immutable');
END;

CREATE TRIGGER effects_no_delete
BEFORE DELETE ON effects BEGIN
    SELECT RAISE(ABORT, 'effects are durable history');
END;

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
      (OLD.state = 'pending' AND NEW.state = 'running' AND NEW.attempt = OLD.attempt + 1) OR
      (OLD.state = 'running' AND NEW.state IN ('unknown', 'succeeded', 'failed') AND NEW.attempt = OLD.attempt) OR
      (OLD.state = 'unknown' AND NEW.state IN ('pending', 'succeeded', 'failed') AND NEW.attempt = OLD.attempt)
  )
BEGIN
    SELECT RAISE(ABORT, 'invalid effect transition');
END;

CREATE TRIGGER observations_no_delete
BEFORE DELETE ON effect_observations BEGIN
    SELECT RAISE(ABORT, 'effect observations are immutable');
END;

CREATE TRIGGER records_no_update
BEFORE UPDATE ON records BEGIN
    SELECT RAISE(ABORT, 'records are immutable');
END;

CREATE TRIGGER records_no_delete
BEFORE DELETE ON records BEGIN
    SELECT RAISE(ABORT, 'records are immutable');
END;

CREATE TRIGGER artifacts_no_update
BEFORE UPDATE ON artifacts BEGIN
    SELECT RAISE(ABORT, 'artifacts are immutable');
END;

CREATE TRIGGER artifacts_no_delete
BEFORE DELETE ON artifacts BEGIN
    SELECT RAISE(ABORT, 'artifacts are immutable');
END;
