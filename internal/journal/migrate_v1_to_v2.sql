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
    PRIMARY KEY (destination_id, sequence),
    UNIQUE (run_id, destination_id, source_event_offset),
    UNIQUE (run_id, message_id),
    FOREIGN KEY (run_id) REFERENCES runs(run_id)
) WITHOUT ROWID, STRICT;

CREATE INDEX IF NOT EXISTS eval_records_by_run_offset
ON eval_records(run_id, source_event_offset);

CREATE INDEX IF NOT EXISTS outbox_delivery_order
ON notification_outbox(destination_id, state, sequence);

CREATE INDEX IF NOT EXISTS outbox_by_run
ON notification_outbox(run_id, destination_id, sequence);

-- v1 wrote valid UTC RFC3339Nano values with a variable-width fractional
-- component. The cockpit selects a bounded recent-attempt window lexically,
-- so normalize those existing values to the fixed-width representation used
-- by v2 before admitting the migrated journal.
UPDATE attempts
SET created_at = CASE
    WHEN length(created_at) = 20
         AND substr(created_at, 20, 1) = 'Z'
    THEN substr(created_at, 1, 19) || '.000000000Z'
    WHEN length(created_at) BETWEEN 22 AND 30
         AND substr(created_at, 20, 1) = '.'
         AND substr(created_at, -1, 1) = 'Z'
    THEN substr(created_at, 1, length(created_at) - 1)
         || substr('000000000', 1, 30 - length(created_at))
         || 'Z'
    ELSE created_at
END;

PRAGMA user_version = 2;
