```baton-plan-v2
{
  "schema_version": "baton.plan/v2",
  "release": "sworn-v0.3.0-ci-reliability",
  "revision": 1,
  "previous_plan": null,
  "repository": "swornagent/sworn",
  "target_ref": "refs/heads/release/v0.3.0",
  "approval_ref": "github://swornagent/sworn/pulls/158#baton-plan-approval-sworn-v0.3.0-ci-reliability-v1",
  "tracks": [
    {
      "id": "T0-ci",
      "depends_on": [],
      "slices": [
        {
          "id": "W0-ci-reliability",
          "outcome": "Make the complete Sworn release gate reliable under GitHub runner contention without changing product behavior or weakening functional coverage.",
          "scope": {
            "include": [".github/workflows/ci.yml", "cmd/sworn/binary_integration_test.go"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-CI-product",
              "text": "Product package tests remain parallel and cover cmd/sworn, internal packages, and tools once."
            },
            {
              "id": "A-CI-e2e",
              "text": "The process-heavy test/e2e package retains every functional journey but runs separately with one top-level test at a time and a suite-only timeout that cannot starve its valid inner command deadlines."
            },
            {
              "id": "A-CI-race",
              "text": "Race instrumentation covers the shipping product packages. The E2E harness is not repeated under race because its spawned Sworn binaries are ordinary separately built processes and therefore are not race-instrumented by go test -race."
            },
            {
              "id": "A-CI-git",
              "text": "The test-owned temporary Git repository disables automatic maintenance and GC before add and commit so no detached Git writer races TempDir cleanup."
            },
            {
              "id": "A-CI-inert",
              "text": "No runtime source changes, no inner E2E deadline changes, no provider recertification, and the stripped Sworn binary remains byte-identical with SHA-256 7a72bb6bb25c15147bcd185f8dd28172470a1ba2a1813989ff5f6a39f77d4f28."
            }
          ],
          "checks": [
            "GOFLAGS=-buildvcs=false GOWORK=off go test -count=50 -run '^TestProductCopyAndArchiveExcludeBatonRecords$' ./cmd/sworn",
            "GOFLAGS=-buildvcs=false GOWORK=off go test -count=1 -parallel=1 -timeout=20m ./test/e2e",
            "go mod tidy -diff",
            "CGO_ENABLED=0 GOFLAGS=-buildvcs=false go build -mod=readonly -buildvcs=false -trimpath -ldflags='-s -w' -o /tmp/sworn-ci-reliability ./cmd/sworn",
            "test \"$(sha256sum /tmp/sworn-ci-reliability | cut -d' ' -f1)\" = \"7a72bb6bb25c15147bcd185f8dd28172470a1ba2a1813989ff5f6a39f77d4f28\"",
            "git diff --check"
          ],
          "constraints": [
            "Change only the approved workflow and test-owned temporary Git setup.",
            "Do not skip functional E2E coverage, increase inner HTTP or subprocess deadlines, change product behavior, or add retries that mask failures.",
            "The completed release receipts and unchanged provider certifications remain immutable and are not rerun.",
            "GitHub CI remains an external readiness gate and never creates a Baton verdict or grants merge, tag, deployment, or publication authority."
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

Repair the independent CI contention and temporary-Git cleanup failures exposed
after the Linux portability repair, without reopening the completed Sworn
delivery.

# Delivery

One track and one slice own two test surfaces. Product tests stay parallel,
process-heavy journeys run once in a bounded serial lane, and race
instrumentation stays on the product code it actually instruments. The
test-owned repository opts out of background Git maintenance. The exact release
binary must remain unchanged.
