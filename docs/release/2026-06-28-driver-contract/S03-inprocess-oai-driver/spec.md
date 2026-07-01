---
title: 'S03-inprocess-oai-driver'
description: 'Driver-contract re-architecture slice.'
---

# Slice: `S03-inprocess-oai-driver`

## User outcome

The existing in-process agent loop + OpenAI-compatible client is available as ONE `Driver` behind the contract (an option, not the default) — with the content-tag fix — so OAI-compatible models work without the orchestrator touching wire types.

## Entry point

internal/driver/oai (new) — wraps internal/agent + internal/model behind the Driver contract.

## In scope

- New `internal/driver/oai.go`: implement `Driver` by adapting `internal/agent` + `internal/model` (the S27 content-tag fix carried in).
- Exit on 'no tool calls'; turn cap is a circuit-breaker; force one summary turn on empty terminal text.
- All provider-wire handling stays inside this driver; nothing leaks to the orchestrator.

## Acceptance checks

- [ ] WHEN dispatched with an OAI-compatible model, THE SYSTEM SHALL run the multi-turn loop and return a normalized Result.
- [ ] WHEN the model returns a tool-only turn (empty content), THE SYSTEM SHALL still serialize a present `content` field (no omitempty regression).
- [ ] THE oai driver SHALL exit when the model returns no tool calls; the turn cap SHALL act only as a backstop.

## Planned touchpoints

- `internal/driver/oai.go`
- `internal/driver/oai_test.go`

## Required tests

- `go test ./internal/driver/... -run TestOAIDriver`

## Deferrals allowed?

No.
