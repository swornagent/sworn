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
- an exact Codex builder profile with attempt-bound publication and recovery;
- current-authorized, restart-recoverable local checks;
- one bounded `sworn run` path from an active work item to `reviewable`; and
- v1-specific CI.

The intended command surface is `init`, `run`, `revise`, `retry`, `board`,
`integrate`, `doctor`, and `version`. Unimplemented commands fail explicitly;
there are no compatibility shims.

## Current implementation

The transactional control core, exact-plan authority boundary, exact local
candidate path, and contained Linux executor are composed behind one bounded
production command. Together they provide
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

The sole production adapter accepts one exact static Codex CLI profile. Store
prevalidates the exact claim and process configuration before agent execution;
the worker prepares an unpublished candidate; Store binds the typed result,
publishes candidate and attempt refs, and only then commits success. Restart
either completes that bound result or requeues an unbound attempt only after a
Store-bound composite proof of absent publication and complete writable
cleanup. Each pending local check separately requires freshly resolved current
authority and an exact Store-issued execution capability. Interrupted checks
likewise converge from a bound result or from an attempt-bound proof that the
content process is quiescent and its private materialization has been removed.

`sworn run <run> [<work>] --config <absolute-path>` acquires exclusive Store
ownership, completes the recovery barrier, and advances exactly the selected
current work item through builder, ordered local checks, and deterministic
admission. Stable command identities make the same work attempt convergent
across restart. It does not initialize or activate a delivery, poll for work,
advance another work item, obtain an independent verdict, or update a target.
Historical approval remains provenance rather than a standing execution
permit, and `reviewable` is not a verdict or `PASS`.

This is a bounded production vertical, not the autonomous product loop. There
is no public initializer, verifier, verdict routing, bounded repair policy,
integration edge, or scheduler. Its Store must already contain an exact planned
and activated delivery. See [Running the bounded vertical](docs/run.md),
[Exact local candidate](docs/exact-candidate.md), and
[ADR 0008](docs/adr/0008-builder-to-reviewable-production-vertical.md).

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
go build ./cmd/sworn
go run ./cmd/sworn version --json
SWORN_REQUIRE_LINUX_EXECUTOR=1 go test ./internal/executor
SWORN_CODEX_BINARY=/absolute/path/to/codex \
  SWORN_REQUIRE_CODEX_BOUNDARY=1 \
  go test -run TestRealCodexCLIBoundaryFeasibility ./internal/adapter
```

The final two test commands are fail-if-unavailable real containment suites. The
Codex proof requires an exact static CLI and exercises it against a local
scripted Responses endpoint without making a provider model call. Both require
the Linux capability floor described in the executor document. Ordinary tests
skip those integration cases when their host capability or explicit binary is
unavailable.

That scripted proof is token-free. A real `sworn run` uses the built-in OpenAI
provider and can consume billable model tokens; no live-provider delivery is
part of the ordinary test suite. The accepted adapter currently requires the
exact 304,169,008-byte `codex-cli 0.145.0-alpha.18` static binary described in
[ADR 0007](docs/adr/0007-native-agent-boundary.md); Sworn does not yet install or
acquire it.

See [ADR 0001](docs/adr/0001-greenfield-v1-kernel.md) for ownership boundaries
and [the implementation sequence](docs/roadmap.md) for the walking skeleton.
