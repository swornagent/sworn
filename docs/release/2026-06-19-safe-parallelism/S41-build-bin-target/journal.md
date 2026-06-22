# Journal — S41-build-bin-target

## 2026-07-05: Implementation session

**State transition:** `design_review` → `in_progress` → (pending proof)

**Captain review pins addressed:**
1. **Pin 1 (AC4 evidence):** Replaced `verify --help` smoke step with live state-writing
   invocation (`./bin/sworn run --task "test write"` from repo root) confirming
   `.sworn/sworn.db` lands at repo root, not under `cmd/sworn/`. Backed by code
   citation to `internal/db/db.go:87` (`DefaultPath`) which constructs the path
   relative to workspace root.
2. **Pin 2 (design_decisions):** Populated `design_decisions` in status.json with
   the 5 §2 decisions (all Type-2) in S38-compatible format.

**Implementation:**
- No Makefile edits needed — existing Makefile (since S01) already has
  `build`, `test`, `vet`, `fmt`, `clean` targets with correct LDFLAGS.
- Created `docs/build.md` documenting canonical `make build` and
  run-from-repo-root convention.
- CI untouched — it uses `go vet ./...` and `go test ./...` directly,
  consistent with the Makefile targets.
- No changes to sworn's state-dir resolution — deferred per spec (Rule 2,
  Coach ack 2026-06-21).

**Deferrals (carried forward from spec/design):**
1. **sworn cwd-relative state-dir resolution** — deferred; Makefile+doc
   convention fixes observed clutter. Coach ack 2026-06-21.
2. **reachability smoke-step prompt wording** — deferred to
   S33-spec-template-hardening to avoid T3 prompt-ownership collision.
   (S33 is now verified but did not add make build wording; filed as GH #9.)

**Verification:**
- `make build`: produces `./bin/sworn` (12.8 MB), `git status` clean
- `make test`: all packages pass (30/30 ok)
- `make vet`: clean
- State-write test: `./bin/sworn run --task "test write"` from repo root writes
  `.sworn/sworn.db` at repo root, nothing under `cmd/sworn/`