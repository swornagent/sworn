# Journal — S04-typed-reference-ambiguity

## 2026-07-17T05:42:04+10:00 — Implementer design checkpoint

- Transitioned `planned -> design_review`; `start_commit` remains unset. No
  production code, tests, S20 artefacts, homes, release-wt bytes, or main-branch
  bytes were changed.
- Confirmed through the release oracle that S01 and S19 are verified, S02 is
  deferred, and S04 is now the next T1 slice before blocked S20. The current
  T1 worktree is clean and its branch is the required
  `track/2026-07-15-baton-v0.15-conformance/T1-foundation`.
- Confirmed the boundary from live code and S20 evidence: generic parsing
  validates the schema but drops the model-emitted `check` before comparing it
  to the requested type; S20's canonical fixture update does not own that
  production correction and its `ac-satisfaction` evidence remains blocked.
- Confirmed C-02's dedicated schema and typed-reference rules are already
  vendored, but the current generic gate still routes `spec-ambiguity` through
  the generic report shape. The proposed design moves resolution, rendering,
  dedicated parsing, and generic identity binding behind one gate authority,
  with thin CLI and MCP adapters.
- Single Captain pin: generic vendored prompts omit `check` while the exact
  vendored schema requires it. Do not change those parity-controlled bytes or
  weaken identity enforcement. Captain must decide whether the S04 gate may add
  a non-vendored runtime output-contract instruction within its current scope,
  or whether planning must amend S04's planned-file/scope boundary first.
- No cross-track collision is present: all declared implementation surfaces are
  T1-only. S20 remains blocked until a fresh S04 verifier PASS; no workaround
  or S20 mutation is authorized.

## 2026-07-17T05:59:16+10:00 — Automatic Coach acknowledgement and Captain PROCEED

- Under the Coach's standing instruction to orchestrate this release, the
  Captain's `PROCEED` verdict in `review.md` (commit `de87826`) is
  acknowledged. There are no `[escalate]` pins and no new Type-1 decision to
  seek.
- Apply pin 1 inline: preserve the exact v0.15.1 vendored prompt and user
  payload bytes. Use the exact generic schema only as a separately labelled,
  schema-constrained output envelope; never synthesize `check` or fall back to
  unconstrained text.
- Apply pin 2 inline: prove wrong, missing, and unknown emitted identities fail
  through the public CLI and registered MCP paths.
- Apply pin 3 inline: reject retired generic `maintainability-review` before
  release/model/diff work in the gate, CLI, and MCP, with zero calls and no
  mutation.
- Apply pin 4 inline: retain typed `spec.references` and the dedicated
  ambiguity report as the sole ambiguity authority.
- Proceed to `in_progress` only in a fresh Implementer session. That session
  must stop at `implemented`; a fresh S04 verifier PASS is the only event that
  may unblock S20, which must then rerun its own readiness and maintainability
  evidence.
