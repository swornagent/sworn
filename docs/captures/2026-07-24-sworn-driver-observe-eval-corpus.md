# Sworn driver, observability, and evaluation corpus

Date: 2026-07-24
Track: P2-driver-eval
Revision base: `0f219da3b8dfe2207656ad2eb8a5f5bffbeb0bd5`

## Current authorities

- Sworn scope:
  `docs/captures/2026-07-24-sworn-v0.3-greenfield-scope.md`, read in this
  worktree at the revision base above.
- Baton process contract:
  `reference/driver/contract.md` at Baton worktree commit
  `893f6fe8b6a52cebc8e7ccecc745ed5d138f3184`.
- Baton engine adapter:
  `conformance/engine-adapter.md` at the same Baton commit.
- Executable fake-driver cases:
  `test/driver/fake-driver.test.mjs` at the same Baton commit.
- Local executable authority: `codex-cli 0.145.0`, from `codex --version` and
  the complete current output of `codex exec --help` on 2026-07-24.

The previously cited `docs/roadmap-drafts/driver-architecture.md` and
`docs/captured/driver.md` do not exist in this branch and are not authorities.
No credential or secret file or value was read. Historical v0 implementation
is not an implementation base for this greenfield line.

## Contract decision

Every driver is an executable implementing exactly:

```text
driver info
driver run < request.json > result.json
```

`info` emits one strict object containing only `contract_version`, `driver_id`,
and `driver_version`. `run` reads one strict
`baton.driver-request/v1` object from stdin and emits one strict
`baton.driver-result/v1` object to stdout. Exit zero means a valid result was
emitted, including a typed transport failure. Non-zero means stdout is empty.
Stderr diagnostics are bounded and contain no credentials or request contents.

The request fields are exactly:

- `schema_version`;
- `invocation_id`;
- `role`;
- `operation` with exact `id`, `version`, canonical SHA-256 `digest`, and raw
  `instructions`;
- `model`, an explicit non-empty string or deliberate `null`;
- `workspace` with absolute `path` and `read_only | read_write` access;
- `inputs`, an ordered list of unique `name`, repository-relative `path`, and
  raw-byte SHA-256 `digest`;
- `fresh_context`; and
- `limits` with positive `timeout_ms` and `output_bytes`.

The result fields are exactly `schema_version`, request `invocation_id`,
`driver_id`, `driver_version`, non-empty-or-null `observed_model`,
non-negative `duration_ms`, optional reported `usage`, bounded `text`, and one
`transport_status`:

```text
completed | transport_error | timeout | cancelled | runner_error
```

`completed` is transport-only. The engine separately validates the returned
text into the required Captain decision, Verifier verdict, proof, or Merge
handoff before any Baton transition. A transport failure cannot become a Baton
verdict or outcome. No extra result fields are admitted.

The only roles are `planner`, `implementer`, `captain`, `verifier`, and
`merge`, bound respectively to the canonical operations `baton-plan`,
`baton-implement`, `baton-design-review`, `baton-verify`, and `baton-merge`.
The engine explicitly selects a driver executable/configuration and a model
for each invocation. Driver selection is not an added request field. The same
driver implementation serves all five roles.

Drivers translate one invocation. They never schedule, choose defaults, retry,
fall back, rotate providers, or reinterpret roles. Sworn owns dispatch,
cancellation, timeout, retry policy, fresh process isolation, and workspace
access outside model instructions.

## Deterministic corpus

All cases run without network access or credentials. Every named driver
(`baton.fake`, Codex CLI, Claude Code CLI, OpenAI-compatible, DeepSeek profile,
Gemini, and Bedrock) must pass the same case IDs through its real process
adapter, using a fake CLI process or fake HTTP server behind that adapter.
Results are recorded per driver; one adapter's result cannot satisfy another.

| ID | Required observation |
|---|---|
| P01 | `info` has exactly the three contract fields and one JSON object. |
| P02 | All five roles use one executable; operation tuple, explicit model, workspace access, input order/digests, freshness, and limits survive translation exactly. |
| P03 | Duplicate names, trailing JSON, unknown/missing fields, relative workspace, duplicate inputs, stale/substituted operation bytes, and role/operation mismatch fail closed. |
| P04 | Each of the five transport profiles emits exactly one valid bound result with exit zero; optional usage is absent, not zero, when unreported. |
| P05 | `completed` contains no verdict, outcome, proof, Merge fact, or freshness claim; engine handoff validation is a separate assertion. |
| P06 | Invalid command/request, crash, missing result, extra stdout, and result-binding mismatch are protocol failures with empty result and bounded diagnostics. |
| P07 | Output is capped at `limits.output_bytes`; deadline and cancellation reach the child/HTTP request and stop further work. A killed process cannot fabricate a result. |
| P08 | No default, fallback, retry, provider rotation, role-derived model, or provider scheduling occurs. A deliberate null model remains null in the contract case. |
| P09 | A seeded implementation task produces the exact expected file digest and bounded final handoff, rather than only returning a completion envelope. |
| P10 | Verifier starts as a new process in a read-only workspace; attempted mutation and memory-based context features are unavailable; candidate/ref digests remain unchanged. |

The canonical Baton fake itself is a process fixture, not HTTP:
`driver info` and `driver run` use strict stdin/stdout JSON. Its deterministic
profiles are exactly the five transport statuses above. The corpus reuses
Baton's valid request/result fixtures and process crash, missing-result, and
stderr-noise boundaries.

For native Codex and Claude adapters, a controlled fake executable validates
argv/stdin, performs the seeded implementation attempt through the contained
workspace, emits bounded native-shaped output, blocks until cancelled when
asked, and attempts a forbidden write in P10. Native production CLIs retain
their own agentic tool loops; Sworn does not reproduce or interpret their tool
calls.

HTTP providers use one shared, small allowlisted workspace-tool loop. The fake
server drives a deterministic read, patch, allowlisted check, and final
response. Tests cap tool-call count, per-call and aggregate bytes, command
allowlist, repository-relative path containment, elapsed time, and final
output. Cancellation prevents the next tool call. Read-only access rejects
patches. Provider adapters only translate authentication, messages, tool
calls, usage, cancellation, and errors into the common loop/result.

DeepSeek is a named configuration profile over the OpenAI-compatible runner
interface. It has no special lifecycle, implicit header behavior, fallback, or
retry path. Gemini and Bedrock have translation/signing fixtures but return the
same process result shape. Synthetic configuration and fake signing inputs
exercise credential rejection without reading credentials.

## Versioned native CLI argv

The Codex adapter test fixture is named `codex-exec-argv/v0.145.0`. It compares
this ordered argv exactly after placeholder substitution for non-Verifier roles:

```text
[
  "codex", "exec",
  "--dangerously-bypass-approvals-and-sandbox",
  "--ephemeral",
  "-C", "${workspace}",
  "--json",
  "-o", "${engine_control_dir}/last-message",
  "--ignore-user-config",
  "--ignore-rules",
  "--model", "${model}",
  "-"
]
```

Verifier invocations append the supported ordered pair `--disable`, `memories` in
the ordered position before model selection:

```text
[
  "codex", "exec",
  "--dangerously-bypass-approvals-and-sandbox",
  "--ephemeral",
  "-C", "${workspace}",
  "--json",
  "-o", "${engine_control_dir}/last-message",
  "--ignore-user-config",
  "--ignore-rules",
  "--disable", "memories",
  "--model", "${model}",
  "-"
]
```

The Baton instructions are supplied on stdin. JSONL stdout and the
engine-owned, bounded last-message file are captured by the adapter; neither
is the driver's stdout result until parsed and validated. The fixture rejects
the unsupported `codex run`, `--accept-feedback`, `--yes`, `--workdir`, and
`--format` spellings, reordered/extra argv, `resume`, and a missing explicit
model. Verifier fixtures additionally reject omitted `--disable memories`; the
ordered presence of this pair is required for the Verifier role.

The bypass flag is permitted only inside Sworn's external containment. For
Verifier, the executor mounts/provides the candidate workspace read-only,
places the control output outside it, starts a new OS process without resume,
and proves no candidate or ref mutation and no memory capability. For verifier-only
clean context, `--ephemeral`, `--ignore-user-config`, and `--ignore-rules`
do not disable memories and are insufficient to satisfy the memory-unavailable
requirement.

Claude argv is admitted only after its installed version/help is captured and
an ordered executable fixture is added. This document does not guess it. The
current lack of a configured Claude account makes its live smoke `NOT RUN`;
the deterministic Claude adapter corpus remains mandatory.

## Live-smoke separation

Live smokes are a separate credential-gated suite. Gate requirements,
credential injection, endpoint restrictions, and account readiness come from
explicit driver configuration, not hard-coded or guessed environment-variable
names. Tests use an opaque fake gate resolver and never inspect a credential
file or value.

Each configured smoke proves only the actual external boundary: selected
driver/model, authentication handoff, one bounded invocation, cancellation
wiring, and parseable contract result. It does not inherit deterministic
corpus evidence or prove Baton approval, implementation quality, isolation,
recovery, or integration.

The live report uses the engine-conformance vocabulary `PASS | FAIL | NOT RUN`.
A missing gate, account, executable, or supported live boundary is `NOT RUN`,
never PASS or a deterministic-corpus failure. Release evidence lists the gate
state for every named driver independently.

## Local evaluation record

The local corpus record contains corpus/case version; run, candidate, and
invocation IDs; Baton and Sworn versions; role and operation; configured
driver/model and observed driver/model; workspace access/freshness; exact
transport status; process exit; duration; reported-usage presence and values;
output byte count/truncation; tool count/cap/cancellation facts; candidate
before/after digests; handoff kind/validation/digest; and live
`PASS | FAIL | NOT RUN` where applicable. Unknown usage stays unknown.

Measures include delivery and exact-integration rate, false green/red,
blocked/no-verdict rate, transport-status rate, protocol rejection, output
truncation, cancellation latency, tool-cap/path/read-only violations, handoff
validation, reported-usage coverage, elapsed/orchestration time, verifier
disagreement, repair effectiveness, and results by version/driver/model/case.
Local records are authoritative evaluation inputs, not OTel control truth.

## OTel allowlists and failure behavior

Telemetry is disabled by default, explicitly opt-in, asynchronous, bounded,
lossy, and backed by a no-op default. Queue overflow and exporter failure drop
telemetry and cannot change scheduling, retry, verdict, integration, records,
exit status, or delivery.

Span attributes may include service name/version; run, candidate, and
invocation IDs; role; operation ID; driver ID/version; configured/observed
model; transport status; attempt; duration; output bytes; and usage-known.
Run, candidate, and invocation IDs are never metric labels.

Metric labels are limited to fixed low-cardinality values: role, operation ID,
driver family, transport status, usage-known, and bounded outcome category.
Model, driver version, case ID, error text, attempt number, and all identities
are excluded from metric labels.

No prompt, completion, model output, source, diff, proof/evidence body,
repository or filesystem path, credential, request content, stdout/stderr
body, raw argv, or tool arguments/results are exported by default. Allowlist
tests inspect every emitted span/metric and fail on unknown fields or excessive
label series.

## Parallel-safe ownership and model allocation

- S2 owns `internal/driver/{contract,fake,codex,claude,process_test}*.go` and
  `internal/driver/testdata/process/**`; it publishes the shared process
  harness before S4 consumes it.
- S4 owns `internal/driver/{http_tool_loop,openai_compatible,deepseek_profile,gemini,bedrock}*.go`
  and `internal/driver/testdata/http/**`. It consumes but does not edit S2
  fixtures; contract changes return to S2.
- S5 exclusively owns `internal/observe` and `internal/observe/testdata`.
  It consumes immutable driver result records and does not edit S2/S4 files.
- This capture has one owner during implementation; parallel stages do not
  edit it concurrently. Integration order is S2 harness, S4 adapters, then S5
  projections, with each stage committed independently.

Spark owns mechanical strict-JSON matrices, golden fixtures, low-cardinality
metric checks, redaction checks, and deterministic happy paths. The strongest
available model owns native argv translation, HTTP tool-loop implementation,
Bedrock signing edges, cancellation/process-death races, Verifier containment,
and Captain/Verifier/proof/Merge handoff validation. Both allocations run the
same deterministic checks; model strength changes task assignment, not gates.

## Acceptance

The capture is satisfied only when every named driver passes P01-P10, each
configured live smoke reports its own truthful status, Codex argv matches the
0.145.0 fixture under role-aware expectations byte-for-byte, Verifier containment
and memory capability disablement are externally proven, and non-Verifier roles
retain the current fixture when not requiring memory disabling,
engine validation keeps transport separate from Baton handoffs, telemetry
allowlists/cardinality and failure isolation pass, and parallel owners have no
overlapping writes.
