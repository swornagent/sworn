# SwornAgent

**The verification layer that makes coding agents accountable.** SwornAgent runs
an independent, fresh-context adversarial verifier against a change's
`spec → diff (→ proof)` and returns a fail-closed verdict — so unverified work
cannot reach merged state. Built on the open [Baton](https://github.com/sawy3r/baton)
protocol.

Brand: **SwornAgent**. CLI binary: **`sworn`**.

> Status: pre-1.0. The provider-neutral verifier, driver registry, release
> records, parallel track engine, TUI, MCP server, and deterministic gates are
> implemented. The architecture review in
> [#108](https://github.com/swornagent/sworn/issues/108) is hardening the
> assembled autonomous loop; do not yet treat cold-start-to-release completion
> or remote control as a production guarantee.

## What it is

- **Provider-neutral core** — a single Go binary with no mandatory companion
  service. You choose the model and bring your own key; SwornAgent owns the
  protocol (fresh context, artefact-only inputs, fail-closed verdict).
- **PASS / FAIL / BLOCKED**, exit `0` only on PASS — so a CI required-check
  blocks the merge by default.
- **Native release records and orchestration** — Baton specs, status, proof,
  track worktrees, retries, and fresh verification are represented directly by
  the binary rather than delegated to repository-local shell scripts.
- **Explicit drivers** — hosted API providers and optional subscription CLI
  drivers resolve through one prefix registry and fail before dispatch when a
  requested role is unavailable.

## Quick start

```sh
make build
./bin/sworn verify --spec spec.md --diff change.diff
```

`verify` emits a JSON verdict and sets the exit code from it
(`0`=PASS, `1`=FAIL, `2`=BLOCKED).

Inspect configured execution capabilities without dispatching a model:

```sh
./bin/sworn capabilities
./bin/sworn doctor
```

## Current command groups

- Verification and models: `verify`, `capabilities`, `models`, `bench`
- Delivery and release evidence: `loop`, `board`, `ship`, `journeys`, `top`
- Requirements and design gates: `lint`, `reqvalidate`, `reqverify`,
  `specquality`, `designfit`, `designaudit`, `llm-check`
- Setup and operations: `init`, `doctor`, `mcp`, `account`, `login`, `logout`,
  `telemetry`

Run `sworn --help` for the complete registered surface. `sworn run` remains a
deprecated alias for `sworn loop`.

## Autonomous operations status

The intended loop is plan → implement → fresh verify → retry/escalate → gated
release assembly. Current limitations are tracked publicly:

- task-mode handoff is broken and tracked in
  [#27](https://github.com/swornagent/sworn/issues/27);
- cooperative pause is not reachable across processes
  ([#68](https://github.com/swornagent/sworn/issues/68)); and
- the architecture review is consolidating terminal outcomes, control commands,
  durable notifications, and the planned mobile web board under
  [#108](https://github.com/swornagent/sworn/issues/108).

Until that remediation is verified, use the loop as pre-release software and
inspect its persisted release evidence before merging.

## Development

```sh
go test ./...
go vet ./...
test -z "$(gofmt -l .)"
```

New runtime dependencies require an ADR. See
[`docs/adr/0007-dep-policy-minimal-justified.md`](docs/adr/0007-dep-policy-minimal-justified.md).

## Licence

MIT. SwornAgent is the product; Baton is the open protocol it depends on.
