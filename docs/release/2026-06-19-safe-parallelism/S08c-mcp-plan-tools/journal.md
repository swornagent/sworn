# Journal — S08c-mcp-plan-tools

## 2026-06-21 — design revised after Coach decline

Coach declined the first design ack (`decline.md`). Revised `design.md`, `spec.md`, and
`status.json` to clear all 6 Captain pins; state stays `design_review` for re-review.

- **Pin 1** (server dispatch gap): added `internal/mcp/server.go` to §3 + planned_files —
  `RegisterResource`/`RegisterPrompt` + `resources/read`/`prompts/get` dispatch + `handlePromptsList`
  enumeration. Confirmed against live `server.go`: only `tools/*`, `resources/list`, `prompts/list`
  exist today.
- **Pin 2** (`sworn://baton/rules` source): deferred to S21-canonical-baton (Rule 2; design §4,
  spec annotation, `open_deferrals`). No hard T4→T3 dep — touchpoint note only.
- **Pin 3** (`internal/prompt/baton/` missing): create dir, vendor `~/.claude/baton/track-mode.md`,
  extend the `prompt.go` `go:embed`. `sworn://baton/version` served from existing
  `internal/prompt/VERSION.txt` (no duplicate). Both confirmed present.
- **Pin 4** (yaml.v3 dep): resolved without a dep — `set_track` uses stdlib for the narrow
  frontmatter (§2.2). Flagged overridable to yaml.v3+ADR at Coach's discretion (§6).
- **Pin 5** (`cmd/sworn/mcp.go` missing): added to §3 + planned_files. Wiring point confirmed —
  `cmd/sworn/mcp.go:33` `// Planning tools (S08c) register here`, mirrors `RegisterOpsTools`.
- **Pin 6** (spec reachability references unexposed `create_release`): §5 + spec Required-tests
  rewritten to a `create_slice` demo. Coach-authorized via decline.md.

Flags: dropped "MCP SDK" language (server is bespoke); `prompts/list` will enumerate registered
prompts post-`handlePromptsList` update.

Next: Captain re-review of the revised design; Coach issues a fresh ack/decline.
