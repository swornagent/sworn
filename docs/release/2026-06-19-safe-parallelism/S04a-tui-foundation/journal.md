# Journal — S04a-tui-foundation

## Session: 2026-06-21 — Implementation

### State transitions
- `design_review` → `in_progress` — Coach approved design (approved-ack.md present, 5 pins addressed)
- `in_progress` → `implemented` — code written, tests pass, proof bundle generated

### Pre-code steps (Coach pins)
1. **Pin 1** (go.mod/go.sum in planned_files): Added both to status.json planned_files
2. **Pin 2** (ADR for TUI deps): Created ADR-0004 (bubbletea + lipgloss). ADR-0001 already decided BT was the TUI stack; ADR-0004 records the specific dep addition.
3. **Pin 3** (tui.Run() location): Added `internal/tui/tui.go` to planned_files and created it as the Run() entry point. Matches touchpoint matrix.
4. **Pin 4** (design_decisions): Transcribed all 5 Type-2 §2 decisions into status.json.design_decisions. `sworn designfit 2026-06-19-safe-parallelism` PASS (all 32 slices).
5. **Pin 5** (Q1 spec-answered): Already resolved — no action needed.

### Design decisions
- D1: No-args routes to tui.Run() instead of usage()+exit(64) — Type-2, acked
- D2: sworn top (no arg) → TUI, sworn top <release> → evidence surface — Type-2, acked
- D3: Data from git rev-parse + filepath.Glob — Type-2, acked
- D4: tea.Model pattern with viewState enum — Type-2, acked
- D5: Pure model-state unit tests, no TTY — Type-2, acked

### Key technical decisions
- Model exposes `Releases` and `Board` as exported fields for S04b/S04c extension
- Frontmatter parsing uses yaml.v3 (standard Go YAML package)
- findRepoRoot uses `git rev-parse --show-toplevel` with os.Getwd() fallback
- Binary size: 18MB (includes bubbletea + lipgloss + sqlite deps)
- All 5 unit tests pass (model-state, no TTY)

### Skeptic panel
- Runtime does not support parallel subagent dispatch — panel skipped. Noted `skeptic_panel: skipped — runtime does not support subagent dispatch`.

### First-pass verification script
- `release-verify.sh S04a-tui-foundation 2026-06-19-safe-parallelism` → **PASS** (23/23 checks)
- Result captured in proof.md "First-pass script output" section

### Open items- None — slice implementation complete

### Deferrals
- Live concurrent status from SQLite DB — deferred to S04b (spec §Out of scope)
- Blocked-slice TL;DR panel — deferred to S04c (spec §Out of scope)
- Credits display — deferred to S04b (spec §Out of scope)
- Mouse support — deferred (spec §Out of scope)