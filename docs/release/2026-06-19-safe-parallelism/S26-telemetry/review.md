# Captain review — S26-telemetry
Date: 2026-06-21
Design commit: 58ae0ffff2d1175cc125c7a3a8c5b0f931b4fa98

## Pins

1. [mechanical] §4.NOT1 — ACs 1 & 2 deferred to T3/S09 without a formal Rule 2 deferral entry
   What I observed: Design §4 says "Not modifying cmd/sworn/init.go — T3/S09 owns the init flow; the spec's AC for sworn init consent is accepted as a cross-track dependency that will be verified when S09 lands." status.json.open_deferrals is empty []. AC1 (sworn init interactive consent) and AC2 (--non-interactive defaults to off) are in S26's spec but S26 does not implement them. The Verifier for S26 will see both ACs fail unless the deferral is formally declared.
   What to ask the implementer: Add a Rule 2 deferral to status.json.open_deferrals for AC1 and AC2: why (cmd/sworn/init.go is owned by T3/S09; T9 only provides the callable ShowConsent()), tracking (S09-per-role-model-config planned_files includes cmd/sworn/init.go), acknowledgement (Coach noted below). Without this, the Verifier will BLOCKED on missing AC coverage.

2. [mechanical] §3 — cmd/sworn/main.go claimed by four other in-flight tracks simultaneously
   What I observed: S04a-tui-foundation (T2), S06a-sworn-login-auth (T3), S08a-mcp-transport (T4), and S23-memory-config (T8) all list cmd/sworn/main.go in their planned_files. S26's change is structural (extract entire switch into dispatch()), not additive (add one case). Every subsequent lander must merge a restructured file, not just add a case to an existing switch.
   What to ask the implementer: Note in design §4 that four parallel tracks also plan cmd/sworn/main.go. Confirm with the assembler that T9 merges before or that each second-lander's implementer is aware of the restructured signature. At minimum, add a §4 NOT-doing note: "NOT sequencing main.go merges — assembler coordinates cross-track hunk order."

3. [mechanical] §2.D1 — dispatch() restructure must handle version/help cases that don't call os.Exit
   What I observed: Current main.go (lines 75–87): `case "version", "--version", "-v": fmt.Printf(...)` and `case "help", "--help", "-h": ... return`. Neither calls os.Exit. Design §2.1 describes converting "Each case currently calls os.Exit(cmdXxx(...)) directly; with the wrapper, os.Exit moves to after the telemetry fire" — but this framing implies every case has an os.Exit. Two don't.
   What to ask the implementer: Confirm dispatch() explicitly returns 0 for the version and help cases. Grep for all cases that do not end with os.Exit(cmdXxx(...)) before extracting.

4. [escalate] §6.Q1 — meta-command telemetry exclusion (sworn telemetry *) not in spec
   What I observed: Design §2.D1 says "The telemetry subcommand itself does NOT fire a telemetry event (meta-command exclusion)." The spec has no mention of this exclusion — "every sworn invocation fires a non-blocking telemetry event." The design's rationale (paradoxical to send "telemetry was turned off") is sound, but this is a behaviour omission from the spec. Also unstated: whether sworn version and sworn help are excluded.
   What to ask the implementer: Coach acks whether (a) sworn telemetry * is excluded from firing telemetry, and (b) whether sworn version / sworn help are also excluded or are treated as regular invocations.

5. [escalate] §6.Q2 — os.UserConfigDir() diverges from spec's literal ~/.config/sworn/
   What I observed: All 11 spec ACs reference ~/.config/sworn/ path literals (e.g. "creates ~/.config/sworn/.telemetry-enabled"). Design Decision 2 uses filepath.Join(os.UserConfigDir(), "sworn") — identical on standard Linux, but differs when $XDG_CONFIG_HOME is set (non-standard Linux), on macOS (~/Library/Application Support/sworn/), and on Windows (%AppData%/sworn/). AC-level verification on macOS would fail for every sentinel file check.
   What to ask the implementer: Coach picks: (a) spec literal ~/. config/sworn/ — simpler, matches all ACs verbatim, Linux/CI-targeted; (b) os.UserConfigDir() — cross-platform, requires spec ACs to be rewritten with platform-neutral language. Option (a) prioritises spec fidelity and verifier simplicity; option (b) prioritises portability but requires replan.

6. [mechanical] §2.D4 — ShowDisclosure adds a neutrality precondition absent from spec
   What I observed: Spec defines ShowDisclosure as "called only when ~/.config/sworn/.telemetry-disclosed does not exist; writes the sentinel after printing." Design §2.4 adds: "The disclosure only prints if neither opt-in nor opt-out sentinel exists (neutral/undecided state)." This is a more restrictive check — on a machine where .no-telemetry exists but .telemetry-disclosed was deleted, the spec would re-display the disclosure but the design would not.
   What to ask the implementer: Confirm the neutrality precondition is intentional. Also confirm the reachability artefact (rm -f .telemetry-disclosed && sworn version) is run against a clean config dir (no .telemetry-enabled or .no-telemetry present) so the extra condition doesn't silently suppress the disclosure.

7. [mechanical] §3 — ShowConsent() has no spec definition: no signature, contract, or test
   What I observed: Design §3 adds ShowConsent() to internal/telemetry/telemetry.go. The spec's "Planned touchpoints" note says "T9 ships internal/telemetry.ShowConsent() as a callable function; T3/S09 adds the consent question to sworn init by importing it." But the spec's "In scope" section defines only IsEnabled, InstallID, Fire, ShowDisclosure — not ShowConsent. No signature, no behaviour contract, no test in "Required tests." ShowConsent is the T9→T3 interface boundary; undefined here means T3/S09 cannot reliably import it.
   What to ask the implementer: Define ShowConsent's signature and contract before writing code — at minimum: parameter types (io.Reader for stdin, io.Writer for output?), return type (bool for opted-in? error?), non-interactive-mode behaviour, and add TestShowConsent to the required tests. The spec's user outcome section implies the signature but does not state it.

8. [mechanical] §3 internal/telemetry/telemetry.go — Fire() signature omits sworn_version but the event schema requires it
   What I observed: Spec defines Fire(cmd, sub string, durationMS int64, exitCode int). The event schema includes "sworn_version": "0.1.0". But version is a package-level var in cmd/sworn/main.go — the internal/telemetry package cannot import main (circular). runtime.GOOS, runtime.GOARCH, runtime.Version() are available to the telemetry package directly, but sworn_version is not. Design §2 does not address how sworn_version reaches internal/telemetry.
   What to ask the implementer: Before writing code, pick one: (a) add a version parameter to Fire(); (b) add telemetry.SetVersion(v string) called once from main() before dispatch(); (c) embed via -ldflags in the telemetry package. Apply whichever in the implementation.

---

Pins: 8 total — 6 [mechanical], 0 [memory-cited], 2 [escalate]
Critical pins: 1 (AC1/AC2 verification will BLOCKED without formal deferral), 7 (ShowConsent undefined breaks T9→T3 contract), 8 (sworn_version unreachable from telemetry package breaks AC7 / schema compliance)

## Smaller flags (not pins, worth one-line ack)

- **TestIsEnabled_Neither name ambiguity**: Description says "no env var, no sentinel; IsEnabled() returns true" — but per IsEnabled() logic, "neither file exists → disabled." The test must be setting up .telemetry-enabled as part of its test fixture; the name should be TestIsEnabled_OptedIn_NoOverrides or the description should clarify setup. Minor naming issue, but could cause a future implementer to write a fixture-less test that asserts the wrong direction.
- **sworn_version vs go_version formatting**: Schema shows `"go_version": "go1.26"` (trimmed) but `runtime.Version()` returns `"go1.26.0"`. Implementer should trim to major.minor or document the exact format the schema expects.
- **Telemetry for sworn version / sworn help not addressed**: Design excludes `sworn telemetry *` but is silent on `sworn version` and `sworn help`. Confirm whether all meta-commands are excluded or only the telemetry subcommand. (See Pin 4.)

## Suggested ack reply

Design is solid with one product decision and one platform portability call needing Coach direction before code. 8 pins + 3 flags:

1. **AC1/AC2 Rule 2 deferral.** Add to status.json.open_deferrals: `{"why": "AC1/AC2 (sworn init consent wiring) owned by T3/S09 which holds cmd/sworn/init.go", "tracking": "S09-per-role-model-config planned_files includes cmd/sworn/init.go", "ack": "Coach noted in review 2026-06-21"}`. Empty open_deferrals will block the verifier on both ACs.
2. **main.go four-way collision.** Add a §4 note: "assembler must coordinate main.go merge order across T2/S04a, T3/S06a, T4/S08a, T8/S23 — all plan the same file." No code change needed.
3. **dispatch() version/help cases.** Before extracting dispatch(), grep for all cases that do NOT call os.Exit. Confirm dispatch() returns 0 for version and help branches.
4. **Meta-command exclusion — Coach pick (a) or (b):** (a) exclude sworn telemetry * only; (b) exclude sworn telemetry *, version, help from firing telemetry. Coach answers in ack.
5. **Config path — Coach pick (a) or (b):** (a) hardcode ~/.config/sworn/ matching spec ACs exactly; (b) use os.UserConfigDir() and replan spec ACs to platform-neutral language. Coach answers in ack.
6. **ShowDisclosure neutrality check.** Confirm the extra "neutral state" precondition is intentional. Confirm the reachability artefact smoke test is run with no .telemetry-enabled or .no-telemetry present.
7. **ShowConsent definition.** Before writing code: document the function signature and add TestShowConsent to the test list. Proposed: `ShowConsent(r io.Reader, w io.Writer) (optedIn bool, err error)`. Spec user outcome describes the behaviour; just write it down.
8. **sworn_version in Fire().** Pick one mechanism and implement it: (a) add `version string` param to Fire(); (b) add `telemetry.SetVersion(v)` called from main() before dispatch(); (c) embed via ldflags. Apply inline.

Flags (not pins): (a) rename TestIsEnabled_Neither to TestIsEnabled_OptedIn_NoOverrides or clarify fixture setup in test description; (b) trim go_version to major.minor to match schema literal; (c) explicitly decide whether sworn version and sworn help fire telemetry.

AC deferral noted (Pin 1) — Coach acknowledges AC1/AC2 are verified when T3/S09 lands, not at S26 verification.

Address pins 1–8 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Pins 4 and 5 require Coach product/architecture decisions (meta-command exclusion not in spec; os.UserConfigDir() deviates from spec's literal path and breaks AC verification on macOS).
-->
