# ADR 0007: Dependency Policy — Minimal, Justified

## Status

Accepted (2026-06-19, supersedes ADR-0001).

## Context

ADR-0001 established a "zero runtime dependencies — stdlib only" rule. The
rationale was a single binary that runs anywhere with no external fetching.
That rationale still holds for the majority of the codebase.

S10-provider-foundation introduces a multi-provider model architecture.
Native provider drivers (Anthropic, Google Vertex AI, AWS Bedrock, Azure
OpenAI, Oracle OCI) each require their provider's official Go SDK for:

- Credential signing (AWS SigV4, OCI request signing, Azure token exchange)
- Event-stream parsing (Anthropic SSE, Google streaming)
- Error handling and retry signalling

Reimplementing these from scratch in stdlib is not minimal — it is a large,
error-prone maintenance surface that duplicates the SDK each provider ships,
tests, and security-audits. Using the official SDK is the minimal path.

This ADR replaces the absolute "zero runtime dependencies" rule with a
principle-based gate that preserves the single-binary property while
allowing driver-specific dependencies where the alternative is reimplementing
an SDK.

## Decision

**New rule:** Each new runtime dependency requires an ADR entry justifying
it. The project's default is still the standard library. Any `go get` that
adds a line to `go.mod` must be accompanied by:

1. An ADR (or an entry added to an existing ADR) that names the dependency,
   explains why stdlib cannot serve the purpose, and confirms the dependency
   is from a maintained, trusted source.
2. A `Co-Authored-By:` trailer in the commit that adds the dependency.

ADR-0001's safe-hosted default and single-binary principles remain in force.
This ADR relaxes only the "zero" constraint to "zero unless justified."

## Provider SDKs (pre-ratified)

The following SDKs are pre-ratified for their respective driver slices
(S11-S16). Each driver slice adds its own dependency with an ADR entry:

- `github.com/anthropics/anthropic-sdk-go` (S11)
- `google.golang.org/genai` (S12)
- `github.com/aws/aws-sdk-go-v2/service/bedrockruntime` (S13)
- `github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai` (S14)
- `github.com/oracle/oci-go-sdk/v65/generativeaiinference` (S15)

## Acknowledged trade-offs

### `.env` file in CWD

`LoadDotEnv()` loads `~/.sworn/.env` first, then `.env` in the current
working directory. If `sworn run` is invoked from an unrelated project
directory that contains a `.env` file, unexpected API keys may be injected
into the process environment.

This is an acknowledged risk of convention-based file loading — the same
convention used by Docker Compose, systemd EnvironmentFile, and every
twelve-factor-app helper library. Mitigations:

- `LoadDotEnv()` only sets env vars not already present in the process
  environment (explicit env vars always win).
- The `SWORN_*_API_KEY` prefix namespacing makes accidental collision with
  unrelated `.env` files unlikely.
- The loading path is documented in `sworn run --help` so users can inspect
  and override.

## Consequences

- Each new dependency requires an ADR entry (or an addition to this ADR),
  visible at `docs/adr/0007-*` or inline.
- The single-binary property is preserved (Go's static linking produces
  one binary even with SDK deps).
- Provider driver slices (S11-S16) can use their official SDKs, reducing
  the maintenance surface and security risk.
- Code reviewers check `go.mod` diffs against ADR entries.