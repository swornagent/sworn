# Sworn 1.0.0-rc.1 release-candidate overview

Sworn `1.0.0-rc.1` carries Baton handoffs through planning, design review,
implementation, fresh verification, and a checked Git merge. It includes Baton
`1.0.0-rc.11`.

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
- All 12 autonomous-engine cases supplied by Baton RC11 run against the built
  Sworn product.

The [run guide](../../run.md) explains the current operator journey and its
deliberate limits.

## What an operator still supplies

Sworn does not create the delivery plan, manifest, AI connection
configuration, credentials, or approval. Those inputs are provisioned outside
the product and bound to the run. Sworn does not choose a provider or model,
switch API types, or fall back to another connection.

## Bound Baton identity

- Baton tag: `v1.0.0-rc.11`
- tag object: `427eb665f7ab32ec1b86f4efe4ae76e0627be588`
- peeled commit: `5807eb8c88cd85bdbad9a7ac3343ae8e1a69a19d`
- tree: `5900a2d5ab311184cd2a9d9b048da72fff220aef`
- release archive SHA-256:
  `524a1e4a7ddfa579fec34ca02fc1bb9c630cd018f3575eebfeb7ae7c4febd550`
- published skills payload SHA-256:
  `f3125e25d85f13cbab5437cb52a61627be33775d4f46b5665d4976b94cba12cc`
- autonomous conformance manifest Git blob:
  `75906b2f6584880db498a9a717b2699877551170`
- autonomous conformance manifest SHA-256:
  `f375cc87083ad397689493ef5855f42bbaa34723a5dae515b1fcd846d00e72e9`

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
conversation and starts fresh. Planner, Captain, and every Verifier remain
separate invocations.

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
