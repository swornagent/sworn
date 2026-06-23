# Journal — S60-init-ui-bearing-fix

## 2026-06-23 — planned (replan)

- **Actor**: planner (human Brad + Claude)
- Carved out of an ad-hoc fix session. `sworn init` prompts for design tokens
  source + component library even when the repo is not UI-bearing, because the
  apply-phase design-system block is gated on `cfgErr == nil && !cfgExisted`
  (new config) rather than on `--ui-bearing`. A latent `cfg.UIBearing =
  *uiBearer || true` also forces `ui_bearing` true whenever a design system is
  entered. Both defects confirmed present on `release-wt` at plan time.
- A reference implementation exists (authored against the stale `release/v0.1.0`
  base, 379 commits behind release-wt) on branch `wip/cli-styling-reference`.
  `init.go` is structurally identical across the two branches, so the fix ports
  directly; the implementer should still re-derive it against release-wt.
- Sequenced before `S61-cli-output-styling` in T18 — both touch `init.go`.
