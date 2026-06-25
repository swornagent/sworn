# Captain review — S49-baton-version
Date: 2026-06-23
Design commit: 7093b0c0e4d1b28e1e8b9460ecb51588474dc9be

## Pins

1. [mechanical] §3 — `cmd/sworn/main.go` in design §3 and `status.json` `planned_files` contradicts spec
   What I observed: Spec "Entry point" section reads "reframed to the semver 'on Baton' line **by changing `baton.Version()` / `prompt.BatonVersion()`, NOT by editing `cmd/sworn/main.go` or `cmd/sworn/commands.go` (those are T15-owned)**." Spec planned touchpoints list omits main.go entirely. Yet design §3 adds `cmd/sworn/main.go` ("cmdVersion format string changed to SwornAgent %s\non Baton %s") and status.json `planned_files` includes it. This is the **fourth occurrence** of the main.go-in-planned_files Gate 2 failure pattern in this release (S19, S21, S48 all tripped on it — see trial log). Verifier will fail closed when actual_files conflicts with this constraint.
   What to ask the implementer: Drop `cmd/sworn/main.go` from design §3 and from `status.json.planned_files`. Achieve the AC (`sworn version` output contains "on Baton v") WITHOUT touching main.go: have `prompt.BatonVersion()` return `"on Baton " + baton.Version()` (e.g. `"on Baton v0.3.0"`). The current format string `baton-protocol %s` then produces `baton-protocol on Baton v0.3.0` — contains "on Baton v", AC passes. The "SwornAgent" label (§1 claim) cannot be achieved without main.go, so update §1 to reflect the actual output when main.go is not touched. If the "SwornAgent" label is non-negotiable, document the spec deviation explicitly before code.

2. [mechanical] §2 Decision 5 — `SetVersionForTest` as an exported production function
   What I observed: Design Decision 5 places `baton.SetVersionForTest(v string)` in `internal/baton/version.go` (production code) to allow tests to inject a forced SHA. Exporting a mutation setter from a production package exposes a test-only mutation surface to any caller, which is non-idiomatic Go. The project's existing pattern (doctor.go `var checkDepFreshness = defaultCheckDepFreshness`) uses an unexported package-level var that tests can set within the same package or via `export_test.go`.
   What to ask the implementer: Use an unexported var `var versionForTest string` in `version.go`, checked first by `Version()`. Expose it via an `internal/baton/export_test.go` file (`func SetVersionForTest(v string) { versionForTest = v }`). This matches Go convention and eliminates the exported mutation surface from production code.

3. [memory-cited] §2 Decision 2 — single accessor `baton.Version()` as canonical source
   What I observed: Design Decision 2 picks a single `baton.Version()` accessor reading from `adopt.BatonDocsFS() → baton/VERSION`, with `prompt.BatonVersion()` delegating to it, eliminating the two-source divergence. This directly implements [[project_baton_sworn_architecture]]'s vendor-down flow: "version surfacing — `sworn version`/`doctor` report 'on Baton vX.Y.Z'; pin by tag not SHA." The memory explicitly names this as the desired outcome.
   What to ask the implementer: Confirm the single-accessor decision intentionally honours [[project_baton_sworn_architecture]] — quick ack.
   Citation: [[project_baton_sworn_architecture]]

---

## Summary

Pins: 3 total — 2 [mechanical], 1 [memory-cited], 0 [escalate]
Critical pins: **Pin 1** — `cmd/sworn/main.go` in planned_files is a known Gate 2 failure; shipping with main.go in actual_files contradicts the spec entry point constraint and will FAIL verification.

## Smaller flags (not pins, worth one-line ack)

(a) Decision 4 tightens the existing `VERSION.txt` check in `checkEmbeddedPrompts` to ERROR on non-semver (currently WARN only on `0.0.0`/`-dev`). This isn't explicitly in spec scope — the spec's doctor check is about the `baton-protocol:` pin line, not the VERSION.txt line. The tightening is defensible (consistent with `IsSemverTag`) but confirm it's intentional addition rather than accidental scope drift.

(b) If Pin 1 fix is applied (drop main.go), §1's "SwornAgent vA.B.C\non Baton v0.3.0" output claim must be updated — the "SwornAgent" label comes from the main.go format string; without that change the first line remains "sworn 0.0.0-dev" (or whatever the build version is). Update §1 to reflect the achievable output.

(c) `version.go` adds a new import edge `internal/baton → internal/adopt`. This is expected and correct — baton package reads the embed via adopt; no circularity; no standalone-baton requirement exists. No action needed.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Design sound overall — one critical mechanical fix before code. 3 pins:

1. **Drop main.go from planned_files and design §3.** This is the fourth recurrence of the main.go-in-planned_files Gate 2 failure (S19/S21/S48). Spec entry point says "NOT by editing `cmd/sworn/main.go` (T15-owned)" and spec planned touchpoints omit it. Fix: have `prompt.BatonVersion()` return `"on Baton " + baton.Version()` (e.g. `"on Baton v0.3.0"`) — the existing format string `baton-protocol %s` then produces `baton-protocol on Baton v0.3.0`, which contains "on Baton v" and passes the AC. Remove `cmd/sworn/main.go` from `status.json.planned_files` and from §3 before writing any code. Also update §1 to reflect the output without the "SwornAgent" label (which requires main.go).

2. **`SetVersionForTest` via export_test.go, not production code.** In `version.go`, use an unexported var `var versionForTest string`; `Version()` checks it first. Expose it in a new `internal/baton/export_test.go` file: `func SetVersionForTest(v string) { versionForTest = v }`. Matches the project's established injectable-var pattern.

3. **Decision 2 honours [[project_baton_sworn_architecture]] — confirmed.** Single accessor (`baton.Version()` from adopt embed, `prompt.BatonVersion()` delegates to it) is the explicit outcome the architecture memory names. No change needed.

Flags (not pins): (a) Decision 4 tightens VERSION.txt to ERROR on non-semver — confirm intentional scope addition; (b) §1 update required if main.go dropped (see Pin 1).

§2 decisions 1–4 ack (regex strictness, version source, output format, doctor placement all sound). §6 empty — no open questions.

Address pins 1–2 inline before transitioning to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Both pins are unambiguous apply-inline mechanical fixes; Pin 1 has the established fix pattern from S19/S21/S48 precedent; no human judgement needed.
-->
