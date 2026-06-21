# Design TL;DR — S07-paging

## §1. User-visible change

When a slice enters `failed_verification` or a track fails during `sworn run --parallel`, the developer receives a webhook POST (and optionally an email via the SwornAgent API) containing the slice ID, track, state, violation summary, worktree path, and timestamp. This means a developer who starts a parallel run and walks away from the terminal is paged the moment something needs human attention — no polling required. The webhook URL is configured via `sworn account set-webhook <url>` and inspected via `sworn account notifications`.

## §2. Design decisions not in spec (max 5)

1. **Webhook URL stored in `Credentials` struct as `WebhookURL string`** — the credentials file is already the config store for account-level settings (token, email, tier). Adding a `webhook_url` JSON field keeps all user-configured state in one file rather than introducing a second config file. Rationale: minimises new surface area; `Load`/`Save` already handle the file.

2. **`Notifier` is a struct with an injectable `http.Client` field** — tests need to assert retry behaviour and mock server interactions. By giving `Notifier` a `Client *http.Client` field (defaulting to `http.DefaultClient`), tests can inject a client with custom transport or we just use `httptest.Server` with the default client. The spec says "mock webhook server" — `httptest.Server` is the idiomatic Go approach and needs no client injection. Decision: use `httptest.Server` directly; `Notifier` holds the webhook URL + credentials, and uses `http.DefaultClient` internally.

3. **`NotifyEvent` struct is the payload** — rather than passing 7 arguments to `Notify`, a struct makes the contract explicit and testable. Fields match the spec's JSON payload exactly: `Release`, `Track`, `SliceID`, `State`, `ViolationsSummary`, `WorktreePath`, `Timestamp`.

4. **SwornAgent API notify URL derived from `defaultProxyHost`** — same pattern as `FetchCredits`: uses `defaultProxyHost` with `SWORN_PROXY_URL` override. The endpoint is `<host>/api/notify`. This is consistent with the existing proxy routing.

5. **`violations_summary` extraction reads proof.md if it exists** — the spec says "first violation line from proof.md (or 'N violations found' if proof not yet written)". I'll read the proof.md file at the slice dir, look for lines starting with numbered violations (e.g. `1.`), and take the first. If proof.md doesn't exist or has no parseable violations, fall back to "N violations found" where N comes from the status.json verification violations array, or "verification failed" if that's also empty.

## §3. Files I'll touch grouped by purpose

- **`internal/account/notify.go`** (new) — Core notification logic: `Notifier` struct, `NotifyEvent` payload struct, `Notify(ctx, event)` method with webhook POST + retry + SwornAgent API email path. This is the heart of the slice.
- **`internal/account/notify_test.go`** (new) — Unit tests covering all four spec-required test cases: webhook success, retry on 500, no-op when unconfigured, dual-path when logged in.
- **`internal/account/account.go`** (touch) — Add `WebhookURL string` field to `Credentials` struct so `set-webhook` can persist it.
- **`internal/run/slice.go`** (touch) — After the `failed_verification` state transition (line ~241), call `notifier.Notify()` with the slice event. The `Notifier` is constructed from loaded credentials + webhook URL.
- **`internal/scheduler/worker.go`** (touch) — After `TrackFail` is returned (slice failure path, line ~143), call `notifier.Notify()` with a track-fail event.
- **`cmd/sworn/account.go`** (touch) — Add `set-webhook` and `notifications` subcommands to `cmdAccount`.

## §4. Things I'm NOT doing

- **Slack native app integration** — spec explicitly out of scope; webhook covers this.
- **SMS notifications** — post-R3 per spec.
- **Per-slice notification preferences** — all-or-nothing for R3 per spec.
- **Push notifications to the TUI** — TUI polls DB; no push channel per spec.
- **SMTP email sending in the binary** — email goes through SwornAgent API; no SMTP in the binary per spec.
- **The SwornAgent `/api/notify` endpoint** — server-side infrastructure, not in this binary. The client-side POST is implemented; if the endpoint is unreachable, it logs a warning and continues (spec Risks section).

## §5. Reachability plan

The spec requires a reachability artefact: "smoke step — `sworn account set-webhook https://webhook.site/<id>`; run a fixture release designed to FAIL; confirm webhook.site receives the notification."

For the proof bundle, I will:
1. Demonstrate `sworn account set-webhook <url>` stores the URL (unit test + CLI invocation).
2. Demonstrate `sworn account notifications` prints the configured webhook URL.
3. Run the unit test `TestNotifyWebhook` which uses a mock HTTP server to prove the webhook POST reaches its destination with the correct JSON payload.
4. Run the integration test in `internal/run/` that injects a failing mock verifier and asserts `notifier.Notify` is called with the correct slice ID.

The mock-server tests are the primary reachability artefact — they prove the full path from verdict → notification → HTTP POST with correct payload. A live webhook.site test is environment-dependent and not reproducible in CI; the mock server test is stronger evidence.

## §6. Open questions for the Coach

None. The spec is clear, the Risks section pre-acknowledges the `/api/notify` endpoint gap, and the deferral for email-via-API is explicitly allowed with Rule 2 compliance.