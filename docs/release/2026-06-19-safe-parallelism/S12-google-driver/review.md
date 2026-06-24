# Captain review — S12-google-driver
Date: 2026-07-09
Design commit: 3f04228c12fd645da6e94ff00061e70639c61767
Review round: 2 (revised design — "addresses Captain review pins 1–7")

## Pins

1. [mechanical] §3 / status.json — `internal/model/config.go` missing from `planned_files`.
   What I observed: The design §3 correctly lists `internal/model/config.go` under the "Production dispatch" group (for `swornProviderConfig()` GoogleKey envOrAlias change, `FromEnv()` vertex key-gate bypass, and `FromEnv()` google envOrAlias key-gate). Decision 2 and Decision 6 both describe config.go changes. However, `status.json` `planned_files` contains only: `google.go`, `google_test.go`, `provider.go`, `go.mod`, `go.sum` — config.go is absent. The prior round-1 review flagged this as CRITICAL ("production dispatch path missing from file plan"). The design was revised to acknowledge the config.go work, but `planned_files` was never updated.
   What to ask the implementer: Add `"internal/model/config.go"` to the `planned_files` array in `status.json` before transitioning to `in_progress`. Without it, the Verifier's Gate 2 (actual_files ⊆ planned_files) will fail when config.go appears in the diff.

## Summary

Pins: 1 total — 1 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins: #1 (Gate 2 will fail if config.go is touched but not in planned_files)

## Smaller flags (not pins, worth one-line ack)

- (a) S63-subscription-cli-driver also plans to touch `internal/model/config.go` but is in `planned` state — no active collision. If S63 activates while S12 is in flight, serialise via depends_on or coordinate.
- (b) Design §4 explicitly declines to add `design_decisions` to status.json, classifying all six decisions as Type-2. I concur — every decision follows an established pattern (Anthropic driver S11, provider router S10, error taxonomy S10). No Type-1 choices. Not a pin.
- (c) The proxy routing path in `FromEnv()` (lines 53–64) creates an `*OAI` client for ALL providers when sworn credentials are present and `SWORN_DIRECT` is not set. For `google/*` and `vertex/*`, this would route through the OAI-compat proxy, not the native driver. This is pre-existing behaviour shared with `anthropic/*` (S11 didn't address it either). Not a pin for this slice, but worth awareness if proxy + native driver interaction becomes a user issue.
- (d) Design Decision 3 (error mapping) correctly defers SDK error-type inspection to implementation time, acknowledging the Anthropic driver's string-parse approach may not apply to the genai SDK. This is the right posture per spec Risk #1.

## Suggested ack reply
<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Design is sound — all 8 ACs covered, both spec Risks addressed, decisions follow established S10/S11 patterns. 1 pin + 4 flags:

1. **Add config.go to planned_files.** `internal/model/config.go` is in design §3 (Production dispatch group) but missing from `status.json` `planned_files`. Add it before transitioning to in_progress — Gate 2 will fail without it.

Flags (not pins): (a) S63 also plans config.go but is planned — no active collision; (b) design_decisions correctly omitted (all Type-2); (c) proxy routing creates OAI client for google/vertex — pre-existing, not this slice's issue; (d) error mapping defers to implementation time — correct posture.

§2 decisions 1–6 all ack (Type-2, pattern-following). §6 questions: none. Address pin 1 inline (update planned_files), then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: One mechanical pin (add config.go to planned_files) — apply inline during implementation; design is sound and follows established S10/S11 patterns.
-->