PRAGMA journal_mode = DELETE;
PRAGMA synchronous = FULL;
PRAGMA foreign_keys = ON;
PRAGMA trusted_schema = OFF;
PRAGMA busy_timeout = 5000;
PRAGMA application_id = 1398230866;
PRAGMA user_version = 2;

CREATE TABLE IF NOT EXISTS runs (
    run_id TEXT PRIMARY KEY,
    manifest_digest TEXT NOT NULL,
    repository TEXT NOT NULL,
    release_id TEXT NOT NULL,
    target_ref TEXT NOT NULL,
    created_at TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS commands (
    run_id TEXT NOT NULL,
    replay_key TEXT NOT NULL,
    kind TEXT NOT NULL,
    payload_digest TEXT NOT NULL,
    payload BLOB NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (run_id, replay_key),
    FOREIGN KEY (run_id) REFERENCES runs(run_id)
) WITHOUT ROWID, STRICT;

CREATE TABLE IF NOT EXISTS effects (
    run_id TEXT NOT NULL,
    effect_id TEXT NOT NULL,
    replay_key TEXT NOT NULL,
    kind TEXT NOT NULL,
    state TEXT NOT NULL CHECK (
        state IN ('pending', 'claimed', 'succeeded', 'operational_failed', 'uncertain')
    ),
    before_digest TEXT NOT NULL,
    expected_digest TEXT NOT NULL,
    current_claim TEXT,
    result_digest TEXT,
    result BLOB,
    error_code TEXT,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (run_id, effect_id),
    FOREIGN KEY (run_id, replay_key) REFERENCES commands(run_id, replay_key)
) WITHOUT ROWID, STRICT;

CREATE TABLE IF NOT EXISTS claims (
    run_id TEXT NOT NULL,
    effect_id TEXT NOT NULL,
    token TEXT NOT NULL,
    acquired_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    completed_at TEXT,
    outcome TEXT,
    PRIMARY KEY (run_id, effect_id, token),
    FOREIGN KEY (run_id, effect_id) REFERENCES effects(run_id, effect_id)
) WITHOUT ROWID, STRICT;

CREATE TABLE IF NOT EXISTS attempts (
    run_id TEXT NOT NULL,
    effect_id TEXT NOT NULL,
    attempt INTEGER NOT NULL CHECK (attempt > 0),
    responsibility TEXT NOT NULL,
    transport_status TEXT NOT NULL,
    observation_digest TEXT NOT NULL,
    usage_digest TEXT NOT NULL,
    usage BLOB NOT NULL,
    handoff_digest TEXT,
    created_at TEXT NOT NULL,
    PRIMARY KEY (run_id, effect_id, attempt),
    FOREIGN KEY (run_id, effect_id) REFERENCES effects(run_id, effect_id)
) WITHOUT ROWID, STRICT;

CREATE TABLE IF NOT EXISTS receipts (
    digest TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    effect_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    body BLOB NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (run_id, effect_id) REFERENCES effects(run_id, effect_id)
) WITHOUT ROWID, STRICT;

CREATE TABLE IF NOT EXISTS events (
    event_offset INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    body_digest TEXT NOT NULL,
    body BLOB NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (run_id) REFERENCES runs(run_id)
) STRICT;

CREATE INDEX IF NOT EXISTS effects_by_run_state
ON effects(run_id, state, effect_id);

CREATE INDEX IF NOT EXISTS events_by_run_offset
ON events(run_id, event_offset);

CREATE TABLE IF NOT EXISTS observer_cursors (
    consumer TEXT NOT NULL,
    run_id TEXT NOT NULL,
    event_offset INTEGER NOT NULL CHECK (event_offset >= 0),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (consumer, run_id),
    FOREIGN KEY (run_id) REFERENCES runs(run_id)
) WITHOUT ROWID, STRICT;

CREATE TABLE IF NOT EXISTS eval_records (
    run_id TEXT NOT NULL,
    observer TEXT NOT NULL,
    source_event_offset INTEGER NOT NULL CHECK (source_event_offset > 0),
    record_id TEXT NOT NULL,
    schema_version TEXT NOT NULL,
    body_digest TEXT NOT NULL,
    body BLOB NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (run_id, observer, source_event_offset),
    UNIQUE (run_id, record_id),
    FOREIGN KEY (run_id) REFERENCES runs(run_id)
) WITHOUT ROWID, STRICT;

CREATE TABLE IF NOT EXISTS notification_outbox (
    run_id TEXT NOT NULL,
    destination_id TEXT NOT NULL,
    source_event_offset INTEGER NOT NULL CHECK (source_event_offset > 0),
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    message_id TEXT NOT NULL UNIQUE,
    schema_version TEXT NOT NULL,
    body_digest TEXT NOT NULL,
    body BLOB NOT NULL,
    state TEXT NOT NULL CHECK (
        state IN ('pending', 'claimed', 'delivered', 'dead')
    ),
    attempts INTEGER NOT NULL CHECK (attempts >= 0),
    available_at TEXT NOT NULL,
    claim_token TEXT,
    claimed_until TEXT,
    delivered_at TEXT,
    last_error_code TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (run_id, destination_id, sequence),
    UNIQUE (run_id, destination_id, source_event_offset),
    FOREIGN KEY (run_id) REFERENCES runs(run_id)
) WITHOUT ROWID, STRICT;

CREATE INDEX IF NOT EXISTS eval_records_by_run_offset
ON eval_records(run_id, source_event_offset);

CREATE INDEX IF NOT EXISTS outbox_delivery_order
ON notification_outbox(destination_id, run_id, state, sequence);
