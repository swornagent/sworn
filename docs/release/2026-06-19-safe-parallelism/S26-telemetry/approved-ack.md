Design is solid with two product calls needing Coach direction and three interface-contract items to lock down before code. 8 pins + 3 flags:

1. **AC1/AC2 Rule 2 deferral.** Add to status.json open_deferrals: {"why": "AC1/AC2 (sworn init consent) owned by T3/S09 which holds cmd/sworn/init.go", "tracking": "S09-per-role-model-config planned_files", "ack": "Coach review 2026-06-21"}. Without this the Verifier will BLOCKED on both ACs.
2. **main.go four-way collision note.** Add to design §4: "Assembler must coordinate main.go merge order — T2/S04a, T3/S06a, T4/S08a, T8/S23 all plan this file." No code change needed.
3. **dispatch() version/help handling.** Before extracting switch: grep for non-os.Exit cases (version, help). Confirm dispatch() returns 0 for these explicitly.
4. **Meta-command exclusion — Coach pick: (a).** Exclude sworn telemetry * only from firing telemetry events. sworn version and sworn help still fire — useful version-usage signal. Implement option (a).
5. **Config path — Coach pick: (a).** Hardcode ~/.config/sworn/ matching spec ACs exactly. Windows is deferred post-R3 per spec. Implement option (a).
6. **ShowDisclosure neutrality check.** Confirm extra neutral-state precondition is intentional. Run reachability artefact against clean config dir (no .telemetry-enabled or .no-telemetry).
7. **ShowConsent contract.** Before writing: document signature (proposed: ShowConsent(r io.Reader, w io.Writer) (bool, error)) and add TestShowConsent to required tests.
8. **sworn_version delivery.** Pick one and apply inline: (a) add version string to Fire() params — simplest, no circular import, caller passes it from main. Implement option (a): Fire(cmd, sub, version string, durationMS int64, exitCode int).

Flags: (a) rename TestIsEnabled_Neither → TestIsEnabled_OptedIn_NoOverrides; (b) trim go_version to major.minor; (c) sworn version/help still fire telemetry per pin 4 decision.

Address pins 1–8 inline during implementation, then proceed to in_progress.
