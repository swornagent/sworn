---
title: 'S06-sworn-login — SwornAgent account + credits on-ramp'
description: 'sworn login authenticates via device-code flow; subsequent sworn run invocations route model calls through the SwornAgent proxy consuming credits, with no provider API keys needed.'
---

# Slice: `S06-sworn-login`

## User outcome

A developer runs `sworn login`, completes a device-code authentication flow, and
subsequent `sworn run` invocations route model calls through the SwornAgent managed
proxy — consuming credits — without the developer needing to configure any provider
API keys.

## Entry point

`sworn login` subcommand; `sworn logout`; `sworn account` (shows credits/email/tier);
`sworn account buy <N>` (opens billing URL).

## In scope

- `internal/account/account.go`:
  - Device-code flow: call SwornAgent auth endpoint → receive device code + verification
    URL → open URL in system browser (`xdg-open` / `open` / `start`) → poll for token
    (interval from auth response) → store token + expiry + email at
    `~/.config/sworn/credentials.json` (mode 0600)
  - `account.Load()`: reads credentials from config file; returns nil if absent
  - `account.IsLoggedIn()`: true if valid non-expired token exists
- `internal/account/proxy.go`:
  - `proxy.Endpoint(credentials)` → returns the SwornAgent proxy URL for a given
    model ID; used by `internal/model/client.go` to route requests
  - When `account.IsLoggedIn()` is true and `SWORN_DIRECT=` is not set, model calls
    in `model.FromEnv` route through the proxy endpoint instead of directly to the
    provider; the proxy handles auth (token in `Authorization: Bearer`) and credits
- Credit balance: cached at `~/.config/sworn/credits.json`; refreshed on login and
  on each `sworn run` invocation start (non-blocking, skipped on network error)
- `sworn login`: launches auth flow; on success prints "Authenticated as <email> —
  10 credits available"
- `sworn logout`: deletes credentials file; prints "Logged out"
- `sworn account`: prints email, tier, credit balance (from cache or live API)
- `sworn account buy <N>`: opens `https://swornagent.com/credits/buy?n=<N>` in browser
- `cmd/sworn/main.go` dispatch: `login`, `logout`, `account` subcommands

## Out of scope

- Full billing/payment infrastructure (server-side at SwornAgent; not in this repo)
- SAML/SSO enterprise auth (post-R3)
- Team/shared credit pools (post-R3)
- OAuth PKCE web flow (device-code only for R3; PKCE is post-R3)
- In-TUI credit purchase flow (post-R3; `sworn account buy` opens browser)

## Planned touchpoints

- `internal/account/account.go` (new)
- `internal/account/account_test.go` (new)
- `internal/account/proxy.go` (new)
- `internal/account/proxy_test.go` (new)
- `cmd/sworn/login.go` (new — `sworn login` + `sworn logout`)
- `cmd/sworn/account.go` (new — `sworn account` + `sworn account buy`)
- `internal/model/client.go` (touch — check `account.Load()` and route via proxy)
- `internal/config/config.go` (touch — `~/.config/sworn/` directory helpers)
- `cmd/sworn/main.go` (touch — dispatch `login`, `logout`, `account`)

## Acceptance checks

- [ ] `sworn login` prints a device code and verification URL; after mock-auth token
  injection in tests, credentials are written to `~/.config/sworn/credentials.json`
  with mode 0600
- [ ] `sworn account` with a valid credentials file prints email, tier, and credit
  balance without error; with no credentials file prints "Not logged in — run
  `sworn login`"
- [ ] `sworn logout` deletes the credentials file and prints "Logged out"
- [ ] After `sworn login`, `model.FromEnv(modelID)` returns a client that sends
  requests to the proxy endpoint (verified by checking the request URL in
  `proxy_test.go` using a test HTTP server)
- [ ] With `SWORN_DIRECT=1` set, `model.FromEnv` bypasses the proxy even when logged
  in (escape hatch for debugging)
- [ ] With no credentials and no provider API keys, `sworn run` exits with a clear
  message: "No model configured — set a provider API key or run `sworn login`"
- [ ] `sworn account buy 10` opens `https://swornagent.com/credits/buy?n=10` (verified
  by mocking the browser-open function and asserting the URL)
- [ ] Credentials file is not committed to git (`~/.config/sworn/` is outside the repo;
  verify the file path is not inside the workspace root)

## Required tests

- **Unit**: `internal/account/account_test.go`
  — `TestDeviceCodeFlow`: mock auth server; assert token stored correctly after polling
  — `TestLoadCredentials`: write fixture credentials file; assert Load() returns them
  — `TestIsLoggedIn`: expired token → false; valid token → true
- **Unit**: `internal/account/proxy_test.go`
  — `TestProxyRouting`: `model.FromEnv` with logged-in account sends request to a
    test HTTP server at the proxy URL, not directly to the provider
  — `TestProxyBypassWithDirectFlag`: `SWORN_DIRECT=1` → request goes to provider URL
- **Reachability artefact**: smoke step — run `sworn login` against a staging
  SwornAgent auth endpoint (or mock); confirm credentials file created; run `sworn account`;
  confirm output shows email + credits. Document in proof.md with exact commands.

## Risks

- The SwornAgent auth + proxy endpoints do not exist yet (this is a client-only slice;
  the server is separate infrastructure). The implementer must use a local stub or
  staging endpoint. The acceptance checks use a mock HTTP server for tests; the
  reachability artefact uses a staging endpoint. Document the staging URL in the proof.
- Credentials file permission (0600) is critical for security. AC-1 verifies this.
  If the file is world-readable, it is a security defect — FAIL the slice.

## Deferrals allowed?

No. `sworn login` is the commercial on-ramp. Without it, R3 has no payment surface.
