```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-23-unattended-operability",
  "revision": 3,
  "previous_plan": "4a6bc4fe41b30d327a33b87d7a24b48b898be23e",
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-23-unattended-operability/3",
  "tracks": [
    {
      "id": "T1-unattended",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-credential-preflight",
          "outcome": "A stale or rotating credential stops masquerading as network weather: a dispatch that would die on an expired token is refused up front with a named CREDENTIAL_STALE before any try burns, an auth-class failure from a native CLI is distinguishable from transport on every durable surface, and a credential rotated mid-dispatch by ordinary host use no longer discards the completed work it raced.",
          "contract_path": "contracts/2026-08-23-unattended-operability/S1-credential-preflight.json",
          "digest": "sha256:e9f97bd045611790cabd671816eef8f29dc5de449c336fd5052713e3589d651a",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; no cmd test pins credential preflight or lease semantics"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach the touched packages by import only; no cockpit test pins credential preflight or lease semantics"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages via internal/cockpit by import only; no observe test pins credential preflight or lease semantics"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/driver by import only; dispatch admission consumes the new CREDENTIAL_STALE code through the existing stable-error channel without runtime edits"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages via internal/cockpit and directly by import only; no tui test pins credential preflight or lease semantics"
            }
          ]
        },
        {
          "id": "S2-claimed-recovery",
          "outcome": "A cleanly-released run with expired claimed effects is recoverable instead of permanently wedged: re-entry reconciles the expired claim rather than looping RECOVERY_UNCERTAIN, the operator can always authorize a way forward, and the board's next step is derived from the same gate conditions the control verbs actually check - never advice the verbs refuse.",
          "contract_path": "contracts/2026-08-23-unattended-operability/rev2/S2-claimed-recovery.json",
          "digest": "sha256:fc18991c0fb53cfdc6483f80d61adf6259ba3028905a6ff80fa13e60bdf3a4a8",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/cockpit",
            "internal/journal",
            "internal/runtime",
            "test/e2e"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; no cmd test pins claimed-effect reconciliation or uncertain-state presentation"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages via internal/cockpit by import only; no observe test pins claimed-effect reconciliation or uncertain-state presentation"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages via internal/cockpit and directly by import only; no tui test pins claimed-effect reconciliation or uncertain-state presentation"
            }
          ]
        },
        {
          "id": "S3-honest-yield-parking",
          "outcome": "A worker asking an honest question is heard the first time: a question or blocked yield opens an answerable attention immediately - the worker's own words on the board, no try consumed, the conversation retained so the answer resumes it in place - instead of two thirds of the try budget burning while automation forwards 'no sealed submission' at a worker that asked for help.",
          "contract_path": "contracts/2026-08-23-unattended-operability/rev3/S3-honest-yield-parking.json",
          "digest": "sha256:fc335dfb30a2e234fb24fe6483788ebb33c7cfe0b73c9f6849a08386bc6456d1",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/runtime",
            "test/e2e"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; no cmd test pins yield routing; the attention surfaces they render are consumed unchanged"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach the touched packages by import only; no cockpit test pins yield routing; the attention surfaces they render are consumed unchanged"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages via internal/cockpit by import only; no observe test pins yield routing; the attention surfaces they render are consumed unchanged"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages via internal/cockpit and directly by import only; no tui test pins yield routing; the attention surfaces they render are consumed unchanged"
            }
          ]
        },
        {
          "id": "S4-economy-guards",
          "outcome": "The manifest's economic limits actually govern every dispatch, and a run that stops making real progress parks early with the evidence attached: a per-work turn and output-token budget catches the nine-hundred-turn slice that try-counting misses, N consecutive identical failures on one work park before the try budget burns, and the timeout and output limits the operator already declares finally bind on the API conversation path they have never governed.",
          "contract_path": "contracts/2026-08-23-unattended-operability/S4-economy-guards.json",
          "digest": "sha256:af4b5382f631e46f4ff67c456140e8cb6a3ee100a9f12865fb66e7184018bc92",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; no cmd test pins economy budgets; the park surfaces they render were pinned by the previous release and are consumed unchanged"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach the touched packages by import only; no cockpit test pins economy budgets; the park surfaces they render were pinned by the previous release and are consumed unchanged"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages via internal/cockpit by import only; no observe test pins economy budgets; the park surfaces they render were pinned by the previous release and are consumed unchanged"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages via internal/cockpit and directly by import only; no tui test pins economy budgets; the park surfaces they render were pinned by the previous release and are consumed unchanged"
            }
          ]
        },
        {
          "id": "S5-continuation-lifetime",
          "outcome": "Continuation lifetime is an operator decision and expiry degrades instead of detonating: the 24-hour retention becomes a manifest knob so multi-day releases stop aging out legitimately resumable state, and a continuation that expires mid-yield falls back to a labeled fresh rehydrate exactly as it already does at dispatch start - never a hard INVALID_CONTINUATION that burns the cycle.",
          "contract_path": "contracts/2026-08-23-unattended-operability/S5-continuation-lifetime.json",
          "digest": "sha256:648eedcc0a316e407e42ad34f8914d56c752bbecd3637e752b51803d7b1814a0",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; no cmd test pins continuation lifetime or yield-resume fallback semantics"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach the touched packages by import only; no cockpit test pins continuation lifetime or yield-resume fallback semantics"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages via internal/cockpit by import only; no observe test pins continuation lifetime or yield-resume fallback semantics"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages via internal/cockpit and directly by import only; no tui test pins continuation lifetime or yield-resume fallback semantics"
            }
          ]
        },
        {
          "id": "S6-provider-dialect-tolerance",
          "outcome": "Aggregator dialects stop failing on decoration: OpenRouter's responses surface works on a released engine instead of a hand-patched binary - usage decorations tolerated with the cost decoration captured as provider-reported cost, data-only SSE streams recognized, and the chat root's provider field admitted so the stale qwen-via-openrouter certification becomes honest again.",
          "contract_path": "contracts/2026-08-23-unattended-operability/S6-provider-dialect-tolerance.json",
          "digest": "sha256:475760e186700575c8b96e56336c6bd6332d6816e2451cc9d98f5b025da4407d",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; no cmd test pins provider response decoration handling"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach the touched packages by import only; no cockpit test pins provider response decoration handling"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages via internal/cockpit by import only; no observe test pins provider response decoration handling"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/driver by import only; decode tolerance changes no runtime-visible contract"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages via internal/cockpit and directly by import only; no tui test pins provider response decoration handling"
            }
          ]
        },
        {
          "id": "S7-replan-survivability",
          "outcome": "A captain escalation recovers unattended: the planner is steered into its summary-before-plan turn instead of dying on it, that turn surfaces as an ordinary answerable attention the operator answers headless, the resumed planner's plan bytes are accepted end-to-end, and a sealed proposal's plan bytes survive every dispatch outcome - so a plan revision never again requires an operator to reconstruct approved-candidate bytes from streamed logs.",
          "contract_path": "contracts/2026-08-23-unattended-operability/S7-replan-survivability.json",
          "digest": "sha256:9fa2eaf057e113a8d87db03e3bff1cc46d085ae8f06983f535df34ac32227c92",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver",
            "internal/journal",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; no cmd test pins planner submission steering or proposal persistence"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach the touched packages by import only; no cockpit test pins planner submission steering or proposal persistence"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages via internal/cockpit by import only; no observe test pins planner submission steering or proposal persistence"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages via internal/cockpit and directly by import only; no tui test pins planner submission steering or proposal persistence"
            }
          ]
        },
        {
          "id": "S8-malformed-toolcall-correction",
          "outcome": "One corrupted tool call no longer costs an hour of finished work: a malformed provider tool call is corrected in-conversation - the defect named back to the model through the same bounded correction machinery submissions already use - and only persistent corruption fails the dispatch, so a provider's transient markup leak becomes a thirty-second nudge instead of a dead try.",
          "contract_path": "contracts/2026-08-23-unattended-operability/S8-malformed-toolcall-correction.json",
          "digest": "sha256:5fa26921d4526c6ef64d16cdd898a0d8bba2128352a076c9589078885a372206",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach the touched packages by import only; no cmd test pins tool-call decode correction"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach the touched packages by import only; no cockpit test pins tool-call decode correction"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach the touched packages via internal/cockpit by import only; no observe test pins tool-call decode correction"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach the touched packages via internal/cockpit and directly by import only; no tui test pins tool-call decode correction"
            }
          ]
        }
      ]
    }
  ]
}
```

# Why

The unattended story is the frontier: telemetry-foundations was delivered
by an operator carrying five plan revisions by hand, killing runs at
escalation boundaries, probing provider walls with curl, and
reconstructing sealed plan bytes from streamed logs. Every one of those
interventions marks a place where an unattended run would simply have
died. Meanwhile the ledger of the last month names the recurring
killers: a stale overnight OAuth token indistinguishable from network
weather (sworn#221), a cleanly-released run wedged forever behind an
expired claim that every verb refuses to touch (sworn#207), honest
worker questions burning two thirds of a try budget to be heard
(sworn#191), a nine-hundred-turn slice invisible to try counting
(sworn#205), multi-day releases aging out resumable context by
compile-time constant (sworn#204), aggregator response decorations
crashing strict decoders (the ox-alpha certification session), the
replan flow structurally dead for headless operation (six doomed
planner dispatches in one release), and a single corrupted provider
tool call costing an hour of finished work three separate times.

This release makes the run survivable without a human in the loop:
credentials refuse loudly before burning tries (S1), wedged claims
reconcile (S2), honest questions park answerably on first ask (S3),
manifest economics actually bind everywhere and stop runaways early
(S4), continuation lifetime is governed and expiry degrades (S5),
aggregator dialects work on a released engine (S6), escalations
recover end-to-end headless (S7), and provider corruption becomes a
correction instead of a casualty (S8).

# What is being pinned

1. Preflight over post-mortem: what the engine can know before a
   dispatch - credential expiry - refuses with a named code instead of
   dispatching into a doomed conversation (S1, sworn#221).
2. Benign rotation is not tampering: ordinary host credential refresh
   racing a dispatch never discards completed work (S1, the
   telemetry-foundations r4 datum).
3. Reconciliation over abandonment: expired claims on external effects
   are reconcilable at re-entry, the operator always holds an
   admissible verb, and board hints derive from the gates the verbs
   check (S2, sworn#207).
4. First-ask parking: question and blocked yields open answerable
   attentions immediately, no try consumed, conversation retained
   (S3, sworn#191).
5. Manifest limits govern: timeout and output bind on the API path
   they never governed; per-work turn and token budgets and the
   identical-failure guard park early with evidence (S4, sworn#205
   guards 2 and 3).
6. Lifetime is an operator decision and expiry degrades: the 24h
   continuation constant becomes a manifest knob; mid-yield expiry
   falls back labeled instead of hard-failing (S5, sworn#204).
7. Aggregator decoration is vocabulary, not corruption: OpenRouter's
   usage decorations (cost captured as provider-reported cost),
   data-only SSE, and the chat provider field are admitted; the two
   operator engine patches land with tests (S6).
8. Escalations recover unattended: yield-first steering at the
   submission layer, the replan turn answerable headless, sealed
   proposal bytes durable (S7).
9. Corruption is corrected, not fatal: malformed tool calls become
   bounded in-conversation corrections with durable accounting (S8).

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-23-unattended-operability/1`, per the operator's
standing direction of 2026-08-23 to author and proceed. Planning did
not approve itself.

One track, eight slices, one mechanism each: credential lifecycle,
claim reconciliation, yield routing, economic governance, continuation
lifetime, decoder tolerance, replan survivability, and tool-call
correction are separate seams with separate failure modes. The
verifier judges each slice by its worker-runnable checks and evidence
anchors; e2e conformance remains declared CI evidence (ADR-0010).

Roles: stealth/ox-alpha implements via the openrouter-ox profile
(responses endpoint, reasoning effort max, streamed - the operator's
model-evaluation experiment, its usage telemetry landing in the eval
capture the previous release built), with deepseek-pro/deepseek-v4-pro
the named fallback implementer; claude-opus-5 captains;
claude-sonnet-5 verifies; deepseek-v4-flash plans and recovers. This
release does not alter trust rules, receipt identity, approval
semantics, containment, or what any control verb is permitted to do;
new manifest knobs follow the validated-bounds pattern with defaults
preserving today's behavior, and all vocabulary growth is additive.

# Revision 2

S1 is delivered (candidate 20a83ef2, verifier PASS in r2). The r2 S2
build proved the contract one surface short the honest way: the
candidate was refused CANDIDATE_SCOPE_FAILED naming exactly one path,
test/e2e/topology_recovery_linux_test.go, whose claimed-state
scenarios pin the pre-reconciliation RECOVERY_UNCERTAIN semantics S2
replaces - the same class as telemetry-foundations revision 3, caught
this time by the scope gate instead of a captain escalation, with the
new failed-observation and tool-result machinery from the previous
release providing the diagnosis in one journal query. An operator
sweep confirms that file is the only e2e surface pinning those
semantics. S2's edit surface gains test/e2e bounded to exactly those
scenarios; S1 and S3-S8 are byte-identical with unchanged digests and
S1's receipts adopt by ancestry. Operator-carried under
operator://2026-08-23-unattended-operability/2.

# Revision 3

S2 is delivered (candidate 574cee5c, verifier PASS in r2; S1 was
delivered in r2 as candidate 20a83ef2). The r3 S3 build proved the
contract one surface short the honest way: the Captain escalated
(S3-honest-yield-parking, receipt ea900b94) that no in-scope
implementation of A1 can keep test/e2e/turn_recovery_linux_test.go
green - the same class revision 2 settled for S2, this time surfaced
by a captain escalation instead of the scope gate. The file's
TestProductionTurnRecoveryParksRestartsAndAccountsExactlyOnce pins the
pre-S3 question/blocked flow's aggregate implementation usage fold
28/20 (lines 1537-1538 and 1649-1651), which includes the automation
invocation the slice removes from the first-occurrence yield; under
first-occurrence answerable parking the fold becomes 21/15, and the S3
design flagged exactly this (design ffb2c242, risk 1). An operator
sweep confirms that file is the only e2e surface pinning that
accounting. S3's edit surface gains test/e2e bounded to exactly those
scenarios; S1, S2, and S4-S8 are byte-identical with unchanged digests
and S1/S2's receipts adopt by ancestry. Operator-carried under
operator://2026-08-23-unattended-operability/3.
