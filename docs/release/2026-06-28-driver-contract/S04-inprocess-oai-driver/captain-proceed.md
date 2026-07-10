# Coach acknowledgement — S04-inprocess-oai-driver

Date: 2026-07-10
Decided by: Brad (Coach) — ratified in session, recorded per Rule 9
Review: review.md @ 2026-07-06 (design commit 4142edc, design.md at f268da9)
Verdict: PROCEED — all 6 pins acknowledged, dispositions below

## Pin dispositions

1. **Fail-fast handoff (D5) — ACCEPTED, option (a) + tracked note.**
   `Dispatch` returns the underlying `*model.Error` as its error value so the
   engine's `model.IsTerminal` terminal-halt keeps firing on the pre-S06 path,
   independent of how S06 later reads `Result.ErrKind`. This removes the
   temporal coupling across the rewire boundary: the intermediate state (S04
   landed, S06 not yet) stays safe on its own.
   **Rule 2 tracked obligation on S06-loop-dispatch-rewire:** the rewired loop
   MUST treat `Result.ErrKind ∈ {auth, credits}` as terminal, to keep parity
   with the subprocess drivers (which collapse all terminal cases to `auth`).
   Tracking: this note + S06 spec/design review must cite it; acknowledgement:
   this file (Coach informed and decided).

2. **ErrKind constants (D5/D7) — ACCEPTED.** Use the package `ErrKind*`
   constants (`ErrKindConfig`/`ErrKindTransient`/`ErrKindAuth`/
   `ErrKindProvider`/`ErrKindProtocol`) rather than string literals, and make
   `KindAuth → ErrKindAuth` an explicit mapping, not an incidental
   `String()` collision.

3. **Responses identity vs AC-03 (D1) — RESOLVED by code inspection; AC-03
   narrowed to the chat identity.** `convertMessages`
   (`internal/model/openai_responses.go:382-391`) emits a tool-calling
   assistant turn as pure `function_call` input items and never creates a
   message item, so the Responses wire format structurally cannot express the
   tool-only content-drop. AC-03's test obligation is narrowed to the
   `oai-inprocess` (chat/completions) identity; `oai-responses-inprocess` is
   exempt as moot-by-construction, recorded here rather than silently skipped.
   No spec scope change required.

4. **Rule 9 design_decisions — ACCEPTED.** Implementer populates
   `status.json.design_decisions`. D1 (two registered identities off one
   struct — the driver-identity model S05/S06 consume) and D5 (the
   cross-driver error contract) are classified **Type-1**, decided by the
   Coach in this acknowledgement. D6/D7 remain Type-2 defaults.

5. **CostSource (D6) — ACCEPTED.** Emit `CostSource: "estimated"` (the
   `driver.go:137` documented example), not a new `"nominal"` value. The
   placeholder-vs-0 choice stands as a clearly-tagged estimate for S08 to
   replace.

6. **D5 narrowing of AC-04 (Open Q1) — ACCEPTED.** A classified provider
   error (auth/credits/rate-limit) on the structured-verdict call keeps its
   real `ErrKind` rather than folding into `protocol`. A transport auth
   failure is not a protocol failure; folding would hide the pin-1 signal.

## Flags (a)–(d): acknowledged as recorded in review.md. No action beyond the
above; the merge gate owns full-suite verification of the residual
orthogonal drift noted in (d).

Proceed to implementation.
