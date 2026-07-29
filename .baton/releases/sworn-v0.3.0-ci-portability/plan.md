```baton-plan-v2
{
  "schema_version": "baton.plan/v2",
  "release": "sworn-v0.3.0-ci-portability",
  "revision": 1,
  "previous_plan": null,
  "repository": "swornagent/sworn",
  "target_ref": "refs/heads/release/v0.3.0",
  "approval_ref": "github://swornagent/sworn/pulls/158#baton-plan-approval-sworn-v0.3.0-ci-portability-v1",
  "tracks": [
    {
      "id": "T0-ci",
      "depends_on": [],
      "slices": [
        {
          "id": "W0-ci-portability",
          "outcome": "Make the Sworn release gate portable, proportional, and truthful on its supported Linux CI host without changing runtime behavior.",
          "scope": {
            "include": [".github/workflows/ci.yml", "internal/driver/w8_shared_corpus_linux_test.go", "README.md"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-CI-trigger",
              "text": "Full product CI runs once for every pull request and after integration to main. Pushes to Baton track, release-record, and release candidate refs do not duplicate the same gate."
            },
            {
              "id": "A-CI-host",
              "text": "The pinned Ubuntu 24.04 job provisions root-owned /usr/bin/bwrap, enables the required user-namespace facility only on its disposable runner, and proves Bubblewrap starts before exercising Sworn. Sworn retains no uncontained fallback."
            },
            {
              "id": "A-CI-gates",
              "text": "One non-duplicated job has enough time to run formatting, module tidy, full tests, race tests, vet, and the official binary build. The former W0 subset is not repeated separately because the full test command already contains it."
            },
            {
              "id": "A-CI-hermetic",
              "text": "The W8 Bedrock direct-environment corpus uses a test-owned non-executed AWS placeholder and fails if the AWS CLI runner is called. It has no dependency on one developer machine's installed AWS CLI path."
            },
            {
              "id": "A-CI-install",
              "text": "The public README states the Linux Bubblewrap and user-namespace prerequisite in one direct sentence."
            },
            {
              "id": "A-CI-inert",
              "text": "The stripped Sworn binary is byte-identical to the v1.0.0-rc.1 W8 PASS binary, SHA-256 7a72bb6bb25c15147bcd185f8dd28172470a1ba2a1813989ff5f6a39f77d4f28."
            }
          ],
          "checks": [
            "GOFLAGS=-buildvcs=false GOWORK=off go test -count=1 ./internal/driver -run '^TestW8SharedProductionCorpusHasExactSeventyPassRecords$'",
            "go mod tidy -diff",
            "CGO_ENABLED=0 GOFLAGS=-buildvcs=false go build -mod=readonly -buildvcs=false -trimpath -ldflags='-s -w' -o /tmp/sworn-ci-portability ./cmd/sworn",
            "test \"$(sha256sum /tmp/sworn-ci-portability | cut -d' ' -f1)\" = \"7a72bb6bb25c15147bcd185f8dd28172470a1ba2a1813989ff5f6a39f77d4f28\"",
            "git diff --check"
          ],
          "constraints": [
            "No production Go, driver, provider, model, configuration, protocol, scheduler, journal, board, or binary behavior may change.",
            "Do not skip containment-dependent tests or add an uncontained fallback. Relaxing Ubuntu's AppArmor user-namespace switch is allowed only inside the disposable CI runner.",
            "The terminal sworn-v0.3.0-baton-v2 release, its W0 through W8 PASS receipts, and unchanged provider certifications remain immutable and are not rerun.",
            "GitHub CI after exact release-branch integration is an external readiness gate before main; it never creates a Baton verdict or grants main merge, tag, deployment, or publication authority."
          ],
          "depends_on": [],
          "consumes": []
        }
      ]
    }
  ]
}
```

# Goal

Repair the release gate exposed by PR #158 without reopening the completed
Sworn delivery or recertifying unchanged runtime and provider surfaces.

# Authority

The repository owner approved this exact bounded maintenance outcome in the
active delivery session. The protected approval reference is anchored to PR
#158.

# Scope

One workflow, one hermetic test fixture, and one prerequisite sentence. No
production implementation or provider configuration is in scope.

# Acceptance

The workflow has one meaningful trigger per integration surface, provisions and
smoke-tests its required Linux containment, runs the existing complete gates
within a realistic bound, removes duplicated subset work, and carries no local
AWS installation dependency. The official binary remains byte-identical.

# Ordered tracks and slices

One direct track contains one slice because there is one independent maintenance
outcome. A fresh slice PASS can therefore integrate directly without an
additional assembly cycle.

# Dependencies and inputs

The exact approved target already contains the completed Sworn v1.0.0-rc.1
candidate. Prior Baton receipts and provider certifications are historical
evidence, not consumed product inputs for this ancillary repair.

# Checks

Run the affected W8 corpus, module-tidy and diff checks, then rebuild once and
compare the binary with the exact prior W8 PASS hash. The updated PR workflow
runs the full, race, vet, formatting, tidy, and official-build gates after exact
integration.

# Constraints

Keep the repair fail-closed and host-specific. Do not change product behavior,
weaken containment, add a fallback, trigger full CI for Baton bookkeeping refs,
or turn external CI status into Baton authority.
