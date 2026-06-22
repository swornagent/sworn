---
title: 'S06b-sworn-proxy-credits — model proxy routing + sworn account buy + credit display'
description: 'When sworn login credentials are present, model.FromEnv routes calls through the SwornAgent proxy. sworn account buy opens the billing page. Credit balance is cached and shown in the TUI header.'
---

# Slice: `S06b-sworn-proxy-credits`

## User outcome

After `sworn login`, a developer runs `sworn run` without any provider API keys and
their model calls are routed through the SwornAgent managed proxy consuming credits;
`sworn account` shows their credit balance; `sworn account buy <N>` opens the billing
page in their browser.

## Entry point

`model.FromEnv(modelID)` — checks for credentials; `sworn account` (extended with
credits); `sworn account buy <N>`.

## In scope

- `internal/account/proxy.go`:
  - `Endpoint(creds *Credentials, modelID string) string`: returns the SwornAgent
    proxy URL for the given model ID when credentials are present; returns `""` otherwise
  - Proxy URL format: `https://api.swornagent.com/proxy/v1/<modelID>/...` (exact
    path TBD based on backend). The default host is **compiled in** (ldflags var,
    mirroring S06a's `SWORN_AUTH_URL` pattern — Coach ack pin B). `SWORN_PROXY_URL`
    is a **test-only** override, not a production config knob: it exists so the
    implementer can point at a local stub. The sworn bearer token is only ever sent
    to the compiled-in default host unless `SWORN_PROXY_URL` is explicitly set, and
    when it is set the client warns on stderr that credentials are being routed to a
    non-default host.
  - Request forwarding: the proxy accepts the same JSON body as the provider API;
    SwornAgent adds authentication server-side from the token in the `Authorization:
    Bearer <token>` header
  - Payment / credit-exhaustion failure path (Coach ack pin C): when the proxy
    responds `402 Payment Required` (or an equivalent insufficient-credits body),
    `model.FromEnv`'s client surfaces a clear, non-silent error —
    `"out of SwornAgent credits — run \`sworn account buy\` to add more"` — and
    returns a non-nil error. It never silently downgrades to direct provider calls.
- `internal/model/client.go`: in `FromEnv(modelID)`, call `account.Load()` and
  `proxy.Endpoint()` before constructing the HTTP client; if an endpoint is returned,
  use it as the base URL; honour `SWORN_DIRECT=1` as an override
- Credit balance:
  - `internal/account/account.go` (touch): add `FetchCredits(ctx, creds) (int, error)`
    — GET `https://api.swornagent.com/account/credits`; stores result in
    `~/.config/sworn/credits.json`
  - `sworn run` startup: call FetchCredits non-blocking (goroutine, timeout 3s);
    update cache file if successful; proceed regardless
- `sworn account` (touch `cmd/sworn/account.go`): extended to also print credit balance
  from cache (or "–" if cache absent); "run \`sworn account buy\` to add credits".
  Credit balance is an **integer count of credits** (Coach ack pin A) — displayed as
  `Credits: <int>`. The credit→token→currency conversion rate is a backend concern and
  is out of scope for this slice (see Out of scope / api-contract stub).
- `sworn account buy <N>`: opens `https://swornagent.com/credits/buy?n=<N>` in browser
  via the same `openBrowser()` helper from S06a. `<N>` is a **number of credits**
  (same unit as the displayed balance — Coach ack pin A).

## Out of scope

- Billing webhook or Stripe integration (backend-side)
- Team credit pools (post-R3)
- In-TUI credit purchase flow (post-R3)
- The TUI header credit display integration (S04b reads the cache file directly)

## Planned touchpoints

- `internal/account/proxy.go` (new)
- `internal/account/proxy_test.go` (new)
- `internal/account/account.go` (touch — add FetchCredits)
- `internal/account/account_test.go` (touch — add FetchCredits test)
- `internal/model/client.go` (touch — proxy routing via account.Load + proxy.Endpoint)
- `cmd/sworn/account.go` (touch — credit balance + buy subcommand)

## Acceptance checks

- [ ] `model.FromEnv(modelID)` with valid credentials sends requests to the proxy
  URL (verified by test HTTP server at proxy URL receiving the request)
- [ ] With `SWORN_DIRECT=1` set, requests go to the provider URL even when credentials
  are present
- [ ] **(Coach ack pin B — credential-trust boundary)** With `SWORN_PROXY_URL` unset,
  the bearer token is sent only to the compiled-in default host (verified by a test
  asserting the request target host equals the default, not any env-supplied value).
  When `SWORN_PROXY_URL` is set, the client emits a stderr warning that credentials are
  being routed to a non-default host (verified by capturing stderr in the test).
- [ ] **(Coach ack pin C — payment failure path)** When the proxy returns `402`
  (insufficient credits), `model.FromEnv`'s client returns a non-nil error whose
  message points the user to `sworn account buy`, and does **not** fall back to a
  direct provider call (verified by a mock proxy returning 402 and asserting the error
  text + that no provider request is made).
- [ ] With no credentials file, `model.FromEnv` behaviour is unchanged from before
  this slice (direct to provider or error if no API key)
- [ ] `sworn account` with credentials shows email, tier, and credit balance as
  `Credits: <int>` (from cache) — **(Coach ack pin A — integer credit unit)**
- [ ] `sworn account buy 20` opens `https://swornagent.com/credits/buy?n=20` (verified
  by mocking openBrowser and asserting the URL)
- [ ] `FetchCredits` updates `~/.config/sworn/credits.json` when the API responds;
  `sworn run` startup calls it non-blocking and proceeds even if it times out
- [ ] `go test ./internal/account/...` and `go test ./internal/model/...` pass

## Required tests

- **Unit**: `internal/account/proxy_test.go`
  — `TestProxyEndpointWithCreds`: credentials present → returns proxy URL string
  — `TestProxyEndpointNoCreds`: nil credentials → returns empty string
- **Unit**: `internal/model/client.go` (update existing test or add):
  — `TestFromEnvUsesProxy`: credentials + mock proxy server; assert request hits proxy
  — `TestFromEnvBypassProxy`: `SWORN_DIRECT=1`; assert request hits provider URL
  — `TestFromEnvProxyDefaultHost` (pin B): `SWORN_PROXY_URL` unset + credentials;
    assert the request host is the compiled-in default, not env-derived
  — `TestFromEnvProxyOverrideWarns` (pin B): `SWORN_PROXY_URL` set; assert a stderr
    warning is emitted about non-default credential routing
  — `TestFromEnvInsufficientCredits` (pin C): mock proxy returns 402; assert non-nil
    error pointing to `sworn account buy` and no provider fallback request
- **Unit**: `internal/account/account_test.go`
  — `TestFetchCredits`: mock credits API returns 47; assert cache file written with 47
  — `TestFetchCreditsTimeout`: mock server hangs; assert FetchCredits returns after 3s
    without blocking the caller
- **Reachability artefact**: with a local mock proxy server running: `sworn login`
  (mock auth), then `sworn run --task "hello"` (mock run); assert in logs that the
  request went to the mock proxy URL. Document in proof.md.

## Risks

- The SwornAgent proxy API shape is not yet defined. `SWORN_PROXY_URL` exists as a
  **test-only** override so the implementer can point to a local stub without code
  changes; the production host is compiled in (pin B). Document the expected request
  format, the credit unit (integer credits — pin A), and the 402 insufficient-credits
  response (pin C) in a new `docs/api-contract.md` stub.
- **Credential-trust boundary (pin B):** `model.FromEnv` attaches the sworn bearer
  token to the proxy request. Honouring an arbitrary `SWORN_PROXY_URL` in production
  would be a token-exfiltration vector. Mitigated by compiling in the default host and
  treating the env var as a test override that warns when used.

## Deferrals allowed?

No. Without proxy routing, `sworn login` has no effect on model calls.
