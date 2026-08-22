```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-19-record-economics",
  "revision": 1,
  "previous_plan": null,
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-19-record-economics/1",
  "tracks": [
    {
      "id": "T1-records",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-ancestry-adoption",
          "outcome": "A parked run survives forward motion of its target: saved-plan adoption accepts an approved receipt whose bound target is an ancestor of the live head - the staleness model baton already enforces - while true divergence and stale lineage still refuse, so an additive commit during a park never again strands a run at INVALID_AUTHORITY with no recovery verb.",
          "contract_path": "contracts/2026-08-19-record-economics/S1-ancestry-adoption.json",
          "digest": "sha256:d1426d6d597bef2b7baf7c395d8cfdb51fd280a5f0f2f9a07d4a7f71eed74275",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/runtime", "cmd/sworn", "internal/cockpit", "internal/observe", "internal/tui", "test/e2e"]
        },
        {
          "id": "S2-cycle-retry-pairing",
          "outcome": "A parked implementation cycle recovers with one retry: a derived work whose own epoch holds no attempts follows its parent cycle's retry epoch - the inheritance the journal's own comment already promises - and the board offers exactly one retry action per parked cycle, so the seal/dispatch epoch-skew wedge becomes unreachable instead of unwinnable.",
          "contract_path": "contracts/2026-08-19-record-economics/S2-cycle-retry-pairing.json",
          "digest": "sha256:dc213b4868010d9adf4e2688660d4be0dfd76cb45aea3e4ed5fa29fa3768ec9f",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/journal", "internal/runtime", "internal/cockpit", "internal/tui", "cmd/sworn", "internal/observe", "test/e2e"]
        },
        {
          "id": "S3-records-root-authority",
          "outcome": "The records root has one authority: write, read, migration, reservation guards, and the by-hand recording instructions all resolve the project's configured records root, with the legacy .baton/releases read path surviving only as an explicit observable compatibility shim - so a completed release never again re-introduces a tracked legacy path or re-trips the fresh-clone gate.",
          "contract_path": "contracts/2026-08-19-record-economics/S3-records-root-authority.json",
          "digest": "sha256:8feb717ea174a5b684ef938b0d0f3074f7697390161199b9b6fc14685c31e9f9",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/gitx", "internal/baton", "internal/driver", "internal/skill", "cmd/sworn", "tools/batongolden", "tools/batonassets", "test/e2e", "AGENTS.md", "README.md", ".gitattributes", ".github"],
          "waivers": [
            {"package": "internal/runtime", "reason": "its tests reference the records root only through the exported gitx constants, whose names and values this slice is constrained to keep; runtime behaviour is untouched"},
            {"package": "internal/cockpit", "reason": "consumes gitx and baton by import only; no records-root behaviour is pinned by its tests and the constants it could see are constrained unchanged"},
            {"package": "internal/observe", "reason": "consumes gitx by import only; no records-root behaviour is pinned by its tests and the constants it could see are constrained unchanged"},
            {"package": "internal/tui", "reason": "consumes gitx and baton by import only; no records-root behaviour is pinned by its tests and the constants it could see are constrained unchanged"}
          ]
        },
        {
          "id": "S4-coded-errors-surface",
          "outcome": "Coded failures reach the operator by name: stable error-code resolution unwraps journal and baton record errors, and the baton-action completion path stops flattening to baton_action_failed, so a STALE_RETRY_EPOCH or TARGET_DIVERGED surfaces as itself in sworn status instead of drowning in operational_failure.",
          "contract_path": "contracts/2026-08-19-record-economics/S4-coded-errors-surface.json",
          "digest": "sha256:c8a6e6630f8d03b01979f9296fd28732963326bae7c8ad98ba137ae9a9166b68",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/runtime", "cmd/sworn", "internal/cockpit", "internal/observe", "internal/tui", "test/e2e"]
        }
      ]
    }
  ]
}
```

# Goal

Delivering loop-economics measured a second bleed: the release machinery
itself. Twelve runs, six plan revisions and three journals delivered five
slices, and only one revision changed what was promised. One additive
commit during a park strands a run at INVALID_AUTHORITY that only
hand-rewritten ref surgery clears (sworn#211). A parked cycle offers two
retry actions where only one paired order was survivable, and the wrong
order wedged a run beyond every recovery verb (sworn#216). A one-time
records migration has left twenty-two marker commits across refs because
the by-hand recording doctrine still points at the legacy root the
engine's own write fence refuses (sworn#215). And the wedge cost most of
a day of diagnosis because STALE_RETRY_EPOCH reached the operator as
bare operational_failure.

This release converts that churn ledger into machinery. Adoption follows
baton's own ancestry model instead of a second, stricter one. Derived
works follow their parent cycle's retry epoch and a parked cycle offers
one retry. The records root resolves to one configured authority, with
the legacy path alive only behind an explicit shim, and the instructions
that recorders actually follow name that authority. Coded failures
surface by name. None of this changes what any gate admits - it changes
what survives an operator's ordinary day.

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-19-record-economics/1`. Planning did not approve
itself.

One serial track, four slices, no inter-slice dependencies or consumed
products: one mechanism per slice, per the planner doctrine this
repository carries as of the loop-economics post-mortem. Each slice's
scope is the lint-derived reverse-dependency closure of its owned
packages. S3 carries the release's first scope waivers - four consumer
packages whose tests see only exported constants the slice is
constrained to keep - so the operator judgement the lint solicits is
visible in the recorded bytes. Acceptance criteria cite evidence
anchors throughout; implementation routes stay with the Implementer and
the Captain.

Roles: gemini-3.7-flash implements via the google-native profile under
transport pacing; qwen3.8-max verifies via qwencloud; deepseek plans and
captains. This release does not alter trust rules, receipt schemas, wire
vocabulary, containment, or what any gate admits. Error codes recorded
in existing journals are never renamed; every new signal is additive.
