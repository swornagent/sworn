# Sworn v1

Sworn is a small deterministic delivery engine for autonomous software work. It
turns an approved Baton plan into an exact Git candidate, obtains a fresh
independent verdict, recovers external effects safely, and exposes a truthful
board.

This `v1` branch is a greenfield, pre-alpha implementation with disconnected
history. Sworn v0 remains available as protected archaeology at `legacy/v0` and
`legacy/v0-final`; it is not an implementation base for this branch.

The first commit establishes only the trust boundary:

- a Baton 1.0 release-candidate snapshot pinned to commit
  `732ba47672e12edb55494d120bb7325850187643`;
- checksum verification for every embedded protocol file;
- a minimal `sworn version` command that reports the snapshot digest; and
- v1-specific CI with no production dependencies.

The intended command surface is `init`, `run`, `revise`, `retry`, `board`,
`integrate`, `doctor`, and `version`. Unimplemented commands fail explicitly;
there are no compatibility shims.

## Current implementation

The transactional control core is under review. It contains one pure reducer,
one forward-only SQLite schema, atomic command/event/effect commits,
content-addressed records and artifacts, explicit unknown-effect
reconciliation, and a read-only `board` command. No mutation command is exposed
by the CLI yet; the internal activation transition accepts only the digest of an
authority receipt whose cryptographic resolution is a later gated milestone.

SQLite is the sole production dependency. There is no ORM, workflow framework,
provider SDK, LangChain/LangGraph runtime, or telemetry control path.

## Development

Go 1.26.5 or newer is required so release binaries include the current Go 1.26
security fixes.

```sh
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/sworn version --json
```

See [ADR 0001](docs/adr/0001-greenfield-v1-kernel.md) for ownership boundaries
and [the implementation sequence](docs/roadmap.md) for the walking skeleton.
