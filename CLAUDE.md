# CLAUDE.md

Claude Code — and any other agent or model — should use **[AGENTS.md](AGENTS.md)**
as the canonical guidance for this repository. One source of truth; do not
duplicate guidance here (a second detailed copy drifts).

The rules most worth repeating as a safety net:

1. **Fail closed.** Exit `0` only on PASS. Single Go binary, **zero runtime
   dependencies** — stdlib only; the model client uses `net/http` + `encoding/json`,
   **not** a provider SDK like `github.com/openai/go-openai`. New deps require an ADR.
2. **This repo is public-safe.** No business / pricing / competitive / strategy
   content, and no references to private/internal repositories. Strategy lives
   privately, elsewhere.

Everything else — layout, build/test, the slice workflow, conventions — is in
[AGENTS.md](AGENTS.md).
