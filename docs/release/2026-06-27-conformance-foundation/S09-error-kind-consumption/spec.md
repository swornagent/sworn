---
title: 'S09 — Error{Kind} consumption: terminal kinds halt the loop'
description: 'Fix the slice runner to detect model.IsTerminal() errors and surface them as a BLOCKED verdict before the triage retry loop, preventing KindAuth/KindCredits from being retried+escalated like transient failures.'
---

# Slice: `S09-error-kind-consumption`

## User outcome

When a model dispatch returns a KindAuth or KindCredits error (credentials rejected, credits exhausted), the loop immediately halts that track with a meaningful message ("auth failure — check credentials" or "credits exhausted") rather than retrying the dispatch or escalating to the next model, which silently burns the escalation budget and produces confusing failure logs.

## Entry point

`sworn run --release <name>` → `internal/run/slice.go` lines ~321-327 (the error-handling path after a model dispatch returns an error, per audit ref).

## In scope

- `internal/run/slice.go`: in the dispatch error path (around line 321), call `model.IsTerminal(err)` before constructing the verdict; if true, return a terminal-blocked verdict with a descriptive reason string (e.g. `"KindAuth: credentials rejected — halting; check provider API key"`) instead of passing through to the retry/escalation path
- `internal/model/errors.go`: rename `ErrDriverNotRegistered` sentinel to a more accurate name if warranted (audit: "self-registering factory (sworn#15) or rename the sentinel"); this is a small mechanical rename with a matching `errors.As` update in callers
- `internal/run/slice_test.go` or new `internal/run/slice_terminal_test.go`: test the terminal-halt path

## Out of scope

- Changes to `internal/orchestrator/triage.go` — the halt on terminal errors happens in the slice runner (slice.go) BEFORE triage is called, so triage does not need an Err field for this slice
- Retry policy configuration (KindRateLimit backoff) — separate concern
- Consuming terminal kinds in the orchestrator or worker (the slice runner halts before returning to the worker, so the worker sees a BLOCKED verdict, which already halts via `triage.Decide`)
- Adding new ErrorKind values (the existing taxonomy is correct)

## Planned touchpoints

- `internal/run/slice.go` (lines ~321-327, error-handling path — documented shared file: T2 owns this region; T3 S11 owns lines ~412-429)
- `internal/model/errors.go` (rename ErrDriverNotRegistered if needed)

## Acceptance checks

- [ ] WHEN a model dispatch in `internal/run/slice.go` returns an error where `model.IsTerminal(err)` returns true, THE SYSTEM SHALL return a BLOCKED verdict with reason string containing "KindAuth" or "KindCredits" (depending on the kind) before entering the triage/retry path
- [ ] WHEN a model dispatch returns a KindRateLimit error (non-terminal), THE SYSTEM SHALL NOT apply the terminal halt; the error continues through the existing retry/triage path
- [ ] IF `model.IsTerminal(err)` returns false for nil (no error), THE SYSTEM SHALL NOT alter the current success path
- [ ] `ErrDriverNotRegistered` (or its renamed form) is used consistently in the codebase — `grep -rn "ErrDriverNotRegistered"` returns the same symbol everywhere
- [ ] `slice_terminal_test.go` covers: KindAuth dispatch error → BLOCKED verdict returned; KindCredits dispatch error → BLOCKED verdict returned; KindRateLimit dispatch error → not BLOCKED (passes through)

## Required tests

- **Unit**: `internal/run/slice_terminal_test.go` (new) — table-driven, mock dispatch returning each error kind
- **Reachability artefact**: `go test ./internal/run/... -v -run TestTerminalError` exits 0

## Risks

- `slice.go` is a documented shared file (T2: lines ~321-327; T3: lines ~412-429); the implementer must restrict edits to the error-handling section only and surface any scope bleed immediately

## Deferrals allowed?

No.
