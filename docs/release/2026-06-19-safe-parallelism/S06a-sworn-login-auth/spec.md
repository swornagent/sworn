---
title: 'S06a-sworn-login-auth — device-code auth flow + credentials file'
description: 'sworn login authenticates via device-code flow and stores a token at ~/.config/sworn/credentials.json. sworn logout clears it. No proxy routing yet (S06b).'
---

# Slice: `S06a-sworn-login-auth`

## User outcome

A developer runs `sworn login`, is shown a device code and URL, opens the URL in a
browser to authenticate, and their token is stored locally. `sworn logout` clears it.
`sworn account` shows their email and tier. No proxy routing yet.

## Entry point

`sworn login` subcommand; `sworn logout`; `sworn account` (email + tier display only).

## In scope

- `internal/account/account.go`:
  - `DeviceCodeFlow(ctx, authEndpoint string) (token, email string, err error)`:
    POST to `<authEndpoint>/device/code` → receive `{device_code, verification_uri,
    interval}`; open `verification_uri` in system browser; poll `<authEndpoint>/device/token`
    at `interval` until token returned or context cancelled
  - `Credentials` struct: `{Token, Email, Tier string, ExpiresAt time.Time}`
  - `Save(creds Credentials, dir string) error`: writes JSON to
    `<dir>/credentials.json` with mode 0600; creates `<dir>` (mode 0700) if absent
  - `Load(dir string) (*Credentials, error)`: reads + unmarshals; returns nil if absent
  - `IsLoggedIn(creds *Credentials) bool`: creds != nil && time.Now().Before(ExpiresAt)
  - `configDir() string`: returns `~/.config/sworn` (via `os.UserConfigDir()`)
- `cmd/sworn/login.go`: `sworn login` — calls DeviceCodeFlow, prints progress, calls Save
- `cmd/sworn/login.go` (logout in same file): `sworn logout` — calls `os.Remove` on
  credentials.json, prints "Logged out"
- `cmd/sworn/account.go`: `sworn account` — calls Load; prints email + tier if logged in,
  "Not logged in — run \`sworn login\`" if not
- `cmd/sworn/main.go`: dispatch `login`, `logout`, `account` subcommands
- `internal/config/config.go`: add `ConfigDir() string` helper if not already present

## Out of scope

- Proxy routing and credit display (S06b — depends on this slice)
- `sworn account buy` (S06b)
- SAML/SSO, OAuth PKCE (post-R3)
- Team accounts (post-R3)

## Planned touchpoints

- `internal/account/account.go` (new)
- `internal/account/account_test.go` (new)
- `cmd/sworn/login.go` (new — login + logout commands)
- `cmd/sworn/account.go` (new — account command, email/tier only)
- `internal/config/config.go` (touch — ConfigDir helper)
- `cmd/sworn/main.go` (touch — dispatch login, logout, account)

## Acceptance checks

- [ ] `sworn login` (with a mock auth server) prints the verification URL and device
  code, polls until the mock server returns a token, writes
  `~/.config/sworn/credentials.json` with mode 0600, and prints success message
- [ ] `~/.config/sworn/` directory is created with mode 0700 if it does not exist
- [ ] Credentials file is valid JSON with `token`, `email`, `tier`, `expires_at` fields
- [ ] `sworn logout` removes the credentials file and prints "Logged out"; running
  `sworn logout` with no credentials file is a no-op (no error)
- [ ] `sworn account` with a valid credentials file prints email and tier; with no
  file prints "Not logged in — run \`sworn login\`"
- [ ] `account.IsLoggedIn()` returns false for an expired token (ExpiresAt in the past)
- [ ] `go test ./internal/account/...` passes; mock auth server used in all tests

## Required tests

- **Unit**: `internal/account/account_test.go`
  — `TestDeviceCodeFlow`: mock auth server; assert correct POST parameters sent;
    assert polling; assert returned token stored
  — `TestSaveLoadCredentials`: save known credentials; load them back; assert equal
  — `TestSaveMode0600`: verify file mode after Save()
  — `TestIsLoggedIn`: expired creds → false; valid creds → true; nil → false
  — `TestLogoutRemovesFile`: Save then logout; assert file absent
- **Reachability artefact**: smoke step against a staging SwornAgent auth endpoint
  (or a locally-run stub). Document the stub command and output in proof.md.

## Risks

- Browser-open (`xdg-open` / `open` / `start`) is platform-specific. Wrap in a
  `openBrowser(url string) error` helper that tries each in order; falls back to
  printing "Open this URL in your browser: <url>" if none are available. Never fail
  silently — the user must know the URL.
- The SwornAgent auth endpoint does not exist yet. Use a mock server in all tests.
  The staging endpoint used for the reachability smoke step may be a simple mock HTTP
  server documented in proof.md; it does not need to be production SwornAgent infra.

## Deferrals allowed?

No. S06b's proxy routing depends on Load() and IsLoggedIn() from this slice.
