# Sworn v0.2.0

Sworn v0.2.0 is a small deterministic delivery kernel for autonomous software
work. Its bounded `sworn run` advances one already planned and active work item
through a native Codex builder, exact local checks, and atomic admission to
`reviewable`, while recovering external effects safely and exposing a truthful
board. It does not yet obtain an independent verifier verdict or update the
target ref.

Sworn's architectural target is the complete autonomous loop through fresh
independent verification and safe integration.

v0.2.0 is the first packaged milestone from Sworn's greenfield architectural
v1 kernel. In this repository, **v1** names the kernel architecture and the
first generation of stable schema and reference identifiers; it is not the
binary's SemVer major. Identifiers such as `sworn-run-config-v1` therefore
remain v1 in the v0.2.0 package.

The implementation was developed on the disconnected `release/v1.0.0`
construction branch. Sworn v0 remains available as protected archaeology at
`legacy/v0` and `legacy/v0-final`; it is not an implementation base for this
code line.

The v0.2.0 foundation establishes the architectural v1 trust boundary:

- a Baton v1.0.0-rc.1 snapshot pinned to commit
  `dd41dcc8c46def2f8b7b86a4f9acd26aeb486667`;
- checksum verification for every embedded protocol file;
- a `sworn version` command that reports the snapshot digest;
- one transactional control store and pure reducer;
- exact local Git candidate primitives;
- a contained Linux subprocess boundary with measured writable export;
- an exact Codex builder profile with attempt-bound publication and recovery;
- current-authorized, restart-recoverable local checks;
- one bounded `sworn run` path from an active work item to `reviewable`; and
- release-line CI.

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
The Codex control process authenticates only through a dedicated, file-backed
ChatGPT login managed by the Codex CLI. Sworn binds that single `auth.json`
read-write for the trusted outer process so token refresh can persist; the
model-directed tool sandbox has neither network access nor read access to the
fixed Codex home. Sworn accepts no Platform API key and has no authentication
fallback.

`sworn run <run> [<work>] --config <absolute-path>` acquires exclusive Store
ownership, completes the recovery barrier, and advances exactly the selected
current work item through builder, ordered local checks, and deterministic
admission. Stable command identities make the same work attempt convergent
across restart. It does not initialize or activate a delivery, poll for work,
advance another work item, obtain an independent verdict, or update a target.
Historical approval remains provenance rather than a standing execution
permit, and `reviewable` is not a verdict or `PASS`.

This is a bounded production vertical, not yet the autonomous product loop. The
v0.3.0 development line now contains an internal Store-owned verifier effect,
native memoryless Codex verifier worker, strict execution receipt, and
verdict-routing lifecycle. The adapter is not yet composed into public
`sworn run`. There is still no public initializer, bounded repair policy,
integration edge, or scheduler. Its Store must already contain an exact planned
and activated delivery. See [Running the bounded vertical](docs/run.md), [Exact
local candidate](docs/exact-candidate.md), and [Independent verifier protocol
and Store lifecycle](docs/verifier-protocol.md).

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
CGO_ENABLED=0 SWORN_REQUIRE_LINUX_EXECUTOR=1 go test ./internal/executor
SWORN_CODEX_BINARY=/absolute/path/to/codex \
  SWORN_REQUIRE_CODEX_BOUNDARY=1 \
  go test -run 'TestReal(CodexCLIBoundaryFeasibility|PinnedCodexVerifierBoundary)$' ./internal/adapter
```

The final two test commands are fail-if-unavailable real containment suites. The
Codex proofs require an exact static CLI and exercise the builder and verifier
profiles against a local scripted Responses endpoint while mounting synthetic
file-backed ChatGPT state, without making a provider model call. They prove that
a real nested tool cannot read the mounted authentication file; the verifier
proof additionally exercises its exact read-only candidate and review inputs
without admitting builder context. Both suites require the Linux capability
floor described in the executor document. Ordinary tests skip those integration
cases when their host capability or explicit binary is unavailable.

That scripted proof validates the credential mount and nested denial, but its
test provider uses a separate synthetic bearer and does not prove that a model
request used the mounted ChatGPT state. A real `sworn run` uses the built-in OpenAI
provider through the operator's dedicated Codex CLI ChatGPT login and consumes
that account's Codex usage. It never reads a Platform API key. No live-provider
delivery is part of the ordinary test suite. On 2026-07-21, the opt-in release
smoke test passed at the built-process boundary with `gpt-5.4`: one live turn
created the exact candidate, passed its local check, and reached `reviewable`;
a second process invocation converged without another model turn. The accepted
adapter currently requires the exact 304,169,008-byte
`codex-cli 0.145.0-alpha.18` static binary described in [ADR
0007](docs/adr/0007-native-agent-boundary.md); Sworn does not yet install or
acquire it. Authentication setup and rotation are documented in [Running the
bounded vertical](docs/run.md) and [ADR
0009](docs/adr/0009-codex-cli-managed-chatgpt-authentication.md).

See the [v0.2.0 release notes](docs/releases/v0.2.0.md), [ADR
0001](docs/adr/0001-greenfield-v1-kernel.md) for ownership boundaries, and [the
implementation sequence](docs/roadmap.md) for the walking skeleton and v0.3.0
direction.
