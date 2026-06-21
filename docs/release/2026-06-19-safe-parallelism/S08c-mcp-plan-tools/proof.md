---
title: Slice proof bundle — S08c-mcp-plan-tools
description: Rule 6 proof bundle, generated from live repo state.
---

# Proof Bundle: S08c-mcp-plan-tools

## Scope

An AI assistant connected to `sworn mcp` can plan an entire release — calling `create_slice`, `set_track`, and `update_intake` to write all artefacts into the repo — and pull the planner/implementer/verifier role prompts as MCP resources, without the developer running any CLI commands or copying template files manually.

## Files changed

```
$ git diff --name-only ef0ec299f4ab6f86f8ab38acb2b2057ba0f8b3a4
cmd/sworn/mcp.go
docs/release/2026-06-19-safe-parallelism/S08c-mcp-plan-tools/status.json
internal/mcp/prompts.go
internal/mcp/resources.go
internal/mcp/server.go
internal/mcp/tools_plan.go
internal/mcp/baton/track-mode.md
internal/prompt/prompt.go
internal/mcp/tools_plan_test.go
docs/mcp-setup.md
```

## Test results

### Go

```
$ go test ./internal/mcp/... -v
=== RUN   TestInitializeHandshake
--- PASS: TestInitializeHandshake (0.00s)
=== RUN   TestInitializedNotification
--- PASS: TestInitializedNotification (0.00s)
=== RUN   TestToolsListEmpty
--- PASS: TestToolsListEmpty (0.00s)
=== RUN   TestUnknownMethod
--- PASS: TestUnknownMethod (0.00s)
=== RUN   TestUnregisteredToolCall
--- PASS: TestUnregisteredToolCall (0.00s)
=== RUN   TestRegisteredToolStub
--- PASS: TestRegisteredToolStub (0.00s)
=== RUN   TestResourcesList
--- PASS: TestResourcesList (0.00s)
=== RUN   TestPromptsList
--- PASS: TestPromptsList (0.00s)
=== RUN   TestBatchRejection
--- PASS: TestBatchRejection (0.00s)
=== RUN   TestInvalidJSON
--- PASS: TestInvalidJSON (0.00s)
=== RUN   TestServerContextCancellation
--- PASS: TestServerContextCancellation (0.00s)
=== RUN   TestCreateRelease
--- PASS: TestCreateRelease (0.00s)
=== RUN   TestCreateSlice
--- PASS: TestCreateSlice (0.00s)
=== RUN   TestCreateSliceDuplicate
--- PASS: TestCreateSliceDuplicate (0.00s)
=== RUN   TestSetTrackValidation
--- PASS: TestSetTrackValidation (0.00s)
=== RUN   TestSetTrackUpdates
--- PASS: TestSetTrackUpdates (0.00s)
=== RUN   TestSetTrackColon
--- PASS: TestSetTrackColon (0.00s)
=== RUN   TestUpdateIntakeAppends
--- PASS: TestUpdateIntakeAppends (0.00s)
=== RUN   TestUpdateIntakeCreatesSection
--- PASS: TestUpdateIntakeCreatesSection (0.00s)
=== RUN   TestResourceReadPrompt
--- PASS: TestResourceReadPrompt (0.00s)
=== RUN   TestResourceReadBatonVersion
--- PASS: TestResourceReadBatonVersion (0.00s)
=== RUN   TestResourceReadReleaseBoard
--- PASS: TestResourceReadReleaseBoard (0.00s)
=== RUN   TestResourceReadProofAbsent
--- PASS: TestResourceReadProofAbsent (0.00s)
=== RUN   TestResourceReadSliceSpec
--- PASS: TestResourceReadSliceSpec (0.00s)
=== RUN   TestPromptsGetPlanner
--- PASS: TestPromptsGetPlanner (0.00s)
=== RUN   TestPromptsGetImplementer
--- PASS: TestPromptsGetImplementer (0.00s)
=== RUN   TestPromptsGetVerifier
--- PASS: TestPromptsGetVerifier (0.00s)
=== RUN   TestPromptsListEnumerates
--- PASS: TestPromptsListEnumerates (0.00s)
=== RUN   TestResourceReadTrackMode
--- PASS: TestResourceReadTrackMode (0.00s)
=== RUN   TestResourceReadIntake
--- PASS: TestResourceReadIntake (0.00s)
=== RUN   TestGetBoard
--- PASS: TestGetBoard (0.00s)
=== RUN   TestGetBlockedExtractsViolations
--- PASS: TestGetBlockedExtractsViolations (0.00s)
=== RUN   TestGetSliceContext
--- PASS: TestGetSliceContext (0.02s)
=== RUN   TestDeferSliceWritesRuleTwo
--- PASS: TestDeferSliceWritesRuleTwo (0.00s)
=== RUN   TestGetCreditsAbsent
--- PASS: TestGetCreditsAbsent (0.00s)
=== RUN   TestRerunSliceWritesPID
--- PASS: TestRerunSliceWritesPID (0.00s)
=== RUN   TestPatchSliceWritesInstructions
--- PASS: TestPatchSliceWritesInstructions (0.00s)
=== RUN   TestApproveMergeRejectsUnverified
--- PASS: TestApproveMergeRejectsUnverified (0.00s)
=== RUN   TestListReleases
--- PASS: TestListReleases (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/mcp	0.051s
```

### Full suite

```
$ go test ./...
ok  	github.com/swornagent/sworn/cmd/sworn	0.236s
ok  	github.com/swornagent/sworn/internal/adopt	(cached)
ok  	github.com/swornagent/sworn/internal/agent	(cached)
ok  	github.com/swornagent/sworn/internal/bench	(cached)
ok  	github.com/swornagent/sworn/internal/board	(cached)
ok  	github.com/swornagent/sworn/internal/config	(cached)
ok  	github.com/swornagent/sworn/internal/db	(cached)
ok  	github.com/swornagent/sworn/internal/designaudit	(cached)
ok  	github.com/swornagent/sworn/internal/designfit	(cached)
ok  	github.com/swornagent/sworn/internal/ears	(cached)
ok  	github.com/swornagent/sworn/internal/git	(cached)
ok  	github.com/swornagent/sworn/internal/implement	(cached)
ok  	github.com/swornagent/sworn/internal/journey	(cached)
ok  	github.com/swornagent/sworn/internal/mcp	0.054s
ok  	github.com/swornagent/sworn/internal/model	(cached)
ok  	github.com/swornagent/sworn/internal/prompt	(cached)
ok  	github.com/swornagent/sworn/internal/reqvalidate	(cached)
ok  	github.com/swornagent/sworn/internal/reqverify	(cached)
ok  	github.com/swornagent/sworn/internal/rtm	(cached)
ok  	github.com/swornagent/sworn/internal/run	(cached)
ok  	github.com/swornagent/sworn/internal/scheduler	(cached)
ok  	github.com/swornagent/sworn/internal/specquality	(cached)
ok  	github.com/swornagent/sworn/internal/state	(cached)
ok  	github.com/swornagent/sworn/internal/supervisor	(cached)
ok  	github.com/swornagent/sworn/internal/telemetry	(cached)
?   	github.com/swornagent/sworn/internal/verdict	[no test files]
ok  	github.com/swornagent/sworn/internal/verify	(cached)
```

### go vet

```
$ go vet ./...
(clean — no output)
```

### gofmt

```
$ gofmt -l internal/mcp/tools_plan.go internal/mcp/tools_plan_test.go internal/mcp/resources.go internal/mcp/prompts.go
(clean — no output for files in this slice's scope)
```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: binary smoke test via stdio JSON-RPC (transcript below)
- **User gesture**: Configure `sworn mcp` in an AI tool; ask it to "add slice S99-smoke to release 2026-06-19-mcp-test"; observe the AI call `create_slice`; verify `docs/release/2026-06-19-mcp-test/S99-smoke/{spec.md,status.json}` created.

### Smoke test transcript

Built the binary (`make build`), created a temp repo with a release dir, then sent JSON-RPC requests over stdio:

```
$ cd /tmp/mcp-smoke && /path/to/bin/sworn mcp 2>/dev/null <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"create_slice","arguments":{"release":"2026-06-19-mcp-test","slice_id":"S99-smoke","spec_content":"# S99-smoke\n\nSmoke test slice.","track_id":"T1"}}}
{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"sworn://release/2026-06-19-mcp-test/S99-smoke/spec"}}
{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"sworn://release/2026-06-19-mcp-test/S99-smoke/proof"}}
{"jsonrpc":"2.0","id":5,"method":"resources/read","params":{"uri":"sworn://prompts/plan"}}
{"jsonrpc":"2.0","id":6,"method":"prompts/get","params":{"name":"planner"}}
EOF
```

Response (id=2, create_slice):
```json
{"jsonrpc":"2.0","id":2,"result":{"isError":false,"content":[{"type":"text","text":"Created slice S99-smoke under release 2026-06-19-mcp-test.\nPaths:\n- docs/release/2026-06-19-mcp-test/S99-smoke/spec.md\n- docs/release/2026-06-19-mcp-test/S99-smoke/status.json"}]}}
```

Response (id=3, resources/read spec):
```json
{"jsonrpc":"2.0","id":3,"result":{"contents":[{"uri":"sworn://release/2026-06-19-mcp-test/S99-smoke/spec","mimeType":"text/markdown","text":"# S99-smoke\n\nSmoke test slice."}]}}
```

Response (id=4, resources/read absent proof — empty string, no error):
```json
{"jsonrpc":"2.0","id":4,"result":{"contents":[{"uri":"sworn://release/2026-06-19-mcp-test/S99-smoke/proof","mimeType":"text/markdown"}]}}
```

Response (id=5, resources/read prompts/plan — non-empty embedded content):
```json
{"jsonrpc":"2.0","id":5,"result":{"contents":[{"uri":"sworn://prompts/plan","mimeType":"text/markdown","text":"---\ntitle: Planner role prompt\n...(truncated, full planner prompt from internal/prompt/planner.md embed)..."}]}}
```

Response (id=6, prompts/get planner — non-empty message):
```json
{"jsonrpc":"2.0","id":6,"result":{"description":"Baton planner role prompt","messages":[{"role":"user","content":{"type":"text","text":"---\ntitle: Planner role prompt\n..."}}]}}
```

Created files on disk:
```
$ ls /tmp/mcp-smoke/docs/release/2026-06-19-mcp-test/S99-smoke/
spec.md  status.json

$ cat /tmp/mcp-smoke/docs/release/2026-06-19-mcp-test/S99-smoke/status.json
{
  "$schema": "https://example.com/schemas/baton/slice-status-v1.json",
  "slice_id": "S99-smoke",
  "release": "2026-06-19-mcp-test",
  "track": "T1",
  "state": "planned",
  "owner": "human",
  "last_updated_by": "create_slice",
  "last_updated_at": "2026-06-21T12:08:42Z",
  "spec_path": "docs/release/2026-06-19-mcp-test/S99-smoke/spec.md",
  "proof_path": "docs/release/2026-06-19-mcp-test/S99-smoke/proof.md",
  "journal_path": "docs/release/2026-06-19-mcp-test/S99-smoke/journal.md",
  "verification": {
    "result": "pending"
  },
  "validation": {
    "human_ratified": false
  }
}
```

## Delivered

- **Internal `CreateRelease`** — evidence: `internal/mcp/tools_plan.go` `CreateRelease()` function (lines 344-527); `TestCreateRelease` in `internal/mcp/tools_plan_test.go` asserts directory structure, intake.md with goal, index.md from template, screenshots/.gitkeep, activity.md, .gitattributes
- **`create_slice` tool** — evidence: `internal/mcp/tools_plan.go` `RegisterPlanTools` registers `create_slice` (line 19); `TestCreateSlice` asserts spec.md content + status.json with state=planned and track=T1; `TestCreateSliceDuplicate` asserts error on second call
- **`set_track` tool** — evidence: `internal/mcp/tools_plan.go` registers `set_track` (line 90); `TestSetTrackValidation` asserts error for non-existent slice_id; `TestSetTrackUpdates` asserts index.md frontmatter updated; `TestSetTrackColon` asserts colon-space slice IDs produce valid output
- **`update_intake` tool** — evidence: `internal/mcp/tools_plan.go` registers `update_intake` (line 265); `TestUpdateIntakeAppends` asserts both contents present and order preserved; `TestUpdateIntakeCreatesSection` asserts new heading created at EOF
- **`resources/read sworn://prompts/plan`** — evidence: `internal/mcp/resources.go` registers handler (line 16); `TestResourceReadPrompt` asserts non-empty content from embed; smoke test confirms non-empty planner prompt returned
- **`resources/read sworn://baton/version`** — evidence: `internal/mcp/resources.go` (line 44); `TestResourceReadBatonVersion` asserts non-empty parseable version string
- **`resources/read sworn://release/{name}/board`** — evidence: `internal/mcp/resources.go` dynamic handler (line 53); `TestResourceReadReleaseBoard` asserts board content returned
- **`resources/read sworn://release/{name}/{slice}/proof` for absent proof** — evidence: `internal/mcp/resources.go` (line 98) returns empty string, no error; `TestResourceReadProofAbsent` asserts empty string
- **`docs/mcp-setup.md`** — evidence: file exists at `docs/mcp-setup.md` with Claude Code JSON config block, Codex, Cursor, Windsurf, Gemini CLI configs, tool/resource/prompt listings, example workflow
- **`go test ./internal/mcp/...` covers all tools, resources, prompts** — evidence: 20 new tests in `internal/mcp/tools_plan_test.go` covering createRelease, create_slice, set_track, update_intake, resource reads (prompts, version, board, proof-absent, spec, track-mode, intake), prompts/get (planner, implementer, verifier), prompts/list enumeration
- **Server dispatch wiring** — evidence: `internal/mcp/server.go` `RegisterResource`/`RegisterPrompt` methods (lines 99-110), `resources/read` and `prompts/get` in `buildMethodHandlers()` (lines 205-208), `handlePromptsList` enumerates registered prompts (line 408)
- **`cmd/sworn/mcp.go` wiring** — evidence: `mcp.RegisterPlanTools(server, ".")` at line 35, `mcp.RegisterResources(server, ".")` at line 36, `mcp.RegisterPrompts(server)` at line 37
- **`internal/prompt/prompt.go` embed extended** — evidence: `//go:embed` directive at line 14 includes `baton/track-mode.md`; `TrackMode()` accessor at line 82
- **`internal/prompt/baton/track-mode.md`** — evidence: file exists, vendored from `~/.claude/baton/track-mode.md`
- **Embed-absent error text** — evidence: `internal/mcp/resources.go` lines 19, 25, 31, 39, 47 all return spec-prescribed format: `"sworn://<uri>: embedded prompt not found — this is a binary build error; please reinstall sworn."`

## Not delivered

- **`sworn://baton/rules` MCP resource** — DEFERRED to S21-canonical-baton.
  - **Why**: its source, `internal/prompt/baton/rules.md`, is created by S21-canonical-baton (T3); no consolidated Baton-protocol file exists yet.
  - **Tracking**: S21-canonical-baton (T3-commercial track).
  - **Acknowledged**: Coach, 2026-06-21 (`decline.md`).

## Divergence from plan

- The checkpoint commit (`c143570`) contained the production code from a prior session that was interrupted. This session added the test suite (`internal/mcp/tools_plan_test.go`), `docs/mcp-setup.md`, and the proof bundle — completing the slice against the approved design.
- `CreateRelease` is implemented as an exported function (not a registered MCP tool) per the spec note that S20's `plan_release` will call it internally.