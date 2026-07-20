# Sworn v1

Sworn is a small deterministic delivery engine for autonomous software work. It
turns an approved Baton plan into an exact Git candidate, obtains a fresh
independent verdict, recovers external effects safely, and exposes a truthful
board.

The `release/v1.0.0` branch is a greenfield, pre-alpha implementation with
disconnected history. Sworn v0 remains available as protected archaeology at
`legacy/v0` and `legacy/v0-final`; it is not an implementation base for this
branch.

The v1 foundation establishes the trust boundary:

- a Baton 1.0 release-candidate snapshot pinned to commit
  `732ba47672e12edb55494d120bb7325850187643`;
- checksum verification for every embedded protocol file;
- a `sworn version` command that reports the snapshot digest;
- one transactional control store and pure reducer;
- exact local Git candidate primitives;
- a contained Linux subprocess boundary with measured writable export; and
- v1-specific CI.

The intended command surface is `init`, `run`, `revise`, `retry`, `board`,
`integrate`, `doctor`, and `version`. Unimplemented commands fail explicitly;
there are no compatibility shims.

## Current implementation

The transactional control core, exact-plan authority boundary, exact local
candidate path, and contained Linux executor are implemented internally.
Together they provide
atomic command/event/effect commits, content-addressed records, unknown-effect
reconciliation, live Git measurement, plain workspaces, exact single-parent
candidates, immutable or fresh writable executor staging, default-denied
networking, finite live resource and retained-output ceilings, process-tree
cleanup, quiescent measured workspace export, typed lease-bound effect results,
an explicit content-bound local-check runtime, and one ordered, serially claimed
plan-derived local-check batch after a succeeded builder. Real-boundary tests
prove both staged runtime execution and the writable-export handoff into exact
Git candidate capture.

No mutating command is exposed by the CLI yet. The internal check-dispatch edge
requires exact plan, policy, definition, historical approval, builder-journal,
and process-configured runtime agreement in one transaction, but it is not a
current authority permit. No command service, native agent adapter, or autonomous
claim loop can execute it. These remain internal primitives rather than a
delivery loop. See
[Exact local candidate](docs/exact-candidate.md) and
[Contained executor](docs/contained-executor.md).

SQLite is the sole Go production dependency. Linux execution relies on the
host's systemd user manager, cgroup v2, and Bubblewrap; it fails closed when that
capability floor is absent. There is no ORM, workflow framework, provider SDK,
LangChain/LangGraph runtime, or telemetry control path.

## Development

Go 1.26.5 or newer is required so release binaries include the current Go 1.26
security fixes.

```sh
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/sworn version --json
SWORN_REQUIRE_LINUX_EXECUTOR=1 go test ./internal/executor
```

The last command is the fail-if-unavailable real containment suite; it requires
the Linux capability floor described in the executor document. Ordinary tests
skip those integration cases on hosts that cannot run user services.

See [ADR 0001](docs/adr/0001-greenfield-v1-kernel.md) for ownership boundaries
and [the implementation sequence](docs/roadmap.md) for the walking skeleton.
