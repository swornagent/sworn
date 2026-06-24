# Journal — S62-baton-upstream-source

## 2026-06-23 — planned (replan)

- **Actor**: planner (human Brad + Claude)
- **Why**: heading to public release, the embed's source of truth should be the public
  Baton repo at a locked version — not a personal local install (`~/.claude/baton`), which
  is exactly what produced the S48 corruption. Lifts the network-fetch deferral tracked in
  **issue #11**.
- **Design (decided 2026-06-23)**:
  - Transport: **stdlib HTTPS tarball** (codeload `tar.gz` → `net/http` + `compress/gzip` +
    `archive/tar`). No git binary, **no module dependency, no ADR**. (Rejected git-clone and
    go-git.)
  - Default repo `github.com/sawy3r/baton`, overridable.
  - Lock: **tag + commit-SHA / content-digest, fail-closed** on force-moved tag / digest
    mismatch / network error. No `--tag` ⇒ the S49 pinned tag; never `latest`.
- **Placement**: appended to the tail of **T14-baton-integration** (S48 → S49 → S50 → S62).
  `depends_on S48` (source resolver + transform) and `S49` (semver pin + VERSION format).
- **Blocked on (external)**: implementation waits on the Baton repo being synced to canonical
  truth (the local install had drifted ahead) and **tagged** — that tag is the lock target.
- Sequenced after S50; T14 is in_progress (S48 implemented/re-verifying, S49/S50 planned).

## 2026-07-09 — design_review (design TL;DR)

- **Actor**: implementer (Claude)
- **DoR gate**: `sworn lint` subcommand not available (planned as S16 in fidelity release);
  reqverify and reqvalidate not checked — manual session, not `sworn implement`.
- **Design TL;DR** written to `design.md`; awaiting Captain review.
- **Key decisions**: SHA-256 digest, temp-dir lifecycle, VERSION write-after-success,
  flat function (no interface), positional arg optional with --upstream.

## 2026-07-09 — in_progress → implemented

- **Actor**: implementer (Claude)
- **Coach pins applied**:
  1. `design_decisions` array added to `status.json` (5 Type-2 decisions)
  2. `planned_files` reconciled: `source.go` → `version.go`
  3. Commit SHA resolution via `api.github.com/repos/{owner}/{repo}/commits/{tag}`
     (separate from codeload fetch — handles annotated tags correctly)
  4. First-fetch bootstrap: absent `upstream-digest` skips digest verification;
     SHA still catches force-moved tags
- **Coach flags applied**:
  (a) Repo override via `SWORN_BATON_REPO` env var (not Config struct — zero-migration path)
  (b) `repo` param validated as `owner/name` format
- **Implementation**:
  - `internal/baton/fetch.go` — `FetchUpstream()`, `FetchResult`, `Cleanup()`,
    `extractTarball()`, with `baseURLForTest` for test URL injection
  - `internal/baton/fetch_test.go` — 11 tests: success, SHA mismatch, digest
    mismatch, no-digest bootstrap, no-pins bootstrap, API 404, codeload 404,
    server 500, bad gzip, repo format validation, empty tag
  - `internal/baton/version.go` — `ReadUpstreamPin()`, `WriteUpstreamPin()`,
    `UpstreamPin` struct, `parseUpstreamPin()`, `upstreamPinForTest` override
  - `cmd/sworn/baton.go` — `--upstream`/`--tag`/`--repo` flags, `findRepoRoot()`
    extraction, `printVendorResult()` helper
- **Tests**: all 27 baton tests pass; all 2 cmd/sworn baton tests pass; go build + vet clean
- **Divergence from plan**: config-based repo override via env var instead of Config
  struct field (Type-2 reversible decision — Config schema migration out of scope for this slice)
- **Skeptic panel**: skipped — runtime does not support subagent dispatch
- **start_commit**: `e9d73cc14fe53cec60d12867e00cf3d83d270807`
- **Terminal state**: `implemented`