# Captain review — S22-openrouter-tool-structured-output
Date: 2026-07-18
Design commit: 49f95160e0d74b3ca1f0d83a89e276d62e00ae80

## Pins

1. [mechanical] §3.3 — Separate immutable GLM history from the config-selected attempt-3 identity.
   What I observed: §3 says to “validate immutable v1 history” and then select v2, but live candidate `d02899f6` passes one `ProofReceiptBinding`, built from the configured `modelID`, through attempts 1–3. Its tests also configure `s22ProofReceiptModel`, so they neither prove a different configured model can recover nor pin attempt 2 to the exact `opaque/UNPARSEABLE/2` tuple. The selected client is then created through environment-aware `model.FromEnv`, so the design does not yet prove that the dispatched model and v2 receipt model are identical.
   What to ask the implementer: Use distinct immutable-history and attempt-3 bindings; require the exact attempt-1 and attempt-2 model/class/result/exit tuples; pin client construction to the exact `ResolveVerifierModel("", cfg)` result with no environment model substitution or fallback; add per-field zero-dispatch mutations plus a happy path whose configured model differs from historical GLM.

2. [mechanical] §3.1 — Make Captain acknowledgement and proof prerequisites machine-authoritative before reservation.
   What I observed: AC-09/AC-12 and §4 require fresh Captain acknowledgement plus deterministic tests, full suite, vet, build, and regenerated proof before attempt 3. §3 only says “enforce capability/preflight gates”; live candidate `d02899f6` does not read S22 `state`, the Captain verdict/acknowledgement, or proof evidence, so the public command can reach reservation while the slice is still `design_review`.
   What to ask the implementer: Name and validate the exact durable lifecycle/proof authorities that permit attempt 3, fail before reservation when any is absent, stale, or mismatched, and add built-command zero-dispatch tests for pre-acknowledgement state and missing/stale proof evidence.

3. [mechanical] §3.1 — Reject `--configured-recovery` unless `--proof-receipt` owns the invocation.
   What I observed: §1 defines configured recovery as an explicit proof-receipt mode, but live candidate `d02899f6` registers the flag globally and only consults it inside the `--proof-receipt` branch. Supplying `--configured-recovery` alone falls through to ordinary `llm-check` behavior instead of failing closed.
   What to ask the implementer: Add the owning-entry-point pairing guard before model setup and a built-command test proving the orphan flag exits non-zero with zero provider requests and no receipt mutation.

4. [mechanical] §3.5 — Close the v2 schema-to-Go drift surface.
   What I observed: §3 calls `llm-check-proof-receipt-v2.schema.json` the strict record authority “consumed unchanged,” but the live candidate mirrors its ID/version/shape in Go constants and manual checks; no configured-recovery test reads or cross-validates the declared schema file.
   What to ask the implementer: Add a deterministic cross-assertion that rendered and decoded attempt-3 records conform to the actual v2 schema authority, including required/additional fields, enums, version, and ordinal, without changing the prohibited embedded Baton schemas.

5. [mechanical] §1.2 — Make the exact preservation ACs explicit rather than relying only on “remain unchanged.”
   What I observed: §1 names the direct forced-tool and canonical-validation boundary, but AC-05 endpoint override isolation and the exact AC-07/AC-08 null/non-function tool-call rejections appear only later in §5. Under the §1-to-AC drift check, those preservation commitments are implicit.
   What to ask the implementer: Carry AC-05, AC-07, and AC-08 explicitly into the implementation checklist/user-visible preservation statement and retain their exact zero-fallback/one-dispatch tests.

6. [memory-cited] §2.2 — Preserve typed provider-error authority outside the administrative exception.
   What I observed: Decision 2 keeps receipt classification typed and Decision 3 keeps configured recovery outside the retry classifier, aligning with the project decision that retry policy consumes `model.Error{Kind}` rather than raw error text.
   What to ask the implementer: Confirm the configured-recovery path does not reuse broad legacy `IsTransient`, error strings, or unknown failures as dispatch authority; keep the administrative attempt-3 gate separate from the typed retry taxonomy.
   Citation (if [memory-cited]): [[project_provider_error_taxonomy]]

7. [memory-cited] §2.3 — Anchor the no-default model decision to the existing capability-policy memory.
   What I observed: Decision 3 resolves the customer’s configured verifier, applies structured capability as a hard floor, and forbids Sworn-selected fallback, which aligns with the ratified capability-based model-selection policy. The cited alias `[[no-model-defaults-policy]]` does not exist in the project memory index; the existing memory is `[[capability-based-model-selection-ratified]]`.
   What to ask the implementer: Confirm that memory applies, replace the nonexistent alias with the existing citation, and keep config consumption read-only with no inferred provider/model default.
   Citation (if [memory-cited]): [[capability-based-model-selection-ratified]]

## Summary

Pins: 7 total — 5 [mechanical], 2 [memory-cited], 0 [escalate]
Critical pins (if any): 1, 2, 3

## Smaller flags (not pins, worth one-line acknowledgement)

- The project design-fit gate exits 0 as `DESIGNAUDIT EXEMPT — not ui_bearing`.
- The repository design-review LLM check returned `PASS — no findings`.
- No sibling in `in_progress` or `implemented` collides with §3; the overlapping S04/S21 slices are verified, and the design explicitly acknowledges candidate `d02899f6`.

## Suggested acknowledgement reply
<!-- Human-extractable section: a driver that applies the acknowledgement automatically reads everything
     between this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

TL;DR Sound bounded-recovery direction with the Type-1 choice already ratified; close three critical fail-closed entry/binding gaps inline. 7 pins + 3 flags:

1. **Split historical and attempt-3 identity.** Use an immutable GLM binding for exact attempts 1–2, a separate config-resolved binding for attempt 3, prohibit environment model substitution/fallback, and prove a different configured model plus per-field history mutations.
2. **Enforce lifecycle/proof authority.** Validate the exact durable Captain acknowledgement, slice state, deterministic/full-suite/vet/build, and fresh proof evidence before reservation; missing or stale evidence must dispatch zero calls.
3. **Own the flag pair.** Reject `--configured-recovery` without `--proof-receipt` before model setup, with non-zero exit, zero dispatch, and no receipt mutation.
4. **Cross-check the v2 schema.** Prove rendered/decoded attempt-3 receipts conform to the actual Planner-owned v2 schema without changing embedded Baton schemas.
5. **Keep preservation ACs explicit.** Carry AC-05, AC-07, and AC-08 into the implementation checklist and retain their exact isolation/malformed-tool tests.
6. **Preserve typed error policy.** Apply `[[project_provider_error_taxonomy]]`; the administrative attempt-3 gate stays outside retry classification and never derives authority from raw/error-string/unknown outcomes.
7. **Preserve customer model authority.** Apply `[[capability-based-model-selection-ratified]]`, replace the nonexistent `[[no-model-defaults-policy]]` alias, and keep model selection config-only, capability-gated, read-only, and fallback-free.

Flags (not pins): (a) designaudit is correctly exempt because Sworn is not UI-bearing; (b) the design-review LLM check passed with no findings; (c) S04/S21 overlaps are verified and candidate `d02899f6` remains explicitly uncertified.

§2 Decisions 2 and 3 are acknowledged against `[[project_provider_error_taxonomy]]` and `[[capability-based-model-selection-ratified]]`; Decision 1 is clean. §6 has no open questions and is acknowledged.

Address pins 1–7 inline during implementation, then proceed to in_progress.

## Routing verdict

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: yes
REASON: All seven pins are apply-inline technical corrections or memory confirmations; the Coach-ratified Type-1 recovery direction needs no design re-pick.
-->
