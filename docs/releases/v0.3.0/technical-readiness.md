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

### Paired turn-recovery evidence

Focused paired checks captured on 2026-07-30 passed:

| Scenario | Direct implementation | Human recovery | Signed recovery minus direct delta |
| --- | --- | --- | --- |
| Shared runtime fixed fixture | 16 ms; 14 input / 10 output tokens | 66 ms; 28 input / 20 output tokens | +50 ms; +14 input / +10 output tokens |
| Real binary, eval v2 | 6,419,606,905 elapsed ns; 336,441,597 implementation ns; 14 input / 10 output tokens; recovered 0, human escalation 0, false acceptance 0 | 8,512,129,544 elapsed ns; 1,734,950,593 implementation ns; 28 input / 20 output tokens; recovered 1, human escalation 1, false acceptance 0 | +2,092,522,639 elapsed ns; +1,398,508,996 implementation ns; +14 input / +10 output tokens |

The real-binary pair used the same plan bytes and produced the same exact
product tree and outcome. Its focused E2E test passed in 17.989s Go package
time (18.98s command wall time). The shared focused package passed in
0.009s (0.71s command wall time). The signed elapsed delta is one observed
fixture result, evidence that both paths are measured; it is not a performance
guarantee, and the test does not assert its sign.

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
`sha256:12ab8326666c5c942db23d125c21e29c2165dcbd59d4d849356cf2443c0a35af`.
Final-binary `driver inspect --all` returns seven PASS reports; its evidence is
`/tmp/sworn-responses-final.Kz6aNX/driver-inspect-all.json`, SHA-256
`06c7947457ad25253f9649e9b53e0ee0a1f107c381472861eb93372dc5eeba4e`.

The existing frozen live bundle remains visible: its aggregate run passes
Bedrock Mantle, Bedrock Runtime, Claude Code, Codex CLI, and Gemini, and the
targeted DeepSeek recheck passes. Their retained PASS bindings match the final
configuration:

| Profile | Model | Adapter configuration digest |
| --- | --- | --- |
| `bedrock-mantle` | `openai.gpt-oss-120b` | `sha256:47022869b0a241e929b641705386cfbad861dffd763149f3c81d1f907d10d594` |
| `bedrock-runtime` | `amazon.nova-pro-v1:0` | `sha256:c6321453e9e77e40da2aca602ed5147b9617fed249781d1c2767602f2f32311a` |
| `claude` | `claude-sonnet-4-6` | `sha256:d2b61a48df8e2cae86614baa11703bc307c3d0e5ad33ee5423a44208635b95a9` |
| `codex` | `gpt-5.6-sol` | `sha256:a32c4a0215d40b28e9e1596a1950a9e2bec12f1d58d7b9bd08479084a414c5aa` |
| `deepseek` | `deepseek-v4-flash` | `sha256:b9d658e8a418aa9ce63198cb71144dff6d572972b4c69022ee2e9f40d4471834` |
| `gemini` | `gemini-3.5-flash` | `sha256:1dc33732b1ec5c8fc020cc46e1bb82998121f96c9aa615bbca40feb9ee4a0eab` |

The shared Chat source was structurally touched to accept optional native
effort, but with empty effort its DeepSeek and Mantle wire output is
byte-equivalent; the final shared corpus re-proves those paths. Native OpenAI
alone changed surface to Responses and was rerun. The final targeted result
passes `gpt-5.6-sol` with family
`openai_compatible_http`, surface `openai_responses`, adapter
`sworn.openai` `1.0.0`, and adapter configuration digest
`sha256:299d4f4d7981fdacad0fdeb11a3b5d458afc513d911630cbe07707d5f78e132a`.
Its evidence is
`/tmp/sworn-responses-final.Kz6aNX/driver-certify-openai-sourced.json`,
SHA-256
`c471080180ff9047ed2f7ecfccdd0bf89aedd9f4fef623b966e2158bf38172b6`.

Together, the identity-bound bundle covers every required profile and both
Bedrock surfaces without substituting one provider for another. Replaying the
eight preserved product commits after the attempt-3 Captain receipt produced
tree `41c79c4198da60da20c61aa38bad075f3c5b6349`, exactly matching archived
head `d4ea0f536fe8c30703946ef0a11320800b0c447c`; the only deliberate delta is
the current readiness wording in this file and the certification-gap capture.

Revision 10 installs the lean evidence rule while preserving W0 through W6 and
the existing W8 identity. The repository owner approved and installed its
exact plan bytes. Technical facts and Baton authority are ready for fresh
verification; this still grants no tagging, `main` merge, deployment, or
publication authority.
