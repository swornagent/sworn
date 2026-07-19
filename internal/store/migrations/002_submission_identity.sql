CREATE TABLE submission_records (
    submission_id TEXT PRIMARY KEY,
    delivery_id TEXT NOT NULL,
    work_id TEXT NOT NULL,
    attempt INTEGER NOT NULL CHECK (attempt >= 1 AND attempt <= 9007199254740991),
    digest TEXT NOT NULL UNIQUE REFERENCES records(digest),
    UNIQUE (delivery_id, work_id, attempt)
) STRICT;

CREATE TABLE protocol_identities (
    identity_kind TEXT NOT NULL CHECK (identity_kind IN ('authority_approval', 'builder_run', 'producer_run')),
    identity_id TEXT NOT NULL,
    binding_digest TEXT NOT NULL,
    PRIMARY KEY (identity_kind, identity_id)
) STRICT;

CREATE TRIGGER submission_records_no_update
BEFORE UPDATE ON submission_records BEGIN
    SELECT RAISE(ABORT, 'submission identities are immutable');
END;

CREATE TRIGGER submission_records_no_delete
BEFORE DELETE ON submission_records BEGIN
    SELECT RAISE(ABORT, 'submission identities are immutable');
END;

CREATE TRIGGER protocol_identities_no_update
BEFORE UPDATE ON protocol_identities BEGIN
    SELECT RAISE(ABORT, 'protocol identities are immutable');
END;

CREATE TRIGGER protocol_identities_no_delete
BEFORE DELETE ON protocol_identities BEGIN
    SELECT RAISE(ABORT, 'protocol identities are immutable');
END;
