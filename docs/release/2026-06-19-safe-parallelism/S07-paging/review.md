# Captain review — S07-paging
Date: 2026-06-22
Design commit: 5a9f88aa483d82b52a23e019ce89f4ee14c9ed57

## Pins

1. [mechanical] §3.file — spec touchpoint names `run.go`; implementation must target `slice.go`
   What I observed: `spec.md` planned touchpoints list `internal/run/run.go (touch — call notifier on FAIL/BLOCKED)`. Design §3 correctly identifies `internal/run/slice.go` as the call site (after the `failed_verification` transition at line ~241). S02a-run-refactor commit `3cefd09` moved `RunSlice()` — and the `failed_verification` transition — out of `run.go` into `slice.go`. The status.json `planned_files` field also lists `internal/run/run.go`, which is stale. `run.go` is not touched in this slice.
   What to ask the implementer: Before transitioning to in_progress, update `status.json` `planned_files` to replace `internal/run/run.go` with `internal/run/slice.go`. Proceed with `slice.go` throughout — confirm with a grep that `run.go` has no residual FAIL-handling code that needs a parallel call.

2. [mechanical] §3.call — BLOCKED notification call site and payload `state` value are unspecified
   What I observed: `slice.go:218` returns `fmt.Errorf("RunSlice: verification blocked: ...")` without a state transition (confirmed by comment at line 59: "On verifier BLOCKED: returns error immediately (no state change)"). Design §3 places the FAIL notify call at `slice.go:~241` (the `failed_verification` transition) and the track-fail call at `worker.go:~143`. BLOCKED will fall through `worker.go`'s error handler as a track-fail event, NOT trigger the `slice.go` notify. The spec says "on FAIL/BLOCKED verdict, call `notifier.Notify(ctx, event)` after writing state to status.json" — but for BLOCKED there is no state write. The `state` payload field value for BLOCKED is unspecified (spec AC only defines `state = "failed_verification"` for FAIL).
   What to ask the implementer: Before writing code, confirm the two-path decision: (a) Add a `notifier.Notify()` call at `slice.go:218` (the BLOCKED return path) with `state: "blocked"`, in addition to the `worker.go` track-fail call? Or (b) rely solely on `worker.go`'s track-fail notify for BLOCKED, accepting that BLOCKED fires as a track event (not a slice event) with the track-level payload? Document the chosen path as a §2 amendment in design.md. If (b), confirm what `state` value appears in the `worker.go` notification payload for a BLOCKED-caused track failure.

3. [escalate] §5 — design substitutes mock-server tests for the spec's required live webhook.site smoke step
   What I observed: Spec's "Required tests" section requires: "Reachability artefact: smoke step — `sworn account set-webhook https://webhook.site/<id>`; run a fixture release designed to FAIL; confirm webhook.site receives the notification. Document the webhook.site URL and received payload in proof.md." Design §5 explicitly replaces this with mock-server tests: "A live webhook.site test is environment-dependent and not reproducible in CI; the mock server test is stronger evidence."
   What to ask the implementer: This is a Coach authority call. Option (a): accept mock-server tests as the reachability artefact for this slice — update spec via `/replan-release` to remove the webhook.site requirement. Option (b): require the live webhook.site smoke step in addition to the mock-server tests. The implementer must not proceed until Coach acks one of these options.

4. [memory-cited] §2.4 — SwornAgent notify URL via `defaultProxyHost` confirmed against project memory
   What I observed: Design §2 decision 4 derives the SwornAgent `/api/notify` URL from `defaultProxyHost` (same as `FetchCredits` at `account.go:272-273`). This matches the pattern validated by the Coach on 2026-06-21 when OpenRouter routing confirmed the proxy-first architecture.
   What to ask the implementer: No action needed. Pattern is confirmed. [[project_aggregator_proxy_validation]] ack.
   Citation: [[project_aggregator_proxy_validation]]

Pins: 4 total — 2 [mechanical], 1 [memory-cited], 1 [escalate]
Critical pins: Pin 2 (BLOCKED slices may silently not notify or notify with incorrect payload if call site is unresolved), Pin 3 (spec compliance — verifier will check the proof bundle's reachability artefact type).

## Summary

4 pins — 2 mechanical, 1 memory-cited, 1 escalate. Pin 3 ([escalate]) is the gate: Coach must choose between accepting mock-server tests as the reachability artefact or requiring the live webhook.site smoke step before code is written.

## Smaller flags (not pins, worth one-line ack)

(a) **Drift gate**: track worktree is behind `release-wt/2026-06-19-safe-parallelism` by 1 commit (`2027e32` — S10/S44 provider-error taxonomy replan). The commit does not touch any S07 files; the Captain proceeded despite the strict drift-gate BLOCKED rule. Forward-merge before transitioning to `in_progress`.

(b) **`http.Client` injection decision walked back**: Design §2 decision 2 first proposes an injectable `http.Client` field on `Notifier`, then reverses to `http.DefaultClient` (citing `httptest.Server` as sufficient). The reversal is fine — no injectable field needed — but the design leaves the original proposal partially in the description. A tidy-up to decision 2's text would remove the ambiguity before a reader has to parse both halves.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR solid core with two call-site gaps to resolve inline and one spec deviation that Coach has now acked. 4 pins + 2 flags:

1. **`run.go` → `slice.go` in planned_files.** Before in_progress, update `status.json` `planned_files`: replace `internal/run/run.go` with `internal/run/slice.go`. Touch `slice.go` throughout — `run.go` is not touched by this slice.

2. **BLOCKED call site + payload state.** Decide before writing code: (a) add `notifier.Notify()` at `slice.go:218` (BLOCKED return path, `state: "blocked"`) in addition to the `worker.go` track-fail call; or (b) rely on `worker.go` track-fail notify for BLOCKED only, with documented `state` value in the payload. Record the choice as an amendment to design §3.

3. **Reachability artefact: [COACH ACK REQUIRED — see Pin 3].** Coach will insert the accepted option here.

4. **`defaultProxyHost` pattern ack.** Design §2 decision 4 matches `FetchCredits` pattern. Proceed.

Flags: (a) forward-merge from `release-wt/2026-06-19-safe-parallelism` before in_progress; (b) tighten design §2 decision 2 wording to remove the walked-back injectable-client proposal.

§2 decisions 1, 2 (revised), 3, 5 ack. §6 no open questions ack.

Address pins 1–2 inline during implementation. Pin 3 is Coach-resolved before you proceed. Then transition to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: NEEDS_COACH
CONSTITUTIONAL: no
REASON: Pin 3 is a spec-stated reachability requirement the design explicitly overrides — only Coach can ack the substitution or require the live webhook.site step before code is written.
-->
