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
    path TBD based on backend; use a configurable base URL via `SWORN_PROXY_URL` env
    for testability)
  - Request forwarding: the proxy accepts the same JSON body as the provider API;
    SwornAgent adds authentication server-side from the token in the `Authorization:
    Bearer <token>` header
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
  from cache (or "–" if cache absent); "run \`sworn account buy\` to add credits"
- `sworn account buy <N>`: opens `https://swornagent.com/credits/buy?n=<N>` in browser
  via the same `openBrowser()` helper from S06a

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
- [ ] With no credentials file, `model.FromEnv` behaviour is unchanged from before
  this slice (direct to provider or error if no API key)
- [ ] `sworn account` with credentials shows email, tier, and credit balance (from cache)
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
- **Unit**: `internal/account/account_test.go`
  — `TestFetchCredits`: mock credits API returns 47; assert cache file written with 47
  — `TestFetchCreditsTimeout`: mock server hangs; assert FetchCredits returns after 3s
    without blocking the caller
- **Reachability artefact**: with a local mock proxy server running: `sworn login`
  (mock auth), then `sworn run --task "hello"` (mock run); assert in logs that the
  request went to the mock proxy URL. Document in proof.md.

## Risks

- The SwornAgent proxy API shape is not yet defined. Use `SWORN_PROXY_URL` env var so
  the implementer can point to a local stub without code changes. Document the expected
  request format in `docs/mcp-setup.md` or a new `docs/api-contract.md` stub.

## Deferrals allowed?

No. Without proxy routing, `sworn login` has no effect on model calls.
