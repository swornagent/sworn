```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-17-loop-economics",
  "revision": 6,
  "previous_plan": "5f6c5fa411dc81e96ceed45cb1a8b4f2ad86ced5",
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-17-loop-economics/6",
  "tracks": [
    {
      "id": "T1-economics",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-refusals-name-paths",
          "outcome": "A refused candidate tells everyone why: seal and scope refusals persist the offending paths into the journal effect result and into the next try's worker context, so no worker ever retries blind against an invisible wall.",
          "contract_path": "contracts/2026-08-17-loop-economics/S1-refusals-name-paths.json",
          "digest": "sha256:5852f85087889a57844664f191fb59a84b44f71561d78053354d55b54c6bdff4",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/gitx", "internal/baton", "internal/runtime", "internal/journal"]
        },
        {
          "id": "S2-labelled-continuation-exits",
          "outcome": "CONTINUATION_INVALID stops being a bucket: every exit that produces it carries a stable machine-readable site label into the contract error detail and the journal, so the journal alone answers which mechanism fired.",
          "contract_path": "contracts/2026-08-17-loop-economics/S2-labelled-continuation-exits.json",
          "digest": "sha256:6144eb25801e2a25568283ee354fc5e30d7653690eb7dd69679d21f08c275952",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/driver", "internal/runtime"]
        },
        {
          "id": "S3-context-carry-and-degradation-budget",
          "outcome": "Repair attempts inherit their predecessor's knowledge and degradation is loud: a retry or repair dispatch receives the prior try's submission summary and detail as explicit context, every continuation-rehydrate fallback records its reason, and a manifest degradation budget parks the run instead of letting silent context loss burn tokens indefinitely.",
          "contract_path": "contracts/2026-08-17-loop-economics/S3-context-carry-and-degradation-budget.json",
          "digest": "sha256:0b7676b93aaa494572e29722e10d8effea779d4c459ffbeb4f8c01470fa94269",
          "depends_on": ["S2-labelled-continuation-exits"],
          "consumes": ["S2-labelled-continuation-exits"],
          "touchpoints": ["internal/runtime", "internal/driver", "internal/journal", "cmd/sworn"]
        },
        {
          "id": "S4-parallel-tool-calls",
          "outcome": "Workers stop paying one round-trip per probe: the driver accepts multiple tool calls in a single assistant turn, executes them in submission order against the existing tool session, returns all results in one continuation step, and the role assets tell workers to batch independent probes - the measured 1.15 calls per turn (sworn#201) becomes a batching-shaped distribution.",
          "contract_path": "contracts/2026-08-17-loop-economics/S4-parallel-tool-calls.json",
          "digest": "sha256:80f5fd4df6abf425f4690512f8e730c02892b2ba2920e15a4cabbfffc71870a5",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/driver", "internal/baton", "internal/prompt"]
        },
        {
          "id": "S5-scope-lint-at-recording",
          "outcome": "The revision tax gets a deterministic gate: plan recording computes the reverse-dependency closure of every slice's scope and refuses to record when a package containing tests or golden fixtures that pin scope-package behaviour lies outside scope and is not explicitly waived in the plan, so under-derived scope is caught at authoring time instead of after a sealed candidate dies.",
          "contract_path": "contracts/2026-08-17-loop-economics/S5-scope-lint-at-recording.json",
          "digest": "sha256:e731331ee93fc4f585796f2064f63da33e440e10786a20b650da954fffda36be",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/baton", "internal/gitx", "internal/driver", "internal/runtime", "tools"]
        }
      ]
    }
  ]
}
```

# Goal

One week of delivery measured exactly where the loop bleeds: three
six-hundred-turn tries burned against a seal refusal nobody could see
(sworn#203); three unrelated faults hiding behind one opaque error string
(sworn#187); sixty percent of dispatches silently losing their
predecessor's context and forty turns per repair spent reconstructing it
(sworn#204, #205); one probe per round-trip across four hundred turns of
archaeology (sworn#201); and four under-derived contract scopes costing
two plan revisions and a lost candidate (sworn#199).

This release converts each measurement into machinery. Refusals carry
their evidence to the worker that must act on it. Continuation failures
name their mechanism in the journal. Repair dispatches inherit the prior
try's submission, rehydrate fallbacks say why, and a degradation budget
parks a run rather than letting it burn quietly. Tool calls batch. Plan
recording refuses under-derived scope deterministically.

None of this changes what the gates admit - only what they say, and what
the next actor knows. The loop's checks stay exactly as strict; they stop
being silent.

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-17-loop-economics/6`. Planning did not approve itself.
Revision 6 resolves S5's captain escalation exactly as raised: the
scope lint's A4 runs at proposal validation, which lives in
internal/driver and internal/runtime, so S5's scope now includes both
- the release's fifth scope under-derivation, caught on the slice
that exists to end the class. Revision 5 resolved S2's escalation exactly as raised: A3
requires the site label readable back from the journal, and the effect
completion recording it lives in internal/runtime, so S2's scope now
includes internal/runtime. Acceptance criteria are unchanged. The
verifier moves to claude-opus-5 via the claude_code_cli native closure
(2.1.234); grok-4.6 stands down on cost.

One serial track. S3 consumes S2's labels. S4's scope was derived by
enumerating role-asset pins first (the conformance-gate lesson); S5's
regression corpus is the four historical scope failures themselves.
Roles: gemini-3.7-flash implements via the google-native profile - its
first release - and grok-4.6
verifies; DeepSeek, GLM and Qwen stand as certified alternates.

This release does not alter trust rules, receipt schemas, wire
vocabulary, containment, or what any gate admits. Error codes recorded in
existing journals are never renamed; every new signal is additive.
