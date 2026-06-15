---
title: S03-agentic-tool-loop
description: The implementer engine — a read/write/edit/bash/grep/glob tool loop over the model client.
---

# Slice: `S03-agentic-tool-loop`

## User outcome

The engine can drive a model through a tool loop (read/write/edit files, run
commands, grep/glob) to perform multi-step work within a workspace.

## Entry point

Internal `agent` package, consumed by the implementer (S06).

## In scope

- Tool definitions: Read, Write, Edit, Bash, Grep, Glob (workspace-confined).
- The loop: model proposes tool calls → execute → feed results back; turn cap and
  per-tool output cap; operates over the S02 client.

## Out of scope

- Implementer role logic / proof bundle (S06). The verifier does NOT use this.

## Planned touchpoints

- `internal/agent/`

## Acceptance checks

- [ ] Given a task, the loop performs ≥1 file edit and ≥1 command, then terminates.
- [ ] Tool errors are returned to the model (not fatal); the loop continues.
- [ ] All file/command operations are confined to the workspace root.
- [ ] The turn cap halts a non-terminating loop deterministically.

## Required tests

- **Unit**: a fake model emitting scripted tool calls; assert the resulting file
  changes and that the loop terminates at/under the turn cap.

## Risks

- Runaway loop — turn cap + output cap.
- Unsafe command execution — workspace confinement; document the sandbox boundary.

## Deferrals allowed?

No.
