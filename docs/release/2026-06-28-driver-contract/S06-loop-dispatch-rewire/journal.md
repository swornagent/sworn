# Journal — S06-loop-dispatch-rewire

## 2026-07-10 — Session 1 (Implementer): design TL;DR produced, halted at design_review

- Grounded the design against live worktree code: `internal/run/slice.go`
  (factory fields + nil-default patch at 57-63/193-201, terminal check at
  ~487, five dispatch sites), `internal/verify/verify.go` (RunAgentic +
  acceptStructuredVerdict), `internal/implement/{implement,ready}.go` (DoR
  agentVerifier seam), `internal/driver/*` (S01-S04 contract + drivers),
  `internal/driver/registry/registry.go` (S05), `internal/model/config.go`
  (FromEnv proxy block — the R-04 duplication source).
- Wrote `design.md`. Ten decisions D1-D10; the ones needing reviewer eyes
  are flagged inline and re-listed in "Design-level risks / pins":
  D2 (RoleCaptain only on in-process drivers; captain-leg resolve failure →
  existing design-gate deferral, not hard halt — an AC-02 interpretation),
  D7 (ProviderConfigFromEnv gains SWORN_* fallbacks so the loop keeps
  honouring SWORN_*-only setups), D9 (delete dead InterpretVerifier field).
- R-03 answered with `driver.TerminalErrKind` (set exactly {auth, credits},
  per the S04 Coach ack binding record) consumed at both the implement leg
  and the verify leg — one predicate, four halt tests planned.
- R-04 answered with extracted `model.ProxyRoute` (single predicate) +
  `model.ResolveLoopClient` as the in-process drivers' FromEnv-equivalent
  client default + registry delegation; three-part reachability test
  (advertise / observe proxy hit server-side / SWORN_DIRECT flips both).
- Discovery beyond the spec's touchpoints (named in design.md "Files to
  touch", to land in this slice because the seam forces them):
  cmd/sworn/verify.go's agentic path also calls RunAgentic;
  run.Options carries duplicate factory fields; inprocess default timeout
  (300s) is shorter than DefaultImplementTimeout (15m) so DispatchInput.Timeout
  must be passed explicitly or implement legs get silently capped.
- **R-02 memory note (named here per the spec's mitigation):** the
  S05-strict-reader lesson — a tightened reader/contract regressed board.json
  fixtures in OTHER packages (internal/board + cmd/sworn). Before any
  transition to `implemented`, run the FULL `go test -timeout 300s ./...`;
  the highest cross-package risk for this slice is cmd/sworn (three files
  compile against re-cut signatures) and internal/scheduler (worker fakes).
- No production code written this session (Rule 9 — design review before
  code). State: planned → design_review. Next: `/design-review
  S06-loop-dispatch-rewire 2026-06-28-driver-contract` (Captain), then Coach
  acknowledgement, before implementation resumes.
