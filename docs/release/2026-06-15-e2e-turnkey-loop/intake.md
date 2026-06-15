---
title: Release intake — 2026-06-15-e2e-turnkey-loop
description: sworn v0.1 — the native-Go end-to-end loop (plan→implement→verify→merge), turnkey self-serve, one zero-dependency binary.
---

# Release Intake: `2026-06-15-e2e-turnkey-loop`

## Release goal

Ship **`sworn` v0.1**: one Go binary that takes a repo and a task and runs the
full loop end-to-end — **implement → verify → (retry/escalate) → gated merge** —
with the verification gate as its trustworthy core. Native Go, **zero runtime
dependencies**. Turnkey self-serve: `brew install sworn` (or `go install`) →
configure one key → `sworn run` → automated development with independent,
fail-closed verification, no plumbing to assemble. The implementer model is
customer-chosen and BYO-key (safe-hosted default); the verifier runs the protocol
SwornAgent owns. "Shipped" = a developer on a clean machine installs the binary,
points it at a repo + a task, and gets a verified, merged change without writing
any spec/proof plumbing themselves.

## Source of truth

- **Stakeholder**: repo owner.
- **Discovery**: complete (prior design sessions). `docs/adr/0001` (one binary,
  embedded protocol, distribution), `docs/adr/0002` (CLI `sworn` + command
  surface). Verifier protocol = the open Baton protocol (embedded).

## Users and their gestures

- **Mass-market developer (self-serve)**: `brew install sworn` → `sworn init` →
  `sworn run --task "<what to build>"` → gets a verified, merged change. Zero
  setup beyond one API key. **Must be 100% turnkey to value.**
- **Operator**: chooses the implementer + verifier models (BYO-key, safe-hosted
  default), entirely via config.

## What the loop must do (the cold-start fix)

The loop **generates the verifier's own inputs** — it writes the spec, drives a
coding model to implement it, produces the proof bundle, then verifies — so the
user never has to assemble the spec/proof plumbing. This is what makes the
verifier valuable to someone who has none of the surrounding machinery.

## Constraints and non-negotiables

- **Native Go, single binary, zero runtime deps** (no bash/jq/curl shell-out).
- **Fail-closed** verdict polarity throughout; unverified work never merges.
- **Turnkey self-serve** for mass-market: sensible zero-config defaults; the only
  required input is one API key.
- **Customer owns models + key; SwornAgent owns the protocol.** Safe-hosted
  default model (trusted jurisdiction); never bless a non-trusted-hosted default.
- **Public-safe docs** — release specs are technical only.

## Adjacent / out of scope (Rule 2 deferrals)

- **`sworn top` TUI (Bubble Tea)**: deferred to a later release. Why: the loop
  runs headless first; the TUI is observability over it. Tracking: roadmap.
  Acknowledged: 2026-06-15.
- **Full planner / multi-slice decomposition**: v0.1 takes a single task/spec;
  rich `sworn plan` decomposition is a later release.
- **Enterprise tier** (private ledger, SSO, outcome billing, sovereignty config,
  high-touch onboarding): post-MVP; enterprise gets handholding, not self-serve.
- **GitLab/Bitbucket/Azure adapters, provenance/license gate, web UI, tracker/
  actor config, telemetry/data-moat**: roadmap.
- **Standalone CI-gate (verify-on-top of an existing pipeline)**: a secondary
  adoption mode for teams that already produce specs; not the v0.1 lead.

## Decisions made during planning

### 2026-06-15 — MVP = the E2E loop, not the standalone gate

- **Context**: the gate needs a `spec→diff→proof` triple as input, which a normal
  repo/PR does not have — so the gate alone serves only the savvy few.
- **Decision**: the MVP is the end-to-end loop that generates its own inputs; the
  gate is its core. Turnkey self-serve for mass-market.

### 2026-06-15 — Native Go, not a bash bridge

- **Options**: package the existing bash loop now (fast, franken) vs build native
  in Go first (clean, slower).
- **Decision**: native Go, one zero-dep binary from the start. Cost: months;
  the full agentic implementer driver is a big build. Accepted.

### 2026-06-15 — Track decomposition

- 4 tracks: T1 engine (model client + agentic tool loop + verify core + prompts),
  T2 orchestration (state/git + implementer + run loop + gated merge), T3 turnkey
  UX (init/config + distribution), T4 proof (benchmark + dogfood). T2/T4 depend on
  T1; T3 mostly parallel. `cmd/sworn/main.go` dispatch is a documented shared file
  (each subcommand in its own file; only the dispatch switch is shared).

## Proposed slice decomposition

- `S01-verifier-core` — fail-closed verdict contract + first-pass + model stub (DONE → implemented)
- `S02-oai-model-client` — OpenAI-compatible chat client (BYO-key, safe-hosted default, cost)
- `S03-agentic-tool-loop` — read/write/edit/bash/grep/glob tool loop over the model client
- `S04-embed-baton-prompts` — embed planner/implementer/verifier prompts via go:embed
- `S05-state-and-git` — slice state machine (status.json) + git branch/commit ops
- `S06-implementer` — drive the tool loop to implement against a spec + write the proof bundle
- `S07-run-loop` — `sworn run`: implement→verify→retry/escalate→gated merge orchestration
- `S08-init-config` — `sworn init` + turnkey zero-config defaults + BYO-key model config
- `S09-distribution` — goreleaser single binary + Homebrew tap + container (GHCR)
- `S10-benchmark-dogfood` — model × hosting-jurisdiction × cost × pass-rate benchmark + real-repo E2E dogfood

## Open questions

- Safe-default model identity — resolved by the S10 benchmark.
