```sworn-release-manifest-v1
{
  "approval_ref": "operator://2026-08-28-legible-refusals/2",
  "previous_plan": "6f7585403208243e1f549efcff03fb4659cc2f31",
  "release": "2026-08-28-legible-refusals",
  "repository": "sworn",
  "revision": 2,
  "schema_version": "sworn.release-manifest/v1",
  "target_ref": "refs/heads/release/v1.0.0",
  "tracks": [
    {
      "depends_on": [],
      "id": "T1-legibility",
      "slices": [
        {
          "consumes": [],
          "contract_path": "contracts/2026-08-28-legible-refusals/S1-sandbox-refusal-detail.json",
          "depends_on": [],
          "digest": "sha256:38fa29ae5178693421ece5b825d576f8dea9d32bbc1ab25b28d1ad90fd59a9fc",
          "id": "S1-sandbox-refusal-detail",
          "outcome": "A Bash-tool sandbox that refuses to start says which of its six distinct failures happened and what the kernel told it: every PROCESS_START_FAILED raise site carries a named check in the established surface-detail idiom and a bounded, secret-free rendering of the underlying error, so one failing run names the mechanism instead of costing a diagnosis round to guess it - closing the observability half of sworn#251, where four CI occurrences across four different tests could not be told apart because six mechanisms share one bare code.",
          "touchpoints": [
            "internal/driver"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/driver by import only; no cmd test pins Bash-tool sandbox start failures"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/driver by import only; no cockpit test pins sandbox start failures"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no observe test pins sandbox start failures"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/driver by import only; no runtime test pins Bash-tool sandbox start failures"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no tui test pins sandbox start failures"
            }
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-08-28-legible-refusals/S2-submission-refusal-names-the-field.json",
          "depends_on": [],
          "digest": "sha256:8399ed8abf8d9f5139fc5dad7bfe5d24742c6132d5952bc79867ede222458c72",
          "id": "S2-submission-refusal-names-the-field",
          "outcome": "A rejected submission tells the worker what refused it, so there is nothing left to bisect: the submit path names the failing field or bound back to the model in its correction, the correction loop stops treating the first schema-valid payload as the authoritative answer, and a payload whose own summary declares itself a probe can never seal as a design, an implementation, or a captain decision - closing sworn#245, whose six placeholder seals across four native-lane-honesty runs included a captain PROCEED whose entire content was the string \"probe: minimal submission to isolate field validation\".",
          "touchpoints": [
            "internal/driver",
            "cmd/sworn/tui_pty_linux_test.go",
            "test/e2e/production_journey_linux_test.go",
            "test/e2e/turn_recovery_linux_test.go"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/driver by import only; the widened scope raises the below-floor submission fixtures in cmd/sworn/tui_pty_linux_test.go in place, and no other cmd test pins submission validation or correction accounting"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/driver by import only; no cockpit test pins submission validation"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no observe test pins submission validation"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/driver by import only; no runtime test pins submission validation or the correction loop"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no tui test pins submission validation"
            }
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-08-28-legible-refusals/S3-reseal-without-a-code-change.json",
          "depends_on": [],
          "digest": "sha256:0b6a43a2bcaa07d4ffbf8d29c2fb56dfc0f8eb6abd3cd3c08e119659f8fad284",
          "id": "S3-reseal-without-a-code-change",
          "outcome": "Correct work stops being stranded by a receipt: a verifier can say that the implementation stands and only its evidence failed, a remediation answering that can re-seal the unchanged candidate with a truthful receipt instead of staging nothing and dying EMPTY_CANDIDATE, and an exhaustion park caused by empty candidates names its cause on the durable record - closing sworn#246, which in native-lane-honesty burned two full try budgets across two journals and cost an operator track reset that discarded 734 lines of verifier-confirmed-correct code.",
          "touchpoints": [
            "internal/driver",
            "internal/baton",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/driver, internal/baton and internal/runtime by import only; no cmd test pins the verifier-fail remediation transition"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/runtime and internal/driver by import only; no cockpit test pins remediation sealing"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/runtime via internal/cockpit by import only; no observe test pins remediation sealing"
            },
            {
              "package": "internal/skill",
              "reason": "its tests reach internal/baton by import only; no skill test pins verification evidence or the remediation transition"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/runtime via internal/cockpit by import only; no tui test pins remediation sealing"
            },
            {
              "package": "tools/batongolden",
              "reason": "its tests reach internal/baton by import only; it regenerates baton golden fixtures and pins no behaviour this slice changes"
            }
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-08-28-legible-refusals/S4-automation-refusal-legibility.json",
          "depends_on": [],
          "digest": "sha256:5e067d576f254843554f7af34859370c26ecc2a771e4de0bec489e3f18bca29e",
          "id": "S4-automation-refusal-legibility",
          "outcome": "The automation lane stops telling a worker its binding was wrong when the binding was never the problem: AUTOMATION_BINDING_MISMATCH stops standing for three unrelated failures and a fourth unrelated condition, a refused recovery decision names which field disagreed, and the recovery lane's escalation path becomes usable again - closing sworn#250, where a worker reported copying its binding verbatim, was told the binding mismatched, and could not have been right or wrong because the decide tool has no binding field at all.",
          "touchpoints": [
            "internal/driver"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/driver by import only; no cmd test pins automation decision validation"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/driver by import only; no cockpit test pins automation decision validation"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no observe test pins automation decision validation"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/driver by import only; no runtime test pins the automation decision decode path"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no tui test pins automation decision validation"
            }
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-08-28-legible-refusals/S5-verification-integrity.json",
          "depends_on": [],
          "digest": "sha256:2c2c71da4b9c93032cffda3b74f2ecd4a8d62acb6b984f4bde46b30305c21409",
          "id": "S5-verification-integrity",
          "outcome": "A guard that cannot run stops passing for one: a test that pins a named acceptance criterion fails loudly where it would have skipped, host-keyed fixtures stop being the only path to the surfaces they guard, and a verifier's check evidence carries provenance - which command ran, where, and what it actually observed - so \"the checks passed\" can no longer stand in for \"the criterion was exercised\", closing sworn#249 and the fourth instance of a class that has now cost two releases.",
          "touchpoints": [
            "internal/driver",
            "internal/baton"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/driver and internal/baton by import only; no cmd test pins skip policy or check-evidence provenance"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/driver by import only; no cockpit test pins skip policy or check evidence"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no observe test pins skip policy"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/driver and internal/baton by import only; no runtime test pins skip policy or check-evidence provenance"
            },
            {
              "package": "internal/skill",
              "reason": "its tests reach internal/baton by import only; no skill test pins verification evidence or the remediation transition"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no tui test pins skip policy"
            },
            {
              "package": "tools/batongolden",
              "reason": "its tests reach internal/baton by import only; it regenerates baton golden fixtures and pins no behaviour this slice changes"
            }
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-08-28-legible-refusals/S6-attested-operator-answers.json",
          "depends_on": [],
          "digest": "sha256:39a59d18400302fdfd742ddd947a9a0081c7f95016cd841fc7558ad23809477b",
          "id": "S6-attested-operator-answers",
          "outcome": "An operator's answer reaches a worker as an attested fact rather than an anonymous string, so a worker can act on operator-supplied values without being asked to trust text it cannot distinguish from injection - closing sworn#247 by wiring the attestation channel this codebase already defines and has never once used: FactOperatorAnswer exists and is admitted as a byte-for-byte fact, and no site in the tree constructs it.",
          "touchpoints": [
            "internal/driver",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/runtime and internal/driver by import only; no cmd test pins how an answer is carried into a worker envelope"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/runtime by import only; no cockpit test pins recovery envelope facts"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/runtime via internal/cockpit by import only; no observe test pins recovery envelope facts"
            },
            {
              "package": "internal/baton",
              "reason": "its tests do not reach the recovery envelope; answer carriage is a runtime and driver concern"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/runtime via internal/cockpit by import only; no tui test pins recovery envelope facts"
            }
          ]
        }
      ]
    }
  ]
}

```


# Why

R5 made one boundary honest and proved, in its own six-run endgame,
that the others are not. Every expensive failure of native-lane-honesty
came from a refusal that named nothing. A worker whose submission was
rejected without being told which field refused did the rational thing
and bisected the validator with a minimal payload, and the correction
loop sealed that probe as authoritative work six times - once as a
captain PROCEED whose entire content was the string "probe: minimal
submission to isolate field validation". A recovery role was told its
binding mismatched when the decide tool has no binding field at all,
and the likely cause was an unrelated shape rule it was never named.
Four CI rounds were spent guessing which of six sandbox failure modes
had fired, because all six share one bare code. An implementer refused
an operator-supplied pin digest it could not distinguish from injected
data - correctly - and cost a run generation, while the fact channel
built for exactly that purpose sat unused. And a verifier that found
the code correct and only the evidence missing had nowhere to say so,
so the remediation had nothing to edit, burned two try budgets, and
ended in an operator track reset that discarded 734 verified-good
lines.

None of that was a model failing. Each was a boundary that could not
say what it refused, and a loop that could not express the only fix
the situation needed.

# What is being pinned

1. Every Bash-tool sandbox start refusal names which of its six sites
   fired and what the kernel said, so the sworn#251 class is
   diagnosable from a CI log and a journal instead of a rebuild (S1).
2. A rejected submission names the failing field or bound, a payload
   that declares itself a probe can never seal as work, and the
   content floor lands as product defence in depth (S2).
3. A verifier can fail evidence while affirming the code, and the
   remediation that answers it can re-seal the unchanged candidate
   with a truthful receipt instead of dying EMPTY_CANDIDATE (S3).
4. AUTOMATION_BINDING_MISMATCH stops standing for three unrelated
   failures and a fourth unrelated condition, and a refused decision
   names the rule that refused it (S4).
5. A test that pins a named acceptance criterion fails loudly rather
   than skipping, criteria stop being guarded only by fixtures keyed
   to one operator's machine, and check evidence carries provenance
   (S5).
6. An operator answer reaches a worker as the attested fact this
   codebase already defines and has never constructed (S6).

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-28-legible-refusals/1`, per the slate ratified
in-session 2026-08-28 from native-lane-honesty's run evidence
(docs/captures/2026-08-28-r6-legible-refusals-plan.md). Planning did
not approve itself.

One track, six slices, one seam each. There are no hard dependencies
between them: S2 and S4 both extend the refusal vocabulary R5 shipped
and neither blocks the other. S1 is sequenced first for an operational
reason rather than a technical one - the fired dogfood is queued behind
this release and will exercise the Bash sandbox heavily on an
unfamiliar host, where the sworn#251 class would be far harder to
diagnose than it is in CI.

Learning-spec phases 3 and 4 are deliberately excluded. Phases 1 and 2
shipped in R5; 3 and 4 are the actual in-engine learning, they carry
new conceptual risk, and the spec itself says they want the literature
to inform the memory and retrieval structure before contracts are
authored. This release finishes R5's honesty work instead.

This release does not alter trust rules, approval semantics,
containment authority, or what any control verb is permitted to do.
Refusals gain names and causes, one narrow and recorded re-seal
exception is added to a rule that otherwise stands, and the
exactly-once machinery is untouched byte for byte.
