# Design TL;DR — S41-build-bin-target

## §1. User-visible change

A contributor runs `make build` at the repo root, which produces `./bin/sworn`
(an already-gitignored path). They then run `./bin/sworn <cmd>` from the repo
root, and sworn's run-state (`.sworn/`, `docs/release/run-*`) lands predictably
at the repo root instead of littering `cmd/sworn/`. The new `docs/build.md`
documents this canonical build + run-from-root convention.

## §2. Design decisions not in spec (max 5)

1. **Makefile already exists.** The spec says "Add a Makefile" but one has
   existed since S01 with `build`, `test`, `vet`, `fmt`, `clean` targets and
   `-ldflags` version injection. These already satisfy the spec's requirements
   (it says "at least: build, test, vet"). No Makefile edits — the existing
   file is sufficient.
2. **`make build` already uses `-ldflags -s -w -X main.version=$(VERSION)`**
   instead of the bare `go build -o bin/sworn ./cmd/sworn` in the spec.
   Preserving the existing flags is correct — they strip debug info and inject
   the version, which CI/`release.yml` relies on. The spec's bare `go build` is
   a floor, not a ceiling.
3. **CI is untouched.** CI uses `go vet ./...` and `go test ./...` directly,
   not via make. The spec's risk section says "do not break the existing CI
   build invocation" — no change needed.
4. **No Makefile drift from existing CI.** The Makefile's `vet` and `test`
   targets already match CI verbatim (`go vet ./...`, `go test ./...`). No
   risk of drift.
5. **`docs/build.md` location:** repo-root `docs/build.md`, not `AGENTS.md`.
   Per spec rationale: `AGENTS.md` is owned by S21/T3 and S22/T4; a separate
   file avoids cross-track collision. S21/S33 can later add an AGENTS.md
   pointer if desired.

## §3. Files I'll touch grouped by purpose

- **New: `docs/build.md`** — canonical build + run-from-root convention doc.
  Why: the spec's sole new deliverable not already present in the repo.
- **No changes to `Makefile`** — already satisfies spec requirements.

## §4. Things I'm NOT doing

- **Not editing `Makefile`.** The existing Makefile already has `build`, `test`,
  `vet` targets with version-injection LDFLAGS. The spec says "at least" those
  targets — satisfied.
- **Not changing sworn's state-dir resolution** — deferred per spec (Rule 2,
  Coach ack 2026-06-21).
- **Not editing reachability smoke-step prompts** — deferred to S33 per spec.

## §5. Reachability plan

Terminal paste in `proof.md`:
- `make build && ls -la bin/sworn` — shows the binary exists and is executable
- `git status --porcelain` — shows clean tree (bin/ is gitignored)
- `./bin/sworn verify --help && ls -d .sworn/ 2>/dev/null || echo "no state written for read-only verify"` — confirms run-from-root writes state at root

## §6. Open questions for the Coach

_None._