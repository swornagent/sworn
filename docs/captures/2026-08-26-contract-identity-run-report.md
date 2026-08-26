# 2026-08-25-contract-identity — run report (2026-08-26)

Nine slices ratified 2026-08-24, authored and recorded 2026-08-25
(revision 1, plan digest `sha256:4c38b39e…8909bd2`, target 300c646f),
merged 2026-08-26 ~10:15 UTC at 0b5159de. **9/9 verified, assembly
passed, zero plan revisions, zero captain escalations.** Evidence
gates green first-pass: CI 32959496094 at the merged head, and the
first fully green local suite this operator host has produced (S6
killed the standing 9-init + 4-codex known-env red classes).

## Delivered

S1 digest-addressed contracts (#200) · S2 plan-authoring surface
(#234; `sworn plan pin|lint|record` — plantool retired, R4 was the
last release authored with scratch tooling) · S3 proposal contract
persistence (#210) · S4 receipt identity split (#218 mech 2 — the
umbrella closes) · S5 role-asset addendum (sworn-owned; vendored
Baton provenance untouched) · S6 init environment honesty
(#226+#228) · S7 worker-surface truthfulness (#188+#189 + the Read
offset/limit/paths + environment-facts amendment; #236/#238 riders)
· S8 temp-root reaper (#194) · S9 scope-refusal retry floor (#224,
ruled (b)-with-floor).

## Run ledger (11 runs, one revision-0 release)

| run | roster (impl/captain/verify/plan+rec) | outcome |
|---|---|---|
| r1 | glm-5.2 / opus / sonnet / kimi-k3 | S1 PASS (t3 after 2 scope refusals; refusal-context carry recovered it); S2 design+PROCEED; parked: identical-failure guard on the *stale* t1/t2 facts after the slice had recovered (#239 production instance #1) |
| r2 | same, park_after=3 | parked in ~2h: 3x PROVIDER_LIMITED = ollama session cap (park carries the provider envelope — R2-S5 machinery live) |
| r3 | same | S2 PASS, S3 design+PROCEED; parked again on the ollama session window (~2h cadence, self-heals) |
| r4-r6 | sonnet / opus / sonnet / qwen3.8-max | 3 parks on NATIVE_SURFACE_INVALID with sanitized diagnostics (`adapter_failed`, duration 0) — two wrong theories consumed (#237 proxy, stale OAuth) before instrumentation |
| r7-r10 | same, diagnostic binaries | operator instrumentation: `fail()` raise-site logging, then scan-trip branch detail → **root cause: the 1MB cumulative native stdout event-stream cap** (`total=1048664` on an ordinary 963B assistant event) |
| r11 | same, operator patch #7 (cumulative cap 16x) | **S3,S4,S5,S6,S7,S8 all verified (S5-S8 first-attempt), S9 designed/PROCEEDed/built/verified, assembly, merge — 8 slices in ~9h** |

## What the release proved about the engine

- **Receipt ancestry adoption held across five fresh journals** — S1's
  PASS survived r1→r11 unbroken; no verified work was ever redone.
- **The refusal-context carry works**: S1's implementer recovered from
  its own scope refusal (a committed root-level `sworn` binary from
  the darwin cross-build check) on the try that received the named
  paths.
- **Park evidence is honest**: every park named its cause, code, and
  (post-S9) will name paths; the ollama parks carried the provider's
  own message.
- **#239's shape appeared in production** (r1): the identical-failure
  guard parked a healthy run on stale work-id archaeology. Ruled
  2026-08-26 (issue comment): lane-scoped drain + lineage-keyed
  freshness; R5 slice.

## Defects found (filing with this report)

1. **The 1MB cumulative native event-stream cap** kills long claude
   implementer sessions legitimately (~20min of streaming), classified
   as NATIVE_SURFACE_INVALID instead of economy. Retroactively the
   likely mechanism behind R3's 5/5 opus deaths attributed to #237 —
   operator patch #6 got credit because r19's smaller S8 session
   stayed under 1MB. Operator patch #7 (16x) carried until the product
   governs it via manifest economics with an honest code.
2. **Failure sanitization scrubs the real diagnostic**
   (`sanitizeFailedObservation` → `adapter_failed`, duration 0):
   three diagnostic run generations were spent recovering facts the
   engine had and discarded.
3. **R3-S1's credential preflight did not catch** what pattern-matched
   as the #221 stale-OAuth class (it wasn't — but six tries burned
   across r4-r6 with `native_preflight_not_required` certs and no
   auth-distinguishable classification, which is what S1-A1 promised).

## Roster verdicts

glm-5.2: real work delivered (S1, S2 — including fixing its own scope
refusal), but the ollama session window caps after ~2h of
implementer-weight use; fine for bounded roles. kimi-k3: certified,
brief planner tenure, same window. qwen3.8-max: certified live,
disciplined cert transcript, streams (the most observable planner
yet); planner/recovery through the entire delivery arc with zero
faults. sonnet: implemented and verified 7 slices in one night once
patch #7 unblocked long sessions. opus: captained throughout, one
transient adapter failure absorbed by t2.

## Operator interventions (all archived)

Three engine patches carried (ops/operator-engine-patches.diff:
#220 claude pin 2.1.241, #6 capture-proxy tool-less admission, #7
stream-cap 16x — the merged product deliberately contains none of
them; R5 lands them properly). Eleven run manifests; two knob
changes (identical_failure_park_after 2→3, roster swaps at park
boundaries per the pre-agreed fallback ladder). Temporary
diagnostics were stripped before the ritual and never committed.
