---
title: 'S41-build-bin-target — canonical `make build` to bin/sworn, run from repo root'
description: 'Adds a Makefile with a canonical build target that outputs bin/sworn (already gitignored) and documents the build + run-from-repo-root convention in AGENTS.md, so sworn run-state (.sworn/, docs/release/run-*) stops cluttering cmd/sworn/.'
---

# Slice: `S41-build-bin-target`

# User outcome

A contributor (or the loop) runs **`make build`** and gets `./bin/sworn`, then runs it
from the repo root — so sworn's run-state (`.sworn/sworn.db`, `docs/release/run-*`) lands
predictably at the repo root (where it is gitignored) instead of being written under
`cmd/sworn/` every time `go run .` is invoked there. The recurring `cmd/sworn/.sworn/` and
`cmd/sworn/docs/release/run-*` clutter across track worktrees stops appearing.

## Entry point

`make build` at the repo root → produces `./bin/sworn`.

## Background

Agents and tests have been running sworn with cwd = `cmd/sworn/` (e.g. `go run .`), which
writes the state DB and run-scratch dirs relative to cwd, littering `cmd/sworn/` in every
track worktree. `/bin/` is already gitignored; what's missing is a canonical, documented
build/run path that keeps invocation at the repo root.

## In scope

- Add a `Makefile` at the repo root with at least:
  - `build` → `go build -o bin/sworn ./cmd/sworn`
  - `test`  → `go test ./...`
  - `vet`   → `go vet ./...`
- Document the canonical build (`make build`) and the run-from-repo-root convention in a
  **new `docs/build.md`** (so sworn is run as `./bin/sworn` from the repo root and run-state
  stays at the repo root, gitignored, not under `cmd/sworn/`). A new file is used rather than
  `AGENTS.md` because `AGENTS.md` is owned by S21/T3 (rewrite) and S22/T4 (splice detection);
  the AGENTS.md pointer to `docs/build.md` is left to S21/S33.

## Out of scope

- Changing sworn's state-dir resolution in code (`cmd/sworn` / `internal/config`) so it
  always writes to a fixed home regardless of cwd — **deferred** (Rule 2). Why: that is a
  production-code change in shared files; the Makefile + doc convention fixes the observed
  clutter without it. Tracking: follow-up slice if cwd-relative state proves insufficient.
  Ack: Coach, 2026-06-21.
- Editing the reachability smoke-step guidance in `internal/prompt/*` — **deferred** to
  S33-spec-template-hardening (same track) to avoid colliding with T3's prompt ownership.
  Why/Tracking/Ack: recorded as a touchpoint note; S33 folds in the "build via make, run
  ./bin/sworn from root" smoke-step wording.

## Planned touchpoints

- `Makefile` (new)
- `docs/build.md` (new)

## Acceptance checks

- [ ] `make build` produces an executable `./bin/sworn`; `git status` stays clean
  (bin/ is gitignored)
- [ ] `make test` and `make vet` run the suite / vet across `./...`
- [ ] `docs/build.md` documents `make build` and the run-from-repo-root convention
- [ ] Running `./bin/sworn <a command that writes state>` from the repo root writes
  `.sworn/` / run-scratch at the repo root, **not** under `cmd/sworn/`

## Required tests

- No Go unit test (build tooling). **Reachability artefact**: paste in `proof.md` the
  output of `make build && ls -la bin/sworn`, plus a `git status --porcelain` showing the
  tree clean after a build.

## Risks

- If CI already builds via a different path, keep the Makefile target consistent with the
  CI workflow (check `.github/workflows/`); do not break the existing CI build invocation.

## Deferrals allowed?

Yes, with Rule 2 compliance — the two Out-of-scope items above carry why / tracking / ack.
