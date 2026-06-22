---
title: Slice proof bundle template
description: Rule 6 proof bundle, scoped to one slice. Generated from live repo state, not recollection. Verifier reads this; do not paraphrase.
---

# Proof Bundle: `<slice-id>`

> Copy this file to `docs/release/<release-name>/<slice-id>/proof.md`. Every section must be populated from a live command run, not reconstructed from memory. Replace placeholder commands as appropriate for your stack.

## Scope

`<One sentence. Should mirror the spec's "User outcome" exactly — if it doesn't, fix the spec or fix the implementation; don't paper over the gap here.>`

## Files changed

<Paste raw output of `git diff --name-only <base-branch>`. Do not edit.>

```
$ git diff --name-only main
<paste output here>
```

## Test results

> Each project supplies its own test commands. Replace the commands below with your project's actual invocations. If a stack is not touched by this slice, write the section as `N/A — no <stack> changes`.

### `<Stack 1, e.g. Go>`

```
$ <your backend test command>
<paste full output including exit code>
```

### `<Stack 2, e.g. TypeScript>`

```
$ <your frontend test command>
<paste full output including exit code>
```

## Reachability artefact

`<Path to screenshot / Playwright trace / explicit smoke-step description naming the user gesture. Must exist on disk and be discoverable from this path. "Tests pass" is not a reachability artefact — see Rule 1.>`

- **Type**: `<screenshot | playwright-trace | manual-smoke-step>`
- **Path**: `<relative path from repo root>`
  - When Type is `screenshot`, the canonical path is `<docs-tree>/release/<release-name>/screenshots/<slice-id>-<descriptor>.png`, captured by `tests/e2e/release/<release-name>/<track-id>.spec.ts` via the shared helpers in `tests/e2e/release/_helpers.ts`. Full pattern — including the disambiguation from planner-context screenshots, helper signatures, and the bit-stable capture recipe — lives in [`role-prompts/implementer.md`](../role-prompts/implementer.md) → "Reachability screenshot convention".
  - For `playwright-trace` and `manual-smoke-step`, Path is free-form.
- **User gesture**: `<"User clicks X, observes Y" — exact words>`

## Delivered

`<Bulleted list. Every item from the spec's acceptance checks that is now demonstrably true, each with an evidence reference the verifier can independently confirm.>`

- `<Acceptance check #1>` — evidence: `<file path / test name / artefact path>`
- `<Acceptance check #2>` — evidence: `<file path / test name / artefact path>`

## Not delivered

`<Bulleted list. Every item from the spec's acceptance checks that is NOT demonstrably true. Each must be a Rule 2 deferral: why + tracking + acknowledgement. Empty list is acceptable only if every acceptance check is delivered. Do not omit the section.>`

- `<Item>` — **Why**: `<reason>`. **Tracking**: `<issue link / punch-list entry>`. **Acknowledged**: `<who, when>`.

## Divergence from plan

`<Any implementation that differs from the spec's planned touchpoints or approach. Empty is valid but the section must be present and explicit.>`

- `<Divergence description, or "None">`

## First-pass script output

<Paste the output of `scripts/release-verify.sh <slice-id>`. Must show all deterministic checks green before requesting verifier review.>

```
$ scripts/release-verify.sh <slice-id>
<paste output here>
```
