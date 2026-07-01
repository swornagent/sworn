---
title: 'S02-subprocess-agent-driver'
description: 'Driver-contract re-architecture slice.'
---

# Slice: `S02-subprocess-agent-driver`

## User outcome

A subprocess driver delegates the agentic loop to a real agent CLI (claude-cli / codex exec) and returns a normalized Result; this is the default driver, so the orchestrator never owns the tool loop or wire format.

## Entry point

internal/driver/subprocess (new) — selected by default at resolution for agentic roles.

## In scope

- New `internal/driver/subprocess.go`: spawn `claude -p --model <m>` (and a codex variant) with the command/spec, parse stream-json/result into `Result`.
- The driver owns the stop condition (delegated to the agent CLI); the engine sets only a wall-clock timeout.
- Cache/env hygiene: child env sets GOCACHE/GOMODCACHE/HOME outside the worktree so tool-exec never pollutes the diff.
- Maps CLI exit/errors to Result.Status (error) + Subtype; never returns a half-result.

## Acceptance checks

- [ ] WHEN dispatched with a valid agent-CLI model, THE SYSTEM SHALL run the agent to completion and return Result.Status=ok with ResultText + cost/tokens populated.
- [ ] WHEN the agent CLI exits non-zero or times out, THE SYSTEM SHALL return Result.Status=error with a Subtype, not panic or hang.
- [ ] THE driver SHALL set GOCACHE/GOMODCACHE/HOME outside WorktreeRoot for the child process (verified: no cache dirs appear in the worktree after a dispatch).

## Planned touchpoints

- `internal/driver/subprocess.go`
- `internal/driver/subprocess_test.go`

## Required tests

- `go test ./internal/driver/... -run TestSubprocess`

## Deferrals allowed?

No.
