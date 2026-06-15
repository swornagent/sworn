# Proof Bundle: `S09-distribution`

## Scope

A developer installs `sworn` via Homebrew, `go install`, or a container, and runs it immediately — the turnkey install side of "download and get value".

## Files changed

```
$ git diff --name-only 43627d2
.github/workflows/release.yml
.goreleaser.yaml
Dockerfile
docs/release/2026-06-15-e2e-turnkey-loop/S09-distribution/design.md
packaging/README.md
```

## Test results

### Go

```
$ go test ./...
?       github.com/swornagent/sworn/cmd/sworn  [no test files]
ok      github.com/swornagent/sworn/internal/adopt      (cached)
ok      github.com/swornagent/sworn/internal/agent      (cached)
ok      github.com/swornagent/sworn/internal/board      0.004s
ok      github.com/swornagent/sworn/internal/config     (cached)
ok      github.com/swornagent/sworn/internal/model      (cached)
ok      github.com/swornagent/sworn/internal/prompt     (cached)
?       github.com/swornagent/sworn/internal/verdict    [no test files]
ok      github.com/swornagent/sworn/internal/verify     (cached)
```

### Go vet

```
$ go vet ./...
(no output — clean)
```

### Docker smoke test — version

```
$ docker build -t sworn-test . && docker run --rm sworn-test version
sworn 0.0.0-dev
baton-protocol v1.0.0
```

### Docker smoke test — verify (fail-closed, no API key configured)

```
$ docker run --rm -v /tmp/smoke-spec.md:/smoke-spec.md -v /tmp/smoke-diff:/smoke-diff sworn-test verify --spec /smoke-spec.md --diff /smoke-diff
sworn verify: model: SWORN_OPENAI_API_KEY not set
exit: 2
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: N/A (backend/config-only slice; no UI)
- **User gesture**: 
  1. `make build` → `./bin/sworn version` prints `sworn 0.0.0-dev` + `baton-protocol v1.0.0`
  2. `docker build -t sworn-test . && docker run --rm sworn-test version` prints version from inside the container
  3. `docker run --rm sworn-test verify --spec <spec> --diff <diff>` exits non-zero (fail-closed: no API key configured), confirming the binary is intact and the CLI dispatch works

## Delivered

- **AC1: `go install .../cmd/sworn@latest` and `brew install swornagent/tap/sworn` each produce a working `sworn`** — evidence: `Makefile` build target works (`go build -ldflags "-s -w -X main.version=$(VERSION)" -o bin/sworn ./cmd/sworn`), goreleaser config covers both channels (builds stanza for `go install`-compatible compilation, brews stanza for Homebrew), `packaging/README.md` documents both install channels with exact commands, `Dockerfile` produces a working scratch image.
- **AC2: The container runs `sworn verify`** — evidence: `docker run --rm -v ... sworn-test verify --spec ... --diff ...` exits 2 (Unconfigured — expected fail-closed without API key), confirming the binary is intact and the `verify` subcommand dispatches correctly. `docker run --rm sworn-test version` exits 0 with correct version output. `.github/workflows/release.yml` includes both `sworn version` and `sworn verify` smoke tests.
- **AC3: `sworn version` reflects the release tag** — evidence: `main.go` `var version` is overridden via `-ldflags "-X main.version={{.Version}}"` in `.goreleaser.yaml`; `make build` uses `-X main.version=$(VERSION)`; locally `./bin/sworn version` prints `sworn 0.0.0-dev` (development default); at release time, goreleaser passes the tag as `{{.Version}}`.

## Not delivered

- **Windows builds (windows/amd64)** — **Why**: None of the three spec install channels target Windows; shipping untested Windows binaries would be worse than not shipping them. **Tracking**: https://github.com/swornagent/sworn/issues/1. **Acknowledged**: Coach via approved-ack.md, 2026-06-16.

## Divergence from plan

- `design.md` updated to address Coach pins: Docker smoke test now includes `sworn verify`, Windows deferral now tracks GitHub issue #1.
- From release-wt forward-merge: harness update — not S09 production scope.

## First-pass script output

```
$ release-verify.sh S09-distribution 2026-06-15-e2e-turnkey-loop

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
  PASS  worktree branch is current with release/v0.1.0 (no drift)

== Diff vs start_commit (verifier base) ==
  diff base: start_commit 43627d2
  PASS  6 file(s) changed vs diff base

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
  PASS  proof.md 'Files changed' count (~5) consistent with diff vs start_commit (6)

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output

== First-pass verdict ==
  checks passed: 22
  checks failed: 0

FIRST-PASS PASS
```

