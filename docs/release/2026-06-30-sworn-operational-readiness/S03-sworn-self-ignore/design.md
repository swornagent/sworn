# Design TL;DR — S03-sworn-self-ignore

**User outcome:** When sworn runs against any repo, its per-project runtime
state (`.sworn/` — `sworn.db`, `supervisor-<release>.db`) never appears in that
repo's `git status` and can never be accidentally committed, because sworn
self-ignores the directory it creates.

## Approach

The fix lands at the **single chokepoint** every `.sworn/` DB open routes
through: `db.Open` in `internal/db/db.go`, at the `os.MkdirAll(filepath.Dir(dbPath), …)`
that materialises `.sworn/`. Immediately after that dir is created, best-effort
write a `.gitignore` containing `*` into it. `*` matches every entry in the
directory — the DBs *and* the `.gitignore` itself — so git treats the whole
`.sworn/` directory as ignored and it disappears from `git status` (the classic
self-ignoring-directory trick, confirmed by the spec's AC-01 wording).

**Coverage confirmation (AC-01):** the supervisor DB is not a second creation
site. `supervisor.Open` (`internal/supervisor/supervisor.go:280`) builds
`.sworn/supervisor-<release>.db` and delegates to `db.Open`. Every other opener
(`internal/run/run.go:145`, `internal/tui/concurrent.go`,
`cmd/sworn/run.go:222`, `cmd/sworn/telemetry.go:368`) also calls `db.Open`. So
one write in `db.Open` covers the run DB **and** the supervisor DB — no second
touchpoint is needed.

## Key design choices + rationale

1. **Single helper, `O_CREATE|O_EXCL|O_WRONLY`.** A small unexported
   `writeSelfIgnore(dir string)` opens `<dir>/.gitignore` with
   `os.O_CREATE|os.O_EXCL|os.O_WRONLY`. This one syscall gives us **both**
   required guarantees atomically, with no TOCTOU window:
   - **Idempotency (AC-02):** if `.gitignore` already exists, `O_EXCL` makes
     `OpenFile` fail → we return without touching it → an operator-customised
     ignore is preserved byte-for-byte.
   - **Best-effort (AC-04):** any error (already-exists, or an unwritable
     target) is swallowed and returned; the DB-open path never observes it.
   *Alternative considered:* `os.Stat` then `os.WriteFile`. Rejected — two
   syscalls with a TOCTOU gap, and it needs a separate branch to preserve an
   existing file. `O_EXCL` collapses both AC-02 and AC-04 into one path.

2. **Gate the write on `filepath.Base(dir) == DefaultDir` (`.sworn`).**
   `db.Open` is a generic SQLite opener; it should only stamp a `*`-gitignore
   when the directory it just created is genuinely sworn's runtime dir. Gating
   keeps the behaviour precisely "self-ignore `.sworn/`" (matches AC-01's
   explicit framing) and prevents a stray `.gitignore` if `db.Open` is ever
   reused for a DB outside `.sworn/`. Every real caller passes a `.sworn/…`
   path, so the gate never suppresses a wanted write.
   *Alternative considered:* write unconditionally into `filepath.Dir(dbPath)`.
   Simpler, but writes `*`-gitignore into arbitrary parent dirs — surprising for
   a generic opener. Reversible either way (**Type-2**).

3. **Content is `"*\n"`.** Trailing newline is conventional; `*` is the whole
   contract. No per-file enumeration — the spec explicitly wants the directory
   fully ignored including future DB files.

## Stakes classification (Rule 9)

All choices are **Type-2** (easily reversible, narrow/local — confined to
`internal/db`, no public API change, no architecturally-significant surface).
No Type-1 choice is present, so no human design decision is required; the noted
defaults above stand.

## Files to touch (matches spec `touchpoints`)

- `internal/db/db.go` — add `writeSelfIgnore` helper; call it in `Open` after a
  successful `MkdirAll`, gated on the dir basename being `DefaultDir`.
- `internal/db/db_test.go` — new tests (see AC traceability).

## AC traceability

| AC | How the design satisfies it | Test |
|----|-----------------------------|------|
| AC-01 | `writeSelfIgnore` writes `.sworn/.gitignore`=`*` in `db.Open`; supervisor + run DBs both route through `db.Open` | Test: open a `.sworn/sworn.db`, assert `.sworn/.gitignore` contents == `*`; assert supervisor path (`.sworn/supervisor-r.db`) yields the same ignore |
| AC-02 | `O_EXCL` fails when `.gitignore` exists → no overwrite | Test: pre-write `.gitignore` with custom bytes, call `Open`, assert bytes unchanged |
| AC-03 | `.sworn/` fully ignored → absent from porcelain | Test: `git init` temp repo, `db.Open` under it, assert `git status --porcelain` has no `.sworn/` line |
| AC-04 | write error swallowed; DB-open success independent of it | Test: pre-create `.sworn/.gitignore` as a **directory** (unwritable target) → `Open` still returns a working DB; distinct from AC-02's existing-file case |
| AC-05 | no signature/behaviour change to callers | `go build ./...` + `go test ./internal/db/...` |

## Design-level risks / pins for the reviewer

- **Gate correctness:** confirm the reviewer agrees gating on
  `filepath.Base(dir) == DefaultDir` is desired vs unconditional. If a future
  custom-DB-dir config lands, the gate would skip the ignore there — acceptable
  under this slice's `.sworn/`-only scope, flagged here so it's a decision not a
  surprise.
- **AC-04 testability:** the `.gitignore` and the DB share `.sworn/`, so a
  read-only-dir test would fail the DB open too (coupled). The design makes
  AC-04 independently testable by using a pre-existing **directory** at the
  `.gitignore` path — the write cannot succeed, yet `Open` must. This is the
  intended, deterministic way to prove best-effort.
- **`~/.sworn/` opens:** if any opener targets `~/.sworn/…` (e.g. memory/config
  DBs) via `db.Open`, a harmless `~/.sworn/.gitignore` may be written. No repo
  there normally; courtesy-only. Noted, not a blocker.
