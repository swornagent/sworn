---
title: 'Handoff — ADR-0011 keystone, step 2 (structured-output authoring path)'
description: 'Resume point for the next session. Step 1 (real validation) landed; step 2 is the CapStructuredOutput interface evolution.'
date: 2026-06-30
---

# Handoff — ADR-0011 keystone, step 2

## START HERE (continuation handshake, Rule 6)
On resume, regenerate these from live state and reconcile before any new code:
- `git -C ~/projects/sworn branch --show-current` → expect **`keystone/structured-outputs`**
- `git log --oneline harden/baton-v0.6.3-pin..HEAD` → expect one commit: **`d828ebc` real draft-2020-12 validation**
- `git diff --name-only harden/baton-v0.6.3-pin..HEAD` → `go.mod`, `go.sum`, `internal/baton/validate_schema.go`, `internal/baton/validate_schema_test.go`
- `go build ./...` → OK; `go test ./internal/baton/` → ok
- **gopls is unreliable in this repo** (two sworn checkouts confuse it — the eval clone). Trust `go build`/`go test`, not the IDE diagnostics.

## What this is
Implementing **ADR-0011's authoring path** (ADR-0009's invariant: "the machine parses
JSON only — never prose"): every role/layer message becomes a schema-validated record
emitted via structured output, deleting the free-text scrapers. Full design + the 6 new
schemas + the 6 ratified Type-1 decisions: **`docs/captures/2026-06-29-role-layer-output-schemas.md`**.

## DONE — Step 1 (this session, `d828ebc`)
Real draft-2020-12 validation. `internal/baton/validate_schema.go` adds
`ValidateSchema(name, data)` + `CompiledSchema(name)` (compile cache) over the embedded
schemas, dep `github.com/santhosh-tekuri/jsonschema/v6` (justified per ratified D5 + ADR-0007).
The legacy `baton.Validate` (hand-rolled top-level field checks, `internal/baton/validator.go`)
is **still in place and still the dispatcher** — ValidateSchema is additive infrastructure,
not yet wired in. Tests in `validate_schema_test.go`.

## NEXT — Step 2 (the interface evolution — do this fresh)
**Goal:** let a driver be handed a schema and emit a validated JSON object.
1. **`internal/model/client.go`** — add `CapStructuredOutput` to the `Capability` iota
   (today only `CapVerify`/`CapChat`).
2. **Additive interface** — `type StructuredOutput interface { ChatStructured(ctx, messages []ChatMessage, schema []byte) (*ChatResponse, error) }`. **Do NOT change the `Verify`/`Chat` signatures** (they ripple across 9 drivers + the 5-value Verify; that's how you create a `requiredFields`-class scar). Drivers opt in.
3. **`internal/model/oai.go`** — implement `ChatStructured`: add `response_format: {type:"json_schema", json_schema:{name, schema, strict:true}}` to the request when the driver advertises `CapStructuredOutput`; **tool-call-schema fallback** (one tool whose parameters *are* the schema, `tool_choice` forced) for drivers/models that don't support strict `response_format`. Remember the reasoning-content fallback already in `oai.go` (deepseek v4-pro puts the answer in `reasoning_content`).
4. **Strict generation profile (D1 ratified: lenient canonical for storage, strict projection at call time).** The embedded schemas are `additionalProperties:true` + thin `required` — invalid as OpenAI strict targets. Generate the strict variant (`additionalProperties:false`, all-properties-required, optionals as nullable unions) at call time from the lenient canonical, OR ship a second `*-strict.json`. Validate the returned object with `baton.ValidateSchema` (step 1) before accepting; fail-closed otherwise.
5. **Registry:** advertise `CapStructuredOutput` for `openai`/`openai-responses`/`deepseek` (tool-call works on deepseek — confirmed). Anthropic/claude-cli: not yet (tie to #35).
Build + `go test ./internal/model/...`. Reachability: a real dispatch returns a schema-validated object; a malformed emission fails closed.

## THEN — Step 3 (pilot) and Step 1b
- **Step 3 (pilot, highest leverage):** author **`verifier-verdict-v1`** schema (sketch in the ADR-0011 §3.3); the verifier emits it via `ChatStructured`; validate; **delete `parseInterpretResult`'s `HasPrefix` scrape + `extractViolations`'s prose-split** in `internal/orchestrator/interpreter.go`; merge gate keys off the typed `verdict` enum; invalid → `INCONCLUSIVE` (fail closed). **Explicitly supersedes #32 and #34.** Step 3 needs step 2 + step 1's validator only — NOT the risky 1b rewire.
- **Step 1b (deliberate):** rewire `baton.Validate` → `ValidateSchema` and reconcile the **D6** drift it surfaces (`need_ids`→`covers_needs`; `open_deferrals`/`violations` `[]string`→object). This flips enforcement on existing records — expect breakage in `internal/run` (~28 tests warned about in `validator.go`'s `requiredFields` comment) and the conformance release's committed data; migrate the Go types UP to the schema (D6 ratified direction), don't downgrade schemas.

## Ratified decisions (Brad, Rule-9 Coach, 2026-06-30) — in ADR-0011 §8
D1 lenient-canonical+strict-projection · D2 design/review/verdict=Baton, orchestrator/coach=Sworn ·
D3 `design-v1` canonical writer of `design_decisions` · D4 add `inconclusive` to slice-status enum ·
D5 adopt the validator dep (done) · D6 migrate Go types up to the schemas.

## Other open work (filed, not in the keystone)
Engine-readiness: #25 lint-deferrals · #26 vendor-guard · #27 `--task` handoff · #28 init ·
#29 docs/sworn ADR · #30 start_commit cold-start · #31 openai→responses prefix · #35 anthropic
tool-use · #36 effort/complexity field (rides ADR-0011). Also: **publish the new schemas on
baton-web** (only the old 5 resolve at their `$id`; the rest 404) — prerequisite for model-A
portability. Telemetry transport: `sworn-internal/docs/strategy/2026-06-30-telemetry-eval-transport.md`
(+ `telemetry-event-v1` in ADR-0011 §3.8).

## Branches / where things are
- Keystone work: branch **`keystone/structured-outputs`** (off `harden/baton-v0.6.3-pin`).
- `harden/baton-v0.6.3-pin`: the v0.6.3 baton pin re-vendor + engine fixes #32/#33/#34 (not pushed to origin — local).
- `release/v0.1.0`: pushed to origin; the landed conformance release.
- Baton v0.6.3: released (tag + GH release at sawy3r/baton).
- Eval clone: `~/sworn-eval-engine-readiness` (isolated, local bare origin) — has #32/#33/#34 too; used for the parallel dogfood.
