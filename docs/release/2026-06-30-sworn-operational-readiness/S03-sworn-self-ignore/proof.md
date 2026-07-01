# Proof bundle — S03-sworn-self-ignore

_Rendered from `proof.json` (proof-v1). Generated from live repo state._

## Scope

sworn self-ignores the `.sworn/` runtime dir it creates, so its churning SQLite
DBs never appear in a host repo's `git status` and can never be accidentally
committed.

## Files changed

`git diff --name-only 47d4802..HEAD`:

- `internal/db/db.go`
- `internal/db/db_test.go`
- `docs/release/2026-06-30-sworn-operational-readiness/S03-sworn-self-ignore/status.json`

## Test results

| Command | Result | Exit |
|---------|--------|------|
| `go build ./...` | PASS | 0 |
| `go test ./internal/db/...` | PASS | 0 |
| `go vet ./internal/db/...` | PASS | 0 |
| `gofmt -l internal/db/db.go internal/db/db_test.go` | PASS (empty) | 0 |

## Reachability artefact

`TestSelfIgnoreHidesSwornDir` (`internal/db/db_test.go`): git-inits a temp repo,
opens the sworn DB via `db.Open` under `<repo>/.sworn/`, then runs
`git status --porcelain` and asserts `.sworn/` is absent from the output. This
renders the user-facing outcome (a clean host-repo git status) through the real
integration point (`db.Open`) — not a leaf in isolation. PASS in
`go test ./internal/db/...`.

## Delivered

- **AC-01** — `db.Open` writes `.sworn/.gitignore` = `*` at the `MkdirAll`
  chokepoint; run DB and supervisor DB both route through `db.Open`, so one
  write covers both. Evidence: `writeSelfIgnore` + gated call in `Open`
  (`internal/db/db.go`); `TestSelfIgnoreWritten`.
- **AC-02** — a pre-existing `.sworn/.gitignore` is preserved byte-for-byte
  (`O_EXCL` never overwrites an operator-customised ignore). Evidence:
  `O_CREATE|O_EXCL|O_WRONLY`; `TestSelfIgnoreNotOverwritten`.
- **AC-03** — inside a git repo, `.sworn/` is absent from
  `git status --porcelain` after a DB open. Evidence: `TestSelfIgnoreHidesSwornDir`.
- **AC-04** — a failed `.gitignore` write is best-effort; `Open` still returns a
  working DB and never depends on the courtesy write. Evidence: swallowed error
  in `writeSelfIgnore` caller; `TestSelfIgnoreBestEffort` (pre-creates
  `.gitignore` as a directory so the write fails, asserts `Open` succeeds and
  the DB is queryable).
- **AC-05** — `go build ./...` succeeds and `go test ./internal/db/...` passes
  including the new tests. Evidence: test-results rows 1–2.

## Not delivered

_None._

## Divergence from plan

- **Proof-bundle verification gate.** The role prompt names a
  `sworn verify <slice> <release>` first-pass gate, but the installed `sworn`
  binary exposes the model-backed judge `sworn verify --spec <path> --diff <path>`.
  That gate could not run in this session because `SWORN_ANTHROPIC_API_KEY` is
  unset (only `BRAVE_API_KEY` is present). First-pass was instead run with the
  deterministic gate `~/.claude/bin/release-verify.sh` (with `PLAYWRIGHT_OPTIN=0`
  to work around an unbound-variable script bug at line 541): **19 checks PASS,
  1 FAIL**. The single FAIL is `spec.md missing` — a **documented false
  negative** of the deterministic gate for `spec-v1` slices, which carry
  `spec.json`, not a legacy `spec.md`. Per the known-issue record this is not
  addressed by manufacturing a `spec.md`: verified sibling
  `S04-board-record-reconciliation` reached `verified` with the identical
  condition. All 19 real structural checks pass (artefacts present + valid JSON,
  `state=implemented` eligible, no integration-branch drift, 4 files changed vs
  `start_commit`, no dark-code markers, all 7 proof.md sections present, no
  placeholders, deferrals carry tracking, test-results scope clean). Canonical
  model-backed verification is deferred to the fresh-context `/verify-slice`
  (Rule 7), where it belongs. Tracking: this divergence note; acknowledged in
  the session output.
