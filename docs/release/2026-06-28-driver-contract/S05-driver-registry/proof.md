# Proof Bundle: `S05-driver-registry`

Rendered from `proof.json` (proof-v1). Generated from live repo state, 2026-07-10.

## Scope

One explicit driver registry (`internal/driver/registry`) becomes the single
resolution authority for loop dispatch — explicit prefix -> driver table,
fail-fast role checking with errors that name what IS registered,
dispatch-free enumeration with availability probing and proxy-routing
visibility — plus the sworn#31 prefix rename (`openai/` -> Responses API)
landed at the `model.NewClient` choke point, the codex
`ErrDriverNotImplemented` stub removed, and the hand-maintained
`capabilityRegistry` deleted with a new `sworn capabilities` verb rendering
registry enumeration.

## Files changed

```
AGENTS.md
cmd/sworn/capabilities.go
cmd/sworn/capabilities_test.go
cmd/sworn/main.go
docs/release/2026-06-28-driver-contract/S05-driver-registry/status.json
internal/driver/registry/registry.go
internal/driver/registry/registry_test.go
internal/model/cli_test.go
internal/model/config.go
internal/model/oai_test.go
internal/model/provider.go
internal/model/provider_test.go
internal/model/registry.go (deleted)
```

## Test results

| Command | Result |
|---|---|
| `go build ./...` | PASS (exit 0) |
| `go vet ./internal/driver/... ./internal/model/... ./cmd/sworn/...` | PASS (exit 0) |
| `go test -timeout 300s ./internal/driver/... ./internal/model/... ./cmd/sworn/...` (fresh cache) | PASS (exit 0) |
| `go test -timeout 300s ./...` (full suite, fresh cache — 45 packages ok, 0 FAIL) | PASS (exit 0) |

## Reachability artefact

`cli-run`: built the sworn binary from this branch and ran
`SWORN_DIRECT=1 sworn capabilities` (2026-07-10) — the verb resolves through
the command registry and renders the live registry enumeration: all four
drivers, roles, real availability probes (both CLI binaries found on PATH,
"login not probed"; in-process identities unavailable with no keys/login),
the deprecated-alias marking, and the sworn#31 prefix-semantics footer.
`cmd/sworn/capabilities_test.go:TestCapabilitiesRendersRegistry` re-runs the
same path through `command.Lookup` (the integration point `main.dispatch`
resolves from, Rule 1) on every test run.

**Proof-bundle gate:** `git diff 20dc2dc..HEAD | sworn verify --spec
spec.json --diff - --proof proof.json --verifier-model claude-cli/sonnet` ->
`{"verdict":"PASS"}` (2026-07-10).

## Delivered

See `proof.json` `delivered[]` — every AC (AC-01..AC-06) plus Coach ack pins
3/4 and flags (b)/(c)/(d) carry named test or file evidence.

## Not delivered

- Proxy-aware dispatch for the in-process drivers the registry resolves to —
  Coach-ruled S06 ownership (`S06-loop-dispatch-rewire` spec risk R-04,
  committed e2b5472 on release-wt). Acknowledged: Brad (Coach), 2026-07-10,
  captain-proceed.md disposition 1.

## Divergence from plan

Recorded in `proof.json` `divergence[]`: (i) registry subpackage vs AC-01's
literal file path (ADR-0012 / TestNoWireImports / import cycle; S04
precedent); (ii) `registry.Default` ≡ AC-01's `DefaultRegistry` (qualified
name); (iii) touchpoint `internal/model/registry_test.go` did not exist —
nothing deleted; (iv) `sworn capabilities` created, not re-pointed (no verb
existed; R-02's premise vacuous); (v) deprecation-warning duplication
accepted per flag (a); (vi) `sworn llm-check` could not dispatch in this
environment (no model configured) — manual AC-to-test cross-check performed;
(vii) no `sworn coverage` verb exists in this branch's binary — the manual
AC-to-test matrix stands in.
