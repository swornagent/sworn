```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-25-contract-identity",
  "revision": 1,
  "previous_plan": null,
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-25-contract-identity/1",
  "tracks": [
    {
      "id": "T1-identity",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-digest-addressed-contracts",
          "outcome": "The contract digest becomes the identity and the path becomes provenance: a recorded revision resolves slice-contract bytes by canonical digest, revising a contract in place records and reads cleanly with no rev-directory ceremony, and the canonicalised-digest semantics that make raw sha256sum disagree with the declared digest are documented where an operator authoring contracts will actually find them.",
          "contract_path": "contracts/2026-08-25-contract-identity/S1-digest-addressed-contracts.json",
          "digest": "sha256:bc06e531811085b260455743890a821fd3ca1effcbf3bdd83871a0f7d335377c",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/baton",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; no cmd test pins contract resolution addressing"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach the touched packages by import only; no cockpit test pins contract resolution addressing"
            },
            {
              "package": "internal/driver",
              "reason": "its tests reach internal/baton by import only; no driver test pins contract resolution addressing"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages via internal/cockpit by import only; no observe test pins contract resolution addressing"
            },
            {
              "package": "internal/skill",
              "reason": "its tests reach internal/baton by import only; skill content is keyed on role-asset identity, not contract resolution"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages via internal/cockpit and directly by import only; no tui test pins contract resolution addressing"
            },
            {
              "package": "tools/batongolden",
              "reason": "its golden corpus pins canonical digest computation, which this slice freezes byte-for-byte; no batongolden fixture pins resolution addressing"
            }
          ]
        },
        {
          "id": "S2-plan-authoring-surface",
          "outcome": "The most safety-critical operator work in the product stops living in scratch tooling: sworn plan pin recomputes every manifest-mirrored fact from contract bytes, sworn plan lint runs the recording-time scope lint pre-commit, and sworn plan record performs the recording - so the manifest is derived, never hand-mirrored, and R4 is the last release authored with tmp/plantool and a per-session sync script.",
          "contract_path": "contracts/2026-08-25-contract-identity/S2-plan-authoring-surface.json",
          "digest": "sha256:2254eb73a8164cb49c260f9016014d1dfe875da661261c5430f250d0829e7ece",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "cmd/sworn",
            "internal/baton"
          ],
          "waivers": [
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/baton by import only; no cockpit test pins plan authoring or manifest derivation"
            },
            {
              "package": "internal/driver",
              "reason": "its tests reach internal/baton by import only; no driver test pins plan authoring or manifest derivation"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/baton via internal/driver by import only; no observe test pins plan authoring or manifest derivation"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/baton by import only; the recording guards the new verbs invoke are consumed unchanged"
            },
            {
              "package": "internal/skill",
              "reason": "its tests reach internal/baton by import only; skill content is keyed on role-asset identity, not plan authoring"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/baton via internal/cockpit by import only; no tui test pins plan authoring or manifest derivation"
            },
            {
              "package": "tools/batongolden",
              "reason": "its golden corpus pins canonical digest computation, which the authoring verbs consume unchanged; no batongolden fixture pins the authoring surface"
            }
          ]
        },
        {
          "id": "S3-proposal-contract-persistence",
          "outcome": "A proposal that introduces new contract files can actually install: the contract bytes ride the proposal as durably as its plan bytes now do, install sources them for paths absent from the bound target tree on both the live and the replay path, and replay re-proves them digest-bound - so a plan revision minting a new contract stops requiring an operator to hand-commit the contract and race the proposal's own staleness check.",
          "contract_path": "contracts/2026-08-25-contract-identity/S3-proposal-contract-persistence.json",
          "digest": "sha256:4380db4d43d8c057d755206bc0120991d0e820b30474978debae7dd24e669712",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/baton",
            "internal/driver",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; no cmd test pins proposal persistence or install sourcing"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach the touched packages by import only; no cockpit test pins proposal persistence or install sourcing"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages via internal/driver and internal/cockpit by import only; no observe test pins proposal persistence or install sourcing"
            },
            {
              "package": "internal/skill",
              "reason": "its tests reach internal/baton by import only; skill content is keyed on role-asset identity, not proposal persistence"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages via internal/cockpit and directly by import only; no tui test pins proposal persistence or install sourcing"
            },
            {
              "package": "tools/batongolden",
              "reason": "its golden corpus pins canonical digest computation, which proposal-carried contracts are proven against unchanged; no batongolden fixture pins proposal persistence"
            }
          ]
        },
        {
          "id": "S4-receipt-identity-split",
          "outcome": "A receipt names what it vouches for: acceptance/scope identity and checks identity split at the contract boundary, so editing a contract's checks list stops silently voiding design, captain-PROCEED, candidate, and PASS receipts whose acceptance criteria never changed - and with mechanisms 1, 4, and 5 already delivered and mechanism 3 delivered by S3, the sworn#218 umbrella closes with evidence.",
          "contract_path": "contracts/2026-08-25-contract-identity/S4-receipt-identity-split.json",
          "digest": "sha256:f6e70eb21a09e0fd5e2cca24036922fff35599e87e289b471ce05a213c6330da",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/baton",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; no cmd test pins receipt identity matching"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach the touched packages by import only; no cockpit test pins receipt identity matching"
            },
            {
              "package": "internal/driver",
              "reason": "its tests reach internal/baton by import only; no driver test pins receipt identity matching"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages via internal/driver and internal/cockpit by import only; no observe test pins receipt identity matching"
            },
            {
              "package": "internal/skill",
              "reason": "its tests reach internal/baton by import only; skill content is keyed on role-asset identity, not receipt identity"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages via internal/cockpit and directly by import only; no tui test pins receipt identity matching"
            },
            {
              "package": "tools/batongolden",
              "reason": "its golden corpus pins canonical digest computation, whose recorded values this slice keeps byte-for-byte stable; no batongolden fixture pins receipt matching"
            }
          ]
        },
        {
          "id": "S5-role-asset-addendum",
          "outcome": "The three sentences that cost review turns in both instrumented releases reach the roles that needed them: the implementer, verifier, and captain surfaces state that contract digests are canonical-content digests, that before and product_tree are invocation-state digests, and that the seal epoch moves in lockstep with the try ledger - as a sworn-owned, digest-accounted addendum, with the vendored Baton asset chain and its commit provenance untouched.",
          "contract_path": "contracts/2026-08-25-contract-identity/S5-role-asset-addendum.json",
          "digest": "sha256:c4055ad4af57940a6394b7dacbbad34c679896b753513666c0edc76fdb72ed6c",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; no cmd test pins the role-facing addendum surface"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach the touched packages by import only; no cockpit test pins the role-facing addendum surface"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages directly and via internal/cockpit by import only; no observe test pins the role-facing addendum surface"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages via internal/cockpit and directly by import only; no tui test pins the role-facing addendum surface"
            }
          ]
        },
        {
          "id": "S6-init-environment-honesty",
          "outcome": "Init stops reading host state it should not: on darwin, sworn init states plainly that native dispatch requires Linux instead of tripping a four-path Linux runtime-file preflight that cannot succeed there, and the init test fixture pins the complete agent environment instead of inheriting host PATH - so a real codex on the operator's machine no longer flips nine init tests, and the exact-native certification fixtures skip honestly on host drift instead of failing.",
          "contract_path": "contracts/2026-08-25-contract-identity/S6-init-environment-honesty.json",
          "digest": "sha256:a38326b7b2b146640156aa586a34783c8988a8fd73880aba56b05f3aa97817a4",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "cmd/sworn",
            "internal/driver"
          ],
          "waivers": [
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/driver by import only; no cockpit test pins init preflight or certification-fixture environments"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/driver by import only; no observe test pins init preflight or certification-fixture environments"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/driver by import only; no runtime test pins init preflight or certification-fixture environments"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no tui test pins init preflight or certification-fixture environments"
            }
          ]
        },
        {
          "id": "S7-worker-surface-truthfulness",
          "outcome": "The worker-facing surface stops lying and stops starving: Bash accepts the command alias every model reaches for, Read gains offset/limit and a batched paths form so whole-file rereads stop burning provider windows and batching intent has a legal syntax, the .git mask becomes an honest empty file instead of a character device that breeds theories, and an environment-facts block states the toolchain path, the mask, and the read budget that every worker today re-derives by habit.",
          "contract_path": "contracts/2026-08-25-contract-identity/S7-worker-surface-truthfulness.json",
          "digest": "sha256:048acac3efedd95d171b8be42c2720cb6e8df59d499a8bdc5cdadf8dc84effbb",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/driver by import only; no cmd test pins the worker tool schemas or the prompt envelope"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/driver by import only; no cockpit test pins the worker tool schemas or the prompt envelope"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/driver by import only; no observe test pins the worker tool schemas or the prompt envelope"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/driver by import only; no runtime test pins the worker tool schemas or the prompt envelope"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no tui test pins the worker tool schemas or the prompt envelope"
            }
          ]
        },
        {
          "id": "S8-temp-root-reaper",
          "outcome": "Temp roots stop accumulating forever: the driver factory sweeps stale certification roots at construction in the proven native-session reaper pattern, so a run that dies without unwinding no longer leaves its sworn-driver-certification root behind permanently, and the one gitx temp site cleaned by per-return removes becomes a defer so a panic cannot leak it.",
          "contract_path": "contracts/2026-08-25-contract-identity/S8-temp-root-reaper.json",
          "digest": "sha256:0db31ca70dc91a30c11088b2ab236f352a8d960b5428aa40f7e4adae729a215e",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver",
            "internal/gitx"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; no cmd test pins factory temp-root lifecycle or ref-transaction cleanup"
            },
            {
              "package": "internal/baton",
              "reason": "its tests reach internal/gitx by import only; no baton test pins factory temp-root lifecycle or ref-transaction cleanup"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach the touched packages by import only; no cockpit test pins factory temp-root lifecycle or ref-transaction cleanup"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages directly and via internal/cockpit by import only; no observe test pins factory temp-root lifecycle or ref-transaction cleanup"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach the touched packages by import only; no runtime test pins factory temp-root lifecycle or ref-transaction cleanup"
            },
            {
              "package": "internal/skill",
              "reason": "its tests reach internal/gitx by import only; no skill test pins factory temp-root lifecycle or ref-transaction cleanup"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages via internal/cockpit by import only; no tui test pins factory temp-root lifecycle or ref-transaction cleanup"
            },
            {
              "package": "tools/batongolden",
              "reason": "its tests reach internal/gitx via internal/baton by import only; no batongolden fixture pins temp-root lifecycle"
            }
          ]
        },
        {
          "id": "S9-scope-refusal-retry-floor",
          "outcome": "A scope refusal with succeeded work becomes a correction loop instead of a dead cycle: after a seal or scope refusal following a succeeded dispatch, the cycle re-dispatches the worker in-cycle carrying the refusal context - the named offending paths - instead of burning the remaining tries as WORK_ALREADY_SUCCEEDED admission refusals, and on exhaustion the park names the scope code so the operator sees why instead of a generic exhaustion. Brad's (b)-with-floor ruling on sworn#224, delivered.",
          "contract_path": "contracts/2026-08-25-contract-identity/S9-scope-refusal-retry-floor.json",
          "digest": "sha256:3f6f42f9faafd647cb26941f7eef0ca67d85e3ab0ca292a81e7935c2bdc4c7af",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/journal",
            "internal/runtime",
            "test/e2e"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; no cmd test pins cycle admission or exhaustion-park diagnostics"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach the touched packages by import only; no cockpit test pins cycle admission or exhaustion-park diagnostics"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages directly and via internal/cockpit by import only; no observe test pins cycle admission or exhaustion-park diagnostics"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages directly and via internal/cockpit by import only; no tui test pins cycle admission or exhaustion-park diagnostics"
            }
          ]
        }
      ]
    }
  ]
}
```

# Why

Contract, plan, and receipt identity is path- and whole-bytes-shaped,
and the authoring loop that manages that identity has no product
surface. Every revision of the last two releases paid the toll: `rev2/`
through `rev5/` path churn because records are immutable but identity
rides the path; manifests hand-mirrored from contract bytes by a
per-session scratch script; STALE_BINDING round-trips; design receipts
voided by orthogonal checks-list edits; sealed proposals dying with
their sandbox so an operator reconstructed approved-candidate bytes
from streamed logs; and the most safety-critical operator work in the
product - digest computation, scope lint, revision recording - living
in an uncommitted `tmp/plantool`. R3 made the run survivable
unattended; this release makes the release's own identity machinery
survivable, so the next revision is a recorded fact instead of an
operator ritual.

Beside the identity core ride four bounded operability slices earned
from live-run evidence: init reading host state it must not (sworn#226,
sworn#228), the worker-facing tool surface lying about its own
vocabulary while whole-file reads burn provider windows (sworn#188,
sworn#189, sworn#236, sworn#238), temp roots leaking (sworn#194), and
scope refusals burning try budgets as WORK_ALREADY_SUCCEEDED admission
refusals (sworn#224, ruled).

# What is being pinned

1. Identity is the digest, path is provenance: a recorded revision
   resolves contract bytes by canonical digest from the record tree,
   and path churn disappears (S1, sworn#200).
2. Derive, don't mirror: the manifest's mirrored facts are computed
   from contract bytes by the engine, and the authoring verbs - pin,
   lint, record - are product surface; R4 is the last release authored
   with scratch tooling (S2, sworn#234).
3. Proposals carry their contracts: new-contract bytes introduced by a
   proposal survive dispatch outcomes and install without a
   hand-carried commit (S3, sworn#210).
4. A receipt names what it vouches for: acceptance/scope identity and
   checks/evidence identity split, so a checks edit stops voiding
   design judgement (S4, sworn#218 mechanism 2).
5. The three sentences that cost review turns in both instrumented
   releases move into the role assets: canonical digests,
   invocation-state before/product_tree, seal-epoch lockstep (S5).
6. Init tells the truth about the host: darwin states native dispatch
   requires Linux instead of tripping a Linux preflight, and init
   tests pin their environment instead of inheriting the host's (S6,
   sworn#226 + sworn#228).
7. The worker surface stops lying: Bash accepts the alias every model
   reaches for, Read grows offset/limit and a batched paths form so
   window burn and dup-key batching have a legal syntax, the .git mask
   is an honest empty file, and an environment-facts block states what
   workers today re-derive by habit (S7, sworn#188 + sworn#189 +
   sworn#236 + sworn#238 rider).
8. Temp roots get a reaper in the proven pattern (S8, sworn#194).
9. A scope refusal with succeeded work re-dispatches in-cycle with the
   refusal context instead of burning tries; exhaustion parks naming
   the scope code - Brad's (b)-with-floor ruling (S9, sworn#224).

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-25-contract-identity/1`, per the operator's
standing queue ratified 2026-08-24 (the 9-slice slate including S6-S9
bundle-ins and the S9 ruling) with the S7 amendment of 2026-08-25
included for bytes-approval here. Planning did not approve itself.

One track, nine slices, one mechanism each: contract addressing,
authoring surface, proposal persistence, receipt identity, role-asset
truth, init honesty, worker-surface truth, temp hygiene, and admission
economics are separate seams with separate failure modes. S2 depends
on S1 (the verbs speak digest identity natively); S3 builds on
R3-S7's proposal persistence, now on main; the rest are disjoint. The
verifier judges each slice by its worker-runnable checks and evidence
anchors; e2e conformance remains declared CI evidence (ADR-0010).

Roles (operator-directed 2026-08-25, cost-managed - grok benched, no
xAI spend this release): glm-5.2 implements via ollama-cloud (certify
before launch - it is live-probed only; claude-sonnet-5 is the named
fallback implementer); claude-opus-5 captains; claude-sonnet-5
verifies; qwen-3.8 plans and recovers. The ops binary carries
the sworn#220 claude pin bump as a precedented operator patch until
that issue lands. The ox-alpha re-audition does not ride this release.
This release does not alter trust rules, approval semantics,
containment authority, or what any control verb is permitted to do;
identity changes are additive-with-migration inside the record tree,
new tool vocabulary is additive, and all defaults preserve today's
behavior.
