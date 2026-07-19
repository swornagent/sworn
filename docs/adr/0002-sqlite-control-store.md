# ADR 0002: SQLite control store and driver

- Date: 2026-07-19
- Status: accepted for the transactional-core milestone

## Decision

Use SQLite through Go's `database/sql` package and pin
`modernc.org/sqlite` v1.54.0 as the only production dependency in the first
kernel milestone. The blank driver import belongs only in `internal/store`.
No domain package may import driver-specific APIs.

The store uses one database file and one serialized connection. Every
connection enables foreign keys, disables double-quoted string fallback and
trusted schema behavior, waits for a bounded busy timeout, uses rollback-journal
`DELETE` mode, and keeps `synchronous=FULL`. The file carries a Sworn-specific
SQLite `application_id` and a forward-only `user_version`; an unknown non-zero
application ID or newer schema version fails closed.

The schema stores commands, immutable events, current run snapshots, pending
and resolved effects, canonical records, raw artifacts, and write-once Baton
submission, work-attempt, approval, and run identity bindings. A state
transition, its event, command result, and newly pending effects commit in one
transaction. Effect execution happens only after commit. Mutable effect status
is the journal of external reality; it is not delivery state.

WAL mode is not enabled initially. Sworn has one command writer, board reads are
small, and rollback mode avoids another persistent file and checkpoint lifecycle
before measurements show a need. The driver remains configurable only inside
the store, not through user-facing correctness switches.

## Why this driver

The driver is a stable v1 module, implements `database/sql`, is CGO-free, and
supports Sworn's initial Linux, macOS, and Windows targets. CGO would complicate
reproducible cross-platform CLI releases. A WebAssembly-hosted SQLite driver
would add another runtime boundary without helping this low-throughput control
plane. Reusing a proven dependency is acceptable; no v0 store code or schema is
being ported.

SQLite documents atomic transactions across crashes and power loss, subject to
the filesystem honoring its durability primitives:
<https://www.sqlite.org/atomiccommit.html>. The selected pragmas are documented
at <https://www.sqlite.org/pragma.html>. Driver documentation is at
<https://pkg.go.dev/modernc.org/sqlite>.

## Failure and ownership behavior

- Open, migration, integrity, locking, or commit errors are returned. There is
  no in-memory or JSON-file fallback.
- A command rejection is durable and idempotently replayable; infrastructure
  errors are not converted into domain results.
- A process interruption after an effect starts produces `unknown`, never an
  automatic retry. Only external reconciliation may prove success, failure, or
  that no effect occurred.
- Every claim mints a store-local, attempt-bound lease. Completion uses that
  lease in an effect/owner/attempt compare-and-swap; a stale worker cannot close
  a later attempt after `not_applied` reconciliation and reclaim.
- Reconciliation names the exact unknown attempt it observed, so a delayed
  reconciler cannot resolve a later retry of the same effect.
- The derived effect ID is the only future Baton invocation run ID. Generic
  effect receipt JSON is not journal provenance until a kind-specific receipt
  validator is implemented.
- Commands, events, canonical records, and artifacts are append-only at the SQL
  boundary. Only run snapshots and effect-journal status are mutable.
- The board reads committed run snapshots and cannot write or migrate.

## Removal cost

Domain and reducer packages depend only on Go types. Store tests exercise the
public behavior through `database/sql`; replacing the driver requires changing
the isolated import and any DSN details, then passing the same migration,
atomicity, integrity, and recovery suite. No driver error or connection type is
part of the engine API.
