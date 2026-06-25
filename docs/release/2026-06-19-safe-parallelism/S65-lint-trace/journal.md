# Journal — S65-lint-trace

## Planner corrections

### 2026-06-25 — spec corrected; BLOCKED verdict cleared (Step 2b)

- **Actor**: planner (`/replan-release`, human + Claude).
- **Trigger**: verifier returned BLOCKED — spec AC #1 specified `sworn lint trace --release <name>` (flag form) while the implementation, entry-point description, proof reachability artefact, and all tests use positional `sworn lint trace <release>`. A spec/implementation contract mismatch the verifier cannot grade and the implementer cannot fix without changing the spec.
- **Adjudication**: the implementation is **correct**. Every sibling `sworn lint` subcommand — `ac`, `deps`, `touchpoints`, `symbols`, `status` (`cmd/sworn/lint.go`) — takes the release as a **positional** argument; none use `--release`. The spec's `--release` flag was the defect: inconsistent with the established CLI family (S29/S30/S31 + the S51 registry). This ratifies the verifier's own proposed amendment.
- **Correction** (release-wt):
  - User outcome: `--release <name>` → positional `<release>`, with an explicit note that there is no `--release` flag.
  - Entry point: made explicit — `sworn lint trace <release>`, sole positional arg (`fs.Arg(0)`), matching the lint family convention.
  - AC #1: `sworn lint trace <release>` (positional, no flag) exits 0 on fully-traced release.
  - Reachability artefact: `--release <fixture-release>` → `<fixture-release>`.
- **State**: `verification.result` cleared `blocked` → `pending`; violations cleared; `state` set to `implemented` (the existing implementation already satisfies the corrected spec). `start_commit` (45ab01f) and `actual_files` preserved from the track branch.
- **Next**: a fresh `/verify-slice S65-lint-trace 2026-06-19-safe-parallelism` re-enters verification against the corrected, consistent contract. No code change required.
