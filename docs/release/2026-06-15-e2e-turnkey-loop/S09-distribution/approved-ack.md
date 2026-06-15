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
