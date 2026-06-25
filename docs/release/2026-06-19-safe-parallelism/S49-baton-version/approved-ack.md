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
