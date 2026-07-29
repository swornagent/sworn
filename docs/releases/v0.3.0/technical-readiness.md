# Sworn v0.3 technical-readiness measurements

These measurements describe the Sworn `1.0.0-rc.1` candidate prepared for the
`W8-parity-release` gate. They are evidence, not authority to tag, merge,
deploy, or publish. The final Baton candidate receipt binds the exact commit
and tree externally because a file cannot contain the identity of the Git tree
that contains itself.

## Measurement environment

- captured: 2026-07-30
- operating system: Linux 6.8.0-136-generic, amd64
- Go toolchain: go1.26.5 linux/amd64
- build mode: `CGO_ENABLED=0`, `-mod=readonly`, `-buildvcs=false`,
  `-trimpath`, `-ldflags='-s -w'`

## Candidate verification gates

| Gate | Exact result |
| --- | --- |
| `GOFLAGS=-buildvcs=false go test ./...` | PASS; isolated real-binary E2E package 503.634s |
| `GOFLAGS=-buildvcs=false go test -race ./...` | PASS; isolated real-binary E2E package 515.625s; no race warning |
| `GOFLAGS=-buildvcs=false go vet ./...` | PASS |
| `go mod tidy -diff` | PASS; empty diff |
| `gofmt -l` over all tracked Go files | PASS; no paths |
| `git diff --check` | PASS |
| W8 implementation scope audit | PASS; the live repair is confined to the existing driver/CLI-test readiness scope plus its requested durable capture |
| RC9 revision audit | PASS; the 23-path admission commit binds the exact upstream identity/assets and their generated bindings, tests, and evidence |
| Two fresh product-copy stripped builds | PASS; byte-identical size and SHA-256; record-only history excluded |

## Product size and dependency facts

The approved classifier counts physical lines in Go files below `cmd` and
`internal`, excluding `_test.go`, `testdata`, `fixtures`, and files carrying
the standard generated-code marker. Blank and comment lines count.

| Fact | Measured value |
| --- | ---: |
| Production Go files | 95 |
| Production Go lines | 47,006 |
| Legacy baseline at `bad1a6767994cacef2c354061d22db842cb6ca08` | 10,464 |
| Delta from legacy baseline | +36,542 |
| W8 design measurement | 42,555 |
| W8 implementation delta | +4,451 |
| Production packages below `cmd` and `internal` | 8 |
| Direct module requirements | 10 |
| Direct-dependency delta in W8 | 0 |
| Stripped Linux amd64 binary | 22,237,346 bytes |
| Stripped binary SHA-256 | `7a72bb6bb25c15147bcd185f8dd28172470a1ba2a1813989ff5f6a39f77d4f28` |

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
credential store, or module requirement. The OpenAI Responses codec translates
one additional provider wire format behind the existing dispatcher and bounded
tool loop. New production code is confined to `internal/driver`.

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

The deterministic, parity, telemetry, full, race, vet, format, tidy, and
reproducible-build gates pass. The canonical secret-free seven-profile
configuration now has digest
`sha256:12ab8326666c5b9cdcd99ffc6ccba440db661d312f217804ac953902945c164e`.
Final-binary `driver inspect --all` returns seven PASS reports; its evidence is
`/tmp/sworn-responses-final.Kz6aNX/driver-inspect-all.json`, SHA-256
`06c7947457ad25253f9649e9b53e0ee0a1f107c381472861eb93372dc5eeba4e`.

The existing frozen live bundle remains visible: its aggregate run passes
Bedrock Mantle, Bedrock Runtime, Claude Code, Codex CLI, and Gemini, and the
targeted DeepSeek recheck passes. Those exact profile/model/configuration
surfaces and their wire behavior are unchanged; the final shared corpus also
passes them. Native OpenAI alone changed to Responses and was rerun. The final
targeted result passes `gpt-5.6-sol` with family
`openai_compatible_http`, surface `openai_responses`, adapter
`sworn.openai` `1.0.0`, and adapter configuration digest
`sha256:299d4f4d7981fdacad0fdeb11a3b5d458afc513d911630cbe07707d5f78e132a`.
Its evidence is
`/tmp/sworn-responses-final.Kz6aNX/driver-certify-openai-sourced.json`,
SHA-256
`c471080180ff9047ed2f7ecfccdd0bf89aedd9f4fef623b966e2158bf38172b6`.

Together, the identity-bound bundle covers every required profile and both
Bedrock surfaces without substituting one provider for another. Revision 10
proposes this lean evidence rule while preserving W0 through W6 and the
existing W8 identity. The technical facts are ready; Baton authority remains
fail-closed only until the repository owner approves and installs revision 10.
