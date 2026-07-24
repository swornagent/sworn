# Sworn greenfield-kernel engineering rules

This code line is a greenfield implementation. Never merge or copy v0
production packages into it. Port an invariant only when a focused test states
the failure it prevents. The architectural **v1** label and `*-v1` schema or
reference identifiers are independent of package SemVer; the first packaged
milestone from this line is v0.3.0.

Sworn is a small deterministic delivery engine. Native coding-agent CLIs own
model interaction, tools, and context. Sworn owns authority, isolation, exact
Git candidates, durable state transitions, independent verification, recovery,
and the truthful board.

## Non-negotiable boundaries

- The embedded Baton snapshot is the protocol contract. Do not weaken or
  reinterpret it in adapters.
- One transactional SQLite store will own command, event, effect, and record
  truth. The board is a read-only projection, never another state store.
- One command service and reducer will own transitions. External effects are
  journaled, idempotent, and reconciled after interruption.
- Git facts, immutable record digests, expected revisions, and compare-and-swap
  checks bind every candidate and integration.
- Coding agents run only as contained subprocesses through one executor.
  Provider SDKs and in-process agent loops do not belong in the kernel.
- Telemetry is an optional, bounded, lossy projection. It cannot control a run
  or become required for recovery.
- Unknown capabilities, states, fields, verdicts, authority, or recovery facts
  fail closed before an external effect.

Keep packages aligned with the architecture record. New production dependencies
need a short ADR explaining ownership, failure behavior, and removal cost.
Prefer tests at invariant and process boundaries over mocks of internal wiring.
The Baton authority root `.baton/releases` is never a product, model, check,
candidate, or package input. Keep it out of generic walkers and build identity.
Run `GOFLAGS=-buildvcs=false go test ./...`,
`GOFLAGS=-buildvcs=false go vet ./...`, and the product-only formatting check
before committing. Official binaries use `-buildvcs=false -trimpath`.
