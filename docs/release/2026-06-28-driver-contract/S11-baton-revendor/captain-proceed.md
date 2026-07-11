# Coach acknowledgement — S11-baton-revendor (v0.10.0)

Date: 2026-07-11
Decided by: Brad (Coach) — the critical escalate pin ratified live;
mechanical/memory-cited pins applied per the pre-authorised batch protocol.
Review: review.md (Captain, v0.10.0 design, verdict NEEDS_COACH; 9 pins)
Verdict: PROCEED — dispositions below

## Pin dispositions

1. **[CRITICAL] Board top-level fields vs strict v0.10.0 board-v1 —
   RATIFIED: EXPAND sworn#80 to the release level.** v0.10.0's board-v1
   (additionalProperties:false; top-level = $schema/release/tracks only)
   drops the top-level schema_version, release_worktree_path, and
   release_worktree_branch that sworn's writer still emits. Resolution:
   the writer STOPS persisting all three; the reader DERIVES the release
   worktree branch (release-wt/<release>) and path (sibling-of-repo, the
   same derivation family as the track path); the D1 normalise shim
   STRIPS the three from legacy board.json on read; S12 strips them from
   on-disk data. The vendored board-v1 stays byte-identical to v0.10.0,
   and a WriteBoard round-trip test asserts a freshly-written board
   validates. Spec amended on release-wt (@e8ecbd5): AC-06/AC-07 +
   in_scope. Record as a Type-1 design_decision citing this ack.

2. **[mechanical] design_decisions — ACCEPTED.** Record D1-D6 in
   status.json before in_progress: D1-D4 Type-1 (D1 normalise mechanism,
   D2 track-path-reuse, and the Type-1 board-level pin-1 decision),
   D5/D6 Type-2. Rule 9 gate.

3. **[mechanical] Schema placement — ACCEPTED (spec corrected).** Vendor
   contracts-v1 + assembly-proof-v1 into internal/baton/schemas (the
   sole schema embed root) ONLY — internal/adopt/baton embeds rules/docs,
   not schemas. AC-10/AC-11 + in_scope amended (@e8ecbd5) from the
   handoff's imprecise "both embed roots".

4. **[mechanical] normalise strip-set — ACCEPTED.** Derive the shim's
   strip-set from the ACTUAL live-record-vs-strict-schema delta for
   spec-v1/board-v1/slice-status-v1 (not a hand-maintained literal), and
   test it against a real on-disk record.

5. **[mechanical] AC-09 — ACCEPTED.** Confirm oracle.go ReadSliceStatus
   is already owner-branch-first before writing the AC-09
   regression-only test (do not re-implement; lock against regression).

6. **[memory-cited] D2 track-path — CONFIRMED.** Reuse worker.go's
   sibling-of-release-worktree logic (eval finding 3), NOT the naive
   $HOME formula; move to the shared internal/board helper. The
   release-level path derivation (pin 1) uses the SAME logic.

7. Remaining pins (incl. pin 9, shared-helper home determinable at code
   time — internal/board unless the import graph forces a leaf pkg) are
   accepted as written in the Captain's suggested reply.

Proceed to implementation.
