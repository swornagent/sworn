# Journal — S73-baton-v0.5.0-pin

## 2026-06-25 — Implementer session: BLOCKED on spec defects, routed to /replan-release

Implementer entered the track worktree
(`/home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T14-baton-integration`,
branch `track/2026-06-19-safe-parallelism/T14-baton-integration`). Gate -1 clean,
Step 0b BLOCKED-guard clear (`verification.result: pending`). Did **not** transition
to `in_progress` or edit any production file — required reading surfaced spec defects
that an implementer may not resolve by inferring scope (Baton Rule 8; Implementer
role "Do not infer scope" / "route to /replan-release").

State left at **`planned`**. No production code touched. This entry is the durable
capture (Rule 3); the handoff is forward to the planner.

### Evidence gathered (all live, this session)

- Current vendored pin (`internal/adopt/baton/VERSION`): `baton-protocol: v0.4.2`,
  `upstream-sha: 729f188f6f69f4b807c5974b33fd39ec98671f15`, `vendored: 2026-07-09`.
  **Not** `cf15842` as the spec premise claims.
- `~/projects/baton` is the canonical upstream and **has tag `v0.5.0`**.
  - `git rev-parse v0.5.0`   (annotated **tag object**) = `b8452dd19fb07e2c869dd30471c7a986b2a2dbc6`
  - `git rev-list -n1 v0.5.0` (**commit**)              = `9ae08fbb1ef28ba5a4918a51018b01ba31b4797b`
- Live GitHub commits API (`sawy3r/baton`, network reachable, 200):
  `GET /repos/sawy3r/baton/commits/v0.5.0` → `"sha":"9ae08fbb1ef28ba5a4918a51018b01ba31b4797b"`.
  This is exactly what the vendor's `resolveCommitSHA` (`internal/baton/fetch.go`)
  returns and what `WriteUpstreamPin` writes to `upstream-sha`.
- v0.5.0 content delta vs the vendored 729f188 (`git diff --stat 729f188 v0.5.0 -- claude/baton/`)
  adds `claude/baton/architecture.json` (+81) and `claude/baton/role-prompts/captain.md`
  (+12, also `extensions.md` +84, `watcher-protocol.md` -117), and rewrites
  `requirements-fidelity.md` (+68), `design-fidelity.md` (+42), `planner.md` (+146),
  `implementer.md`, `verifier.md`.
- Vendor file map `internal/baton/source.go` `batonFileMappings` covers rules 01–10,
  README, and role prompts **implementer/planner/verifier only** — **no `captain.md`,
  no `architecture.json`, no rule-11 (`process-global-mutation`)**. `captain.md`
  already exists in the embed (`internal/prompt/captain.md`) but is **unmapped**, so a
  re-vendor will not refresh it; `architecture.json` is absent from the embed entirely.
- `sworn baton diff` / `vendor` only inspect **mapped** files, so they would report
  **zero divergence** even though the embed lacks `architecture.json` and carries a
  stale `captain.md` — a false green against AC3/AC5/AC6.

### Defects requiring planner resolution (/replan-release)

- **D1 (decisive, un-satisfiable + machinery-breaking).** AC1 requires VERSION to
  "reference commit `b8452dd`." `b8452dd` is the **tag-object** hash; the vendor
  resolves and pins the **commit** `9ae08fb` (confirmed live). Forcing `b8452dd`
  into `upstream-sha` makes the next `FetchUpstream` abort ("upstream SHA mismatch …
  tag may have been force-moved", `fetch.go:77`), breaking the S62/`--upstream`
  governance gate. The existing pin already uses a **commit** SHA (`729f188…`), so
  the established convention contradicts AC1. **Fix:** AC1 → `9ae08fb` (commit), and
  state whether `vendored:` should be `2026-06-25` (today) given the spec was authored
  with that date.
- **D2 (scope/mechanism gap).** In-scope names `architecture.json` and `captain.md`
  as content to vendor, but `batonFileMappings` carries neither and the embed has no
  `architecture.json`. Fully vendoring v0.5.0 requires **extending `internal/baton/source.go`**
  (new mappings + embed destinations), registering the new embed file(s) in the
  `internal/adopt` / `internal/prompt` embed FS, and a **design decision** on where
  `architecture.json` embeds (`internal/adopt/baton/` vs `internal/prompt/baton/`) and
  whether `extensions.md` / rule-11 / the `watcher-protocol.md` deletion are in scope.
  None of this is in the Planned-touchpoints, which frame the slice as a pure
  re-vendor. This is a Type-1-flavoured design choice (new embed wiring) the planner/
  Captain should scope, not the implementer by inference.
- **D3 (minor).** The `--upstream` path never updates `baton-protocol:` / `vendored:`
  (only `upstream-sha`/`upstream-digest`). AC4 (`sworn version` shows v0.5.0) needs a
  manual VERSION edit that In-scope should list as an explicit step.
- **D4 (cosmetic).** Spec premise "from `cf15842` (pre-v0.5.0)" is stale — current pin
  is `v0.4.2 / 729f188`. Correct the description.

### Handoff

Route to `/replan-release 2026-06-19-safe-parallelism` to amend the S73 spec:
fix AC1 to the commit SHA `9ae08fb`; add `internal/baton/source.go` + embed
registration to In-scope/touchpoints with the `architecture.json` embed-location
decision; add the explicit VERSION `baton-protocol`/`vendored` bump step; correct the
stale premise. No implementer workaround is permitted for these (handoff directionality
— forward to planner, per session-discipline). Slice remains `planned`.
