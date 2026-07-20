-- Earlier schemas accepted submission identities through the removed
-- structural PutSubmission boundary. They are not admission proof and must not
-- be inherited by the atomic admission owner.
DROP TRIGGER submission_records_no_update;
DROP TRIGGER submission_records_no_delete;
DROP TABLE submission_records;

CREATE UNIQUE INDEX runs_submission_identity ON runs(run_id, delivery_id);
CREATE UNIQUE INDEX commands_submission_identity ON commands(command_id, run_id);

CREATE TABLE submission_records (
    submission_id TEXT PRIMARY KEY,
    delivery_id TEXT NOT NULL,
    work_id TEXT NOT NULL,
    attempt INTEGER NOT NULL CHECK (attempt >= 1 AND attempt <= 9007199254740991),
    digest TEXT NOT NULL UNIQUE REFERENCES records(digest),
    run_id TEXT NOT NULL,
    command_id TEXT NOT NULL UNIQUE,
    UNIQUE (delivery_id, work_id, attempt),
    FOREIGN KEY (run_id, delivery_id) REFERENCES runs(run_id, delivery_id),
    FOREIGN KEY (command_id, run_id) REFERENCES commands(command_id, run_id)
) STRICT;

CREATE TRIGGER submission_records_require_admission
BEFORE INSERT ON submission_records
WHEN NOT EXISTS (
    SELECT 1 FROM commands
    WHERE command_id = NEW.command_id AND run_id = NEW.run_id
      AND kind = 'submission.admit' AND outcome = 'applied'
) BEGIN
    SELECT RAISE(ABORT, 'submission identity requires an applied admission command');
END;

CREATE TRIGGER submission_records_no_update
BEFORE UPDATE ON submission_records BEGIN
    SELECT RAISE(ABORT, 'submission identities are immutable');
END;

CREATE TRIGGER submission_records_no_delete
BEFORE DELETE ON submission_records BEGIN
    SELECT RAISE(ABORT, 'submission identities are immutable');
END;
