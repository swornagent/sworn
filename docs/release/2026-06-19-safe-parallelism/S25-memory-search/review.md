# Captain review — S25-memory-search
Date: 2026-06-22
Design commit: 1803478713c530e0bcfc3a839db6d18110672ded

## Pins

**1. [mechanical] §2b — `design_decisions` absent from status.json (recurring T8 pattern)**

What I observed: status.json has no `design_decisions` field; the five §2 decisions are recorded only in design.md prose. Trial log shows this same miss in S23 and S24. The `sworn designfit` gate (S32) will trivially pass on an empty field, bypassing the Type-1 gate check entirely.

What to ask the implementer: Add `design_decisions` array to status.json before transitioning to `in_progress`. All five decisions appear Type-2 (reversible, not whole-system-shaping), but they must be explicitly typed. D1 (linear scan) — the ANN deferral is a spec-blessed trade-off, Type-2 is correct, but it must be declared.

---

**2. [escalate] CRITICAL §2/Risks §2 — spec Risks §2 makes a false factual claim; shim plan silently kills captain's semantic memory search**

What I observed: Spec Risks §2 states "Coach-loop does not use `--batch` in the current version per captain-handbook §5." Verified from `~/.claude/bin/captain-prepare.sh:249`:
```bash
"$MEMORY_SEARCH" --batch --top-k 5 --project-dir "$PRIMARY_REPO"
```
The coach-loop absolutely uses `--batch`. The shim plan intercepts `--batch`, prints a migration notice, and exits 0 — captain-prepare.sh gets empty stdout, falls back to `SEMANTIC_MATCHES_JSON='{}'`, and the captain design-review function loses its pre-loaded semantic matches silently.

What to ask the implementer: Coach must decide the migration path before code:
- (a) Add `--batch` support to `sworn memory search` in S25 (scope expansion; changes this slice's spec)
- (b) Update `captain-prepare.sh` to call `sworn memory search` per-decision (out-of-spec touchpoint; adds captain-prepare.sh to S25 scope)
- (c) Formally accept the degradation in the spec (captain runs without semantic pre-loading until a future slice)

Option (a) or (b) corrects the functionality; option (c) accepts a regression. The spec Risks §2 claim must be corrected in all cases.

---

**3. [mechanical] CRITICAL §3/AC4 — "no index" detection gap; `OpenIndex` always creates the DB**

What I observed: The design plan says "Opens index at configured path (S24)." But `OpenIndex` in `internal/memory/index.go:83-99` calls `os.MkdirAll` and `CREATE TABLE IF NOT EXISTS`, creating an empty database at `cfg.IndexPath` whether or not `sworn memory build` was ever run. AC4 requires: exit non-zero with "No memory index found. Run `sworn memory build` first." — but with OpenIndex's create-on-open behaviour there is no file-level signal after the fact.

What to ask the implementer: Add `os.Stat(cfg.IndexPath)` BEFORE calling `OpenIndex`. If the file is absent, print the AC4 message and exit non-zero. Do NOT call `OpenIndex` on a non-existent path; it creates a zombie empty DB that subsequent searches silently return 0 results for.

---

**4. [mechanical] CRITICAL §2 D1/spec flow — Voyage `input_type` mismatch; `embed_voyage.go` hardcodes `"document"` for all calls**

What I observed: Spec flow step 1 says "Embed the query using the configured provider with `input_type: "query"` (Voyage)." But `internal/memory/embed_voyage.go:69` hardcodes `InputType: "document"` in all `Embed()` calls. The `Embedder` interface has a single `Embed(ctx, []string) ([][]float32, error)` method with no way to express `input_type`. Using `input_type: "document"` for query embedding measurably reduces asymmetric search recall.

What to ask the implementer: The fix must not break S24's verified state. Choose one approach before writing code:
- Type-assert in `Search()`: check if embedder satisfies an internal `queryEmbedder` interface with `EmbedQuery(ctx, string) ([]float32, error)` — voyage implements it, oai-compat/ollama fall through to `Embed()`. Backward-compatible; does not require re-verifying S24.
- OR add `EmbedQuery` to the public `Embedder` interface and update all three implementations + S40's test helpers. Requires S40 co-ordination.
Do not call `Embed()` with the query text as-is and accept the `input_type: "document"` recall penalty.

---

**5. [mechanical] §3 — `~/.claude/bin/captain-memory-search.py` not in `planned_files`**

What I observed: status.json `planned_files` lists only in-repo files. The shim at `~/.claude/bin/captain-memory-search.py` is a planned touchpoint (spec, design §3) but lives outside the repo and won't appear in `git diff --name-only`. Rule 6's "Files changed" section won't capture it automatically.

What to ask the implementer: Explicitly list the shim path under an "Out-of-repo touchpoints" bullet in proof.md's Delivered section. Include before/after shim content in the proof, not just an assertion that it was updated.

---

## Summary

Pins: 5 total — 4 [mechanical], 0 [memory-cited], 1 [escalate]
Critical pins: 2, 3, 4 (would cause AC violations or functional regression if unaddressed)

## Smaller flags (not pins, worth one-line ack)

(a) Spec Risks §1 (float32 precision drift) mitigation — "embeddings from DB, query freshly computed" — is implicitly followed by the design flow but not explicitly acknowledged in §2. Low risk: the flow matches. No action required, just confirmation.

(b) `captain-memory-search.py --rebuild-only` is also intercepted by the shim. Unlike `--batch`, this one is not used by captain-prepare.sh, so no functional impact. Confirm if any other script relies on it.

(c) AC6 (`--batch` exits 0 with migration notice) and AC5 (shim delegates `--json`) depend on Pin 2's resolution. If option (a) is picked (add --batch to Go), AC6 must be redefined.

## Suggested ack reply

<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Design is sound on search mechanics, test structure, and AC coverage — 5 pins to address inline:

1. **design_decisions in status.json.** Before transitioning to `in_progress`, add `design_decisions` array to status.json classifying all five §2 decisions as Type-2. D1 (linear scan, ANN deferred) = Type-2 explicitly.

2. **--batch migration path — Coach decision required (Pin 2).** See Coach routing. Await Coach directive on option (a/b/c) before writing any shim code. Do not proceed on the shim until this is resolved.

3. **No-index detection.** In `cmdMemorySearch`, add `os.Stat(cfg.IndexPath)` before `memory.OpenIndex()`. File absent → print "No memory index found. Run `sworn memory build` first." → `return 1`. Do NOT call OpenIndex on a missing path.

4. **Voyage `input_type: "query"`.** Before writing `Search()`, choose the interface approach. Recommended: internal `queryEmbedder` interface type-assertion in `Search()` — backward-compatible with S24's verified state. Voyage implements it; oai-compat/ollama fall through to `Embed()`.

5. **Shim in proof.md.** In proof.md Delivered section, add "Out-of-repo touchpoints" bullet for `~/.claude/bin/captain-memory-search.py` with before/after content.

Flags: (a) Spec Risks §1 mitigation implicitly followed — confirm; (b) confirm `--rebuild-only` is not used by any other script; (c) AC5/AC6 definitions depend on Pin 2 resolution.

Address pins 1, 3, 4, 5 inline during implementation. Pin 2 awaits Coach directive before any shim code is written. Transition to `in_progress` only after Pin 2 direction is received and Pin 1 is applied to status.json.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Pin 2 is a genuine scope/priority call — spec Risks §2 contains a false factual claim about coach-loop --batch usage; Coach must pick one of three migration paths before shim code is written.
-->
