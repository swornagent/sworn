# Proof bundle — fix mcp-rules-resource (2026-07-02)

## Scope

Register the `sworn://baton/rules` MCP resource (embedded 11-rule Baton
content, previously built but unreachable) and make `resources/list` enumerate
the actually-registered resources instead of returning a hardcoded empty array.
Closes two adversarially-verified audit findings (`baton-rules-mcp-resource-dead`,
`mcp-resources-list-empty`); resolves the S08c→S21 orphaned deferral and the
stale S08a/S08c `resources/list` deferral. Refs swornagent/sworn#51.

## Files changed

Output of `git diff --name-only 632d4f3` (plus untracked test file):

```
internal/mcp/resources.go
internal/mcp/server.go
internal/mcp/resources_test.go   (new)
```

Constraint honoured: `internal/mcp/catalog.go`, `context.go`, `tools_ops.go`,
`tools_plan.go` (owned by in-flight T3-mcp track) untouched.

## Test results

RED first (Rule 1 — through the JSON-RPC dispatch integration point): before
the fix the new tests failed to build against the hardcoded
`resources: []json.RawMessage{}` shape:

```
internal/mcp/resources_test.go:62:10: res.URI undefined (type "encoding/json".RawMessage has no field or method URI)
FAIL	github.com/swornagent/sworn/internal/mcp [build failed]
```

After the fix:

```
$ go test ./internal/mcp/ -run 'TestBatonRulesResourceRead|TestResourcesListEnumeratesRegistered|TestResourcesList$' -v -timeout 120s
--- PASS: TestBatonRulesResourceRead (0.01s)
--- PASS: TestResourcesListEnumeratesRegistered (0.00s)
--- PASS: TestResourcesList (0.00s)
ok  	github.com/swornagent/sworn/internal/mcp	0.032s

$ go test ./internal/mcp/ -timeout 120s
ok  	github.com/swornagent/sworn/internal/mcp	0.099s

$ go test ./cmd/... -timeout 120s
ok  	github.com/swornagent/sworn/cmd/sworn	49.325s
```

`go vet ./internal/mcp/` clean; `gofmt -l` clean.

## Reachability artefact

Live JSON-RPC session (initialize → resources/list → resources/read) piped
through the freshly built binary (`go build -buildvcs=false -o bin/sworn
./cmd/sworn`):

```
$ printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"repro","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"resources/list","params":{}}' \
  '{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"sworn://baton/rules"}}' \
  | ./bin/sworn mcp

LIST: [{"uri": "sworn://baton/rules", "name": "baton/rules", "mimeType": "text/markdown"},
       {"uri": "sworn://baton/track-mode", ...}, {"uri": "sworn://baton/version", ...},
       {"uri": "sworn://prompts/implement", ...}, {"uri": "sworn://prompts/plan", ...},
       {"uri": "sworn://prompts/verify", ...}]
READ OK, rule headings: 12 len: 86434
```

Before the fix the same session returned `resources: []` for id=2 and
`{"error":{"code":-32000,"message":"resource \"sworn://baton/rules\" not found"}}`
for id=3 (finding evidence). The AGENTS.md-template-advertised URI now resolves.

## Delivered

- `sworn://baton/rules` registered, serving `prompt.Baton("rules.md")` (the
  embedded full Baton rules doc) — `internal/mcp/resources.go`, proven by
  `TestBatonRulesResourceRead` + live session above.
- `resources/list` enumerates registered resources (sorted, with uri/name/
  mimeType), excluding dynamic trailing-slash prefix patterns like
  `sworn://release/` which are not readable at the bare prefix —
  `internal/mcp/server.go` `handleResourcesList`, proven by
  `TestResourcesListEnumeratesRegistered` + live session.
- Mime-type logic factored into `resourceMimeType` so list and read agree.

## Not delivered

- The S22-deferred `sworn doctor` pointer check for `sworn://baton/rules`
  (why: doctor is a separate surface and out of this fix's minimal scope;
  tracking: remains on the audit punch list under swornagent/sworn#51;
  acknowledgement: recorded here and in the audit deliverable).
- MCP `resources/templates/list` support for the `sworn://release/` dynamic
  family (why: new protocol surface, materially larger than the finding;
  tracking: audit punch list under swornagent/sworn#51; acknowledgement:
  recorded here).

## Divergence from plan

None. Fix landed exactly as scoped in the two findings; the in-flight T3-mcp
files were not touched.
