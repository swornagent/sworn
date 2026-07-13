# Reconciling docs/baton/ against canonical upstream Baton

2026-07-01. Prompted by a question during version-hygiene cleanup: "have we
stripped all the stale version refs out of sworn now?" led to discovering
`docs/baton/VERSION` was stale relative to `internal/adopt/baton/VERSION`.
Investigating that surfaced a bigger reconciliation need.

## The worry

Two local copies of Baton content existed in this repo:

- `internal/adopt/baton/` — the embedded copy, `//go:embed`'d into the
  `sworn` binary. This is what `sworn --version` / `sworn doctor` read, and
  what `sworn init` writes into *other* (consumer) repos.
- `docs/baton/` — a second, separate per-repo copy, predating the embed
  mechanism.

Brad's concern: important edits might have been made to the wrong copy and
never made it back to the canonical upstream repo (`github.com/sawy3r/baton`).

## What was actually found

Cloned `github.com/sawy3r/baton` at tag `v0.6.3` (the latest tagged release,
commit `a5aca64849b9c36be6590606269a5c5b363b9888`) and diffed both local
copies against it, file by file:

- `internal/adopt/baton/rules/*.md` (11 files): **byte-identical** to
  upstream `v0.6.3`. `internal/adopt/baton/VERSION` correctly records
  `upstream-sha: a5aca648...` — matching exactly. This copy is genuinely
  canonical-correct; no drift, nothing to fix.
- `docs/baton/rules/*.md`: 7 of 11 files differed from upstream. Inspected
  the diffs directly (not just line counts) for the ones that were *longer*
  in `docs/baton/` (08-requirements-fidelity, 10-customer-journey-validation,
  11-process-global-mutation) to check the worried-about direction. In every
  case the extra length was **older, more verbose prose predating the
  ADR-0009 records-as-JSON rewrite** — the upstream version references
  `spec.json`/`proof.json`/`board.json` and `covers_needs`; `docs/baton/`'s
  version still references the pre-migration `spec.md`/`proof.md`/`index.md`
  forms. Nothing found in `docs/baton/` was upstream-absent, valuable, or at
  risk of being lost — it was a stale, un-refreshed `sworn init` snapshot,
  exactly matching `sworn doctor`'s own diagnosis ("legacy per-repo Baton
  copy... safe to remove").
- `docs/baton/README.md` vs `internal/adopt/baton/README.md`: different
  documents by design — the docs/ copy was a short sworn-specific pointer,
  the embed is upstream's full adopter-facing README. Neither carried unique
  content worth preserving once the pointer purpose was gone.

**Conclusion: no protocol content was lost or stuck in the wrong place.**
The fear was reasonable given the drift, but `internal/adopt/baton/` was
already correctly tracking canonical upstream.

## The one real risk, caught before deleting

`docs/baton/` also held two files with **no counterpart anywhere in the
embed or upstream**: `decisions/orchestrator-model.md` and
`roles/orchestrator.md` — sworn's own Type-1 design-decision record and role
spec for the Orchestrator, filed under `docs/baton/` for no reason other than
convenience. These are hard-linked by exact path from two embedded runtime
prompts (`internal/prompt/design-reviewer.md`,
`internal/prompt/orchestrator-notes.md`) and a historical release journal
entry. A blind `rm -rf docs/baton/` (or `sworn doctor --fix`, which does
exactly that) would have silently deleted them.

`sworn doctor --fix` was also rejected wholesale for a second reason: beyond
removing `docs/baton/`, it unconditionally overwrites `AGENTS.md` with the
generic fresh-repo bootstrap template — which would have destroyed this
repo's own Layout / Build-test / Branching / Conventions sections, not just
the legacy Baton rule splice. That warning is left un-actioned; fixing it
would need a targeted splice, not a full overwrite.

## What was done

1. Relocated the two orchestrator docs out of `docs/baton/` to permanent
   homes: `docs/decisions/orchestrator-model.md`, `docs/roles/orchestrator.md`
   (git mv, history preserved).
2. Updated their two live referrers (`internal/prompt/design-reviewer.md`,
   `internal/prompt/orchestrator-notes.md`) to the new paths. Left the
   historical release-journal mention
   (`docs/release/2026-06-27-conformance-foundation/index.md` etc.)
   untouched — it's a capture of what happened at the time, not a live
   pointer.
3. Removed the confirmed-stale duplicate vendored content:
   `docs/baton/README.md`, `docs/baton/VERSION`, `docs/baton/rules/*`
   (and the now-empty `docs/baton/` directory).
4. Re-pointed the three dangling `docs/baton/` references in `AGENTS.md`
   and `CLAUDE.md` to the actual canonical source: `internal/adopt/baton/`
   (inspectable via `sworn doctor`).
5. Full `go build ./...` and `go test ./...` pass. `sworn doctor` now
   reports `docs/baton/: OK — not present (canonical source is the binary
   embed)`.

## Not done (deliberately)

- `AGENTS.md`'s "contains legacy Baton splice content" warning is left
  un-actioned. `sworn doctor --fix` fixes it via full-file replace, which
  would delete repo-specific sections. A real fix needs a targeted splice
  (replace only the `## Engineering Process — Baton` section, matching
  `internal/adopt.SpliceAgents`'s intent, not `doctor --fix`'s current
  full-overwrite behavior) — tracked as a follow-up, not done this session.
