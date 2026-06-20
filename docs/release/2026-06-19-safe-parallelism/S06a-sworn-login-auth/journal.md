# Journal: S06a-sworn-login-auth

## Session: 2026-06-21 — Initial implementation

**Role**: Implementer
**Worktree**: `/home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T3-commercial`
**Branch**: `track/2026-06-19-safe-parallelism/T3-commercial`

### State transitions

- `design_review` → `in_progress` (2026-06-21): Coach approved design via approved-ack.md. Forward-merged release-wt first (1 commit behind).
- `in_progress` → `implemented` (2026-06-21): All code written, all tests pass.

### Decisions and trade-offs

1. **CredentialsPath() exported**: The account package exports `CredentialsPath()` rather than keeping `configDir()` private. This avoids duplicating the path resolution in the CLI layer while keeping `configDir()` unexported for encapsulation. login.go uses `filepath.Dir(CredentialsPath())` for the Save() directory.

2. **AuthURL ldflags pattern**: Followed Coach decision (approved-ack.md pin 4): `var authURL` in login.go is the ldflags-able compile-time default; `SWORN_AUTH_URL` env var overrides at runtime. The auth server base URL is `https://auth.sworn.sh` (without `/device` suffix — `DeviceCodeFlow` appends `/device/code` and `/device/token`).

3. **openBrowser uses os/exec**: The account package imports `os/exec` for browser opening. The spec's Risks section says to wrap it in a helper that falls back to printing the URL. Three-tier fallback: `xdg-open` (Linux), `open` (macOS), `start` (Windows). All failures silently degrade to printing the URL.

4. **Test coverage**: 10 tests covering all 7 acceptance checks plus edge cases (cancelled context, missing file, non-existent dir, JSON field names).

### Coach directives applied (from approved-ack.md)

- **Pin 1**: JSON struct tags `json:"token"`, `json:"email"`, `json:"tier"`, `json:"expires_at"` — applied to Credentials struct
- **Pin 2**: Logout suppresses `errors.Is(err, os.ErrNotExist)` — applied in cmdLogout in login.go
- **Pin 3**: Main.go additive dispatch only with comments noting T3 ownership — applied
- **Pin 4**: SWORN_AUTH_URL env var with ldflags default — authURL variable in login.go + resolveAuthEndpoint()
- **Pin 5**: Tier is free-text string (`Tier string`) — applied
- **Pin 6**: Silent 0600 enforce at write time, no Load() check — applied in Save() and Load()

### Deferrals

None.

### First-pass verification

`release-verify.sh` result: **PASS** (23/23 checks). No failures to address.

### Pre-verification skeptic panel

Runtime does not support subagent dispatch (single-threaded API call mode, no parallel tool). Skipped. Noted: `skeptic_panel: skipped — runtime does not support subagent dispatch`.

### Implementation summary

Production files created:
- `internal/account/account.go` — DeviceCodeFlow, Credentials struct (with JSON tags), Save (0600), Load, IsLoggedIn, openBrowser (3-tier fallback)
- `internal/account/account_test.go` — 10 tests covering all spec acceptance checks
- `cmd/sworn/login.go` — `sworn login` (DeviceCodeFlow with SWORN_AUTH_URL/env), `sworn logout` (suppress os.ErrNotExist)
- `cmd/sworn/account.go` — `sworn account` (display email + tier)

Files touched:
- `internal/config/config.go` — added ConfigDir() helper
- `cmd/sworn/main.go` — added login, logout, account dispatch cases (additive only)