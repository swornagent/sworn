---
title: 'S52-ledger-projection — capture every verifier verdict into an append-only verdict ledger'
description: 'Adds internal/ledger: a Record schema plus a pure projection from each slice''s status.json verification block into an append-only docs/ledger/verdicts.jsonl. Records implementer model + attempt at the verdict-record site so the corpus can answer model-vs-outcome questions. Backfills the whole existing board on first sync.'
---

# Slice: `S52-ledger-projection`

## User outcome

A maintainer runs the sync (delivered as the CLI in S53, but the engine lands here) and the
repo gains a durable, git-tracked `docs/ledger/verdicts.jsonl` with one line per slice
verdict — every PASS / FAIL / BLOCKED the board has ever recorded, including the resolved
implementer model and attempt number. The accumulated record of "what good looks like in
this repo" stops being scattered across per-slice `status.json` blocks and becomes one
queryable corpus.

## Entry point

`ledger.Project(status *state.Status, gateCount int) (Record, bool)` and
`ledger.Append(path string, r Record) error` in a new `internal/ledger` package, exercised
through the package's own integration test today and wired to the `sworn ledger sync`
command in S53. The verdict-record site in `internal/run/slice.go` is extended to persist
`verification.model` and `verification.attempt` so the projection has model-vs-outcome data
to read.

## Background

Every field a verdict ledger needs is already produced by the harness, just thrown away
after the slice closes:

- `release`, `track`, `slice_id`, `state` — `status.json` top level.
- `verdict`, `verifier_was_fresh_context`, `verifier_session_id`, `violations[]` —
  `status.json` `verification.*` (structured violations land via S38).
- `gate_count` — count of `- [ ]` acceptance checks in `spec.md`.

The one thing `status.json` does **not** record is which implementer model produced the
artefact under test, or which escalation attempt it was — yet `internal/run/slice.go`
already advances `escalationModels[attempt]` in its loop, so the data exists at runtime and
is merely not persisted. Without it, S54's history-backed routing has nothing to route on.
This slice persists those two fields and harvests the rest by projection.

## In scope

- New `internal/ledger/ledger.go`:
  - `type Record struct` with JSON tags, schema field `V int` (=1), and:
    `Ts, Release, Track, SliceID, SliceKind, Role, Model string`, `Attempt int`,
    `Verdict, State string`, `FreshContext *bool`, `VerifierSessionID string`,
    `Violations []state.Violation` (or `[]string` pre-S38 — match `state.Verification`),
    `GateCount, ViolationCount int`, `SwornVersion string`.
  - `SliceKind(track string) string` — derives the rubric dimension from the track id
    (e.g. `T5-providers`→`provider`, `T12-harness-hardening`→`harness`, `T8-memory`→
    `memory`, `T3-commercial`→`commercial`, `T16-verdict-ledger`→`ledger`), default
    `other`. No edits to existing `status.json` files — the kind is derived, not stored.
  - `Project(status *state.Status, gateCount int) (Record, bool)` — pure; returns `ok=false`
    for a slice with no terminal verdict (`verification.result` empty / `pending`).
  - `Append(path string, r Record) error` — append-only JSONL writer; creates the file and
    parent dir if absent.
  - `Key(r Record) string` and an idempotency guard so re-syncing the same verdict does not
    duplicate a line (dedupe by `slice_id` + `verdict` + `ts`).
- Extend `state.Verification` (in `internal/state/state.go`) with `Model string` and
  `Attempt int` (both `omitempty`), and populate them at the verdict-record site in
  `internal/run/slice.go` from the escalation loop's current model + attempt index.
- `docs/ledger/verdicts.jsonl` is the canonical, git-tracked corpus path (repo-level, not
  release-scoped, because the projection spans every release's board).

## Out of scope

- The `sworn ledger` CLI (`sync`, `report`) — **S53**. This slice ships the library only.
- Aggregation / pass-rate reporting — **S53**.
- Routing recommendations and the config-resolver wire — **S54**.
- Capturing the **verifier** model (as opposed to the implementer model) — deferred (Rule 2).
  Why: the verdict under measurement is the implementer's output; verifier-model capture is
  a second axis with its own value, not needed for S54 routing. Tracking: future-release
  ledger follow-up. Ack: Brad, 2026-06-22.
- Emitting to the remote S26 telemetry endpoint — explicitly **not** this. The ledger is the
  private, identified corpus; S26 is anonymous and path-scrubbed by design.

## Planned touchpoints

- `internal/ledger/ledger.go` (new)
- `internal/ledger/ledger_test.go` (new)
- `internal/state/state.go` (modify — add `Model`, `Attempt` to `Verification`)
- `internal/state/state_test.go` (modify — cover the new fields round-trip)
- `internal/run/slice.go` (modify — persist model + attempt at the verdict-record site;
  T13/S47 owns this file's verdict switch, hence T16 depends on T13)

## Acceptance checks

- [ ] `ledger.Project` on a status with `verification.result: "fail"` returns a `Record`
  with `Verdict=="fail"`, the slice's `release`/`track`/`slice_id`, `SliceKind` derived from
  the track, `GateCount` equal to the count of `- [ ]` lines passed in, and `ok==true`
- [ ] `ledger.Project` on a `planned` slice with empty `verification.result` returns
  `ok==false` (no record emitted for un-verified slices)
- [ ] `ledger.Append` writes exactly one JSON object per line; appending N records yields N
  lines; the file and `docs/ledger/` are created if absent
- [ ] Appending a record whose `Key` already exists in the file is a no-op (idempotent
  re-sync); a second sync of an unchanged board adds zero lines
- [ ] `state.Verification` round-trips `Model` and `Attempt` through `state.Write`/`state.Read`;
  both are omitted from JSON when zero-valued (`omitempty`)
- [ ] After a slice's verdict is recorded through `internal/run/slice.go`, its `status.json`
  `verification.model` and `verification.attempt` reflect the model + attempt index the
  escalation loop used (asserted via the run package's existing verdict-path test)
- [ ] `go test ./internal/ledger/... ./internal/state/... ./internal/run/...` passes; no new
  external deps in `go.mod` (`go build ./...` succeeds without `go get`)

## Required tests

- **Unit**: `internal/ledger/ledger_test.go` — table-driven `Project` (pass/fail/blocked/
  pending), `SliceKind` mapping, `Append` line-count + idempotency on a temp file.
- **Integration**: `internal/run/slice.go`'s verdict path test (in `internal/run`) asserting
  `verification.model` / `verification.attempt` are persisted — this is the Rule 1
  reachability point: the fields are proven through the loop that owns them, not just the
  leaf struct.
- **Reachability artefact**: in `proof.md`, paste a `Project`→`Append` round-trip producing
  a real `verdicts.jsonl` line from a fixture `status.json`, plus the `go test` output.

## Risks

- The verdict-record site in `internal/run/slice.go` is rewritten by S47 (T13 triage call)
  and touched by S42–S44 (T12). T16 depends on T6 + T12 + T13 precisely so this slice edits
  the **settled** verdict path, not a moving one. If the triage refactor moves the model/
  attempt knowledge out of `slice.go`, persist from wherever the post-S47 verdict outcome is
  applied (e.g. `internal/orchestrator/triage.go`) and note the relocation.
- `violations` type depends on S38 (`[]string` → structured). Match `state.Verification`'s
  actual type at implementation time; do not hard-code the pre-S38 shape.

## Deferrals allowed?

Yes, with Rule 2 compliance — verifier-model capture and remote emission are surfaced above
with why / tracking / ack. Anything else carved out mid-implementation requires the same.
