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

`.sworn/records` is control authority only. It is never a product, model,
check, workspace, candidate, build or package input. Product identity excludes
it while preserving exact Git provenance separately.

Before committing, run:

```sh
GOFLAGS=-buildvcs=false go test -count=1 \
  ./cmd/sworn ./internal/... ./tools/...
GOFLAGS=-buildvcs=false go test -count=1 \
  -parallel=1 -timeout=35m ./test/e2e
GOFLAGS=-buildvcs=false go test -count=1 -race \
  ./cmd/sworn ./internal/... ./tools/...
GOFLAGS=-buildvcs=false go vet ./...
gofmt -l ./cmd ./internal ./tools
```

Run the long process tests once and in order. Use the race detector on the
product packages, not on the timing-sensitive end-to-end suite. These are the
same boundaries used by CI.

`gofmt -l` must print nothing. CI enforces formatting as its own step, so a
slice contract that omits this check can pass every gate it declares and
still land red: that is exactly how three driver files reached the release
branch unformatted. A check the repository enforces belongs in the contract
that claims to have checked.

The end-to-end suite runs at the host boundary. A contained role cannot
execute it: nested containment is prevented both by the uid-0 trust check on
`bwrap` (which reads as uid 65534 inside a sandbox) and by `--disable-userns`,
so every nested dispatch returns `ISOLATION_UNAVAILABLE`. A slice contract may
therefore **declare** this check, but must never require a worker or verifier
to run it or to observe its result empirically. Such a contract is
unsatisfiable, and a correct worker will refuse it rather than fabricate
evidence. See ADR 0010.

Official binaries use `CGO_ENABLED=0`, `-buildvcs=false` and `-trimpath`.
