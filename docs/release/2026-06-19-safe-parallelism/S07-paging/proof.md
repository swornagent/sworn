# Proof Bundle — S07-paging

## Scope

FAIL/BLOCKED webhook + email notification: when a slice enters `failed_verification` or `BLOCKED`, sworn fires a webhook POST and optionally an email via the SwornAgent API.

## Files changed

```
cmd/sworn/account.go
cmd/sworn/run.go
internal/account/account.go
internal/account/notify.go
internal/account/notify_test.go
internal/run/parallel.go
internal/run/slice.go
internal/scheduler/worker.go
```

## Test results

### `go test ./internal/account/... -v -count=1`

```
=== RUN   TestNotifyWebhook
--- PASS: TestNotifyWebhook (0.00s)
=== RUN   TestNotifyRetryOnFailure
sworn notify: webhook returned HTTP 500 (attempt 1/3)
sworn notify: webhook returned HTTP 500 (attempt 2/3)
sworn notify: webhook returned HTTP 500 (attempt 3/3)
sworn notify: webhook delivery failed after 3 attempts
--- PASS: TestNotifyRetryOnFailure (3.00s)
=== RUN   TestNotifyNoOp
--- PASS: TestNotifyNoOp (0.00s)
=== RUN   TestNotifyNoOp_NilNotifier
--- PASS: TestNotifyNoOp_NilNotifier (0.00s)
=== RUN   TestNotifyWithAccount
--- PASS: TestNotifyWithAccount (0.00s)
=== RUN   TestNotifyWithAccount_ExpiredToken
--- PASS: TestNotifyWithAccount_ExpiredToken (0.00s)
=== RUN   TestNotifyWebhook_TimeoutContext
sworn notify: webhook POST attempt 1/3: Post "http://127.0.0.1:37883": context deadline exceeded
--- PASS: TestNotifyWebhook_TimeoutContext (0.10s)
=== RUN   TestViolationsSummary_FromFile
--- PASS: TestViolationsSummary_FromFile (0.00s)
=== RUN   TestViolationsSummary_Truncation
--- PASS: TestViolationsSummary_Truncation (0.00s)
=== RUN   TestNotifyEvent_JSONShape
--- PASS: TestNotifyEvent_JSONShape (0.00s)
=== RUN   TestNotify_TimestampDefault
--- PASS: TestNotify_TimestampDefault (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/account	10.124s
```

### `go test ./internal/run/... -v -count=1 -run 'TestRunSlice'`

```
=== RUN   TestRunSlice
sworn run: attempt 1/1 — implementing with fake/impl
sworn run: verifying with fake/verifier
sworn run: verdict PASS (cost $0.0000)
sworn run: rationale: PASS: all good
--- PASS: TestRunSlice (0.05s)
=== RUN   TestRunSliceFail
sworn run: attempt 1/2 — implementing with fake/impl1
sworn run: verifying with fake/verifier
sworn run: verdict FAIL (cost $0.0000)
sworn run: rationale: FAIL: missing test
sworn run: verification failed — retrying with escalated implementer model
sworn run: attempt 2/2 — implementing with fake/impl2
sworn run: verifying with fake/verifier
sworn run: verdict FAIL (cost $0.0000)
sworn run: rationale: FAIL: still missing
--- PASS: TestRunSliceFail (0.08s)
=== RUN   TestRunSlice_MissingVerifierModel
--- PASS: TestRunSlice_MissingVerifierModel (0.04s)
PASS
ok  	github.com/swornagent/sworn/internal/run	0.173s
```

### `go test ./internal/scheduler/... -v -count=1 -run 'TestRunTrack'`

```
=== RUN   TestRunTrack_AllSlicesPass
[T1] starting
[T1] running slice S01-test
[T1] done
--- PASS: TestRunTrack_AllSlicesPass (0.00s)
=== RUN   TestRunTrack_ContextCancelled
[T1] skipped: depends_on failed
--- PASS: TestRunTrack_ContextCancelled (0.00s)
=== RUN   TestRunTrack_SliceFail
[T1] starting
[T1] running slice S01-fail
[T1] slice S01-fail failed: simulated failure: S01-fail
--- PASS: TestRunTrack_SliceFail (0.00s)
=== RUN   TestRunTrack_MultiSliceOrdering
[T1] starting
[T1] running slice S01-first
[T1] running slice S02-second
[T1] running slice S03-third
[T1] done
--- PASS: TestRunTrack_MultiSliceOrdering (0.00s)
=== RUN   TestRunTrack_MaterialisesWorktree
[T1] starting
[T1] materialising worktree at /tmp/TestRunTrack_MaterialisesWorktree2198333066/001/nonexistent-worktree
[T1] worktree materialisation failed: exit status 128
  fatal: not a git repository (or any of the parent directories): .git

--- PASS: TestRunTrack_MaterialisesWorktree (0.00s)
=== RUN   TestRunTrack_EmptySlices
[T1] starting
[T1] done
--- PASS: TestRunTrack_EmptySlices (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/scheduler	0.017s
```

### `go vet`

```
(clean — no output)
```

## Reachability artefact

**Live webhook.site smoke test (2026-06-22):**
- Webhook.site URL: `https://webhook.site/e79d3ba0-435e-473d-8d0e-c3d8209bcae2`
- POST sent with exact Notifier JSON payload: `{release: "2026-06-19-safe-parallelism", track: "T3-commercial", slice_id: "S07-paging", state: "failed_verification", violations_summary: "1. Missing reachability artefact in proof bundle", worktree_path: "/home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T3-commercial", timestamp: "2026-06-22T03:44:39Z"}`
- Webhook.site received the request (UUID: `38181bb4-533e-45c9-9d75-8c5338aa9dd4`, IP: 220.245.209.43, method: POST, size: 375 bytes, content-type: application/json)
- Verified via `GET https://webhook.site/token/e79d3ba0-435e-473d-8d0e-c3d8209bcae2/requests?sorting=newest` — response confirms exact payload match

**Mock server reachability (unit tests — current run):**
- `TestNotifyWebhook`: mock HTTP server receives correct JSON payload with all 7 fields (release, track, slice_id, state, violations_summary, worktree_path, timestamp)
- `TestNotifyWithAccount`: both webhook (mock server #1) and SwornAgent API (mock server #2) are called when account is logged in

## Delivered

- [x] `Notifier` struct in `internal/account/notify.go` — wraps webhook URL + credentials, no-ops when unconfigured
- [x] `Notify(ctx, event)` — sends webhook POST with retry (3 attempts, 1s/2s/4s backoff), SwornAgent API email when logged in
- [x] `NotifyEvent` struct — JSON payload matching spec: `{release, track, slice_id, state, violations_summary, worktree_path, timestamp}`
- [x] BLOCKED notification at `slice.go:222` — fires before error return, `state: "blocked"`, summary from verdict rationale
- [x] FAIL notification at `slice.go:265` — fires after `failed_verification` state write, summary from proof.md violations or fallback
- [x] Track-fail notification at `worker.go:153` — fires on any RunSlice error, `state: "track_failed"`, summary from error message
- [x] `ViolationsSummary()` — reads first numbered violation from proof.md (max 200 chars), falls back to "N violation(s) found"
- [x] `sworn account set-webhook <url>` — stores webhook URL in `~/.config/sworn/credentials.json`
- [x] `sworn account notifications` — prints webhook URL + email notification status
- [x] `WebhookURL` field on `Credentials` struct — JSON `webhook_url` field, omitempty
- [x] Retry behaviour — 3 POST attempts on 500, logged to stderr, does not block caller (returns nil)
- [x] No-op when unconfigured — no webhook URL + no account = zero network calls
- [x] Expired token skip — `IsLoggedIn()` check prevents API call with expired token
- [x] Unit tests: TestNotifyWebhook, TestNotifyRetryOnFailure, TestNotifyNoOp, TestNotifyNoOp_NilNotifier, TestNotifyWithAccount, TestNotifyWithAccount_ExpiredToken, TestNotifyWebhook_TimeoutContext, TestNotifyEvent_JSONShape, TestNotify_TimestampDefault, TestViolationsSummary_FromFile, TestViolationsSummary_Truncation

## Not delivered

- **Email via SwornAgent API (`/api/notify` endpoint):** The client-side POST to `<host>/api/notify` is implemented and tested with a mock server (`TestNotifyWithAccount`). The server-side endpoint does not exist yet — the client logs a warning if unreachable. **Acknowledged**: Coach, 2026-06-22. Tracking: SwornAgent backend backlog.

## Divergence from plan

- **Pin 1:** `planned_files` updated: `internal/run/run.go` → `internal/run/slice.go` (S02a refactor moved `RunSlice`). Added `internal/account/account.go` (WebhookURL field on Credentials).
- **Pin 2 (Option a):** BLOCKED notification fires at `slice.go:222` (before error return) with `state: "blocked"`. FAIL notification fires after `failed_verification` state write at `slice.go:265`. Track-fail notification in `worker.go` covers unexpected/non-verdict errors. Double-notify for BLOCKED/FAIL is intentional: slice-level event has rationale/summary, track-level event is the coarse failure signal.
- **Pin 3 (Coach acked):** Live webhook.site smoke test performed in addition to mock-server unit tests.