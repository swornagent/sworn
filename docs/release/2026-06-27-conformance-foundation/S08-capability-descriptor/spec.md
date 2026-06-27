---
title: 'S08 — Capability descriptor + fail-fast at implementer-model resolution'
description: 'Add a Capabilities() method to all model drivers; check Chat capability at implementer-model resolution so a misconfigured driver fails fast at startup with a descriptive error, not mid-run.'
---

# Slice: `S08-capability-descriptor`

## User outcome

When a user configures an implementer model that uses a driver without Chat support (e.g. an Anthropic-verify-only driver, or a keyless CLI with no Chat path), `sworn run` fails at startup with a message "driver <name> does not support Chat — required for the implementer role" before dispatching any slices. No more silent mid-run failures after the implementer is dispatched.

## Entry point

`sworn run --release <name>` → `internal/run/run.go` `ResolveImplementerModel` (audit ref `internal/run/run.go:343-352`) — capability check happens here, before the scheduler starts.

## In scope

- New `model.Capability` type: `type Capability uint; const (CapVerify Capability = 1 << iota; CapChat)`
- New `model.CapabilityProvider` interface: `Capabilities() Capability`
- All existing drivers implement `CapabilityProvider`:
  - `OAI`: returns `CapVerify | CapChat`
  - `OpenAIResponses` (if it exists separately): returns `CapVerify | CapChat`
  - `Anthropic`: returns `CapVerify` (Chat added in S10)
  - `cliDriver`: returns `CapVerify` (Chat deferred)
  - `Unconfigured`: returns 0
  - All other drivers (Azure, Bedrock, Google, OCI, Env): return `CapVerify`
- `internal/run/run.go` `ResolveImplementerModel`: after resolving the driver, check `driver.Capabilities() & CapChat != 0`; if not, return a descriptive error before returning the resolved driver
- New `internal/model/registry.go`: a thin registry mapping driver name → Capabilities() result for discoverability; not required for S08 to pass verification (the interface method is the canonical check) but provides `sworn capabilities` human-readable output
- `config.go` update: `--implementer-model` validation calls `ResolveImplementerModel` and returns error if Chat not available

## Out of scope

- Adding Chat to Anthropic (S10)
- The self-registering factory refactor — this slice adds the interface; the factory rename (ErrDriverNotRegistered) is in S09
- CapStream and other capability bits

## Planned touchpoints

- `internal/model/client.go` (add Capability type + CapabilityProvider interface)
- `internal/model/registry.go` (new — thin name→capabilities map for `sworn capabilities` output)
- `internal/model/oai.go` (add Capabilities() method)
- `internal/model/anthropic.go` (add Capabilities() method)
- `internal/model/cli.go` (add Capabilities() method)
- `internal/model/azure.go`, `internal/model/bedrock.go`, `internal/model/google.go`, `internal/model/oci.go`, `internal/model/env.go` (add Capabilities() method — boilerplate)
- `internal/run/run.go` (add capability check in ResolveImplementerModel)
- `internal/config/config.go` (validation at flag parse time, optional; can be deferred to run time)

## Acceptance checks

- [ ] ALL driver types in `internal/model/` implement `CapabilityProvider` (verified by compile-time interface assertion or test)
- [ ] OAI driver `Capabilities()` returns a value with `CapChat` bit set; Anthropic driver `Capabilities()` returns a value WITHOUT `CapChat` bit set (before S10)
- [ ] WHEN `sworn run --implementer-model anthropic/claude-3-7-sonnet-20250219` is called and the Anthropic driver is selected, THE SYSTEM SHALL return an error "driver anthropic does not support Chat — required for implementer role" before any slice is dispatched
- [ ] WHEN `sworn run --implementer-model gpt-4o` is called and the OAI driver is selected, THE SYSTEM SHALL proceed without error (CapChat satisfied)
- [ ] `client_test.go` or new `capabilities_test.go`: table-driven test asserting each driver's Capabilities() return value

## Required tests

- **Unit**: `internal/model/capabilities_test.go` (new) — table test, one entry per driver, assert correct capability bits
- **Integration**: `internal/run/run_test.go` — add scenario: non-Chat driver selected for implementer role → ResolveImplementerModel returns error with correct message
- **Reachability artefact**: `go test ./internal/model/... ./internal/run/... -v -run TestCapabilit` exits 0

## Risks

- Adding Capabilities() to every driver is mechanical but touches many files; all are in T2's touchpoints so no cross-track collision

## Deferrals allowed?

No.
