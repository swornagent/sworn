---
title: Rule 5 — Session Discipline
description: Implementation sessions anchored to GitHub Issues (or equivalent durable tracker); captures at session boundaries
---

# Rule 5 — Session Discipline

## The rule

Implementation sessions of any non-trivial scope are **anchored to a GitHub Issue** (or equivalent durable tracker — Linear, Jira, etc.). Decisions, progress, and deferrals are captured *to the anchor* at session boundaries, not just into the agent's working context.

## Why

A session is a working surface; an issue is a durable record. Without an anchor:

- Future sessions can't tell what was already tried.
- Multiple sessions on the same work fragment context across them.
- "Status" becomes a chat-history grep instead of a one-click view.
- Decisions made in chat never make it to anyone who wasn't in the room.

Anchoring fixes all four. The discipline is procedural: every session has a known durable home, and the session ends with that home updated.

## How to apply

### Session start

- Ask which issue the work belongs to. If none exists, create one before starting. **Exception — Baton release-mode sessions:** when the work is a release-mode command (`/plan-release`, `/replan-release`, `/implement-slice`, `/verify-slice`, `/merge-track`, `/merge-release`) operating on a `docs/release/<name>/` tree, that tree — `index.md`, `intake.md`, and each slice's `spec.md` / `status.json` / `journal.md` — **is** the durable anchor; it is exactly the "equivalent durable tracker" this rule allows. Do **not** open by asking for, or creating, a GitHub issue. Proceed straight to the command's own Step 0.
- Read the issue's existing comments / linked context. This is what you'd already have known if the previous session had captured properly.
- Set a goal for the session in plain text — what does "done" look like?

### During the session

- At natural breakpoints (a sub-task completes, a decision is made, a blocker surfaces), offer to capture to the issue.
- "Natural breakpoint" is not a fixed cadence — it's whenever the context-vs-anchor sync is at risk of diverging.
- If a decision is made that's worth keeping (option chosen, scope cut, deferral acknowledged), capture it the moment it happens, not at the end.

### Session end

- Capture: decisions made, completed work (with PR / commit links), deferred items (with reason + tracking), next steps.
- If substantial analysis happened, write a handoff capture per Rule 3 (Capture Discipline).
- If the issue is now done, close it with the closing commit referenced. If not, leave a "where we stopped" comment.

### When the session was exploratory / planning, not implementation

- The issue can be a planning issue, an epic, or an RFC. Anchor isn't optional just because no code changed.
- The session handoff capture (Rule 3) becomes the primary artefact; the issue gets a comment linking to it.

## The issue is the contract; the doc is the design

Issues are mutable state — they describe what's in flight, who owns it, what's blocking, current state. Issues are the right place for:

- Epics tracking multi-issue work
- Feature specs at the level needed to start work
- Session captures and decision logs
- Bug reports and reproductions
- Roadmap items pre-commitment

`/docs/` content is stable reference material — it describes what something IS or how something WORKS at a moment, not who's doing what to it. Right home for:

- ADRs (architectural decision records)
- RFCs (proposals not yet committed)
- Operational runbooks
- Strategy docs
- Design specs that survive across many issues

Rule of thumb: *if it would become stale as work progresses, it belongs in an issue.* If it would still be true after the work ships, `/docs/`.

## When to skip the anchor

- Quick questions ("how does X work?").
- One-off fixes that fit in a single commit and don't need cross-session continuity.
- Spike sessions where the spike branch is itself the artefact and gets deleted.
- **Baton release-mode sessions** — the release artefact tree under `docs/release/<name>/` is the anchor (see the Session-start exception above). Never prompt the human for a GitHub issue.

If you're unsure whether to anchor: anchor. The cost is one `gh issue create`. The cost of *not* anchoring is the rework discussed in Rule 3.

## Handoff directionality

A session that ends by passing work to another role emits a **handoff**. Every handoff resolves in exactly one of two directions:

- **Forward** — to the next role in the pipeline (planner → implementer → verifier).
- **Up** — escalated to the human, when no role can resolve the matter.

A handoff **never returns to its sender.** A return-to-sender handoff — a verifier handing back to an implementer who hands back to the verifier, or an implementer handing back to a planner who hands back to the implementer — is non-terminating by construction: neither party gained the authority to break the cycle, so the work oscillates indefinitely.

This is why a verifier `BLOCKED` verdict routes to `/replan-release` (forward, to the planner) and never to "re-run `/verify-slice`" (back, to itself); why the planner's two legal responses to an inbound BLOCKED slice are *correct the spec* (forward — the slice re-enters verification) or *escalate to the human* (up), never *return to the verifier*; and why `/implement-slice` halts on a slice with an open BLOCKED verdict rather than absorbing a blocker an implementer has no authority to clear.

When you are about to hand work back to whoever just handed it to you, stop: either you can resolve it (do so, then hand forward) or you cannot (escalate up). "Hand it back and hope" is not a third option.

## Symptoms of broken session discipline

- "What were we working on last week?" requires reading chat history.
- The same blocker gets re-discovered in multiple sessions.
- PRs land with no traceable origin issue.
- Decisions made in one session are unknown to a different person / agent in the next.

## Provenance

The the source project AGENTS.md has had a Session Discipline section since well before this rule-set was codified. The v0.5.0 audit found that the *stated rule* (anchor to issues) had drifted from *lived practice* (substantial planning in `/docs/plans/` markdown files with no issue anchor). The rule below tightens the discipline by explicitly distinguishing issue-shaped state from doc-shaped reference material, and by treating session captures as mandatory rather than offer-only.
