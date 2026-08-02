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

## Review corrections, 2026-08-02

A review of PR #175 at 21aec2b raised six objections. Five were verified as
correct and are folded in below. Recording them here rather than only in review
comments, because the divergence between this document and the issue bodies was
itself the most serious finding.

**Single contract.** This document is the implementation contract. Issue bodies
summarise and link here; they do not restate requirements. The live-streaming
requirements changed three times on 2026-08-02 and each change landed here as a
commit but in the issues only as a comment, leaving issue bodies asserting
"ephemeral, no persistence, blocked by F4" while this document said the
opposite. A worker reads the body, not the comment thread. That is the exact
"N places that should be one, therefore drift, therefore silent divergence"
failure this project has hit repeatedly.

**Prerequisite: adopt Baton RC13, which opens S1.** `internal/baton/assets.go:27`
pins `1.0.0-rc.12`. Tag `v1.0.0-rc.13` exists and changes how target movement is
judged, in `reference/records/state.mjs`:

```diff
-  const targetStale = assembly.status !== 'complete'
-    && approval.receipt.target !== captured[1].head;
+  const targetStale = assembly.status !== 'complete'
+    && !isAncestor(repo, approval.receipt.target, captured[1].head);
```

Under RC12 any forward movement of the target branch raises `TARGET_MOVED`.
Under RC13 only movement that leaves the approved target off the ancestry does.
The `TARGET_MOVED` observed on `2026-07-31-ownership-outside-household` is
therefore most likely an RC12 false positive from an ordinary branch advance.

S1 is the work of explaining diagnostics in plain language. Doing that on top of
an engine that emits a spurious diagnostic ships a well-worded false alarm, so
RC13 adoption precedes it, which is why it opens S1.

**Fixtures must be synthetic, not live.** The `fired` acceptance criteria have
already gone stale. `release-wt/2026-07-30-customer-operations-inbox` moved from
`2c12ae90e` to `33512c66c` and now has a valid plan, so "reads: this release has
no plan" no longer reproduces. The 2026-08-01 observations stand as history and
are not to be edited. Acceptance criteria move to synthetic fixtures built in
the test suite.

## Delivery plan

Consolidated 2026-08-02 from seven fixes into four vertical slices plus one
standalone correction. Each slice delivers a coherent change to the product
rather than a fragment of one, and each is independently shippable in the order
given.

| Slice | Contains | Issue |
| --- | --- | --- |
| S1 Baton RC13 and truthful cockpit states | F1, F3 | #169 |
| S2 Supervised run lifecycle with detach and reconnect | F4 | #172 |
| S3 Structured activity with an honest event association | F5 | #173 |
| S4 Provider-neutral live output with bounded, explicit storage | F6a, F6b | #176 |
| S5 A usable on-ramp: `sworn init` and cwd-relative `sworn run` | F7 | #177 |
| S6 Move the native CLI pin out of the engine and into policy | F8 | #179 |
| Standalone: correct the plan fence error text | F2 | #170 |

The plan fence fix stays standalone deliberately. It is a wrong error string
with no coupling to anything else, and bundling it behind a vendor bump would
delay a correctness fix for no gain.

Superseded issues: #171 folded into #169, #174 folded into #176.

### S1. Baton RC13 and truthful cockpit states

Adopt Baton RC13 first, for the reason recorded above: RC12 raises
`TARGET_MOVED` on ordinary target movement, and explaining a spurious diagnostic
in plain language ships a well-worded false alarm. Then make every cockpit state
truthful.

**RC13 adoption is a protocol port, not a vendor bump.** `internal/baton` is a
Go port of Baton's reference implementation, so a reference behaviour change
must be mirrored by hand. Bumping the embedded assets alone would leave the
engine behaving like RC12 while reporting RC13: a silent divergence between the
vendored protocol and the code implementing it.

Inventory of the RC12 to RC13 delta the port must mirror. Reference tags
resolve locally as `v1.0.0-rc.12` at `caac9f0` and `v1.0.0-rc.13` at `4c9f9b5`.

| Reference change | Sworn site |
| --- | --- |
| `TARGET_MOVED` renamed `TARGET_DIVERGED`, meaning "the target history changed, reconcile it" | `internal/baton/state.go:648`, and `diagnosticExplanation()` at `internal/tui/view.go:407` still cases on `TARGET_MOVED` |
| Staleness by ancestry, not equality | `internal/baton/state.go:642`; the helper `repository.isAncestor` already exists at `internal/baton/repository.go:259` |
| Plan node state `revision_required` becomes `blocked` when the target is stale | graph projection |
| `planNextOperation` removed, so target movement no longer emits a planner handoff | this is the obsolete "replan" instruction shown on ordinary target movement |
| New `requireTargetLineage`, failing `TARGET_DIVERGED` | `internal/baton/actions.go` |
| New `prepareAssemblyCandidate`: assembly starts from release authority, adds the current target, then adds passed track products, so a later target advance stales only the assembly | `internal/baton/actions_assembly.go` |
| The approved target is the immutable track floor; live target movement cannot race preparation | assembly and track preparation |

The rename is why this slice and the cockpit work cannot be separated. If
`diagnosticExplanation()` is not changed in the same commit, the cockpit
silently falls through to its default text for a diagnostic Baton now emits
under a different name.

#### S1a. Surface projection failures as diagnostics, not as staleness

Carry the Baton error code through to the board instead of erroring the board
out, and split "read failed" from "data is stale".

- `Board` gains an explicit unreadable state, or the projection failure is
  returned as a `Diagnostic` on an otherwise empty board.
- `PLAN_NOT_FOUND`, `INVALID_PLAN_FENCE`, `REF_NOT_FOUND` and
  `INVALID_HEAD_OBJECT` gain entries in `diagnosticExplanation()`.
- `controlsAllowed()` stops keying the control lockout off the same flag used
  for staleness.
- Acceptance, against synthetic fixtures rather than `fired`: a release ref with
  no plan reads "this release has no plan"; a release ref carrying a
  `baton-plan-v1` document reads that its plan is an older format Sworn does not
  admit. Both fixtures are constructed in the test suite so they cannot go stale.

#### S1b. Explain why no controls are available

When a release has no manifest, say so on the board, name the directory Sworn
searched, and state that the manifest must be provided. Do not leave `a` silent.

- Acceptance: on a manifest-less release the board states that no run definition
  was found in `.sworn/runs/`, and pressing `a` shows the same sentence rather
  than nothing.

### Standalone. Correct the plan fence error text

`INVALID_PLAN_FENCE: plan must begin at byte zero` is raised for a v1 plan whose
fence does start at byte zero. Distinguish a wrong-version fence from a
misplaced fence so the message names the real cause.

- Acceptance: a `baton-plan-v1` document reports a version mismatch, not a byte
  offset.

### S2. Supervised run lifecycle with detach and reconnect

**Decided 2026-08-02: hand off to a supervised background driver.** The earlier
"or refuse to start and print the command" alternative is withdrawn. It
contradicts the cockpit being the product, and leaving an unresolved "or" in a
contract that autonomous workers consume invites exactly the divergence this
document is trying to prevent.

Starting a run detaches a supervised driver. The TUI follows the journal and can
be closed and reopened without affecting the run.

- Acceptance: the board keeps refreshing while a run started from the TUI is
  driving; quitting the TUI leaves the run driving; reopening reconnects to it
  without a `takeover`.
- Note: `sworn loop --parallel` is not dependable on a cold start, so a frozen
  cockpit holding a live run is a compounding failure, not a cosmetic one.

### S3. Structured activity with an honest event association

Build the feed the current journal can honestly support, and make the projection
able to support what the feed claims.

**The per-track promise does not hold today, and the gap is in the projection,
not the UI.** `Evidence` is `{offset, kind, created_at}`
(`internal/cockpit/model.go:125`). There is no effect, work, or track
association on an event, so "filter by track" has nothing to filter on. Stage
and outcome are available from graph nodes, and tokens and cost from
`AttemptView`, which is keyed by effect ID; only the events themselves cannot be
joined.

This slice therefore carries one minimal projection change, or it narrows its
promise. It must not claim per-track events without adding the association.

- Add a minimal event-to-effect association to `Evidence`, and derive the track
  from the effect. This is the smaller of the two honest options.
- Extend the TUI backend with a paged event read over `Projector.Events`.
- Filter by the selected graph node's track.
- Render kind, timestamp, and the node's stage and outcome, with attempt tokens
  and cost where present.
- Acceptance: selecting a track shows that track's events advancing during a
  live run, and the header's `LIVE · <offset>` corresponds to something the user
  can read. If the association is not added, the feed is presented as run-wide
  and the per-track claim is removed from the UI and from this document.

### S4. Provider-neutral live output with bounded, explicit storage

Added 2026-08-02 at Brad's direction, with a stated operational reason: he has
repeatedly stopped churn by watching verbose model output and noticing work
going off the rails **before** any of the automated checks fired. Live model
output is therefore an early-warning control, not a debugging luxury, and it
sits ahead of pruning policy.

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
- A credential tripwire already exists at exactly the right point:
  `containsCapability(line, capability)` fails the whole invocation if the
  credential ever appears in output. Note precisely what this is. It is
  `bytes.Contains` against one known credential
  (`internal/driver/broker.go:583`). It aborts, it does not redact, and it knows
  nothing about repository secrets or customer data. It is not a redaction
  layer and must not be described as one.

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

#### Storage follows the agent CLI convention

Decided 2026-08-02. Retain the full verbose history so it is replayable, and put
it in the user directory where the other agent CLIs put theirs, organised so
Sworn can find it.

Three conventions observed on this machine:

| Tool | Layout | Indexed by |
| --- | --- | --- |
| Claude Code | `~/.claude/projects/<encoded-path>/<session-uuid>.jsonl` | project path |
| Codex | `~/.codex/sessions/YYYY/MM/DD/rollout-<ISO8601>-<uuid>.jsonl` | date |
| opencode | `~/.local/share/opencode/` with `storage/`, `log/`, `tool-output/` | mixed, SQLite index |

What they agree on: the user directory rather than the repository, append-only
JSONL, one file per session, full content retained, and a stable key that makes
a session findable later.

Sworn's identity is richer than a chat session. A chat has one session; a Sworn
run has a release, a run, and many concurrent effects across tracks. The layout
should reflect that:

```text
~/.sworn/projects/<encoded-project>/
  project.json                 canonical absolute path, so the encoding is not authoritative
  <run-id>/
    run.json                   release, run, started, manifest digest
    <effect-id>.jsonl          one writer, one file, full verbose stream
```

Honour `XDG_STATE_HOME` when it is set. Default to `~/.sworn/` for symmetry with
`~/.claude` and `~/.codex`, which is where a user of those tools will look
first.

Two deliberate departures from the chat CLIs, both because Sworn is concurrent
where a chat is serial:

- **One file per effect, not per session.** Interleaving parallel workers into a
  single file makes per-track tailing hard and invites interleaved writes. One
  writer per file removes both problems and gives the F5 and F6a per-track
  filter for free.
- **The path encoding is not authoritative.** Claude Code's scheme replaces
  every `/` with `-`, which is lossy: `-home-brad-projects-fired-worktrees` is
  ambiguous between `/home/brad/projects/fired-worktrees` and a nested
  `fired/worktrees`. Since Sworn is required to *find* these reliably, keep the
  readable directory name for browsability but disambiguate it with a short
  digest of the real path, and record the canonical path in `project.json`.

This also strengthens the previous requirement rather than weakening it. A user
directory default is outside every repository by construction, so the common
path never touches a repository at all. The admission check from the previous
section becomes a backstop for an explicit override, not the primary defence.

#### The cockpit receives a provider-neutral contract

Added 2026-08-02 from review. Raw `stream-json` from Claude, or Codex's
`item.completed` envelopes, must never reach the cockpit. Piping provider JSON
to the UI couples every surface to provider schemas that change without notice,
and makes a second provider a rewrite rather than a driver.

The driver translates its native stream into one neutral record before anything
else sees it:

```text
effect · sequence · time · kind · text
```

`kind` is a small closed set, along the lines of `text`, `thinking`,
`tool_use`, `tool_result`, `usage`. Provider-specific fields are dropped at the
driver boundary, not filtered downstream. The retained JSONL is this neutral
record, which is also what makes replay portable across providers and across
Sworn versions.

#### Storage failure semantics belong in the first implementation

Added 2026-08-02 from review. "Retain everything", "bounded storage" and "decide
pruning later" cannot all be true, and the gap between them is where a first
implementation quietly does something unsafe. The following are settled before
the first line of storage code, not deferred to F6b:

- A per-effect size cap and a per-run cap, with defined behaviour on reaching
  them. Truncate and mark truncated, rather than stopping silently.
- Writes are asynchronous and bounded. A slow disk must not enter the driver's
  critical path.
- Disk-full, permission-denied, and directory-vanished all degrade to capture
  disabled with a visible cockpit warning. **A transcript write failure must
  never fail the run.** Capture is an observation surface, not a delivery
  dependency.
- The buffer between the driver and the writer drops oldest under pressure and
  records that it dropped. A gap that is marked is recoverable; a gap that is
  silent is a lie about what the model said.

S4e below covers the pruning defaults that ship with this slice.

#### Remaining constraints

- Keyed by effect ID so it filters to one track, matching S3.
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

#### S4e. Pruning defaults

Folded into this slice rather than deferred, because a storage implementation
that ships without a pruning default is a storage implementation that grows
without bound on someone's laptop.

Persistence, location, layout and consent are settled above. What remains, and
ships with the slice: when transcripts are pruned, whether a per-project size
ceiling exists, and whether a retained transcript is ever in scope for
attestation.

### S5. A usable on-ramp: `sworn init` and cwd-relative `sworn run`

Added 2026-08-02. The invocation a human has to type today is:

```sh
sworn run --manifest ABS --journal ABS [--config ABS]
```

Three absolute paths, all of which Sworn can already work out for itself. This
is fine for a script and hostile to a person. The predecessor tool set the bar:
`coach loop`, bare, kicked off or restarted the currently scoped release.

**The discovery already exists and only the TUI is allowed to use it.**
`resolveProjectPaths` (`cmd/sworn/tui_project.go:151`) defaults the journal,
config and manifest directory from the Git root. `discoverProject`
(`cmd/sworn/tui_project.go:53`) takes a start path defaulting to `os.Getwd()`
and walks to the root, so it already works from any subdirectory.
`discoverProjectManifests` already maps release name to manifest path. Meanwhile
`runStart` (`cmd/sworn/main.go:222`) hard-requires `--manifest` and `--journal`.
Both are `package main` in the same binary. The on-ramp is present and unused.

#### S5a. `sworn init`

Once per project. Writes `.sworn/drivers.json` and the project defaults that a
manifest needs but cannot be derived: role and model selections, recovery model,
and limits.

This is possible precisely because the driver config is specified as canonical
and **secret-free** (`docs/run.md`). Credentials are not in the file, so Sworn
can author the whole thing. Verify the result with the existing
`sworn driver doctor` rather than inventing a second check.

**Not once-only. Idempotent and re-runnable.** Once per project to get started,
but models get swapped, providers get added, and limits get tuned, so `init`
has to be safe to run again.

That safety has a specific edge, and it is not obvious. **The manifest pins
`driver_config_digest`** (`internal/runtime/manifest.go:105`), and
`validateDriverConfigMode` checks it at `Start`. So rewriting `drivers.json`
invalidates the digest in every manifest already minted against it, and those
runs stop being startable. A second `init` therefore must:

- never silently rewrite an existing `drivers.json`;
- show what would change and require explicit confirmation;
- when the config does change, report which existing manifests are now stale
  and re-mint them, since S5b mints manifests anyway.

An `init` that quietly bricks yesterday's manifests would be a worse failure
than the flags it replaces.

Open question, not blocking: whether a user-level `~/.sworn/drivers.json`
should seed the project file, matching how the agent CLIs keep provider
configuration per user rather than per repository. The project file stays
authoritative either way, because the digest binds to it.

#### S5b. Cwd-relative `sworn run`

```sh
sworn run                     # cwd, project, current scoped release
sworn run <release-name>      # named release
```

`--manifest`, `--journal` and `--config` are retained for scripts and exact
control. They stop being the only way in.

Resolution precedence, so that one verb covers "kick off or restart":

1. An unfinished run exists for the scoped release: continue it. Sworn decides
   from the journal whether that means resume, takeover, or plain continuation.
   Making a person diagnose an orphaned owner lease and reach for `takeover` is
   the same failure as the flags: Sworn knows and asks anyway.
2. No run, and exactly one startable release: start it.
3. Ambiguous: list the candidates and stop.

If no manifest exists for the release, mint one. Repository, release and target
ref are derivable from the Baton release; everything else comes from the
defaults written by `sworn init`. Failing that, fail with a message naming
`sworn init` rather than describing a schema.

- Acceptance: in a project with `sworn init` already run, `sworn run` from any
  subdirectory starts or continues the scoped release with no flags, and
  `sworn run <release-name>` selects a specific one. Quitting a supervised run
  and re-issuing bare `sworn run` reconnects rather than requiring `takeover`.

#### Parked: `--track <track-name>`

`sworn run <release> --track <track>` was specified and then deliberately
deferred on 2026-08-02. Recorded here so it is not re-derived as a gap.

The manifest has no track scope. `Manifest` carries `MaxParallelTracks`
(`internal/runtime/manifest.go:102`), which caps how many tracks run
concurrently but not which. Tracks exist in the run's projection
(`internal/runtime/service.go:112` carries a `Track` field) but never as a
dispatch scope. Supporting the flag needs a manifest field or a runtime scope
parameter plus scheduler support. That is a real change, and `coach loop` could
not do it either, so it is not a regression against the bar being matched.

Worth revisiting once S4 lands: scoping to one track is how you would sanely
dogfood a shaky engine, with a narrow blast radius and live output to watch.

### S6. Move the native CLI pin out of the engine and into policy

Found 2026-08-02 while preparing the first Sworn-driven run in this repository.

Sworn admits a native CLI driver only when the CLI matches a version and digest
**compiled into Sworn itself**:

```go
// internal/driver/native.go:12
CodexCLIVersion  = "0.146.0"
ClaudeCLIVersion = "2.1.208"

// internal/driver/native.go:174, validateNativeConfig
case ProfileCodex:
    if config.CLIVersion != CodexCLIVersion ||
        config.CLI.Digest != CodexCLIDigest ||
        config.CredentialTarget != CodexCredentialTarget ||
        config.VersionOutput != "codex-cli "+CodexCLIVersion {
        return fail("NATIVE_NOT_CERTIFIED")
    }
```

The runtime path is bound to the same constant: `nativeVersion`
(`internal/driver/native_linux.go:2106`) executes the CLI and its output is
compared to `config.VersionOutput`, which validation has already forced to equal
the constant.

Observed on this machine:

| CLI | Installed | Sworn pins | Result |
| --- | --- | --- | --- |
| Codex | `codex-cli 0.146.0` | `0.146.0` | admitted, digest matches exactly |
| Claude | `2.1.220 (Claude Code)` | `2.1.208` | `NATIVE_NOT_CERTIFIED` |

Claude Code was twelve patch versions ahead and therefore unusable, for no
reason connected to its behaviour.

**The defect is the layer, not the existence of a pin.** `NativeAdapterConfig`
already carries `CLI.Path`, `CLI.Digest`, `CLIVersion` and `VersionOutput`
(`internal/driver/native.go:26`). Validation then discards those values by
requiring each to equal a constant. The configuration fields are decorative:
the operator cannot express any pin except the one Sworn was built with.

The consequence is that **upgrading your agent CLI requires recompiling Sworn.**
These CLIs ship near daily. A provenance mechanism that forces a rebuild of the
engine on every upstream patch release will be routed around, and a mechanism
that gets routed around provides no provenance at all.

#### What the pin is actually for, and what follows

The goal is answering "exactly which agent produced this work", which
attestation depends on. That goal is served by **recording** the digest of what
ran, not by refusing to run anything except one blessed build. Recording is
strictly more informative than a whitelist of one, because it also captures the
versions nobody thought to bless.

- The digest and version in the driver config become authoritative. Validation
  checks the config against the **live binary**, not against a constant.
- The resolved path, digest and version output are recorded in the run record as
  provenance. This is content-free and belongs in the journal.
- Any further restriction is operator policy expressed in the config, not engine
  policy compiled into the binary: a minimum version, a set of accepted digests,
  or accept-and-record. Default to accept-and-record.
- `NATIVE_NOT_CERTIFIED` stops meaning "not the blessed build" and starts
  meaning what it says: the binary at the configured path did not match the
  configured identity.

This is a prerequisite for S5a. `sworn init` derives a driver config from a live
install, which is only meaningful if the config it writes is authoritative.
Today `init` could only ever write the one digest Sworn was compiled against,
and would fail on any other machine.

- Acceptance: a native CLI one patch release ahead of whatever Sworn was built
  against is admitted, its exact digest is recorded in the run record, and
  tightening beyond that is expressible in the driver config without rebuilding
  Sworn.

## What was not changed

Nothing in `fired`. The two unreadable releases there are genuine data states
and are useful fixtures for F1 and F2.
