# Design TL;DR: S06a-sworn-login-auth

## §1. User-visible change

A developer runs `sworn login`, sees a device code + verification URL in their terminal, opens the URL in a browser to authenticate, and their credentials are saved to `~/.config/sworn/credentials.json`. `sworn logout` clears the file. `sworn account` prints their email and tier (from the saved credentials). No API calls are proxied yet — that's S06b.

## §2. Design decisions not in spec (max 5)

1. **ConfigDir() lives in `internal/config/config.go` as a thin wrapper around `filepath.Dir(Path())`.**
   Rationale: `config.Path()` already resolves the config file path with XDG/cross-platform logic. `ConfigDir()` is `filepath.Dir(Path())` — one line. Adding it to `config` avoids duplicating the directory-resolution logic in `account`.

2. **Credentials file lives at `~/.config/sworn/credentials.json`, alongside `config.json`.**
   Rationale: The spec says `configDir() returns ~/.config/sworn` — that's the same directory the config package already targets. Colocating credentials with the config file keeps the file-system surface simple.

3. **Browser-open helper uses a three-tier fallback: `xdg-open` (Linux), `open` (macOS), `start` (Windows), then prints the URL. Never errors.**
   Rationale: The spec's Risks section prescribes this approach. Making the browser-open best-effort (print the URL on failure) is the pragmatic path — we control the server, not the terminal environment.

4. **DeviceCodeFlow accepts an `authEndpoint string` parameter rather than pulling it from a central constant.**
   Rationale: The spec explicitly parameterises the auth endpoint. In tests, a mock `httptest.Server` provides the endpoint. In production, the SwornAgent auth server URL will come from config/env (S06b or later) — parameterising now avoids a hidden dep on a not-yet-existing config field.

5. **Poll interval defaults to 2 seconds if the server doesn't provide one.**
   Rationale: The device-code spec says the server returns an `interval` field. If zero or missing, polling every 2s is a reasonable default that avoids hammering the server but keeps UX responsive.

## §3. Files I'll touch grouped by purpose

| Purpose | Files | Why |
|---|---|---|
| **Auth logic** | `internal/account/account.go` (new) | Device-code flow, credential persistence, login status check |
| **Auth tests** | `internal/account/account_test.go` (new) | Mock HTTP server; test Save/Load/IsLoggedIn/Logout |
| **CLI commands** | `cmd/sworn/login.go` (new) | `sworn login` + `sworn logout` subcommands |
| **CLI account** | `cmd/sworn/account.go` (new) | `sworn account` — display email + tier |
| **Config helper** | `internal/config/config.go` (touch) | Add `ConfigDir() string` — one line |
| **CLI dispatch** | `cmd/sworn/main.go` (touch) | Add `login`, `logout`, `account` to the switch |

## §4. Things I'm NOT doing

- **Proxy routing** (S06b). No proxy-aware model client, no credential-check on API calls.
- **`sworn account buy`** (S06b). No payment flow.
- **Token refresh**. The credentials have `ExpiresAt`; `IsLoggedIn` checks it. If expired, the user must re-run `sworn login`. Refresh tokens are post-R3.
- **Any real auth endpoint**. All tests use `httptest.NewServer`. The reachability smoke step will use a locally-run stub server documented in proof.md.
- **Persistent session in the TUI**. S04a/b/c handle the TUI. This slice is CLI-only.
- **Loading credentials from env vars**. Credentials go to disk only. Future slices may add env overrides.

## §5. Reachability plan

1. Unit tests: `go test -v -count=1 ./internal/account/...` — exercises DeviceCodeFlow (mock server), Save, Load, IsLoggedIn, Logout.
2. `go build -o /tmp/sworn ./cmd/sworn && /tmp/sworn login` against a Go stub HTTP server (started in a separate terminal/screen). Capture the stub command and actual terminal output (device code, verification URL printed, then polling + success message).
3. `go vet ./...` — no new warnings.

## §6. Open questions for the Coach

- **Auth endpoint URL**: The mock server URL works for tests, but what URL should the production `sworn login` target? Should it be a compile-time constant, an env var (`SWORN_AUTH_ENDPOINT`), or part of the config file? Currently leaning toward env var so no recompile needed, but that means S06a ships without a wired production endpoint — only mock tests. Is that acceptable for an `implemented` slice?
- **Credentials file permissions**: Save() writes mode 0600, Save() creates the dir with mode 0700. Should we also warn if the file has broader permissions when Load() reads it? (Defensive check against users who chmod.)
- **What format should the Tier field be?** The spec says `tier` is one of the JSON fields stored. Is it a free-text string (e.g. "free", "pro"), an int, or something else? This affects the `sworn account` display and (in S06b) credit-gating logic.