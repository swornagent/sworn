# Proof bundle — fix init-agents-embed (2026-07-02)

## Scope

Make cold-start `sworn init` work in any repo by embedding the scaffolding
templates (AGENTS.md MCP-pointer, consideration catalog, decision registry) in
the binary instead of reading them from the TARGET repo's never-present
`docs/templates/` (sworn#28). Closes the adversarially-verified audit finding
`init-agents-template-unreachable`. Refs swornagent/sworn#51.

## Files changed

Output of `git diff --name-only 632d4f3` scoped to this fix (the internal/mcp
and first-proof entries belong to the prior commit on this branch):

```
cmd/sworn/induction.go
cmd/sworn/init.go
cmd/sworn/init_design_system_test.go
cmd/sworn/init_test.go
internal/templates/agents.md          (moved from docs/templates/agents.md)
internal/templates/considerations.md  (moved from docs/templates/considerations.md)
internal/templates/decisions.md       (moved from docs/templates/decisions.md)
internal/templates/templates.go       (new — go:embed package)
internal/templates/templates_test.go  (new)
```

## Test results

RED first (Rule 1 — through the `cmdInit` dispatch integration point): new
`TestInitColdStartCreatesAgentsMD` (empty temp dir, no seeded templates)
failed before the fix:

```
sworn init: read AGENTS.md template: open /tmp/.../docs/templates/agents.md: no such file or directory
    init_test.go:340: cmdInit --yes exited 1, want 0 (cold start must not depend on repo-local templates)
--- FAIL: TestInitColdStartCreatesAgentsMD (0.00s)
```

After the fix:

```
$ go test ./cmd/... ./internal/templates/... -timeout 300s
ok  	github.com/swornagent/sworn/cmd/sworn	38.848s
ok  	github.com/swornagent/sworn/internal/templates	0.003s

$ go test ./cmd/sworn/ -run 'TestInitColdStartCreatesAgentsMD|TestInitCreatesAgentsMD|TestInitCreatesBothTemplates' -v
--- PASS: TestInitCreatesBothTemplates (0.00s)
--- PASS: TestInitCreatesAgentsMD (0.00s)
--- PASS: TestInitColdStartCreatesAgentsMD (0.00s)
```

`go vet ./cmd/sworn/ ./internal/templates/` clean; `gofmt -l` clean on all
touched files. The `setupTemplates` fixture seeding (which masked the bug) was
removed from init tests — every init test now runs cold-start.

## Reachability artefact

Live cold-start run in a fresh empty scratch git repo, binary built with
`go build -buildvcs=false -o bin/sworn ./cmd/sworn`:

Before (base 632d4f3 behaviour, reproduced live this session):

```
$ git init inittest-red && cd inittest-red && sworn init --yes
sworn init: read AGENTS.md template: open .../inittest-red/docs/templates/agents.md: no such file or directory
EXIT=1        # ls: only .git and config.json — no AGENTS.md
```

After:

```
$ git init inittest-green && cd inittest-green && SWORN_CONFIG_PATH=$PWD/config.json sworn init --yes
  created  AGENTS.md (MCP-pointer template)
  created  docs/considerations.md
  created  docs/decisions.md
Done. Connect your AI to sworn mcp to get the Baton protocol and role prompts. ...
EXIT=0
$ grep -n "sworn://baton/rules" AGENTS.md
26:| Full Baton protocol | `sworn://baton/rules` |
```

The advertised `sworn://baton/rules` URI resolves against the same binary
(see companion proof bundle `2026-07-02-fix-mcp-rules-resource-proof.md`), so
the init → MCP journey is closed end-to-end.

## Delivered

- New `internal/templates` package embedding agents.md / considerations.md /
  decisions.md via `go:embed` (single source of truth — files MOVED from
  `docs/templates/`, not copied, so no drift). Evidence: `internal/templates/
  templates.go`, `TestEmbeddedTemplatesNonEmpty`, `TestAgentsMDAdvertisesBatonRules`.
- `createAgentsMD` and `materialiseCatalog` (cmd/sworn/init.go) write from the
  embedded templates; cold-start `sworn init --yes` exits 0 and produces
  AGENTS.md + both catalog files. Evidence: `TestInitColdStartCreatesAgentsMD`
  + live run above.
- `initializeCatalog` (cmd/sworn/induction.go) keeps the repo-local
  `docs/templates/considerations.md` as an explicit override (its tests feed
  custom fixtures through that path) and falls back to the embedded template,
  so cold-start `sworn induction` no longer hard-fails either.
- Removed the test fixture seeding (`setupTemplates`) that masked sworn#28.

## Not delivered

- Closing GitHub issue sworn#28 itself (why: this session must not push or
  touch the remote; tracking: sworn#28 remains open, referenced in the commit
  body for the audit reconciliation step under swornagent/sworn#51;
  acknowledgement: recorded here).

## Divergence from plan

- The finding scoped createAgentsMD (init.go:267-279); the fix also covers
  `materialiseCatalog` (same `sworn init --yes` path — cold-start init cannot
  exit 0 without it) and the `initializeCatalog` fallback in induction.go
  (the template files moved out of docs/templates/, so leaving it would have
  regressed induction in this repo). Both are the finding's own noted sibling
  instances of the same defect class, not new scope.
