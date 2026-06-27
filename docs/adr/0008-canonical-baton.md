# ADR 0008: Canonical Baton Protocol in Binary

- **Status**: accepted
- **Date**: 2026-07-05
- **Release**: 2026-06-19-safe-parallelism (S21-canonical-baton)

## Decision

The `sworn` binary is the single source of truth for the Baton protocol. All
role prompts and protocol documentation are embedded in the binary via
`go:embed` under `internal/prompt/baton/`. The MCP server serves them as
`sworn://baton/*` and `sworn://prompts/*` resources. `sworn init` no longer
copies Baton content into repos; repos contain only a minimal MCP-pointer
`AGENTS.md`.

## Rationale

- **Eliminates per-repo drift.** Prior to this ADR, `sworn init` copied Baton
  rules into `docs/baton/` and spliced a seven-rule fragment into
  `AGENTS.md`/`CLAUDE.md`. Each repo had its own frozen copy. Protocol
  improvements required re-running `sworn init` on every repo.
- **One canonical version.** Protocol improvements roll out to all users on
  binary update. Customers can report issues against a specific binary version.
- **Support cost is bounded.** One canonical version, not N per-repo forks. No
  "which Baton version is this repo on?" debugging.

## Supersedes

This ADR supersedes the `adopt.Materialise` / `adopt.SpliceAgents` approach
from R1. The `adopt` package is retained for `sworn doctor` legacy support
(`BatonDocsFS()`, `BatonSectionHeading`, `AgentsFragment()`) but `sworn init`
no longer calls it.

## Consequences

- `docs/baton/` in existing repos is now legacy; `sworn doctor` warns and
  advises removal
- `~/.claude/baton/` on developer machines continues to work for local
  slash-command harness; it is **NOT** deprecated, but it is no longer
  installed by `sworn init`
- The slash-command harness (`/implement-slice` etc.) reads from
  `~/.claude/baton/` for now; a future release will migrate them to read via
  `sworn://prompts/*`
- After `sworn init` on a new repo, the only Baton-related artefact is
  `AGENTS.md` pointing at the MCP server

## Deferrals

- **User prompt overrides / project-level Baton customisation**: Deferred
  post-launch. Why: opening overrides before the canonical protocol is stable
  creates N support surfaces before we can iterate. Tracking: post-launch
  feature. Acknowledged: 2026-06-20 planning session (S21 spec).
- **Migration of slash-command harness to read from sworn MCP**: Post-launch.
  Tracking: post-launch feature.