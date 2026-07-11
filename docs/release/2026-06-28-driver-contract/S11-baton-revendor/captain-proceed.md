# Coach acknowledgement — S11-baton-revendor

Date: 2026-07-11
Decided by: Brad (Coach) — both escalate pins ratified live; mechanical
and memory-cited pins applied per the pre-authorised batch protocol.
Review: review.md (Captain, verdict NEEDS_COACH)
Verdict: PROCEED — dispositions below

## Pin dispositions

1. **[escalate/critical] D1 transition mechanism — RATIFIED:
   NORMALISE-BEFORE-VALIDATE.** During the S11-landed/S12-pending
   window, un-migrated records are bridged by ONE read-path normalise()
   shim that maps retired `chore`->`quick` / `epic`->`beast`, strips
   `schema_version`, and reshapes the legacy `board-v1 tracks[]` into
   canonical v0.9.0 form BEFORE validation/checksum. `Validate` and
   `additionalProperties:false` stay STRICTLY strict throughout — they
   are never weakened, so genuine drift is still caught during the
   window. The vendored v0.9.0 schemas stay byte-identical to the tag
   (no sworn-local schema relaxation). S12 migrates the on-disk data and
   DELETES the shim wholesale (no tolerance branches left behind). Spec
   amended on release-wt (@3a7264e): in_scope items 2/4 + new item,
   AC-02, and the rationale all reworded. Record as D1's Type-1
   design_decision citing this acknowledgement.

2. **[escalate] AC-08 command specs — RATIFIED: SATISFIED-BY-ENGINE.**
   sworn vendors no `commands/` (only the 11 rule docs); the
   implement-slice.md/merge-track.md prose lives in the private
   `~/.claude` harness + upstream baton (ADR-0010 boundary). sworn's
   sworn#80 obligation is the Go behavior (no engine write of track
   worktree/state to board.json), owned by AC-06/AC-07. AC-08 amended
   (@3a7264e) to that engine assertion + "sworn's vendored rules/
   contains no implement-slice.md/merge-track.md" as the
   satisfied-by-engine evidence. The command-spec prose edit is tracked
   baton-side: **baton#61**. Record as D3's design_decision.

3. **[mechanical] status.json design_decisions — ACCEPTED.** Populate
   D1-D5 at the in_progress transition, D1/D2 as Type-1 with recorded
   Coach decisions (D1 = pin 1 above; D2 = pin 4), per the S01/S05/S09
   record shape (Rule 9 gate fails closed on empty).

4. **[memory-cited] D2 track-path derivation — CONFIRMED.** Reuse
   worker.go's repo-local sibling-of-release-worktree logic, NOT the
   naive `$HOME` track-mode formula — this preserves the real prior
   incident's fix (eval finding 3). Move the logic into the shared
   internal/board helper and repoint; do not re-derive the naive
   convention. Cite [[project_driver_contract_recut]].

5. **[mechanical] AC-07 worker.go defaultTrackWorktreePath — ACCEPTED.**
   Sweep callers (worker.go:212), move the logic into the shared
   internal/board helper, repoint, THEN delete — no dead code. Note
   worker.go changed under S14 since base; forward-merge is current.

Proceed to implementation.
