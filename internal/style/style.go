// Package style provides sworn's terminal output styling: a small, hand-rolled
// ANSI palette shared by the CLI entry points (cmd/sworn) and the report
// renderers (internal/*). Sworn is a single binary, so colour is a handful of
// raw escape codes rather than a styling dependency (Rule 1).
//
// Colour is enabled only when stdout is a real terminal and NO_COLOR is unset
// (https://no-color.org). In a pipe, a CI log, a redirected file, or a test
// harness, every helper degrades to plain text — so machine-readable output and
// golden-string tests are never polluted with escape sequences. Set
// SWORN_FORCE_COLOR to override the TTY check (useful for demos and piping into
// a pager that understands ANSI).
//
// Stream gate: Enabled() uses os.Stdout exclusively. Per spec (Risk #3), a
// single global gate is sufficient — if a renderer writes to stderr it is still
// gated on the same stdout TTY check. No per-stream gate is needed.
package style

import "os"

// enabled is computed once at process start from the environment and stdout.
var enabled = detect()

func detect() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("SWORN_FORCE_COLOR") != "" {
		return true
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	// Character device => an interactive terminal. Pipes and regular files are not.
	return info.Mode()&os.ModeCharDevice != 0
}

// Enabled reports whether colour output is active. Renderers that build aligned
// tables can use this to budget for the zero-width escape sequences.
func Enabled() bool { return enabled }

// ANSI SGR codes. Private; callers use the semantic helpers below so the
// vocabulary stays small and consistent across every command.
const (
	reset   = "\033[0m"
	cBold   = "\033[1m"
	cDim    = "\033[2m"
	cRed    = "\033[31m"
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
	cCyan   = "\033[36m"
)

func wrap(code, s string) string {
	if !enabled || s == "" {
		return s
	}
	return code + s + reset
}

// Semantic helpers. Call these, not raw codes.
func Bold(s string) string    { return wrap(cBold, s) }
func Dim(s string) string     { return wrap(cDim, s) }
func Heading(s string) string { return wrap(cBold+cCyan, s) }
func Success(s string) string { return wrap(cGreen, s) }
func Warn(s string) string    { return wrap(cYellow, s) }
func Danger(s string) string  { return wrap(cRed, s) }
func Accent(s string) string  { return wrap(cCyan, s) }

// Verdict colours a PASS/FAIL/BLOCKED token: green for PASS, red for FAIL, yellow
// for anything else (BLOCKED, SKIP, etc.). The argument is returned styled but
// otherwise unchanged, so callers keep control of surrounding text and width.
func Verdict(token string) string {
	switch token {
	case "PASS":
		return Success(token)
	case "FAIL":
		return Danger(token)
	default:
		return Warn(token)
	}
}

// Banner renders the sworn wordmark used as a command header, e.g.
//
//	⚔ sworn · init
func Banner(title string) string {
	b := wrap(cBold+cCyan, "⚔ sworn")
	if title == "" {
		return b
	}
	return b + Dim(" · "+title)
}

// Rule returns a horizontal divider of the given width.
func Rule(width int) string {
	line := make([]rune, 0, width)
	for range width {
		line = append(line, '─')
	}
	return Dim(string(line))
}