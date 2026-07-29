# Sworn v0.3 technical-readiness measurements

These measurements describe the Sworn `1.0.0-rc.1` candidate prepared for the
`W8-parity-release` gate. They are evidence, not authority to tag, merge,
deploy, or publish. The final Baton candidate receipt binds the exact commit
and tree externally because a file cannot contain the identity of the Git tree
that contains itself.

## Measurement environment

- captured: 2026-07-29
- operating system: Linux 6.8.0-136-generic, amd64
- Go toolchain: go1.26.5 linux/amd64
- build mode: `CGO_ENABLED=0`, `-mod=readonly`, `-buildvcs=false`,
  `-trimpath`, `-ldflags='-s -w'`

## Candidate verification gates

| Gate | Exact result |
| --- | --- |
| `GOFLAGS=-buildvcs=false go test ./...` | PASS; real-binary E2E package 498.522s |
| `GOFLAGS=-buildvcs=false go test -race ./...` | PASS; real-binary E2E package 505.975s |
| `GOFLAGS=-buildvcs=false go vet ./...` | PASS |
| `go mod tidy -diff` | PASS; empty diff |
| `gofmt -l` over all tracked Go files | PASS; no paths |
| `git diff --check` | PASS |
| W8 path-scope audit | PASS; all 34 changed paths are within the approved scope |
| Two fresh product-copy stripped builds | PASS; byte-identical size and SHA-256 |

## Product size and dependency facts

The approved classifier counts physical lines in Go files below `cmd` and
`internal`, excluding `_test.go`, `testdata`, `fixtures`, and files carrying
the standard generated-code marker. Blank and comment lines count.

| Fact | Measured value |
| --- | ---: |
| Production Go files | 94 |
| Production Go lines | 46,495 |
| Legacy baseline at `bad1a6767994cacef2c354061d22db842cb6ca08` | 10,464 |
| Delta from legacy baseline | +36,031 |
| W8 design measurement | 42,555 |
| W8 implementation delta | +3,940 |
| Production packages below `cmd` and `internal` | 8 |
| Direct module requirements | 10 |
| Direct-dependency delta in W8 | 0 |
| Stripped Linux amd64 binary | 22,204,578 bytes |
| Stripped binary SHA-256 | `de293261198a0180d573d6e322214fc9c6f4f84e85f283ea349d5351b7a8051a` |

The ten direct requirements are:

- `go.opentelemetry.io/otel v1.44.0`
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.44.0`
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.44.0`
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.44.0`
- `go.opentelemetry.io/otel/sdk v1.44.0`
- `go.opentelemetry.io/otel/sdk/metric v1.44.0`
- `go.opentelemetry.io/otel/trace v1.44.0`
- `go.opentelemetry.io/proto/otlp v1.10.0`
- `google.golang.org/protobuf v1.36.11`
- `modernc.org/sqlite v1.54.0`

The implementation adds no package, provider SDK, scheduler, tool loop,
credential store, or module requirement. It composes the configured factory
over the existing registry, dispatcher, adapters, journal, runtime, Git/Baton
actions, and operator telemetry service. New production code is confined to
the existing `cmd/sworn`, `internal/driver`, and `internal/runtime` packages.

## Executable scenario facts

| Scenario | Exact result | Elapsed |
| --- | --- | ---: |
| RC9 autonomous-engine conformance | 12 of 12 manifest-derived cases PASS; zero missing, duplicate, extra, skipped, or `NOT RUN` results | 582.70s |
| Real configured production journey | Three tracks, one dependency, two serial T1 slices, two families, four explicit role models, 18 unique dispatches, 22 provider turns, exact assembly and target | 60.26s |
| Verifier FAIL and repair | Attempt 1 FAIL; distinct attempt 2 candidate; fresh read-only PASS; 20 unique dispatches, 25 provider turns; failed product excluded from assembly | 71.99s |
| Composition conflict | Three exact `MERGE_CONFLICT` attempts; S1/S2 remain PASS; S3 untouched; target unchanged; run truthfully parked | 15.09s |
| Telemetry non-interference | Disabled, HTTP 503, and sustained exporter backpressure produce byte-identical candidate, PASS, assembly, merge, target, command-state, and exit evidence | 44.26s |
| Shared production driver corpus | Seven targets times P01-P10 equals exactly 70 PASS records; mutation gate rejects missing, extra, duplicate, and non-PASS records | 9.09s |

The conformance gate reads the case identities from the embedded Baton RC9
manifest with SHA-256
`cb7681e1d52cabc0c220491636b40837c86f1658bd8583421294804ab3abf61c`.
It executes the real-binary walking-skeleton, consumed-base, and
topology/recovery journeys before deriving the 12 Sworn PASS records.

The composition fixture creates a genuine conflict on `shared.txt`; it does
not inject an error code. Exhaustion is one initial attempt plus two automatic
retries in the same persisted epoch. No S3 model work or assembly work begins
after the prerequisite base composition fails.

## Usage and quality facts

The deterministic production providers report token usage and no cost:

| Journey | Input tokens | Output tokens | Cost |
| --- | ---: | ---: | --- |
| Normal production journey | 154 | 110 | unavailable |
| FAIL-repair journey | 175 | 125 | unavailable |

Every usage total is reconstructed from the durable per-dispatch usage
receipts. No token or cost value is estimated. The deterministic fixture does
not report a quality score, so local quality remains explicitly unknown/null;
Sworn does not invent a score or turn delivery success into a quality verdict.

## Readiness verdict

The deterministic, parity, telemetry, and touched-package gates pass. The
credential-backed all-family certification gate is currently
**NOT CERTIFIED** because `SWORN_DRIVER_CONFIG` is not provisioned on the
measurement host and the native Claude credential reports logged out with
empty access and refresh tokens. The exact pinned Claude Code `2.1.208`
executable and its declared Linux runtime closure are present; their
availability does not turn an unauthenticated probe into a pass. The 70-record
shared corpus is not a substitute for that live gate.

Technical readiness therefore remains fail-closed until one exact configured
run of:

```sh
sworn driver certify --all --config "$SWORN_DRIVER_CONFIG" --json
```

returns PASS for Codex CLI, Claude Code CLI, OpenAI-compatible HTTP, DeepSeek,
Gemini, Bedrock Runtime Converse, and Bedrock Mantle with their explicit
configured models, or an approved plan revision names an exact deferral.
