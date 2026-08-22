```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-22-drive-survival",
  "revision": 1,
  "previous_plan": null,
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-22-drive-survival/1",
  "tracks": [
    {
      "id": "T1-survival",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-owner-self-release",
          "outcome": "A finished or cancelled drive can always hand the run back: releasing an owner claim that is still the current claim succeeds even after its lease expired, so a dead host's claim never strands a run claimed-expired - while a superseded owner stays fenced exactly as before, and renewal of an expired lease stays refused.",
          "contract_path": "contracts/2026-08-22-drive-survival/S1-owner-self-release.json",
          "digest": "sha256:ee4dcec224a0fa6febd4b9a17c63627f73b39b122ad462432ecac441e00cbc85",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/journal",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/journal and internal/runtime by import only; no cmd test pins that releasing one's own expired-but-current owner claim fails, and the CLI surface is S2's ground - this slice is constrained to never touch cmd"
            },
            {
              "package": "internal/cockpit",
              "reason": "consumes journal and runtime by import only; no cockpit test pins release-of-expired refusal semantics, and acquisition, takeover, and renewal - the semantics its surfaces do observe - are constrained unchanged"
            },
            {
              "package": "internal/observe",
              "reason": "consumes runtime via cockpit by import only; no observe test pins lease release semantics"
            },
            {
              "package": "internal/tui",
              "reason": "consumes journal and runtime by import only; no tui test pins lease release semantics, and the TUI surface is S3's ground"
            }
          ]
        },
        {
          "id": "S2-cli-drive-host",
          "outcome": "A one-shot CLI verb that starts a drive hosts it to its natural stop: answer, resume, and takeover print their durable status and then the process carries the background drive until the run parks, completes, or refuses - and run --detached stops lying, refusing with a stable coded error that names the resident serve path until a real detach exists. Killing the host mid-drive stays recoverable, and nothing changes what any verb is permitted to do.",
          "contract_path": "contracts/2026-08-22-drive-survival/S2-cli-drive-host.json",
          "digest": "sha256:56c0e0dc4c2ee258881dc674d1005bbcee8196563e49019e8ae0b0f815e70338",
          "depends_on": [
            "S1-owner-self-release"
          ],
          "consumes": [],
          "touchpoints": [
            "cmd/sworn",
            "internal/runtime",
            "test/e2e"
          ],
          "waivers": [
            {
              "package": "internal/cockpit",
              "reason": "consumes runtime by import only; the resident serve's drive hosting and Service.Close teardown semantics are constrained unchanged by this slice"
            },
            {
              "package": "internal/observe",
              "reason": "consumes runtime via cockpit by import only; no observe test pins CLI process lifecycle"
            },
            {
              "package": "internal/tui",
              "reason": "consumes runtime by import only; the TUI's drive hosting is S3's mechanism and this slice is constrained not to touch it"
            }
          ]
        },
        {
          "id": "S3-tui-resident-host",
          "outcome": "The terminal board keeps its promise after you press enter: a TUI action that starts a drive hands it to a resident per-project command host that outlives the action, so an accepted answer is followed by the delivery actually continuing while the person watches - and quitting the TUI is an ordinary host death, recoverable like any other, never a silent stall dressed up as success.",
          "contract_path": "contracts/2026-08-22-drive-survival/S3-tui-resident-host.json",
          "digest": "sha256:64840e6028fe0a8e53259eaf10767b5cc753be2ce7b5e5fca8d6995c2436b5ef",
          "depends_on": [
            "S1-owner-self-release"
          ],
          "consumes": [],
          "touchpoints": [
            "cmd/sworn",
            "internal/tui",
            "internal/cockpit",
            "test/e2e"
          ],
          "waivers": [
            {
              "package": "internal/observe",
              "reason": "consumes cockpit by import only; no observe test pins who hosts TUI-started drives"
            },
            {
              "package": "internal/journal",
              "reason": "consumed by import; lease semantics are S1's ground and this slice is constrained to leave ownership untouched"
            },
            {
              "package": "internal/runtime",
              "reason": "consumed by import; Service.Wait and drive hosting primitives are used as-is, with any behavioral change to them out of scope"
            }
          ]
        }
      ]
    }
  ]
}
```

# Why

The 2026-08-20-operator-surface release made control verbs return once
durable, with the drive continuing "under a background owner in the same
process lifetime". The first real execution of the conformance suite
(the 2026-08-21-detached-conformance merge and its CI postscript,
sworn#230) proved that promise is only kept by the resident serve. On
every other surface the process that starts the drive kills it at
teardown: Service.Close cancels background drives, so one-shot CLI
verbs cancel theirs milliseconds after returning, `run --detached`
prints "Watch progress:" and then cancels its own drive, and the TUI's
per-action command facade closes the journal store beneath the drive
337 milliseconds in. Service.Wait - the built affordance for hosting a
drive to its natural stop - has zero production callers. The tail of
the same mechanism is a stranded lease: a cancelled drive that outlives
its lease cannot release it (checkOwner refuses expired leases), the
owner lands claimed-expired, and `run` cannot adopt it.

Two conformance scenarios are deliberately red on release/v1.0.0 today
as the executable statements of sworn#230:
TestDetachedLifecycleScenarioProvesDetach and the surfaceTUIAnswer
phase of TestSwornConformanceObservableSurfaceParity. They flip green
when this release's mechanisms are real, and the CI run on the merge is
this release's own evidence.

# What is being pinned

Four rulings, operator-proposed on sworn#230 and human-ratified
2026-08-21, one per mechanism seam:

1. A still-current expired owner claim may be released by its own
   holder; a superseded owner stays fenced; expired renewal stays
   refused (S1).
2. The one-shot CLI process that starts a drive hosts it until the run
   parks, completes, or refuses; the verb's output and exit code keep
   reflecting command admission (S2).
3. `run --detached` refuses honestly until a real detach exists (S2).
4. The TUI hosts drives in a resident per-project command service that
   outlives individual board actions (S3).

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-22-drive-survival/1`. Planning did not approve
itself.

One track, three slices, one mechanism each per the planner doctrine:
lease release semantics, one-shot CLI drive hosting, and TUI drive
hosting are separate seams with separate failure modes; S2 and S3 both
stand on S1's un-strandable release. The verifier judges each slice by
its worker-runnable checks and evidence anchors; the two red
conformance scenarios are declared CI evidence (ADR-0010), first
executed by the CI run on the merge.

Roles: gemini-3.7-flash implements via the google-native profile under
transport pacing with the 2.4M input-token headroom cap; grok-4.6
verifies via xai-grok (qwencloud's weekly token plan resets 2026-08-25
20:47 UTC); deepseek plans and captains. This release does not alter
trust rules, receipt schemas, wire vocabulary, containment, journal
schemas, or what any gate admits - S1 widens one release path inside
the existing lease model and is contract-bounded to never touch
acquisition, takeover, or renewal.
