# Start and operate Sworn

This guide covers a current Sworn `1.0.0-rc.2` production run. Sworn owns the
handoffs and approvals, carries the work, saves progress, and stops when it
cannot continue safely.

## What you need

Sworn does not yet create a delivery plan, run manifest, or AI connection file.
The release or deployment process must provide:

- a repository with the required release records;
- an approved, canonical `sworn.runtime-manifest/v5` file;
- a canonical, secret-free `sworn.driver-config/v1` file whose digest matches
  the manifest; and
- an absolute path for the private SQLite journal that will hold this run.

All supplied paths must be absolute and clean. The manifest is a compact JSON
document with a final newline. It binds the repository, release, target branch,
intent, approval source, role/model choices, recovery model, limits, and driver
configuration digest. It contains no credential values.

The journal is the saved run record. Sworn creates it with mode `0600` if it
does not exist; its parent directory must already exist and must not be reached
through a symlink. Reuse the same journal path when viewing, controlling, or
recovering that run.

## Open the project view

From anywhere inside the Git project, run:

```sh
sworn
```

In an interactive terminal, Sworn finds the project root and opens a list of
its local release records and saved Sworn runs. You can move between release
boards, including releases that do not have a Sworn run yet. When a run
does exist, its live state, questions, and available controls appear on that
board.

The TUI offers only controls allowed by the current board. It does not decide
that an action is safe on its own. The full commands described below remain
available for scripts and exact run control.

`sworn tui` opens the same view explicitly. Its default project files are:

```text
.sworn/sworn.db       saved runs
.sworn/drivers.json   AI connection configuration
.sworn/runs/*.json    run manifests
```

Viewing the project does not create these files. Override their locations only
when needed:

```text
sworn tui [--project ABS] [--journal ABS] [--config ABS] [--manifest-dir ABS]
```

Bare `sworn` prints help instead of opening the TUI when its input or output is
piped or redirected.

## 1. Check the AI connection

Check one profile and model before starting:

```sh
sworn driver inspect \
  --config /absolute/path/drivers.json \
  --profile openai \
  --model YOUR_EXACT_MODEL \
  --json

sworn driver doctor \
  --config /absolute/path/drivers.json \
  --profile openai \
  --model YOUR_EXACT_MODEL \
  --json
```

The checks are deliberately different:

- `inspect` confirms that the profile, model, adapter, and configuration fit
  together. It does not contact the provider.
- `doctor` checks the local executable or connection boundary. It does not make
  a paid HTTP model request. Its JSON output carries `"live_call": false`, so
  a reader never has to infer that from the command name alone.
- `certify` makes the separately authorized live check. It needs real
  credentials, runs the whole agent loop, and may consume provider usage.
  Its JSON output carries `"live_call": true`.

Run live certification only when that use is intended:

```sh
sworn driver certify \
  --config /absolute/path/drivers.json \
  --profile openai \
  --model YOUR_EXACT_MODEL \
  --json
```

Use `--all` instead of `--profile` and `--model` only with a release-wide
configuration that includes every profile of the single declared production
roster: the `codex_cli`, `claude_code_cli`, `openai_compatible_http`,
`gemini_generate_content` and `bedrock` families, plus the
`bedrock_runtime_converse` surface. `doctor --all` and `certify --all` name
every missing family and surface in the refusal detail (for example
`missing families: bedrock; missing surfaces: bedrock_runtime_converse`), with
a note that `--all` checks the complete production roster while
`--profile P --model M` checks one lane.

Each JSON report has a `state` and a stable `code`:

- `PASS` means the selected check passed.
- `FAIL` means the check ran and found a problem.
- `NOT_CERTIFIED` means that exact model is not listed for certification in the
  selected profile.

Read the result in the context of the command: an `inspect` pass confirms
configuration, while only a `certify` pass confirms the live provider path.

### Live lane probe

`sworn driver probe` proves one configured lane is admitting requests right
now, without running the agent loop or submitting a repository byte:

```sh
sworn driver probe \
  --config /absolute/path/drivers.json \
  --profile openai \
  --model YOUR_EXACT_MODEL \
  --json
```

Its cost is one minimal request per lane: no tools, a small declared output
bound (16 tokens), the fixed literal prompt `sworn lane probe`, and no
repository content. Unlike `inspect`, `doctor` and `certify`, `--json` is
optional; the default is one human-readable line carrying the same typed
code, provider message, provider request id and latency.

Use `probe` instead of `certify` when the question is "is this lane admitting
requests right now", not "does the whole submission contract still hold":
probe checks admission only (a live 2xx response), never the agent loop or
the submission contract certify proves. It is the check a Manager seat runs
every few minutes while waiting out a provider stall (manager policy M4);
`certify` stays the release-wide, separately authorized live check. `probe`
always makes a live call (`"live_call": true`), just as `certify` does.

## 2. Start the run

For a production manifest, supply the matching driver configuration:

```sh
sworn run \
  --manifest /absolute/path/run.json \
  --journal /absolute/path/run.sqlite \
  --config /absolute/path/drivers.json
```

Sworn prints a readable summary first: the run status, what is happening, what
comes next, whether a person is needed, and what it checked. Stable state names,
digests, and generation numbers remain under `Technical details`.

Scripted manifests are for deterministic tests and recovery compatibility.
They contain their own scripted attempts and must not be combined with a
production driver configuration.

## 3. See what is happening

The project TUI is the normal view for a person. To read one exact run from a
script or print its complete terminal report, use:

```sh
sworn board \
  --run RUN_ID \
  --journal /absolute/path/run.sqlite
```

It begins with the same readable summary and then shows the complete recorded
facts under `TECHNICAL DETAILS`. Add `--json` when another program needs the
stable `sworn.cockpit/v2` document:

```sh
sworn board \
  --run RUN_ID \
  --journal /absolute/path/run.sqlite \
  --json
```

`sworn status` is machine-readable only:

```sh
sworn status \
  --run RUN_ID \
  --journal /absolute/path/run.sqlite \
  --json
```

### Worker-turn journal

Every dispatch, on every lane, journals one bounded, redacted record per
worker turn as it happens. An HTTP-lane dispatch has always journaled a
`tool_result_observed` event (schema `sworn.tool-result-turn/v1`) per turn's
tool results; a native CLI-lane dispatch (Claude or Codex) now does the
same, correctly keyed one turn at a time instead of collapsing onto turn 0.

Native CLI lanes additionally journal what the worker said and which tools
it called, as a `worker_turn_observed` event (schema `sworn.worker-turn/v1`)
beside the tool-result event for the same turn. Each event carries the same
identity fields as a tool-result event - run, track, slice, role,
responsibility, attempt, epoch, try, work and effect identifiers, turn, and
part/parts geometry for a turn split across multiple events - plus an
ordered `content` list of bounded parts. Each part names a `kind` (`text`,
`reasoning` where the CLI emits it, `tool_call` with the tool name and its
bounded canonical-JSON input, or `tool_result_reference` naming the tool
call a result belongs to rather than duplicating its bytes), `total_bytes`,
`omitted_bytes`, `redacted_bytes`, and a `head`/`tail` pair of standard
base64-encoded bytes bounded to 2,048 bytes each. Every capability,
capture bearer, and credential fragment is redacted before it ever reaches
the journal. `dropped_events` names any worker-turn event the reader could
not decode or recognize, loudly, rather than silently discarding it.

To profile one dispatch by turn, read the journal and filter both event
kinds on that dispatch's identity fields (run, work, effect, attempt, epoch,
try), then group by `turn`: a native-lane dispatch reads back exactly like
an HTTP-lane dispatch, one row (or named part) per turn.

### Live worker activity

While a dispatch runs, its turns are visible turn by turn on every lane,
in the browser board's activity pane and in the TUI's activity screen.
Both render the same activity projection over the journaled worker-turn
and tool-result events: for a run, optionally narrowed to a track, slice
or dispatch, an ordered, paged list of turns, each with its identity
(slice, role, responsibility, attempt, try, turn), the bounded decoded
parts of the worker turn and the tool results keyed to that turn, with
omitted and redacted byte counts and dropped-event counts shown rather
than hidden.

The browser board shows the live dispatch's turns in order for the
selected work, each with the role, the turn number, the worker's bounded
text, the tools it called and each tool result's pass or fail state and
size, following new turns as they arrive. It opens the activity stream
only while the pane is visible and closes it when the pane or run
changes. The TUI's activity screen (`v` from the board for the selected
work) is fed through the same projection over the journal, refreshed on
the 2s cadence, scrollable with `j`/`k` (`g`/`G` for top and bottom), and
honest about narrow terminals. The TUI never connects to the serve host;
it stays a direct journal reader.

The serve host exposes the same projection on a new route under the run's
API:

```text
GET /api/v2/runs/<run>/activity?after=<offset>&limit=<n>&track=<t>&slice=<s>&effect_id=<e>&work_id=<w>
```

As JSON it returns one `sworn.activity/v1` page. When the client accepts
`text/event-stream` it returns server-sent events whose `activity` frames
carry the turn content (`{"schema_version":"sworn.activity/v1","turn":{...}}`)
with `id` set to the turn's durable offset. The stream resumes exactly
from `Last-Event-ID` or `after` with no gap and no duplicate, keeps the
1s keepalive cadence, and shares the concurrent-stream gate and its
`SSE_LIMIT` refusal with the existing events route. The existing events
route keeps its contract byte for byte: `invalidate` frames that carry
only the schema version and the through offset.

What is retained: the journaled worker-turn and tool-result events
themselves, bounded to 2,048 head and tail bytes each with omitted,
redacted and dropped counts. What is not retained: the in-memory ring
that shortens latency when the serve host drives the run. That ring is
bounded to 64 turn events and 256 KiB per live dispatch, drops its
oldest turn with a loud count when full, never blocks the dispatch, is
dropped when the dispatch ends, and is never persisted. When the run is
driven by another process the route serves the same content from the
journal alone (`live:false`) and says nothing false about liveness; when
the serve host drives the run in-process the route merges the ring ahead
of the journal on the one durable cursor (`live:true`).

### Failure turn context

When a dispatch fails operationally, its durable failure record carries a
bounded tail of the worker's last turns, so an operator or a Manager seat
can see what the worker was doing when it failed without opening the
journal. The context rides on the failure or uncertain event body in the
same journal transaction as the failure itself, so a reader never sees a
failure without its context or a context without its failure. It is read
back through the digest-checked journal read, so tampering surfaces as
`CORRUPT_JOURNAL` rather than shown.

What it holds: the last turns of that dispatch attempt, at most 5
turn-events and 48 KiB of context JSON, each as the same bounded redacted
projection the worker-turn journal holds (2,048 head and tail bytes each,
with omitted, redacted and dropped counts), with the count of earlier
turns omitted. An HTTP-lane dispatch shows tool-result turns; a native
CLI-lane dispatch shows interleaved worker and tool-result turns. The tail
behind it holds the newest 16 turn-events and 128 KiB per live dispatch,
fed only after the durable journal append, so it costs no second read of
the provider stream and no journal scan at failure time.

Empty, absent, and unavailable have one meaning each. `empty` (explicit
`turns: []`) means the dispatch failed before any turn was observed in its
live tail. Empty only ever means that: a dispatch that took turns always
carries at least its newest turn, truncated by whole parts (latest kept,
dropped counted on the turn) when that turn alone exceeds the byte bound,
never an empty list with omitted turns. `absent` (no `failure_turn_context`
key) means a record written before this release, or a sweep reconcile
(`implementation_dispatch_uncertain` and its siblings) that never held the
dispatch. `unavailable` with a named reason (`no_live_tail` for a
prior-process or ownerless path with no live tail, `tail_encode_failed`
and `context_over_budget` as defensive loud fallbacks) means the failure
is journaled exactly as today but its tail could not be produced;
assembling context never turns one failure into another and never alters
the failure code, the refusal result, the observation digest, or the try
accounting. Dropped turns are counted as `dropped_max_visible`: the maximum
observer drop count visible in the retained tail, not an exact total, since
a drop with no later success never rides onto a journaled turn.

Where each surface shows it: `sworn status --json` carries
`failure_turn_context` (schema `sworn.failure-turn-context/v1`) on each
failed `driver.dispatch` effect beside its failure code, and on each pinned
work beside its code and detail; `sworn board --json` (`sworn.cockpit/v2`)
and the `sworn_status` tool carry the same field on effects and, for a
pinned lane, on its actionable work-detail node (the track's ready slice,
or the assembly node for the release lane); the browser board's work detail
and the TUI's work detail render it in parity (the TUI truncates to its
detail budget with an honest "+N more" line). A following successful try of
the same work shows no context (absent, not a stale copy), and a healthy
lane shows no pinned context.

The driver-side causes are unchanged and still win: the CLI's own error
result, the provider-limit classification, the redacted stderr tail, and
every typed failure code keep their present precedence and text. The turn
context is additional evidence beside the cause, never a replacement for
it and never parsed to derive a cause, a park, or a retry decision. See
"Pause, resume, cancel, or recover" below for the recovery verbs that use
those causes.

## 4. Use the local browser board

For an existing run:

```sh
sworn serve \
  --run RUN_ID \
  --journal /absolute/path/run.sqlite \
  --config /absolute/path/drivers.json
```

By default, Sworn listens only on `127.0.0.1:7337`. Open:

```text
http://127.0.0.1:7337/runs/RUN_ID
```

Add `--manifest` only when the operator service must accept a start request for
that exact manifest. Without an operator configuration, there is no public
listener, webhook delivery, or telemetry export. See
[docs/launch.md](launch.md) for the launch refusals `serve` prints and what
to do about them.

## 5. Pause, resume, cancel, or recover

Controls include the generation from the latest board plus a new command ID.
These values stop an old screen or script from changing a newer run. The board
JSON `actions` list supplies the current generation and, for retries, the work
digest and epoch. The TUI uses this same list; it never exposes a control that
is absent from the current board.

```sh
sworn pause \
  --run RUN_ID \
  --journal /absolute/path/run.sqlite \
  --command UNIQUE_COMMAND_ID \
  --generation CURRENT_GENERATION

sworn resume \
  --run RUN_ID \
  --journal /absolute/path/run.sqlite \
  --command UNIQUE_COMMAND_ID \
  --generation CURRENT_GENERATION \
  --config /absolute/path/drivers.json
```

`cancel` stops the run after in-flight work reaches a safe boundary.
`takeover` resumes a run whose previous Sworn process stopped. `retry` applies
only to the exact stopped work item and epoch shown by the latest board:

```sh
sworn retry \
  --run RUN_ID \
  --journal /absolute/path/run.sqlite \
  --command UNIQUE_COMMAND_ID \
  --generation CURRENT_GENERATION \
  --work SHA256_FROM_LATEST_ACTION \
  --epoch EPOCH_FROM_LATEST_ACTION \
  --config /absolute/path/drivers.json
```

A work item that stopped because it reached its configured API-turn,
API-output-token, or native-output-byte budget parks with its code retained
and no accepted candidate; `retry` alone is refused for it
(`ECONOMY_GRANT_REQUIRED`). `grant` admits an explicit, finite, bounded
capacity increase for exactly the named unit and work item, then the same
run continues from the preserved work:

```sh
sworn grant \
  --run RUN_ID \
  --journal /absolute/path/run.sqlite \
  --command UNIQUE_COMMAND_ID \
  --generation CURRENT_GENERATION \
  --work SHA256_FROM_LATEST_ACTION \
  --epoch EPOCH_FROM_LATEST_ACTION \
  --unit economy_turns \
  --amount 50 \
  --config /absolute/path/drivers.json
```

`--unit` is one of `economy_turns`, `economy_output_tokens`, or
`economy_output_bytes`, matching the board's named exhausted unit. A grant is
refused above the hard per-invocation ceiling (`GRANT_ABOVE_HARD_CEILING`),
for the wrong unit (`GRANT_WRONG_UNIT`), or when this work's recorded spend
carries a crash-before-usage-receipt gap that has not been acknowledged
(`ECONOMY_USAGE_UNKNOWN`) — add `--acknowledge-unknown-usage` only once you
have reviewed that gap and accept resuming within this work's own
already-declared ceiling. A grant unblocks only its named work and unit; it
never changes the driver, model, or any other limit.

Sworn's orchestrator handles a worker turn that ends with a question, reports a
block, or does not return a usable handoff. It can resume the same worker with
an answer grounded in saved facts, ask the Lead for advice, retry an
operational failure, or park only that track for a human answer. Independent
tracks can continue.

The orchestrator is not a sixth role. It cannot approve a plan, invent a
Lead decision or Verifier verdict, or merge code. When it parks a track, the
browser board provides an answer form. The equivalent command is:

```sh
sworn answer \
  --run RUN_ID \
  --journal /absolute/path/run.sqlite \
  --attention ATTENTION_SHA256 \
  --generation 1 \
  --answer "YOUR ANSWER" \
  --config /absolute/path/drivers.json
```

When a dispatch fails operationally, its failure record already carries
the bounded tail described under "Failure turn context" above: read the
failed work's `failure_turn_context` on the latest status or board before
retrying, so the retry decision sees what the worker was doing when it
failed.

## What the run statuses mean

| Status shown | Recorded state | What to do |
| --- | --- | --- |
| Sworn is working | `running` | No action unless Sworn asks a question. |
| Waiting for approval | `awaiting_approval` | Approve the proposed plan through the configured approval source. |
| Pausing safely | `pausing` | Wait for current work to reach a safe stopping point. |
| Paused | `paused` | Resume when you want work to continue. |
| Cancelling safely | `cancelling` | Wait for current work to stop safely. |
| Cancelled | `cancelled` | No further delivery work will run. |
| Stopped and needs your attention | `parked` | Answer the saved question or review the current retry action. |
| Resume required | `takeover_required` | Use `takeover` with the latest generation and driver configuration. |
| Needs confirmation | `uncertain` | Recover the run before repeating the last external action. |
| Complete | `complete` | No delivery work remains. |

`uncertain` means Sworn cannot confirm whether the last external action
finished. It will not repeat that action until recovery can do so safely.

## Checkpoints and work preservation

When an implementation dispatch terminates without a successful handoff (such as
from an error, bounded budget pause, or graceful cancellation), Sworn captures an
unverified product checkpoint before disposable workspace cleanup. On an
authorized retry with unchanged authority and prepared base, Sworn restores
these product additions, edits, deletions, and executable permissions before the
worker begins.

Saved checkpoints are unverified product recovery data, never candidate
admissions, verification verdicts, or merge permissions. Checkpoint trees are
anchored by Git references under `refs/heads/checkpoints/...` so they survive
Git garbage collection (`git gc --prune=now`).

### Storage bounds and defaults

Checkpoint storage has reviewed finite bounds:

- **Max bytes per checkpoint**: 64 MiB (`MaxCheckpointBytes`).
- **Max files per checkpoint**: 2,048 files (`MaxCheckpointFiles`).
- **Retained generations**: 3 generations per work item (`MaxCheckpointGenerations`).
- **Aggregate capacity**: 256 MiB per repository/run (`MaxAggregateCheckpointBytes`),
  measured by staged bytes at capture time.

Superseded generations are pruned in crash-safe order: older refs are removed
only after a durable replacement ref and journal event are committed. Automatic
cleanup never evicts the last recoverable copy of unfinished work. When
aggregate capacity is reached and no superseded copies remain to prune, Sworn
pauses capture and new dispatch with the named remedy
`CHECKPOINT_CAPACITY_EXCEEDED`.

### Quarantined workspaces and operator reclamation

If checkpoint capture encounters a fault condition (such as `ENOSPC`,
`CHECKPOINT_OVERSIZE`, `CHECKPOINT_TOO_MANY_FILES`,
`CHECKPOINT_UNSUPPORTED_ENTRY`, `CHECKPOINT_SCOPE_VIOLATION`, or
`CHECKPOINT_CAPACITY_EXCEEDED`), Sworn does not delete the workspace. Instead,
it quarantines the worktree under a `.fence` record.

Startup and shutdown cleanup skips fenced workspaces, and new writers are
refused with `WORKSPACE_FENCED` to prevent overwriting uncheckpointed progress.
The board and `sworn status --json` distinguish unverified saved work
(`saved`), restored work (`restored`), interrupted-process recovery data
(`salvaged`, or `restored_salvaged` once restored), and capture failure with a
fenced workspace (`fenced`), reporting the affected slice and failure reason.
See "Interrupted-process reconciliation" below for `salvaged`.

To reclaim or resolve a quarantined workspace:

1. Check the fenced workspace path reported by `sworn status --json` or `sworn board`.
2. Inspect or salvage the worktree files under that path if needed.
3. Remove the quarantined worktree and lease:
   ```sh
   git worktree remove --force <path-to-fenced-worktree>
   ```
   Or remove the `.fence` marker file inside the workspace root so normal
   abandoned-workspace cleanup can reclaim it.

### Interrupted-process reconciliation

If the driving process is killed (host crash, `SIGKILL`, power loss) after a
production implementation worker has opened its workspace and written scoped
code, but before it hands off or checkpoints, that worktree is not silently
deleted by the replacement owner's ordinary abandoned-workspace cleanup.
Sworn durably attributes an implementation workspace to its run before any
driver dispatch begins; a replacement owner's first owned cycle scans for
attributed abandoned workspaces, admits the sole writer for that track (a
live prior worker or an already-quarantined workspace is left alone), and
either finds nothing changed since its prepared base or captures the
interrupted bytes as an explicitly **unverified salvaged checkpoint** before
the worktree is reclaimed.

A salvaged checkpoint is not the same durability claim as an ordinary saved
checkpoint:

- **`saved`**: a completed capture, staged and measured by the same worker
  that wrote it, with the tree digest recorded before the workspace closed.
- **`salvaged`**: recovered from a workspace whose owning process never
  reached its own checkpoint or handoff. It may be a complete write, or it
  may reflect a filesystem write that had not finished when the process
  died; sudden power loss during an unacknowledged write is never reported
  as zero-loss recovery. `sworn status --json` and the board report
  `salvaged` (or `restored_salvaged` once a later attempt restores it)
  distinctly from `saved`/`restored`, alongside the same affected slice,
  tree digest, and file/byte counts.

A changed plan, contract, or track base between the interrupted attempt and
the replacement owner's authority blocks automatic restoration into the new
authority: the checkpoint stays exactly where it is, under its Git ref and
journal row, inspectable with a specific `stale_base`, `stale_plan`, or
`stale_contract` reason, rather than silently rebasing or discarding it.
Ownership mismatch, a foreign run or repository attribution, or a corrupt
recovery binding fences the workspace the same way a capture fault does,
rather than restoring it or clearing the way by deleting it.

## Configure one AI connection

The driver file contains connection descriptions and credential references,
not secrets. This is a complete canonical example for an OpenAI Responses
profile; replace `YOUR_EXACT_MODEL` and choose the exact endpoint, API, and
reasoning effort intended for the run:

```json
{"schema_version":"sworn.driver-config/v1","credentials":[{"key":"openai-env","kind":"environment","reference":"OPENAI_API_KEY"}],"adapters":[{"openai":{"key":"openai-responses","id":"sworn.openai","version":"1.0.0","endpoint":"https://api.openai.com/v1/responses","credential_header":"Authorization","credential_prefix":"Bearer ","credential_refs":["openai-env"],"response_bytes":1048576,"api":"responses","reasoning_effort":"medium"}}],"profiles":[{"key":"openai","adapter":"openai-responses","network":"required","credential_source":"openai-env","certification_models":["YOUR_EXACT_MODEL"]}]}
```

The driver file must match this compact JSON form exactly, with no trailing
newline or whitespace. Provide `OPENAI_API_KEY` to the Sworn process through
the host's secret manager or private service environment; do not place its
value in the file or directly in a shell command.

Credential references may select an environment variable, an owner-only file,
or the AWS credential chain, depending on the adapter. Native Codex and Claude
profiles additionally bind the exact CLI binary, version output, required
runtime files, and owner-only credential file. Bedrock profiles explicitly
choose Runtime Converse or Mantle Chat; Sworn never switches between them.

A native profile may name any CLI version. The configured digest and version
output are the pin: every launch checks the binary's bytes and its `--version`
against them, so a run always uses exactly the CLI the file names. Sworn is
tested with Claude Code 2.1.241 and Codex 0.146.0. `sworn driver doctor` and
`certify` report `cli_compatibility` on native lanes: `tested` for those
versions, and `untested` for any other, with a note that the CLI may work but
compatibility and stability are not guaranteed. An untested version never
changes the readiness state or code. `pin_mode` is still accepted for existing
files and no longer changes admission. To move to a new CLI release, point
`cli.path` at a frozen copy of it and update `cli.digest`, `cli_version` and
`version_output`; the new configuration digest then goes into the next run's
manifest.

A CLI that updates itself does not need a frozen copy. Set
`"cli_resolution":"run_snapshot"` on the native adapter, point `cli.path` at
the installed command by absolute path (a symlink such as
`/home/you/.local/bin/claude` is followed; a bare command name is refused),
and leave out `cli.digest`, `cli_version` and `version_output`. When a run
starts, Sworn copies the CLI into a read-only store under
`$SWORN_ARTEFACT_HOME/native-cli-snapshots` (default
`~/.local/share/sworn/native-cli-snapshots`), names the copy by its SHA-256,
and records its path, digest and `--version` output as a
`native_cli_snapshot` event before the first dispatch. Every dispatch, restart
and `sworn retry` in that run uses that copy and never reads the installed CLI
again. A run that cannot take the copy stops with
`NATIVE_CLI_SNAPSHOT_UNAVAILABLE`; a copy that later goes missing or changes
stops it with `NATIVE_CLI_SNAPSHOT_INVALID` and is never replaced.
`sworn driver doctor`, `probe` and `certify` read the installed CLI the same
way, without copying it, and report the version a run would take in
`cli_compatibility`.

A header credential file may end in a line ending, which is ignored. A file
that is otherwise empty, or that holds any other control byte, is refused as
`CREDENTIAL_MALFORMED`, and `sworn driver doctor` fails it as
`credential_malformed`.

An OpenAI-compatible adapter may declare `max_output_tokens`, an optional
integer from 1 to 1048576, when the provider's output ceiling is lower than
the limit Sworn would send. Certification and dispatch then send the smaller
of the two; leaving the field out changes nothing.

Either OpenAI-compatible surface (chat completions or responses) may also
declare `context_window_tokens`, an optional integer up to 10,000,000
naming the model's total context window. When set, every request after the
first clamps the output ceiling actually sent to the room left in that
window after the previous turn's reported input tokens, minus a small fixed
safety margin: `min(max_output_tokens, context_window_tokens -
last_input_tokens - margin)`. This only ever lowers what would have been
sent; the first request of a dispatch carries no prior turn to clamp
against and is unaffected, and leaving the field out changes nothing. When
the room left cannot fit even a minimal reply, the adapter refuses the turn
before sending it with the typed code `ECONOMY_CONTEXT_EXHAUSTED`, naming
the window, the last input tokens, and the ceiling. That refusal parks the
work under the `economy_context_window` cause: unlike a turn- or
output-token economy park, it is never Grant-eligible (there is no
manifest limit to raise for a fixed context window), so its only unblock
verb is a bare retry, admitted immediately on any try. A retry issued
without first editing `context_window_tokens` or `max_output_tokens` in the
driver config will simply reach the same refusal again. The status
projection's dispatch view and the live activity stream also show the
latest turn's reported input tokens for an in-flight or failed dispatch, so
a context approaching its window is visible before it is exhausted.

A Responses adapter may also declare `reasoning_summary` (`auto`, `concise`
or `detailed`), which asks the provider to stream a reasoning summary while
the model thinks. Set it when a provider cuts a streaming request whose first
event has not arrived within its own limit, since without a summary the first
event waits for the whole think and the longest turns are exactly the ones
dropped. The summary is rendered on the live stream only; leaving the field
out sends `reasoning.effort` alone, as before.

Sworn does not currently include a driver-config generator. Production
provisioning should create the canonical file and use the
`configuration_digest` reported by `sworn driver inspect` in the run manifest.

## Optional local operator settings

The browser service needs no configuration for its local default. To choose a
different loopback port, create an owner-only file such as:

```json
{"schema_version":"sworn.operator-config/v1","local":{"listen":"127.0.0.1:7444"}}
```

```sh
chmod 0600 /absolute/path/operator.json
sworn serve \
  --run RUN_ID \
  --journal /absolute/path/run.sqlite \
  --config /absolute/path/drivers.json \
  --operator-config /absolute/path/operator.json
```

The file must be a regular, non-symlink file reached by a clean absolute path.
Public listening is opt-in and requires an exact origin, TLS certificate,
private key, and access token. Webhook destinations and OpenTelemetry export
are also opt-in. Do not expose the operator service publicly until those
settings have been provisioned and reviewed.

## Current limits

Sworn does not currently provide `init`, plan creation, manifest generation,
driver-config generation, provider/model defaults, or credential hosting.
Approval still comes from the source named in the manifest. Telemetry can
report what happened but cannot approve, block, or advance work.

Linux production execution requires root-owned `bwrap` discoverable on PATH
(for example `/usr/bin/bwrap`) and unprivileged user namespaces. Live
`driver certify`, `driver probe`, and production runs can
consume provider usage; the ordinary Go test suite does not make live provider
requests.
## Host-check repair input

When an implementation's host check fails, Sworn retains the exact failed
check (command, candidate, output, exit status and digest) and the submitted
handoff as `host_repair` in the failed dispatch. This is **unverified repair
input**, not a candidate receipt or a verifier PASS. A same-authority retry
restores its matching checkpoint and receives that context so it can repair
the existing work. Fresh host checks and independent verification still gate
delivery. Missing legacy context or mismatched checkpoint, plan, candidate or
check bindings refuse model dispatch instead of starting a blind rebuild.

Repeated failures publish the existing typed park event at the between-tries
gate, with a retained-candidate diagnostic, so configured notification
consumers can observe the stop without waiting for another scheduler tick.
This does not invent a human approval question or authorize an automatic
budget increase.

### Transient provider stall backoff

When a try fails with `PROVIDER_UNAVAILABLE`, or `PROVIDER_LIMITED` with no
provider-named reset time, the engine does not start the next try
immediately: it waits with a bounded backoff (60, 120, 240, then 240
seconds; a `PROVIDER_LIMITED` reset time is used instead of the first step
when one is named), probes the same lane after each wait with the identical
live probe `sworn driver probe` makes, and starts the next try only once a
probe passes. Every wait and probe is journaled, and the status projection
shows a work waiting this way, its next probe time and the last probe
result, so it reads as a wait, not a hang. If no probe passes within a
declared total bound of 30 minutes, the run parks with the typed
`provider_stall` cause, naming the failure code, the wait so far and the
last probe result; manager policy M4 covers it exactly as it covers any
other provider refusal. The try budget, `identical_failure_park_after` and
every other failure code's handling are unchanged: this only changes when
the next try of a transient provider failure starts.

### Host-check failure fact

For the latest failed host check of a work, the status projection shared by
`sworn status --json`, `sworn_status`, the board and the TUI carries one
bounded host-check failure fact (`sworn.host-check-failure-fact/v1`): the
check command as declared in the contract, its outcome and exit code,
whether it was re-executed and that re-execution's outcome, the declared
checks that were not run because this one failed, and a bounded output
excerpt taken from the same stored result the implementer's repair context
already holds.

The fact is derived at Status time from the already-journaled,
digest-checked `check.host` results; it adds no journal write and no new
field on the repair, result or work-context records. It is evidence, never
authority: nothing in the engine reads it to decide a retry, a park, a
verdict or a repair, and the repair context the implementer receives is
unchanged. Records written before this release report the fact as absent
rather than corrupt.

Bounds: the excerpt carries at most 4096 bytes of the stored output with a
truthful truncation marker; the whole fact is capped at about 8 KiB;
`not_run` is bounded by the declared contract length and is derived as the
checks after the failed check's position in the phase order the engine used
(quick checks first, long suites after). When the resolved contract does not
match the stored result's contract digest, the failed check is not in its
list, or the contract cannot be resolved at Status time, `not_run` is
absent and marked unknown rather than guessed. A later terminal dispatch
for the same work clears the fact; an in-flight next try does not.

Each `check.host` effect in the projection also reports the check's outcome
(`pass`, `fail`, `timeout`, `overflow`) beside the effect state, so an
executed-and-failed check no longer reads only as `succeeded`. Journal
effect states are unchanged. The `HOST_CHECK_FAILED` refusal detail names
the check command and exit code alongside the outcome and candidate.

## Assembly host-check evidence

At assembly preparation the engine executes the union of the declared
`host_checks` of every slice in the assembly against the exact composed
candidate, once per candidate, in the same phase order as a slice seal
(quick checks first, long suites after). Each check is journaled as a
`check.host` effect keyed by the assembly candidate (an empty slice, the
candidate, the assembly's union contract digest and the check), exactly-once
like a slice's, so a relaunch or a retry replays the recorded results instead
of re-running them. One reuse rule applies: when the assembled product tree
is exactly the product tree of a slice candidate whose recorded result for
the same check command in this run's journal is a pass (a single serial
track fast-forwards to its last verified candidate), that record is cited
instead of executing again. Identity is the product tree, the identity every
candidate receipt carries: the assembly is composed from the release head,
whose reserved record root holds the installed plan and contract records a
track candidate never carries, so the Git trees of an identical product
differ there and only there. The engine-built manifest of those results becomes the
assembly candidate receipt's checks digest, exactly as the slice seal binds
its manifest; an assembly whose slices declare no host checks keeps the
input-pin digest. A failing, timed-out or overflowed check refuses the
preparation under the existing `HOST_CHECK_FAILED` path: the
`prepare_assembly` action fails operationally on each try (a plain failure
gets the one bounded re-execution a slice candidate gets), and the spent try
budget parks the run on `exhaustion` with `HOST_CHECK_FAILED` and a detail
naming the check and the candidate. There is no assembly repair dispatch:
the assembled product is composed from verified slices, so only new slice
work changes the tree.

The assembly verification dispatch receives the same
`protocol/host-evidence.json` input as a slice verification, as a roll-up
(`sworn.assembly-host-evidence/v1`) with two parts. The `assembly` section is
the evidence produced in the judging run about the assembled tree itself: the
candidate, its Git tree and product tree, the union contract digest, the
digest of the manifest rebuilt from the journaled results, whether the
receipt's checks digest is that digest (`receipt_binds_manifest`), and each
check with its outcome, exit code, output digest, `host_effect` and, for a
reused record, the slice it was reused from. It is `"evidence": "proven"`
only when every declared check resolves to a recorded pass for exactly this
candidate's product tree in this run's
journal, `"missing"` with a reason code otherwise, and `"none_declared"` when
no slice declares a host check. The per-slice `slices` section stays as
supporting context: one entry per evidence pin, each proven the way the seal
bound it (the journaled `check.host` results must rebuild exactly the
manifest the candidate receipt's checks digest covers), plus whether the
assembly candidate's tree is the tree of one of those verified candidates. A
relaunched run adopts slice passes by ancestry, not another run's journal,
so its per-slice entries read `"missing"` while its `assembly` section is
proven from the checks it ran itself. Nothing in the roll-up is ever
projected as a pass without a journaled record, and the dispatch still
prepares, so the Verifier sees exactly what lacks proof instead of no
projection at all.

## Submission-refusal repair input

When a submitted handoff is refused for a field-level reason (an
implementer's malformed or incomplete `sworn_submit` call), that refusal is
durably reserved before it is ever returned to the worker, alongside the
dispatch try that raised it. If correction exhausts its budget or the
process stops before a valid handoff, the next same-authority continuation
receives that exact refusal and matching checkpoint provenance as
`submission_repair` in its work context - again **unverified repair
input**, never a candidate receipt or a verifier PASS - so it can complete
the handoff on the retained code instead of an empty commit or a blind
regeneration. A refusal already corrected by an accepted submission in the
same or a later try is not replayed as outstanding.

## Seal-time gates and their repair input

Before any declared check ever runs, Sworn evaluates two deterministic gates
against the exact sealed candidate, and orders the declared checks so the
cheap facts are cheap. Neither gate calls a model, and neither judges whether
a touched file proves anything - that stays the Verifier's job.

**Quick-before-long execution order.** Among a slice's declared checks, the
five that are not a `go test` invocation (vet, the asserted formatting check,
module tidiness, the diff check, the Darwin build) run first, in their
declared relative order, before any of the three long process suites (the
product suite, the serial end-to-end suite, the race suite). A failing quick
check blocks the seal before a single long-suite `check.host` effect is ever
journaled for that candidate; each check keeps its own effect identity, so
reordering never creates a duplicate.

**Anchor presence.** For each acceptance criterion whose text carries a
trailing `Anchor: path[, path][ and path].` clause, Sworn extracts the named
paths that exist in the slice's own prepared base - the parent of the
slice's earliest candidate under the current plan revision, never the
current round's lease head or refresh point, so a file covered in an earlier
attempt is never re-flagged as untouched - and refuses the seal with
`ANCHOR_NOT_TOUCHED` when a criterion's declared anchors, and no valid
declared substitute, appear in the candidate's diff from that base. The
refusal names the base it used, every criterion still missing an anchor and
its files, and separately, as a distinct fact, any declared substitute that
failed and why (untouched, or outside the slice's approved scope). It is
captured as repair context for the next same-authority implementer dispatch,
so the cost is one further dispatch, not a full evidence round. An
implementer may declare a substitute anchor for a criterion via
`anchor_substitutes` on its `implementer_implementation` submission
(criterion ID to path); a substitute the candidate honours is recorded on
the seal itself (criterion ID to file), so a reader does not have to
reconstruct it from the dispatch effect. An unreadable base tree or an
ambiguous diff refuses `ANCHOR_GATE_UNREADABLE` instead of silently passing.

**Degenerate submission body.** At the same author-side boundary that
already refuses a self-declared probe, Sworn also measures a submission's
`summary` and `detail` for repetition: a distinct-token ratio and a
compressed-size ratio, each independently bounded. A field refuses with
`SUBMISSION_DEGENERATE_BODY`, carrying both measured ratios, only when both
signals agree the body is degenerate - a single coincidental signal never
triggers alone. This is raised through the existing submit-boundary refusal
path, so it corrects in-turn without consuming a dispatch try.

For worktree-hosted operation, pass the intended `--operator-config` explicitly
to `sworn serve`. Default discovery searches the current checkout; a config
in another linked checkout is not automatically inherited. Check telemetry
health and actual collector receipt before assuming a run is observable.
