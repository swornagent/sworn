-- v2 stored an attempt's observation digest but threw the marshaled
-- observation away, so a failed dispatch left no readable evidence. v3 adds
-- the body and its honest truncation mark.
--
-- The two ADD COLUMN statements must stay in this exact order and text:
-- ALTER TABLE ADD COLUMN splices each definition inline into the stored
-- CREATE TABLE text, so the shipped schema.sql authors the attempts table in
-- the same spliced layout and a migrated journal fingerprints byte-identically
-- to a fresh one. Historical rows read the new columns as NULL / 0 and stay
-- distinguishably absent rather than corrupt.
ALTER TABLE attempts ADD COLUMN observation_body BLOB;
ALTER TABLE attempts ADD COLUMN observation_partial INTEGER NOT NULL DEFAULT 0 CHECK (observation_partial IN (0,1));

PRAGMA user_version = 3;
