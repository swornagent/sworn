```baton-plan-v2
{
  "schema_version": "baton.plan/v2",
  "release": "2026-08-04-provider-neutral-authorization",
  "revision": 1,
  "previous_plan": null,
  "repository": "sworn",
  "target_ref": "refs/heads/main",
  "approval_ref": "operator://2026-08-04-provider-neutral-authorization/1",
  "tracks": [
    {
      "id": "T1",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-authority",
          "outcome": "Sworn starts and resumes releases from a portable project-scoped authority record without requiring GitHub repositories, issues, users, associations, tokens, or APIs.",
          "scope": {
            "include": [
              "project manifest authority",
              "saved approved-plan adoption",
              "runtime authorization state and migration",
              "provider-neutral operator-facing status"
            ],
            "exclude": [
              "remote Git hosting integration",
              "general secret management",
              "worker process supervision"
            ]
          },
          "acceptance": [
            {
              "id": "A1",
              "text": "A project can initialize, start, and resume a release without GitHub-specific repository, issue, user, association, token, environment variable, network request, or approval URL state."
            },
            {
              "id": "A2",
              "text": "An exact already-approved saved plan is adopted idempotently after discovery or restart instead of being rejected or forced through a meaningless revision."
            },
            {
              "id": "A3",
              "text": "Legacy GitHub-shaped run state fails closed with a clear migration status and cannot silently grant portable authority."
            }
          ],
          "checks": [
            "GOFLAGS=-buildvcs=false go test -count=1 ./cmd/sworn ./internal/... ./tools/...",
            "GOFLAGS=-buildvcs=false go test -count=1 -parallel=1 -timeout=20m ./test/e2e",
            "GOFLAGS=-buildvcs=false go test -count=1 -race ./cmd/sworn ./internal/... ./tools/...",
            "GOFLAGS=-buildvcs=false go vet ./...",
            "git diff --check"
          ],
          "constraints": [
            "Authorization is explicit and exact; missing, ambiguous, or stale facts fail closed.",
            "Git transport and hosting provider are outside Sworn authority semantics.",
            "Reserved .baton/releases records are never product inputs."
          ],
          "depends_on": [],
          "consumes": []
        },
        {
          "id": "S2-approve",
          "outcome": "The sworn approve command grants exact local authority for one discovered plan digest and remains safe across interruption and retry.",
          "scope": {
            "include": [
              "sworn approve CLI",
              "exact plan digest binding",
              "duplicate and stale approval handling",
              "journaled authorization effects"
            ],
            "exclude": [
              "hosted issue comments",
              "provider webhooks",
              "interactive identity federation"
            ]
          },
          "acceptance": [
            {
              "id": "A1",
              "text": "sworn approve records authority only for the exact current plan digest and rejects wrong, stale, missing, or ambiguous selections."
            },
            {
              "id": "A2",
              "text": "Approval is journaled, idempotent after interruption, and automatically unblocks the same run without a GitHub dependency."
            },
            {
              "id": "A3",
              "text": "CLI and TUI explain the approved digest, current authority, and next autonomous action in provider-neutral language."
            }
          ],
          "checks": [
            "GOFLAGS=-buildvcs=false go test -count=1 ./cmd/sworn ./internal/... ./tools/...",
            "GOFLAGS=-buildvcs=false go test -count=1 -parallel=1 -timeout=20m ./test/e2e",
            "GOFLAGS=-buildvcs=false go test -count=1 -race ./cmd/sworn ./internal/... ./tools/...",
            "GOFLAGS=-buildvcs=false go vet ./...",
            "git diff --check"
          ],
          "constraints": [
            "No human approval may be inferred from Git authorship, hosting membership, or environment credentials.",
            "A retry cannot duplicate authority or advance an unrelated plan."
          ],
          "depends_on": [],
          "consumes": []
        },
        {
          "id": "S3-baton-rc14",
          "outcome": "Sworn embeds Baton RC14 and durably supplies one configured provider-neutral Git identity before any Baton record-writing effect.",
          "scope": {
            "include": [
              "Baton v1.0.0-rc.14 snapshot and release metadata",
              "engine Git identity configuration",
              "journaled effect input",
              "historical replay after configuration changes"
            ],
            "exclude": [
              "changes to Baton protocol",
              "hosting-provider account management",
              "using Git identity as authority"
            ]
          },
          "acceptance": [
            {
              "id": "A1",
              "text": "Every Baton record-writing effect receives one validated explicit Git name and email; Sworn has no built-in invalid-domain or provider-specific identity."
            },
            {
              "id": "A2",
              "text": "The admitted identity is persisted with the effect before execution so replay remains exact even if current configuration changes."
            },
            {
              "id": "A3",
              "text": "The embedded Baton package, version, manifest, generated assets, Go adapter, and conformance corpus all agree on immutable v1.0.0-rc.14 bytes."
            }
          ],
          "checks": [
            "GOFLAGS=-buildvcs=false go test -count=1 ./cmd/sworn ./internal/... ./tools/...",
            "GOFLAGS=-buildvcs=false go test -count=1 -parallel=1 -timeout=20m ./test/e2e",
            "GOFLAGS=-buildvcs=false go test -count=1 -race ./cmd/sworn ./internal/... ./tools/...",
            "GOFLAGS=-buildvcs=false go vet ./...",
            "git diff --check"
          ],
          "constraints": [
            "Baton changes are vendored only; protocol changes require a separate Baton release.",
            "Git identity is attribution metadata and never role or approval authority.",
            "Historical effects never borrow current configuration."
          ],
          "depends_on": [],
          "consumes": []
        },
        {
          "id": "S4-captain-writes",
          "outcome": "Captain writes its own bounded Baton governance decision and Sworn continues automatically when that decision is within Captain remit.",
          "scope": {
            "include": [
              "Captain submission contract",
              "journaled Captain decision effect",
              "PROCEED and bounded REVISE continuation",
              "informational human notification"
            ],
            "exclude": [
              "Captain product-code writes",
              "arbitrary Git ref mutation",
              "human-only high-stakes authorization",
              "detached worker process survival"
            ]
          },
          "acceptance": [
            {
              "id": "A1",
              "text": "A Captain invocation can submit only its own decision and bounded detail; the command service validates eligibility and writes the exact Baton receipt through the journaled effect path."
            },
            {
              "id": "A2",
              "text": "PROCEED and an in-remit REVISE continue the run without waiting for a human while emitting a durable informational notification."
            },
            {
              "id": "A3",
              "text": "ESCALATE or any requested change outside Captain remit fails closed for explicit human authority, and Captain cannot write product files, commits, refs, or another role's receipt."
            },
            {
              "id": "A4",
              "text": "Crash and retry around Captain submission reconcile to one canonical decision without duplicate effects."
            }
          ],
          "checks": [
            "GOFLAGS=-buildvcs=false go test -count=1 ./cmd/sworn ./internal/... ./tools/...",
            "GOFLAGS=-buildvcs=false go test -count=1 -parallel=1 -timeout=20m ./test/e2e",
            "GOFLAGS=-buildvcs=false go test -count=1 -race ./cmd/sworn ./internal/... ./tools/...",
            "GOFLAGS=-buildvcs=false go vet ./...",
            "git diff --check"
          ],
          "constraints": [
            "Only the command service and journal own mutations.",
            "Captain capability is decision-specific, attempt-bound, and role-bound.",
            "Human notification is informational unless Baton authority requires escalation."
          ],
          "depends_on": [],
          "consumes": []
        }
      ]
    }
  ]
}
```
