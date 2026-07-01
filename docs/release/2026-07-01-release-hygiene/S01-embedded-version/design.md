---
title: 'Design TL;DR — S01-embedded-version'
description: 'Design plan for a single embedded version source, submitted for Rule 9 design review before implementation.'
---

# Design TL;DR: S01-embedded-version

## Approach

Introduce one new package, `internal/version`, holding the version string as
a `//go:embed`'d text file. `cmd/sworn/main.go`'s existing `version` package
var stops being a hardcoded `"0.0.0-dev"` literal and instead resolves to
this embedded value when ldflags didn't set it — preserving the existing
override mechanism rather than replacing it.

## Key design choice: resolve at runtime, not at compile time

`-X main.version=<value>` (the existing ldflags override, AC-03) only
reliably overrides a package-level string var that has **no computed
initializer** — `var version string` or `var version = "literal"`. If
`main.go` instead wrote `var version = internalversion.Get()`, Go's
generated package-init code would run that function call at program startup
and silently stomp whatever the linker patched in, breaking the release-build
override path. So:

- `cmd/sworn/main.go` keeps `var version string` (empty zero-value, still
  ldflags-patchable).
- At the top of `main()`, before `version` is used anywhere: `if version ==
  "" { version = internalversion.Get() }`. A plain `go build`/`go install`
  never sets `version` via ldflags, so it stays `""` until this line resolves
  it to the embedded default. A `make build` / release build that passes
  `-ldflags "-X main.version=X"` has already patched `version` to `X` before
  `main()` runs, so the `== ""` check is false and the override wins
  untouched.
- This resolves once, centrally, rather than duplicating the fallback check
  at both existing use sites (`cmdVersion` at line 82, `telemetry.Fire` at
  line 51) — bounding the main.go diff to two lines.

## Files touched

- `internal/version/version.txt` (new) — the version string, initial value
  `0.1.0`. Single source of truth (AC-01).
- `internal/version/version.go` (new) — `//go:embed version.txt` + `Get()
  string` returning the trimmed contents.
- `internal/version/version_test.go` (new) — `Get()` returns a non-empty,
  whitespace-trimmed string; embed actually loads (AC-05).
- `cmd/sworn/main.go` — `var version = "0.0.0-dev"` → `var version string`;
  add the one-line resolution at the top of `main()` (AC-02, AC-03).
- `Makefile` — `VERSION ?= 0.1.0` → `VERSION ?= $(shell cat
  internal/version/version.txt)`, so `make build`'s ldflags-injected value
  and the embedded default read from the same file (AC-04).

## Why `internal/version/version.txt`, not a Go const

A plain string constant in Go source (`const Version = "0.1.0"`) would work
functionally, but re-introduces exactly the two-copies-of-the-version problem
this slice exists to close if the Makefile can't `cat` a `.go` file for its
own `VERSION ?=` line without a fragile grep/sed against Go syntax. A
`.txt` file is trivially readable by both `go:embed` and `$(shell cat ...)` —
one file, two consumers, textually identical read.

## Risks / pins for the reviewer

- **None functional** — this is additive (`go:embed`) plus a two-line change
  to an existing var's initializer and its resolution point; no existing
  behavior changes for any build that already passes `-ldflags`.
- **Scope note**: this slice does not touch `sworn doctor`'s version-drift
  detection or add a CI gate — that is S02-version-bump-ci-gate, next in this
  track, per spec's stated out-of-scope boundary.
