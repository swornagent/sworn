```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-17-loop-economics",
  "revision": 3,
  "previous_plan": "0c3509e9e27e76e08a9fc2685f55f135408d395d",
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-17-loop-economics/3",
  "tracks": [
    {
      "id": "T1-economics",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-refusals-name-paths",
          "outcome": "A refused candidate tells everyone why: seal and scope refusals persist the offending paths into the journal effect result and into the next try's worker context, so no worker ever retries blind against an invisible wall.",
          "contract_path": "contracts/2026-08-17-loop-economics/S1-refusals-name-paths.json",
          "digest": "sha256:5ebaaec97a5de91f3cd2439d0d198a38adc224aeb4b7371d7a88004a950055a2",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/gitx", "internal/baton", "internal/runtime", "internal/journal"]
        },
        {
          "id": "S2-labelled-continuation-exits",
          "outcome": "CONTINUATION_INVALID stops being a bucket: every exit that produces it carries a stable machine-readable site label into the contract error detail and the journal, so the journal alone answers which mechanism fired.",
          "contract_path": "contracts/2026-08-17-loop-economics/S2-labelled-continuation-exits.json",
          "digest": "sha256:6ea0d9217e8342bf36dae364e372ecaf60b13d757d39c780e9fc9b7763875dbf",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/driver"]
        },
        {
          "id": "S3-context-carry-and-degradation-budget",
          "outcome": "Repair attempts inherit their predecessor's knowledge and degradation is loud: a retry or repair dispatch receives the prior try's submission summary and detail as explicit context, every continuation-rehydrate fallback records its reason, and a manifest degradation budget parks the run instead of letting silent context loss burn tokens indefinitely.",
          "contract_path": "contracts/2026-08-17-loop-economics/S3-context-carry-and-degradation-budget.json",
          "digest": "sha256:ab122794f1422d373f0c659cba19cf95b9c201e698ca24a5e58ca102b2fb3375",
          "depends_on": ["S2-labelled-continuation-exits"],
          "consumes": ["S2-labelled-continuation-exits"],
          "touchpoints": ["internal/runtime", "internal/driver", "internal/journal", "cmd/sworn"]
        },
        {
          "id": "S4-parallel-tool-calls",
          "outcome": "Workers stop paying one round-trip per probe: the driver accepts multiple tool calls in a single assistant turn, executes them in submission order against the existing tool session, returns all results in one continuation step, and the role assets tell workers to batch independent probes - the measured 1.15 calls per turn (sworn#201) becomes a batching-shaped distribution.",
          "contract_path": "contracts/2026-08-17-loop-economics/S4-parallel-tool-calls.json",
          "digest": "sha256:ccfe0efc752f8debf3669c4c341e55d70c3bc1a37873f0a6031ea27db6be0c7c",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/driver", "internal/baton", "internal/prompt"]
        },
        {
          "id": "S5-scope-lint-at-recording",
          "outcome": "The revision tax gets a deterministic gate: plan recording computes the reverse-dependency closure of every slice's scope and refuses to record when a package containing tests or golden fixtures that pin scope-package behaviour lies outside scope and is not explicitly waived in the plan, so under-derived scope is caught at authoring time instead of after a sealed candidate dies.",
          "contract_path": "contracts/2026-08-17-loop-economics/S5-scope-lint-at-recording.json",
          "digest": "sha256:db71d1686716ace0d156b51075ee47a1353efdb2d23e33edcef6be72c0661a33",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/baton", "internal/gitx", "tools"]
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
`operator://2026-08-17-loop-economics/3`. Planning did not approve itself.
Revision 3 re-binds revision 2 unchanged to the head carrying quota
pacing (sworn#217): three runs proved gemini-3.7-flash cannot survive an
implementation dispatch against the 3M input-tokens/min tier-2 quota
without it. The driver now paces at the operator-stated cap and honours
the provider's RetryInfo instead of restarting the dispatch.

One serial track. S3 consumes S2's labels. S4's scope was derived by
enumerating role-asset pins first (the conformance-gate lesson); S5's
regression corpus is the four historical scope failures themselves.
Roles: gemini-3.7-flash implements via the google-native profile - its
first release - and grok-4.6
verifies; DeepSeek, GLM and Qwen stand as certified alternates.

This release does not alter trust rules, receipt schemas, wire
vocabulary, containment, or what any gate admits. Error codes recorded in
existing journals are never renamed; every new signal is additive.
