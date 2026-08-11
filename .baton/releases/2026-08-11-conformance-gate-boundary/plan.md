```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-11-conformance-gate-boundary",
  "revision": 1,
  "previous_plan": null,
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-11-conformance-gate-boundary/1",
  "tracks": [
    {
      "id": "T1-gate",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-declared-checks",
          "outcome": "A slice contract can declare a check that requires containment, and the engine executes it at the host boundary, journals its exit code and bounded output, and binds the result as evidence a verifier reads rather than re-runs - with provenance explicit enough that host-executed evidence can never be mistaken for in-worker evidence.",
          "contract_path": "contracts/2026-08-11-conformance-gate-boundary/S1-declared-checks.json",
          "digest": "sha256:985f14476db6c20ea8a085569e15754b609919e5959782362e94229d1d4043a1",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/runtime",
            "internal/baton",
            "internal/journal"
          ]
        },
        {
          "id": "S2-test-only-uncontained-dispatch",
          "outcome": "End-to-end tests that exercise scheduling, journal semantics, exactly-once and crash recovery run inside a worker sandbox through a test-only uncontained dispatch mode, while tests that prove isolation keep real containment - and no production binary can reach the uncontained path at all.",
          "contract_path": "contracts/2026-08-11-conformance-gate-boundary/S2-test-only-uncontained-dispatch.json",
          "digest": "sha256:3f48eb7fe0e79cc92aa681d19d82f261a8d5f30735b2817997ca2ffd3b59cdf8",
          "depends_on": [
            "S1-declared-checks"
          ],
          "consumes": [
            "S1-declared-checks"
          ],
          "touchpoints": [
            "internal/driver",
            "internal/runtime",
            "test/e2e"
          ]
        }
      ]
    }
  ]
}
```

# Goal

Every slice contract Sworn has ever written declares an end-to-end conformance
check that no worker or verifier can run. Nested containment is prevented
twice over: `trustedBubblewrap` requires uid 0 and a sandboxed `bwrap` reads as
uid 65534, and `--disable-userns` makes the kernel refuse a nested user
namespace outright. Every nested dispatch returns `ISOLATION_UNAVAILABLE` —
155 occurrences in a single recent run.

The gate has appeared to pass only because verifiers correctly classify those
failures as environmental. That held until a contract first required
*empirical* observation of the gate: the implementer could not obtain the
evidence, correctly refused to fabricate conformance-pin revisions that would
have corrupted the suite, and escalated. A human ran the suite on the host and
answered. Delivery completed because a person was available to execute a check
the loop had declared for itself.

This release makes the declaration honest. The engine executes
containment-requiring checks at the host boundary and binds their results as
labelled evidence (S1), and the majority of the suite — the tests about
scheduling, journal semantics, exactly-once and crash recovery rather than
isolation — becomes runnable inside a worker through a test-only uncontained
dispatch gate that no production binary can reach (S2).

Decision and rejected alternatives are recorded in ADR 0010. Notably rejected:
dropping `--disable-userns` to permit nesting, which would grant every
model-directed worker the ability to create user namespaces in order to buy
test-execution convenience. The containment boundary is the product.

Empirical grounding: `docs/captures/2026-08-11-sworn-engine-recording-dogfood-defects.md`
(F15), and issue #190.

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-11-conformance-gate-boundary/1`. Planning did not approve
itself.

Two slices in one serial track: S2 consumes S1 because the split between
host-executed and in-worker checks is expressed through the declaration
mechanism S1 introduces. Scopes are package directories per the revision-5
lesson — acceptance changes behavior that existing tests pin, so the pinning
tests are part of each deliverable.

Per ADR 0010, both slices *declare* the end-to-end check rather than requiring
a role to execute it. Until S1 lands, that check is satisfied at the host
boundary by the operator, and the evidence says so.

Development targets `refs/heads/release/v1.0.0`. Promotion to `main` remains a
separate human-gated step.

This release does not alter trust rules, receipt schemas, role independence, or
the containment boundary for any production binary. It adds no provider, ports
no platform-gated file, and does not change what any existing conformance case
asserts.
