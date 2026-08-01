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

| Fix | Issue |
| --- | --- |
| F1 Surface projection failures as diagnostics | #169 |
| F2 Correct the plan fence error text | #170 |
| F3 Explain why no controls are available | #171 |
| F4 Stop driving a run in the TUI foreground | #172 |
| F5 Per-track activity feed from existing data | #173 |
| F6a Live model output stream | #176 |
| F6b Durable transcript retention | #174 |

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

### F6a. Live model output stream

Added 2026-08-02 at Brad's direction, with a stated operational reason: he has
repeatedly stopped churn by watching verbose model output and noticing work
going off the rails **before** any of the automated checks fired. Live model
output is therefore an early-warning control, not a debugging luxury, and it
sits ahead of durable retention.

The provider capability is not in question. Every path Sworn uses can stream:
OpenAI-compatible vendors stream deltas, and both native CLIs already do.
The entire gap is Sworn-side, and it is two different sizes.

**Native drivers (Claude, Codex): the stream already flows through Sworn and is
discarded.**

- `internal/driver/native_linux.go:2703` launches with
  `--output-format stream-json --verbose`.
- `scanNativeEvents` (`internal/driver/native_linux.go:2923`) reads stdout line
  by line off `command.StdoutPipe()` and hands each line to
  `nativeEventState.accept`.
- `accept` validates the envelope and extracts usage. For `ProfileClaude` it
  handles `system`/`init` and `result`; for `ProfileCodex` it handles
  `thread.started`, `item.started`, `item.completed` and `turn.completed`.
  Assistant text, thinking, and tool-use events fall through the switch and are
  dropped at `native_linux.go:2943-2944`.
- Redaction already exists at exactly the right point:
  `containsCapability(line, capability)` fails the whole invocation if the
  credential ever appears in output.

A live tap is therefore an observer hook at the point the line is currently
discarded. No new provider work, no new parsing, no journal change.

**HTTP drivers: streaming is switched off, deliberately and hardcoded.**

- `internal/driver/openai.go:265` and `internal/driver/responses.go:111` both
  send `Stream: false`.
- Enabling it means real work: request the stream, parse SSE deltas, accumulate
  incrementally, and keep the existing strict response admission intact so the
  final observation is byte-identical to what non-streaming produced.

**Transport is the hard part, and it is coupled to F4.**

Every cross-process observer in Sworn today reads the journal. The journal is
the IPC. A live model stream cannot use it without destroying the content-free
guarantee, so this needs a genuinely new channel:

- If the TUI drives the run in its own process, an in-memory fan-out suffices.
- If F4 detaches the driver, the stream needs a real IPC hop, such as a unix
  socket beside the journal, with the same 0600 and no-symlink discipline.

**Resolved 2026-08-02: the transport is a file, not a socket.** The driver
writes, any number of readers tail. This works identically whether the run is
driven in-process or detached, so **F6a is no longer blocked by F4**. The
earlier in-memory-versus-socket framing was a false choice.

Correcting the reasoning that produced it: "the journal must stay content-free"
is an architectural invariant with a concrete reason, namely that the journal is
the attestable record and the evaluation export surface is deliberately
content-free so telemetry can leave the machine without carrying customer code
or model prose. "Model output must not be persisted anywhere" is a strictly
stronger claim that was inherited from it without being checked. Only the first
is real.

#### Never in the repository, and never silent

Two requirements set by Brad on 2026-08-02. Both are hard.

**1. Transcripts must never be persisted inside the repository.** Not by
convention, by admission check.

`.gitignore` is not sufficient. It is a per-repository convention that
`git add -f` overrides, that a differently configured repository will not carry,
and that says nothing about a path outside `.sworn/`. Note that `fired` already
ignores `.sworn/*` with a single `!.sworn/project.json` exception, so the
default location would be ignored today. That is not the point. The guarantee
has to hold when the convention does not.

- The transcript directory is supplied explicitly as an absolute, clean path,
  matching the existing house style for `--journal` and `--config`.
- Sworn refuses the path if it resolves inside the repository work tree. The
  containment test already exists: `gitx.Open` plus `repository.Root()`.
- Containment must be tested **after** symlink resolution, and must cover linked
  worktrees, not only the primary root. `fired` has roughly ninety worktrees
  under a sibling directory; a check against `repository.Root()` alone would
  wave every one of them through.
- Failure is a coded refusal at admission, consistent with `INSECURE_PERMISSIONS`
  and the other fail-closed path checks.
- Files are `0600`, as the journal already enforces at
  `internal/journal/store.go:99`.

This is why the journal's own location is not a precedent. The journal lives at
`.sworn/sworn.db` inside the repository and that is fine, because the journal is
content-free. The transcript is not, so it does not get the same latitude.

**2. Capture must be explicit and visible.** No silent recording of model
output.

- Off by default. Enabling is a deliberate act, per run or per session.
- At the moment of enabling, Sworn states the exact directory being written to.
- While capture is active, the cockpit shows it. A user must never be able to
  look at the board and not know that model output is being written to disk.
- The journal records **that** capture was enabled, as a boolean fact. That is
  content-free, so the invariant holds, and it makes "was this run observed"
  an attestable property rather than an unrecorded side effect.

#### Remaining constraints

- Keyed by effect ID so it filters to one track, matching F5.
- Bounded, with a size cap and rotation. A parallel run with `--verbose`
  produces a large volume of output.
- Back-pressure must never stall a worker. A file writer that never waits on a
  reader gives this nearly for free.
- Reuses the existing per-line secret guard. A line that trips it fails the
  invocation exactly as it does today.
- Data at rest deserves deliberate handling. `fired/.sworn/project.json`
  declares its own stakes as `pii`, `financial` and `credentials` under the
  Privacy Act, and verbose model prose about that codebase inherits them.

Acceptance: with a run driving and capture explicitly enabled, the cockpit shows
verbose model output as it is produced, filterable to a single track, and states
where it is being written. Disconnecting or lagging the viewer has no effect on
the run. Pointing the transcript directory anywhere inside the repository or one
of its worktrees is refused at admission.

### F6b. Retention rule for captured transcripts

Now much smaller than originally scoped. Persistence, location and consent are
settled in F6a. What remains is how long transcripts are kept, what the rotation
and cap rules are, and whether a retained transcript is ever in scope for
attestation. Does not block F6a.

## What was not changed

Nothing in `fired`. The two unreadable releases there are genuine data states
and are useful fixtures for F1 and F2.
