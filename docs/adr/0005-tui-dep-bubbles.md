# ADR 0005 — TUI dependency: charmbracelet/bubbles (textinput)

Status: accepted (2026-06-21)

## Context

S04c-tui-resolution adds a deferral-reason text input to the blocked-slice
resolution panel. The Coach design review (Pin 3) decided to use
`github.com/charmbracelet/bubbles/textinput` rather than an inline rune-buffer,
and explicitly instructed: "Do not implement the inline rune-buffer alternative."

ADR-0004 already names "Bubble Tea + Lip Gloss + Bubbles" as the intended TUI
stack and deferred the bubbles import until a slice needed it. This ADR records
that the need has arrived.

## Decision

1. Add `github.com/charmbracelet/bubbles` to `go.mod` at the version resolved
   by `go get` (v1.0.0 at implementation time).
2. Import `bubbles/textinput` in `internal/tui/blocked.go` for the deferral
   reason input field.
3. Pin by `go.sum` hash — no `replace` directives.
4. ADR-0004 point 3 ("no TUI package imports bubbles until a slice needs it")
   is now satisfied — S04c is the slice that needs it.

## Consequences

- Binary size increases slightly (bubbles is a small, pure-Go package).
- The deferral input gets cursor positioning, placeholder text, and character
  width handling for free, rather than a hand-rolled rune buffer.
- Dep policy is satisfied per ADR-0001; this ADR is the record for the
  `go.mod` commit.