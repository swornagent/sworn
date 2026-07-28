CREATE TABLE IF NOT EXISTS observer_cursors (
    observer TEXT NOT NULL,
    run_id TEXT NOT NULL,
    event_offset INTEGER NOT NULL CHECK (event_offset >= 0),
    batch_digest TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (observer, run_id),
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
    message_id TEXT NOT NULL,
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
    UNIQUE (run_id, message_id),
    FOREIGN KEY (run_id) REFERENCES runs(run_id)
) WITHOUT ROWID, STRICT;

CREATE INDEX IF NOT EXISTS eval_records_by_run_offset
ON eval_records(run_id, source_event_offset);

CREATE INDEX IF NOT EXISTS outbox_delivery_order
ON notification_outbox(destination_id, run_id, state, sequence);

PRAGMA user_version = 2;
