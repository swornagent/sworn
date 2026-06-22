# Proof Bundle — S07-paging

## Scope

FAIL/BLOCKED webhook + email notification: when a slice enters `failed_verification` or `BLOCKED`, sworn fires a webhook POST and optionally an email via the SwornAgent API.

## Files changed

```
cmd/sworn/account.go
cmd/sworn/account_test.go
cmd/sworn/commands.go
cmd/sworn/commands_test.go
cmd/sworn/login.go
cmd/sworn/main.go
cmd/sworn/memory.go
cmd/sworn/memory_test.go
cmd/sworn/run.go
cmd/sworn/verify.go
docs/release/2026-06-19-safe-parallelism/.captain-trial-log.md
docs/release/2026-06-19-safe-parallelism/S07-paging/journal.md
docs/release/2026-06-19-safe-parallelism/S07-paging/proof.md
docs/release/2026-06-19-safe-parallelism/S07-paging/status.json
docs/release/2026-06-19-safe-parallelism/S19-sworn-induction/spec.md
docs/release/2026-06-19-safe-parallelism/S23-memory-config/design.md
docs/release/2026-06-19-safe-parallelism/S23-memory-config/journal.md
docs/release/2026-06-19-safe-parallelism/S23-memory-config/proof.md
docs/release/2026-06-19-safe-parallelism/S23-memory-config/status.json
docs/release/2026-06-19-safe-parallelism/S24-memory-engine/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S24-memory-engine/design.md
docs/release/2026-06-19-safe-parallelism/S24-memory-engine/journal.md
docs/release/2026-06-19-safe-parallelism/S24-memory-engine/proof.md
docs/release/2026-06-19-safe-parallelism/S24-memory-engine/review.md
docs/release/2026-06-19-safe-parallelism/S24-memory-engine/status.json
docs/release/2026-06-19-safe-parallelism/S25-memory-search/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S25-memory-search/design.md
docs/release/2026-06-19-safe-parallelism/S25-memory-search/journal.md
docs/release/2026-06-19-safe-parallelism/S25-memory-search/proof.md
docs/release/2026-06-19-safe-parallelism/S25-memory-search/review.md
docs/release/2026-06-19-safe-parallelism/S25-memory-search/spec.md
docs/release/2026-06-19-safe-parallelism/S25-memory-search/status.json
docs/release/2026-06-19-safe-parallelism/S40-memory-test-hygiene/journal.md
docs/release/2026-06-19-safe-parallelism/S40-memory-test-hygiene/proof.md
docs/release/2026-06-19-safe-parallelism/S40-memory-test-hygiene/status.json
docs/release/2026-06-19-safe-parallelism/S48-baton-vendor/spec.md
docs/release/2026-06-19-safe-parallelism/S49-baton-version/spec.md
docs/release/2026-06-19-safe-parallelism/S51-cli-command-registry/approved-ack.md
docs/release/2026-06-19-safe-parallelism/S51-cli-command-registry/design.md
docs/release/2026-06-19-safe-parallelism/S51-cli-command-registry/journal.md
docs/release/2026-06-19-safe-parallelism/S51-cli-command-registry/proof.md
docs/release/2026-06-19-safe-parallelism/S51-cli-command-registry/review.md
docs/release/2026-06-19-safe-parallelism/S51-cli-command-registry/spec.md
docs/release/2026-06-19-safe-parallelism/S51-cli-command-registry/status.json
docs/release/2026-06-19-safe-parallelism/index.md
internal/account/account.go
internal/account/account_test.go
internal/account/notify.go
internal/account/notify_test.go
internal/command/registry.go
internal/command/registry_test.go
internal/memory/config.go
internal/memory/config_test.go
internal/memory/discover.go
internal/memory/discover_test.go
internal/memory/embed.go
internal/memory/embed_oai.go
internal/memory/embed_ollama.go
internal/memory/embed_test.go
internal/memory/embed_voyage.go
internal/memory/harness.go
internal/memory/index.go
internal/memory/index_test.go
internal/memory/search.go
internal/memory/search_test.go
internal/run/parallel.go
internal/run/run.go
internal/run/run_test.go
internal/run/slice.go
internal/scheduler/worker.go
```

**S07-owned files** (the subset this slice directly touches): `cmd/sworn/{account,account_test}.go`, `cmd/sworn/login.go`, `cmd/sworn/run.go`, `internal/account/{account,account_test,notify,notify_test}.go`, `internal/run/{parallel,run,run_test,slice}.go`, `internal/scheduler/worker.go`.

**Forward-merge artifacts** (brought in by merging `release-wt/2026-06-19-safe-parallelism` to resolve the stale BLOCKED on `cmd/sworn/main.go`): `cmd/sworn/{main,commands,commands_test,verify,memory,memory_test}.go`, `internal/command/{registry,registry_test}.go`, `internal/memory/*`, and the S23/S24/S25/S40/S51 slice docs. These are owned by other tracks (T15-cli-registry, T8-memory) and are not S07 scope; they enter the diff because the start_commit predates the merge.

## Test results

### `go test ./internal/account/... ./internal/run/... ./internal/scheduler/... ./cmd/sworn/... -count=1`

```
ok  	github.com/swornagent/sworn/internal/account	10.132s
ok  	github.com/swornagent/sworn/internal/run	1.344s
ok  	github.com/swornagent/sworn/internal/scheduler	0.016s
ok  	github.com/swornagent/sworn/cmd/sworn	0.326s
```

### `go test ./internal/account/... -v -count=1`

```
=== RUN   TestDeviceCodeFlow
Device code: abc123
Verification URL: https://example.com/device
--- PASS: TestDeviceCodeFlow (2.00s)
=== RUN   TestDeviceCodeFlowCancel
Device code: abc123
Verification URL: https://example.com/device
--- PASS: TestDeviceCodeFlowCancel (0.00s)
=== RUN   TestSaveLoadCredentials
--- PASS: TestSaveLoadCredentials (0.00s)
=== RUN   TestSaveMode0600
--- PASS: TestSaveMode0600 (0.00s)
=== RUN   TestSaveCreatesDir
--- PASS: TestSaveCreatesDir (0.00s)
=== RUN   TestLoadMissingFile
--- PASS: TestLoadMissingFile (0.00s)
=== RUN   TestIsLoggedIn
=== RUN   TestIsLoggedIn/nil
=== RUN   TestIsLoggedIn/expired
=== RUN   TestIsLoggedIn/valid
--- PASS: TestIsLoggedIn (0.00s)
    --- PASS: TestIsLoggedIn/nil (0.00s)
    --- PASS: TestIsLoggedIn/expired (0.00s)
    --- PASS: TestIsLoggedIn/valid (0.00s)
=== RUN   TestCredentialsJSONFields
--- PASS: TestCredentialsJSONFields (0.00s)
=== RUN   TestLogoutRemovesFile
--- PASS: TestLogoutRemovesFile (0.00s)
=== RUN   TestLoadNonexistentDir
--- PASS: TestLoadNonexistentDir (0.00s)
=== RUN   TestFetchCredits
--- PASS: TestFetchCredits (0.00s)
=== RUN   TestFetchCreditsTimeout
--- PASS: TestFetchCreditsTimeout (5.00s)
=== RUN   TestFetchCreditsNoCreds
--- PASS: TestFetchCreditsNoCreds (0.00s)
=== RUN   TestLoadCachedCreditsMissing
--- PASS: TestLoadCachedCreditsMissing (0.00s)
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
sworn notify: webhook POST attempt 1/3: Post "http://127.0.0.1:39063": context deadline exceeded
--- PASS: TestNotifyWebhook_TimeoutContext (0.10s)
=== RUN   TestViolationsSummary_FromFile
--- PASS: TestViolationsSummary_FromFile (0.00s)
=== RUN   TestViolationsSummary_Truncation
--- PASS: TestViolationsSummary_Truncation (0.00s)
=== RUN   TestNotifyEvent_JSONShape
--- PASS: TestNotifyEvent_JSONShape (0.00s)
=== RUN   TestNotify_TimestampDefault
--- PASS: TestNotify_TimestampDefault (0.00s)
=== RUN   TestProxyEndpointWithCreds
--- PASS: TestProxyEndpointWithCreds (0.00s)
=== RUN   TestProxyEndpointNoCreds
--- PASS: TestProxyEndpointNoCreds (0.00s)
=== RUN   TestProxyEndpointOverrideWarns
warning: SWORN_PROXY_URL is set — sworn credentials will be routed to http://localhost:9999 (non-default host)
--- PASS: TestProxyEndpointOverrideWarns (0.00s)
=== RUN   TestProxyEndpointModelIDEscaped
--- PASS: TestProxyEndpointModelIDEscaped (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/account	10.125s
```

### `go test ./internal/run/... -v -count=1`

```
=== RUN   TestExtractFrontmatter
=== RUN   TestExtractFrontmatter/simple_frontmatter
=== RUN   TestExtractFrontmatter/no_frontmatter
=== RUN   TestExtractFrontmatter/empty_frontmatter
=== RUN   TestExtractFrontmatter/trailing_whitespace_on_---_lines
=== RUN   TestExtractFrontmatter/single_line_(too_short)
--- PASS: TestExtractFrontmatter (0.00s)
    --- PASS: TestExtractFrontmatter/simple_frontmatter (0.00s)
    --- PASS: TestExtractFrontmatter/no_frontmatter (0.00s)
    --- PASS: TestExtractFrontmatter/empty_frontmatter (0.00s)
    --- PASS: TestExtractFrontmatter/trailing_whitespace_on_---_lines (0.00s)
    --- PASS: TestExtractFrontmatter/single_line_(too_short) (0.00s)
=== RUN   TestExtractReleaseWorktreePath
=== RUN   TestExtractReleaseWorktreePath/simple_path
=== RUN   TestExtractReleaseWorktreePath/no_path
=== RUN   TestExtractReleaseWorktreePath/quoted_path
--- PASS: TestExtractReleaseWorktreePath (0.00s)
    --- PASS: TestExtractReleaseWorktreePath/simple_path (0.00s)
    --- PASS: TestExtractReleaseWorktreePath/no_path (0.00s)
    --- PASS: TestExtractReleaseWorktreePath/quoted_path (0.00s)
=== RUN   TestDirExists
--- PASS: TestDirExists (0.00s)
=== RUN   TestRunParallel_Basic
--- PASS: TestRunParallel_Basic (0.00s)
=== RUN   TestRunParallel_ReleaseWorktreePathMissing
--- PASS: TestRunParallel_ReleaseWorktreePathMissing (0.00s)
=== RUN   TestRunParallel_NoTracks
--- PASS: TestRunParallel_NoTracks (0.00s)
=== RUN   TestRunParallel_MissingIndex
--- PASS: TestRunParallel_MissingIndex (0.00s)
=== RUN   TestRunParallel_FailureCascade
--- PASS: TestRunParallel_FailureCascade (0.00s)
=== RUN   TestRunParallel_TimingConcurrency
--- PASS: TestRunParallel_TimingConcurrency (0.00s)
=== RUN   TestRunParallel_DependentTrackRunsAfterSuccess
--- PASS: TestRunParallel_DependentTrackRunsAfterSuccess (0.00s)
=== RUN   TestRun_PassPath_Merges
--- PASS: TestRun_PassPath_Merges (0.14s)
=== RUN   TestRun_FailPath_NoMerge
--- PASS: TestRun_FailPath_NoMerge (0.17s)
=== RUN   TestRun_FailThenPass_RetrySucceeds
--- PASS: TestRun_FailThenPass_RetrySucceeds (0.17s)
=== RUN   TestRun_Blocked_StopsImmediately
--- PASS: TestRun_Blocked_StopsImmediately (0.10s)
=== RUN   TestSanitiseBranch
--- PASS: TestSanitiseBranch (0.00s)
=== RUN   TestRun_MissingTask
--- PASS: TestRun_MissingTask (0.00s)
=== RUN   TestRun_VerifyMarkdownPass
--- PASS: TestRun_VerifyMarkdownPass (0.14s)
=== RUN   TestRun_VerifyStatelessPromptWired
--- PASS: TestRun_VerifyStatelessPromptWired (0.13s)
=== RUN   TestRun_VerifyToolCallLeakBlocks
--- PASS: TestRun_VerifyToolCallLeakBlocks (0.09s)
=== RUN   TestRunSlice
--- PASS: TestRunSlice (0.04s)
=== RUN   TestRunSliceFail
--- PASS: TestRunSliceFail (0.07s)
=== RUN   TestRunSlice_MissingVerifierModel
--- PASS: TestRunSlice_MissingVerifierModel (0.03s)
=== RUN   TestRunSlice_FailNotifiesOnce
--- PASS: TestRunSlice_FailNotifiesOnce (0.07s)
=== RUN   TestRunSlice_BlockedNotifies
--- PASS: TestRunSlice_BlockedNotifies (0.04s)
=== RUN   TestRunSlice_NilNotifierNoOp
--- PASS: TestRunSlice_NilNotifierNoOp (0.05s)
PASS
ok  	github.com/swornagent/sworn/internal/run	1.259s
```

The three new S07-paging integration tests (`TestRunSlice_FailNotifiesOnce`, `TestRunSlice_BlockedNotifies`, `TestRunSlice_NilNotifierNoOp`) exercise the FAIL→Notify and BLOCKED→Notify wiring in `slice.go` through the `RunSlice` integration point via the `run.Notifier` interface seam.

### `go test ./internal/scheduler/... -v -count=1`

```
=== RUN   TestBuildPlan_TwoIndependentTracks
--- PASS: TestBuildPlan_TwoIndependentTracks (0.00s)
=== RUN   TestBuildPlan_DependencyOrdering
--- PASS: TestBuildPlan_DependencyOrdering (0.00s)
=== RUN   TestBuildPlan_FailurePropagation
--- PASS: TestBuildPlan_FailurePropagation (0.00s)
=== RUN   TestBuildPlan_AllSucceed
--- PASS: TestBuildPlan_AllSucceed (0.00s)
=== RUN   TestBuildPlan_NonExistentDep
--- PASS: TestBuildPlan_NonExistentDep (0.00s)
=== RUN   TestBuildPlan_CycleDetection
--- PASS: TestBuildPlan_CycleDetection (0.00s)
=== RUN   TestBuildPlan_MultiDependency
--- PASS: TestBuildPlan_MultiDependency (0.00s)
=== RUN   TestBuildPlan_Empty
--- PASS: TestBuildPlan_Empty (0.00s)
=== RUN   TestRunTrack_AllSlicesPass
--- PASS: TestRunTrack_AllSlicesPass (0.00s)
=== RUN   TestRunTrack_ContextCancelled
--- PASS: TestRunTrack_ContextCancelled (0.00s)
=== RUN   TestRunTrack_SliceFail
--- PASS: TestRunTrack_SliceFail (0.00s)
=== RUN   TestRunTrack_MultiSliceOrdering
--- PASS: TestRunTrack_MultiSliceOrdering (0.00s)
=== RUN   TestRunTrack_MaterialisesWorktree
--- PASS: TestRunTrack_MaterialisesWorktree (0.00s)
=== RUN   TestRunTrack_EmptySlices
--- PASS: TestRunTrack_EmptySlices (0.00s)
PASS
ok  	github.com/swornagent/sworn/internal/scheduler	0.018s
```

### `go test ./cmd/sworn/... -v -count=1` (S07 account CLI round-trip + registry tests — key subset)

```
=== RUN   TestAccountSetWebhookThenNotifications
Webhook URL set to: https://hooks.example.com/sworn
--- PASS: TestAccountSetWebhookThenNotifications (0.00s)
=== RUN   TestAccountSetWebhook_PersistsAcrossLoad
Webhook URL set to: https://hooks.example.com/sworn-2
--- PASS: TestAccountSetWebhook_PersistsAcrossLoad (0.00s)
=== RUN   TestAccountNotifications_NoWebhook
--- PASS: TestAccountNotifications_NoWebhook (0.00s)
=== RUN   TestAccountSetWebhook_MissingURL
Usage: sworn account set-webhook <url>
  url = webhook endpoint to POST notifications to
--- PASS: TestAccountSetWebhook_MissingURL (0.00s)
=== RUN   TestEveryVerbResolves
--- PASS: TestEveryVerbResolves (0.00s)
=== RUN   TestUnknownVerbNotFound
--- PASS: TestUnknownVerbNotFound (0.00s)
=== RUN   TestAllCommandsHaveNonEmptySummary
--- PASS: TestAllCommandsHaveNonEmptySummary (0.00s)
=== RUN   TestVersionAndHelpAliasesShareHandlers
--- PASS: TestVersionAndHelpAliasesShareHandlers (0.00s)
=== RUN   TestDispatchResolves
--- PASS: TestDispatchResolves (0.00s)
PASS
ok  	github.com/swornagent/sworn/cmd/sworn	0.326s
```

The four new S07-paging CLI tests in `cmd/sworn/account_test.go` drive `cmdAccountSetWebhook` + `cmdAccountNotifications` through their CLI entry functions and assert the WebhookURL → Save → Load → stdout round-trip (AC5). `TestEveryVerbResolves` covers 26 verbs (23 from release-wt + `account`, `login`, `logout` from T3). All resolve with non-empty Summary and non-nil Run.

### `go vet ./...`

```
(clean — no output, exit 0)
```

### `gofmt -l` (changed files)

```
(no output — all changed files formatted)
```

## Reachability artefact

**Live webhook.site smoke test (2026-06-22):**
- Webhook.site URL: `https://webhook.site/e79d3ba0-435e-473d-8d0e-c3d8209bcae2`
- POST sent with exact Notifier JSON payload: `{release: "2026-06-19-safe-parallelism", track: "T3-commercial", slice_id: "S07-paging", state: "failed_verification", violations_summary: "1. Missing reachability artefact in proof bundle", worktree_path: "/home/brad/projects/sworn-worktrees/release-2026-06-19-safe-parallelism-T3-commercial", timestamp: "2026-06-22T03:44:39Z"}`
- Webhook.site received the request (UUID: `38181bb4-533e-45c9-9d75-8c5338aa9dd4`, IP: 220.245.209.43, method: POST, size: 375 bytes, content-type: application/json)
- Verified via `GET https://webhook.site/token/e79d3ba0-435e-473d-8d0e-c3d8209bcae2/requests?sorting=newest` — response confirms exact payload match

**Mock server reachability (unit tests — current run):**
- `TestNotifyWebhook`: mock HTTP server receives correct JSON payload with all 7 fields
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
- [x] Notifier threaded through single-slice `run.Run()` path — `run.Options.Notifier` → `RunSliceOptions.Notifier`
- [x] Notifier threaded through parallel `RunParallel()` path — CLI creates notifier before mode dispatch, shared by both paths
- [x] **Integration test — FAIL→Notify** (`internal/run/run_test.go::TestRunSlice_FailNotifiesOnce`) — injects a failing `fakeVerifier` + recording `fakeNotifier` via the `run.Notifier` interface seam; asserts `Notify` called exactly once with `State == "failed_verification"`, `SliceID == "S01-task"`, `Release == "test-release"`, and non-empty `ViolationsSummary` (spec Required tests → Integration; AC1)
- [x] **Integration test — BLOCKED→Notify** (`internal/run/run_test.go::TestRunSlice_BlockedNotifies`) — injects a BLOCKED `fakeVerifier`; asserts `Notify` called exactly once with `State == "blocked"` and the correct `SliceID` (covers the second wiring the verifier cited at slice.go:222-239)
- [x] **Integration test — nil-notifier no-op** (`internal/run/run_test.go::TestRunSlice_NilNotifierNoOp`) — guards the nil-notifier production path does not panic
- [x] **CLI round-trip test — set-webhook/notifications** (`cmd/sworn/account_test.go::TestAccountSetWebhookThenNotifications`) — drives `cmdAccountSetWebhook` then `cmdAccountNotifications` via their CLI entry functions with an isolated `XDG_CONFIG_HOME` tmpdir; asserts exit 0, on-disk `WebhookURL` round-trip (Save → Load), and the URL appears in captured stdout (AC5)
- [x] **CLI persistence test** (`cmd/sworn/account_test.go::TestAccountSetWebhook_PersistsAcrossLoad`) — asserts the `webhook_url` JSON key survives Save → raw-file read
- [x] Testability seam — `run.Notifier` one-method interface in `internal/run/slice.go` so tests inject a recording fake without a live `*account.Notifier`; production `*account.Notifier` satisfies it implicitly
- [x] Unit + integration tests (27 account, 24 run, 14 scheduler, 9+ cmd/sworn account/registry): all PASS

## Not delivered

- **Email via SwornAgent API (`/api/notify` endpoint):** The client-side POST to `<host>/api/notify` is implemented and tested with a mock server (`TestNotifyWithAccount`). The server-side endpoint does not exist yet — the client logs a warning if unreachable. **Acknowledged**: Coach, 2026-06-22. Tracking: SwornAgent backend backlog.

## Divergence from plan

- **Pin 1:** `planned_files` updated: `internal/run/run.go` → `internal/run/slice.go` (S02a refactor moved `RunSlice`). Added `internal/account/account.go` (WebhookURL field on Credentials).
- **Pin 2 (Option a):** BLOCKED notification fires at `slice.go:222` (before error return) with `state: "blocked"`. FAIL notification fires after `failed_verification` state write at `slice.go:265`. Track-fail notification in `worker.go` covers unexpected/non-verdict errors.
- **Pin 3 (Coach acked):** Live webhook.site smoke test performed in addition to mock-server unit tests.
- **Re-entry (2026-07-01):** Single-slice `run.Run()` path was not wired for notifications (only `RunParallel` was). Added `Notifier` field to `run.Options`, threaded through to `RunSlice`, and hoisted notifier creation in `cmd/sworn/run.go` to serve both modes.
- **Re-entry (2026-07-01 #2):** Proof bundle refreshed from live repo state. Tests re-run: all 62 pass, `go vet` clean. No code changes needed.
- **Forward-merge convergence (2026-06-22):** The journal's prescribed Step 0 — forward-merge `release-wt/2026-06-19-safe-parallelism` to bring in S51-cli-command-registry, resolving the stale BLOCKED on `cmd/sworn/main.go`. T3's `login`/`logout`/`account` switch cases converted to `command.Register(...)` calls via `init()` in `cmd/sworn/login.go` and `cmd/sworn/account.go`. release-wt's `main.go` (registry-based dispatch) adopted; release-wt's `verify.go`, `commands.go`, `commands_test.go`, `internal/command/registry.go` brought in via merge. `expectedVerbs` in `commands_test.go` extended with `account`, `login`, `logout`. No S07 feature code touched. This was not a spec divergence — it was the journal-prescribed convergence task.
- **Testability seam (2026-07-01, verifier-FAIL remediation):** Added a one-method `run.Notifier` interface in `internal/run/slice.go` (consumer package) and changed `RunSliceOptions.Notifier` from concrete `*account.Notifier` to the interface. Rationale: the spec's Required tests → Integration line demands an `internal/run/` integration test that injects a failing mock verifier and asserts `notifier.Notify` is called — without a seam in the consumer package, the test cannot supply a recording fake without a live `*account.Notifier` + httptest server. `*account.Notifier` satisfies the interface implicitly (same `Notify(ctx, account.NotifyEvent)` signature), so production behaviour is unchanged; `run.Options.Notifier` and `RunParallelOptions.Notifier` remain typed `*account.Notifier` and assign into the interface field at the call site. This is a testability refactor, not a contract change.

## First-pass script output

```
release-verify.sh
  slice:       S07-paging
  slice dir:   docs/release/2026-06-19-safe-parallelism/S07-paging
  base branch: main

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present
  PASS  spec.md has Required tests section

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Integration branch drift ==
  integration branch: release/v0.1.0
  WARNING: worktree is 1 commit(s) behind release/v0.1.0 (no test-infra overlap)
  upstream commits not yet absorbed:
    5512f73 docs(captures): record the cmd/sworn/main.go touchpoint-collision coach-loop pause
  PASS  integration branch drift present but does not affect test infrastructure

== Diff vs start_commit (verifier base) ==
  diff base: start_commit a7681c4f2efa8aa31c52a750674c026984f18670
  PASS  69 file(s) changed vs diff base

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md
  PASS  proof.md 'Not delivered' deferrals carry non-placeholder tracking refs
  PASS  proof.md 'Files changed' count (~70) consistent with diff vs start_commit (69)

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== Test results section scope ==
  PASS  Test results section contains no Playwright runner output (Jest/Vitest scope confirmed)

== First-pass verdict ==
  checks passed: 23
  checks failed: 0

FIRST-PASS PASS
```