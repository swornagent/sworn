
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
2. **Tracking** — a linked GitHub issue, plan task, or punch-list item that the decision lives in. Not just the inline comment.
3. **Acknowledgement** — the user, product owner, or decision-maker has been told, in plain text, that this item is being deferred.

Without all three, the inline comment is dark code's data-model cousin: it looks tracked, isn't.

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

- Ask which issue the work belongs to. If none exists, create one before starting.
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
3. The slice artefacts (`spec.md`, `proof.md`, `status.json`, the repo diff).
4. A PASS / FAIL / BLOCKED return contract.

That is the entire requirement. Anything beyond it is optimisation.

## The verifier contract

The verifier session must:

- **Load only**: `spec.md`, `proof.md`, `status.json` for the target slice, and live repo state via `git diff` / test commands. It must not load the implementer's session transcript, conversational memory, or any "wrap-up" prose.
- **Return exactly one of**:
  - `PASS` — every required gate is satisfied; the slice can transition to `verified`.
  - `FAIL: <numbered list of concrete violations>` — each violation tied to a specific spec acceptance check or proof-bundle gate.
  - `BLOCKED: <reason>` — verification cannot be completed because an external dependency is missing (test command undefined, artefact path unreadable, etc.).
- **Not propose redesigns.** The verifier's job is to falsify, not to help finish.
- **Not edit implementation code.** The verifier may add or repair *verification artefacts* (tests, smoke scripts, missing assertions) that expose a failure, but never the production code under review.
- **Fail closed.** Absence of evidence is a `FAIL`, not an optimistic `PASS`.

The verifier role prompt is provided in `role-prompts/verifier.md`. Paste it verbatim into the fresh session.

## When this rule applies

- Any slice with a `spec.md` that has been moved to `implemented` state.
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

1. Implementer session works one vertical slice. Writes `spec.md`, `journal.md`, `proof.md`, updates `status.json` to `implemented`.
2. Run `scripts/release-verify.sh <slice-id>` from a terminal. The script does deterministic first-pass: confirms `proof.md` exists, confirms `git diff` is non-empty, greps for dark-code markers, runs the test commands listed in `proof.md`. If the script fails, the slice never reaches the verifier — fix and re-run.
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

**The spec is not an axiom.** Before a slice enters implementation, its
requirements must be:

1. **Verified** — each acceptance criterion is singular, unambiguous,
   complete, consistent, feasible, and verifiable (ISO/IEC/IEEE 29148:2018
   quality characteristics). A fresh-context gate checks this.
2. **Validated** — the requirement makes sense and serves the need. A
   human-owned scenario sense-check (positive AND negative) confirms the
   spec answers the right question, not just a well-formed one.
3. **Traced** — every need in the intake links to at least one acceptance
   criterion, every acceptance criterion links to a need and at least one
   test, and every slice links up the vertical golden thread (org objective
   -> release benefit -> slice, or the lightweight floor: slice -> release
   goal). The 2-D requirements traceability matrix (RTM) enforces this
   fail-closed.

A need that drops silently between intake and spec is a requirements-fidelity
defect. The RTM makes it visible and blocks the release.

## Why

Baton Rules 1/6/7 verify **delivery against the spec** rigorously. They treat
the spec itself as an axiom — the spec is the contract, and the verifier
checks the code against it. But the spec can be wrong, incomplete, or
disconnected from what the user actually asked for. The front half of the
fidelity chain — from intake need to spec acceptance criterion — is
unverified by the delivery rules. A perfectly implemented, perfectly verified
slice that answers the wrong question is a fidelity defect that no amount of
delivery rigour will catch.

The gap is structural: the delivery rules are **downstream** of the spec.
They cannot see upstream. Rule 8 closes the front half.

## The 2-D requirements traceability matrix (RTM)

The RTM is the enforcement mechanism. It has two axes:

### Horizontal: need -> acceptance criterion -> test -> proof

```
intake.md          spec.md              spec.md             proof.md
--------           --------              --------            --------
N-01: need  --->   - [ ] AC cites N-01   Required tests  ->  test results
                   - [ ] AC cites N-01                      reachability
```

- **Needs** are enumerated with stable ids (`N-01`, `N-02`, ...) in
  `intake.md`. The planner assigns ids at planning time; they are never
  reused.
- **Acceptance criteria** in each `spec.md` cite the need id(s) they satisfy.
  The citation is inline in the AC text (e.g. "WHEN ... THE SYSTEM SHALL ...
  (N-01)").
- **Required tests** in `spec.md` cite the acceptance check they exercise.
- **Proof** in `proof.md` closes `AC -> test -> proof` (already enforced by
  Rule 6).

The RTM adds the front half: `need -> AC`. An orphaned need (no linked AC) or
an orphaned AC (cites no need, or cites a need but has no test) is a broken
trace.

### Vertical: org objective -> release benefit -> slice

```
org objective  --->  release benefit  --->  slice
(optional)          (index.md)            (status.json)
```

- **Org objective** is opt-in. A solo founder or small team may have no
  declared objective — the vertical floor is `slice -> release goal`.
- **Release benefit** is the value the release delivers, recorded in
  `index.md`.
- **Slice link** is the slice's contribution to the release benefit, recorded
  in `status.json` (`release_benefit` field).

The vertical trace is the golden thread: it carries line-of-sight from
strategy (if declared) through release value to individual slices. For
solo/small teams, the floor is lightweight: `slice -> release goal` satisfies
the vertical trace without an org-objective link.

## Enforcement

`sworn lint trace <release>` builds the matrix from `intake.md` / `spec.md` /
`status.json` / `index.md` alone — no separate datastore. It fails closed
(exit non-zero) on:

- An orphaned need (need with no linked acceptance criterion).
- An orphaned acceptance criterion (cites no need id, or cites a need but has
  no linked test).
- A slice with no vertical link (no release goal in intake and no release
  benefit or org objective on the slice).

A fully-traced release prints the matrix and exits 0.

## Lightweight by default

The RTM and the front-end gates must not over-proceduralise solo / small-team
work. The design choices that keep it lightweight:

- **Stable but simple need ids** — `N-01`, not a database. The planner
  assigns them; they survive edits.
- **Inline citation** — need ids are cited inline in AC text, not in a
  separate mapping file. The RTM parses them from the spec.
- **Vertical floor** — `slice -> release goal` is enough when no org
  objective is declared. Enterprise depth is opt-in.
- **No separate datastore** — the RTM threads through existing artefacts.

## Relationship to existing rules

| Rule | What it does | How Rule 8 complements it |
|---|---|---|
| Rule 1 — Reachability Gate | Tests exercise the integration point | Rule 8 ensures the integration point is the *right* one — traced to a need |
| Rule 6 — Proof Bundle | Closes AC -> test -> proof | Rule 8 adds the front half: need -> AC. Together they form the full horizontal chain |
| Rule 7 — Adversarial Verification | Fresh-context verification of delivery | Rule 8 verifies the spec itself, before delivery verification runs |

## When this rule applies

- Any release with an `intake.md` that declares needs. The RTM is the
  enforcement; the planner constructs the trace as a by-product of planning.
- The `planned -> in_progress` transition (Definition of Ready, Rule 8 +
  S06) gates on the RTM passing.

## When this rule does NOT apply

- Spikes or exploratory work without a release intake.
- A release with no declared needs (the RTM reports an empty matrix and
  exits 0 — no needs means no traces to break).

## Spec-quality metric — pre-code soundness + completeness

Before a spec reaches verification or validation, `sworn specquality <release>`
provides a deterministic, pre-code first-pass: soundness + completeness metrics
computed from a slice's **acceptance examples** alone, with no source code and
no model call.

### Acceptance examples

Every spec SHOULD include a `## Acceptance examples` section with one or more
**input → expected-output** pairs per acceptance check. These are the data the
metric operates on. Two formats are accepted:

**Structured** (preferred for complex specs):
```
## Acceptance examples

- name: "whole-ears-pass"
  input: "a release where every AC matches an EARS pattern"
  expected: "sworn lint ac exits 0 and prints per-pattern distribution"

- name: "free-form-fail"
  input: "a release with at least one free-form AC"
  expected: "sworn lint ac exits 1 naming the slice and line"
```

**Shorthand** (for simple cases where input/expected is clear):
```
## Acceptance examples

- valid ears release → exits 0
- free-form ac present → exits 1
```

### Soundness

For each example, `specquality` checks that the expected output is consistent
with the acceptance criteria — the criteria must not reject a valid output.
This is a limited deterministic check: it flags contradictions like "expects
failure where criteria describe only a pass case" or "references commands not
mentioned in the criteria."

### Completeness (mutation analysis)

For each example, `specquality` applies deterministic mutation operators to the
expected output (flip exit codes, negate assertions, remove keywords, change
case) and checks what fraction the criteria would reject. The completeness
score is `caught / total`. Below the configurable threshold (default 50%),
the gate fails closed.

### Enforcement

`sworn specquality <release> [--threshold <0.0-1.0>]` iterates every slice in
a release, computes soundness + completeness from each slice's acceptance
examples, and fails closed (exit non-zero) on:

- **Missing examples** — a slice with no `## Acceptance examples` section
- **Unsound example** — expected output contradicts the criteria
- **Low completeness** — below the threshold (default 0.5 / 50%)

A fully passing release prints per-slice scores and exits 0.

### Relationship to verify and validate

| Gate | What it checks | When | Owner | Tool |
|---|---|---|---|---|
| **Spec-quality** (S03) | Soundness + completeness of acceptance examples | Pre-code, pre-verification | Deterministic | `sworn specquality` |
| **Verify** (S04) | Quality characteristics per 29148 | After spec-quality passes | Model (fresh context) | `sworn reqverify` |
| **Validate** (S05) | Scenarios + benefit (sense-check) | After verify passes | Human | `sworn reqvalidate` |

The three gates form a pipeline: spec-quality is the cheapest (deterministic,
no model, no human), so it runs first. A spec with no acceptance examples or
low completeness never reaches model-based verification.

## Provenance
Rule 8 was introduced in the `2026-06-16-fidelity-layer` release. It closes
the "front half" fidelity gap identified during the v0.5.0 release cycle: the
delivery rules (1/6/7) verify code against spec, but nothing verified the
spec against the need. The RTM is the keystone — it threads through existing
artefacts and enforces traceability fail-closed, so a need cannot drop
silently between intake and spec.
## EARS notation — structured acceptance criteria

The RTM enforces *traceability* (need -> AC -> test). EARS (Easy Approach to
Requirements Syntax) enforces *structure* — each acceptance criterion is a
single sentence with a fixed keyword shape, not free-form prose. Together
they form the front-end fidelity gate: traced AND well-formed.

`sworn lint ac <release>` classifies every acceptance check in every slice's
`spec.md` by EARS pattern and fails closed on any free-form check that matches
no pattern, naming the slice + the offending line. A release whose every AC is
well-formed EARS passes and prints the per-pattern distribution.

### The six EARS pattern classes

| Class | Pattern | Example |
|---|---|---|
| Ubiquitous | `THE SYSTEM SHALL <action>` | `THE SYSTEM SHALL display the dashboard.` |
| Event-driven | `WHEN <trigger> THE SYSTEM SHALL <action>` | `WHEN a user clicks save THE SYSTEM SHALL persist the form.` |
| State-driven | `WHILE <state> THE SYSTEM SHALL <action>` | `WHILE the system is in maintenance mode THE SYSTEM SHALL show a banner.` |
| Optional-feature | `WHERE <feature> THE SYSTEM SHALL <action>` | `WHERE a premium feature is enabled THE SYSTEM SHALL show the export button.` |
| Unwanted-behaviour | `IF <condition> THEN THE SYSTEM SHALL <action>` | `IF the database is unreachable THEN THE SYSTEM SHALL return a 503 error.` |
| Complex | Two or more preconditions combined | `WHEN a user clicks save WHILE the form is valid THE SYSTEM SHALL persist the form.` |

### The NOTE: escape

A line prefixed with `NOTE:` is a deliberate non-requirement note and is
excluded from validation. Use it for context that is not a testable
requirement (e.g. a design constraint, a cross-reference, a rationale note).
Without the escape, such lines would fail the gate as free-form.

### Why EARS, not Gherkin

Gherkin (Given-When-Then) was considered and rejected. EARS is lighter (one
sentence per requirement, no scenario tables), is the de-facto notation for
agent-authored requirements, and maps cleanly to the checkbox format already
used in `spec.md` acceptance checks. The decision is recorded; do not
re-litigate at implementation.

## Validation — human-owned sense-check

Validation answers "are we building the *right* requirements?" — does the spec
make sense and serve the need (as distinct from verification's "are the
requirements well-formed?"). This is the cheapest defect-catch point and is
**human-owned**: the model drafts scenarios + a benefit hypothesis; the human
ratifies. Spec validation has no oracle but the user, so this gate is never LLM
self-certified.

### Validation record

Every slice must carry a validation record in its `status.json` under the
`validation` field (see `state.ValidationRecord`):

| Field | Required | Description |
|---|---|---|
| `human_ratified` | Yes | Must be `true`. Model-only validation is not a pass. |
| `ratified_by` | Yes | Who ratified (human identifier). |
| `ratified_at` | Yes | When ratified (ISO 8601). |
| `positive_scenarios` | Yes (≥1) | Scenarios where the requirement works as intended. |
| `negative_scenarios` | Yes (≥1) | Edge + failure flows; what should *not* happen. |
| `benefit_hypothesis` | Yes | This slice's benefit and its vertical link (slice -> release benefit -> objective). |

### Enforcement

`sworn reqvalidate <release>` reads every slice's `status.json` and fails
closed on:

- **Missing record** — no `validation` field at all
- **Model-only** — `human_ratified` is false or absent
- **Missing positive scenarios** — empty `positive_scenarios` array
- **Missing negative scenarios** — empty `negative_scenarios` array
- **Missing benefit hypothesis** — empty or blank `benefit_hypothesis`

A fully-validated release exits 0 and prints the per-slice summary.

### Relationship to verification

| Gate | What it checks | Owner | Tool |
|---|---|---|---|
| **Verify** (S04) | Quality characteristics per 29148 (well-formedness) | Model (fresh context) | `sworn reqverify` |
| **Validate** (S05) | Scenarios + benefit (sense-check) | Human | `sworn reqvalidate` |

The two gates are complementary. A spec can be perfectly well-formed (pass
reqverify) but answer the wrong question (fail reqvalidate), and vice versa.
Both must pass before a slice enters implementation (Definition of Ready, S06).
## Definition of Ready

The Definition of Ready (DoR) is the gate that every slice must pass before it
can transition from `planned` to `in_progress`. It composes the three
requirements-fidelity checks into a single fail-closed verdict:

1. **Traced** — the RTM verifies the slice has complete traceability: every need
   links to an acceptance criterion, every acceptance criterion links to a need
   and a test, and the slice has a vertical golden-thread link (slice → release
   benefit → org objective or the lightweight floor: slice → release goal).
2. **Verified** — every acceptance criterion passes the 29148 quality-
   characteristic check (singular, unambiguous, complete, consistent, feasible,
   verifiable, necessary) via a fresh-context model pass.
3. **Validated** — the slice carries a human-ratified validation record with
   positive + negative scenarios and a benefit/alignment hypothesis.

If any gate fails, the transition is blocked and the failing gate(s) are named.
If any gate cannot be evaluated (e.g. the RTM cannot build due to a missing
artefact, or no verifier model is configured), the transition is also blocked —
fail closed. The slice remains `planned` until all three gates pass.

### Enforcement

The DoR is enforced programmatically by `internal/implement.CheckDoR()`, which
calls `rtm.Build()`, `reqverify.Run()`, and `reqvalidate.Run()` and filters
their results to the target slice. The implementer session calls CheckDoR before
any code is written; if it fails, the slice stays `planned` and the specific
violations are surfaced.

### Bypass

There is no bypass for the DoR. An explicit human re-plan (/replan-release) is
the only way to change a spec — never a silent Gate 0 skip. The
`state.TransitionGate` API enforces this by requiring the gate callback to
return nil before the transition proceeds.


# Rule 9 — Design Fidelity

**Meeting a requirement is not the same as the right solution for the whole.**
Solution fit is a quality the delivery verifier (Rule 7) cannot see. This rule
keeps design **human-owned**, AI-augmented, and calibrates how much human
judgement each choice demands by its stakes.

## Classification: stakes = reversibility x blast-radius

Every design choice has a **stakes class**:

| Class | Reversibility | Blast radius | Decision requirement |
|-------|--------------|--------------|---------------------|
| Type-1 | Hard to reverse | Wide / structural | Full human decision with options + rationale recorded |
| Type-2 | Easy to reverse | Narrow / local | AI may proceed with noted default |

**Architecturally-significant choices are always Type-1**, regardless of other
factors. A choice that shapes the whole system, the data model, the deployment
architecture, or an external contract is architecturally significant.

## Enforcement

`sworn designfit <release>` is a deterministic, fail-closed gate that reads each
slice's `design_decisions` from `status.json`. It checks:

1. Every Type-1 choice has a non-empty `human_decision` field — otherwise
   violates, naming the slice + choice.
2. Every `architecturally_significant` choice is classified Type-1 — otherwise
   violates, naming the slice + choice.

## Option surfacing

When the planner reaches a design choice during planning:

1. The planner drafts **at least two options** with trade-offs and prior art.
2. For Type-1 choices, the human selects one and records the decision +
   rationale in the slice's `status.json` `design_decisions` field.
3. For Type-2 choices, the planner records a noted default and proceeds.

The model may propose options, classify stakes, and surface trade-offs — but
for a Type-1 choice, the model **may not record the human decision itself**.

## Record format

Each design decision is recorded as an entry in `status.json`:

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

## Design-system input

Design fidelity requires a declared source of truth. Every UI-bearing project
must declare its design system in its sworn project config before design
conformance can be audited (S09).

The design system is a **three-tier concept**:

| Tier | Name | Role | Example |
|------|------|------|---------|
| Umbrella | **Design system** | The whole declared input — token source + component library | `design_system` in `config.json` |
| Atoms | **Design tokens** | The named-value source of truth (colours, spacing, typography) | `tokens.json` (W3C DTCG), CSS custom properties, JS theme object |
| Reusables | **Component library** | The coded, reusable UI components | `packages/ui/`, `src/components/` |

### Schema

A project's `config.json` carries an optional `design_system` block:

```json
{
  "ui_bearing": true,
  "design_system": {
    "token_source": "tokens.json",
    "component_library": "packages/ui"
  }
}
```

### Enforcement

- `ui_bearing: true` with no `design_system` block = fail closed (design
  conformance cannot proceed without a declared source of truth).
- `ui_bearing: false` or absent = design system not applicable (CLI projects
  and non-UI tools are exempt).
- The format hint for tokens is not mandated here — S09's audit adapts to
  the project's token format (DTCG JSON, CSS vars, JS themes).

### Discovery

`sworn init` prompts for the design system declaration when initialising a
UI-bearing project. The `--ui-bearing` flag marks the project explicitly.

## Design-system conformance audit

`sworn designaudit <project-dir>` runs a two-layer conformance audit:

### Layer 1 — Deterministic first-pass (machine-check)

Scans UI source files (`.css`, `.scss`, `.ts`, `.tsx`, `.js`, `.jsx`, `.vue`, `.svelte`)
for three categories of design drift:

| Category | Pattern | Example violation |
|----------|---------|------------------|
| **Hardcoded colour** | `color: #ff0000` | Hex literal in CSS property — use `var(--color-primary)` |
| **Off-scale spacing** | `margin: 17px` | Hard-coded `px`/`rem` value — use `var(--spacing-4)` |
| **Recreated component** | `function Button()` in app code | Component defined outside the library when a library `Button` exists |

Each violation is reported with `file:line: [kind] message`.

**Sanctioned exceptions:** append `/* sworn-design-allow */` to a line to suppress
its violation and record a deliberate, human-approved deviation.

### Layer 2 — Human cohesion verdict (human-owned)

The deterministic pass cannot assess whether the overall design **feels on-brand** —
typography consistency, visual rhythm, spacing coherence. This judgement is human-owned.

Supply it with `--cohesion=on-brand|off-brand`. The system will NOT auto-pass the
cohesion check; it requires a human-set value to reach exit 0.

### Exit codes

| Condition | Exit code |
|-----------|-----------|
| Machine violations found | 1 |
| Clean pass, no cohesion verdict | 1 (blocked until human sets verdict) |
| Clean pass + cohesion verdict recorded | 0 |
| Project is not `ui_bearing` | 0 (exempt) |
| Config error (no design system declared for UI-bearing project) | 2 |

### CI usage

`bin/design-audit.sh <project-dir>` wraps `sworn designaudit` for first-pass CI use.

## Out of scope (sibling rules)

- Design-system declaration (tokens + component library) — S08.
- Requirements validation (Rule 8) — design fit assumes the requirement is
  already validated.

---
title: Rule 10 — Customer Journey Validation
description: Critical customer journeys are a first-class platform artefact — AI-drafted, human-ratified, version-controlled, and fail-closed on absence or staleness. A journey is the unit of end-to-end evidence.
---

# Rule 10 — Customer Journey Validation

## The rule

**Critical customer journeys are a first-class platform artefact, not a per-release afterthought.** Before a release can ship, its customer journeys must be:

1. **Elicited** — the model drafts candidate critical customer journeys from the app; no draft means no journeys gate.
2. **Ratified** — a human reviews, edits, and ratifies the journeys; model-only journeys are unratified and fail the gate.
3. **Durable** — journeys are persisted to a version-controlled artefact (`.sworn/journeys.json`) that survives session boundaries and is maintained release over release.

A journey is an ordered, end-to-end path a user type takes across the app to achieve an outcome. It is the unit of end-to-end evidence — if a release changes a user-visible surface, the journey that crosses that surface must be updated.

## Why

Baton Rules 1/6/7 verify **delivery against the spec** within a single slice. The s

pec scopes one slice, one outcome. A critical customer journey crosses many slices — it is the full path a user takes. If release work changes a surface that a journey crosses, the journey (not just the slice) must be re-verified.

Journey validation exists at a different level of abstraction from slice verification:

| Artefact | Scope | Owned by | Gate |
|---|---|---|---|
| Slice spec | One slice, one user-reachable outcome | Planner + Verifier | Rule 7 (adversarial verification) |
| Journey | End-to-end user path across many slices | Human + Model | Rule 10 (elicitation + ratification) |

A slice that passes Rule 7 but leaves a journey stale is an integration defect that no per-slice gate catches. Journey validation closes that gap.

## The journey artefact

The journeys artefact lives at `<project-root>/.sworn/journeys.json`. It is a JSON document containing:

- **Version** — schema version for forward compatibility.
- **Journeys** — the list of critical customer journeys, each with:
  - **id** (e.g. `J01-onboard-new-user`)
  - **user_type** (e.g. `free_user`, `pro_user`, `admin`)
  - **outcome** — what the user achieves
  - **steps** — ordered sequence of actions
  - **entry_surface** — where the journey begins
- **Ratification metadata** — `is_ratified`, `ratified_by`, `ratified_at`

Provisional: the exact schema is refined by the live journey-validation hand-run (field-level detail appended via `/replan-release`; verified work stays immutable).

## Enforcement

`sworn journeys --check <project>` is a deterministic, fail-closed gate that reads `.sworn/journeys.json` and returns:

- **Exit 0** — artefact exists and is human-ratified. The journeys are listed.
- **Exit 1** — artefact is missing (elicitation has not been run) or exists but is unratified.
- **Exit 2** — unrecoverable error (parse failure, I/O error).

The gate is additive — it does not replace any existing gate. It runs alongside per-slice verification (Rule 7), not instead of it.

## Workflow

1. A maintainer runs `sworn journeys <project>`.
2. The model drafts candidate journeys from the project structure.
3. The human reviews, edits, and ratifies the artefact (sets `is_ratified=true`,
   `ratified_by`, `ratified_at`).
4. `sworn journeys --check <project>` passes.
5. The journeys artefact is committed to version control and maintained as the project evolves.

## When this rule applies

- Any release that changes a user-visible surface (UI, API, CLI command, form, route).
- Pre-release cutover checks — the journeys gate runs after all slices are verified but before the release merges.

## When this rule does NOT apply

- Infrastructure-only releases with no user-visible change.
- A release with no ratifiable journeys (the tooling produces a minimal set; the human may ratify that minimal set).

## Relationship to existing rules

| Rule | What it does | How Rule 10 complements it |
|---|---|---|
| Rule 1 — Reachability Gate | Tests exercise the integration point | Rule 10 ensures the integration point's journey is documented |
| Rule 6 — Proof Bundle | Closes AC -> test -> proof per slice | Rule 10 adds cross-slice journey evidence |
| Rule 7 — Adversarial Verification | Fresh-context verification of one slice | Rule 10 verifies the end-to-end paths that span slices |
| Rule 8 — Requirements Fidelity | Need -> AC -> test -> proof horizontal trace | Rule 10 adds the vertical journey trace across the release |


## No-mock boundary (S10 enforcement)

Journey validation exists to prove the *assembled system actually works end-to-end*. A journey walked over a mocked boundary proves nothing — the mock answers however the test author wired it to, not however the real system would. So the no-mock boundary is **constitutive of Rule 10, not a detachable add-on**: it is the enforcement that makes a walked journey count as proof. (It also reads as a Rule-2 concern — an undeclared mock is a silent deferral — but its home is here, because the failure it prevents is a journey that lies about working.)

**An undeclared mock at a validated boundary is an undeclared Rule-2 deferral and fails closed.** The validated boundaries are: database (DB), authentication (auth), and entitlement (premium/subscription tier). A mock/stub/fake at one of these boundaries is permitted only if declared as a Rule-2 deferral in the slice's `status.json` `open_deferrals` with all three elements (why + tracking + acknowledgement).

### Detection

The `sworn verify` first-pass gate (`internal/verify.CheckBoundaryMocks`) scans the slice's diff for lines that combine:
1. A mock/stub/fake marker (`mock`, `fake`, `stub`, etc.)
2. A validated-boundary keyword (`sql.DB`, `auth`, `premium`, etc.)

Lines matching both patterns are flagged. If the mock's boundary + type matches any open deferral (case-insensitive substring match on boundary name + mock/fake/stub keyword), it is treated as declared and surfaced as a known deferral. Otherwise it is an undeclared boundary mock — the gate exits non-zero (FAIL) and names the offending mock + boundary.

### Implementer guidance

An implementer that cannot reach real infrastructure at a validated boundary must **stop and surface the blocker** (record a blocked-on-environment state) rather than mock around it. The implementer role prompt (`internal/prompt/implementer.md`) instructs this under "Hard constraints" — the stop-don't-mock principle is a binding constraint, not advisory.

### Relationship to journey validation

A journey that crosses a validated boundary (every journey touches the DB; most touch auth) must run that crossing against real infra, or declare the mock as a deferral, for the verification gate to pass. The artefact (the journey) and the gate (no-mock) are not two rules that happen to compose — they are one rule's two faces: the journey says *what* end-to-end path must work; the no-mock gate guarantees the walk that proves it didn't cheat at the boundary. A journey whose boundary is mocked is a journey that hasn't been validated at all, regardless of a green test.

### When this applies

- Every slice whose diff introduces, uses, or constructs a mock/stub/fake at a validated boundary.
- Every implementer session that operates the S10 implementer prompt.

### When this does NOT apply

- Pure unit test mocks that do not touch a validated boundary (e.g. a mock calculator, a mock string formatter). These are internal to the unit and are not flagged.
- The human walkthrough (S13), where mocks are fully off and real journeys run against real infra — that slice is out of scope for S10.


## Impact analysis (S12)

**Per-release journey-impact analysis ties Rule 10 into the release workflow.** For a given release, `sworn journeys --impact <release>` computes which critical journeys the release touches, derived from the release's slice planned/actual files and the journeys' step surfaces. The output is the release's validation scope: the set of journeys that must be walked and re-tested before cutover.

### Algorithm

1. **Load the journeys artefact** from `.sworn/journeys.json` — fail-closed if missing or unratified.
2. **Collect slice touchpoints** — scan `docs/release/<release>/S*/status.json` for each slice's `planned_files` and `actual_files` (the files the release changes).
3. **Heuristic surface matching** — for each journey, its `entry_surface` and each step's `surface` are matched against the collected touchpoint files:
   - Level 1: direct substring match (normalised to lowercase).
   - Level 2: token-level match (alphanumeric tokens from both file path and surface).
   - Level 3: conventional mapping (surface "CLI" maps to files under `cmd/`).
4. **Output the touched set** — a journey is in-scope if any of its surfaces match any touchpoint file. The heuristic is biased toward over-inclusion (a journey is touched if any step's surface is touched), so the walkthrough scope errs safe.

### Fail-closed on missing artefact

If no ratified journeys artefact exists at `.sworn/journeys.json`, impact analysis cannot run and directs the user to run elicitation (S11) first. An unratified artefact also fails — ratification is required before impact analysis.

### Empty touched set

A release that touches no journeys (e.g. an internal-only refactor with touchpoints that no journey surface matches) reports an empty touched-set explicitly rather than failing. This allows infrastructure-only releases to pass the gate.

### CLI

```
sworn journeys --impact <release> [project-path]
```

Exit codes:
- **0** — success; the touched-journey set is reported (may be empty).
- **1** — journeys artefact missing or unratified.
- **2** — unrecoverable error (I/O or parse failure).

The impact result is consumed by S13 (walkthrough attestation) and S14 (journey regression suite) to determine the release's validation scope.


## Walkthrough attestation (S13)

**A release cannot ship unless every touched journey carries a human-walkthrough attestation.** The human who walks the journey is the acceptance authority: the model (software) may load/save attestation records but cannot author the human attestation's attested-by field.

### Attestation record

Each walkthrough attestation records:

- **journey_id** — the journey this attestation covers (e.g. `J01-verify-flow`)
- **walked_by** — the human who walked this journey (mandatory; model cannot set)
- **walked_at** — ISO 8601 timestamp when the walkthrough was performed
- **real_infra** — boolean assertion that the walkthrough used real infrastructure
- **mocks_off** — boolean assertion that no mocks/stubs were present at validated boundaries
- **passed** — whether the walkthrough passed or failed
- **notes** — free-form observations (optional)

### Storage

Attestations are stored per-release at `<project-root>/.sworn/attestations/<release-name>.json`. A human edits this file to record walkthroughs. The file is version-controlled alongside the journeys artefact.

### Ship gate

`sworn ship <release>` is a deterministic, fail-closed gate that checks:

1. The journeys artefact exists and is human-ratified.
2. The release's validation scope (touched journeys, computed by S12 impact analysis).
3. Every touched journey has an attestation with all required fields.

The gate exits:
- **0** — all touched journeys have complete, passing human attestations; the release may proceed to cutover.
- **1** — one or more journeys are un-walked, incomplete, or have failed attestations; each is named in the kill-list.
- **2** — unrecoverable error (I/O or parse failure).

### Fail-closed semantics

The ship gate is fail-closed:
- A journey with **no attestation** blocks cutover (un-walked).
- A journey whose attestation is **missing the human-walked-by field** blocks cutover (model cannot author attestations).
- A journey whose attestation **lacks real_infra or mocks_off** blocks cutover.
- A journey whose attestation records a **failed walkthrough** blocks cutover.
- Only a complete, passing attestation for every touched journey allows cutover.

### Relationship to S14

The walkthrough attestation is deliberately manual (S13). S14 (journey regression suite) is the maturity path that shrinks the manual set over time — as journeys are codified into automated regression tests, fewer require human walkthrough at cutover. The attestation record is designed so a journey can transition from manual-walkthrough-required to regression-covered without schema changes.



## Regression codification (S14)

**Every walked-pass journey should eventually have an automated regression test.** S14 introduces the `sworn journeys --regen <release>` command that codifies journey steps into structured test scaffolds.

### Codification algorithm

1. **Load** the ratified journeys artefact and attestations from `.sworn/`.
2. **Collect** journeys with a `walked-pass` attestation status.
3. **Filter** — journeys already marked `has_regression: true` or whose `regression_test_path` points to a file that exists on disk are skipped (accretive).
4. **Generate** a Go test scaffold for each un-covered walked journey:
   - File name: `journey_<sanitised-id>_test.go`
   - Package: `journey_test`
   - One test function per journey containing the journey's steps as structured comments and a `t.Skip` marker.
5. **Mark** the journey's `has_regression: true` and set `regression_test_path` in the artefact.
6. **Save** the updated artefact.

### Coverage check

`sworn journeys --regen <release>` performs a coverage check before codification. If any coverage gaps existed at run start (walked-pass journey without a committed regression test), the command exits non-zero (FAIL) even if those gaps are filled during the same run. This fail-closed signal forces the release to either:
- Commit the scaffolds and re-run `--regen` (no pre-codification gaps → exit 0), or
- Explicitly acknowledge the gap as a Rule 2 deferral.
### Accretive, not regenerated

Previously-codified journeys are never re-generated. The scaffold file existence + `has_regression` flag together define "covered." An existing file is not overwritten, and a journey with `has_regression: true` is not re-processed. This ensures the regression suite is additive across releases — last release's walked journeys are this release's automated coverage.

### Default output path

Scaffolds are written to `<project-root>/tests/e2e/journeys/` by default. This path is configurable via an `outputDir` parameter (planned enhancement).

### Relationship to S13 (walkthrough attestation)

The ship gate (S13) currently requires human walkthrough attestations for every touched journey. A journey with `has_regression: true` is still subject to the ship gate — regression coverage does NOT replace the walkthrough. Over future releases, the ship gate may relax requirements for regression-covered journeys (e.g., re-walk only changed surfaces), but that is out of scope for v1 of S14.


## Provenance

Rule 10 was introduced in the `2026-06-16-fidelity-layer` release. It closes the cross-slice integration-evidence gap: per-slice verification (Rules 1/6/7) catches within-slice defects, but no artefact captures the end-to-end user path. Journey validation fills that gap with a lightweight, version-controlled artefact that survives release boundaries.

