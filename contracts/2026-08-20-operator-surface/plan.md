```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-20-operator-surface",
  "revision": 1,
  "previous_plan": null,
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-20-operator-surface/1",
  "tracks": [
    {
      "id": "T1-surface",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-project-discovery",
          "outcome": "One discovery seam serves every entry point: a shared resolver over the project's .sworn surface (runs, manifests, releases on release-wt refs, drivers.json, operator.json by convention) powers bare sworn run, sworn run with a release name under the three-step precedence, and the flagless surfaces that follow - so knowing absolute paths stops being the price of admission.",
          "contract_path": "contracts/2026-08-20-operator-surface/S1-project-discovery.json",
          "digest": "sha256:42d7bdf338ce7150a4e33f0a0aa8a2971de2f047bc316efe890d557cec553b99",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["cmd/sworn"]
        },
        {
          "id": "S2-detached-drive",
          "outcome": "Control verbs stop holding the run hostage: Start, AnswerAttention, and Control return once the command is durable while the drive continues under a background owner in the same process lifetime or a named resumable state - so a closed laptop lid, a piped answer, or a timeout wrapper can never again orphan a run mid-dispatch.",
          "contract_path": "contracts/2026-08-20-operator-surface/S2-detached-drive.json",
          "digest": "sha256:b61fe0343a297de63a49c257020c6dbe08f745ab61d2dbe580e2eb579711fa26",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/runtime", "cmd/sworn", "internal/cockpit", "internal/observe", "internal/tui", "test/e2e"]
        },
        {
          "id": "S3-verb-truthfulness",
          "outcome": "Control verbs tell the truth about why nothing happened: a takeover during an unexpired owner lease returns a named wait signal with the remaining lease time instead of a silent status dump, and the board's takeover hint derives from the actual admission conditions - so the operator's next command is informed, not guessed.",
          "contract_path": "contracts/2026-08-20-operator-surface/S3-verb-truthfulness.json",
          "digest": "sha256:1939f88b60593423420a25bafa440ebb5b2222121745448435394f0bb4f37d4d",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/runtime", "cmd/sworn", "internal/cockpit", "internal/observe", "internal/tui"]
        },
        {
          "id": "S4-global-serve",
          "outcome": "sworn serve becomes the project's resident surface: started with no flags it discovers the project, lists releases and runs with their lifecycle states and a needs-you row, follows new runs as they appear, and reads the operator config by convention - one serve per project, started once, instead of one per run wired by hand.",
          "contract_path": "contracts/2026-08-20-operator-surface/S4-global-serve.json",
          "digest": "sha256:db6e71d19ca4161aa743d3d4996d7e851844378031ab4f4edc3dee8b1e9facbe",
          "depends_on": ["S1-project-discovery"],
          "consumes": ["S1-project-discovery"],
          "touchpoints": ["internal/cockpit", "cmd/sworn", "internal/tui", "internal/observe", "test/e2e"]
        },
        {
          "id": "S5-event-association",
          "outcome": "The event feed knows what it is talking about: runtime-journaled events carry their effect, work, and track association in the event body, the cockpit projection surfaces that association on each evidence row, and the feed becomes filterable per track and per slice - honest structure instead of a run-wide blur.",
          "contract_path": "contracts/2026-08-20-operator-surface/S5-event-association.json",
          "digest": "sha256:36162781caf94d7ccfbd64202ebdd6e63e5ee5d02f81fb45dd98847988db602f",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/runtime", "internal/cockpit", "internal/tui", "cmd/sworn", "internal/observe", "test/e2e"]
        },
        {
          "id": "S6-tui-live-cockpit",
          "outcome": "The TUI behaves like a cockpit instead of a console lock: it attaches to and reconnects with detached runs without ever driving one in its own foreground, keys stay live while work executes, refusals explain themselves in plain language for the codes operators actually hit, and the project's key configuration - the role/profile/model matrix, operator config, records root, paths - is readable in place.",
          "contract_path": "contracts/2026-08-20-operator-surface/S6-tui-live-cockpit.json",
          "digest": "sha256:72f1573be72691e033c0a6bbe919183056d25ac7968979d4645ae195ecbb42ef",
          "depends_on": ["S2-detached-drive"],
          "consumes": ["S2-detached-drive"],
          "touchpoints": ["internal/tui", "cmd/sworn"]
        },
        {
          "id": "S7-guided-init",
          "outcome": "sworn init becomes the guided front door: run in a project it walks the operator through what exists and what is missing - driver config with agent detection, the operator config, and where records will live - writes what was chosen, is idempotent on re-run (showing a diff-shaped summary instead of overwriting silently), and ends by printing the exact next command; --yes preserves the non-interactive scriptable path.",
          "contract_path": "contracts/2026-08-20-operator-surface/S7-guided-init.json",
          "digest": "sha256:342ca21f423a0f2a76bea2b8280ae8e00a4b78f5d5dcbef548c1d1e3ca6d008d",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["cmd/sworn"]
        }
      ]
    }
  ]
}
```

# Goal

The engine delivers releases; operating it still assumes the operator
reads Go. Starting a run demands two absolute paths. Serve is wired by
hand, once per run - five kill-and-relaunch cycles in one observed
evening. Answer and takeover drive the run in the caller's foreground,
so a piped command orphaned a run twice in one night. A takeover
refused during a live lease prints a success-shaped status dump. The
TUI freezes while work executes and explains four operator-hit
refusal codes with one generic sentence. The event feed cannot say
which track a row belongs to. Init scaffolds files and leaves the
operator to discover the rest from the source.

This release makes Sworn operable by a person with a day job: one
discovery seam behind every entry point, verbs that return when their
command is durable while the drive continues detached, refusals that
say what to do next, a resident project serve with a needs-you row, an
event feed with honest structure, a TUI that stays alive and shows the
configuration it is running under, and a guided init that ends with
the next command. No gate, receipt, or authority rule changes - only
who has to wait, and what they are told.

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-20-operator-surface/1`. Planning did not approve
itself.

One serial track, seven slices, one mechanism each per the planner
doctrine. Two real product edges: S4's flagless serve consumes S1's
resolver; S6's reconnect consumes S2's detached drive. Slices touching
internal/runtime carry the lint-derived closure (cmd/sworn, cockpit,
observe, tui); S1 and S7 stay inside cmd/sworn; S6 stays inside the
TUI and its backend. Acceptance criteria cite evidence anchors;
implementation routes stay with the Implementer and the Captain.

Rulings this plan encodes (2026-08-20): sworn init is the guided
per-project setup; key configuration is surfaceable read-first in the
TUI; serve-per-run dies with the resident project serve. The run-side
telemetry export move is explicitly out of scope here and lands with
telemetry-foundations.

Roles: gemini-3.7-flash implements via the google-native profile under
transport pacing; qwen3.8-max verifies via qwencloud; deepseek plans
and captains. This release does not alter trust rules, receipt
schemas, wire vocabulary, containment, journal schemas, or what any
gate admits. Error codes recorded in existing journals are never
renamed; every new signal is additive.
