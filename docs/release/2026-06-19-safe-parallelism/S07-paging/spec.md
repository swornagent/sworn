---
title: 'S07-paging — FAIL/BLOCKED webhook + email notification'
description: 'When a slice enters failed_verification or BLOCKED, sworn fires a webhook POST to the configured endpoint so the developer is paged without watching the terminal.'
---

# Slice: `S07-paging`

## User outcome

A developer who has closed their terminal after starting `sworn run --parallel` receives
a webhook or email notification when any slice enters `failed_verification` or `BLOCKED`,
including the slice ID, track, and a one-line violation summary — enabling them to act
without polling.

## Entry point

Internal — triggered by `internal/run/run.go` and `internal/scheduler/worker.go` on a FAIL
or BLOCKED verdict transition. Configured via `sworn account set-webhook <url>`.

## In scope

- `internal/account/notify.go`:
  - `Notifier` struct: wraps credentials + webhook URL; no-ops when neither is configured
  - `Notify(ctx, Event)`: sends HTTP POST to webhook URL with JSON payload:
    `{release, track, slice_id, state, violations_summary, worktree_path, timestamp}`
  - `violations_summary`: first violation line from proof.md (or "N violations found"
    if proof not yet written); max 200 chars
  - Retry: 3 attempts, exponential backoff (1s, 2s, 4s); failure is logged to stderr
    and does not block or fail the run loop
  - Email notification: when SwornAgent account is active (`account.IsLoggedIn()`),
    also calls `POST /api/notify` on the SwornAgent API with the same payload;
    SwornAgent sends email to the registered address server-side (no SMTP in the binary)
- `sworn account set-webhook <url>`: stores webhook URL in `~/.config/sworn/credentials.json`
- `sworn account notifications`: prints current webhook URL + whether email is enabled
- Integration with `internal/run/run.go`: on FAIL/BLOCKED verdict, call
  `notifier.Notify(ctx, event)` after writing state to status.json
- Integration with `internal/scheduler/worker.go`: on track FAIL, call
  `notifier.Notify(ctx, trackFailEvent)`

## Out of scope

- Slack native app (webhook covers this; users route via Zapier/n8n/Make)
- SMS notifications (post-R3)
- Per-slice notification preferences (all-or-nothing for R3)
- Push notifications to the TUI (TUI polls DB; no push channel between processes)
- In-app notification inbox (post-R3)

## Planned touchpoints

- `internal/account/notify.go` (new)
- `internal/account/notify_test.go` (new)
- `internal/run/run.go` (touch — call notifier on FAIL/BLOCKED)
- `internal/scheduler/worker.go` (touch — call notifier on track FAIL)
- `cmd/sworn/account.go` (touch — `set-webhook`, `notifications` subcommands)

## Acceptance checks

- [ ] On a FAIL verdict in `run.Run()`, `notifier.Notify()` is called with the correct
  payload (slice ID, track, violations_summary, state = "failed_verification")
- [ ] The webhook POST reaches a test HTTP server with the correct JSON shape:
  `{release: string, track: string, slice_id: string, state: string,
   violations_summary: string, worktree_path: string, timestamp: string}`
- [ ] If the webhook server returns 500, the run loop continues (notification failure
  does not abort the run); 3 retry attempts are logged
- [ ] If no webhook URL is configured and no account is active, `Notify()` is a no-op
  (no error, no network call)
- [ ] `sworn account set-webhook https://hooks.example.com/sworn` stores the URL in
  the credentials file and `sworn account notifications` shows it
- [ ] When `account.IsLoggedIn()` is true, `Notify()` also sends a POST to the
  SwornAgent `/api/notify` endpoint (verified with a mock server in tests)
- [ ] `go test ./internal/account/...` covers all notify paths (mock webhook server)

## Required tests

- **Unit**: `internal/account/notify_test.go`
  — `TestNotifyWebhook`: mock HTTP server; call Notify; assert server received correct
    payload; assert function returns nil
  — `TestNotifyRetryOnFailure`: mock server returns 500 three times; assert 3 POST
    attempts; assert run does not error
  — `TestNotifyNoOp`: no webhook URL, no account; assert zero network calls
  — `TestNotifyWithAccount`: logged-in account; assert both webhook + SwornAgent API
    called (two different mock endpoints)
- **Integration**: `internal/run/run.go` integration test — inject a failing mock
  verifier; assert `notifier.Notify` is called exactly once with the correct slice ID
- **Reachability artefact**: smoke step — `sworn account set-webhook https://webhook.site/<id>`;
  run a fixture release designed to FAIL; confirm webhook.site receives the notification.
  Document the webhook.site URL and received payload in proof.md.

## Risks

- The SwornAgent `/api/notify` endpoint does not exist yet. The email-via-API path
  degrades gracefully (logs a warning if the endpoint is unreachable) and is not
  required for the reachability smoke step. The webhook path is the primary test target.
- `worktree_path` in the payload contains an absolute path from the developer's machine.
  This is intentional (the TUI uses it to open the worktree) but means the payload
  is not portable across machines. Acceptable for R3; noted in proof.md.

## Deferrals allowed?

Yes, with Rule 2 compliance:
- Email via SwornAgent API: if the `/api/notify` endpoint is not ready at implementation
  time, this path is stubbed with a log line. Why: server-side infrastructure gated on
  the SwornAgent backend build timeline. Tracking: SwornAgent backend backlog. Ack: now.
