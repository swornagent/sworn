---
title: 'S40-memory-test-hygiene — memory tests use t.TempDir(), leave no stray tree artefacts'
description: 'Rewrites the internal/memory tests so they write all fixtures and the fake embedding server under t.TempDir() instead of into the worktree, so `go test ./internal/memory/...` leaves git status clean and never trips the Gate -1 cleanliness check.'
---

# Slice: `S40-memory-test-hygiene`

## User outcome

A developer (or the loop) running the memory test suite gets a **clean worktree
afterwards** — no `test-fixture/` directory and no root-level `fake_ollama.go` left
behind — so the Gate -1 worktree-cleanliness check never trips on T8 and the build
binary / fixtures never risk being accidentally committed.

## Entry point

`go test ./internal/memory/...` followed by `git status --porcelain` (must be empty).

## Background

During S24-memory-engine, the memory tests wrote scratch into the tree: a
`test-fixture/` directory (fixture `MEMORY.md` / flat files / `memory.db`) and a
root-level `fake_ollama.go` (`package main` fake embedding server). Both were untracked
and recurred after every manual clean; a defensive `.gitignore test-fixture/` was added
on release-wt (commit `5d1b7c4`) as a stopgap. This slice removes the root cause.

## In scope

- Rewrite the `internal/memory` tests to create **all** fixtures (memory dirs, flat
  files, the SQLite index path) under `t.TempDir()`, so nothing is written into the
  tracked tree.
- Fold the fake embedding server (`fake_ollama.go`) into a test helper that lives in a
  `_test.go` file — prefer `httptest.NewServer` (already the pattern in `embed_test.go`)
  — and **delete the root-level `fake_ollama.go`**.
- Ensure no test creates a `test-fixture/` directory in the repo tree.

## Out of scope

- Any change to memory production code (`internal/memory/*.go` non-test) — tests only.
- Removing the defensive `.gitignore test-fixture/` line (harmless belt-and-braces; keep).

## Planned touchpoints

- `internal/memory/embed_test.go`
- `internal/memory/discover_test.go`
- `internal/memory/index_test.go`
- `fake_ollama.go` (delete; relocate logic into a `_test.go` helper)

## Acceptance checks

- [ ] After `go clean -testcache && go test -race ./internal/memory/...`, `git status
  --porcelain` is **empty** (no untracked `test-fixture/`, no `fake_ollama.go`, no DB files)
- [ ] `fake_ollama.go` no longer exists at the repo root; its fake-server logic lives in a
  `_test.go` file (or is replaced by `httptest.NewServer`)
- [ ] No test references a path under the tracked tree for fixtures — all use `t.TempDir()`
- [ ] All previously-passing memory tests still pass (`go test -race ./internal/memory/...`)

## Required tests

- The existing memory tests are the subject — they must still pass and must self-clean.
- **Reachability artefact**: paste in `proof.md` the output of
  `go test -race ./internal/memory/...` immediately followed by `git status --porcelain`
  showing an empty result.

## Risks

- A fixture currently read by relative path may need its path threaded through `t.TempDir()`;
  ensure helpers return the temp path rather than assuming CWD.

## Deferrals allowed?

No deferrals expected — this is a bounded test-only refactor.
