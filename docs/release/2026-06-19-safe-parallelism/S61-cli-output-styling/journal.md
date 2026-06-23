# Journal — S61-cli-output-styling

## 2026-06-23 — planned (replan)

- **Actor**: planner (human Brad + Claude)
- Scope: a shared `internal/style` package + premium, consistent colour across the
  whole CLI command surface and the delegated report renderers. TTY/`NO_COLOR`
  aware; plain output byte-identical so golden tests pass unchanged.
- **Base divergence**: a reference implementation was authored against
  `release/v0.1.0`, which is **379 commits behind** `release-wt`. release-wt's
  command surface is larger (account/doctor/induction/login/mcp/memory/telemetry/
  verify were added by later tracks) and `main.go` is now command-registry-based,
  not switch-based. The reference diff lives on `wip/cli-styling-reference`.
  Implementer: reuse `internal/style` verbatim; re-apply command-layer styling
  against release-wt's real surface (all 21 command files); do NOT port the stale
  `main.go`.
- **Touchpoints**: S61 shares files with three not-yet-started planned slices —
  S27-public-readiness-scrub (T10: main.go, bench.go), S17-tui-provider-config
  (T6: top.go), S59-scheduler-relayer (T17: run.go). Resolved by making T6/T10/T17
  `depends_on T18-cli-polish` so T18 lands first; no concurrent edit.
- Sequenced after `S60-init-ui-bearing-fix` in T18 (both touch init.go).
