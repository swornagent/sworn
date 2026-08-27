```sworn-release-manifest-v1
{
  "approval_ref": "operator://2026-08-26-native-lane-honesty/3",
  "previous_plan": "8beca4c8cae77b77f44382fcee152aeedceb16c7",
  "release": "2026-08-26-native-lane-honesty",
  "repository": "sworn",
  "revision": 3,
  "schema_version": "sworn.release-manifest/v1",
  "target_ref": "refs/heads/release/v1.0.0",
  "tracks": [
    {
      "depends_on": [],
      "id": "T1-honesty",
      "slices": [
        {
          "consumes": [],
          "contract_path": "contracts/2026-08-26-native-lane-honesty/S1-cli-pin-admission-policy.json",
          "depends_on": [],
          "digest": "sha256:5e82b1832f83325cec1361dc02ac0ebdff2abfda8d0810c182c328bc5a0a350a",
          "id": "S1-cli-pin-admission-policy",
          "outcome": "The native CLI pin stops being a compiled trap: admission becomes policy - exact mode preserves today's byte-for-byte closure, minor mode admits a binary whose self-reported version satisfies the pinned major.minor range - while receipts stay exact always because the digest of the binary that actually ran lands durably per dispatch on success and failure alike, the shipped claude pin moves to the live 2.1.241 so a main-built binary stops compiling a server-side-dead CLI, and a dead pin refuses at admission with a named code instead of burning tries as opaque transport failures - honestly scoped to what a side-effect-free probe can establish: a pinned binary that cannot execute and report its version refuses before any try burns, while server-side death, which no local probe can establish without transacting, is retired by the live pin and minor policy here and made instantly legible by S5's death classification.",
          "touchpoints": [
            "internal/driver",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/driver by import only; no cmd test pins native admission policy or pin constants"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/driver by import only; no cockpit test pins native admission policy"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no observe test pins native admission policy"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no tui test pins native admission policy"
            }
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-08-26-native-lane-honesty/S2-capture-proxy-tool-less-tolerance.json",
          "depends_on": [],
          "digest": "sha256:91643d28a707701f87f5f5a2a816d842c3ce01714790546f8c50bd142ae3ec74",
          "id": "S2-capture-proxy-tool-less-tolerance",
          "outcome": "The certification capture proxy stops killing the claude CLI's tool-less housekeeping requests: a captured provider request that carries no tool surface cannot reach the workspace and is admitted without the exact-match model and tool checks, without consuming the capture slot, and without minting tool-digest evidence for a surface it never observed - while every tool-bearing request still pins the session model and the exact tool definitions, so the certificate's evidence stays truthful and the sworn#237 tolerance defect closes as product instead of living in an operator patch.",
          "touchpoints": [
            "internal/driver"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/driver by import only; no cmd test pins capture-proxy request validation"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/driver by import only; no cockpit test pins capture-proxy request validation"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no observe test pins capture-proxy request validation"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/driver by import only; no runtime test pins capture-proxy request validation"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no tui test pins capture-proxy request validation"
            }
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-08-26-native-lane-honesty/S3-output-stream-economy.json",
          "depends_on": [],
          "digest": "sha256:61b7210da6af3c84969f4b6a83b0cc4153932fce7f9fe7c2a9831d21840758bf",
          "id": "S3-output-stream-economy",
          "outcome": "The 1MB cumulative cap on the native CLI's event stream stops wearing a surface-integrity code: the cumulative budget becomes manifest-governed economy failing ECONOMY_OUTPUT_BUDGET_EXCEEDED with a receipt-bearing failure observation and a park whose facts are byte-true, the per-line and per-event bounds stay surface integrity, and a long implementer session that legitimately streams more than a megabyte of assistant turns stops dying as NATIVE_SURFACE_INVALID - the r10-instrumented killer of contract-identity r4-r10 and the retroactive suspect for R3's opus deaths, landed as product instead of operator patch #7's 16x hack.",
          "touchpoints": [
            "internal/driver",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; no cmd test pins native stream budgets or economy park facts"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach the touched packages by import only; no cockpit test pins native stream budgets or economy park facts"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages directly and via internal/cockpit by import only; no observe test pins native stream budgets"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages via internal/cockpit by import only; no tui test pins native stream budgets"
            }
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-08-26-native-lane-honesty/S4-refusal-taxonomy.json",
          "depends_on": [],
          "digest": "sha256:ed0a54ed1db90c7fb96cde7bb09d334277a38cd736d7b8be82a2192a8105021c",
          "id": "S4-refusal-taxonomy",
          "outcome": "Driver-boundary refusals stop being one opaque mush: every refusal carries a typed Kind distinguishing cause, hard provider exhaustion is never paced-retried into a rate window, a NATIVE_SURFACE_INVALID names which of its checks refused and persists a bounded secret-free head of the offending request, and the provider's own words reach the durable record once per dispatch instead of being cleared - the training signal the in-engine-learning spec's phase 3 graph requires, landed as phase 2.",
          "touchpoints": [
            "internal/driver"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/driver by import only; no cmd test pins refusal kinds or pacing classification"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/driver by import only; no cockpit test pins refusal kinds"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no observe test pins refusal kinds"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/driver by import only; runtime consumes refusal codes through the existing stableErrorCode and refusal-binding surfaces, which are unchanged in shape"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no tui test pins refusal kinds"
            }
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-08-26-native-lane-honesty/S5-preflight-probes.json",
          "depends_on": [
            "S4-refusal-taxonomy"
          ],
          "digest": "sha256:fe4798563ff9aebe7e57e4a433ee7f269e3a7c5024c4e12df7af4b3cbc795328",
          "id": "S5-preflight-probes",
          "outcome": "A dispatch stops burning tries on failures a cheap probe could have named: a preflight registry runs side-effect-free, bounded, journaled probes at dispatch admission and refuses with named codes before anything is spent; sworn driver certify stops claiming native_preflight_not_required for credentials it never evaluated; and an instant native death becomes auth-vs-transport-vs-surface distinguishable on a durable surface - closing sworn#243's two broken promises (six burned tries under a PASS certificate, and a classification that could not tell a dead credential from a dead pin from a dead stream).",
          "touchpoints": [
            "internal/driver",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; the certify surface change is pinned by driver tests, not cmd tests"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach the touched packages by import only; no cockpit test pins preflight admission"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages directly and via internal/cockpit by import only; no observe test pins preflight admission"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages via internal/cockpit by import only; no tui test pins preflight admission"
            }
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-08-26-native-lane-honesty/S6-sanitization-keeps-diagnostics.json",
          "depends_on": [
            "S4-refusal-taxonomy"
          ],
          "digest": "sha256:27ce49b13a4b9031a11bc22442ff2e20deffa93a42bb17f1991fe731639cbc6a",
          "id": "S6-sanitization-keeps-diagnostics",
          "outcome": "Failure sanitization stops scrubbing the evidence: a failed observation whose diagnostic code is outside the admitted set keeps a bounded, re-validated, secret-free record of what actually refused instead of flattening to adapter_failed with zeroed facts - so the class that cost contract-identity three diagnostic run generations and two wrong theories (duration_ms 0, adapter_failed, stderr 0, pattern-matching stale OAuth while the stream cap was the killer) becomes a query against the durable record, and hostile-adapter discipline survives intact because preservation is bounded re-validation, never trust.",
          "touchpoints": [
            "internal/driver"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/driver by import only; no cmd test pins failed-observation sanitization"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/driver by import only; no cockpit test pins failed-observation sanitization"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no observe test pins failed-observation sanitization"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/driver by import only; the persisted observation-body shape it consumes grows additively and its fixtures are unchanged"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no tui test pins failed-observation sanitization"
            }
          ]
        },
        {
          "consumes": [],
          "contract_path": "contracts/2026-08-26-native-lane-honesty/S7-park-lane-scoping.json",
          "depends_on": [],
          "digest": "sha256:e740f53f1575652e7243abb40524ae3e8f2008a5d928539e06749ad087f5021b",
          "id": "S7-park-lane-scoping",
          "outcome": "A park becomes evidence about one work item instead of a run verdict: a work-scoped park crossing pins the affected work immediately with cause, code, and named paths stamped at crossing time, every other lane keeps dispatching, the run reports parked only when no admissible non-parked work remains, and a consecutive-failure streak breaks on any later success in the same slice-stage lineage - Brad's sworn#239 ruling delivered, killing both the CI-observed parked-with-admissible-work shape and the production instance where contract-identity r1 parked a healthy run on a recovered slice's stale facts.",
          "touchpoints": [
            "internal/runtime",
            "internal/cockpit",
            "test/e2e"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; the run-status and needs-you shapes it renders grow additively"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages directly and via internal/cockpit by import only; no observe test pins park scoping"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages via internal/cockpit by import only; parked-state rendering it consumes grows additively"
            }
          ]
        }
      ]
    }
  ]
}

```


# Why

The native lane works only when a human operator carries uncommitted
engine patches, and when it fails it lies about why. Every claude-lane
run since R3 has required a hand-patched binary - the pin bump, the
capture-proxy tolerance, the stream-cap widening archived as the
three-patch ops diff - and the contract-identity r4-r10 diagnostic
spiral cost six burned tries plus three run generations because
sanitization scrubbed the real failure site to adapter_failed/duration
0, preflight certification said PASS about credentials it never
evaluated, and an economy condition wore a surface-integrity code.
Meanwhile the park machinery treated evidence about one work item as a
run verdict: a healthy run parked on a recovered slice's stale failure
facts, and CI caught a drive reporting parked while an independent
track still held admissible work. This release lands the patches as
product, makes the failure surfaces tell the truth - which is also the
training-signal prerequisite for the in-engine-learning graph - and
delivers the sworn#239 ruling.

# What is being pinned

1. Pin admission becomes policy - exact by default, minor-range by
   declaration - while receipts stay exact always: the digest of the
   binary that actually ran rides attempt.Usage on every completed
   dispatch, success or failure, with a usage-preservation gate
   keeping it on failed native dispatches; the shipped claude pin
   moves to the live 2.1.241; and a dead pin refuses at admission
   with a named code at the pre-attempt prepareDriverDispatch seam,
   burning zero tries, with the probe result journaled (S1,
   sworn#220).
2. The certification capture proxy admits the claude CLI's tool-less
   housekeeping requests without consuming the capture slot or
   minting vacuous tool-digest evidence; tool-bearing requests stay
   exactly pinned (S2, sworn#237 tolerance half).
3. The 1MB cumulative event-stream cap becomes manifest-governed
   economy failing ECONOMY_OUTPUT_BUDGET_EXCEEDED with a
   receipt-bearing observation and byte-true park facts; per-line and
   per-event bounds stay surface integrity (S3, sworn#241).
4. Driver-boundary refusals carry a typed Kind; hard exhaustion never
   paces; NATIVE_SURFACE_INVALID names its check and keeps a bounded
   request head; the provider's own words reach the durable record
   once per dispatch (S4, the provider-error-taxonomy line,
   learning-spec phase 2).
5. A preflight registry refuses inadmissible dispatches with named
   codes before anything burns; certify stops claiming
   native_preflight_not_required for credentials it never evaluated;
   instant native deaths classify auth-vs-transport-vs-surface (S5,
   sworn#243, learning-spec phase 1).
6. Sanitization preserves a bounded, re-validated, secret-free record
   of what actually refused instead of scrubbing it to adapter_failed
   with zeroed facts (S6, sworn#242).
7. A park pins the affected work with cause, code, and named paths;
   other lanes keep dispatching; the run reports parked only when
   nothing admissible remains; failure streaks break on lineage
   success - the sworn#239 ruling (S7).

# Revision 2

Revision 2 answers the S1 captain escalate (track head 2aff93a2,
design attempt 3): A3's pinned outcome - a named admission refusal
with zero burned tries - is only representable pre-attempt, because
the attempt-write seam (internal/runtime/dispatch.go:2255-2296)
journals an OperationalFailed attempt for any invoke error and the
scheduler (scheduler.go:1418-1425) then advances to tries 2 and 3,
and the only pre-attempt seam is prepareDriverDispatch
(internal/runtime/dispatch.go:1178-1201), which revision 1's scope
did not include. The decision the escalate carried to the plan,
ratified by the operator: S1 gains internal/runtime for the
admission call site and the journaled probe result. The two findings
the escalate carried forward land with it: a side-effect-free probe
cannot transact with the provider, so server-side pin death (the
2.1.234 class) is not locally provable - a local --version run
(nativeVersion, native_linux.go:2418-2444) is passed by a
server-side-dead CLI - and A3 therefore pins the only class a
bounded local liveness run can prove, a pinned binary that cannot
execute and report its version, leaving server-side death to the
live pin bump, minor policy, and S5's first-occurrence
classification; and the executed digest must survive failed
dispatches, which today it does not (post-closure failures return an empty Observation, and
sanitizeFailedObservation at internal/driver/invoke.go:383-406
preserves Usage for only three diagnostic codes).

The S1 contract changes; S2-S7 stay byte-identical. What changed in
S1:

1. scope.include gains internal/runtime, and the internal/runtime
   waiver is retired in the same move: waivers suppress test-closure
   findings only (scopelint.go:271-345) while scope.include is the
   edit permit, so a package that becomes editable cannot stay
   waived.
2. A2 names its carrier: attempt.Usage, stamped at the single
   attempt-write seam on every completed attempt, success or failure,
   as one additive omitempty UsageReceipt field in the established
   pattern, plus the usage-preservation gate that keeps the executed
   digest on failed native dispatches instead of zeroing it.
3. A3 names its seam and its honest reach: the refusal lands at
   prepareDriverDispatch before any attempt is written, so zero
   tries burn and the probe result is journaled at admission; the
   probe pins the locally-provable class only, bound expiry admits
   as honestly-unevaluable, and server-side pin-death detection is
   excluded by name in scope.

The invalidated dependency closure is S1 alone: no slice depends on
or consumes S1's product (S5 and S6 depend on S4's Kind vocabulary,
which is unchanged), and no slice's consumed inputs changed. The
revised contract file must be committed as ordinary product-tree
content at target_ref before this revision is recorded; its exact
bytes land in the same commit as this plan.

Provenance, honestly: revision 2 was first proposed by the planner
and approved by the operator in-session against plan bytes
sha256:3969d2dce35b5f08a9ccbaf2fd50cbc421dee8c821645ab543e8456f80f5a2aa
whose file did not survive the proposing sandbox; the planner's
re-proposal could not recover them and re-authored an S1 contract
that drifted from the approved acceptance (its A3 demanded a
side-effect-free probe prove server-side death - the exact demand
the escalate and the approved bytes had retired). The operator
therefore carried this revision in the precedented pattern: the S1
contract bytes beside this plan are the approved first-proposal
bytes, recovered byte-exact from the sealed submission record (raw
sha256:66eda41952a0dabe90bc9e8f949520e3f7b62f84b71d7e4c747d975336fe1f85,
canonical digest
sha256:deb16f6efd24570dcfaf88683b2465d22170bfeba81c3784282e7cc3dab610b4),
and this plan restates the re-proposal's manifest and narrative
around them with the drift corrected. The approval under
operator://2026-08-26-native-lane-honesty/2 binds these exact
bytes.

# Revision 3

Revision 3 changes one sentence of S1's A1 and nothing else: the two
ClaudeCLI digest literals are spelled in full (the dead
sha256:3473601ea695d5bf769c5b202844d4cb4fbf723ae995450fcb6973204775c84a
and the live
sha256:0771bd866cff82b76581fc0499f6529e1a36845078f144f8c81dccb3bc7037b8
for 2.1.241), making the pin values normative contract bytes instead
of truncated provenance prefixes. Why: the r2 implementer, obeying its
ratified design (risk 4) and captain correction 7, rightly refused to
adopt the full digest from an operator answer it could not verify from
the sandbox - the archived R4 ops-home patch is unreachable there, and
only a truncated prefix existed in the product tree. The authority
chain the worker already trusts (contract bytes -> plan manifest
digest -> approved authority digest) is the correct channel for these
values, so they now live in it. S2-S7 stay byte-identical; the
invalidated closure is S1 alone; the values are seeded verbatim from
the archived operator patch and are the constants today's certified
live 2.1.241 binary was probed against.

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-26-native-lane-honesty/2`, per the operator's
slate ratified in-session 2026-08-26 (native-lane honesty + learning
foundations: the three operator patches as product, preflight probes,
refusal taxonomy, sanitization honesty, and the sworn#239 ruling; the
sworn#227 remainder was verified moot against post-R4 main during
authoring and closed with evidence, and sworn#219 slots behind) and
the operator's in-session revision-2 ratification of the S1
escalate's decision (2026-08-27). Planning did not approve itself.

One track, seven slices, one mechanism each: pin policy, capture
tolerance, stream economy, refusal taxonomy, preflight admission,
sanitization honesty, and park scoping are separate seams with
separate failure modes. S5 and S6 depend on S4 - both consume the
Kind vocabulary - so taxonomy lands first among the honesty slices;
S1-S3 retire the hand-patched-binary class and land before the fired
dogfood per the ratified sequencing; S7 is the sole runtime-projection
slice and rides last. The verifier judges each slice by its
worker-runnable checks and evidence anchors; e2e conformance remains
declared CI evidence (ADR-0010).

Roles (operator proposal for ratification at approval, precedented by
the R4 endgame roster): claude-sonnet-5 implements, claude-opus-5
captains, claude-sonnet-5 verifies, qwen3.8-max plans and recovers;
the ollama lane (glm/kimi) stays available for bounded roles under its
~2h session windows. Until S1-S3 merge, any main-built ops binary
still requires the three archived operator patches for claude-lane
work; this release is the first authored on R4's own surface (sworn
plan pin, lint, record with digest-addressed contracts - the scratch
tooling is retired), and that authoring is itself part of R4's
acceptance in anger. This release does not alter trust rules, approval
semantics, containment authority, or what any control verb is
permitted to do; admission grows policy and probes whose defaults
preserve today's behavior, refusal vocabulary grows additively, and
exactly-once machinery is untouched byte for byte.
