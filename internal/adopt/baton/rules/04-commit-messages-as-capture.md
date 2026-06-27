---
title: Rule 4 — Commit Messages as Capture Layer
description: Commits that land a documented decision restate the decision in the message body. Plans move; git log is permanent.
---

# Rule 4 — Commit Messages as Capture Layer

## The rule

Commits that land a documented decision **must restate the decision in the message body** — not just "see plan X" or "implement issue #123."

## Why

Plans get edited and moved. Issues get closed and archived. Memory entries get rewritten. `git log --format="%B"` is permanent and traverses the full history of a branch with `--follow` even across renames.

The commit message body is the **only durable, in-repo, traceable record of *why* a change happened that travels with the diff itself**. Treating it as an afterthought ("see plan X") wastes its permanence and offloads the rationale to a layer (the plan) that is by definition more mutable.

## How to apply

For any commit that:

- Lands a feature decided in conversation or planning
- Implements a workaround for a discovered issue
- Resolves a design choice between multiple options
- Defers something explicitly
- Changes behaviour visible to users

The message body should include:

1. **What** — one line in the subject (the existing convention).
2. **Why** — a paragraph or short bullet list in the body, restating the decision and its rationale. Don't link out for the rationale; restate it.
3. **Linked context** — references to issues, plans, ADRs as supplementary, not as the load-bearing rationale.
4. **Trailers** (Co-Authored-By, Refs, etc.) at the bottom.

Minimum bar: 3-5 line bodies for any non-trivial commit. Single-line messages are fine for mechanical changes (typos, formatting, dependency bumps) but not for decisions.

## Examples

### Weak

```
feat(dashboard): add Summary/Detail toggle (see plan 2026-05-11)
```

### Strong

```
feat(dashboard): per-section Summary/Detail toggle with progressive disclosure

Each dashboard section (Account & Profile, Billing & Subscriptions, Invoices,
Resources, Settings) now carries its own Summary/Detail toggle
rather than a global mode. Default is Summary; users open Detail only
on the sections that need it.

Rationale: a global toggle either hides too much (Summary global) or
overwhelms newcomers (Detail global). Per-section progressive
disclosure lets the user expand only the surfaces they care about,
matching the IA rework principle.

Refs: docs/plans/2026-05-13-v0.5.0-completion-punch-list.md W1
```

The strong version survives any future move of the plan doc. It also surfaces in `git log` searches for "progressive disclosure" or "Summary/Detail" without depending on external state.

## When this rule applies extra hard

- **Decisions made in chat with no other written form.** The commit is the only record. Make it count.
- **Behaviour changes visible to users.** Six months later, "why does X behave this way?" should be answerable by `git blame` + `git log`.
- **Workarounds for external issues.** External issues get fixed; the workaround stays. Future you needs to know whether the workaround can be removed.
- **Deferrals.** A `// deferred` comment in code (see Rule 2) gets its full rationale in the commit message that landed it.

## When less rigour is fine

- Pure mechanical changes (lint fixes, typos, automated formatter passes).
- Dependency bumps where the changelog tells the story.
- Reverting a clearly-named prior commit.

## Symptoms of weak commit messages

- `git log` reads like a series of "fix bug" / "update X" with no clue why.
- "Why did we change X?" requires reading the entire diff to guess.
- Plan docs and issues become the only place rationale lives — and they move and change.
- New team members can't trace decisions without scheduling explainer calls.

## Provenance

The v0.5.0 source project audit found multiple "completed" plans whose decisions were captured in conversation but only thinly summarised in commits. Re-tracing rationale required reading the original plan doc — which by then had been edited multiple times. This rule prevents that loss: the commit message body is the immutable record of the decision at the moment the change landed.
