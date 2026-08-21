```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-21-detached-conformance",
  "revision": 1,
  "previous_plan": null,
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-21-detached-conformance/1",
  "tracks": [
    {
      "id": "T1-conformance",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-conformance-repin",
          "outcome": "The conformance suite tells the truth about detached control verbs: every real-binary scenario that used answer, resume, or takeover as the engine's driver now records the verb's durable return and performs the drive as its own explicit step - so the e2e gate proves the merged detached-drive semantics instead of mourning the old ones, and the CI run on the merge is the release's own green evidence.",
          "contract_path": "contracts/2026-08-21-detached-conformance/S1-conformance-repin.json",
          "digest": "sha256:43cf94f436f6b3ebe7e7a4d235f7656b0d675617e7935930d30c86f5f9b8cec9",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["test/e2e"]
        }
      ]
    }
  ]
}
```

# Why

The 2026-08-20-operator-surface release merged with every slice and the
assembly independently verified, and then CI run 32436674619 failed 74
end-to-end tests through one mechanism: the conformance suite encodes
the pre-detached world in which answer, resume, and takeover drove the
run in the calling process, and asserts driven states on those verbs'
immediate output. The merged S2-detached-drive contract deliberately
ended that world - control verbs return once the command is durable,
and driving belongs to run or a resident serve. The unit suites are
green at the same head; only the conformance suite's assumptions are
stale. e2e is a CI-only gate by ruling (stripped from slice contracts
in loop-economics), so this collision was structurally invisible until
the merge - the declared-not-executed gap at release scale.

# What is being pinned

Three readings, operator-ruled 2026-08-21, all conservative:

1. Control verbs return once durable; observing driven state is a
   separate drive of the same journal.
2. Crash-cut scenarios cut the drive, not the control verb - exit 86
   was always the test crash hook's own signature, never a product
   exit code, so the cut moves to where driving now happens.
3. An identical-command replay is idempotent and duplicates no
   authority; conformance asserts the cardinality (one attention.answer
   command per answered attention), not the refusal shape.

No product package changes. A scenario that cannot be repinned without
an engine change is an escalation, not a workaround. No exactly-once,
cardinality, or authority assertion may be weakened.

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-21-detached-conformance/1`. Planning did not
approve itself.

One track, one slice, one mechanism per the planner doctrine: the
repin is a single doctrine applied across the suite, and splitting it
by file would add toll without a mechanism boundary. The verifier
judges the repinned scenarios by reading them against the acceptance
criteria and by the compile coverage in the checks; the first full
execution is the CI run on the merge, which this plan names as the
release's own evidence.

Roles: gemini-3.7-flash implements via the google-native profile under
transport pacing with the 2.4M input-token headroom cap; grok-4.6
verifies via xai-grok (qwencloud's weekly token plan is exhausted
until 2026-08-25); deepseek plans and captains. This release does not
alter trust rules, receipt schemas, wire vocabulary, containment,
journal schemas, or what any gate admits.
