---
title: Rule 1 — Reachability Gate
description: TDD's failing test must render through the integration point that owns the user-facing affordance, not the leaf component in isolation
---

# Rule 1 — Reachability Gate

## The rule

For any feature that has a user-facing affordance (UI control, route, form field, API endpoint), the **first failing test in a TDD cycle must render through the integration point that owns the affordance** — not the leaf component in isolation.

## Why

The most common AI-assist failure mode is "dark code":

1. Plan calls for feature X with affordance Y.
2. AI agent (or human) writes a leaf component for X.
3. Agent writes a leaf-level unit test: `render(<X prop="...">)` with the prop set to what the affordance would produce.
4. Test passes green.
5. Agent marks task done.
6. Nobody ever wires X into a parent that *can produce* that prop value through Y.
7. Feature ships unreachable. Tests stay green. CI stays green. Reviewers don't catch it because the diff looks complete.

The trap is that the test was *technically valid* — the leaf does render correctly under that prop. But the test never asked "can the user reach this?" That question must be at the *top* of the TDD cycle, not the bottom.

## How to apply

- **The first failing test must render at or above the integration point that owns the affordance.**
  - For a UI toggle: render the parent panel/container that owns the toggle UI, simulate the click, assert the leaf's state changes.
  - For a form field: render the section/page that owns the field, fill it, assert downstream behaviour (validation, projection update, persistence).
  - For an API endpoint: assert via an integration test that hits the route, not by importing the handler function directly.

- **If the integration point can't render the feature yet** (because the toggle/state/route doesn't exist), THAT failure is the correct TDD red. Build the integration glue first; the leaf falls out.

- **Leaf-level unit tests are fine** *in addition* for edge cases (error states, boundary values, prop combinations). They cannot be the **sole** proof of life.

- **"Pass 1 / Pass 2" splits** — building a primitive now and wiring it later — are acceptable ONLY when:
  - The Pass 2 task is created in your tracker the moment Pass 1 lands.
  - Pass 2 has a named owner.
  - Pass 2 has a deadline or a clear unblocking condition.
  - All three are visible to the decision-maker, not just inferred from a code comment.

## Red flags

A new component, hook, or module is suspect if, after a phase merges, it:

- Is imported only by its own test file.
- Has no `grep` hits outside its own module's directory.
- Has a unit test that hardcodes a state value the user has no UI affordance to produce.
- Has a sibling component that would naturally consume it but doesn't.

A `grep` heuristic that surfaces these: list new files added during a phase; for each, run `grep -rln "<FileBaseName>" .` excluding test files. If zero hits outside the module's own directory, investigate before declaring the phase done.

## Phase completion artefact

Before marking any phase complete, produce a **reachability artefact**:

- A screenshot of the rendered affordance, OR
- A Playwright (or equivalent end-to-end test) run that clicks through to it, OR
- An explicit "open browser, do X, observe Y" smoke step that names the *user gesture* — not just "the test passes"

A green typecheck plus green unit-test suite is **not** a reachability artefact. End-to-end coverage is.

For release-mode slices whose artefact is a screenshot, the canonical path, per-track spec layout, and bit-stable capture pattern live in [`role-prompts/implementer.md`](role-prompts/implementer.md) → "Reachability screenshot convention". This rule defines *what counts*; the implementer prompt defines *where it goes* and *how to capture it reproducibly*.

## When this rule applies

- Any feature with a user-facing affordance.
- Any code with a contract surface (a public type, a schema, an API endpoint, a CLI flag) — even if not user-facing, the contract has a "consumer" that plays the role of the integration point.

## When this rule does NOT apply

- Pure utility functions with no consumer yet (rare — usually a smell that the utility is premature).
- Internal helpers exercised exclusively by their parent module (the parent module's test IS the integration test).
- Deliberate scaffolding clearly marked as such with tracking — see Pass 1 / Pass 2 conditions above.

## Provenance

The v0.5.0 audit on the source project's monorepo (May 2026) found five primitives shipped as dark code, all with passing TDD-written unit tests: per-section Summary/Detail mode toggle (component prop existed, parent hardcoded the literal), `SectionStatusBadge` (built + tested, zero render sites), `FieldErrorIndicator` (built, no consumers), `useCheckoutFlow` (496 lines, 26 tests, no consumer), per-line `taxRate` Pro gate inside `InvoiceSection.Detailed` branch (Detail mode unreachable from UI). Each had been "done" at the leaf-component test level. None were reachable.

---
title: Rule 2 — No Silent Deferrals
description: Inline "deferred" comments are rationalisations, not decisions, unless they carry why + tracking + acknowledgement
---

# Rule 2 — No Silent Deferrals

## The rule

An inline code comment marking something as "deferred" / "later" / "future" / "TODO" is **not a decision**. It becomes one only when all three of the following are present:

1. **Why** — a concrete reason the deferral is necessary (framework limitation, blocking dependency, explicit scope cut). Not "we ran out of time on this PR."
2. **Tracking** — a **concrete, resolvable reference** the work lives in (see "What counts as tracking" below). Not just the inline comment.
3. **Acknowledgement** — the user, product owner, or decision-maker has been told, in plain text, that this item is being deferred.

Without all three, the inline comment is dark code's data-model cousin: it looks tracked, isn't.

### What counts as tracking (hardened)

A deferral's **Tracking** is satisfied **only** by one of:

- **An owning slice id** — a concrete slice that will deliver the deferred work, e.g. `S14-board-json`. The slice must actually exist (planned or beyond) in a release; a slice id you invent on the spot but never create does not count.
- **A tracker ref** — an item in whatever issue tool the project uses, **tool-agnostic**: GitHub `#123` (or a `gh` issue URL), Jira `ABC-123`, Linear `ENG-123`, or an explicit issue URL.

The following are **NOT** tracking — a deferral whose only "tracking" is one of these is a **violation**, not a decision:

- Vague futures: "a follow-up slice", "a future slice", "later", "future concern", "future cleanup", "future enhancement", "when X is built".
- A **release-theme** or epic name (e.g. "the proof-visibility theme") — a theme is not a tracked unit of work.
- A **decision record** (ADR-NNNN) — it records *why*, not a tracked *work item*.
- A **process or ceremony** name (`/mark-shipped`, "ship cutover").
- A **circular self-reference** — the deferral pointing at its own `not_delivered` / `open_deferrals` entry.

If you cannot fill Tracking with a real owning-slice id or tracker ref, you do not have a deferral — you have a punt. **Create the tracking item first** (file the issue, or add the owning slice), then record the deferral citing it.

### Where deferrals hide

The same triple applies wherever work is punted, not just inline comments. Audit **all** of these surfaces:

- inline code comments (`// deferred` / `// TODO` / `// later` / `// future`);
- a slice's `proof.json` / `proof.md` **`## Not delivered`** section;
- a slice's `status.json` **`open_deferrals`** array;
- a slice's `spec.json` / `spec.md` **`## Out of scope`** block (an out-of-scope item is fine *only* if it names the owning slice that does deliver it; an out-of-scope item that simply punts the work with no owner is an untracked deferral);
- user-visible "coming soon" / "deferred" labels (see "The UI-label cousin").

### Encoding in the `open_deferrals` record

When a deferral is recorded in a slice's `status.json` `open_deferrals[]` (schema `slice-status-v1`), the conceptual triple maps to **four required fields** — `acknowledgement` splits into the plain-text evidence and its structured attribution:

- **`why`** — the concrete reason (rule part 1).
- **`tracking`** — the resolvable owning-slice id or tracker ref (rule part 2).
- **`acknowledgement`** — the plain-text record that the decision-maker was **told** (rule part 3): the "told" evidence itself.
- **`acknowledged_by`** — **who** acknowledged it (required since Baton v0.7.0). Strict-additive to `acknowledgement`, not a substitute: a name alone is not the plain-text "told" evidence Rule 2 demands, and the evidence alone leaves the decision unattributed. `acknowledged_at` (ISO 8601) is optional.

An `open_deferrals[]` entry missing `acknowledged_by` is schema-invalid — the record has an unattributed acknowledgement, which is not a decision.

## Why

The v0.5.0 audit traced six schema-level "deferrals" in inline header comments. Of the six, **only one had a real framework-level reason**. The other five were silent absences dressed up as decisions:

- "Land later when Detail mode UI is wired" — Detail mode itself was unbuilt and unscheduled.
- "Cross-field deferred (see header)" — base version existed; per-item version simply never written.
- "Deferred to a later phase" — no specific reason given; scope management punt.
- Several entire entities (Line Item, Payment Method, Subscription Plan, Custom Resource, deferred payment, five action types) had no schema at all — "deferred" in the team summary but in fact never started.

The decision-maker had not been told about any of them. They surfaced only when an audit grepped the schema headers.

## How to apply

- **Before writing `// deferred` / `// later` / `// future` / `// TODO`** on a schema rule, contract surface, or other publicly-consumed declaration: surface the decision to the user first. Get explicit acknowledgement. Create a tracking item. Then write the comment, with the tracking ID in the comment.
- **Pattern to use:** `// deferred: <reason> — tracked at <issue/plan-id>`. If you can't fill both blanks honestly, you don't have a deferral, you have a punt.
- **When reviewing code** with bare `// deferred` / `// TODO` and no linked tracking: flag it. The comment author owes you the why + tracking + acknowledgement chain.
- **At phase / sprint boundaries:** grep your changeset for "deferred", "later", "future", "TODO". For each hit, verify all three conditions are met. Any failure becomes a tracked item or gets resolved before close.

## The schema-cousin pattern

Silent deferrals show up most often where types or contracts publish a promise that the implementation doesn't keep. Examples:

- A schema declares a field but no rule validates it.
- An engine input type comment says "computed via X" but no calc-X function exists.
- A type union enumerates cases the matching switch doesn't handle.
- A public function signature accepts a parameter that's silently ignored.
- **A type signature claims a field is non-optional, but the runtime returns `undefined` during normal lifecycle states** (initial render, loading window, error path). Every consumer that trusts the type is a landmine waiting for the wrong render-order to fire.

The reachability gate (Rule 1) catches the UI version of this; this rule catches the *contract-published* version. Both are forms of dark code.

## Rule of three — escalating from band-aids to contract fixes

When the schema-cousin pattern fires at runtime (page 500s, test 500s, `Cannot read properties of undefined`), it is tempting to fix the immediate consumer with an optional-chain or a guard and ship. That fix is a band-aid — the *next* consumer of the same lying contract is the next bug.

**Decision discipline**: count the band-aids on the same root contract.

- **One band-aid**: legitimate fix. The other consumers may genuinely tolerate the absence; case-specific guard is appropriate.
- **Two band-aids**: coincidence. Keep your eye on it; the pattern may be real or may not.
- **Three band-aids on the same root contract**: stop. The contract itself is the bug. Tighten the type (or split into a discriminated union), let the type-checker surface every consumer in one pass, and fix them as a batch with a single consistent pattern.

The cost of whack-a-mole is open-ended — you only find the consumers your tests happen to traverse. The cost of the type-tightening pass is bounded by the typecheck's output count. Above ~20 consumer sites, the contract fix deserves its own slice; below that, fold into the slice that surfaced the third instance.

### Why this lives under Rule 2

Each band-aid leaves the underlying contract lie in place. The remaining consumer sites are *future bugs nobody has tracked*. That is the silent-deferral pattern in a slightly different shape: a known incorrectness, not surfaced, not tracked, not acknowledged. The fix isn't another band-aid; it's writing the why + tracking + acknowledgement (via the typecheck pass) or doing the contract fix that obviates the question.

### Concrete case

During the source project's v0.5.0 push, a Playwright spec slice (S06-checkout-view-playwright) failed pro-tier walks with `Cannot read properties of undefined (reading 'years')`. The contract: `ForecastResult.projection: ForecastVectors` (non-optional). The runtime: `projection` was `undefined` during initial render before the engine settled.

Three consumer fixes landed in sequence — `InvoiceDeficitAlerts.tsx`, `ResultsDisplay.tsx:176`, `ResultsDisplay.tsx:983` — each catching the same root cause from a different angle. After the third, the implementer surfaced: *"each fix exposes the next one in a cascade. The previous non-optional contract was the lie that hid the crash."* That was the rule-of-three signal. The decision: stop whacking, flip the type to `projection?: ForecastVectors`, let TypeScript surface every consumer, fix them in one pass.

The band-aid tests written during the whack-a-mole phase stayed — they became real regression assertions against the new tightened contract.

### Guardrail on the escalation

The type-tightening pass can cascade further than expected. **Halt at >20 consumer sites surfaced by the typecheck.** That's a sign the contract change has wider blast radius than the surfacing slice should carry alone. Either carve a sibling slice for the contract fix and resume the original slice once it lands, or split the consumer fixes into batched commits with explicit acknowledgement of the broader scope.

Do not introduce default-value fallbacks (`x ?? defaultX`) at consumer sites to silence the symptom. That re-introduces the silent-failure mode in different clothing. Honest patterns: early return for the absent state, optional chaining for fields that gracefully render absence, or a narrowing guard hoisted to the integration point so leaf components stay strict.

## The UI-label cousin

The rule originated against `// deferred` code comments. The same three-component requirement applies to **user-visible labels** that announce future work:

- Dropdown rows labelled `(coming soon)` or `(deferred)` shipping behind a disabled state.
- "Available in a future release" empty-state messages.
- Tooltip hints saying "Feature X — not yet supported."
- Inline footer text on form sections: "Note — Y will be added later."

Each of these is a public promise that something specific is *not* shipped. Each one requires the same three components — why + tracking + acknowledgement — or it falls back to the same failure mode: a rationalisation dressed as a decision, surfaced only on audit.

### Concrete case

A surplus-allocation editor shipped four rule-kind rows in a target dropdown labelled `(coming soon)` / `(deferred - future portfolio release)`. The reachability-gate verifier failed the slice because the rows existed in the UI without slice-local tracking — no `proof.md § "Not delivered"` entry, no cross-reference to the canonical Rule 2 surfacing elsewhere in the project.

Remediation: `proof.md` was extended with a section enumerating each disabled row, cross-referenced to the existing canonical deferral docs. The labels were honest; the slice's own surfacing of them was missing.

### How to apply to UI labels

Same triple, same discipline:

1. **Why** — concrete reason the row / message / tooltip ships in its disabled / future-promise form.
2. **Tracking** — link to the slice, issue, or audit doc that owns the deferred work.
3. **Acknowledgement** — surfaced in the slice's `proof.md § "Not delivered"` for the slice that ships the label.

If the user-facing label promises work that isn't tracked, the label is dishonest in the same way a `// deferred` comment with no tracking is dishonest. Same remediation: track it or remove the promise.

## Symptoms to grep for

After any sprint that touches schemas, contracts, or public APIs:

```bash
grep -rn "deferred\|TODO\|FIXME\|later\|future" src/lib/ | \
  grep -v "\.test\." | grep -v "node_modules" | grep -v "\.next"
```

For each hit, ask: is this tracked, why, and was the user told?

## Provenance

The source project v0.5.0 audit (May 2026) found 6 silent deferrals in `src/lib/validation/src/schemas/` header comments. User's reaction when surfaced: "I'm not sure why these were deferred, I don't remember making that call." The pattern was rationalisation, not decision. The fix (this rule) makes the rationalisation impossible by demanding the three conditions up front.

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
verified -> [track integrator: deterministic post-sync invalidation only] -> failed_verification
```

State transitions:

- **Implementer can move**: `planned` → `in_progress`, `in_progress` → `implemented`, anything → `deferred` (with Rule 2 surfacing).
- **Verifier can move**: `implemented` → `verified` or `failed_verification`.
- **Track Integrator can invalidate**: `verified` → `failed_verification` only when canonical
  track-integration freshness composition proves that a recognized synchronization merge contributed
  an intersecting path after its latest authoritative frontier, supplies an exact trustworthy
  parent-2 rollback baseline, and the invalidated slice's complete candidate set is disjoint from
  every later authoritative slice. It
  preserves the append-only report ledger, clears the stale pinned head, sets
  `maintainability.state: re_slice_required`, records the deterministic violation, commits locally,
  and stops for `/replan-release`. It cannot append a Verifier report, repair code, push, or
  invalidate human-terminal `shipped` state. Unowned semantic commits, custom merges, or invalid
  provenance, or a later-slice candidate overlap BLOCK without a lifecycle mutation because bounded
  rollback cannot preserve the trustworthy baseline and later verified bytes simultaneously.
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

---
title: Rule 8 — Requirements Fidelity
description: The spec is not an axiom. Requirements are verified (quality), validated (sense-check), and traced (need -> AC -> test -> proof) so a need cannot drop silently between intake and spec.
---

# Rule 8 — Requirements Fidelity

## The rule

**The spec is not an axiom.** Before a slice enters implementation, its requirements must be:

1. **Verified** — each acceptance criterion is singular, unambiguous, complete, consistent, feasible, and verifiable (the ISO/IEC/IEEE 29148:2018 quality characteristics). A fresh-context gate checks this.
2. **Validated** — the requirement makes sense and serves the need. A human-owned scenario sense-check (positive AND negative) confirms the spec answers the right question, not just a well-formed one.
3. **Traced** — every need in the intake links to at least one acceptance criterion, every acceptance criterion links back to a need and forward to at least one test, and every slice links up a vertical golden thread (org objective → release benefit → slice, or the lightweight floor: slice → release goal).

A need that drops silently between intake and spec is a requirements-fidelity defect. The traceability matrix makes it visible and blocks the release.

## An acceptance criterion must be bounded

Verifiable (29148) is necessary but **not sufficient**: an acceptance criterion can *look* verifiable and be **unbounded**, and an unbounded criterion produces a **non-terminating verification loop** — it can only be failed again, never discharged.

**An acceptance criterion whose satisfaction cannot be enumerated is not verifiable, however verifiable it sounds.** If an AC quantifies over an open domain — *"no claim in the doc that the code contradicts"*, *"the system is secure"*, *"the API is consistent"* — it has no edge for verification to converge on. Bound it to a **named, enumerable set**, make each member machine-checkable, and declare everything outside the bound explicitly **non-normative**. An open-domain AC is not a criterion; it is an infinite regress with a checkbox.

**The honest-bounding test.** Correctly bounding an AC is simultaneously a **narrowing** (of the claim) and a **strengthening** (of the enforcement) — the claim covers less, but every member of what it now covers is actually checked. That combination is the signature of an honest bound; a bounding that only narrows (drops items to make the check pass) without strengthening (checking the ones it keeps) is a dodge, and the gate should treat the distinction as the test.

This is the front-half twin of Rule 12's scope-parity condition: Rule 12 forbids a *check* narrower than its claim; this forbids a *criterion* wider than any check can discharge. Both are the same root cause — a claim made wider than the evidence that backs it — caught at different layers.

**Evidence.** A slice failed fresh-context verification **seven times, and not one failure touched the AC's named items.** Every failure lived in an `in_scope` clause reading *"no claim in the doc that the code contradicts"* — which asks a prose document to be verifiably true about an entire monorepo. The guard suite sat 125-green while the document could still have claimed the wrong font and a non-existent component variant — two of the four things the AC named by name. The fix (bound the AC to its six named items, machine-check all six) narrowed *and* strengthened at once.

## Why

Rules 1, 6, and 7 verify **delivery against the spec** rigorously. They treat the spec itself as an axiom — the spec is the contract, and the verifier checks the code against it. But the spec can be wrong, incomplete, or disconnected from what the user actually asked for. The front half of the fidelity chain — from intake need to spec acceptance criterion — is unverified by the delivery rules. A perfectly implemented, perfectly verified slice that answers the wrong question is a fidelity defect no amount of delivery rigour will catch.

The gap is structural: the delivery rules are **downstream** of the spec. They cannot see upstream. Rule 8 closes the front half.

This is the same insight the README frames around requirements failure: decades of post-mortems converge on *poor requirements* — lost, drifted, met-technically-but-missed-the-intent — as the dominant cause of project failure. Rules 1–7 keep delivery honest; Rule 8 keeps the requirement itself honest before delivery begins.

## The 2-D requirements traceability matrix (RTM)

The RTM is the enforcement mechanism. It has two axes and threads through the existing artefacts — no separate datastore.

### Horizontal: intake need → slice → acceptance criterion → test → proof

```
intake.md          status.json         spec.json               spec.json             proof.json
--------           ------------        --------               --------            --------
N-01: need  --->   covers_needs:  -->   - [ ] AC cites N-01    Required tests  ->  test results
                   [N-01, N-03]         - [ ] AC cites N-01                        reachability
```

- **Needs** are enumerated with stable ids (`N-01`, `N-02`, …) in `intake.md`. The planner assigns ids at planning time; they are never reused.
- **Slice coverage** — every slice declares which intake needs it delivers in `status.json` `covers_needs` (array of need IDs). This is the intake→slice link: a deterministic gate can verify every N-NN appears in at least one slice, and no slice claims a need it doesn't cite in its ACs.
- **Acceptance criteria** in each `spec.json` cite the need id(s) they satisfy, inline in the AC text (e.g. "WHEN … THE SYSTEM SHALL … (N-01)").
- **Required tests** in `spec.json` cite the acceptance check they exercise.
- **Proof** in `proof.json` closes `AC → test → proof` (already required by Rule 6).

The RTM now closes the full chain: `intake need → slice → AC → test → proof`. An orphaned need (no slice covers it, or no AC cites it), an orphaned AC (cites no need, or cites a need but has no test), or a slice that claims a need it doesn't cite in its ACs (mismatch) is a broken trace.

### Vertical: org objective → release benefit → slice

```
org objective  --->  release benefit  --->  slice
(optional)           (board.json)           (status.json)
```

- **Org objective** is opt-in. A solo founder or small team may have no declared objective — the vertical floor is `slice → release goal`.
- **Release benefit** is the value the release delivers, recorded in `board.json`.
- **Slice link** is the slice's contribution to the release benefit, recorded in `status.json`.

The vertical trace is the golden thread: line-of-sight from strategy (if declared) through release value to individual slices. For solo/small teams the floor is lightweight: `slice → release goal` satisfies the vertical trace without an org-objective link.

## Enforcement

A deterministic, fail-closed **trace gate** (reference implementation: `sworn trace <release-name>`) builds the matrix from `intake.md` / `spec.json` / `status.json` / `board.json` alone. It exits 0 on a fully-traced release, non-zero with enumerated violations on any break.

The gate checks:

- **Orphaned need** — an intake need ID (N-NN) that appears in no slice's `covers_needs`. The intake→slice gap.
- **Invalid covers** — a slice's `covers_needs` references a need ID not in intake.md.
- **Unclaimed coverage** — a need ID in `covers_needs` with no AC in that slice's spec citing it. The slice→spec gap.
- **Free-form AC** — an acceptance check that lacks the EARS `shall` keyword and has no `NOTE:` escape. The AC→structure gap.
- **"See intake" reference** — any spec content that refers the implementer to intake.md. The spec must stand alone.
- **Vague AC / scope** — an AC or in-scope item describing no concrete artefact (file, testid, status code, label string, value). The content-density gap.

Run the trace gate at two points in the workflow: (a) planner Phase 6 before handoff, and (b) as the DoR gate at `planned → in_progress`. A release that fails the trace may not ship.

## EARS notation — structured acceptance criteria

The RTM enforces *traceability* (need → AC → test). EARS (Easy Approach to Requirements Syntax) enforces *structure* — each acceptance criterion follows a fixed keyword pattern, not free-form prose. Together they form the front-end fidelity gate: traced AND well-formed.

EARS was developed at Rolls-Royce PLC in 2009 (Mavin et al., IEEE RE'09) and is used worldwide by Airbus, Bosch, Dyson, Honeywell, Intel, NASA, Rolls-Royce, and Siemens.

A deterministic gate classifies every acceptance check in every slice's `spec.json` by EARS pattern and fails closed on any free-form check that matches no pattern, naming the slice and the offending line.

| Class | Pattern | Keywords | Example |
|---|---|---|---|
| Ubiquitous | `<system> shall <response>` | none (always active) | `The API shall return 200 for valid input.` |
| Event-driven | `When <trigger>, <system> shall <response>` | `When` | `When the user clicks Save, the form shall persist to the backend.` |
| State-driven | `While <state>, <system> shall <response>` | `While` | `While the modal is open, the page shall not scroll.` |
| Optional-feature | `Where <feature>, <system> shall <response>` | `Where` | `Where Premium is enabled, the export button shall be visible.` |
| Unwanted-behaviour | `If <condition>, then <system> shall <response>` | `If … then` | `If the database is unreachable, then the API shall return 503.` |
| Complex | Two or more keywords combined | e.g. `While … When …` | `While on mobile, when the user taps Edit, the settings sheet shall open.` |

ACs that use no EARS keyword pattern and no `NOTE:` escape are free-form and fail the gate. The `<system>` slot can be implicit (e.g. "the page", "the API", "the component") or omitted — the litmus is the keyword + `shall` structure, not the specific system noun.

## Spec-quality metric — pre-code soundness + completeness

Before a spec reaches verification or validation, a deterministic, pre-code first-pass computes soundness + completeness from a slice's **acceptance examples** alone — no source code, no model call.

### Structural completeness (the sniff-test gate)

The RTM verifies *traceability* (every need has an AC, every AC has a test) but not *content-density*. A spec can pass traceability while being a thin shadow of its intake section — "fix the windfall bug" passes the EARS check but captures none of the detail the intake elaborated. This is the decomposition-fidelity failure mode: the planner splits intake into slices but fails to decompose the intake-level description into spec-level precision.

Intake is the epic level — broad user outcomes, "what the human wants" in natural language. The spec is the feature/story level — decomposed into concrete, verifiable, implementation-precision acceptance criteria. "Replicate intake detail" is the wrong framing; the spec must *refine* intake detail into finer granularity. Intake says *what* (ticker search); the spec says *where* (`PortfolioEditor.tsx`), *how* (`<TickerSearch />` with `accessToken` prop, Name field `disabled={true}`), and *proves* (testids, status codes, screenshot paths).

A structural-completeness check runs at the `planned → in_progress` transition and fails closed on:

1. **Vague-scope spec** — an AC or in-scope item that could describe *any* slice of its kind ("fix the bug", "add the missing code", "wire up the component"). Every AC must name at least one concrete artefact (a file path, a label string, a data-testid, an assertion value, an HTTP status code). A spec without concretes is a spec that can't be verified — the verifier has nothing concrete to check against.
2. **Missing detail** — a behavioural detail present in the intake's "What the human wants" section for this slice's scope that has no corresponding AC, in-scope item, or planned touchpoint in the spec. A single unmatched intake detail fails the gate.
3. **"See intake" reference** — any spec content that refers the implementer to intake.md (directly or indirectly: "see intake", "refer to intake", "as described in the intake"). The spec owns every detail it covers.

This is a deterministic gate, not a model call — it checks for concrete terms (file paths, quoted strings, testids, status codes) and cross-references intake detail against spec content. A spec with no concretes or missing intake detail never reaches implementation.

### Numeric completeness (mutation analysis)

Every spec SHOULD carry a `## Acceptance examples` section with one or more **input → expected-output** pairs per acceptance check:

```
## Acceptance examples

- name: "valid-ears-pass"
  input: "a release where every AC matches an EARS pattern"
  expected: "the AC lint exits 0 and prints the per-pattern distribution"
- name: "free-form-fail"
  input: "a release with at least one free-form AC"
  expected: "the AC lint exits 1 naming the slice and line"
```

- **Soundness** — for each example, the expected output must be consistent with the acceptance criteria (the criteria must not reject a valid output). A limited deterministic check that flags contradictions like "expects failure where criteria describe only a pass case."
- **Completeness (mutation analysis)** — deterministic mutation operators are applied to the expected output (flip exit codes, negate assertions, remove keywords) and the gate checks what fraction the criteria would reject. The score is `caught / total`; below the threshold (default 50%) the gate fails closed.

Because it is the cheapest check (deterministic, no model, no human), spec-quality runs first. A spec with no acceptance examples or low completeness never reaches model-based verification.

## Validation — human-owned sense-check

Validation answers "are we building the *right* requirements?" — does the spec make sense and serve the need (distinct from verification's "are the requirements well-formed?"). This is the cheapest defect-catch point and is **human-owned**: the model drafts scenarios + a benefit hypothesis; the human ratifies. Spec validation has no oracle but the user, so this gate is never model self-certified.

Every slice carries a validation record in its `status.json`:

| Field | Required | Description |
|---|---|---|
| `human_ratified` | Yes | Must be `true`. Model-only validation is not a pass. |
| `ratified_by` | Yes | Who ratified (human identifier). |
| `ratified_at` | Yes | When ratified (ISO 8601). |
| `positive_scenarios` | Yes (≥1) | Scenarios where the requirement works as intended. |
| `negative_scenarios` | Yes (≥1) | Edge + failure flows; what should *not* happen. |
| `benefit_hypothesis` | Yes | This slice's benefit and its vertical link (slice → release benefit → objective). |

A deterministic gate fails closed on a missing record, model-only ratification, empty positive or negative scenarios, or a blank benefit hypothesis.

## Definition of Ready

The Definition of Ready (DoR) is the gate every slice passes before it can transition from `planned` to `in_progress`. It composes the three checks into a single fail-closed verdict:

1. **Traced** — the RTM verifies complete traceability (horizontal + vertical).
2. **Verified** — every acceptance criterion passes the 29148 quality-characteristic check via a fresh-context model pass.
3. **Validated** — the slice carries a human-ratified validation record.

If any gate fails, the transition is blocked and the failing gate(s) named. If any gate cannot be evaluated (missing artefact, no verifier model configured), the transition is also blocked — fail closed. There is no bypass: an explicit human re-plan is the only way to change a spec, never a silent skip.

## Relationship to existing rules

| Rule | What it does | How Rule 8 complements it |
|---|---|---|
| Rule 1 — Reachability Gate | Tests exercise the integration point | Rule 8 ensures the integration point is the *right* one — traced to a need |
| Rule 2 — No Silent Deferrals | Surfaces drift explicitly | Rule 8 makes a dropped need a hard, detectable trace break |
| Rule 6 — Proof Bundle | Closes AC → test → proof | Rule 8 adds the front half: need → AC. Together they form the full horizontal chain |
| Rule 7 — Adversarial Verification | Fresh-context verification of delivery | Rule 8 verifies the spec itself, before delivery verification runs |

## When this rule applies

- Any release with an `intake.md` that declares needs. The RTM is the enforcement; the planner constructs the trace as a by-product of planning.
- The `planned → in_progress` transition (Definition of Ready) gates on the RTM, verification, and validation all passing.

## When this rule does NOT apply

- Spikes or exploratory work without a release intake.
- A release with no declared needs (the RTM reports an empty matrix and exits 0 — no needs means no traces to break).

## Provenance

Rule 8 was introduced in the `2026-06-16-fidelity-layer` cycle. It closes the "front half" fidelity gap surfaced during the v0.5.0 cycle: the delivery rules (1/6/7) verify code against spec, but nothing verified the spec against the need. The RTM is the keystone — it threads through existing artefacts and enforces traceability fail-closed, so a need cannot drop silently between intake and spec.

---
title: Rule 9 — Design Fidelity
description: Meeting a requirement is not the same as the right solution for the whole. Design stays human-owned and AI-augmented, with the amount of human judgement calibrated to each choice's stakes (reversibility x blast-radius).
---

# Rule 9 — Design Fidelity

**Meeting a requirement is not the same as the right solution for the whole.** Solution fit is a quality the delivery verifier (Rule 7) cannot see — the verifier checks the diff against the spec, but the spec does not encode whether *this* design was the right one for the system. Rule 9 keeps design **human-owned**, AI-augmented, and calibrates how much human judgement each choice demands by its stakes.

## Classification: stakes = reversibility × blast-radius

Every design choice has a **stakes class**:

| Class | Reversibility | Blast radius | Decision requirement |
|---|---|---|---|
| Type-1 | Hard to reverse | Wide / structural | Full human decision with options + rationale recorded |
| Type-2 | Easy to reverse | Narrow / local | AI may proceed with a noted default |

**Architecturally-significant choices are always Type-1**, regardless of other factors. A choice that shapes the whole system, the data model, the deployment architecture, or an external contract is architecturally significant — and therefore Type-1 — even if it feels locally reversible.

The Type-1/Type-2 split is the well-known "one-way vs two-way door" heuristic applied per choice. Its purpose is to spend scarce human attention where it matters: forcing a human decision on every trivial reversible call drowns the genuinely consequential ones.

## Option surfacing

When the planner reaches a design choice during planning:

1. The planner drafts **at least two options** with trade-offs and prior art.
2. For Type-1 choices, the human selects one and records the decision + rationale in the slice's `status.json`.
3. For Type-2 choices, the planner records a noted default and proceeds.

The model may propose options, classify stakes, and surface trade-offs — but for a Type-1 choice the model **may not record the human decision itself**. (This is the design-time analogue of Rule 7: the agent that proposes is not the authority that decides.)

## Prevalence is not correctness

**"Most of the code already does X" is a reason X spread. It is not a reason X is right.** A design or architecture decision ratified on prevalence **launders an existing defect into an official standard** — it takes something that was drifted into, never chosen, and stamps it as the contract every future slice must conform to.

When proposing a decision from an audit or inventory:

1. **Separate the prevalence finding from the recommendation.** "60 files do X" and "we should standardise on X" are two different claims; the first does not establish the second. State them separately so the recommendation must stand on its own argument.
2. **Run the domain's quality floor on the incumbent before ratifying it** — contrast and touch-target size for UI, latency for a query pattern, correctness for an algorithm, whatever the floor is. If the incumbent fails that floor, prevalence becomes an argument **for** change, not against it.

**The tell:** any decision whose rationale is *"this ratifies reality"*, *"it follows the code's gravity"*, or *"it minimises migration"*. Those are **cost arguments dressed as design arguments**. They may still be right — migration cost is real — but they must be argued **on cost, openly**, not smuggled in as correctness. This is the structural failure mode of every codification or consolidation effort: the audit is necessary (you cannot uplift what you cannot see) but it is **descriptive**, and description cannot distinguish a convention from a bug when the only evidence is "most files do this."

This pairs with Rule 9's human-ownership stance and the same root cause the sibling rules catch (a claim made wider than its evidence — here, "prevalent" widened into "correct"). **The machine can prove a colour fails a contrast ratio; it cannot notice that a button feels like it is shouting** — and in the motivating case those turned out to be the same defect.

**Evidence.** A Type-1 decision ratified the source project's primary button colour, explicitly reasoned as *"follow the code's actual gravity: 60 files already do this."* White text on that colour measures **3.29:1; WCAG AA requires 4.5:1.** The decision would have made a button whose own label fails accessibility the official design standard. It was caught only because the human said *"it's too loud"* — and the loudness and the contrast failure were the same defect.

## Record format

Each design decision is an entry in `status.json`:

```json
{
  "design_decisions": [
    {
      "choice": "database-engine",
      "stake_class": "Type-1",
      "options": ["PostgreSQL", "SQLite"],
      "human_decision": "PostgreSQL",
      "rationale": "migrations matter and we already have the infra",
      "architecturally_significant": true
    }
  ]
}
```

## Enforcement

A deterministic, fail-closed gate reads each slice's `design_decisions` and checks:

1. Every Type-1 choice has a non-empty `human_decision` field — otherwise it violates, naming the slice + choice.
2. Every `architecturally_significant` choice is classified Type-1 — otherwise it violates, naming the slice + choice.

This is the design-time counterpart to the delivery first-pass: cheap, deterministic, and run before model or human review time is spent.

## Autonomous-mode gate semantics

The design-review gate's **human-in-the-loop** behaviour is well-defined: the implementer produces the Design TL;DR and halts at `design_review`; a captain surfaces pins; the Coach acknowledges before code lands. Its **autonomous** behaviour — when no human Coach is present at dispatch time (an unattended `loop` run) — must be defined too, or "autonomous" silently downgrades design review to a no-op. That is the exact fidelity gap Rule 9 exists to close (2026-07-12 dogfood, finding 6: an autonomous loop generated the Design TL;DR and proceeded straight to implementing, because the gating captain dispatch had deferred out).

The gate's autonomous behaviour is **keyed to the stakes class it already computes**:

- **Type-2 choices** (and slices with no Type-1 choice): auto-proceed is permitted, provided the noted default is **recorded** in `status.json` exactly as a human-attended run would record it. A reversible, local choice does not need a human present.
- **Type-1 / architecturally-significant choices**: by **default** the loop **must hard-pause and surface** the decision for asynchronous Coach acknowledgement (page/notify + halt at `design_review`); it does not auto-proceed, and a captain-role self-review may *enrich* the pins the Coach later sees but may not *clear* the gate. This is the safe default: an autonomous loop treating "no human here right now" as licence to auto-authorise a one-way-door choice is the exact silent downgrade Rule 9 exists to prevent.

**The default is overridable — delegation, not abdication.** How much autonomous authority is appropriate is not universal: it scales with the maturity of the codebase and its tooling, the capability of the driving model (a frontier model can be trusted with more than a weak one), and the operator's risk tolerance. So an operator **may** grant an autonomous loop authority to auto-proceed on Type-1, via an explicit, recorded governance setting — never a flag an agent can set for itself. This preserves Rule 9's core principle intact: **the human still decides *whether*; they decide it once, ahead of time, as a standing delegation, rather than per-choice.** The authority to auto-proceed is itself a Type-1 human decision — recorded when the envelope is set, not invented at dispatch.

An `autonomous_design_authority` setting (project governance config, e.g. `docs/baton/design-fidelity.json`) declares the envelope:

| Value | Type-1 behaviour when no human is present |
|---|---|
| `hard_pause` (default) | Halt at `design_review` and page; never auto-proceed. |
| `auto_proceed_recorded` | The loop may choose and record the Type-1 decision, attributed to the standing delegation. |

The setting must name **who delegated** and **the envelope's rationale/scope** (the audit trail: *"Coach Brad, 2026-07-12 — Fable-5-driven runs on this repo may auto-proceed on Type-1; revisit at v1.0"*), and an operator may narrow it — e.g. gate `auto_proceed_recorded` on the driving model meeting a capability bar (tie-in: [capability-policy](capability-policy.md)), or on specific choice domains. Whatever the envelope, the invariants below always hold, so autonomy is never a silent no-op:

- Every auto-proceeded Type-1 choice is **fully recorded** in `status.json` — options, the chosen option, rationale, and the delegation it was authorised under — so it is auditable after the fact exactly as a human-attended decision would be.
- With `hard_pause` (or no setting), a Type-1 choice with no human decision **blocks**, exactly as the deterministic gate already enforces. Fail closed is the default; opting out is an explicit, recorded, human act.

## Design-system input (UI-bearing projects)

### Canonical architecture — the source of truth

LLMs are optimisers: they produce working code but not necessarily well-architected code. Without explicit constraints, every slice reinvents patterns. The antidote is canonical architectural documents — the source of truth that every slice conforms to.

A project declares its canonical docs in `docs/baton/architecture.json` `canonical_docs`:

```json
{
  "canonical_docs": {
    "data_model": "docs/data_models/SCHEMA.md",
    "api_contracts": ["docs/api/openapi.yaml"],
    "component_hierarchy": ["packages/ui/README.md"],
    "architectural_decisions": "docs/adrs/",
    "design_tokens": "tokens.json"
  }
}
```

The planner consults these during Layer 4 discovery and flags gaps. The architecture audit script checks slice diffs for conformance: new entities must match the canonical schema patterns, new components must extend (not duplicate) the component hierarchy, API changes must follow the established contract shapes.

If a project lacks any of these documents, the planner MUST flag it. A project with no canonical data model is a project where every slice invents its own — the accumulated divergence is exponentially more expensive to fix than the upfront cost of defining the schema. Recommend creating missing canonical artefacts as a pre-release or parallel planning activity.

### Design-system input (UI-bearing projects)

Design fidelity for a UI requires a declared source of truth. Every UI-bearing project declares its design system before design conformance can be audited. The design system is a three-tier concept:

| Tier | Name | Role |
|---|---|---|
| Umbrella | **Design system** | The whole declared input — token source + component library |
| Atoms | **Design tokens** | The named-value source of truth (colours, spacing, typography) |
| Reusables | **Component library** | The coded, reusable UI components |

A project config carries an optional declaration:

```json
{
  "ui_bearing": true,
  "design_system": {
    "token_source": "tokens.json",
    "component_library": "packages/ui"
  }
}
```

- `ui_bearing: true` with no design-system declaration = fail closed (conformance cannot proceed without a source of truth).
- `ui_bearing: false` or absent = not applicable. CLI projects and non-UI tools are exempt.

## Design-system conformance audit

A two-layer conformance audit guards UI-bearing projects against design drift.

### Layer 1 — Deterministic first-pass (machine-check)

The mechanical gate is the design-conformance gate (reference implementation: `sworn designaudit`) — run by the verifier as Gate 6 of the verification workflow. It scans UI files in the slice's diff for:

| Category | Pattern | Detection |
|---|---|---|
| **Hardcoded colour** | Hex `#ff0000`, `rgb()`, `hsl()` | Regex scan of diff; compared against declared design tokens |
| **Off-scale spacing** | Hardcoded `px`/`rem` values off the spacing scale | Requires token config with spacing scale |
| **Recreated component** | Duplicate primitive impl outside component library | Requires component library path mapping |

**Escape hatch.** Three levels of accepted deviation:

1. **Per-line allowlist.** `design-allowlist.json` in the slice folder, maps `file:line` patterns to rationale. The script reads it automatically. For pre-existing violations an implementer cannot fix (e.g. legacy code outside slice scope).
2. **Rule 2 deferral.** Listed in `proof.md` "Not delivered" with all three Rule 2 elements: why (pre-existing, out of scope), tracking (slice or issue), and **explicit human or captain acknowledgement**. The verifier reads `proof.md` and accepts the deferral.
3. **Per-project token config.** Declared in `docs/baton/design-fidelity.json` with `token_source` pointing to the design-token file. Colours matching declared tokens pass automatically; only undeclared colours flag.

The script exits 0 on clean pass, non-zero with `file:line [kind] value` violations. Projects without a design-fidelity config (`ui_bearing: false` or absent) pass automatically.

### Layer 2 — Human cohesion verdict (human-owned)

The deterministic pass cannot assess whether the overall design *feels on-brand* — typography consistency, visual rhythm, spacing coherence. That judgement is human-owned. The audit will **not** auto-pass cohesion; it requires a human-set `on-brand` / `off-brand` verdict to reach exit 0. A clean machine pass with no cohesion verdict stays blocked.

## Relationship to existing rules

| Rule | What it does | How Rule 9 complements it |
|---|---|---|
| Rule 7 — Adversarial Verification | Verifies the diff against the spec | Rule 9 governs the choice the spec doesn't encode — *was this the right design* |
| Rule 8 — Requirements Fidelity | Verifies the requirement is right | Rule 9 assumes the requirement is already validated and governs the solution's fit |
| Rule 2 — No Silent Deferrals | Surfaces deferrals explicitly | Rule 9 makes an unrecorded Type-1 decision a hard, detectable gate failure |

## When this rule applies

- Any slice that makes a design choice with structural reach or hard-to-reverse consequences.
- Any UI-bearing project, for the design-system conformance audit.

## When this rule does NOT apply

- Purely local, easily-reversed implementation choices (Type-2) — a noted default is sufficient.
- Non-UI projects, for the conformance-audit half (the stakes-classification half still applies).

## Provenance

Rule 9 was introduced in the `2026-06-16-fidelity-layer` cycle alongside Rule 8. It closes the design half of the fidelity gap: Rule 8 ensures the requirement is right; Rule 9 ensures the solution chosen to meet it is right for the whole — a quality the delivery verifier structurally cannot assess from the diff.

---
title: Rule 10 — Customer Journey Validation
description: Critical customer journeys are a first-class artefact — AI-drafted, human-ratified, version-controlled, fail-closed on absence or staleness. A journey is the unit of end-to-end evidence, and a journey walked over a mocked boundary proves nothing.
---

# Rule 10 — Customer Journey Validation

## The rule

**Critical customer journeys are a first-class artefact, not a per-release afterthought.** Before a release can ship, its customer journeys must be:

1. **Elicited** — the model drafts candidate critical journeys from the app. No draft means no journeys gate.
2. **Ratified** — a human reviews, edits, and ratifies the journeys. Model-only journeys are unratified and fail the gate.
3. **Durable** — journeys are persisted to a version-controlled artefact that survives session boundaries and is maintained release over release.

A journey is an ordered, end-to-end path a user type takes across the app to achieve an outcome. It is the unit of end-to-end evidence: if a release changes a user-visible surface, the journey that crosses that surface must be updated.

## Why

Rules 1, 6, and 7 verify **delivery against the spec** within a single slice. A slice spec scopes one slice, one outcome. A critical customer journey crosses many slices — it is the full path a user takes. If release work changes a surface a journey crosses, the journey (not just the slice) must be re-verified.

Journey validation sits at a different level of abstraction from slice verification:

| Artefact | Scope | Owned by | Gate |
|---|---|---|---|
| Slice spec | One slice, one user-reachable outcome | Planner + Verifier | Rule 7 (adversarial verification) |
| Journey | End-to-end user path across many slices | Human + Model | Rule 10 (elicitation + ratification) |

A slice that passes Rule 7 but leaves a journey stale is an integration defect no per-slice gate catches. Journey validation closes that gap: Rule 7 verifies the parts; Rule 10 verifies the assembled whole.

## The journey artefact

The journeys artefact is a version-controlled JSON document at a stable project path. It contains:

- **Version** — schema version for forward compatibility.
- **Journeys** — the list of critical journeys, each with an **id** (e.g. `J01-onboard-new-user`), a **user_type** (e.g. `free_user`, `pro_user`, `admin`), an **outcome** (what the user achieves), ordered **steps**, and an **entry_surface** (where the journey begins).
- **Regression + boundary metadata** (per journey, added in v0.7.0) — `regression_test_path`, the path to the regression test asserted by `has_regression` (present once `has_regression` is true); and `no_mock_boundary`, the entitlement/infra boundary this journey must cross against **real** infrastructure when walked (its absence means no boundary is declared for the journey). `no_mock_boundary` is the machine-readable home of the "No-mock boundary" enforcement below — the gate reads it to know which boundary a walk may not mock.
- **Ratification metadata** — `is_ratified`, `ratified_by`, `ratified_at`.

## Enforcement

A deterministic, fail-closed gate reads the journeys artefact and returns:

- **Exit 0** — artefact exists and is human-ratified; the journeys are listed.
- **Exit 1** — artefact is missing (elicitation not run) or exists but is unratified.
- **Exit 2** — unrecoverable error (parse failure, I/O error).

The gate is additive — it runs alongside per-slice verification (Rule 7), after all slices satisfy track-mode's canonical integration-ready predicate but before the release merges. It does not replace any existing gate.

## No-mock boundary — the enforcement that makes a journey count

Journey validation exists to prove the **assembled system actually works end-to-end**. A journey walked over a *mocked* boundary proves nothing — the mock answers however the test author wired it to, not however the real system would. So the no-mock boundary is **constitutive of Rule 10, not a detachable add-on**: it is the enforcement that makes a walked journey count as proof.

The artefact and the gate are not two rules that happen to compose — they are one rule's two faces. The journey says *what* end-to-end path must work; the no-mock gate guarantees the walk that proves it didn't cheat at the boundary. A journey whose boundary is mocked is a journey that has not been validated at all, regardless of a green test.

**The validated boundaries** are: database (DB), authentication (auth), and entitlement (premium/subscription tier) — the integration points where a mock most easily hides a journey that doesn't really work.

**The constraint.** On an environment wall — when real infrastructure at a validated boundary cannot be reached — the implementer must **stop and surface the blocker**, never mock around it. A mock/stub/fake at a validated boundary is permitted only if it is a declared deferral with all three Rule 2 elements (why + tracking + acknowledgement). An *undeclared* boundary mock is an undeclared silent deferral and fails the gate closed.

This reads as a Rule 2 concern too — an undeclared mock is a species of silent deferral — but its home is Rule 10, because the specific failure it prevents is *a journey that lies about working*.

**What "mock at a boundary" means — a code construct, not a string.** A mock at a validated boundary is a **code construct**: a call, binding, or type that *substitutes* the real boundary (a fake `sql.DB`, a stubbed auth client, a hand-rolled entitlement double). It is not the mere textual appearance of the words `mock` / `fake` / `stub` / `@no-mock`. The distinction is load-bearing: code that legitimately *handles the boundary-mock vocabulary* — a slice whose job is to parse `// @no-mock` / `// @mock-boundary` annotations — contains those tokens inside **string literals and comments** without mocking anything (2026-07-12 dogfood, finding 5: an assemble slice failed its own gate closed because a string literal `"// @no-mock\n// @mock-boundary (boundary: entitlement)"` matched the detector).

**Detection (deterministic first-pass).** A diff-scanning check flags **code tokens** — spans that are neither string literals nor comments (AST-level, or a lexer that skips string/comment spans) — combining a mock/stub/fake construct with a validated-boundary reference (`sql.DB`, `auth`, `premium`, …). Occurrences inside string literals or comments are **not** mocks and do not trip the gate. If a flagged construct matches an open declared deferral, it is surfaced as a known deferral; otherwise it is an undeclared boundary mock and the gate exits non-zero, naming the offending construct and boundary. The gate stays fail-closed on real substitutions while not penalising code that handles the annotation vocabulary.

**When the no-mock gate applies:** every slice whose diff introduces, uses, or constructs a mock/stub/fake at a validated boundary. **When it does not:** pure unit-test mocks that touch no validated boundary (a mock calculator, a mock string formatter), and the human walkthrough itself, where mocks are fully off and real journeys run against real infra.

## Mock parity at registered contract boundaries (sub-rule)

The no-mock boundary above activates at release level, at the validated infra boundaries (DB, auth, entitlement). This sub-rule applies the same principle **earlier and lower**: at slice-implementation time, at every boundary registered in the release's contract registry (`contracts.json`, `contracts-v1` schema).

**The sub-rule.** A consumer slice may mock a registered boundary **only with fixtures recorded by the owner's live contract test.** The owner's proof bundle commits actual request/response pairs captured from its passing live test (`fixtures/` in the slice folder, path recorded in the registry entry). Consumer tests load those fixtures as mock data. A hand-written mock at a registered boundary is a silent deferral (Rule 2) unless the consumer includes at least one unmocked in-process round-trip against the real handler.

**Why.** A mock and a spec written from the same assumption share the same blind spot. The observed failure (2026-07-10): a consumer slice passed legitimate fresh-context adversarial verification with a latently-400 PUT, because its mock and its spec both encoded the spec author's wrong body-shape assumption — implementer, tests, and verifier were structurally blind together. Owner-recorded fixtures break the symmetry: the fixture can only contain what the real handler actually accepted, so a wrong consumer assumption goes red at the consumer's own test run instead of surviving to assembly.

**Freshness invariant.** Fixtures are regenerated by the owner's live contract test on green, and must be **newer than the owner's last production-code change to the surface**. A stale fixture is treated as no fixture: drift between handler and fixture must break the consumer's tests visibly, never silently re-agree with an outdated shape. (How freshness is checked is the gate's concern; the invariant is the rule.)

**Mechanics are file-based, inside the existing artefact model** — pact-style without a broker: owner test writes `fixtures/<surface>.json` `{request, response}`; the registry's `fixtures` field points at it; the consumer's mocks import from that path (a grep-level check suffices to start).

**Status: advisory until the grading gate ships.** Per the skew-window policy (baton#59), planners and implementers follow this discipline now, enforced by review; it flips to fail-closed when the reference gate (`sworn lint contracts` fixture checks) ships.

## The assembly stage

Per-slice verification proves the parts; the journey walk proves the whole — but between them sits the assembled system, and every decisive 2026-07-10 failure (a CORS preflight no unit, handler, or per-slice test could see) was caught by an assembly phase that existed only as orchestrator improvisation. Rule 10 makes it first-class: the machine half of validating the assembled whole, run **before** the human half.

**Release-level chain:** `tracks-merged → assembled → journey-validated → merged`.

- **`assembled` is a derived state, not a stored one** (the same invariant that keeps `board.json` a pure plan): a release is `assembled` when `docs/release/<name>/assembly-proof.json` (schema `assembly-proof-v1`, `https://baton.sawy3r.net/schemas/assembly-proof-v1.json`) exists on `release-wt/<name>` with `verdict: "pass"`. No record stores the state; the artefact's existence and verdict are the state.
- **The assembly run** (reference implementation: `sworn assemble <release>`) brings up the stack from the release worktree, runs the release's deferred end-to-end set — no-mock, serially, with verified teardown — and emits `assembly-proof.json` (per-suite results, boundary/preflight observations, screenshot paths, and the authoritative verdict). Non-zero exit on any non-excepted failure. The record is structurally fail-closed on the two things a runner most easily drops silently: an unexplained non-pass result (a `fail`/`skip` must carry a disposition, and any excepted disposition must carry tracking — Rule 2), and an undeclared server teardown after the stack was brought up (Rule 11 guaranteed-restore).
- **The human walk comes after.** The touched journeys are re-walked against real infra **after the assembly run passes**, not merely "after all slices verify" — the machine half catches the wire-level seams (the CORS class) so the human walk spends its attention on journey semantics, not transport failures.
- **`/merge-release` gates on `assembled`** the way it gates on per-slice `verified`. Until the reference implementation ships, the gate is advisory (a missing `assembly-proof.json` is a surfaced warning, a failing one is a hard block); it flips to fail-closed when `sworn assemble` ships (baton#59 skew-window policy).

## The cutover QA runbook

The assembly run is the machine half of validating the whole; the human walk is the other half. Between them sits a gap: the human re-walking the touched journeys needs to know *what changed in this release and how to check it* — and reconstructing that by hand, per release, is the step that does not scale as agent throughput rises. The **cutover QA runbook** closes it: a generated, human-facing walk of exactly what this release changed and how to verify it, that **targets** manual QA rather than replacing it.

**It is a rendered view, not new data.** Every input already exists as an artefact the loop produced; the runbook aggregates and renders them (the same records → view move `index.md` makes from `board.json`):

- each verified slice's **reachability** smoke-step (Rule 1) — "open X, do Y, observe Z";
- the **journeys** this release touched (this doc's artefact);
- each slice's **`delivered`** list (Rule 6 proof bundle);
- the **new wire surfaces** the release introduced (`contracts-v1`), as spot-checks.

Rendered into one targeted walk grouped by touched journey: what changed, how to check each thing, the expected result.

**Where it sits in the chain.** `assembly-proof.json` is the machine's end-to-end pass; the runbook is the **guided human pass**; the **attestation** (`attestations-v1`) is the signed output. The runbook is the *input a human walks*; the attestation is the *output they sign* — they pair, they do not duplicate. A human walking a targeted runbook spends attention on journey semantics, not on rediscovering the diff.

**Form and rendering.** `qa-runbook.md` is **prose, rendered from the records** (human-facing, so Markdown never parsed for a decision — the records-vs-prose rule). It is emitted at cutover, after the assembly run passes, by the reference implementation (`sworn`) or the orchestrator. It is **advisory**: an aid to the human walk, not a new fail-closed gate; the attestation remains the gating artefact and may reference the runbook it was walked against.

## Workflow

1. A maintainer runs journey elicitation against the project.
2. The model drafts candidate journeys from the project structure.
3. The human reviews, edits, and ratifies the artefact (`is_ratified=true`, `ratified_by`, `ratified_at`).
4. The journeys gate passes; the artefact is committed and maintained as the project evolves.
5. After all tracks merge to `release-wt/<name>`, the assembly run executes and emits a passing `assembly-proof.json` (the release is now `assembled`).
6. The **cutover QA runbook** (`qa-runbook.md`) is rendered from the release's records — a targeted walk of what changed and how to check it.
7. At release cutover, the journeys that the release touches are re-walked against real boundaries (no-mock) **after the assembly run passes**, guided by the runbook, and the walkthrough is human-attested before ship.

## Relationship to existing rules

| Rule | What it does | How Rule 10 complements it |
|---|---|---|
| Rule 1 — Reachability Gate | Tests exercise the integration point | Rule 10 ensures the integration point's journey is documented and re-walked |
| Rule 2 — No Silent Deferrals | Surfaces deferrals explicitly | An undeclared boundary mock is a silent deferral caught by Rule 10's no-mock gate |
| Rule 6 — Proof Bundle | Closes AC → test → proof per slice | Rule 10 adds cross-slice journey evidence |
| Rule 7 — Adversarial Verification | Fresh-context verification of one slice | Rule 10 verifies the end-to-end paths that span slices |
| Rule 8 — Requirements Fidelity | Need → AC → test → proof horizontal trace | Rule 10 adds the vertical journey trace across the release |

## When this rule applies

- Any release that changes a user-visible surface (UI, API, CLI command, form, route).
- Pre-release cutover — the journeys gate runs after all slices are integration-ready but before the release merges.

## When this rule does NOT apply

- Infrastructure-only releases with no user-visible change.
- A release with no ratifiable journeys (the tooling produces a minimal set; the human may ratify that minimal set).

## Provenance

Rule 10 was introduced in the `2026-06-16-fidelity-layer` cycle. It closes the integration gap above per-slice verification: a release of individually-verified slices can still leave a cross-slice user path broken or stale. The no-mock boundary is folded in as Rule 10's enforcement teeth — the recognition that an end-to-end journey only counts as evidence if it ran against real boundaries, not mocks.

---
title: Rule 11 — Process-Global Mutation Guard
description: Any change that mutates process-global state (working directory, environment, or which worktree/branch a tool acts on) must guarantee restore, assert the target before acting, and show a reachability artefact proving the guard.
---

# Rule 11 — Process-Global Mutation Guard

## The rule

Any change — test or production — that mutates **process-global state** (the
working directory, environment variables, or which git worktree/branch the
process operates on) must satisfy all three of the following before the owning
slice can reach `verified`:

1. **Guaranteed restore.** Mutated state must be restored before the owning
   unit of work returns — via a test-framework scoped helper, a deferred
   restore, or a cleanup callback that runs irrespective of outcome. Prefer
   *scoped* mutation (invoking the tool with an explicit working-directory
   argument, or a child process) over mutating the ambient process and
   restoring it.

2. **Fail-closed target assertion.** Any operation that acts on a path or
   worktree — especially a `git` operation carrying a directory argument —
   must first assert the target exists and is the expected directory. If the
   path is empty, missing, or unexpected, the operation must not proceed.

3. **Reachability artefact.** The slice cannot be marked `verified` without
   evidence the guard exists and fires: a test exercising the restore path, or
   an explicit smoke step demonstrating the assertion firing on a bad target.

## Why

In a parallel or multi-worktree harness, process-global state is shared across
units of work: a mutation left unrestored is silently inherited by the next
test, or the next operation in the same process. The worst case is a git
operation that runs in an unexpected (or empty) directory and corrupts branch
state — a worktree silently flipped to its base branch — surfacing later as an
unrelated-looking failure. Wherever sessions run concurrently against a shared
base, this is a systematic failure class, not an incidental one.

## Resumed-loop restore contract

The same fail-closed principle extends to **resumption after an unclean exit**. A crashed or interrupted run can leave a track worktree holding uncommitted implementer output — debris that is process-global state by another name: it is silently inherited by whatever acts in that worktree next.

**A resumed loop must restore each track worktree to its committed slice state before it re-dispatches into that worktree.** Concretely: `git reset --hard` to the slice's committed head and `git clean` untracked debris, having first asserted the target is the expected worktree on the expected branch (clause 2 above — a `reset --hard` in the wrong directory is exactly the high-blast-radius case). Only then may the resumed unit of work re-dispatch.

Restoring to committed state is not the same as re-bootstrapping. A correct resume **preserves** committed progress and the board (the release plan is intact, verified slices stay verified); it discards only the *uncommitted* leftovers of the interrupted attempt. "Recovers without corrupting" is the floor; "recovers **cleanly**" — no crash debris surviving into the retry — is the contract, because leftover code contaminates the new attempt's diff and every diff-scanning gate that reads it (the 2026-07-12 dogfood: leftover implementer output tripped a boundary-mock detector on code the retry never wrote).

Any Baton engine that runs concurrent worktrees and supports resume inherits this hazard; the restore is therefore part of the protocol, not an engine detail.

## How to apply

- **Implementers:** prefer scoped mutation (pass an explicit working directory
  to the tool, or use a framework directory/env helper that auto-restores) over
  mutating the ambient process; when you must mutate, pair it with a deferred
  restore. Assert any path/worktree target before acting on it. Cite the
  guarding test or smoke step in `proof.md`.
- **Captains (design review):** scan any design that touches the working
  directory, environment, or worktree selection. Flag any occurrence lacking
  (1) restore, (2) a fail-closed target assertion, and (3) a reachability
  artefact.
- **Verifiers:** the reachability gate must specifically demonstrate the guard
  when the slice's diff touches process-global state.

## When this rule applies

- Any slice that changes the process working directory, environment variables,
  or which worktree/branch a tool operates on.
- Any slice that creates, switches, or removes git worktrees.
- Any test that mutates the working directory, environment, or process
  arguments without a framework-scoped, auto-restoring helper.

## When this rule does NOT apply

- Tests that mutate only framework-scoped state with automatic restore — the
  framework itself is the guard.
- Single-worktree, single-session workflows with no shared process state: the
  failure class does not arise, though the discipline remains good practice.

## Provenance

Codified after a recurring failure class in multi-worktree release harnesses: a
git operation run against a stale or empty directory silently flipped a
worktree to its base branch, and the pattern recurred across slices until the
guard was made a standing design-review check. It composes Rule 9 (design
review flags the unsafe design) with Rules 1/6 (reachability/proof that the
guard fires), specialised onto one high-blast-radius pattern.

---
title: Rule 12 — Guard Fidelity
description: A check must be mutation-proved against the form the defect actually takes, and its scope must equal the scope of the claim it backs. A check narrower than its claim is a decoration.
---

# Rule 12 — Guard Fidelity

## The rule

A **guard** is any automated check whose purpose is to prevent a class of defect from recurring: a regression test, a lint rule, a CI gate, an invariant assertion. Guards are the only durable output of a quality effort. Everything else is a convention, and conventions lose.

Before a guard may be cited as evidence in a proof bundle (Rule 6) or relied on by a verifier (Rule 7), it must satisfy **four** conditions:

1. **Mutation proof.** The guard must be demonstrated to FAIL. Break the thing it protects, observe red, restore, observe green. Record the mutation and both outcomes in the proof bundle. **A guard that has never failed is not a guard; it is a decoration that returns green.**

2. **Scope parity.** The domain the guard *checks* must equal the domain the claim *quantifies over*. If the claim is "no component does X", the guard must search every component — not every component in one directory. **A check whose scope is narrower than the claim it backs is a decoration**, and it is worse than no check, because it converts an unknown into a false assurance.

3. **Mutate the form the defect ACTUALLY takes.** This is the condition that is nearly always violated. Authors mutate the form they *imagined* — and real defects arrive in forms they did not. A guard that catches only the shape you thought of will pass its own mutation test and still miss every real instance. Derive the mutation from **how the defect has actually occurred in this codebase**, not from how you would write it.

4. **Right instrument.** If detecting the defect requires resolving scope, bindings, or structure, use a **parser**, not a pattern match. A regex over a structured language is a guess that looks like a check.

### The corollary: quantifier discipline

**A universally-quantified claim is a promise about a search you have not run.**

"No X exists." "Every Y is Z." "This is machine-checked." "It never happened."

Each of those is a claim over a domain. State it only if a check covers that whole domain. Otherwise **bound the claim to the search you actually ran** ("no X in `packages/ui`") and say so. An unbounded claim backed by a bounded check is the single most common way a green suite ships a live defect.

## Why

Rules 6 and 7 assume the *evidence* is sound and adversarially verify the *delivery*. Rule 12 closes the gap underneath them: **the evidence itself can be structurally incapable of detecting the defect it claims to prevent**, and neither a proof bundle nor a fresh-context verifier will notice, because both see the same green.

A guard fails in one of exactly two ways, and the second is the dangerous one:

- **It fails loudly** — a broken guard that goes red on correct code. Annoying, self-correcting.
- **It fails silently** — a guard that returns green over a domain it never searched. This *adds confidence while removing safety*, and it is indistinguishable from success at every layer above it: the implementer's proof cites it, the verifier runs it, CI enforces it, and the defect ships.

The economics are stark. Writing the guard costs an hour. Writing the guard *wrong* costs every verification round that follows, plus the false confidence banked in every artefact that cites it.

## Priority-order note

Rule 12 is numbered last but sits **logically upstream of Rules 6 and 7**: it governs whether the evidence those rules rest on means anything. The number is an append, not a ranking — Baton's priority order breaks *conflicts* ("higher rules win"), and Rule 12 rarely conflicts with 1–11; it strengthens their foundation. It is numbered 12 rather than renumbered into the low positions because renumbering eleven established rules would break every reference in every adopter's pasted fragment, every vendored engine copy, and every provenance citation — a cost far larger than the small conceptual awkwardness of a foundational rule wearing a high number.

## Relationship to existing rules

| Rule | What it does | How Rule 12 relates |
|---|---|---|
| Rule 6 — Proof Bundle | Requires evidence generated from live repo state | Rule 12 requires that evidence be *sound* — a guard cited in a proof bundle must be mutation-proved against the real defect form and scope-matched to its claim |
| Rule 7 — Adversarial Verification | Fresh-context verifier grades delivery against the spec | The verifier sees the same green a silent guard shows; Rule 12 is what stops a structurally-blind guard from passing verification. Enforced in the verifier role prompt: before accepting a guard as evidence, mutate the form the defect actually took and confirm the guard fails |
| Rule 8 — Requirements Fidelity | The criterion must be bounded and enumerable | Same root cause one layer up: an unbounded AC is a claim wider than any check can discharge, just as a narrow guard is a check narrower than its claim |
| Rule 2 — No Silent Deferrals | Surfaces deferrals explicitly | A guard whose scope is narrower than its claim is an *undeclared* deferral of the uncovered domain — Rule 12 makes it a named condition rather than a silent gap |

## When this rule applies

- Any guard cited as evidence in a proof bundle or relied on by a verifier — regression test, lint rule, CI gate, invariant assertion, or a documentation/prose check.
- Any claim in a spec, proof, or verdict that quantifies over a domain ("no X", "every Y", "machine-checked", "never happens").

## When this rule does NOT apply

- Exploratory or scratch checks not cited as evidence — a guard becomes subject to Rule 12 the moment a proof bundle or verifier leans on it.
- A claim already bounded to the exact domain its check covers ("no undeclared colour in `packages/ui`") — that is Rule 12 satisfied, not exempted.

## Provenance

Derived from a design-system release in the source monorepo (2026-07-11/12), where a single guard — enforcing that UI components own their own styling — failed fresh-context verification **four consecutive times**, each time in a new disguise of one error: **the check's scope was narrower than the claim it backed.**

It was defeated, in turn, by:

1. **No word boundary inside an identifier.** `/\bfieldClassName\b/` does not match `termFieldClassName`. A clone shipped, and the guard's own name claimed it caught "every incarnation".
2. **A tag scanner that stops at the first `>`.** In JSX that is routinely the arrow in `onChange={(e) => ...}` — long before `className` is reached. (The codebase already contained a brace-aware scanner whose doc comment *warned about this exact bug*. It was walked into anyway, ten lines below the warning.)
3. **Literal-only class reading.** `className={someConst}` was invisible.
4. **Template literals.** `` className={`${a} ${b}`} `` — the extractor's `[^}]*` truncated at the first interpolation.
5. **`cn()` / `clsx()` composition.**
6. **Double-quoted bindings** (the resolver handled only single quotes).
7. **A basename-anchored exemption** (`/Input\.tsx$/` exempts *any* `Input.tsx`, anywhere).
8. **An incomplete file list** — two whole applications were outside the glob.
9. **A missing element type** (`<textarea>` was never in the list, so a textarea had no owner and went back to improvising).
10. **Fill-only surfaces** — the guard required a border *and* a radius, and the style the slice itself introduced was fill-first.

**Every one of those guards passed its author's own mutation tests.** Each author dutifully broke the thing, watched it go red, restored it, and recorded the proof — because each author mutated the form they *imagined*, and every real clone used a form they did *not*. That is condition (3), and it is why conditions (1) and (2) alone are insufficient.

A sibling slice in the same release failed verification **seven** times on the same root cause in prose rather than code: a documentation guard that asserted the **absence of a known-bad string** (`not.toMatch(/tremor/i)`) rather than the **presence of the truth**, and so sat green while the document stated a falsehood. Same disease: the check's scope (one string) was narrower than the claim's scope (the document is true).

Two live WCAG failures in that codebase — a primary button at 3.29:1 against a 4.5:1 floor, and a mobile touch target at ~20px against a 24px minimum — had shipped and persisted for the same structural reason: **there was no guard for them to violate.** Neither was ever *chosen*. Both were drifted into, at call sites, because the system had authority nowhere and enforcement nowhere.

The releasing engineer's summary, which is the rule in one line:

> *A guard that has to be clever is a guard that will be outsmarted. Ask "is this a field at all?" before you ask "is this styled like a field?" — the first needs a substring search, the second needs a compiler.*
