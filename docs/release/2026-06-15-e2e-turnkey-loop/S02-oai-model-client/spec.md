---
title: S02-oai-model-client
description: OpenAI-compatible chat-completions client — turns the stub into a real verdict. BYO-key, customer-chosen, safe-hosted default.
---

# Slice: `S02-oai-model-client`

## User outcome

With a configured model + key, `sworn verify` produces a **real** adversarial
verdict from an OpenAI-compatible endpoint (not the fail-closed stub).

## Entry point

Internal `model.Verifier` implementation, selected by config; surfaced via
`sworn verify --verifier-model <id>` + provider key in env.

## In scope

- `net/http` chat-completions client (`/chat/completions`), provider + key
  resolution from env (multiple providers), usage → `cost_usd` calc.
- Wire the client into `verify.Run` (replace `Unconfigured`).
- Document a **safe-hosted default** posture (no non-trusted-hosted model blessed
  as default; explicit selection required until the S10 benchmark picks one).

## Out of scope

- The full agentic tool loop (S03 — the verifier needs a completion, not an agent).
- Streaming (optional, later).

## Planned touchpoints

- `internal/model/` (new OAI client), `internal/verify/verify.go` (wire)

## Acceptance checks

- [ ] A real PASS and a real FAIL are produced from a (fake/live) endpoint.
- [ ] `cost_usd` is computed from token usage and surfaced in the verdict.
- [ ] Provider key is read from env (BYO-key); never logged.
- [ ] An HTTP/timeout error → BLOCKED (fail-closed), not a crash or false PASS.

## Required tests

- **Unit**: table-driven against an `httptest` fake server — PASS reply, FAIL
  reply, HTTP 500, timeout (each → expected verdict).
- **Reachability artefact**: `sworn verify ... --verifier-model <id>` against a
  fake server prints a real verdict with non-zero cost.

## Risks

- Provider response-shape variance — normalise; fail closed on unrecognised shape.
- Key/payload leakage in logs — never log request bodies or keys.

## Deferrals allowed?

No.
