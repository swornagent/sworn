---
title: Rule 6 — Proof Bundle
description: Completion claims require machine-verifiable evidence written to disk; agents cannot self-attest done through prose alone
---

# Rule 6 — Proof Bundle

## The rule

**Before any task, issue phase, or session is marked complete, the agent must produce a structured proof bundle** — a file written to `docs/captures/<date>-<topic>-proof.md` — containing machine-verifiable evidence drawn from repo state, not from conversational memory.

Claiming completion without a proof bundle is a silent deferral of verification. It is subject to Rule 2.

## Why

The five existing Baton rules are **backward-looking capture rules**: they ensure knowledge is preserved after something happens. They cannot prevent an agent from self-attesting completion, because prose-based capture and prose-based completion claims are indistinguishable from the agent's perspective. A well-written session handoff that follows every capture and session-discipline rule can still be factually wrong about what landed.

This failure mode has a specific pattern:

1. A long session runs with subagents handling parallel workstreams.
2. The orchestrator synthesises subagent reports into a summary.
3. The summary accurately reflects the *plan state* and the *intent* — both of which the agent knows well.
4. The summary conflates plan state with repo state — a distinction the agent cannot verify from context alone.
5. The session ends with a "100% complete" claim.
6. The next session's stocktake, run against actual repo state, finds a thin slice of what was claimed.

The root cause is not dishonesty — it is that the agent is a reliable narrator of its own intentions and an unreliable narrator of repo state. The fix is to require the agent to read repo state rather than recall it.

## The proof bundle format

Create `docs/captures/<date>-<topic>-proof.md` with the following sections. Every section must be populated from a live command run, not reconstructed from memory.

```markdown
# Proof Bundle — <topic> — <date>

## Scope

<One sentence: what was this task / phase / session meant to deliver?>

## Files changed

<Output of: git diff --name-only <base-branch>>

## Test results

### <Stack 1, e.g. Go>
<Output of: your test command>

### <Stack 2, e.g. TypeScript>
<Output of: your frontend test command>

## Reachability artefact

<Path to screenshot / Playwright run / smoke-step description. Must name the
user gesture. "Tests pass" is not a reachability artefact — see Rule 1.>

## Delivered

<Bulleted list of items from the plan that are confirmed delivered, with
evidence reference (file path, test name, or artefact path) for each.>

## Not delivered

<Bulleted list of items from the plan that are NOT present in the current
repo state. Each item must be surfaced as a deferral per Rule 2 —
with why, tracking link, and acknowledgement.>

## Divergence from plan

<Any items where the implementation differs from the plan in a meaningful
way. Empty is fine if there is no divergence. Do not leave this section out.>
```

## The continuation handshake

Every session that resumes previous work must open with a **continuation handshake** before any new implementation begins:

1. Read the most recent proof bundle from `docs/captures/`.
2. Regenerate the "Files changed" and "Test results" sections from live repo state.
3. Compare the regenerated state against the prior bundle's "Delivered" list.
4. Surface any divergence — items claimed delivered but absent from current state — before proceeding.
5. Only after reconciliation is complete may the session continue with new work.

The continuation handshake is the direct fix for the "orchestrator makes bold claims; next session's stocktake shows thin delivery" failure mode. It prevents prior-session prose from substituting for current repo reality.

## Scope ceilings

Proof bundles only work if the scope they cover is narrow enough to verify. Subagent dispatches must be bounded to **one vertical slice** — one user-reachable journey, one API endpoint, one UI section, one migration. Dispatches scoped as "finish the feature" or "complete the phase" produce subagent reports that are too broad to verify against a single proof bundle and too likely to conflate intent with delivery.

If a phase is too large to cover in a single proof bundle, decompose it into slices first. Each slice gets its own bundle. The phase is complete only when all slice bundles are present and their "Not delivered" sections are empty or have tracked deferrals.

## Relationship to existing rules

| Rule | What it does | How Rule 6 complements it |
|---|---|---|
| Rule 1 — Reachability Gate | Requires a reachability artefact before marking phase done | Rule 6 requires that artefact path to be recorded in the proof bundle, making it discoverable across sessions |
| Rule 2 — No Silent Deferrals | Requires why + tracking + acknowledgement for deferrals | Rule 6 forces all undelivered items to be enumerated, making silent omission structurally harder |
| Rule 3 — Capture Discipline | Requires subagent findings saved to durable storage | Rule 6 extends this to *verification outputs*, not just findings |
| Rule 5 — Session Discipline | Requires session end capture to durable storage | Rule 6 adds a *structured schema* to that capture, replacing free-form prose with verifiable fields |

## When this rule applies

- Any task or phase that has a plan, issue, or spec it is meant to satisfy.
- Any session that ends with a completion claim.
- Any continuation session resuming prior work.

## When this rule does NOT apply

- Exploratory spikes with no prior plan (the spike *is* the output).
- Quick fixes that fit in a single commit with no prior spec.
- Sessions that produce no completion claim — only findings, drafts, or proposals.

## Anti-patterns

- **"The tests are green"** — green tests confirm the tests pass, not that the feature is reachable or that the plan was delivered. Tests must be cited in the bundle with their output, not asserted.
- **"I checked the files"** — the proof bundle requires `git diff --name-only` output, not the agent's recollection of what it changed.
- **"It's all in the session handoff"** — a free-form handoff is a capture artefact (Rule 3); it is not a proof bundle. Both are required for completion claims.
- **"The subagent confirmed it"** — subagent confirmation is a narration. The proof bundle is a verification. Narrations from subagents do not substitute for repo state reads.

## Symptoms of broken proof-bundle discipline

- Orchestrator declares a phase complete; the next session's stocktake finds a thin slice actually landed.
- "Delivered" lists in session handoffs that don't correspond to files in `git diff`.
- Reachability artefacts referenced in handoffs that don't exist on disk.
- Subagent reports that claim completion of items the repo has no trace of.
- Continuation sessions that spend the first 20 minutes re-establishing what the previous session did.

## Provenance

This rule emerged directly from the v0.5.0 release cycle at the source project (May 2026), where multiple consecutive sessions across Claude Code and Codex ended with orchestrator claims of high completion followed by stocktakes revealing thin delivery. The pattern was consistent: the orchestrator was a reliable narrator of plan state and intent, and an unreliable narrator of repo state. The five existing rules — all backward-looking capture rules — were followed correctly and still did not prevent the overclaiming, because the failure mode is verification, not capture.

The rule is the minimal intervention: require the agent to read repo state and write the output to disk before making a completion claim. No new tooling, no new infrastructure — just a structured file that cannot be written without running the commands.
