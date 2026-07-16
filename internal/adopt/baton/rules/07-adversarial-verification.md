---
title: Rule 7 — Adversarial Verification
description: Completion claims must be verified by a fresh-context session loaded only with slice artefacts; the implementing reasoning thread is not allowed to certify its own work
---

# Rule 7 — Adversarial Verification

## The rule

**No slice may transition to `verified` state without a PASS verdict from a fresh-context session loaded only with the slice artefacts and live repo state.**

The session that implemented the slice is never allowed to certify the slice. Self-certification is rejected by definition — not because the implementer is dishonest, but because the implementer's context window is contaminated with optimism about its own work.

Claiming `verified` without an adversarial verification record is a silent deferral of verification and is subject to Rule 2.

## Why

Rule 6 (Proof Bundle) requires that completion claims be backed by machine-verifiable artefacts written to disk. That is necessary but not sufficient. The implementer can still:

- Generate a `Delivered` list that interprets ambiguous diff hunks in its own favour.
- Mark items "done" when the file exists but the user-reachable journey is not wired through.
- Pass tests that exercise the leaf in isolation but never the integration point.
- Write a reachability artefact path that points to a screenshot of a state that doesn't reflect the user gesture it claims.

These are all *consistent with* a valid Rule 6 bundle. The proof bundle catches *forgotten work*; it does not catch *misinterpreted delivery*. The gap is closed only when a different context window — one that has not seen the implementer's framing — reads the bundle and tries to falsify it.

The principle is straightforward and well-documented in agent-engineering practice: a builder and a critic that share the same uninterrupted reasoning thread will converge toward agreement, because agreement is the path of least resistance for the model. Separation is what makes the critic adversarial.

## Context boundary, not model boundary

The separation that matters is the *context window*, not the model identity. The same model running in a fresh window with only the slice artefacts loaded is meaningfully adversarial; a different model running in a window that inherits the implementer's optimistic wrap-up is not.

This is the cheap-and-strong pattern. You do not need a second model subscription, a paid orchestration platform, or continuous multi-agent loops. You need:

1. A fresh terminal window or new agent session.
2. A `verifier.md` role prompt that forbids reading prior conversation.
3. The slice artefacts (`spec.json`, `proof.json`, `status.json`, the repo diff).
4. A PASS / FAIL / BLOCKED return contract.

That is the entire requirement. Anything beyond it is optimisation.

## The verifier contract

The verifier session must:

- **Load only**: `spec.json`, `proof.json`, `status.json` for the target slice, and live repo state via `git diff` / test commands. It must not load the implementer's session transcript, conversational memory, or any "wrap-up" prose.
- **Return exactly one of**:
  - `PASS` — every required gate is satisfied; the slice can transition to `verified`.
  - `FAIL: <numbered list of concrete violations>` — each violation tied to a specific spec acceptance check or proof-bundle gate.
  - `BLOCKED: <reason>` — verification cannot be completed because an external dependency is missing (test command undefined, artefact path unreadable, etc.).
- **Not propose redesigns.** The verifier's job is to falsify, not to help finish.
- **Not edit implementation code.** The verifier may add or repair *verification artefacts* (tests, smoke scripts, missing assertions) that expose a failure, but never the production code under review.
- **Fail closed.** Absence of evidence is a `FAIL`, not an optimistic `PASS`.

The verifier role prompt is provided in `role-prompts/verifier.md`. Paste it verbatim into the fresh session.

## When this rule applies

- Any slice with a `spec.json` that has been moved to `implemented` state.
- Any release-mode work where Rule 6 already requires a proof bundle.
- Any continuation session that needs to confirm prior-session claims before building on top of them.

## When this rule does NOT apply

- Spikes, prototypes, or exploratory work without a slice spec.
- Trivial fixes that fit in a single commit and have no acceptance checks beyond "test passes."
- Documentation-only commits.

If in doubt, run verification anyway. The cost of a falsely-skipped verification is much higher than the cost of running one unnecessarily.

## Slice state machine

Rule 7 introduces a small state machine that lives in `status.json` per slice:

```
planned -> in_progress -> implemented -> [verifier] -> verified | failed_verification
                                                  \-> deferred (per Rule 2)
verified -> [human] -> shipped
```

State transitions:

- **Implementer can move**: `planned` → `in_progress`, `in_progress` → `implemented`, anything → `deferred` (with Rule 2 surfacing).
- **Verifier can move**: `implemented` → `verified` or `failed_verification`.
- **Human can move**: `verified` → `shipped`.
- **No agent can move directly to `verified` from `in_progress`.** The `implemented` checkpoint exists to mark "implementer believes done; awaiting fresh-context verification."

## Cheap-cost workflow

The minimum-cost loop that satisfies Rule 7:

1. Implementer session works one vertical slice. Emits `proof.json` (and maintains `journal.md`), updates `status.json` to `implemented`.
2. Run the **proof-bundle verification gate** (reference implementation: `sworn verify <slice-id>`). It does a deterministic first-pass: confirms `proof.json` exists and validates against `proof-v1`, confirms `git diff` is non-empty, greps for dark-code markers, runs the test commands listed in `proof.json`. If the gate fails, the slice never reaches the verifier — fix and re-run.
3. Open a fresh agent session (new terminal, new window, no prior context). Paste `role-prompts/verifier.md`. Provide the slice id.
4. The verifier reads only the artefacts and returns PASS / FAIL / BLOCKED.
5. Implementer (in a separate session, or the same session after reading the verdict from disk) addresses any FAIL items, regenerates the proof bundle, and re-submits.

This loop costs one extra fresh session per slice. On a Max plan that is effectively free; on API usage it is still cheaper than the rework cost of an overclaimed slice that gets discovered three sessions later.

## Relationship to existing rules

| Rule | What it does | How Rule 7 complements it |
|---|---|---|
| Rule 1 — Reachability Gate | Requires a reachability artefact before claiming phase done | Rule 7 requires that artefact to be *verified by a fresh context*, not just declared by the implementer |
| Rule 2 — No Silent Deferrals | Requires why + tracking + acknowledgement for deferrals | Rule 7 makes "I deferred the verification" detectable: a slice with `status: implemented` and no verifier verdict is a stuck slice |
| Rule 5 — Session Discipline | Anchors sessions to durable trackers | Rule 7 adds a slice-level tracker (`status.json`) underneath the issue-level one |
| Rule 6 — Proof Bundle | Requires verifiable evidence written to disk | Rule 7 requires the evidence to be *read and challenged* by a context that did not produce it. Rule 6 produces the artefact; Rule 7 consumes it adversarially. |

Rule 6 and Rule 7 are intentionally a pair. Rule 6 alone is self-attestation in a more structured shape. Rule 7 alone is unfounded because there is nothing for a verifier to read. Together they form a producer-consumer loop where the producer cannot also be the consumer.

## Anti-patterns

- **"Same session, fresh prompt."** A new prompt in the same context window inherits everything that came before. This is not adversarial separation.
- **"The implementer ran the verifier prompt at the end."** Same context, same reasoning thread. Returns PASS by default.
- **"The verifier asked the implementer for clarification."** The verifier is not allowed to consult the implementer. If the artefacts don't answer the question, the verdict is FAIL or BLOCKED — never an extended dialogue.
- **"We agreed on PASS together."** Agreement between implementer and verifier is suspicious, not reassuring. The verifier exists to falsify; alignment without effort suggests the verifier did not read the artefacts.
- **"The verifier proposed a redesign."** Out of scope. Verifier returns concrete violations tied to spec gates, not architectural opinions.
- **"Tests pass, so PASS."** Tests pass is one input. The verifier must also confirm planned-vs-actual file inventory, reachability artefact presence, and absence of silent deferrals.

## Symptoms of broken adversarial-verification discipline

- Slices spend zero time in `implemented` state — they jump straight to `verified`. This indicates self-certification.
- Verifier verdicts are uniformly PASS with no FAIL or BLOCKED entries. A healthy loop produces FAILs; their absence indicates the verifier is not reading.
- Verifier FAIL messages quote the implementer's wrap-up rather than the artefacts. This means the wrap-up leaked into the verifier context.
- Slices marked `verified` later get re-opened during continuation handshake. The fresh-context regeneration is detecting things the verifier missed.
- The same agent session both implemented and verified — visible in commit history if commits land between status transitions.

## Provenance

This rule was drafted in response to a Perplexity-assisted analysis of the source v0.5.0 release cycle (May 2026), where Rule 6 (Proof Bundle) had been introduced two days earlier and was still insufficient on its own. The analysis identified that the proof bundle was being written by the same reasoning thread that did the implementation, preserving the failure mode in a more structured shape.

The fix — fresh-context verification with artefact-only inputs — was the single recommendation that survived multiple framings of the problem. It is the cheapest intervention with the largest effect: no new tools, no new infrastructure, just a discipline that the certifier must not share a context window with the implementer.
