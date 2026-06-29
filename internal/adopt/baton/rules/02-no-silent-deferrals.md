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
