# Journal — S06b-sworn-proxy-credits

## Session: re-entry (in_progress → implemented)

### Prior session context

A prior implementer session wrote the core implementation (proxy.go, FetchCredits,
model.FromEnv proxy routing, cmd/sworn/account.go buy subcommand) and committed
a WIP checkpoint ("tests RED"). The design was already Coach-approved via
`approved-ack.md` with three pins (A: integer credits, B: credential-trust
boundary, C: 402 payment failure path).

### Fixes applied in this session

1. **Test credentials path bug**: The S06b proxy routing tests in
   `internal/model/oai_test.go` wrote credentials to
   `$XDG_CONFIG_HOME/credentials.json`, but `configDir()` appends `/sworn`,
   so the actual path is `$XDG_CONFIG_HOME/sworn/credentials.json`. Fixed by
   introducing `writeTestCreds()` helper that creates the `sworn/` subdirectory.
   All four failing tests now pass:
   - `TestFromEnvUsesProxy`
   - `TestFromEnvProxyDefaultHost` (pin B)
   - `TestFromEnvProxyOverrideWarns` (pin B)
   - `TestFromEnvInsufficientCredits` (pin C)

2. **TestFromEnv table test isolation**: Added `SWORN_DIRECT` and
   `SWORN_PROXY_URL` to the env-var clear list, and set `XDG_CONFIG_HOME` to
   a temp dir, so real machine credentials don't interfere with the
   "missing key" test case.

3. **Import block corruption**: The prior session's WIP checkpoint had
   tab/newline corruption in the import block of `oai_test.go` (missing
   newlines between import lines). Repaired.

4. **Non-blocking FetchCredits in `sworn run`**: Added a goroutine in
   `run.Run()` that calls `account.FetchCredits` with a 3s context timeout
   at startup. It proceeds regardless of outcome; errors are logged to stderr
   as warnings. This satisfies the spec AC: "sworn run startup calls it
   non-blocking and proceeds even if it times out."

5. **`docs/api-contract.md` stub**: Created per spec Risks section. Documents
   the proxy request/response format, the 402 insufficient-credits response,
   the account credits endpoint, and the integer credit unit (pin A).

6. **gofmt**: Ran `gofmt -w .` to fix formatting across all files (the prior
   session left many files unformatted). Only slice-relevant files were staged;
   gofmt-only changes to other tracks' files were reverted.

### Design decisions

- `writeTestCreds` helper centralises the credentials file creation pattern
  for all S06b proxy routing tests, ensuring the `sworn/` subdirectory is
  created correctly.
- The FetchCredits goroutine in `run.Run()` uses `context.Background()` (not
  the run's ctx) because the run ctx may be cancelled before the 3s timeout,
  and we want the credit fetch to complete independently.

### Track collisions (planner matrix gaps)

The following files are touched by this slice but are not listed in the
touchpoint matrix under T3-commercial. They are T1-owned (T1 is merged,
so no in-flight collision). The spec explicitly requires these changes;
the planned_files list in status.json is incomplete.

- `internal/model/config.go` (spec says `internal/model/client.go` — file
  was renamed or planner got the name wrong; `FromEnv` lives in `config.go`).
  Proxy routing logic added here per spec "In scope".
- `internal/model/oai.go` — 402 Payment Required handling (pin C) added to
  both `Verify()` and `Chat()` methods. T1-owned, merged; additive change.
- `internal/model/oai_test.go` — S06b proxy routing tests. T1-owned, merged.
- `internal/run/run.go` — non-blocking FetchCredits call at startup per spec
  "In scope" ("sworn run startup"). T1-owned, merged; additive change.

### Deferrals

None. All acceptance checks are addressed.
### Pre-verification skeptic panel

skeptic_panel: skipped — runtime does not support subagent dispatch (no Agent
tool available in this session). The panel is an accelerant, not a gate; the
real verifier (Rule 7) is the backstop regardless.

## Verifier verdicts received

### Verdict: PASS — 2026-06-21T13:08:50Z

Fresh-context verifier session (Rule 7). All six gates passed:

1. **User-reachable outcome exists**: `model.FromEnv` proxy routing in `config.go`, `sworn account`/`buy` wired in `main.go:111-112`.
2. **Planned touchpoints match actual changed files**: Divergences (`config.go` not `client.go`; `oai.go`, `oai_test.go`, `run/run.go`, `api-contract.md`) all explained in proof.md and grounded in spec requirements.
3. **Required tests exist and exercise the integration point**: 9 spec-named test functions all present, all pass (`go test ./internal/account/...` + `go test ./internal/model/...` = 100% PASS).
4. **Reachability artefact**: `TestFromEnvUsesProxy` exercises full path: credentials → `account.Load` → `account.Endpoint` → `FromEnv` → `OAI{BaseURL: proxyURL}` → `Verify()` → HTTP request to mock proxy.
5. **No silent deferrals or placeholder logic**: Zero TODO/FIXME/deferred/placeholder in changed production code.
6. **Claimed scope matches implemented scope**: Every Delivered item in proof.md verified against source code; all acceptance checks covered by passing tests.