# Sworn v0.3 engineering rules

Sworn is a small deterministic delivery engine for Baton. Native coding-agent
CLIs and provider adapters own model interaction. Sworn owns authority,
isolation, exact Git candidates, durable transitions, recovery and the
truthful board.

The v0.3 source tree has six production ownership areas:

- `cmd/sworn`: CLI and process lifetime;
- `internal/baton`: the exact embedded Baton package and action authority;
- `internal/runtime`: command service, scheduling and recovery;
- `internal/journal`: durable commands, effects, receipts and events;
- `internal/gitx`: sanitized Git facts and compare-and-set mutations; and
- `internal/driver`: one role-neutral invocation and submission contract.

The v0.2 packages are archaeology. Do not copy them into this line. Port an
invariant only when a focused test states the failure it prevents. Add a
dependency only with the behavior that consumes it and a clear removal cost.

## Non-negotiable boundaries

- The embedded Baton snapshot is the protocol contract. Node and Baton's
  JavaScript reference are development oracles only.
- Planner, Implementer, Captain and Verifier may be model-backed. Merge is
  deterministic, engine-owned and never dispatched to a model.
- One command service and reducer own transitions. Effects are journaled,
  idempotent and reconciled after interruption.
- Git facts, exact record digests and compare-and-set checks bind candidates
  and integration.
- Drivers are role-neutral. Every model-facing invocation names its driver and
  model explicitly; there are no model defaults or fallbacks.
- Telemetry is optional, bounded and lossy. It never controls or recovers a
  run.
- Unknown state, capability, authority, evidence or recovery facts fail closed
  before an external effect.

`.baton/releases` is control authority only. It is never a product, model,
check, workspace, candidate, build or package input. Product identity excludes
it while preserving exact Git provenance separately.

Before committing, run:

```sh
GOFLAGS=-buildvcs=false go test ./...
GOFLAGS=-buildvcs=false go test -race ./...
GOFLAGS=-buildvcs=false go vet ./...
```

Official binaries use `CGO_ENABLED=0`, `-buildvcs=false` and `-trimpath`.
