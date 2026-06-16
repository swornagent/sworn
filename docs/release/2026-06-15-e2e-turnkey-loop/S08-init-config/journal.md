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

### Skeptic panel

Skipped — Agent/Workflow tool not available in this harness. Noted per implementer.md
Step 5 escalation clause.
### AC1 narrowing (Coach Pin 1)

AC1 ("After `sworn init` + one key, `sworn run` works with defaults") is cross-slice.
S08 delivers the config infra + init subcommand. AC1 closes when S07 (`sworn run`)
lands and wires config into the full loop.

## Verifier verdicts received

### 2026-06-16T10:00:00Z — PASS

Fresh-context verifier session. No prior implementer context loaded.

**Gate 1 (Entry point wiring):** PASS — `sworn init` wired in `main.go` `case "init"`. Binary builds and live smoke test confirms exit 0 with correct output.

**Gate 2 (Touchpoints match diff):** PASS — all implementation files fall within planned touchpoints (`internal/config/`, `internal/adopt/`, `cmd/sworn/init.go`, `cmd/sworn/main.go`). Release artefact files (status.json, proof.md, journal.md) are expected noise.

**Gate 3 (Required tests pass):** PASS — ran `go test ./internal/config/ ./internal/adopt/ -v` independently. All 14 tests pass: config precedence subtests (flag/env/config fallback), idempotent scaffold, missing-key error, materialise writes docs, all splice variants (no file, existing no section, existing section replace, idempotent, idempotent when current).

**Gate 4 (Reachability artefact):** PASS — live smoke test executed during verification: `sworn init` creates config + vendors 7 rule files + creates AGENTS.md (exit 0); idempotent re-run exits 0 with "already has current Baton rules section"; missing-key path exits 2 with actionable error.

**Gate 5 (No silent deferrals):** PASS — no TODO/FIXME/deferred/placeholder code markers in implementation files. Embedded Baton rule text in `adopt.go` is a string constant (not a code deferral). The `// later sworn run` comment in `config.go` is a package-level documentation comment, not deferred code. AC1 partial deferral has all three Rule 2 elements inline (why: S07 dependency; tracking: S07-run-loop slice; acknowledgement: Captain, 2026-06-16).

**Gate 6 (Claimed scope matches implemented scope):** PASS — all Delivered items verified against live code. AC1 partial delivery correctly documented per Rule 2.

Verdict: **PASS**. Slice transitions to `verified`.