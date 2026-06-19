# Journal: `S08-design-system-input`

## Session log

### 2026-06-20 02:49 — start implementation

- **State**: `planned → in_progress`
- **Notes**:
  - Set up status.json with state: in_progress and start_commit.
  - Created track worktree already existed at `/home/brad/projects/sworn-worktrees/release-2026-06-16-fidelity-layer-T3-leaf-gates`.

### 2026-06-20 02:54 — implement design system types and validation

- **State**: `in_progress`
- **Notes**:
  - Added `DesignSystem` struct with `TokenSource` and `ComponentLibrary` fields to `internal/config/config.go`.
  - Added `UIBearing` bool and `DesignSystem *DesignSystem` fields to `Config` struct.
  - Added `Validate()` method to `Config` that returns `ErrNoDesignSystem` when `ui_bearing: true` but no design system declared.
  - Updated `DefaultConfig()` to set `UIBearing: false, DesignSystem: nil` (sworn is a CLI tool).
  - Added `PromptDesignSystem()` function in `internal/config/init.go` for interactive prompting.
  - Updated `cmd/sworn/init.go` to add `--ui-bearing` flag and design system prompting in scan + apply phases.
  - Updated `internal/adopt/baton/rules/09-design-fidelity.md` with `## Design-system input` section documenting the three-tier concept.
  - **Key design decisions:**
    - Design system lives in the same Config struct as verifier settings, not a separate file — keeps the config surface minimal.
    - The `--ui-bearing` flag marks a project explicitly; without it, `sworn init` assumes CLI (non-UI-bearing).
    - The `PromptDesignSystem` function is in `internal/config/init.go` (the planned touchpoint) alongside `Scaffold`.
    - `cmd/sworn/init.go` was an unplanned file but necessary for the init prompting integration.
    - Token format hint is not mandated in the schema — S09's audit will adapt to format.

## Open questions

- None.

## Deferrals surfaced

- None.

## Verifier verdicts received

- Pending (slice not yet verified).