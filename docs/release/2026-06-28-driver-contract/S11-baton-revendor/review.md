# Captain review — S11-baton-revendor
Date: 2026-07-11
Design commit: dfd47e414ba8dee1ccecfbf9e21e942d926dbfa4

Scope of this review: the v0.10.0 re-target delta. The D1 (normalise-before-validate),
AC-08 (satisfied-by-engine) and D2 (reuse worker.go derivation + git.IsAncestor)
decisions are Coach-ratified and in-spec; this review confirms the design honours them
and does not re-open them.

## Verification performed (live repo + upstream tag)
- **Pin target.** Upstream `baton` tag `v0.10.0` = annotated-tag object `fa591989…`, peels to
  commit **a5ab2aa** — exactly the design's "v0.10.0 @ a5ab2aa (annotated-tag object fa591989…)".
  Live pin is `v0.6.3 @ a5aca64` (`internal/adopt/baton/VERSION`) — matches the design's "superseded" claim.
- **New schemas exist at the tag.** `schemas/contracts-v1.json` and `schemas/assembly-proof-v1.json`
  are present at `v0.10.0`; their `$id`s are exactly `https://baton.sawy3r.net/schemas/contracts-v1.json`
  and `…/assembly-proof-v1.json` — so "byte-match to the tag file" == "byte-match to the published `$id`" (AC-10/AC-11 satisfiable).
- **Embed-root reality.** Root A (`internal/adopt/baton/`) holds `rules/ README.md VERSION architecture.json`
  and **no schemas dir**; `adopt.go:14` `//go:embed` ships **no schemas** to consumer repos. Root B
  (`internal/baton/schemas/`) is the sole home of every existing schema. (Basis for Pin 3.)
- **Code citations all check out:** `doctor.go:432 baton.Version()`; `worker.go:698 defaultTrackWorktreePath`
  (+ "eval finding 3" comment); `state.go` chore/epic sites at 97/415/424-425/431/437 + `Validate` at 446;
  `spec.go` has NO scope fields (AcceptanceCriteria at 30); `spec_record.go:18 SchemaVersion`;
  `board.go:241 boardTracksToTrackInfos`, `BoardTrack` worktree/branch/state at 87/88/89;
  `track.go:37/39/41` regex + 153-169; `git.go:153 IsAncestor`; `mcp/tools_plan.go:148 fmt.Sprintf("track/%s/%s")`;
  `oracle.go:282 ReadSliceStatus`, `:376 readTrackInfos`. The design's line-citations are accurate against
  live code (the stale numbers are in the *spec* — R-05 worker.go:600-621, rationale state.go:410,426 —
  which the design silently corrected; not a design defect).
- **The advisory schemas are correctly scoped:** vendored + byte-matched + doctor-declarable, NOT graded,
  NOT forked under the same `$id`. No `lint contracts` / `assemble` grader or `doctor --sync-baton` (S15)
  work strays into this design. Confirmed clean.

## Pins

1. **[escalate] — CRITICAL. §AC-01/AC-06 — byte-identical board-v1 re-sync removes top-level
   `release_worktree_path`/`release_worktree_branch` (and `schema_version`), but the board WRITER still
   emits them and `WriteBoard` validates every write — the design's shim + sworn#80 scope only cover `tracks[]`.**
   What I observed: the `v0.10.0` `board-v1` is `additionalProperties:false` with `required:["release","tracks"]`
   and **no** top-level `release_worktree_path`/`release_worktree_branch` and no `schema_version`
   (tracks[] also shorn of worktree/state, as the design says). But **every live and fixture board.json
   carries `release_worktree_path`/`release_worktree_branch`** (e.g. this release's `board.json:12-13`),
   `BoardRecord` still declares+emits them (`board.go:24-25`), and **`board.go:171 WriteBoard` runs
   `baton.Validate("board-v1", …)` on every write.** The design updated the *spec* writer for spec-v1's
   new shape (AC-03) but there is no equivalent board-writer update: the normalise shim is enumerated as
   "exactly … schema_version, board tracks[] worktree/state, quadrant" — it does **not** strip the
   top-level `release_worktree_*`, and it is a read-path shim while `WriteBoard` is a write-path validate.
   Result: after a byte-identical re-sync, any board.json write (including this slice's own board fixtures)
   fails schema validation. The AC-06 scope ("tracks[]… only id/slices/depends_on remain") does not mention
   the top-level fields, and `out_of_scope`/baton#55 lists specific deferred items (additionalProperties
   strictness, id_token, vertical_trace/target_version/integration_branch) but **not** `release_worktree_*`,
   so this is genuinely unspecified.
   What to ask the Coach: this is a scope/spec-direction decision with no single determinable answer —
   (a) extend sworn#80 derivation to the release-level worktree fields (derive + stop emitting, keeping
   board-v1 byte-identical and `WriteBoard` green), which expands this slice beyond AC-06's "tracks[] only";
   (b) retain `release_worktree_*` in a sworn-local board-v1 (NOT byte-identical, contradicting AC-01); or
   (c) explicitly defer via a Coach-acknowledged Rule-2 window tolerance (and confirm no in-track board.json
   write occurs before S12 migrates + a matching S12 emission-stop). Option (a) also touches every
   `ReleaseWorktreePath` consumer, so it should be re-reviewed before code, not silently absorbed.

2. **[mechanical] — §Decision list — `status.json` carries no `design_decisions` array; the Rule-9
   design-fit gate cannot pass as-is.**
   What I observed: `status.json` has `effort_complexity` + `verification` but no `design_decisions` field,
   while the design records D1–D4 as Type-1 (ratified Coach 2026-07-11) and D5/D6 as Type-2.
   What to ask the implementer: record D1–D6 in `status.json.design_decisions` before `in_progress` —
   D1/D2/D3/D4 as Type-1 with the Coach 2026-07-11 ratification reference, D5/D6 as Type-2. (This is the
   already-tracked pre-transition step.)

3. **[mechanical] — §AC-10/AC-11 (design P1) — advisory-schema placement is determinable: root B only.**
   What I observed: the design escalates "both embed roots" placement. It resolves from live facts:
   AC-01 itself defines the two roots as `{internal/adopt/baton = rules/docs, internal/baton/schemas = schemas}`;
   `adopt.go:14` embeds no schemas (root A ships none to consumers); `spec-v1`/`board-v1` live only in root B.
   What to ask the implementer: vendor `contracts-v1.json` + `assembly-proof-v1.json` into **`internal/baton/schemas/` (root B) only**,
   add the two `//go:embed` vars + two `SchemaMap` entries in `embed.go`, byte-match test each. Do **not**
   create an `internal/adopt/baton/schemas/` mirror — scaffolding schemas into consumer repos via `sworn adopt`
   would be a separate adoption-bundle change, out of this vendor-bump's scope. Record this reading so the
   Rule-7 verifier reads AC-10/AC-11's literal "both embed roots" against AC-01's definition (= the schemas root).

4. **[mechanical] — §D1 — derive the normalise-shim strip-set from the ACTUAL live-record ↔ strict-schema
   delta, not a hand-listed subset; test with a real on-disk record.**
   What I observed: the shim is enumerated as "exactly the retired fields (schema_version, board tracks[]
   worktree/state, quadrant chore/epic)." Pin 1 shows that enumeration is already incomplete for board-v1.
   The current vendored `spec-v1` is `additionalProperties:true`+requires `schema_version`; the tag's is
   `additionalProperties:false`+requires `in_scope`/`out_of_scope`+no `schema_version` — a real strictening.
   What to ask the implementer: for EACH re-synced schema (spec-v1, board-v1, slice-status-v1), diff the
   byte-identical strict schema against the shapes sworn currently emits/holds and confirm the shim (or the
   writer update) covers every forbidden-but-present field. Add a test that normalises a **real current**
   `board.json`/`spec.json` and validates it against the re-synced schema (not just a hand-built fixture).

5. **[mechanical] — §AC-09 — "no production change" rests on `ReadSliceStatus` already being track-branch-first.**
   What I observed: `oracle.go:282 ReadSliceStatus` exists, and the live board oracle already reports
   unmerged-but-verified slices (S06/S07/S08/S14) as `verified` — the track-branch-first ordering appears live.
   What to ask the implementer: confirm the owner-branch → release-wt → HEAD ordering before writing the
   regression-only test; if any path still falls back to release-wt first, AC-09 needs production work, not just a test.

6. **[memory-cited] — §design P3 — full-suite + newline-eating backstop is mandatory.**
   What I observed: a byte-identical re-sync rewrites many `.go`-adjacent vendored files and edits
   `state.go`/`spec.go`/`board.go`; the shim tightens a reader.
   What to ask the implementer: run `gofmt -l` + `go vet` + full `go test -count=1 -timeout 300s ./...`
   before ANY state transition (not just the AC-05 subset), and grep re-synced/edited `.go` for fused
   `//`+code. Citation: [[Newline-eating edit corruption (3x)]] + [[R3 S05 strict-reader regressed sibling fixtures]].

7. **[memory-cited] — §process — `release-verify.sh` will false-FAIL "spec.md missing" on this spec-v1 slice.**
   What I observed: this slice ships `spec.json` (no `spec.md`).
   What to ask the implementer: do NOT manufacture a `spec.md` to satisfy the deterministic first-pass;
   the canonical gate is the model-backed `sworn verify`. Citation: [[release-verify.sh spec.md false-FAIL]].

8. **[memory-cited] — §AC-08/D5 — command-spec write-isolation correctly routes to upstream baton, not this binary.**
   What I observed: D5 says sworn vendors no `commands/`; the implement-slice.md/merge-track.md prose edit is
   `baton#61`. Confirmed: `adopt.go` embeds no commands, and `baton#61` is OPEN
   ("Command specs … remove board.json track worktree/state writes (sworn#80 parity)").
   What to ask the implementer: honour the ADR-0010 boundary — no command-spec edits in this binary; the
   evidence test (vendored `rules/` contains no implement-slice.md/merge-track.md) is the right proof.
   Citation: [[Harness fix → public parity]].

9. **[mechanical] — §design P2 — shared derivation-helper home is determinable at code time.**
   What I observed: worker.go's path logic + branch/state helpers need one home both `internal/board` and
   `internal/scheduler`/`internal/mcp` import without a cycle.
   What to ask the implementer: place it where consumers already depend (`internal/board`) unless the import
   graph forces a leaf pkg; `go build ./...` + the verifier backstop this — no pre-code veto needed.

## Summary
Pins: 9 total — 5 [mechanical], 3 [memory-cited], 1 [escalate].
Critical pins: **Pin 1** (byte-identical board-v1 re-sync breaks `WriteBoard` validation for every board.json
that still emits top-level `release_worktree_*`; ships broken if unaddressed).

## Smaller flags (not pins, worth one-line acknowledgement)
- Spec R-01/R-05 are stale post-retarget (R-01 mitigation still names `v0.9.0 @ fc497b4`; R-05 cites
  `worker.go:600-621` vs live `690-698`). The design silently corrected to `v0.10.0` / `worker.go:698` — cosmetic spec lag, not a design defect.
- `status.json.effort_complexity.quadrant` is legacy `epic` and its rationale still says `v0.9.0`; per D6 the
  record stays legacy-valid until the enum flips in-track — acceptable, but the rationale text should say v0.10.0 when D6/decisions are recorded.
- Design asserts slice order S11 → S15 → S12; both S15 and S12 are `planned` in the same track under a serial
  implementer, so ordering resolves itself — confirm `depends_on` encodes it only if load-bearing.
- Open design-space question surfaced by Pin 3 (should `sworn adopt` ever scaffold schemas into consumer repos?)
  is genuinely out of this slice's scope; captured here rather than filed — no defect, no masked bug.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver applying the acknowledgement reads everything between this heading
     and the next ## heading (or EOF). Verbatim-pasteable into the Implementer session. -->

TL;DR strong, exceptionally well-grounded design — every code + tag citation verified — but one CRITICAL scope
gap (Pin 1) needs a Coach decision before code. 9 pins + 4 flags:

1. **board-v1 release_worktree_* write-path gap (Coach decision first).** Byte-identical `v0.10.0` board-v1
   drops top-level `release_worktree_path`/`release_worktree_branch` + `schema_version`, but the board writer
   still emits them and `board.go:171 WriteBoard` validates every write — your shim + sworn#80 only cover
   `tracks[]`. Per the Coach's decision on this scope (extend derivation to release-level / retain sworn-local
   non-byte-identical / defer with explicit window tolerance + matching S12 emission-stop), implement that path
   and add a test that normalises a real board.json and validates it against the re-synced schema.
2. **Record design_decisions.** Add D1–D6 to `status.json.design_decisions` (D1-D4 Type-1 w/ Coach 2026-07-11 ref, D5/D6 Type-2) before `in_progress`.
3. **Advisory schemas → root B only.** Vendor `contracts-v1`/`assembly-proof-v1` into `internal/baton/schemas/`
   only (+ 2 embed vars + 2 SchemaMap entries + byte-match tests). No `adopt/baton/schemas/` mirror. Note in the
   proof that AC-10/AC-11's "both embed roots" reads as AC-01's schemas root.
4. **Shim completeness.** Derive the strip-set from the real live-record ↔ strict-schema delta for spec-v1,
   board-v1 AND slice-status-v1 — not a hand-list; validate a real on-disk record, not just a fixture.
5. **AC-09.** Confirm `oracle.go ReadSliceStatus` is owner-branch-first before writing the regression-only test.
6. **Full-suite backstop.** `gofmt -l` + `go vet` + full `go test -count=1 -timeout 300s ./...` before any
   transition; grep re-synced/edited `.go` for fused `//`+code (newline-eating scar).
7. **Don't manufacture a spec.md** — `release-verify.sh` false-FAILs "spec.md missing" on spec-v1 slices; canonical gate is `sworn verify`.
8. **AC-08 boundary.** No command-spec edits in this binary; baton#61 owns the prose; the no-commands evidence test is the right proof.
9. **Helper home.** `internal/board` unless the import graph forces a leaf pkg; `go build` + verifier backstop.

Flags (not pins): (a) spec R-01/R-05 stale post-retarget, design corrected — fix the spec text at next replan;
(b) status.json quadrant/rationale still say epic/v0.9.0 — refresh when recording decisions;
(c) S11→S15→S12 order resolves itself under a serial track owner; (d) "should adopt scaffold schemas?" is out of scope, captured only.

§2 decisions: D1/D2/D3 [memory-cited] ([[board-v1 release shape]], [[Newline-eating edit corruption]], [[Harness fix → public parity]]),
D4/D5/D6 acknowledged. §6/P-items: P1→Pin 3 (resolved root B), P2→Pin 9, P3→Pin 6, P4 acknowledged.

Once the Coach resolves Pin 1's scope, address pins 1–9 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Pin 1 — byte-identical v0.10.0 board-v1 re-sync drops top-level release_worktree_*/schema_version that the board writer still emits and WriteBoard validates on every write; resolving it (expand sworn#80 to release-level vs retain non-byte-identical vs deferred window tolerance) is a scope/spec-direction call the Coach owns, not a determinable single answer.
-->
