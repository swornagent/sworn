---
title: Brainstorm Patterns — visual decision surfaces for the planner role
description: Lightweight markdown patterns for making brainstorming decisions visible and capturable during the planner phase. Portable to any tool; Claude Code adopters use AskUserQuestion with preview content as the implementation.
---

# Brainstorm Patterns

The planner role's Phase 2 (Discovery) and Phase 3 (Decomposition) are where most of a release's quality is decided. Long prose paragraphs of "what about this, also consider that" make decisions invisible. These patterns make every decision a discrete, visible, capturable event.

The patterns are portable markdown. On Claude Code, the implementation is `AskUserQuestion` with the `preview` field carrying the visual block. On other tools, render the same content inline and capture the response into `intake.md`.

## When to use these patterns

- **Always** for the brainstorm questions surfaced in `intake.md` "Open questions" section.
- **Always** for slice decomposition decisions ("is this one slice or two?").
- **Always** for scope cuts that become Rule 2 deferrals.
- **Sometimes** for trade-offs that emerged during discovery and would otherwise live only in prose. If a decision is going to be referenced later, it should be visible.

## When NOT to use these patterns

- Routine confirmation ("the release name is `2026-05-20-billing-redesign`, right?") — a one-line prose question is fine.
- Information-gathering questions where the human is just describing reality, not choosing between options.
- Anything that doesn't actually have alternatives worth comparing.

A decision card with one option is a smell. If there's only one path, just take it and capture *why* in `intake.md`.

---

## Pattern 1 — Option Matrix

Use when there are two-to-four discrete approaches to the same problem, each with distinguishable trade-offs.

```
┌─────────────────────────────────────────────────────────────────┐
│ DECISION: Where does tax rate live canonically?                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  A. Account Settings only        B. Settings + plan tier         │
│  ─────────────────────────       ───────────────────────────     │
│  + Single source of truth        + Matches prod expectation      │
│  + No migration                  + Per-tier flexibility          │
│  − No per-tier override          − Two sources to reconcile      │
│  − Diverges from prod            − Override-validation complex   │
│  ~3 files                        ~12 files                       │
│  Migration: trivial              Migration: moderate             │
│                                                                  │
│  C. Per-plan-tier only           D. Defer the question           │
│  ─────────────────────────       ───────────────────────────     │
│  + Maximum flexibility           + Unblocks form work            │
│  − Settings page becomes UI-     − Question reappears at         │
│    less for tax rate               implementation                │
│  − Implicit per-tier drift       − Verifier may FAIL slices      │
│  ~8 files                          that depend on the decision   │
│  Migration: moderate             0 files                         │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

In `AskUserQuestion`: render each option's label as the option `label`, the bullet block as the `preview`. The user picks one; the planner appends the choice + a one-line reason to `intake.md` "Decisions made during planning".

## Pattern 2 — Decision Card (binary or short-list)

Use for yes/no questions or quick three-way picks. Smaller than the matrix; same rhythm.

```
┌────────────────────────────────────────────────┐
│ DECISION: Custom rate at plan-tier level?      │
├────────────────────────────────────────────────┤
│                                                 │
│  YES   adds a numeric input per plan tier      │
│        when tax rate = custom; existing        │
│        flat-rate users default to model rate   │
│                                                 │
│  NO    tax rate is select-only at              │
│        plan-tier level; numeric rates only     │
│        live in Account Settings                 │
│                                                 │
│  DEFER to a later release — ship the           │
│        select-only version now                  │
│                                                 │
└────────────────────────────────────────────────┘
```

## Pattern 3 — Scope-Ceiling Bar

Use during decomposition. Visualises which slices blow the 15-25 file ceiling and need splitting before they can be specced.

```
S01-invoice-list-view               [████░░░░░░░░░░░░░░░░]  ~4 files   ✓
S02-payment-form                    [█████░░░░░░░░░░░░░░░]  ~5 files   ✓
S03-account-settings-page           [██████░░░░░░░░░░░░░░]  ~6 files   ✓ (blocked on #43)
S04-email-notifications             [████████░░░░░░░░░░░░]  ~8 files   ✓
S05-dashboard-refactor              [████████████████████]  ~22 files  ⚠ split required

Ceiling: 15-25 files / one user journey / one user-reachable affordance.
Slices over the ceiling MUST be re-decomposed before specs are written.
```

The estimate is rough — it doesn't have to be exact. The bar makes the *relative* scope visible at a glance, which is what triggers the split conversation.

## Pattern 4 — Dependency Graph

Use when slice ordering matters or when blockers run across multiple slices.

```
                          ┌──────────────────────────┐
                          │ #42 migration safety net  │
                          │      (blocks release)     │
                          └────────────┬──────────────┘
                                       │
        ┌──────────────────────────────┼───────────────────────────────┐
        ▼                              ▼                               ▼
┌──────────────┐              ┌──────────────┐               ┌─────────────────┐
│ S01 invoices │              │ S02 payment  │               │ S04 email       │
│  (list view) │              │    form      │◄── deferral?  │  notifications  │
└──────────────┘              └──────────────┘               └────────┬────────┘
                                                                      │
                                                                      ▼
                              ┌──────────────┐               ┌─────────────────┐
                              │  S03 account │◄── #43 ───────│  S05 dashboard  │
                              │   settings   │   pattern     │    refactor     │
                              └──────────────┘               │ (likely splits  │
                                                              │   into 3 subs)  │
                                                              └─────────────────┘
```

Arrows show "X depends on Y" or "X blocks Y". External dependencies (other issues, prerequisite releases) sit at the top.

## Pattern 5 — Deferral Card (Rule 2 surfacing)

Use whenever a scope item is being carved out. This is the *exact* structure Rule 2 requires; using the card form means you can never accidentally surface a deferral without the three components.

```
┌──────────────────────────────────────────────────────────┐
│ DEFERRAL: S02 — Payment form multi-currency support       │
├──────────────────────────────────────────────────────────┤
│ Why:           Multi-currency is a complex cross-cutting  │
│                concern; the current release targets       │
│                single-currency billing flows only.        │
│                                                           │
│ Tracking:      Issue #44, sub-task 2; carry forward to    │
│                next Invoices-themed release if priority   │
│                increases.                                 │
│                                                           │
│ Acknowledged:  Owner, YYYY-MM-DD, during brainstorm       │
│                session for release 2026-05-20-billing-redesign │
└──────────────────────────────────────────────────────────┘
```

Without all three lines populated, the card is invalid and the deferral is unacceptable per Rule 2.

---

## Implementation note — Claude Code

`AskUserQuestion` is the native tool for this. The `preview` field renders monospace content side-by-side with options. For a Pattern 1 (Option Matrix), invoke it once per decision with each option's label as the `label` and the corresponding bullet block as the `preview`.

```typescript
// pseudo-call — actual invocation is via the AskUserQuestion tool
{
  question: "Where does tax rate live canonically?",
  header: "Tax rate home",
  multiSelect: false,
  options: [
    { label: "Account Settings only",         preview: "+ Single source of truth\n+ No migration\n− No per-tier override\n~3 files" },
    { label: "Settings + plan tier",          preview: "+ Matches prod expectation\n+ Per-tier flexibility\n− Two sources to reconcile\n~12 files" },
    { label: "Per-plan-tier only",            preview: "+ Maximum flexibility\n− Settings page UI-less for tax rate\n~8 files" },
    { label: "Defer the question",            preview: "+ Unblocks form work\n− Question reappears at implementation\n0 files" }
  ]
}
```

The user's pick comes back as a single `answer`; the planner immediately appends it to `intake.md` "Decisions made during planning" with the preview body as the captured reasoning. The decision is durable on disk before the next question.

For Patterns 3 (Scope-Ceiling Bar) and 4 (Dependency Graph), `AskUserQuestion` is overkill — those are usually informational displays leading into a Pattern 1/2 decision. Render them as a plain markdown code block in the conversation, then invoke `AskUserQuestion` for the actual choice.

## Implementation note — other tools

The patterns are markdown-renderable in any chat-based interface. If your tool doesn't have a native equivalent of `AskUserQuestion`:

- Render the pattern in the chat message as a code block (the box-drawing characters work in any monospace context).
- End the message with a clear "pick one" prompt and an instruction to reply with the option letter.
- Capture the human's response into `intake.md` "Decisions made during planning" immediately, in the same conversation turn that processes the reply.

The patterns work the same way; only the tool affordance differs.

## What this is not

- Not a formal decision-tree framework. These are visual rhythms, not analytical methods.
- Not a substitute for the conversation itself. The patterns surface decision points; the discovery conversation is what produces them.
- Not a gate. A planner session that produces good intake without using these patterns is still a valid planner session — but in practice, the planner role almost always benefits from the rhythm they create.

## Provenance

The decision-surfacing *rhythm* here — brainstorm before building, and surface a small number of concrete options for the human to choose between rather than burying choices in prose — was inspired by the brainstorming skill in [obra/superpowers](https://github.com/obra/superpowers) (MIT). The patterns themselves are baton's own: superpowers keeps tradeoff comparisons in the terminal and ships no option-matrix or decision-card template, and nothing equivalent to the Scope-Ceiling Bar, Dependency Graph, or Deferral Card — those are specific to baton's slice model (the file-count ceiling, cross-slice ordering, and Rule 2's three-part deferral). Baton grew that inspiration into its own templates and their binding to `AskUserQuestion` and `intake.md`.
