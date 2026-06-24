# Journal — S50-baton-governance

## 2026-07-07 — Implementation session

**State transition:** design_review → in_progress → implemented

**Coach approval:** PROCEED (via approved-ack.md, CAPTAIN-VERDICT: PROCEED, two mechanical pins + three flags)

### Pin 1 — design_decisions
Added `design_decisions` array to `status.json` with all 5 design decisions from design.md §2, all classified Type-2 (narrow, reversible, follow existing conventions). This satisfies the designfit gate for `cmd/sworn/baton.go` (architecturally-significant prefix).

### Pin 2 — live-remote diff deferral tracking
Concretised the live-remote diff deferral tracking from "future slice / sawy3r/baton issue" to "S62-baton-upstream-source" (the planned slice for network fetch).

### Flag (a) — governance doc links ADR
`docs/baton-governance.md` links ADR-0006 rather than duplicating its decision rationale. The governance doc is an operational how-to with steps and links.

### Flag (b) — public repo hygiene
Verified `docs/baton-governance.md` contains no private repo refs (no firedau/fired, no release codenames, no slice IDs from other releases). Public-safe.

### Flag (c) — diff vs vendor --check distinction
`sworn baton diff --help` explicitly distinguishes the governance/fail-closed diff surface from the developer dry-run (`sworn baton vendor --check`). The help text notes: "'diff' is the fail-closed governance gate — does the embed match the pinned source?"

### Implementation decisions
- `DiffOpts` is separate from `VendorOpts` (narrower, self-documenting).
- `Divergence` has `File` (embed-relative) + `Reason` (short descriptor).
- `Diff` reuses `batonFileMappings`, `ValidateSource`, and `Transform` from vendor (shared code path, spec Risk #1 satisfied).
- Exit codes: 0 = in sync, 1 = divergent, 64 = usage error (consistent with `sworn baton vendor`).
- ADR-0006 already Status: accepted; no open questions — enforcement now exists.

### Deferrals

- **CI wiring:** `docs/baton-governance.md` recommends wiring `sworn baton diff` into CI. No CI workflow file is created in this slice.
  - **Why:** CI configuration is a separate harness change; this slice delivers the diff command and the governance doc.
  - **Tracking:** S50 proof.md "Not delivered"
  - **Acknowledged**: Coach, 2026-07-07

- **Live-remote diff:** The diff compares against the pinned local source (same source directory S48 vendors from). Live-remote fetch is deferred.
  - **Why:** Network fetch boundary is distinct from local-source diff; requires S62-baton-upstream-source infrastructure.
  - **Tracking:** S62-baton-upstream-source
  - **Acknowledged**: Coach, 2026-07-07

- **Upstream Baton PRs:** Actually filing/merging the upstream PRs for fidelity-layer rules is upstream work tracked at sawy3r/baton#31 — not a sworn slice deliverable.

### Deferral ack durability

Both CI wiring and live-remote diff deferrals are acknowledged inline here and in `proof.md` "Not delivered" — per the durable-inline rule (feedback_deferral_ack_durable_inline), acks are not dependent on transient `approved-ack.md` lifetime.