# Start and operate Sworn

This guide covers a current Sworn `1.0.0-rc.2` production run. Sworn owns the
handoffs and approvals, carries the work, saves progress, and stops when it
cannot continue safely.

## What you need

Sworn does not yet create a delivery plan, run manifest, or AI connection file.
The release or deployment process must provide:

- a repository with the required release records;
- an approved, canonical `sworn.runtime-manifest/v3` file;
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
  a paid HTTP model request.
- `certify` makes the separately authorized live check. It needs real
  credentials and may consume provider usage.

Run live certification only when that use is intended:

```sh
sworn driver certify \
  --config /absolute/path/drivers.json \
  --profile openai \
  --model YOUR_EXACT_MODEL \
  --json
```

Use `--all` instead of `--profile` and `--model` only with a release-wide
configuration that includes every supported production connection family and
both Bedrock surfaces.

Each JSON report has a `state` and a stable `code`:

- `PASS` means the selected check passed.
- `FAIL` means the check ran and found a problem.
- `NOT_CERTIFIED` means that exact model is not listed for certification in the
  selected profile.

Read the result in the context of the command: an `inspect` pass confirms
configuration, while only a `certify` pass confirms the live provider path.

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
listener, webhook delivery, or telemetry export.

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

Sworn's orchestrator handles a worker turn that ends with a question, reports a
block, or does not return a usable handoff. It can resume the same worker with
an answer grounded in saved facts, ask the Captain for advice, retry an
operational failure, or park only that track for a human answer. Independent
tracks can continue.

The orchestrator is not a sixth role. It cannot approve a plan, invent a
Captain decision or Verifier verdict, or merge code. When it parks a track, the
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

Linux production execution requires root-owned `/usr/bin/bwrap` and
unprivileged user namespaces. Live `driver certify` and production runs can
consume provider usage; the ordinary Go test suite does not make live provider
requests.
