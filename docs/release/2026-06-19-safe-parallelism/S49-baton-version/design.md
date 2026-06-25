# S49-baton-version — Design TL;DR

## §1. User-visible change

`sworn version` currently prints `sworn 0.0.0-dev\nbaton-protocol v1.0.0` — two
unrelated strings from different sources. After S49, it prints `SwornAgent
vA.B.C\non Baton v0.3.0` — a single semver tag read from the one canonical
source (`internal/adopt/baton/VERSION`). `sworn doctor` gains a check in Group 1
that fails closed (non-zero exit) if the embedded pin is a 40-char SHA instead of
a semver tag.

## §2. Design decisions not in spec (max 5)

1. **`IsSemverTag` regex**: `^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$` —
   strictly `vMAJOR.MINOR.PATCH`, no pre-release/build suffixes. Rationale: Baton
   publishes simple tags (`v0.3.0`); stricter is safer for the "is it a tag?" gate.
2. **Version source strategy**: `baton.Version()` reads from
   `adopt.BatonDocsFS()` → `baton/VERSION`, parses the `baton-protocol:` line.
   `prompt.BatonVersion()` delegates to `baton.Version()`. `VERSION.txt` is
   updated to match for the embed transition but is no longer the authority —
   this avoids two accessors reading different files.
3. **Output reframing**: "SwornAgent vA.B.C\non Baton vX.Y.Z" — two-line format.
   Fits the existing `cmdVersion` function trivially; no structural changes to
   `cmd/sworn/main.go` beyond the format string. The `main.version` value stays
   as-is (build-time ldflags), untouched.
4. **Doctor check placement**: Added as the last check in Group 1
   (`checkEmbeddedPrompts`), after the existing VERSION.txt check. Both checks
   now read the same embedded file — the old VERSION.txt check remains for
   backward compatibility but its logic is tightened to ERROR on non-semver
   (currently it only WARNS on `0.0.0`/`-dev`).
5. **Injection point for forcing-SHA tests**: The doctor pin check calls
   `baton.Version()` which reads from the embed at init time. To test failure
   mode, we inject via a package-level override
   `baton.SetVersionForTest(v string)` that tests set and `Version()` checks
   first. Cleaner than a global `os.ReadFile` mock.

## §3. Files I'll touch grouped by purpose

- **New accessor**: `internal/baton/version.go` — `Version()`, `IsSemverTag()`,
  `SetVersionForTest()`. The single source of truth for what Baton version the
  binary carries.
- **New tests**: `internal/baton/version_test.go` — table-driven
  `TestIsSemverTag`, `TestVersionIsSemverNotSha` reading from the real embed,
  `TestVersionIsSemverTag` format check.
- **Pin reconciliation**: `internal/adopt/baton/VERSION` — replace SHA with
  `v0.3.0` on the `baton-protocol:` line.
- **Prompt agreement**: `internal/prompt/VERSION.txt` — update to `v0.3.0`.
- **Delegation**: `internal/prompt/prompt.go` — `BatonVersion()` delegates to
  `baton.Version()` instead of reading its own `VERSION.txt`.
- **Version output**: `cmd/sworn/main.go` — `cmdVersion` format string changed
  to `SwornAgent %s\non Baton %s`.
- **Doctor check**: `cmd/sworn/doctor.go` — add pin-is-a-tag check in
  `checkEmbeddedPrompts`; tighten existing VERSION.txt check to ERROR on
  non-semver.
- **Doctor tests**: `cmd/sworn/doctor_test.go` — `TestDoctorReportsBatonTag`,
  `TestDoctorFailsOnShaPin`, and verify `TestDoctorAllOK` still passes.

## §4. Things I'm NOT doing

- Not changing the vendor/transform mechanism (S48's domain).
- Not adding a `sworn baton diff` or governance doc (S50).
- Not changing protocol content or `rules-added` provenance lines.
- Not adding networked upstream verification of the tag.
- Not adding `version` command logic to main.go beyond the mechanical
  format-string update — the output reframing is data-driven (`baton.Version()`
  returns the semver tag, `main.version` is the SwornAgent version).

## §5. Reachability plan

Two CLI invocations as artefacts:
1. `sworn version` output showing `on Baton v0.3.0`
2. `sworn doctor` output showing the Baton tag line (exit 0), and a forced-SHA
   failure run (exit non-zero with `[ERROR]`)

Pasted into `proof.md`.

## §6. Open questions for the Coach

None.