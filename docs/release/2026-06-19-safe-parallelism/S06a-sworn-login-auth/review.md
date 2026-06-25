# Captain review — S06a-sworn-login-auth
Date: 2026-06-21
Design commit: b2c9f90d2036051dd03240568cbcb73516042a76

## Pins

1. **[mechanical] §3 / AC4 — `os.Remove` no-op for missing credentials file**
   What I observed: Design §3 says `sworn logout` "calls `os.Remove` on credentials.json, prints 'Logged out'." `os.Remove` returns `*os.PathError` when the file is absent. Spec AC4 requires "running `sworn logout` with no credentials file is a no-op (no error)." If the PathError is not suppressed, the CLI will exit with an error and fail AC4.
   What to ask the implementer: After `os.Remove`, check `if err != nil && !errors.Is(err, os.ErrNotExist) { return err }`. One line of error handling. Apply inline during implementation.

2. **[mechanical] §3 / AC3 — JSON struct tags missing from Credentials; AC3 will fail**
   What I observed: Design §3 defines `Credentials` struct as `{Token, Email, Tier string, ExpiresAt time.Time}`. Go's `encoding/json` uses the exported field name by default → produces `{"Token":..., "Email":..., "Tier":..., "ExpiresAt":...}`. Spec AC3 explicitly requires field names `token`, `email`, `tier`, `expires_at` (lowercase/snake_case).
   What to ask the implementer: Add json struct tags:
   ```go
   type Credentials struct {
       Token     string    `json:"token"`
       Email     string    `json:"email"`
       Tier      string    `json:"tier"`
       ExpiresAt time.Time `json:"expires_at"`
   }
   ```
   Apply inline. The Verifier will check field names in the written credentials file — without tags this is a guaranteed AC3 failure.

3. **[mechanical] §3 / Step 6 — `cmd/sworn/main.go` documented shared with T4-mcp; convention not acknowledged in design**
   What I observed: Design §3 lists `cmd/sworn/main.go` as a touchpoint (add `login`, `logout`, `account` to the switch). S08a-mcp-transport (T4, running in parallel) also plans to touch `cmd/sworn/main.go`. The touchpoint matrix in `index.md` marks this file as "DOCUMENTED SHARED — additive dispatch only" with ✓ in both T3 and T4 columns.
   What to ask the implementer: Confirm T3's `main.go` changes are additive dispatch cases only (new `case "login":`, `case "logout":`, `case "account":` blocks; no structural changes to the switch). The T4 parallel lander resolves hunk conflicts on final assembly. No design change needed — just acknowledge in §3 notes.

4. **[escalate] §6 Q1 — Production auth endpoint: how does `sworn login` wire the real SwornAgent endpoint at R3?**
   What I observed: Design Decision 4 correctly parameterizes `authEndpoint` for testability and defers the production value to "S06b or later." §6 Q1 asks: compile-time constant, env var (`SWORN_AUTH_ENDPOINT`), or config file field?
   What the Coach must decide:
   - Option (a) `SWORN_AUTH_ENDPOINT` env var: no recompile; consistent with S06b's `SWORN_PROXY_URL` pattern; user must set it for `sworn login` to do anything real.
   - Option (b) Compile-time constant (linker flag at release): transparent UX; requires a new binary build to change the URL.
   - Option (c) Config file field in `config.json`: adds config coupling; defers to S09 or later.
   Option (a) is the pattern S06b already uses for the proxy URL. The implementer leans toward (a) and can proceed with that assumption unless Coach picks otherwise. Coach should confirm or redirect.

5. **[escalate] §6 Q3 — Tier field vocabulary: string values undefined; S06b credit-gating will branch on them**
   What I observed: Spec and design define `Tier string` but specify no vocabulary. §6 Q3 asks: free-text ("free", "pro"), int tier level, or something else? S06b's credit-gating and TUI display will branch on `creds.Tier`.
   What the Coach must decide: The SwornAgent backend defines this contract. The implementer can proceed with `Tier string` for S06a (any string is valid for AC3). But the Coach should confirm the expected vocabulary so S06b doesn't have to retrofit its gating logic. Even a provisional answer ("use 'free' and 'pro' for now") unblocks S06b planning.

6. **[escalate] §6 Q2 — Permissions warning in Load(): opt-in security hardening**
   What I observed: §6 Q2 asks whether `Load()` should warn on stderr if `credentials.json` is world-readable (broader than 0600). This is a defensive check against users who `chmod`. Not in spec ACs.
   What the Coach must decide:
   - Option (a) Add warning: one `os.Stat` call + mode check in `Load()`. Surfaces exposure silently introduced by a chmod. Minor code.
   - Option (b) Defer post-R3: YAGNI; 0600 write guard is sufficient for R3. Rule 2 card if deferred.
   If (b): implementer creates a Rule 2 card in `open_deferrals` in status.json with why/tracking/acknowledgement.

---

## Summary

Pins: 6 total — 3 [mechanical], 0 [memory-cited], 3 [escalate]
Critical pins: **1 and 2** — both would cause the slice to ship broken if unaddressed (pin 1: AC4 failure on missing-file logout; pin 2: AC3 failure on JSON field names).

## Smaller flags (not pins, worth one-line ack)

- **Drift note**: T3 branch is 1 commit behind `release-wt/` (`f1b9fa9 materialise T4-mcp worktree` — touches only `index.md`, not any spec). Implementer should `git merge release-wt/2026-06-19-safe-parallelism` at implementation start per implement-slice.md step 3.
- **ConfigDir() confirmed absent**: grepped `internal/config/config.go` — `ConfigDir()` is not present. Design Decision 1 ("add ConfigDir()") is correctly needed; no duplication risk.
- **S06b openBrowser() handoff**: S06b spec calls "the same `openBrowser()` helper from S06a." Both files (`login.go`, `account.go`) are in `package main` — unexported helpers are shared within the package. No export needed. No pin.
- **Test coverage vs spec Required tests**: Design §5 enumerates the exact test names from the spec. Coverage plan is complete. No gap.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Design is clear and well-scoped. 2 critical apply-inline fixes + 1 convention note + 3 Coach decisions logged (§6). Coach's answers to §6 follow in this ack.

1. **JSON struct tags.** Add `json:"token"`, `json:"email"`, `json:"tier"`, `json:"expires_at"` to the Credentials struct. Apply inline. AC3 failure without this.
2. **Logout no-op.** After `os.Remove`, suppress `errors.Is(err, os.ErrNotExist)`. One line. Apply inline. AC4 failure without this.
3. **Shared main.go.** T3's additions to `cmd/sworn/main.go` are additive dispatch only (`case "login":` / `case "logout":` / `case "account":`). No structural changes. T4 resolves merge hunks on assembly. Acknowledge in §3 notes.
4. **Auth endpoint (§6 Q1).** [Coach decision TBD — fill in one of: (a) SWORN_AUTH_ENDPOINT env var / (b) compile-time constant / (c) config field before implementer starts.]
5. **Tier format (§6 Q3).** [Coach decision TBD — confirm tier string vocabulary, e.g. "free" / "pro", or int. Implementer uses `Tier string`; vocabulary to be documented.]
6. **Permissions warning (§6 Q2).** [Coach decision TBD — (a) add Load() mode check now, or (b) defer post-R3 with Rule 2 card.]

Address pins 1–3 inline during implementation. Pins 4–6: implement per Coach answers above. Proceed to in_progress once Coach fills in §6 answers.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: yes
REASON: Three escalate pins (production auth endpoint strategy, Tier field vocabulary, permissions hardening) need Coach product decisions; two critical mechanical pins (JSON tags, os.Remove ENOENT) are apply-inline but too important to surface only at verify time.
-->
