# Sworn v0.3 technical-readiness evidence

Sworn `1.0.0-rc.1` is the public release-candidate label for the v0.3 product
line. It embeds Baton `1.0.0-rc.11` and proves technical readiness only; it does
not authorize a tag, target merge, hosted deployment, or publication.

## Bound protocol and conformance input

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

The existing `support_package_sha256` version field retains its wire name and
reports that published payload digest. Sworn does not embed, install, or
recompute the six skill files.

`TestAutonomousEngineConformance` reads the manifest from the embedded package
at runtime, rejects missing, duplicate, extra, or unanchored case identities,
runs the real-binary walking-skeleton, dependency-base, and topology/recovery
scenarios, and emits a separate Sworn PASS result for each of the 12 cases. It
does not modify Baton's immutable `NOT RUN` source manifest.

## Driver and journey boundary

The runtime and readiness CLI share one canonical
`sworn.driver-config/v1` loader and host factory. Production manifests bind the
configuration digest and four explicit role/model selections; scripted
manifests and production configuration are mutually exclusive. Native OpenAI
uses Responses; Chat Completions remains the OpenAI-compatible surface for
providers that speak that dialect. The all-driver gate requires Codex CLI,
Claude Code CLI, OpenAI-compatible HTTP, DeepSeek, Gemini, Bedrock Runtime
Converse, and Bedrock Mantle to pass with their exact configured models. There
is no skip, fake, fallback, or substitution path.

The deterministic production journey uses a disposable repository, three
tracks, a dependency, and two serial slices on one track. It reopens the same
journal with byte-identical configuration, performs fresh read-only work and
assembly verification, and requires the final target tree to equal the passed
assembly tree. Credential-backed provider certification is an additional
explicit gate and is never replaced by deterministic local servers.

Implementer continuation is a process-local optimization across independent
Captain review. Exact authority drift or process restart discards it and
rehydrates fresh; Planner, Captain, and every Verifier remain separate
invocations. Turn recovery uses an explicit automation profile with bounded
actions and budgets. It cannot synthesize Baton authority, and unresolved
judgment becomes durable human attention while unrelated tracks may continue.

## Compatibility boundary

- Runtime manifests admit canonical v3 with explicit recovery automation and
  legacy v2 without automation.
- Cockpit snapshots and HTTP routes are v2; there is no HTTP v1 alias.
- New evaluation records are `sworn.eval/v2`.
- The SQLite journal schema is v2.
- Webhook events remain `sworn.webhook-event/v1`.

## Release checks and measurements

The candidate is frozen only after fresh product copies pass full tests, the
race detector, vet, formatting, module-tidy, diff checks, and two independent
non-CGO stripped builds with identical bytes and SHA-256. Exact candidate,
binary, package-count, direct-dependency, source-size, retry, timing, usage, and
quality facts are recorded with the final candidate evidence and Baton
verification receipt.

See [technical-readiness.md](technical-readiness.md) for the measured facts,
executable parity results, and current fail-closed readiness verdict.

## Deliberate limits

Sworn does not host credentials, choose provider/model defaults, retry inside a
provider adapter, treat telemetry as delivery authority, or infer a Baton
verdict from runtime success. Configuration and credentials are operator
provisioned. Older internal v0.3 identifiers and prior release documents remain
historical wire and development context; the public identity is
`1.0.0-rc.1`.
