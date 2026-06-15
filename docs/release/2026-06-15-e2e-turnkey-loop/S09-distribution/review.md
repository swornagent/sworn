# Captain review — S09-distribution
Date: 2026-06-16
Captain version: 0.1
Design TL;DR commit: 5e4606ddc36d1a4a0dadde8b0eaa2526c4c679e9

## Pins

1. [mechanical] §1 vs AC2 — Container smoke test misses `sworn verify`
   What I observed: Spec AC2 says "The container runs `sworn verify`." Design §1 says the container "responds to `sworn version` with the release tag." The reachability plan §5.2 smoke-tests `docker run --rm sworn-test version` but never tests `sworn verify`.
   What to ask the implementer: Add `docker run --rm sworn-test verify` (or a stub verify) to the smoke test in §5.2. The binary already supports `sworn version`; confirming it also runs `sworn verify` closes the AC.

2. [mechanical] §4 NOT-doing — Windows deferral tracking is a vague phrase
   What I observed: §4 Windows exclusion says "Deferred with tracking: add `windows/amd64` to goreleaser when a user asks for it." Per `feedback_placeholder_tracking_smell`, "when a user asks for it" is not concrete tracking — no issue number, release folder, or slice ID.
   What to ask the implementer: Create a GitHub issue for the Windows build gap and cite the number in the deferral comment, or explicitly state "no tracking — this is a conscious omission, not a deferral."

## Summary

Pins: 2 total — 2 [mechanical], 0 [memory-cited], 0 [escalate]
Critical pins (if any): none — both are editorial/test-coverage gaps; neither would cause the slice to ship broken.

## Smaller flags (not pins, worth one-line ack)

- (a) §4 go-install testing rationale ("testing it pre-release is nonsensical") is slightly imprecise — `go install` supports tagged versions, it's just that the tag doesn't exist pre-release. The practical point stands; acknowledge and reword for precision.
- (b) Decision #1 (no Windows) rationale is sound; the tracking gap (pin 2) is the only issue.

## Suggested ack reply

TL;DR Clean design — distribution is pure config/infra, all channels align with spec. 2 pins + 2 flags:

1. **AC2 gap: Docker smoke test misses `sworn verify`.** Add `docker run --rm sworn-test verify` alongside the `version` smoke test in §5.2.
2. **Windows deferral needs concrete tracking.** Replace "when a user asks for it" with a GitHub issue number, or declare it a conscious omission (no tracking).

Flags (not pins): (a) reword go-install rationale for precision; (b) Decision #1 rationale is sound.

§2 decisions 1-5 ack. §6 question none ack.

Address pins 1-2 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: 2 mechanical pins (add Docker verify smoke test, fix deferral tracking) — both apply-inline editorial corrections, no design changes needed.
-->