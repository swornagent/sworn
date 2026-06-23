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
