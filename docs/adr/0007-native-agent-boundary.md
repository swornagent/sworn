# ADR 0007: Prove the native-agent boundary before composing a loop

- Date: 2026-07-21
- Status: accepted
- Superseded in part by: [ADR
  0009](0009-codex-cli-managed-chatgpt-authentication.md), credential transport
  and its API-key canary only

## Context

Sworn can recover one native writable effect, but the shipped binary still has
no agent adapter or mutating command. The next trust question is narrower than
provider orchestration: can a real networked agent CLI run model-directed tools
without giving those tools the model credential, host network, or files outside
the measured builder workspace?

The agent control process and its tools need different capabilities. A provider
SDK, workflow framework, or larger prompt would not create that split. We first
need one falsifiable native-CLI boundary.

## Decision

### Add only the executor capabilities the proof requires

An invocation may select one existing digest-pinned input as
`ExecutableInput`. The selected copy is staged `0500`; every other input remains
`0400`. Its argv must directly name `/inputs/<selected-name>`, so there is no
`PATH`, shell, or mutable-workspace lookup. The raw completion binds the
selector and the already observed input name, digest, and size. Inputs remain in
the private attempt root and never enter the workspace export.

Further user namespaces remain disabled by default. They are enabled only when
both `Invocation.NestedSandbox` and `Options.AllowNestedSandbox` are true. The
executor configuration digest binds the latter. This permits a trusted agent
control process to create its own tool sandbox without weakening ordinary
invocations or the capability probe.

The existing network rule is unchanged: host networking also requires both an
invocation request and executor admission. These are three explicit facts, not
an agent framework: exact executable input, nested-sandbox admission, and host
network admission.

### Prove one exact Codex CLI before building an adapter

The executable, nested-sandbox, and outer-versus-tool capability split in this
section remain accepted. ADR 0009 supersedes its credential transport: the
production profile uses a dedicated CLI-managed ChatGPT `auth.json`, not an
environment-supplied Platform API key or the generic credential language below.

The first target is one static-PIE Codex CLI supplied by clean absolute path and
copied through the executable-input boundary. The proof records its observed
digest and version. A future production profile must additionally bind those
facts, the literal argv, model, environment names, tool schema, and executor
configuration digest. Sworn will not discover an agent through `PATH`, silently
upgrade it, or choose a model default.

The accepted Codex invocation is noninteractive and explicit:

- `codex exec` with approval `never`, nested `workspace-write`, a named model,
  ephemeral operation, strict configuration, ignored user configuration and
  execution-policy rules, and JSONL events;
- an engine-owned empty `/tmp` as the primary project and `/workspace` only as
  an added writable root, so candidate `AGENTS.md` and `.codex` policy cannot
  configure the control plane;
- web search, tool networking, login shells, history, updates, hooks, apps,
  goals, memories, subagents, remote plugins, shell snapshots, and automatic
  skill dependency installation disabled; and
- a tool environment that inherits nothing and sets only fixed non-secret
  values.

The pinned Codex process is trusted control-plane code. It receives the model
credential and the executor's deliberately broad host network. Its
model-directed tool runs in Codex's nested sandbox, can edit `/workspace`, and
has neither network nor the credential. Sworn authority and integration
credentials are not supplied to either process.

There is no generic adapter registry, provider SDK, LangChain/LangGraph runtime,
or bundled model configuration. A second CLI or a materially changed Codex
version must prove its own exact profile before it is admitted.

### Keep current authority at capability-granting edges

Plan approval and reviewable admission are historical facts. Fresh current
authority is required immediately before agent execution, check or verifier
execution, accepting a verdict that grants integration capability, and
integration itself. Replay, convergence of an already bound result, and atomic
admission of exact completed evidence remain deterministic history and do not
gain a redundant permit check.

## Accepted feasibility evidence

This is retained as historical evidence for the original process split. Its
random bearer canary proved environment and process isolation, but ADR 0009's
file-backed ChatGPT proof is the release evidence for current production
authentication.

`TestRealCodexCLIBoundaryFeasibility` runs the real CLI inside the production
Linux executor against a local scripted Responses endpoint. It makes no
provider model call and consumes no model tokens. The accepted run used:

- version `codex-cli 0.145.0-alpha.18`; and
- SHA-256
  `16db86b6bf81cc426032fd42216dd97e60f97b149272f1f9963845a0675dae94`.

The host-observed proof establishes that:

1. the digest-staged CLI reaches the outer endpoint with the exact canary bearer
   credential and performs two bounded Responses turns over HTTP SSE;
2. the first turn advertises the pinned tool allowlist and dispatches a real
   `exec_command`; the second binds its exact call ID and successful output;
3. the nested process can write the measured workspace but cannot write `/usr`
   or `/inputs`, read a host-only canary, or reach the endpoint;
4. the nested environment and visible process command lines/environments do not
   contain the random credential canary;
5. hostile candidate project configuration, instructions, and rules never
   enter the model request; and
6. the source workspace and executable inputs remain unchanged, the measured
   export contains no executable residue, and export discard leaves both
   executor roots empty.

The ordinary Linux executor suite separately proves timeout, cancellation,
process-tree death, quiescence, output bounds, measurement, and restart cleanup
for the same execution path. The real-Codex test stays opt-in because it needs a
specific external binary:

```sh
SWORN_CODEX_BINARY=/absolute/path/to/codex \
  SWORN_REQUIRE_CODEX_BOUNDARY=1 \
  go test -run TestRealCodexCLIBoundaryFeasibility ./internal/adapter
```

## Budget gate

The merged base was 14,366 semantic and 15,900 physical production Go lines.
The completed tree is 14,432 semantic and 15,971 physical lines: a delta of
+66 / +71. The delta adds only the executable-input selector, the double-gated
nested-user-namespace exception, configuration binding, and fail-closed
completion checks. The larger feasibility harness is test-only. There is no
schema migration, production dependency, public command, or adapter runtime.

## Consequences and non-claims

The boundary is feasible; an autonomous loop is not yet shipped. This decision
does not prove model quality, independent verification, provider portability,
or a complete builder-to-reviewable vertical.

Allowing the trusted Codex process to create a child user namespace retains the
kernel's user-namespace attack surface and does not prevent deeper descendant
nesting. Descendants still cannot recover the outer mount, network, capability,
PID, or cgroup boundary. The outer Codex process has broad host network, not
endpoint-filtered egress, and its credential remains visible to the trusted
process and same-UID host observers. Host `/usr`, the kernel, systemd,
Bubblewrap, and the Sworn host account remain in the trusted computing base.

The exact Codex binary is currently copied and hashed per attempt. A verified
cache is justified only if measurement shows that cost matters. The next
production slice must enter through the built `sworn` binary and reach
`reviewable` through the real builder, current-authorized restart-recoverable
checks, and deterministic admission. It must not expose a builder-only command
or introduce a second state machine.
