# S02-board-render — journal

## 2026-07-01 — Implementer session halted at design-review gate (Rule 9)

**State on entry:** `design_review`, `verification.result: pending`, `design_decisions: null`.
`review.md` terminal verdict was `DECISION: NEEDS_COACH` (escalate Pin 1), no PROCEED
acknowledgement present. Per the Session-start handshake I did not write code, did not
transition to `in_progress`, and did not record the Type-1 decision (Rule 9 forbids the
model recording it).

**Blocker verified live (not recalled):** AC-04 and AC-05 are mutually unsatisfiable as
written.
- AC-04 (unwanted): render must fail closed when `board.json` is invalid against board-v1.
- AC-05 (event-driven): render must succeed against this release's `board.json`.
- Live `board.json` `release` field is the bare string `"2026-06-30-sworn-operational-readiness"`
  (`jq` → `type=string`).
- Canonical board-v1 reader rejects a bare-string release: `internal/board/board.go:54-62`
  — "not a canonical {name} object (a bare string release is no longer read — migrate it
  to {\"name\":...})".

Root cause is a **board-v1 contract skew**, not an S02 design flaw: the in-tree
`board.go` was tightened (T4 / S05, verified+merged) to reject string releases, while the
live `board.json` still carries the string shape and commit `8fadf68` deliberately set it
to string to match the **installed** `sworn` binary. The installed binary and the in-tree
contract disagree about board-v1.

**Coach decision (via /implement-slice session, 2026-07-01):** *Reconcile the contract
skew first.* Neither S02 option (tolerant render reader / migrate-then-strict). Pause S02;
decide the canonical `release` shape once (string vs object); align the installed binary +
`board.json` + `board.go` to that single contract; then re-spec S02 against the settled
contract.

**Next step (not an implementer action):** route to `/replan-release
2026-06-30-sworn-operational-readiness` (or a dedicated contract-reconciliation session).
The Planner records the settled shape + this decision into `status.json.design_decisions`
as the Type-1 record and revises AC-04/AC-05 so they are consistent. S02 remains
`design_review` until then.

## 2026-07-01 — CORRECTION: shape already decided; blocker is the S05 AC-06 cutover

The earlier entry framed this as "decide the canonical shape (string vs object)". That is
WRONG and is corrected here: the canonical `release` shape is **already decided and
documented — OBJECT / strict** — in T4:
- S05-board-canonical-emit AC-03 (Coach-ratified 2026-07-01, "no-wild-data"): reader is
  object-only, a bare string release fails closed; "legacy operator string boards are
  migrated, not read-tolerated — see AC-06".
- S05 AC-06: string boards are migrated `release:"X"` -> `{"name":"X"}` as a one-time
  CUTOVER, applied only once every active session is on a canonical (S04/S05) binary;
  "hold this for the operational-readiness cutover — the op-readiness board itself is among
  the boards to migrate."

So `board.go` rejecting the string is INTENDED, and the string board.json is a
known-deferred un-migrated artefact — not undecided contract. Verified live: the installed
binary is pre-S05 string-tolerant (`8fadf68`, 2026-06-30T14:25:32Z — reads the string board
exit=0); board.json is still string on release-wt too.

**S02 is blocked on the S05 AC-06 operational-readiness cutover, NOT a design decision:**
(1) build+install canonical (S05) `sworn`; (2) get all in-flight sessions (T3, loop) on it
per AC-06 sequencing; (3) migrate this release's board.json to object form; (4) then S02's
AC-04/AC-05 are consistent as written — re-run `/design-review` (expect PROCEED, render via
canonical ReadBoard) then implement. No AC revision needed; a `/replan-release` is only to
record the cutover dependency if desired. Cutover execution is a Coach/operator action
(blast radius: breaks pre-S05 string-binary sessions if mis-sequenced).
