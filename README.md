# Sworn

Sworn is a small deterministic delivery engine. It carries software work from
a proposed plan through to a checked merge, and it is the sole active
authority for that journey: no separate installation, release, version match,
or certification of another product is required to build or run it.

Sworn's design absorbed [Baton](https://github.com/sawy3r/baton)'s trust
rules and recorded handoffs. Baton-authored history remains safely readable
as legacy provenance, and its published receipt schema stays stable, but it
is design heritage, not a product Sworn depends on at build or run time.

Sworn gives each part of the job a clear owner:

- the Planner proposes the work and its order;
- the Implementer designs and writes the change;
- the Captain reviews the design before implementation continues;
- a fresh Verifier checks the finished candidate; and
- Sworn performs the Git merge only after its own recorded handoffs show a
  passing result.

```text
Planner
   |
   v
Implementer --> Captain
   ^              |
   +---- revise --+
   |
   +---- proceed --> Implementer --> fresh Verifier --> checked Merge
```

AI models do the role work. Sworn controls what is allowed to run, what happens
next, how failed work is retried, and when Git may change. It saves commands and
external work before execution, so a stopped process can recover from recorded
facts instead of guessing. A successful model response is never treated as an
approval or a passing verdict.

### Sworn's orchestrator

Sworn's orchestrator keeps the relay moving when a worker asks a question,
reports a block, or returns something Sworn cannot safely use. It can resume
the same worker with an answer grounded in saved facts, ask the Captain for
advice, retry an operational failure, or pause only that track for a human
answer while independent work continues.

The orchestrator is not a sixth role. It cannot approve a plan, invent a
Captain decision or Verifier verdict, or merge code. Only Sworn's own recorded
handoffs decide what advances.

## Current release candidate

Sworn `1.0.0-rc.2-dev` embeds its own Planner, Implementer, Captain, and
Verifier role assets; some of that prose and reference material still carries
legacy Baton `1.0.0-rc.14` content verbatim. Check the installed version and
embedded role-asset identity with `sworn version`, or use `sworn version
--json` when another tool needs to read them.

This candidate can:

- run the Planner, Implementer, Captain, Verifier, and merge journey;
- work on independent tracks in parallel while keeping changes on each track
  in order;
- pause, resume, cancel, take over, and retry a saved run safely;
- find the project's local delivery releases and saved Sworn runs in one
  terminal view;
- show the same saved facts in the terminal, JSON, and a local browser board;
- use Codex CLI, Claude Code CLI, OpenAI Responses, OpenAI-compatible Chat
  Completions, DeepSeek, Gemini, and two Amazon Bedrock connection types; and
- check connection configuration locally or perform an explicit live
  certification.

This is a public release candidate for evaluation. Its command and file
contracts may still change before `1.0.0`.

## Install

Sworn currently ships for Linux x86_64:

```sh
brew install --cask swornagent/tap/sworn
```

The GitHub release also provides a verified archive for direct installation.
See the [installation guide](docs/install.md) for checksums, upgrades, and
runtime requirements.

## Start here

The [run guide](docs/run.md) covers the files an operator supplies, AI
connection checks, starting a run, viewing progress, recovery states, and the
local browser board.

From anywhere inside a Git project, run:

```sh
sworn
```

In an interactive terminal, this opens the project view. It finds local
delivery releases and saved Sworn runs, lets you move between their boards,
and offers only the controls allowed by the current board. `sworn tui` opens
the same
view explicitly. When input or output is piped or redirected, bare `sworn`
prints help instead. The full command line remains available for scripts and
exact run control.

| Command | What it does |
| --- | --- |
| `sworn` or `sworn tui` | Open the interactive project view. |
| `sworn run` | Start or continue the run described by a manifest. |
| `sworn board` | Show what Sworn is doing, what is next, and whether a person is needed. |
| `sworn serve` | Open the same run board in a local browser service. |
| `sworn pause`, `resume`, `cancel`, `takeover` | Safely control a saved run. |
| `sworn retry` | Retry one stopped work item with its current safety values. |
| `sworn answer` | Answer a question that has paused one part of the work. |
| `sworn status --json` | Return the stable run record for another program. |
| `sworn driver inspect`, `doctor`, `certify` | Check configured AI connections at increasing depth. |

Exact command syntax:

```text
sworn tui [--project ABS] [--journal ABS] [--config ABS] [--manifest-dir ABS]
sworn version [--json]
sworn run --manifest ABS --journal ABS [--config ABS]
sworn pause|cancel --run ID --journal ABS --command ID --generation N
sworn resume|takeover --run ID --journal ABS --command ID --generation N [--config ABS]
sworn retry --run ID --journal ABS --command ID --generation N --work SHA256 --epoch N [--config ABS]
sworn answer --run ID --journal ABS --attention SHA256 --generation 1 --answer TEXT [--config ABS]
sworn status --run ID --journal ABS --json
sworn board --run ID --journal ABS [--json]
sworn serve --run ID --journal ABS [--manifest ABS] [--config ABS] [--operator-config ABS]
sworn driver inspect|doctor|certify --config ABS (--profile PROFILE --model MODEL | --all) --json
```

## AI connections

A production run names the exact AI profile and model for every role. It also
binds a secret-free `sworn.driver-config/v1` file. The matching file must be
supplied when the run starts and whenever a restart command needs to drive more
work.

The configuration names connection types, endpoints, model names, and
references to credentials. Secret values stay in environment variables,
owner-only files, or the AWS credential chain. Sworn does not copy them into
the manifest, journal, diagnostics, evidence, or telemetry.

Sworn never chooses a provider or model, changes API dialect, or falls back to a
different connection. OpenAI Responses and OpenAI-compatible Chat Completions
are separate, explicit choices.

## Build and verify

Go 1.26.5 or newer is required. Production model execution currently requires
Linux and root-owned `/usr/bin/bwrap` with unprivileged user namespaces
enabled.

The local code checks are:

```sh
GOFLAGS=-buildvcs=false go test -count=1 \
  ./cmd/sworn ./internal/... ./tools/...
GOFLAGS=-buildvcs=false go test -count=1 \
  -parallel=1 -timeout=20m ./test/e2e
GOFLAGS=-buildvcs=false go test -count=1 -race \
  ./cmd/sworn ./internal/... ./tools/...
GOFLAGS=-buildvcs=false go vet ./...
test -z "$(git ls-files -z -- '*.go' \
  ':(exclude,top).baton/releases/**' | xargs -0 -r gofmt -l)"
go mod tidy -diff
git diff --check
```

The release process also builds the candidate twice and runs the separately
authorized live connection certification. See the
[current release-candidate overview](docs/releases/v0.3.0/README.md) for the
exact evidence and known limits.

## Technical source map

```text
cmd/sworn         command line, project TUI, and local browser service
internal/baton    included Baton protocol and deterministic decisions
internal/runtime  work order, recovery, and the single state owner
internal/journal  saved commands, external work, receipts, and events
internal/gitx     measured Git facts and controlled Git changes
internal/driver   AI connection selection, tools, and credentials
internal/cockpit  terminal, JSON, and browser views of the same run
internal/tui      interactive navigation across project releases and runs
internal/observe  local evaluation and optional telemetry
```

`.baton/releases` is the default records root: it contains delivery control
records, not product source. Every host location Sworn reads or writes is
configurable — project-scoped locations (records, journals, contracts root
and the commit-message prefix) come from the committed
`docs/sworn/sworn.json`, and machine/user locations (workspace factory root,
temp roots, credentials directory, artefact home) resolve from `SWORN_*`
environment variables with XDG-conformant defaults. See
[docs/sworn/config.md](docs/sworn/config.md) for the schema, the defaults,
and the refusal rules. Earlier releases and retired protocol lines remain
unchanged as historical evidence.
