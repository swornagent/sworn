# Coach ack — S22-sworn-doctor

**Date:** 2026-06-22
**Acked by:** Coach (human, brad)
**Captain verdict:** NEEDS_COACH (3 escalated pins) → resolved; PROCEED to in_progress.

The design is sound. All three escalated pins are the same finding — the spec was
written against an assumed Baton structure, but the **actual embed is authoritative**.
I verified the embed; the implementer's proposed answers are factually correct. The
spec has been amended so the verifier gates on reality (not the stale assumptions).

## Pin 1 — single `baton/rules.md` vs multi-file structure → MATCH EMBED

There is no single `baton/rules.md`. The embed is
`internal/adopt/baton/rules/01-*.md` … `10-*.md` (10 files) + `README.md`. doctor
checks that all 10 rule files exist and are non-empty, and that `README.md` carries
its rules-index heading. (Confirmed by `find internal/adopt/baton`.)

## Pin 2 — 7 vs 10 rules → CHECK 10

The protocol genuinely grew. The 10 files are the canonical 7 (reachability-gate …
adversarial-verification) **plus** `08-requirements-fidelity`, `09-design-fidelity`,
`10-customer-journey-validation` — each a distinct rule. doctor checks for 10.

## Pin 3 — `## Phase` vs `### Phase` → CHECK `### Phase`, AND there are 6 phases

`planner.md` uses `### Phase` (h3), and has Phase 1 through Phase 6 (not 1–4 as the
spec said). doctor checks `### Phase 1`…`### Phase 6`. (Confirmed by grep.)

## Coach add-on (not a captain pin, but required) — S18/S19 checks WARN, not ERROR

The design (§2.2) had doctor emit `[ERROR]` for `implementer.md`/`verifier.md`
S19-headings and treated `docs/considerations.md` (S18) as required. Those slices
have not landed. A health tool must **not** report a clean repo as broken for unbuilt
future features. Decision: S18/S19-dependent checks emit `[WARN]` (or skip) until
those slices land — never `[ERROR]`, never a non-zero exit on an otherwise-clean repo.
`TestDoctorAllOK` must exit 0 against today's embed.

## Acknowledged design decisions (not pins — confirmed correct)

- **§2.3 splice marker**: detect `## Engineering Process — Baton`
  (`adopt.BatonSectionHeading`), not `<!-- baton:start -->` (never used). Use the
  `adopt` constant directly. Spec amended.
- **§2.4 `sworn://baton/rules` MCP-pointer check**: deferred (Rule 2) — the `sworn://`
  URI scheme doesn't exist yet; the check would WARN on every repo. Skip for now,
  re-add when the scheme lands. Record the deferral in proof.md.
- **§2.5 injectable registry check**: fine (enables the unreachable test).

## Net

No spec deviation requiring further escalation — these are spec corrections, now
applied to `spec.md` acceptance checks. Implementer: build against the amended spec;
the captain's review.md verdict is resolved. Proceed to `in_progress`.
