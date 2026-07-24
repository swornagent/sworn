# Sworn Driver + Observability + Eval Corpus

Date: 2026-07-24
Track: P2-driver-eval
Branch base: `release/v0.3.0` at `6ab7dc251ff4cac23cdbffa9cd1a828961efe61f`

## Sources and refs

### Scope and contract inputs
- `docs/captures/2026-07-24-sworn-v0.3-greenfield-scope.md`
- `docs/roadmap.md`
- `docs/roadmap-drafts/driver-architecture.md`
- `docs/captured/driver.md`
- `docs/run.md`
- `docs/contained-executor.md`
- `docs/measured-submission.md`

### Baton seam and role-driver shape
- `/home/brad/projects/baton/docs/captures/2026-07-22-baton-v1-rc2-rebuild-plan.md`
- `/home/brad/projects/baton/docs/captures/2026-07-24-baton-rc2-sworn-coach-parity-execution-charter.md`

### Prior driver artifacts (for reusable translation patterns)
- `internal/adapter/codex.go` (this tree)
- `internal/adapter/codex_test.go`
- `internal/adapter/codex_boundary_linux_test.go`

### Historical Baton references (adapter history)
- `/home/brad/projects/fired/baton-install-backup/baton/runtime-drivers.md`
- `/home/brad/projects/fired/baton-install-backup/bin/bats/test_dispatch_driver_contract.bats`
- `/home/brad/projects/fired/baton-install-backup/bin/bats/test_driver_codex.bats`
- `/home/brad/projects/fired/baton-install-backup/bin/bats/test_driver_claude_cli.bats`
- `/home/brad/projects/fired/baton-install-backup/bin/test-driver-live.sh`

### Open driver/credential design references
- `/home/brad/projects/fired/baton-install-backup/opencode/commands/drivers/codex.sh`
- `/home/brad/projects/fired/baton-install-backup/opencode/commands/drivers/oai-compat.sh`
- `/home/brad/projects/fired/baton-install-backup/opencode/skills/baton/runtime-drivers.md`
- `/home/brad/projects/sworn-internal/docs/decisions/2026-06-24-sworn-orchestration-surfaces-and-subscription-drivers.md`
- `/home/brad/projects/sworn-internal/docs/strategy/2026-06-30-aws-agentcore-intersection.md`
- `/home/brad/projects/sworn-internal/docs/strategy/2026-06-30-telemetry-eval-transport.md`

No credential or secret file was read.

## Required corpus goals

- Single shared, parallel-executable corpus for S2/S4/S5 without provider-specific orchestration.
- No model/provider defaulting or fallback in any role path.
- Explicit driver and model selection per role.
- Deterministic fake and live-smoke split with the same request/result contract.
- Transport outcome recorded separately from Baton outcome.
- Deterministic run behavior in Verifier: read-only command, bounded output, bounded memory, cancellable.
- OTel as opt-in and explicit allow-listing, privacy-safe by construction.
- Eval corpus usable by low-cost model for coverage and strongest model for critical translation edges.

## 1) Deterministic fake driver corpus (local-first)

### 1.1 Fake server contract
- `test/fake_drivers` owns a local HTTP endpoint that accepts a minimal Baton-compatible request envelope and returns deterministic JSON.
- Request correlation uses `run_id`, `candidate_id`, `role` and `trace_id` fields.
- Behavior is purely deterministic per `(transport_case, seed, input_hash)`.

### 1.2 Golden responses
For each role/driver combo, fixed outputs are required:
- `ok`: valid final result object.
- `no_result`: transport returns no choice tokens but valid envelope.
- `error`: transport returns structured fatal error object.
- `bad_stream`: stream/frame syntax violation and recovery expectations.
- `error_max_turns`: max-turns limit exceeded before completion.

### 1.3 Verifier assertions on fake
- Verifier command must be read-only (`-R`, `--readonly`, non-writing working directory, no mutation of candidate artifacts).
- Request object is canonicalized before sending: no provider-specific defaults inserted.
- Result parser accepts missing usage as `usage=unknown` (never coerces to zero).
- Missing/invalid usage is captured as `usage_status=unknown` with explicit test assertion.

## 2) CLI shape corpus (headless/yolo/ephemeral)

### 2.1 Codex CLI
- Headless mode required and asserted through CLI args, for example:
  - `codex run --accept-feedback --yes --sandbox read-only --workdir <dir> --format json --output <tmp>/result.json --`
- Codex must be run in deterministic mode (single-shot request input file, no interactive prompt dependencies).
- Response parser must verify `usage` if present and treat missing/empty usage as unknown.

### 2.2 Claude CLI shape
- The harness must invoke a Claude CLI-like executable through driver abstraction with explicit model + role args.
- Record a truthful, test-controlled gate: **no local Claude account available in this environment for live execution**.
- Fake path remains required for all non-live paths and must enforce read-only/no-persistence constraints.

### 2.3 OpenAI-compatible HTTP driver shape
- HTTP JSON payload shape is provider-normalized.
- URL, model, headers, timeout, retries are configuration-driven but never defaulted by role.
- Transport outcome includes `transport_status`, `http_status`, and `parse_status` independent of Baton result.

### 2.4 DeepSeek profile
- Separate model profile entry point with explicit provider model map and no fallback from base OpenAI-compatible profile.

### 2.5 Gemini driver shape
- Explicit provider profile with JSON schema for role/task framing.
- Non-streaming transport in fake/live default path unless streaming test explicitly enabled.

### 2.6 Bedrock signing/protocol boundary
- Bedrock tests are protocol-boundary tests only in this tree:
  - request signing input serialization,
  - canonical payload hash,
  - required headers,
  - reject-path for missing credentials/region/profile without network use.
- Live credential-gated smoke is deferred until credential environment exists (see provider questions below).

## 3) Common request/result matrix (role-neutral driver contract)

### 3.1 Roles and explicit selections
No default model/provider is allowed. Each role in each test case must provide all fields:
- `role`
- `driver`
- `model`
- `provider_profile`
- `max_tokens`
- `timeout_ms`
- `output_budget`

| Role | Request object minimum | Transport outcome assertions | Baton outcome assertions |
|---|---|---|---|
| planner | `{goal, constraints, context_refs}` + driver explicit selection | `transport_status` present; parse/HTTP statuses asserted separately | `status in {ok,no_result,error,error_max_turns,bad_stream}` |
| architect | same fields + `design_scope` | timeout/cancel semantics explicitly asserted | `status` and `result` deterministic vs fake goldens |
| implementer | same fields + `patch_id` | bounded output byte cap validated | parse to `ok` with bounded output + no side effects |
| verifier | same fields + `read_only=true` | process executes with read-only executor contract | no fs mutation and read-only error on write attempts |

### 3.2 Shared success/failure matrix

| Case | Input class | Expected Baton status | Transport status expectation | Notes |
|---|---|---|---|---|
| happy path | complete JSON + explicit role model | `ok` | `transport_status=transport_ok` | parse/status mapped to Baton status only |
| empty completion | valid envelope, empty assistant content | `no_result` | transport parse OK | verifies semantic no-result path |
| usage missing | usage omitted | `ok` or `error` depending parser policy | `usage_status=unknown` | never convert to zero |
| max turns | long iterative fake stream | `error_max_turns` | transport ok; transport stop cause surfaced |
| stream corrupt | invalid chunk frame | `bad_stream` | transport parse failure recorded separately |
| timeout | budget exceeded | `error` (verifier-level) or provider-level transport reason | `transport_status=timeout` |
| cancel | explicit cancellation signal | `error` | transport status includes cancel reason |
| invalid role | undefined role | `error` | contract rejects before driver selection |

## 4) Provider-specific translation cases

| Provider profile | Mapping focus | Test fixture | Expected output fields | Expected rejects |
|---|---|---|---|---|
| deterministic-fake | schema-only pass-through | local fixture file | `content`, optional `usage`, `finish_reason` | malformed payloads and bad streams |
| codex-cli | CLI argv + JSON parser shape | fixed fixture | `run.exit_code`, `stdout_payload`, `stderr` | interactive mode flags or missing required args |
| claude-cli | role-aware wrapper | fake wrapper fixture | `status`, `message`, `usage` optional | missing local account context |
| oai-compatible | endpoint + auth + model body | fake HTTP server | standardized result object | provider error objects preserved as transport errors |
| deepseek-compat | same as oai-compatible + profile header | profile fixture | compatible normalized result | unknown profile id rejected |
| gemini | provider key profile + framing | fake fixture | `role`, `model`, `completion` | unsupported response shape |
| bedrock-boundary | signer and request envelope | signer fixture | signed_request metadata | missing region/profile/retries contract |

## 5) Fake-server / live-smoke separation

### Fake-server suite (always runnable)
- Deterministic matrix above.
- Uses local stub binary/server only.
- No external network, no secrets, no credentials.
- Verifier read-only semantics and output bounds enforced.

### Live-smoke suite (credential-gated)
- Skipped unless `*_CREDENTIAL` and `*_ENDPOINT` gates are set.
- One test per provider class: codex/claude/oai-compatible/bedrock/gemini/DeepSeek profile.
- Must assert exact CLI argv/model/driver for each role.
- Failure mode must be explicit when gates missing (not skipped silently with pass reason only).
- Bedrock live smoke remains boundary-first; no claim in corpus that transport has been validated without real credentials.

## 6) Versioned CLI argv assertions

Every role+driver test fixture must include exact argv assertions:
- `driver`
- `binary`
- `--accept-feedback` / equivalent acceptance flags
- timeout/retry values
- read-only enforcement flags
- output redirection path
- model argument
- format argument (`json`/provider equivalent)
- env allowlist keys

Examples are represented as versioned schema fixtures (v1, v1.1, v1.2) and must be compared in-order during tests.

## 7) OTel span/metric field allowlist and forbidden telemetry

### Allowed OTel fields
- `trace_id`, `span_id`
- `service.name`, `service.version`
- `baton.run.id`, `baton.role`, `baton.provider`, `baton.driver`, `baton.model`
- `transport.name`, `transport.phase`, `transport.status`
- `outcome.category` (`transport|baton`)
- `timing.ms`, `attempt`
- `limits.max_tokens`, `limits.timeout_ms`, `output.bytes`

### Explicitly forbidden fields
- raw credentials, token/secrets/API keys
- full user prompts or raw source code snippets
- file paths containing absolute local paths under workspace
- evaluator evidence blobs, stdout/stderr contents in full
- exact command lines with secrets substituted
- `candidate_diff` or other high-cardinality raw content

### Telemetry constraints
- OTel export defaults to disabled.
- When enabled, export endpoint and headers must be in env allowlist only.
- Local evaluation results are stored locally (read/write under eval store), never sent as structured truth.
- Commercial aggregation reads only anonymized summaries; no payload-level prompts/outputs/results.
- Cardinality caps: provider/driver/model/role at fixed enum cardinality, bounded `attempt` numeric bucketing only.

## 8) Eval schema and measures

### Core schema fields
- `run_id`, `candidate_id`, `role`, `driver`, `model`, `provider`
- `request_hash`/`request_size_bytes`
- `transport_status`, `baton_status`, `usage_status`
- `duration_ms`, `timeout_ms`, `cancelled`
- `max_output_bytes`, `captured_output_bytes`
- `read_only_violation` boolean
- `error_class`, `error_code`

### Measures
- pass rate by role/driver/model triplet
- transport failure rate split by status class
- parse rejection rate
- max-turn/error rate
- output truncation rate
- read-only violation rate
- usage_coverage (`known|unknown`) and unknown-vs-zero distinction
- median and p95 latency per provider profile

### Acceptance thresholds for shared corpus
- fake suite: 100% matrix conformance in parser+status separation.
- live-smoke: only requires successful invocation for credential-gated providers with clear gate metadata and exact-argv conformance.

## 9) Credential isolation and gating

1. No credentials loaded at test declaration time.
2. Credentials are read only from process env at execution time.
3. Gate checks happen before transport creation:
   - required env key exists,
   - command/endpoint host is allowlisted,
   - redaction policy in place.
4. If gate missing:
   - fake tests run unchanged,
   - live suites report `SKIPPED_CREDENTIAL_GATED` with explicit provider name and expected env keys.

## 10) Parallel-safe ownership plan for S2 / S4 / S5

- `docs/captures/2026-07-24-sworn-driver-observe-eval-corpus.md` is shared source of truth; no other file mutation required in this track.
- S2 owns fake fixture and test matrix authoring.
- S4 owns transport status/error translation assertions and protocol boundary cases.
- S5 owns OTel allow-list, eval schema/metrics, and telemetry gating.
- Any implementation PR must cite this capture and reuse identical test IDs to avoid duplication.

## 11) Test allocation by workload

1. Small deterministic matrix and protocol fixtures: run on Spark.
2. Cross-driver translation and cancellation/timeout behavior: run on strongest model.
3. Live smoke gating/error messaging and protocol boundary for Bedrock signing: run on strongest model with explicit credential note.
4. OTel cardinality and redaction checklists: run on Spark with targeted assertions.

## 12) Self-review gates (role-driver / managed inference drift)

- If any runner mutates `driver`/`model` defaults before execution: violation.
- If any mapping logic infers `model` from role: violation.
- If transport and Baton outcomes are conflated: violation.
- If usage missing is normalized to zero: violation.
- If Verifier can write outside read-only policy: violation.
- If telemetry transmits local prompt/result/evidence/credentials: violation.

