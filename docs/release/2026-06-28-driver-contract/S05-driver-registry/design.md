# Design TL;DR — S05-driver-registry

## User outcome (from spec.json)

A model ID resolves through one explicit registry to a registered driver
with fail-fast role checking — "no driver for prefix X" and "driver X cannot
serve role Y" errors name what IS registered — and `openai/` now routes to
the Responses API (sworn#31), with the sworn-proxy path resolving through
the same visible registry.

## Approach

One explicit table replaces three overlapping resolution surfaces. Today,
"which code serves this model ID" is answered in three drifting places:
`model.NewClient`'s provider switch (`internal/model/provider.go`),
`model.FromEnv`'s keyless-then-proxy-then-direct routing
(`internal/model/config.go`), and the hand-maintained `capabilityRegistry`
(`internal/model/registry.go:13`). This slice makes a new driver registry the
single authority for LOOP dispatch resolution:

1. A `Registry` value holds an ordered, explicit table of the four
   compiled-in `driver.Driver`s (claude subprocess, codex subprocess,
   in-process oai, in-process responses), each with its prefix list, plus a
   deprecated-alias map. No `init()` self-registration, no smart fallback —
   the registration mechanism is the Type-2 decision already recorded in
   `status.json` (Brad, 2026-07-02).
2. `Resolve(modelID, role)` is the only resolution path the loop will use
   (S06 consumes it; this slice builds and proves it): parse the prefix,
   resolve aliases (with a deprecation warning), look up the driver, check
   `Roles().Has(role)` BEFORE returning — fail fast with errors that
   enumerate what is registered.
3. `Drivers()` enumerates every entry — name, prefixes, RoleSet,
   availability in this environment, and whether a prefix currently routes
   through the sworn proxy — without any model dispatch (probes are
   LookPath / key-presence / login-file checks only).
4. The sworn#31 prefix rename lands at the client-construction choke point
   (`model.NewClient`) so the registry table and the in-process drivers'
   internal re-resolution can never disagree (see D3).
5. `capabilityRegistry` + `CapabilityRegistry()` + `HasChat` are deleted
   (grep-verified: zero non-test consumers), and a new `sworn capabilities`
   verb renders the registry enumeration instead.

`FromEnv`/`NewClient` remain as the one-shot utility path's constructors
(gates/bench) — out of scope to remove, per spec.

## Key design decisions

**D1 — Placement: `internal/driver/registry/` subpackage, not a file in the
contract package (recorded divergence from AC-01's literal path).**
AC-01 names `internal/driver/registry.go`, but that file would be package
`driver`, and `DefaultRegistry(cfg)` must (a) accept `model.ProviderConfig`
and (b) construct `inprocess.NewOAIChat/NewOAIResponses` — package `driver`
cannot import `internal/model` (ADR-0012 invariant, enforced by
`TestNoWireImports` over every `*.go` in that directory) and cannot import
`internal/driver/inprocess` (import cycle: inprocess imports driver). The
literal AC path is unsatisfiable without breaking the release's own Type-1
contract decision. Resolution: the whole registry lives in a new subpackage
`internal/driver/registry/` (package `registry`: `registry.go`,
`registry_test.go`) — still under `internal/driver/`, still covered by the
spec's `go test ./internal/driver/...` command. This is byte-for-byte the
divergence class S04 already recorded and passed verification with (see
`internal/driver/inprocess/inprocess.go`'s placement note). **Flagged for
the Captain** — it is a path divergence from AC-01's text, not from its
intent.

**D2 — The explicit table (what routes where).**
`registry.Default(cfg model.ProviderConfig) *Registry` registers:

| Driver (Name) | Prefixes |
|---|---|
| `claude-subprocess` (`NewClaudeDriver()`) | `claude-cli/` |
| `codex-subprocess` (`NewCodexDriver()`) | `codex/` |
| `oai-responses-inprocess` (`inprocess.NewOAIResponses(cfg)`) | `openai/` (+ deprecated alias `openai-responses/`) |
| `oai-inprocess` (`inprocess.NewOAIChat(cfg)`) | `openai-completions/`, `deepseek/`, `groq/`, `mistral/`, `openrouter/`, `cloudflare/`, `github/`, `anthropic/` |

Rationale for the `oai-inprocess` prefix set: the loop today
(`internal/run.newAgentFromModel`) accepts any chat-capable provider, and
`model.NewClient` resolves deepseek/groq/mistral/openrouter/cloudflare/
github to `*OAI` and anthropic to `*Anthropic` — all of which implement
`Chat` (grep-verified: `*OAI`, `*OpenAIResponses`, `*Anthropic` are the only
`Chat` implementers). Registering only the `openai*` trio would silently
shrink loop reach when S06 rewires dispatch through this registry.
Verify-only providers (google, vertex, bedrock, azure, oci, ollama) are NOT
registered — they stay on the one-shot utility path; the unknown-prefix
error names what IS registered, so "why can't the loop use bedrock" is
answered by the error text, not a mid-run type-assert. **Type-2 default,
flagged for review** (the spec names the four drivers but does not enumerate
the non-openai prefixes).

**D3 — The sworn#31 rename lands in `model.NewClient`, not as registry-side
ID rewriting.**
The in-process drivers' `Dispatch` re-resolves `DispatchInput.ModelID` via
`model.NewClient` (S04 D1: both identities behave identically; prefix
routing is S05's decision). If the registry mapped `openai/` to the
Responses identity while `NewClient`'s `"openai"` case still returned the
chat/completions `*OAI`, the registry's routing would be a lie — dispatch
would contradict enumeration. Two ways out: rewrite model IDs inside the
registry before dispatch (hidden mutation — rejected: "explicit prefix, no
magic"), or change the prefix SEMANTICS at the construction choke point.
Chosen: `NewClient` cases become

- `"openai"` → `NewOpenAIResponses(...)` (was `*OAI` chat/completions),
- `"openai-completions"` → `*OAI` chat/completions (new case, the legacy
  wire format under its new explicit name),
- `"openai-responses"` → `NewOpenAIResponses(...)` + deprecation warning to
  stderr (alias kept for one release, sworn#31).

`FromEnv`'s proxy block (config.go:73, the `openai-responses` special case
that picks the Responses struct over `*OAI`) is updated to key on the NEW
semantics: `openai` and `openai-responses` → `OpenAIResponses`,
`openai-completions` → `*OAI`. Consequence (flagged): the rename applies to
the one-shot utility path (verify gates/bench) too, not just loop
resolution — I read that as sworn#31's intent ("openai/ now routes to the
Responses API", unqualified), and it is what keeps ONE authority for prefix
meaning. Existing tests/fixtures across packages that assume
`openai/` = chat/completions wire shape will surface in the full-suite run
and are updated in-slice (R-01 breadth; this is the "grind" in the
effort/complexity call).

**D4 — Resolve error shape (AC-02/AC-03).**
`Resolve(modelID string, role driver.Role) (driver.Driver, error)`:

- Malformed ID (no `/`): explicit error, same `provider/model` wording as
  `parseModelID`.
- Unknown prefix: `no driver for prefix "foo/" — registered prefixes:
  anthropic/, claude-cli/, codex/, deepseek/, ...` (sorted, complete,
  aliases included and marked). `TestResolveUnknownPrefix` asserts the
  error text contains the full registered list.
- Role not declared: `driver "codex-subprocess" cannot serve role "captain"
  — declared roles: implementer,verifier; drivers declaring "captain":
  (none)` — names the driver, the missing role, and which registered
  drivers DO declare it (using `RoleSet.String()`'s deterministic order).
  No fallback to another driver, ever.
- Deprecated alias: resolves to the canonical entry's driver AND emits a
  deprecation warning naming old → new. The warning writer is an injectable
  `Warnf` field on `Registry` (default `os.Stderr`) so tests capture it
  without stderr scraping.

**D5 — Enumeration + availability probing (AC-05), no dispatch by
construction.**
`Drivers()` returns `[]Info{Name, Prefixes, DeprecatedAliases, Roles,
Available bool, Detail string, ViaProxy []string}`. Probes are closures
supplied at `Default()` construction (the `Registry` type itself never
touches env/fs, so unit tests inject fakes):

- `claude-subprocess` / `codex-subprocess`: `exec.LookPath("claude")` /
  `exec.LookPath("codex")`.
- In-process identities: per-prefix API-key presence from the
  `ProviderConfig` (e.g. `OpenAIKey` for `openai/`+`openai-completions/`,
  `DeepSeekKey` for `deepseek/`, `AnthropicKey` for `anthropic/`), OR an
  active proxy login (`account.Load` + `account.IsLoggedIn` with
  `SWORN_DIRECT` unset — the same condition `FromEnv` routes on).
- Proxy visibility (sworn#69): when the login condition holds, every
  API-key prefix of the in-process identities is listed in `ViaProxy` —
  enumeration SHOWS the S06b routing instead of it being discovered at
  dispatch. Keyless subprocess prefixes (`claude-cli/`, `codex/`) never
  appear there.

None of these probes can dispatch: they are PATH lookups, struct-field
checks, and a credentials-file read.

**D6 — `sworn capabilities` verb + capabilityRegistry deletion (AC-01,
R-02).**
No `capabilities` verb exists today (the spec's "re-pointed" reads as
"the capability question is re-pointed"): new `cmd/sworn/capabilities.go`
renders `registry.Default(model.ProviderConfigFromEnv()).Drivers()` as a
table (driver, prefixes, roles, available, via-proxy). It registers its verb
via its own `init()` + `command.Register` — `cmd/sworn/commands.go` is NOT
in this track's touchpoints, and its header comment explicitly anticipates
per-file self-registration, so this stays inside the declared file set.
`cmd/sworn/main.go` usage text gains the `capabilities` synopsis + the new
prefix documentation (AC-04). `internal/model/registry.go` is deleted
outright — `CapabilityRegistry()` and `HasChat` have zero non-test
consumers (grep-verified; the loop's chat gate uses the
`CapabilityProvider` interface, untouched here). The touchpoint-listed
`internal/model/registry_test.go` does not exist on this branch; nothing to
delete there. `internal/model/capabilities_test.go` does not reference the
table (verified) and is untouched.

**D7 — Codex stub removal (AC-04, closes sworn#19's last remnant).**
`NewClient`'s `"codex"` case (provider.go:175-180, `ErrDriverNotImplemented`
stub) is deleted; `codex/` now falls to the default unknown-provider error
on the utility path, because codex is served by the subprocess DRIVER via
this registry, not by a `model.Verifier`. `FromEnv`'s keyless block drops
`codex` accordingly (comment updated: claude-cli remains the only keyless
`model.Verifier`). The `ErrDriverNotImplemented` sentinel itself stays (the
default case and `FromEnv`'s `errorsIs` check still use it).

**D8 — Docs breadth (R-01).**
AGENTS.md gains the prefix table (openai/ = Responses API;
openai-completions/ = legacy chat/completions; openai-responses/ =
deprecated alias, one release; claude-cli/, codex/ = subscription
subprocess drivers; + the loop-registered OAI-compat set). Repo-wide grep
for `openai/` and `openai-responses/` citations; every doc/help-text hit
whose semantics changed is updated in the same diff.

## Files touched

- `internal/driver/registry/registry.go` — NEW: `Registry`, `Default(cfg)`,
  `Resolve`, `Drivers`, probe closures (D1 divergence from the literal
  `internal/driver/registry.go`).
- `internal/driver/registry/registry_test.go` — NEW: tests per AC below.
- `internal/model/provider.go` — #31 rename cases; codex stub removal.
- `internal/model/config.go` — proxy-block Responses selection keyed on new
  semantics; keyless block drops codex.
- `internal/model/registry.go` — DELETED.
- `cmd/sworn/capabilities.go` — NEW verb, self-registering `init()`.
- `cmd/sworn/capabilities_test.go` — NEW (verb output through the command
  registry — the integration point that owns the affordance, Rule 1).
- `cmd/sworn/main.go` — usage synopsis + prefix help text.
- `AGENTS.md` — prefix documentation.
- Possible collateral: `*_test.go` fixtures in `internal/model` /
  `internal/verify` / `internal/bench` / `internal/run` that pin
  `openai/` to the chat/completions wire shape (D3 consequence; discovered
  by the full-suite run, fixed in-slice, enumerated in proof.json).

## Test plan → AC traceability

- **AC-01**: `TestDefaultRegistryTable` — `Default(cfg)` registers exactly
  the four driver names with the D2 prefix sets; `Resolve` returns the
  right `Driver` by name for each prefix. Deletion half: the tree no longer
  contains `capabilityRegistry` (proof-bundle grep) and
  `cmd/sworn/capabilities_test.go` proves `sworn capabilities` renders from
  enumeration.
- **AC-02**: `TestResolveUnknownPrefix` — error text contains the full
  sorted registered-prefix list and the unknown prefix.
- **AC-03**: `TestResolveRoleFailFast` — a role no registered driver
  declares (captain) errors naming driver + role + "(none)"; a fake-driver
  registry variant proves the "which drivers DO declare it" enumeration and
  that no alternative driver is returned (no fallback).
- **AC-04**: `TestResolvePrefixRename` — `openai/x` → `oai-responses-
  inprocess`, `openai-completions/x` → `oai-inprocess`, `openai-responses/x`
  → `oai-responses-inprocess` + captured deprecation warning. Model-side:
  `TestNewClientOpenAIIsResponses` (type-asserts `*OpenAIResponses`),
  `TestNewClientOpenAICompletions` (type-asserts `*OAI`),
  `TestNewClientCodexRemoved` (unknown-provider error, no "deferred" stub
  text). AGENTS.md/help: proof-bundle grep + `cmd/sworn` usage test if one
  exists.
- **AC-05**: `TestDriversEnumeration` — injected probes: binary-found /
  key-present / logged-in permutations flip `Available` without any HTTP
  server existing (nothing to dispatch TO proves no dispatch);
  `TestEnumerationShowsProxyRouting` — login active + `SWORN_DIRECT` unset
  → `ViaProxy` lists the API-key prefixes; `SWORN_DIRECT=1` → empty.
- **AC-06**: `go build ./...`; `go test ./internal/driver/...
  ./internal/model/... ./cmd/sworn/...`; plus full `go test -timeout 120s
  ./...` per the project's cross-package-fixture hazard.

## Design-level risks / pins for the reviewer

1. **D1 path divergence** — `internal/driver/registry/` subpackage vs
   AC-01's literal `internal/driver/registry.go`. Forced by ADR-0012 +
   `TestNoWireImports` + the driver↔inprocess import cycle; S04 precedent.
   Needs the Captain's explicit ack so the verifier reads it as recorded
   divergence, not a miss.
2. **D2 prefix breadth** — registering the full chat-capable OAI-compat set
   (+ `anthropic/`) under `oai-inprocess`, not just the `openai*` trio.
   Default chosen to preserve today's loop reach through the S06 rewire;
   trimming the table is a one-line-per-prefix change if the Captain wants
   the literal-minimal table.
3. **D3 utility-path spillover** — the #31 rename changes what
   `NewClient("openai/…")` constructs for gates/bench too. I believe this
   is #31's intent and the only non-contradictory reading; if the Captain
   wants the rename scoped to loop resolution only, the registry would have
   to rewrite model IDs (the rejected magic) — surface disagreement now,
   not after the fixture sweep.
4. **Deprecation-warning surface** — stderr via injectable `Warnf` at both
   `Resolve` (loop path) and `NewClient` (utility path). Cheap, testable;
   flagging only because AC-04 says "resolves with a deprecation warning"
   without naming the sink.
