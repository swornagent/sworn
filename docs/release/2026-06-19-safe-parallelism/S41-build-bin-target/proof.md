# Proof Bundle — S41-build-bin-target

## Scope

A contributor (or the loop) runs `make build` and gets `./bin/sworn`, then runs it
from the repo root — so sworn's run-state lands predictably at the repo root instead
of under `cmd/sworn/`.

## Files changed

```
docs/build.md
docs/release/2026-06-19-safe-parallelism/S41-build-bin-target/journal.md
docs/release/2026-06-19-safe-parallelism/S41-build-bin-target/status.json
```

## Test results

### make build

```
$ make build
go build -ldflags "-s -w -X main.version=0.0.0-dev" -o bin/sworn ./cmd/sworn
$ ls -la bin/sworn
-rwxrwxr-x 1 brad brad 12878089 Jun 23 03:30 bin/sworn
```

### make test

```
$ make test
go test ./...
ok  	github.com/swornagent/sworn/cmd/sworn	0.295s
ok  	github.com/swornagent/sworn/internal/adopt	(cached)
ok  	github.com/swornagent/sworn/internal/agent	(cached)
ok  	github.com/swornagent/sworn/internal/bench	(cached)
ok  	github.com/swornagent/sworn/internal/board	0.005s
ok  	github.com/swornagent/sworn/internal/command	(cached)
ok  	github.com/swornagent/sworn/internal/config	(cached)
ok  	github.com/swornagent/sworn/internal/db	(cached)
ok  	github.com/swornagent/sworn/internal/designaudit	(cached)
ok  	github.com/swornagent/sworn/internal/designfit	(cached)
ok  	github.com/swornagent/sworn/internal/ears	(cached)
ok  	github.com/swornagent/sworn/internal/git	0.180s
ok  	github.com/swornagent/sworn/internal/implement	(cached)
ok  	github.com/swornagent/sworn/internal/journey	(cached)
ok  	github.com/swornagent/sworn/internal/lint	(cached)
ok  	github.com/swornagent/sworn/internal/mcp	(cached)
ok  	github.com/swornagent/sworn/internal/memory	(cached)
ok  	github.com/swornagent/sworn/internal/model	(cached)
ok  	github.com/swornagent/sworn/internal/prompt	(cached)
ok  	github.com/swornagent/sworn/internal/reqvalidate	(cached)
ok  	github.com/swornagent/sworn/internal/reqverify	(cached)
ok  	github.com/swornagent/sworn/internal/rtm	(cached)
ok  	github.com/swornagent/sworn/internal/run	(cached)
ok  	github.com/swornagent/sworn/internal/scheduler	(cached)
ok  	github.com/swornagent/sworn/internal/specquality	(cached)
ok  	github.com/swornagent/sworn/internal/state	(cached)
ok  	github.com/swornagent/sworn/internal/supervisor	(cached)
ok  	github.com/swornagent/sworn/internal/telemetry	(cached)
ok  	github.com/swornagent/sworn/internal/tui	(cached)
?   	github.com/swornagent/sworn/internal/verdict	[no test files]
ok  	github.com/swornagent/sworn/internal/verify	(cached)
```

30/30 packages pass.

### make vet

```
$ make vet
go vet ./...
```

Clean — no output.

## Reachability artefact

**Path:** `docs/build.md` (documentation convention).

**Smoke step:** `make build && ls -la bin/sworn` produces an executable at `./bin/sworn`.

**State-write test:** Running `./bin/sworn run` from repo root creates `.sworn/sworn.db` at repo root, nothing under `cmd/sworn/`:

```sh
$ cd <repo-root>
$ SWORN_VERIFIER_MODEL=openai/gpt-4o-mini ./bin/sworn run --task "verify state writes to repo root" --retry-cap 0
# (fails on API key, but DB is created before model call)
$ find .sworn -type f
.sworn/sworn.db
$ find cmd/sworn/.sworn -type f 2>/dev/null || echo "No cmd/sworn/.sworn (correct)"
No cmd/sworn/.sworn (correct)
```

**git status after build:**

```
$ git status --porcelain
?? docs/baton/rules/08-requirements-fidelity.md
?? docs/baton/rules/09-design-fidelity.md
?? docs/baton/rules/10-customer-journey-validation.md
?? docs/baton/rules/11-process-global-mutation.md
```

No tracked files modified — `bin/sworn` is gitignored.

## Delivered

- **AC1:** `make build` produces `./bin/sworn`; `git status` stays clean (bin/ gitignored)
  - Evidence: `make build` output above; `git status --porcelain` shows only untracked baton rules.
- **AC2:** `make test` and `make vet` run the suite / vet across `./...`
  - Evidence: `make test` passes 30/30 packages; `make vet` is clean.
- **AC3:** `docs/build.md` documents `make build` and the run-from-repo-root convention
  - Evidence: `docs/build.md` exists with canonical build section, run-from-repo-root section, build flags, and other targets table.
- **AC4:** Running `./bin/sworn <a command that writes state>` from the repo root writes `.sworn/` at repo root, not under `cmd/sworn/`
  - Evidence: State-write test above confirms `.sworn/sworn.db` at repo root; nothing at `cmd/sworn/.sworn/`.

## Not delivered

1. **sworn cwd-relative state-dir resolution** — deferred. The Makefile + doc convention fixes the observed clutter without changing code in `cmd/sworn/internal/config`. Tracking: follow-up slice if cwd-relative state proves insufficient. **Acknowledged**: Coach, 2026-06-21.
2. **reachability smoke-step prompt wording** — deferred to S33-spec-template-hardening to avoid T3 prompt-ownership collision. S33 is now verified but did not add make build wording; filed as GH #9.

## Divergence from plan

- **Makefile already existed** (since S01 with build/test/vet/fmt/clean targets + ldflags). No new Makefile was created — the existing one satisfies the spec. The spec's bare `go build -o bin/sworn ./cmd/sworn` is a floor; the existing target adds `-ldflags "-s -w -X main.version=$(VERSION)"` which CI/release.yml relies on.
- **`docs/build.md` created at repo root** rather than embedded in AGENTS.md — avoids cross-track collision with S21/T3 (AGENTS.md rewrite) and S22/T4 (splice detection).

## First-pass script output
```
All 23 checks passed, 0 failed.

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  integration branch: release/v0.1.0
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  PASS  3 file(s) changed vs diff base
    docs/build.md
    docs/release/2026-06-19-safe-parallelism/S41-build-bin-target/journal.md
    docs/release/2026-06-19-safe-parallelism/S41-build-bin-target/status.json

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md
  PASS  proof.md 'Not delivered' deferrals carry non-placeholder tracking refs
  PASS  proof.md 'Files changed' count (~3) consistent with diff vs start_commit (3)

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output (Jest/Vitest scope confirmed)

FIRST-PASS PASS
```
