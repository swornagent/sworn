§1. User-visible change

When a user logs in with Sworn credentials, model calls are routed through the SwornAgent proxy, credit balance is displayed in the `sworn account` output, and `sworn account buy <N>` opens the billing page.

§2. Design decisions not in spec (max 5)

- Use `SWORN_PROXY_URL` env var to configure the proxy base URL, allowing local stubs for testing.
- Implement `FetchCredits` as a non‑blocking goroutine with a 3 s timeout using a cancellable context.
- Re‑use the existing `openBrowser` helper (from S06a) for the `account buy` command.

§3. Files I'll touch grouped by purpose

- **Proxy implementation**: `internal/account/proxy.go`, `internal/account/proxy_test.go` – endpoint logic and tests.
- **Model client routing**: `internal/model/client.go` – load credentials, call `proxy.Endpoint`, honour `SWORN_DIRECT=1`.
- **Credit fetching**: `internal/account/account.go`, `internal/account/account_test.go` – add `FetchCredits` and its tests.
- **CLI extensions**: `cmd/sworn/account.go` – display credits, add `buy` subcommand.

§4. Things I'm NOT doing

- No billing webhook or external-billing integration.
- No team credit pools (post‑R3).
- No TUI header credit display integration (handled by S04b).

§5. Reachability plan

1. Start a local mock proxy server listening on a configurable URL.
2. Run `sworn login` (mock auth) then `sworn run --task "hello"` and verify in logs that the request was sent to the mock proxy URL.
3. Run `sworn account` and check that the cached credit balance is printed.
4. Run `sworn account buy 20` and assert that `openBrowser` is called with `https://swornagent.com/credits/buy?n=20`.

§6. Open questions for the Coach

*None.*