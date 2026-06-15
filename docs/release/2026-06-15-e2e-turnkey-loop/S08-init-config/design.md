# Design TL;DR — S08-init-config

## §1. User-visible change

`sworn init` is a new subcommand that bootstraps SwornAgent in a repo: it writes a
`~/.config/sworn/config.json` (or `$SWORN_CONFIG_PATH`) with sensible model
defaults, prompts for one API key, and adopts the Baton protocol by vendoring
`docs/baton/` (rules + VERSION) into the target repo and splicing the seven-rule
fragment into `AGENTS.md`. After init, `sworn verify` can resolve a verifier model
from config (env > file > default) without the user passing `--verifier-model`
every time. The adoption step ensures other contributors' agents see the Baton
rules even if they don't have a global `~/.claude/CLAUDE.md`.

## §2. Design decisions not in spec (max 5)

1. **Config format: JSON** — stdlib `encoding/json`, no new deps. TOML/YAML would
   require third-party parsers, violating the "zero runtime dependencies" constraint.
   Rationale: same constraint that drove the OAI client to stdlib `net/http`.

2. **Config path: `$SWORN_CONFIG_PATH` with fallback `$HOME/.config/sworn/config.json`**
   (XDG-compatible on Linux, `$HOME/Library/Application Support/sworn/config.json` on
   macOS). `$SWORN_HOME` also respected as a config-directory override. Rationale:
   standard Go/XGD patterns, no new deps, env-var overridable for CI.

3. **API key storage: interactive prompt or `--api-key` flag during init; written to
   config file with `0600` permissions.** The key is also settable via env var
   (`SWORN_<PROVIDER>_API_KEY`) which takes precedence at load time. Rationale: env
   vars are the secure path; the config file is the convenience path. Tight file
   permissions + a `sworn init` warning about key-in-file risk satisfy the "never log
   keys" constraint.

4. **Adoption splice: idempotent section-level edit.** The implementer scans
   `AGENTS.md` for the marker heading `## Engineering Process — Baton`. If absent, it
   appends the section (rules + protocol version). If present, it replaces the section
   body but preserves any user-authored content before/after. On re-run with an
   identical section, it is a no-op (byte-level compare). Rationale: the spec requires
   idempotent re-run; section-level replacement is the simplest correct approach.

5. **Adoption materialises from embedded `internal/prompt/` VERSION.txt**, not from a
   separate `go:embed` of `docs/baton/`. The Baton protocol version recorded in the
   vendored `docs/baton/VERSION` will be read from the embedded `prompt.BatonVersion()`
   string already available at `sworn version`. Rationale: single source of truth for
   the protocol version; avoids embedding two copies.

## §3. Files I'll touch grouped by purpose

- **Config types + loading**: `internal/config/config.go` (new) — `Config` struct,
  `Load()` with env > file > default merge, `ModelConfig` per-role model selections.
  This is the core new infrastructure that `sworn verify` and future `sworn run` will
  consume.

- **Init scaffolding**: `internal/config/init.go` (new) — `Scaffold(path, key)` writes
  the default config file with tight permissions. Interactive prompting lives in the
  CLI layer.

- **Baton adoption**: `internal/adopt/adopt.go` (new) — `Materialise(repoRoot string)`
  writes `docs/baton/` rules + VERSION from embedded content; `SpliceAgents(repoRoot)`
  idempotently inserts the Baton section into `AGENTS.md`.

- **CLI entry point**: `cmd/sworn/init.go` (new) — `cmdInit` function: flag parsing,
  interactive key prompt, calls config.Scaffold + adopt.Materialise + adopt.SpliceAgents.

- **Dispatch**: `cmd/sworn/main.go` (edit) — add `case "init":` to the switch, routing
  to `cmdInit(os.Args[2:])`.

## §4. Things I'm NOT doing

- Enterprise config (SSO, tenancy, sovereignty — post-MVP per spec).
- Config migration or upgrade paths.
- `sworn run` integration — S07 owns wiring config into the full loop.
- Provider API key validation (calling the provider to verify the key works) — the
  error surfaces naturally on first `sworn verify`.
- Splicing Baton into `CLAUDE.md` or any file other than `AGENTS.md`.

## §5. Reachability plan

- **Unit tests**: `go test ./internal/config/ ./internal/adopt/` — config merge
  precedence, idempotent init, missing-key error path, adoption splice edge cases
  (no AGENTS.md, existing section, duplicate run).
- **CLI smoke**: `go run ./cmd/sworn init --api-key test-key-123` in a temp repo,
  verify `~/.config/sworn/config.json` written, `docs/baton/` materialised,
  `AGENTS.md` section present.

## §6. Open questions for the Coach

None.