```baton-plan-v2
{
  "schema_version": "baton.plan/v2",
  "release": "2026-08-04-provider-neutral-authorization",
  "revision": 2,
  "previous_plan": "688ff2c940e5af65fd33df1355dea6097969b322",
  "repository": "sworn",
  "target_ref": "refs/heads/main",
  "approval_ref": "operator://2026-08-04-provider-neutral-authorization/2",
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
          "outcome": "One exact, durable, idempotent, journaled approval command service is used identically by the TUI for interactive human approval, MCP for headless agent or orchestrator approval, and CLI for recovery, testing, scripting, and low-level operations.",
          "scope": {
            "include": [
              "shared approval command service",
              "TUI interactive human approval UX",
              "MCP headless agent and orchestrator adapter",
              "CLI recovery, testing, scripting, and low-level operational adapter",
              "exact approval binding, authorization, replay, and run continuation"
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
              "text": "TUI, MCP, and CLI submit the same approval command bound to the exact release, project, target, plan digest, and decision class, apply identical authorization checks, and produce the same approval result."
            },
            {
              "id": "A2",
              "text": "Approval is durably journaled before its effect, is idempotent across interruption and replay, and unblocks only the same exactly approved run."
            },
            {
              "id": "A3",
              "text": "Every adapter rejects missing, stale, wrong, conflicting, insufficient, or ambiguous binding or authority facts without approving or advancing any run."
            },
            {
              "id": "A4",
              "text": "The TUI is the primary interactive human approval UX, MCP is the primary headless agent or orchestrator adapter, and CLI remains available for recovery, testing, scripting, and low-level operations."
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
            "All adapters are projections of one approval command service and cannot alter binding, authorization, replay, or transition semantics.",
            "New scope, changed target, protocol changes, destructive or high-stakes actions, remit expansion, ambiguity, and out-of-policy decisions require explicit human approval and fail closed otherwise.",
            "Missing, stale, conflicting, or insufficient authorization or delegation fails closed; no actor can grant or expand its own remit.",
            "Git identity, provider credentials, authorship, membership, or hosting state never confer approval authority.",
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
          "outcome": "Captain uses the shared approval service to exercise recorded decision-class-specific delegation for Planner proposals and bounded replans only within an explicitly pre-authorized release envelope, while writing only its own bounded governance decisions and detail through the command and effect path.",
          "scope": {
            "include": [
              "Captain submission contract",
              "recorded decision-class-specific delegation",
              "Planner proposal and bounded replan approval within a pre-authorized release envelope",
              "journaled Captain governance decision effect through the shared approval service",
              "PROCEED and in-remit REVISE continuation",
              "durable informational human notification"
            ],
            "exclude": [
              "Captain product-code writes",
              "arbitrary Git ref mutation",
              "human-only authorization classes",
              "self-granted or expanded remit",
              "detached worker process survival"
            ]
          },
          "acceptance": [
            {
              "id": "A1",
              "text": "A Captain invocation can submit only its own bounded governance decision and detail; the command service validates eligibility and writes the exact Baton record through the journaled effect path."
            },
            {
              "id": "A2",
              "text": "Recorded delegation permits Captain to approve a Planner proposal or bounded replan only when that decision class and the exact release envelope are explicitly pre-authorized."
            },
            {
              "id": "A3",
              "text": "Captain approval uses the S2 approval service with the exact release, project, target, plan digest, and decision class bindings and the same authorization, replay, idempotency, and run-unblocking semantics as every other adapter."
            },
            {
              "id": "A4",
              "text": "PROCEED and an in-remit REVISE continue the run without waiting for a human while emitting a durable informational human notification."
            },
            {
              "id": "A5",
              "text": "New scope, changed target, protocol changes, destructive or high-stakes actions, remit expansion, ambiguity, out-of-policy decisions, and ESCALATE require explicit human authority and fail closed otherwise."
            },
            {
              "id": "A6",
              "text": "Crash and retry around Captain submission reconcile to one canonical decision without duplicate approval, record, effect, or continuation."
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
            "Captain capability is recorded, decision-class-specific, release-envelope-bound, attempt-bound, and role-bound.",
            "Missing, stale, conflicting, or insufficient delegation fails closed.",
            "No actor can grant or expand its own remit.",
            "Git identity, provider credentials, authorship, membership, or hosting state never confer approval authority.",
            "Human notification for PROCEED and in-remit REVISE is durable and informational; human-only boundaries remain authorization gates."
          ],
          "depends_on": [
            "S2-approve"
          ],
          "consumes": [
            "S2-approve"
          ]
        }
      ]
    }
  ]
}
```
