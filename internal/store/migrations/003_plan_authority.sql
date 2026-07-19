DROP TRIGGER protocol_identities_no_delete;

-- v2 accepted structural authority receipt IDs from unauthenticated
-- submissions. Authenticated authority_approvals is now their sole truth.
DELETE FROM protocol_identities
WHERE identity_kind = 'authority_approval';

CREATE TABLE authority_source_snapshots (
    source_ref TEXT NOT NULL,
    source_version INTEGER NOT NULL CHECK (source_version >= 1 AND source_version <= 9007199254740991),
    source_id TEXT NOT NULL,
    source_digest TEXT NOT NULL REFERENCES records(digest),
    status TEXT NOT NULL CHECK (status IN ('active', 'revoked')),
    repository_id TEXT NOT NULL,
    target_ref TEXT NOT NULL,
    authorizer_ref TEXT NOT NULL,
    valid_from TEXT NOT NULL,
    valid_until TEXT NOT NULL,
    authenticated_at_us INTEGER NOT NULL,
    PRIMARY KEY (source_ref, source_version),
    UNIQUE (source_ref, source_digest),
    UNIQUE (source_ref, source_version, source_digest)
) STRICT;

CREATE TABLE authority_source_authentications (
    source_ref TEXT NOT NULL,
    source_version INTEGER NOT NULL,
    source_digest TEXT NOT NULL,
    source_artifact_digest TEXT NOT NULL REFERENCES artifacts(digest),
    proof_digest TEXT NOT NULL REFERENCES artifacts(digest),
    proof_canonical_digest TEXT NOT NULL,
    plan_digest TEXT NOT NULL REFERENCES records(digest),
    authority_digest TEXT NOT NULL,
    root_key_id TEXT NOT NULL,
    approved_at TEXT NOT NULL,
    authenticated_at_us INTEGER NOT NULL,
    PRIMARY KEY (source_ref, source_version, source_artifact_digest, proof_digest),
    UNIQUE (source_ref, source_version, source_digest, source_artifact_digest, proof_digest),
    FOREIGN KEY (source_ref, source_version, source_digest)
        REFERENCES authority_source_snapshots(source_ref, source_version, source_digest)
) STRICT;

CREATE TABLE authority_approvals (
    receipt_id TEXT PRIMARY KEY,
    receipt_digest TEXT NOT NULL UNIQUE REFERENCES artifacts(digest),
    plan_digest TEXT NOT NULL REFERENCES records(digest),
    authority_digest TEXT NOT NULL,
    source_ref TEXT NOT NULL,
    source_version INTEGER NOT NULL,
    source_digest TEXT NOT NULL,
    source_artifact_digest TEXT NOT NULL REFERENCES artifacts(digest),
    proof_digest TEXT NOT NULL UNIQUE REFERENCES artifacts(digest),
    proof_canonical_digest TEXT NOT NULL,
    root_key_id TEXT NOT NULL,
    authorizer_ref TEXT NOT NULL,
    approved_at TEXT NOT NULL,
    recorded_at_us INTEGER NOT NULL,
    FOREIGN KEY (source_ref, source_version, source_digest)
        REFERENCES authority_source_snapshots(source_ref, source_version, source_digest),
    FOREIGN KEY (source_ref, source_version, source_digest, source_artifact_digest, proof_digest)
        REFERENCES authority_source_authentications(
            source_ref, source_version, source_digest, source_artifact_digest, proof_digest
        )
) STRICT;

CREATE TRIGGER authority_source_snapshots_no_update
BEFORE UPDATE ON authority_source_snapshots BEGIN
    SELECT RAISE(ABORT, 'authority source snapshots are immutable');
END;

CREATE TRIGGER authority_source_snapshots_no_delete
BEFORE DELETE ON authority_source_snapshots BEGIN
    SELECT RAISE(ABORT, 'authority source snapshots are immutable');
END;

CREATE TRIGGER authority_source_authentications_no_update
BEFORE UPDATE ON authority_source_authentications BEGIN
    SELECT RAISE(ABORT, 'authority source authentications are immutable');
END;

CREATE TRIGGER authority_source_authentications_no_delete
BEFORE DELETE ON authority_source_authentications BEGIN
    SELECT RAISE(ABORT, 'authority source authentications are immutable');
END;

CREATE TRIGGER authority_approvals_no_update
BEFORE UPDATE ON authority_approvals BEGIN
    SELECT RAISE(ABORT, 'authority approvals are immutable');
END;

CREATE TRIGGER authority_approvals_no_delete
BEFORE DELETE ON authority_approvals BEGIN
    SELECT RAISE(ABORT, 'authority approvals are immutable');
END;

CREATE TRIGGER protocol_identities_no_delete
BEFORE DELETE ON protocol_identities BEGIN
    SELECT RAISE(ABORT, 'protocol identities are immutable');
END;
