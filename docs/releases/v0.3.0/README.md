# Sworn 1.0.0-rc.1 release-candidate overview

Sworn `1.0.0-rc.1` carries Baton handoffs through planning, design review,
implementation, fresh verification, and a checked Git merge. It includes Baton
`1.0.0-rc.12`.

Current status: ready for technical evaluation, but not yet approved for a tag,
merge to `main`, hosted deployment, or publication.

## What can be evaluated

- A complete run can be started, paused, resumed, cancelled, taken over, and
  recovered from one private journal.
- Independent tracks can proceed together while each track keeps a single,
  ordered writer.
- The terminal, JSON API, and local browser board show the same saved run
  facts.
- Codex CLI, Claude Code CLI, OpenAI Responses, OpenAI-compatible Chat
  Completions, DeepSeek, Gemini, Bedrock Runtime Converse, and Bedrock Mantle
  Chat are available as explicit connection choices.
- Connection configuration can be inspected, checked locally, or certified
  with an explicitly authorized live provider call.
- All 12 autonomous-engine cases supplied by Baton RC12 run against the built
  Sworn product.

The [run guide](../../run.md) explains the current operator journey and its
deliberate limits.

## What an operator still supplies

Sworn does not create the delivery plan, manifest, AI connection
configuration, credentials, or approval. Those inputs are provisioned outside
the product and bound to the run. Sworn does not choose a provider or model,
switch API types, or fall back to another connection.

## Bound Baton identity

- Baton tag: `v1.0.0-rc.12`
- tag object: `caac9f0ab32a596600874f911c7f2a5cd24b6552`
- peeled commit: `5bc374451d0e31d74948ea63010f87d017a3abd5`
- tree: `27297a37e7efd0154c487abfc5bae98fe711a8df`
- release archive SHA-256:
  `620e0f04ddcfa10067a8519d23b169d5e3fcc2751f28652990c889b72e0e4afb`
- published skills payload SHA-256:
  `f2db06b64a31403e7a864816a3b278a48578a5788eed3235d2be95cfbf093ef2`
- autonomous conformance manifest Git blob:
  `80a9666f0fb214bc0f11f4bb36db5e1ef40c6522`
- autonomous conformance manifest SHA-256:
  `8c3b7247a782a55a08c2ca09226123e4b96b8e80f4c2649a950a0df699988018`

The existing `support_package_sha256` JSON field keeps its stable wire name and
reports the published skills payload digest. Sworn does not embed, install, or
recompute the six skill files.

`TestAutonomousEngineConformance` reads the manifest from the included Baton
package, rejects missing, duplicate, extra, or unanchored case identities, and
runs the built-product planning, dependency, topology, and recovery journeys.
It records a separate Sworn PASS result for each of the 12 cases and does not
change Baton's immutable `NOT RUN` source manifest.

## AI connection and run evidence

The runtime and readiness command use the same canonical
`sworn.driver-config/v1` loader. Production manifests bind its digest and the
explicit profile/model choice for each role; scripted manifests and production
configuration cannot be mixed.

Native OpenAI uses Responses. Chat Completions is a separate compatibility
choice for providers that speak that API. The complete connection gate requires
Codex CLI, Claude Code CLI, OpenAI-compatible HTTP, DeepSeek, Gemini, Bedrock
Runtime Converse, and Bedrock Mantle to pass with their configured models.
There is no skip, fake, fallback, or substitution path.

The deterministic product journey uses a disposable repository, three tracks,
one dependency, and two ordered slices on one track. It reopens the same
journal with byte-identical configuration, performs fresh work and assembly
verification, and requires the final target tree to equal the assembly tree
that passed. Credential-backed live certification is a separate explicit gate;
local deterministic servers do not replace it.

An Implementer conversation may continue across an independent Captain review
inside one process. A changed authority record or process restart discards that
conversation and starts fresh. The first work Verifier starts fresh, read-only,
and independent of the delivery roles. After that Verifier records an exact
FAIL, Sworn may keep its thread for the direct repair. Each repaired candidate
gets a new read-only invocation and a full contract check. Missing or stale
context falls back to a fresh Verifier. If an exact head changed after a
candidate receipt, the Implementer can recheck and record that head without an
empty commit; evidence still covers the whole change from the prior candidate.
A retained Verifier thread can cross that refresh only when Baton's accepted
candidate receipts form a valid chain back to its recorded FAIL. Assembly
verification always starts fresh. Planner and Captain remain separate
invocations.

Recovery uses an explicitly selected automation model with limited actions and
budgets. It cannot create Baton approval or handoff records. When safe recovery
needs human judgment, Sworn saves a question and pauses only the affected part
of the work.

## Stable technical contracts

- Runtime manifests accept canonical v3 with an explicit recovery selection
  and legacy v2 without it.
- Browser-board JSON and HTTP routes are v2; there is no HTTP v1 alias.
- New evaluation records are `sworn.eval/v2`.
- The SQLite journal schema is v2.
- Webhook events remain `sworn.webhook-event/v1`.

These schema names, JSON fields, error codes, state values, and identifiers
remain machine-facing contracts even where the product now presents a clearer
human explanation first.

## Release checks and measurements

Before the candidate is frozen, fresh product copies must pass the full Go test
suite, race detector, vet, formatting, module-tidy, and diff checks. Two
independent non-CGO stripped builds must also produce identical bytes and
SHA-256 hashes.

Exact candidate identity, binary hash, package and dependency counts, source
size, retry behavior, timings, model usage, and quality results are recorded
with the final candidate evidence and Baton verification receipt.

See [technical-readiness.md](technical-readiness.md) for the measured facts,
executable parity results, and current conservative readiness verdict.

## Deliberate limits

Sworn does not host credentials, select provider/model defaults, retry inside a
provider adapter, let telemetry control delivery, or turn a successful runtime
call into a Baton verdict. Older v0.3 identifiers and earlier release documents
remain historical development context; the public candidate identity is
`1.0.0-rc.1`.
