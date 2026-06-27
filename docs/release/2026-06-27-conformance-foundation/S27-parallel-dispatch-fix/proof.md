# Proof bundle — S27-parallel-dispatch-fix

## Scope

Make `sworn run --parallel` able to dispatch an agentic implementer and run a multi-turn tool session:
default the nil agent/verifier factories in `RunSlice`, and stop dropping the required `content` field on
tool-only agent turns.

## Files changed

`git diff --name-only 707ba1a` (start_commit):
- `internal/run/slice.go` — default `opts.NewAgent`/`opts.NewVerifier` when nil (mirror `run.Run`)
- `internal/model/oai.go` — `ChatMessage.Content` tag `content,omitempty` → `content`
- `internal/run/factory_default_test.go` — new nil-factory no-panic regression test
- `internal/model/content_tag_test.go` — new content-always-emitted serialization test
- `docs/release/2026-06-27-conformance-foundation/S27-parallel-dispatch-fix/{spec,status,proof}.md`

## Test results

Slice-scoped (`go test ./internal/run/... ./internal/model/... -run 'TestRunSliceDefaultsNilFactories|TestChatMessageAlwaysEmitsContent'`):
```
ok  github.com/swornagent/sworn/internal/run     0.033s
ok  github.com/swornagent/sworn/internal/model   0.014s
```
Both new tests PASS. Full `internal/run` and `internal/model` suites also pass (run during the eval; no
regression).

## Reachability artefact

The 2026-06-28 three-model dogfood (`docs/captures/2026-06-28-sworn-eval-findings.md`) is the end-to-end
artefact:
- BEFORE the fix: `sworn run --parallel` SIGSEGV'd at `internal/run/slice.go` (design-TL;DR dispatch,
  then verify step) — nil factory dereference, before any model call.
- AFTER the fix: the parallel loop dispatched the implementer and ran multi-turn tool sessions on
  DeepSeek and glm (DeepSeek reached `verifying`); no serialization rejects ("missing field content" /
  "content: got null") recurred.

## Delivered

- Nil `NewAgent`/`NewVerifier` defaulting in `RunSlice` — `internal/run/slice.go`; regression test
  `TestRunSliceDefaultsNilFactories` (no panic on the nil-factory path).
- `content` always emitted on tool-only turns — `internal/model/oai.go`; test
  `TestChatMessageAlwaysEmitsContent` asserts `"content":""` present and text content preserved.

## Not delivered

- None for this slice's scope. Related dogfood findings (Responses-API `input[].output`, 25-turn cap,
  retry-worktree-reset, openai-only escalation cascade, cold-start bootstrap, board `actionable`,
  schema validation) are explicitly out of scope and tracked in the findings doc as separate work
  (why: distinct root causes / FT-1/FT-2/FT-3 tracks; acknowledged: Brad, 2026-06-28).

## Divergence from plan

This slice was not in the original 26-slice plan; it was discovered during the 2026-06-28 dogfood as the
prerequisite that unblocks the whole autonomous loop, and added as S27 in T1-orchestration. The two
production fixes were authored by the supervising Coach during the eval (to keep the loop moving) and are
landed here as a tracked slice. Per Rule 7 the slice is `implemented`, not `verified` — it requires a
fresh-context verifier (the Coach who authored the fix cannot self-certify).
