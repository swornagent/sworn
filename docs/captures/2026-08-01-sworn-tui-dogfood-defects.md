# Sworn TUI dogfood defects and fix plan

Recorded 2026-08-01 from the first real use of the project cockpit (commit
c60334bd) against the `fired` project. Every finding below was reproduced
against `/home/brad/projects/fired`, not inferred.

## What was observed

Three Baton releases appear in the catalog. Only the most recent one renders a
board. There is no visible way to start a run. There is no way to watch what
workers are doing.

## Evidence

`baton.ReadState` was run directly against each release ref of `fired`:

| Release | Result |
| --- | --- |
| `2026-07-25-systematic-flair-ui-hydration-safe` | `INVALID_PLAN_FENCE: plan must begin at byte zero` |
| `2026-07-30-customer-operations-inbox` | `PLAN_NOT_FOUND: release has no plan` |
| `2026-07-31-ownership-outside-household` | 10 nodes, 4 diagnostics (`TARGET_MOVED`, 3x `TRACK_REF_ABSENT`) |

The two failures have separate real causes:

- `2026-07-25` has `.baton/releases/<name>/plan.md`, but its fence is
  ` ```baton-plan-v1 `. `internal/baton/plan.go:17` admits only
  ` ```baton-plan-v2 `. It is a legacy v1 plan, not a corrupted one, and the
  error text names the wrong cause.
- `2026-07-30` has no plan file on its release-wt ref at all. The ref exists, so
  the release lists, but there is nothing behind it to project.

`fired/.sworn/` contains only `project.json`. There is no `runs/` directory and
no `sworn.db`.

## Defects

### D1. Baton's diagnosis is discarded

`cmd/sworn/tui_backend.go:141` collapses every Baton read error into the single
string `project board is unavailable`. `internal/tui/model.go:415-421` turns any
board error into `Stale = true` plus "Live updates paused. Showing the last
confirmed board."

A coded, actionable projection failure is therefore presented as a transport
problem, on a release that was never readable. The board already carries a
diagnostics channel and `diagnosticExplanation()` at `internal/tui/view.go:407`
renders codes in plain language. That path is unused for this class of failure.

`Stale` also doubles as the control lockout in `controlsAllowed()`
(`internal/tui/model.go:323`), so "the read failed" and "the data is old" cannot
be distinguished by the user or by the code.

### D2. An empty action list is never explained

The `start` action is offered only when a run manifest exists
(`cmd/sworn/tui_backend.go:155`). Manifests come from `.sworn/runs/*.json`, must
parse as a canonical `sworn.runtime-manifest/v3`, and must carry `repository`
equal to the project root and `release` equal to the release name.

With no manifest the action list is empty, `a` opens nothing, and the TUI gives
no reason. The TUI cannot author a manifest either; `docs/run.md:8` states that
Sworn does not create a plan, manifest, or driver config. The user is left with
no path forward and no message saying so.

### D3. Start drives the whole run inside the TUI foreground

`Execute("start")` calls `service.Start`, which calls `driveOwned`
(`internal/runtime/scheduler.go:5854`) and drives the run to a safe stop
synchronously. For that entire period `m.executing` is true, so the refresh tick
is suppressed and every key except `q` and `ctrl+c` is ignored.

The result is a static screen reading "Start run…" for the length of the run.
Quitting kills the driving process and leaves an owner lease that requires
`takeover`. This is the worst possible pairing with the absence of any live
output.

### D4. There is no live worker output, at any layer

- The TUI backend contract is `Catalog` / `Board` / `Execute`
  (`internal/tui/contracts.go:12`). There is no stream or event method.
- `Projector.Events` and `EventPage` exist but are consumed only by the browser
  board (`internal/cockpit/web/app.js:302`). The TUI header prints
  `LIVE · <offset>` from `ThroughOffset` while offering no way to read that
  offset.
- The feed is not a log regardless. `Evidence` is `{offset, kind, created_at}`
  (`internal/cockpit/model.go:125`). Journal event bodies are small structured
  tags such as the responsibility or effect kind.
- Raw worker output and model transcripts are captured nowhere. The evaluation
  surface documents the exclusion explicitly: no command, event, result,
  receipt, diagnostic, or model-output body
  (`internal/journal/evaluation.go:10`).

Per-track live output is therefore a capture and retention decision before it is
a UI feature. The content-free journal is a deliberate property, not an
oversight: it is what keeps the run record auditable and secret-free, which is
the same property attestation depends on.

## Fix plan

Ordered. Each item is independently shippable.

### F1. Surface projection failures as diagnostics, not as staleness

Carry the Baton error code through to the board instead of erroring the board
out, and split "read failed" from "data is stale".

- `Board` gains an explicit unreadable state, or the projection failure is
  returned as a `Diagnostic` on an otherwise empty board.
- `PLAN_NOT_FOUND`, `INVALID_PLAN_FENCE`, `REF_NOT_FOUND` and
  `INVALID_HEAD_OBJECT` gain entries in `diagnosticExplanation()`.
- `controlsAllowed()` stops keying the control lockout off the same flag used
  for staleness.
- Acceptance: opening `2026-07-30-customer-operations-inbox` in `fired` reads
  "this release has no plan", and `2026-07-25-systematic-flair-ui-hydration-safe`
  reads that its plan is an older format Sworn does not admit.

### F2. Correct the plan fence error text

`INVALID_PLAN_FENCE: plan must begin at byte zero` is raised for a v1 plan whose
fence does start at byte zero. Distinguish a wrong-version fence from a
misplaced fence so the message names the real cause.

- Acceptance: a `baton-plan-v1` document reports a version mismatch, not a byte
  offset.

### F3. Explain why no controls are available

When a release has no manifest, say so on the board, name the directory Sworn
searched, and state that the manifest must be provided. Do not leave `a` silent.

- Acceptance: on a manifest-less release the board states that no run definition
  was found in `.sworn/runs/`, and pressing `a` shows the same sentence rather
  than nothing.

### F4. Stop driving a run in the TUI foreground

Either hand off to a detached driver and have the TUI follow the journal, or
refuse to start from the TUI and print the exact `sworn run` invocation.

- Acceptance: the board keeps refreshing while a run started from the TUI is
  driving, or the TUI never starts one and says what to run instead.
- Note: `sworn loop --parallel` is not dependable on a cold start, so a frozen
  cockpit holding a live run is a compounding failure, not a cosmetic one.

### F5. Per-track activity feed from existing data

Build the feed the current journal can honestly support before deciding
anything about transcripts.

- Extend the TUI backend with a paged event read over `Projector.Events`.
- Filter by the selected graph node's track, since the board already tracks node
  selection.
- Render kind, timestamp, and the node's stage and outcome, with attempt tokens
  and cost where present.
- Acceptance: selecting a track shows its events advancing during a live run,
  and the header's `LIVE · <offset>` corresponds to something the user can read.

### F6. Transcript capture, as a separate decision

Raw worker output requires a side channel keyed by effect ID, with redaction and
size caps, kept out of the journal so the content-free guarantee survives. This
is a policy change and should be specced on its own, not folded into F5.

## What was not changed

Nothing in `fired`. The two unreadable releases there are genuine data states
and are useful fixtures for F1 and F2.
