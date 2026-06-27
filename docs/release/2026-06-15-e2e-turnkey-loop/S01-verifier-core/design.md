# Design TL;DR — S01-verifier-core

## §1. User-visible change

A developer runs `sworn verify --spec <path> --diff <path>` and gets a JSON
verdict printed to stdout. The process exits 0 only on PASS; FAIL exits 1 and
BLOCKED exits 2. The BLOCKED state covers three fail-closed paths: empty or
missing inputs, an unconfigured verifier model, and an unparseable model reply.
This is the fail-closed core — the CI gate that makes SwornAgent's adversarial
verification enforceable.
## §2. Design decisions not in spec (max 5)

1. **PASS/FAIL/BLOCKED as typed Go constants, not raw strings** — the `verdict`
   package defines `Verdict` as a string type with three named constants. This
   prevents typos (e.g. `"PAS"`) from silently becoming false BLOCKEDs elsewhere.
2. **Unconfigured model is a sentinel type (`Unconfigured{}`), not a nil check
   on an interface** — the `model.Verifier` interface lets callers pass any
   implementation or leave it nil. When nil, `verify.Run` substitutes
   `Unconfigured{}`, which returns `ErrNotConfigured`. This keeps the fail-closed
   default in one place and avoids nil-deref panics.
3. **Verdict parse is prefix-based, case-insensitive** — `parseVerdict` matches
   on `strings.HasPrefix(upper, "PASS")` etc. This is conservative: the model
   might append rationale after the verdict token, but a reply like `"looks good"`
   (no prefix match) falls through to BLOCKED.
4. **Exit codes: FAIL=1, BLOCKED=2, anything-else=2** — `ExitCode()` returns 2
   for any unknown Verdict value, not just the named `Blocked` constant. This
   is fail-closed by construction: a future Verdict value added without updating
   the switch still exits non-zero.
5. **`--proof` flag is wired but optional** — the spec says the proof bundle is
   optional in S01. The flag exists so S05+ can pass it without changing the CLI
   surface; `verify.Run` silently skips proof if empty.

## §3. Files I'll touch grouped by purpose

- **Verdict contract**: `internal/verdict/verdict.go` — the PASS/FAIL/BLOCKED
  type, Result struct, ExitCode mapping. This is the single source of truth for
  the fail-closed contract; every other package imports it.
- **Model abstraction**: `internal/model/client.go` — Verifier interface +
  Unconfigured sentinel. Provider-neutral; S02 wires the real OpenAI-compatible
  client behind this interface.
- **Verification orchestration**: `internal/verify/verify.go` — the
  deterministic first-pass (spec/diff non-empty gate) → model dispatch →
  conservative verdict parse. `internal/verify/verify_test.go` — table-driven
  tests for all four AC paths.
- **CLI entry point**: `cmd/sworn/main.go` — subcommand dispatch with `verify`
  wired. Kept minimal; each subcommand gets its own file in later slices.

## §4. Things I'm NOT doing

- Real model dispatch (S02) — `Unconfigured{}` is the stub.
- Enriched first-pass (S03) — only spec/diff emptiness check.
- Embedded prompts (S04) — the system prompt is a hardcoded const.
- File-system write-back, git ops, state management (S05).

## §5. Reachability plan

- **Artefact**: `go test ./internal/verify/` + manual binary run:
  `sworn verify --spec <(echo "spec") --diff <(echo "diff")` → JSON BLOCKED
  (unconfigured model), exit 2. Then with a fake model → PASS, exit 0.
- **E2E spec**: not applicable (CLI, no Playwright).

## §6. Open questions for the Coach