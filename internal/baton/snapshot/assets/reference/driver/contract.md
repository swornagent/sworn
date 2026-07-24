# Baton process-driver contract

Version: `baton.driver/v1`

A Baton driver is one process adapter between an orchestration engine and one
runner class. It translates transport and authentication details only. The same
driver can serve every Baton role; the engine selects a driver and an explicit
model for each invocation.

## Commands

```text
driver info
driver run < request.json > result.json
```

`info` writes exactly one JSON object containing `contract_version`,
`driver_id`, and `driver_version`.

`run` reads exactly one strict JSON object from standard input and writes
exactly one strict result object to standard output. Diagnostics may be written
to standard error, but must be bounded and must not contain credentials or
request contents.

Exit `0` means a contract-valid result was emitted, including a typed transport
failure. Non-zero means the process could not honour the protocol, so standard
output must be empty.

## Request

`baton.driver-request/v1` has these exact fields:

- `invocation_id`: stable invocation identity;
- `role`: `planner | implementer | captain | verifier | merge`;
- `operation`: exact `id`, `version`, SHA-256 `digest`, and raw
  `instructions`; IDs are `baton-plan`, `baton-implement`,
  `baton-design-review`, `baton-verify`, or `baton-merge`, the shared version
  is `baton.operation/v1`, and the digest is over the exact UTF-8/LF operation
  document including its final newline;
- `model`: an explicit non-empty model string or deliberate `null`;
- `workspace`: absolute `path` plus `read_only | read_write` access;
- `inputs`: ordered, uniquely named records with canonical repository-relative
  `path` and raw-byte SHA-256 `digest`;
- `fresh_context`: the engine's explicit context-isolation requirement; and
- `limits`: positive `timeout_ms` and `output_bytes`.

The driver validates the complete operation tuple against the canonical
operation installed beside it. A caller-supplied digest that merely matches
caller-supplied replacement text is not a canonical operation.

Drivers do not choose a default model, retry, fall back, rotate providers, or
reinterpret roles. Workspace access, fresh context, cancellation, and timeout
remain engine dispatch obligations and must be enforced outside model text.

## Result

`baton.driver-result/v1` binds:

- the request `invocation_id`;
- exact `driver_id` and `driver_version`;
- `observed_model`, which is a non-empty string or `null`;
- non-negative `duration_ms`;
- optional non-negative integer token `usage`;
- bounded response `text`; and
- exactly one `transport_status`:
  `completed | transport_error | timeout | cancelled | runner_error`.

`completed` says only that the runner returned a final response. It is not a
Baton outcome, Captain decision, Verifier verdict, proof, or Merge fact. The
engine validates role output and advances durable Baton records separately.

There is no provider-specific lifecycle, event-file format, `complete` command,
result-interpreter model, cost policy, or retry policy in this contract.
