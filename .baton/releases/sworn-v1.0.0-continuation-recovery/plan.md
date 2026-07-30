```baton-plan-v2
{
  "schema_version": "baton.plan/v2",
  "release": "sworn-v1.0.0-continuation-recovery",
  "revision": 1,
  "previous_plan": null,
  "repository": "swornagent/sworn",
  "target_ref": "refs/heads/release/v0.3.0",
  "approval_ref": "github://swornagent/sworn/issues/157#baton-plan-approval-sworn-v1.0.0-continuation-recovery-v1",
  "tracks": [
    {
      "id": "T0-contract",
      "depends_on": [],
      "slices": [
        {
          "id": "W0-continuation-contract",
          "outcome": "Define one bounded role-neutral continuation capability whose loss can affect efficiency but never Baton authority or correctness.",
          "scope": {
            "include": ["internal/driver/continuation.go", "internal/driver/invoke.go", "internal/driver/selection.go", "docs/captures/2026-07-30-sworn-continuation-and-turn-recovery.md"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-CON-handle",
              "text": "A process-local non-serializable continuation handle is bound to the exact run, release, slice lineage, Implementer role, plan and target authority, adapter configuration, profile, model, and tool contract."
            },
            {
              "id": "A-CON-authority",
              "text": "A continuation owns no live workspace lease, Git authority, submission permission, or Baton decision. Every resumed turn receives a new invocation and tool session after exact binding revalidation."
            },
            {
              "id": "A-CON-lifecycle",
              "text": "Handles are bounded and explicitly zeroed on close, mismatch, cancellation, candidate completion, expiry, or resource overflow. They have no JSON, journal, board, notification, log, or OTel content surface."
            },
            {
              "id": "A-CON-fallback",
              "text": "Unsupported, lost, stale, or invalid continuation always falls back once to fresh rehydration from durable inputs and cannot change the resulting Baton operation."
            }
          ],
          "checks": [
            "GOFLAGS=-buildvcs=false GOWORK=off go test -count=1 ./internal/driver -run 'Continuation|Selection|Invocation'",
            "go test -count=1 -race ./internal/driver",
            "git diff --check"
          ],
          "constraints": [
            "Keep one common capability; do not create provider-specific role drivers or a second scheduler.",
            "Do not persist raw transcript, reasoning, provider cursor, native session ID, prompt, tool payload, or credential.",
            "Verifier remains unconditionally fresh and read-only."
          ],
          "depends_on": [],
          "consumes": []
        }
      ]
    },
    {
      "id": "T1-api",
      "depends_on": ["T0-contract"],
      "slices": [
        {
          "id": "W1-api-continuation",
          "outcome": "Resume API-backed Implementer conversations by exact transcript and opaque-provider-state replay across the Captain handoff.",
          "scope": {
            "include": ["internal/driver/provider.go", "internal/driver/openai.go", "internal/driver/responses.go", "internal/driver/gemini.go", "internal/driver/bedrock.go", "internal/driver/mantle.go", "internal/driver/config.go", "internal/driver/profile.go"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-API-resume",
              "text": "Before suspension, a successful sworn_submit gains its exact accepted tool result. Resume appends the new implementation envelope, Captain receipt, invocation identity, and newly admitted read-write tool surface."
            },
            {
              "id": "A-API-chat",
              "text": "Standard Chat Completions resumes from the complete bounded assistant, tool-call, and tool-result transcript without relying on a server session."
            },
            {
              "id": "A-API-opaque",
              "text": "Responses encrypted items, DeepSeek reasoning_content, OpenRouter reasoning_details, Gemini thoughtSignature, and Bedrock reasoning blocks are retained and replayed exactly through only their admitted dialect; hidden reasoning is never inferred."
            },
            {
              "id": "A-API-responses",
              "text": "OpenAI Responses initially remains store:false with exact client-owned item replay. Server cursors and compaction are optional later capabilities, not correctness dependencies."
            },
            {
              "id": "A-API-bounds",
              "text": "No codec silently truncates or translates continuation. A size, ordering, correlation, dialect, or context failure discards the handle and requests fresh rehydration."
            }
          ],
          "checks": [
            "GOFLAGS=-buildvcs=false GOWORK=off go test -count=1 ./internal/driver -run 'Responses|OpenAI|DeepSeek|OpenRouter|Gemini|Bedrock|Mantle|Continuation'",
            "go test -count=1 -race ./internal/driver",
            "git diff --check"
          ],
          "constraints": [
            "Do not add a provider SDK, framework, hosted tool, managed inference path, automatic provider fallback, or dialect guessing.",
            "Opaque reasoning bytes must remain memory-only, bounded, ordered, and unmodified.",
            "Changed driver surfaces require targeted certification; unchanged profile and model identities retain their existing evidence."
          ],
          "depends_on": ["W0-continuation-contract"],
          "consumes": ["W0-continuation-contract"]
        }
      ]
    },
    {
      "id": "T2-native",
      "depends_on": ["T0-contract"],
      "slices": [
        {
          "id": "W2-native-continuation",
          "outcome": "Resume certified Codex and Claude Implementer sessions through private Sworn-owned native session state while preserving fresh Captain and Verifier isolation.",
          "scope": {
            "include": ["internal/driver/native.go", "internal/driver/native_linux.go", "internal/driver/native_capture_linux.go", "internal/driver/broker.go"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-NAT-session",
              "text": "Sworn captures and validates the native session identity, retains only a bounded private session home for the same Implementer lineage, and uses the vendor's explicit resume command."
            },
            {
              "id": "A-NAT-isolation",
              "text": "Captain receives a separate fresh home. Verifier retains no user config, rules, memories, prior session, or writable workspace and cannot resume any other role."
            },
            {
              "id": "A-NAT-tools",
              "text": "The resumed Implementer transition from design read-only authority to implementation read-write authority is newly surface-certified; stale capability material cannot cross the handoff."
            },
            {
              "id": "A-NAT-cleanup",
              "text": "Session homes and identifiers are never journalled or observed and are deleted on completion, cancellation, mismatch, expiry, or process recovery; deletion failure parks safely."
            },
            {
              "id": "A-NAT-fallback",
              "text": "An unavailable or failed native resume uses fresh durable rehydration once. It never selects another driver, model, credential, or tool authority."
            }
          ],
          "checks": [
            "GOFLAGS=-buildvcs=false GOWORK=off go test -count=1 ./internal/driver -run 'Native|Codex|Claude|Continuation'",
            "go test -count=1 -race ./internal/driver",
            "git diff --check"
          ],
          "constraints": [
            "No ambient native home, interactive prompt, persistent user memory, or user-wide session store may enter Sworn.",
            "Pin and certify exact native versions and resume argv; do not weaken closure or binary identity checks.",
            "No model-written shell recovery command may execute."
          ],
          "depends_on": ["W0-continuation-contract"],
          "consumes": ["W0-continuation-contract"]
        }
      ]
    },
    {
      "id": "T3-runtime",
      "depends_on": ["T1-api", "T2-native"],
      "slices": [
        {
          "id": "W3-resumed-implementer",
          "outcome": "Use continuation only for the same Implementer across distinct Captain review, with exact authority revalidation and truthful fresh fallback.",
          "scope": {
            "include": ["internal/runtime/production_dispatch.go", "internal/runtime/production_driver.go", "internal/runtime/dispatch.go", "internal/runtime/service.go", "internal/runtime/scheduler.go", "internal/observe"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-RUN-flow",
              "text": "Implementer design begins fresh, suspends its bounded continuation, Captain reviews independently, and only the exact current Captain receipt resumes that same Implementer for implementation."
            },
            {
              "id": "A-RUN-fresh",
              "text": "Planner and Captain are distinct invocations, and every work and assembly Verifier is fresh, read-only, and unable to receive an Implementer continuation."
            },
            {
              "id": "A-RUN-revalidate",
              "text": "Run, track, slice, attempt, plan, target, design, Captain, adapter, profile, model, and tool facts are revalidated before resume. Drift discards the handle and rehydrates fresh."
            },
            {
              "id": "A-RUN-restart",
              "text": "Process restart requires no raw continuation recovery: durable design and Captain inputs deterministically reconstruct a fresh implementation turn."
            },
            {
              "id": "A-RUN-observe",
              "text": "Local eval and opt-in OTel report only continuation mode, reuse, fallback, expiry, replay-size buckets, tokens, and elapsed time, with no content or session identifiers."
            }
          ],
          "checks": [
            "GOFLAGS=-buildvcs=false GOWORK=off go test -count=1 ./internal/runtime ./internal/observe",
            "go test -count=1 -race ./internal/runtime ./internal/observe",
            "git diff --check"
          ],
          "constraints": [
            "Continuation remains advisory process state; Git, Baton records, and the journal remain truth.",
            "Do not retain a live workspace, lease, process, or submission permission across Captain review.",
            "No continuation fact may create a lifecycle transition or suppress fresh verification."
          ],
          "depends_on": ["W1-api-continuation", "W2-native-continuation"],
          "consumes": ["W1-api-continuation", "W2-native-continuation"]
        },
        {
          "id": "W4-turn-recovery",
          "outcome": "Restore Coach's fast semantic recovery seam as bounded non-authoritative Sworn automation that keeps independent tracks moving.",
          "scope": {
            "include": ["cmd/sworn", "internal/driver/tools.go", "internal/driver/recovery.go", "internal/driver/config.go", "internal/driver/selection.go", "internal/runtime", "internal/journal", "internal/cockpit", "internal/observe", "docs/releases/v0.3.0"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A-REC-config",
              "text": "Recovery has an explicit Sworn automation profile and model with no bundled default. It uses the common driver layer but is not a Baton role or responsibility."
            },
            {
              "id": "A-REC-yield",
              "text": "One bounded sworn_yield tool represents question or blocked. Valid sworn_submit bypasses recovery; malformed submission receives at most two same-session corrections."
            },
            {
              "id": "A-REC-actions",
              "text": "The controller may only resume_worker, ask_captain, retry_operationally, or pause_track_for_human. It cannot synthesize a submission or Baton decision, change scope, execute shell, move refs, or Merge."
            },
            {
              "id": "A-REC-order",
              "text": "Deterministic transport and durable-state diagnosis outrank interpretation. Exact engine facts may answer a question; judgment routes to a distinct non-gate Captain advisory turn; uncertainty pauses only that track."
            },
            {
              "id": "A-REC-human",
              "text": "Human attention is durable, visible on board and terminal, answerable by a typed idempotent command, and push-notified through the existing bounded webhook outbox without leaking hidden context."
            },
            {
              "id": "A-REC-bounds",
              "text": "Per-turn correction, operational retry, same-progress, and total recovery bounds prevent loops. Exhaustion parks the affected track while independent work continues."
            },
            {
              "id": "A-REC-eval",
              "text": "Shared and real-binary scenarios measure recovery success, false acceptance, human escalation, token delta, and elapsed time. False acceptance remains zero."
            }
          ],
          "checks": [
            "GOFLAGS=-buildvcs=false GOWORK=off go test -count=1 ./internal/driver ./internal/runtime ./internal/journal ./internal/cockpit ./internal/observe",
            "GOFLAGS=-buildvcs=false GOWORK=off go test -count=1 -parallel=1 -timeout=20m ./test/e2e",
            "go test -count=1 -race ./cmd/sworn ./internal/... ./tools/...",
            "go vet ./...",
            "go mod tidy -diff",
            "git diff --check"
          ],
          "constraints": [
            "Do not restore the original BLOCKED:<shell> plus bash -c surface, unbounded RETRY, or prose-derived verifier outcomes.",
            "Do not add LangChain, LangGraph, DBOS, or another orchestration runtime; keep the engine in Go and project standard OTel.",
            "Do not reset or replace existing slices, recertify unchanged drivers, or create proof artefacts beyond the plan, compact receipts, capture, tests, and release notes."
          ],
          "depends_on": ["W3-resumed-implementer"],
          "consumes": []
        }
      ]
    }
  ]
}
```

# Goal

Close the two remaining Coach-parity gaps without expanding Baton: preserve the
same Implementer's bounded context through distinct Captain review, and recover
semantically from healthy turns that do not end in the expected terminal
submission.

# Shape

One small contract slice enables API and native work in parallel. Runtime then
uses both through one capability, and one final slice adds bounded recovery,
human attention, notification, evaluation, and release evidence. Existing
successful driver evidence remains valid unless that exact driver surface
changes.

# Non-goals

Continuation never becomes durable authority. The plan excludes managed
inference, role-specific drivers, raw telemetry, model-generated shell
remediation, server-state dependency, and a second orchestration framework.
