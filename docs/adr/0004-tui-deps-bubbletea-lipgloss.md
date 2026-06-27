# ADR 0004 — TUI dependencies: Bubble Tea + Lip Gloss

Status: accepted (2026-06-21)

## Context

S04a-tui-foundation introduces a Bubble Tea TUI for `sworn` when invoked with no
arguments (and `sworn top` with no release arg). This requires two Go packages:

- `github.com/charmbracelet/bubbletea` — the Bubble Tea TUI framework (model,
  update, view loop, keyboard handling)
- `github.com/charmbracelet/lipgloss` — style / layout helpers for terminal
  rendering (colours, borders, alignment)

ADR-0001 (one binary, embedded protocol) already names "Bubble Tea + Lip Gloss
+ Bubbles" as the intended TUI stack. This ADR records the specific dep addition
commit and version pin.

## Decision

1. Add `github.com/charmbracelet/bubbletea` and
   `github.com/charmbracelet/lipgloss` to `go.mod` at versions resolved by
   `go get` at implementation time (minimum: bubbletea v1.x, lipgloss v1.x).
2. Pin by `go.sum` hash — no `replace` directives.
3. No TUI package (`internal/tui/`) imports `bubbles` (the third component from
   ADR-0001) until a slice needs it — avoid pulling an unused dep.

## Consequences

- Binary size increases by ~2–3 MB (bubbletea + lipgloss).
- Dep policy is satisfied per ADR-0001's architectural decision; this ADR is the
  record for the `go.mod` commit.
- `go test ./internal/tui/...` runs without a TTY (model-state tests only).