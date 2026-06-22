---
title: Slice journal template
description: Implementation log for one slice. Append-only. Visible to verifier as context, but verifier verdict is based on proof.md and repo state, not journal prose.
---

# Journal: `<slice-id>`

> Copy this file to `docs/release/<release-name>/<slice-id>/journal.md`. Append entries chronologically. Do not delete history. Decisions captured here must also land in commit message bodies per Rule 4 — this journal is a working surface, not a substitute for durable capture.

## Session log

### `<YYYY-MM-DD HH:MM>` — `<session start / state transition>`

- **State**: `<planned | in_progress | implemented | failed_verification | verified | deferred | shipped>`
- **Notes**:
  - `<Decisions made>`
  - `<Trade-offs encountered>`
  - `<Subagent dispatches and where their outputs landed>`

### `<YYYY-MM-DD HH:MM>` — `<next event>`

- ...

## Open questions

\<Anything the implementer needs the human to resolve. Each open question blocks state transition to `implemented` until answered.\>

- ...

## Deferrals surfaced

`<Per Rule 2: each deferral needs why + tracking + acknowledgement. Cross-link to GitHub issue or punch-list entry. If empty, write "None" explicitly.>`

- ...

## Verifier verdicts received

`<Append every verifier verdict here. Even FAIL verdicts stay — they are part of the slice's history.>`

### `<YYYY-MM-DD HH:MM>` — `<PASS | FAIL | BLOCKED>`

- **Verifier session**: `<fresh / inherited — should always be fresh>`
- **Verdict body**: `<paste the full verifier output>`
- **Action taken**: `<how the implementer responded>`
