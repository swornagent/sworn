# ADR 0001: Greenfield Sworn v1 kernel

- Date: 2026-07-19
- Status: accepted

## Context

Sworn v0 accumulated competing state stores, orchestration paths, provider and
agent runtimes, and manual-process prompts. Important safety rules existed, but
ownership was split across mutable files, databases, Git refs, prompts, and
process-local state. Repairing those seams while retaining compatibility would
carry v0's proof burden into a rewrite.

Before this branch was created, every local and remote ref, linked worktree,
stash, dirty path, selected runtime file, and refutation fixture was captured in
a private external archive and restored in scratch clones. Canonical v0 commit
`303dc1d2fc86b3775ef3cb00961e70c49bec87bf` is retained by protected branch
`legacy/v0` and protected annotated tag `legacy/v0-final`.

## Decision

Build v1 from an orphan branch in a fresh clone of the existing repository.
Preserve repository identity and issues, but share no source history with v0.
Port invariants and black-box failure fixtures, not production packages.

Sworn is:

> A small deterministic delivery engine that turns an approved Baton plan into
> exact candidates, obtains fresh independent verdicts, recovers external
> effects safely, and exposes a truthful board.

The kernel has one command service, one reducer, one transactional SQLite
control store, one external-effect journal, and one contained subprocess
boundary. Its package ownership is deliberately small:

```text
cmd/sworn/          thin command adapters
internal/protocol/  embedded Baton records, validation, canonical digests
internal/engine/    command service, reducer, retry policy
internal/store/     transactional commands, events, effects, records
internal/effects/   effect execution and reconciliation
internal/repo/      Git workspaces, candidates, compare-and-swap integration
internal/workspace/ shared plain-tree manifest and staging
internal/executor/  contained subprocess and process-lifetime boundary
internal/producer/  measured local check receipts and evidence facts
internal/adapter/   runner argv construction and result decoding
internal/policy/    authority, checks, and assurance selection
internal/board/     read-only projection
internal/config/    runners and local restrictions
```

There is no independent scheduler, router, supervisor, mutable state package,
provider/model layer, or in-process coding-agent loop. Native agent CLIs own
model and tool evolution. Sworn owns their authority, containment, immutable
inputs, process lifetime, and exact outputs.

The control database is authoritative. OpenTelemetry may later receive a
bounded asynchronous projection of exact engine records, disabled by default.
Export failure cannot block, retry, resume, or alter a run. LangSmith may be a
dogfood or evaluation destination; LangChain and LangGraph do not own kernel
semantics. Sensitive content is not exported by default.

The embedded protocol is pinned to Baton commit
`732ba47672e12edb55494d120bb7325850187643`, admitted after 7 strict-JSON cases,
2 canonicalization vectors, 46 schema fixtures, and 99 cross-record cases
passed. The 18 real-boundary engine cases remain explicitly unclaimed until
they pass through the real Sworn binary.

## Required invariants

1. Schema validity is not approval. An authenticated authority source must bind
   the exact canonical plan digest, repository, target, grants, assurance, and
   authorizer before activation.
2. Each accepted command has an idempotency identity and expected revision.
   The state transition and pending effect are committed atomically.
3. Effects execute outside the transaction and bind one immutable typed result.
   Unknown completion is reconciled from that result or an explicit external
   determination before retry.
4. Builder and verifier receive immutable dispatches. A verdict is fresh only
   when it binds the dispatched candidate, submission, policy, and verifier
   identity.
5. Candidate facts come from Git objects, not agent claims. Integration uses
   compare-and-swap against the approved target and exact candidate tree.
6. The board is derived solely from authoritative records. It never repairs or
   mutates control truth.
7. Unsupported or ambiguous capability fails before runner dispatch. No path
   silently weakens assurance or manufactures a verdict.

## Walking-skeleton limit

The first executable delivery supports one work contract, Standard assurance
without packs, locally producible evidence, serial execution, and a direct
fast-forward target. Multiple units and arbitrary selected assurance packs must
use the same reducer before a public release. Live evidence, parallel work, and
PR/squash/merge-commit integration remain later capabilities.

If this serial kernel needs dozens of packages or materially exceeds 8–10k
production lines, implementation stops for architecture review.

## Consequences

There is no v0 compatibility surface and no migration reader in the kernel.
Features arrive only through the single reducer and effect model. Dependencies
must justify their ownership and failure behavior. Recovery, authority, and
real-binary conformance proofs gate unattended use; type-checking and happy-path
tests alone do not.
