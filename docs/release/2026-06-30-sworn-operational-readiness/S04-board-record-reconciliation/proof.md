---
title: 'Proof bundle — S04-board-record-reconciliation'
description: 'Oracle reads the canonical coach board.json (release object form), tolerating the legacy string; round-trip preserves the object fields. Implemented, not yet Rule-7 verified.'
date: 2026-07-01
---

# Proof bundle — S04-board-record-reconciliation

## Scope
Make sworn read a real coach-produced board.json whose `release` is the canonical baton
OBJECT form ({name, vertical_trace, ...}) without an unmarshal error (tolerating the legacy
string), so the oracle/run load a real release instead of failing at board-read. The
board-level companion to D6.

## Files changed
`git diff --name-only release-wt/… HEAD` (production):
```
internal/board/board.go            (Release type: tolerant Unmarshal + fidelity Marshal; BoardRecord.Release; migrate uses StringRelease)
internal/board/board_test.go       (existing tests updated for the Release type)
internal/board/board_release_test.go (NEW — object/string/missing-name/round-trip tests)
internal/baton/validator.go        (board-v1 release accepts string OR object-with-name)
internal/baton/schemas/board-v1.json (release oneOf string|{name} — accepts canonical, back-compat string)
```
cmd/sworn/board.go was briefly edited then reverted — `bs.Release` there is `BoardState.Release`
(an already-resolved string), not `BoardRecord.Release`; no change needed.

## Test results
- `go build ./...` → exit 0
- `go test ./internal/board/... ./internal/baton/...` → ok (both)
- `go test ./...` (full suite) → all green (no FAIL)

## Reachability artefact (Rule 1 — the real integration point)
`sworn board --release 2026-06-28-yearSnapshot-schema-cleanup` run against the LIVE consumer repo
(`~/projects/fired`) — the exact command that previously failed with
`cannot unmarshal object into BoardRecord.release of type string` — now reads the real
coach-produced (object-form) board:
```
Release board: 2026-06-28-yearSnapshot-schema-cleanup
Track T1-schema-cleanup — in_progress
    S01-networth-hierarchy-remap — planned [human]
    S02-rename-field-families — planned [human]
    S03-other-asset-appreciation-fields — planned [human]
```
Binary built from this track (`/tmp/sworn-s04`), run from `~/projects/fired`. No parse error; exit 0.

## Delivered
- AC-01 (object form reads): `Release.UnmarshalJSON` object branch + `TestRelease_ObjectForm`; proven live against fired.
- AC-02 (legacy string still reads): string branch + `TestRelease_StringForm`.
- AC-03 (object missing name fails closed): `TestRelease_ObjectMissingName_FailsClosed`.
- AC-04 (embedded schema reconciled): board-v1.json `release` is `oneOf [string, {name}]`.
- AC-05 (reachability against fired): the live `sworn board` output above.
- AC-06 (build/tests): full suite green.
- AC-07 (round-trip fidelity): `Release.raw` preserved + re-emitted; `TestRelease_RoundTripPreservesObjectFields` asserts vertical_trace/target_version survive; `TestRelease_StringRoundTripsAsString` for the string form.

## Not delivered (Rule 2)
- **Full baton re-vendor of ALL embedded schemas** (status/spec/proof/etc. to canonical). Why:
  out of scope — this slice is the board-read unblock only; the broader pin skew (binary on
  Baton v0.6.3) is a separate follow-up. Tracking: the baton vendor-pin work (#26 vicinity) +
  the operational-readiness intake. Acknowledged 2026-07-01.
- **Revert THIS release's own board.json to canonical nested**: deferred until S04 lands on the
  running binary (it is string-form right now only to be readable by the stale binary). Tracking:
  intake decision "2026-07-01 — Add S04 (direction reversal)". Acknowledged.

## Divergence from plan
- Spec listed `cmd/sworn/board.go` as a touchpoint; in practice that file reads `BoardState.Release`
  (a resolved string), not `BoardRecord.Release`, so it needed no change. Added
  `internal/baton/validator.go` (not in the original touchpoint list) because `WriteBoard`
  validates via the legacy validator, which required `release` to be a string — relaxed to accept
  the object so a write-back of a canonical board passes. Net touchpoints: board.go, board_test.go,
  board_release_test.go, validator.go, board-v1.json (schema). Surfaced here, not silent.
