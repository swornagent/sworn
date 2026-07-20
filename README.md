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
- a contained Linux subprocess boundary with measured writable export;
- an internal native builder with attempt-bound publication and recovery; and
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
plan-derived local-check batch after a succeeded builder. An intent-only atomic
admission edge now revalidates the exact plan, authenticated historical
authority, builder/check journal, lease-bounded chronology, runtime, snapshot,
artifact closure, and retained Git candidate before committing one canonical
Baton submission and exposing `reviewable`. Real-boundary tests prove both
staged runtime execution and the writable-export handoff into exact Git
candidate capture.

The native builder now closes one real vertical path internally. Store
prevalidates the exact claim and process configuration before agent execution;
the worker prepares an unpublished candidate; Store binds the typed result,
publishes candidate and attempt refs, and only then commits success. Restart
either completes that bound result or requeues an unbound attempt only after a
Store-bound composite proof of absent publication and complete writable
cleanup. A real composition test carries that result through exact checks and
atomic reviewable admission.

The executor also admits one digest-pinned input as a direct entrypoint and a
separately double-gated nested sandbox. An opt-in real-Codex proof uses those
capabilities to separate the networked agent control plane from its
network-denied, credential-free tool process; no production agent adapter is
connected yet.

No mutating command is exposed by the CLI yet. Historical approval remains
provenance rather than a current execution permit, and reviewable is not a
verdict or `PASS`. The narrow internal composition service does not claim work
or own a loop. No public controller, agent-CLI adapter, verifier, or autonomous
claim loop can execute these internal edges yet. They remain trusted kernel
primitives rather than a delivery loop. See
[Exact local candidate](docs/exact-candidate.md) and
[ADR 0005](docs/adr/0005-native-builder-recovery.md).

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
SWORN_CODEX_BINARY=/absolute/path/to/codex \
  SWORN_REQUIRE_CODEX_BOUNDARY=1 \
  go test -run TestRealCodexCLIBoundaryFeasibility ./internal/adapter
```

The final two commands are fail-if-unavailable real containment suites. The
Codex proof requires an exact static CLI and exercises it against a local
scripted Responses endpoint without making a provider model call. Both require
the Linux capability floor described in the executor document. Ordinary tests
skip those integration cases when their host capability or explicit binary is
unavailable.

See [ADR 0001](docs/adr/0001-greenfield-v1-kernel.md) for ownership boundaries
and [the implementation sequence](docs/roadmap.md) for the walking skeleton.
