---
title: S06-implementer
description: Drive the tool loop to implement a spec and write a proof bundle from live repo state.
---

# Slice: `S06-implementer`

## User outcome

Given a spec, the engine drives the agentic tool loop to implement it, then writes
a **proof bundle** (diff + test output + reachability note) from live repo state,
and stops at `implemented` — it never certifies its own work.

## Entry point

Internal `implement` package, invoked by the run-loop (S07).

## In scope

- Implementer role: uses the S03 tool loop + the embedded implementer prompt (S04)
  to implement against a spec.
- Generate `proof.md` from live repo state (`git diff`, test output, reachability),
  NOT from the model's narration.
- Stop at state `implemented` (hand to the verifier).

## Out of scope

- Verification (S01/S02); run orchestration / retry (S07).

## Planned touchpoints

- `internal/implement/`

## Acceptance checks

- [ ] From a spec, code changes land and `proof.md` is written from live repo state.
- [ ] The proof's "files changed" equals `git diff --name-only` (not model claims).
- [ ] The slice ends at `implemented` — no self-certification to `verified`.

## Required tests

- **Unit/Integration**: a fake model scripted to implement a trivial spec in a
  temp repo; assert the diff and that `proof.md` is generated from git.

## Risks

- Overclaiming in the proof — proof is generated from git, not narration.

## Deferrals allowed?

No.
