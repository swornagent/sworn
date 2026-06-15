---
title: Rule 3 — Capture Discipline
description: Conversation context is the most ephemeral persistence layer; analysis and decisions must land in durable storage before session ends
---

# Rule 3 — Capture Discipline

## The rule

**Conversation context is the most ephemeral persistence layer available.** It loses everything on `/clear`, at session boundaries, and as the context window fills. Any analysis, finding, or decision worth keeping must be written to a durable layer before the session ends.

## The durability hierarchy

In order of permanence (most permanent → most ephemeral):

| # | Layer | Survives | Use for |
|---|---|---|---|
| 1 | **Git history** (commit messages) | Everything except force-push history rewrites | Decision rationale, why a diff happened |
| 2 | **Code itself** | Unless deliberately deleted | The implementation; the contract |
| 3 | **`/docs/` content** in repo | Across branches via merge | ADRs, RFCs, operational guides, design specs |
| 4 | **GitHub Issues + comments** | On the GitHub side; backed up but not in repo | Tracked work, session decisions, in-flight state |
| 5 | **Per-project memory** (`~/.claude/projects/.../memory/`) | Across sessions on the same machine | Project conventions, recurring context, lessons learned |
| 6 | **Conversation context** | Until `/clear` or session end | Working surface — not storage |

## Why

The single biggest source of project churn in AI-assisted work is excellent analysis that lives only in conversation and gets lost at session boundaries. Examples observed at the source project:

- A 2000-line subagent audit returns its findings to chat. User reads, makes decisions. `/clear` happens. Audit is gone. Future session re-runs the same audit.
- Design decisions captured in chat but not in any commit, issue, or doc. Three weeks later, someone re-litigates the decision.
- A "no, let's not do X" moment in conversation that nobody writes down. X gets implemented anyway in a later session.
- A subagent's recommended approach in chat, with the user's "yes, do it that way" reply. No commit message restates it. Six months later, "why did we do it this way?" with no trail.

The fix is mechanical: **bias every capture decision toward higher-numbered-permanence layers**. Conversation is for *working*, not for *storing*.

## How to apply

### When dispatching subagents

Any subagent dispatch that produces a substantial findings doc must save its output to a specific path as part of the agent's task:

- Include in the subagent prompt: "Write your final report to `docs/captures/<date>-<topic>.md` AND return a short summary to the conversation."
- The conversation message back to the user references the saved path. The raw report stays on disk.
- The threshold: *would I want to read this again in two weeks?* If yes, save it. If no, conversation is fine.

### During a session

- At natural breakpoints, ask yourself: *what just got decided / discovered that needs to outlive this conversation?* Write it to the appropriate durable layer.
- Don't wait for session end — if context fills mid-session, the captures don't make it.

### At session end

- For any session that produced substantial analysis (audit, design exploration, plan), write a **session handoff capture** at `docs/captures/<date>-<topic>-handoff.md`. The handoff is the "if we `/clear` tomorrow, here's everything we'd need" document.
- For implementation sessions, ensure: (a) code committed with rationale in messages, (b) issues updated with session decisions as comments, (c) any new learnings saved as memory entries.

### For recurring rules and context

- Memory entries (`feedback_*.md` for rules, `project_*.md` for project context) are the right home for rules and recurring context.
- One-off decisions go in commits + issues, not memory.

## Anti-patterns

- **"I'll write it up later"** — you won't, or by the time you do, half the nuance is gone.
- **"It's in the chat history"** — chat history doesn't survive `/clear` and isn't grep-able by your future self in 6 months.
- **"The commit diff explains it"** — no, the diff shows *what* changed. Commit *messages* explain *why*, and only if you wrote them well.
- **"We talked about it in the design session"** — if the design session output isn't in `/docs`, an issue, or a memory entry, it didn't happen.

## Symptoms of broken capture discipline

- The same audit / analysis getting redone every few weeks.
- "Why did we choose X?" with no answer in the visible record.
- Subagent outputs that exist in screenshots / chat logs but not on disk.
- Plans that get abandoned because nobody remembers what was decided.
- New session feeling like starting from scratch on a familiar project.

## Provenance

The v0.5.0 source project audit produced two substantial subagent findings docs (dashboard IA dark-code audit + validation field-coverage parity audit) that initially lived only in conversation context. Recognition of this risk drove the creation of `docs/captures/2026-05-13-v1.0-audit-handoff.md` with the full audit reports preserved as appendices. The user's framing of the problem: "stuff only living within [conversation context]... ephemeral data... we lose too much, too often, and this is ultimately one of the causes of the churn I have been facing and re-work." This rule is the structural fix.
