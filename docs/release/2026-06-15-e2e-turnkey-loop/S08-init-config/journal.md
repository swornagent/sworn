# Journal — S08-init-config

## 2026-06-16 — Implementation session

### State transition: design_review → in_progress → implemented

Entered with Coach-approved design (PROCEED verdict, 7 pins).

### Decisions

1. **Config format: JSON.** Stdlib `encoding/json`. No new deps.
2. **Config idempotency (Coach Pin 2):** Skip-with-message when config.json exists,
   `--force` flag to overwrite. Messages: "config file already exists at <path> (use --force to overwrite)."
3. **Missing-key error UX (Coach Pin 3):** `sworn verify` prints actionable error:
   "verifier model not configured — run 'sworn init' to scaffold a config file (<path>) or set $SWORN_VERIFIER_MODEL".
4. **Config location docs (Coach Pin 4):** Added to `sworn help` usage text with full precedence chain.
5. **Missing-key smoke test (Coach Pin 5):** `TestResolveVerifierModelMissingKey` asserts both
   "sworn init" mention and "SWORN_VERIFIER_MODEL" mention in the error.
6. **AGENTS.md fragment source (Coach Pin 6):** Hardcoded as Go constant `batonAGENTSFragment`
   in `internal/adopt/adopt.go`. Rules are stable text; change only at protocol re-vendors.
7. **main.go merge convention (Coach Pin 7):** "init" placed alphabetically before "verify"
   in the switch statement.

### Trade-offs

- **API key stored in config file vs env-only:** The config file writes with mode 0600 and
  `sworn init` warns "store it in env var SWORN_OPENAI_API_KEY for production use."
  The env var always takes precedence at load time. This balances turnkey UX (one command)
  against security (env vars are the secure path).
- **Materialise overwrites docs/baton/ on every `sworn init`:** This ensures the vendored
  protocol stays current with the binary. Existing files are replaced byte-for-byte.
- **SpliceAgents normalises trailing newlines for idempotency comparison:** Files may gain
  an extra trailing `\n` from editor round-trips; the comparison strips these before
  determining no-op status.

### Dark-code false positives

The release-verify.sh flagged two dark-code markers:
- `internal/adopt/adopt.go:59` — "deferred items" in the embedded Baton rule text
  (documentation, not a code deferral).
- `internal/config/config.go:25` — "placeholder until the S10 benchmark picks"
  (tracked in the release board, not a silent deferral).

Both are documented text, not code-level deferrals.

### AC1 narrowing (Coach Pin 1)

AC1 ("After `sworn init` + one key, `sworn run` works with defaults") is cross-slice.
S08 delivers the config infra + init subcommand. AC1 closes when S07 (`sworn run`)
lands and wires config into the full loop.