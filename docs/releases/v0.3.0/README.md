# Sworn v0.3 technical-readiness evidence

Sworn `1.0.0-rc.1` is the public release-candidate label for the v0.3 product
line. It embeds Baton `1.0.0-rc.8` and proves technical readiness only; it does
not authorize a tag, target merge, hosted deployment, or publication.

## Bound protocol and conformance input

- Baton tag: `v1.0.0-rc.8`
- tag object: `749714b60ac6356fbeb43d91ee3ad478820f2ad8`
- peeled commit: `a8fdb397e0839bdc58ad4b865e163dd37654752c`
- tree: `b39fe4c538a06ce7f28b70edd551395f99a8373c`
- release archive SHA-256:
  `bcbc310c2c5c98f82c721968ced7929ec58b0cdc2ab531a615fec706fe863582`
- generated support package SHA-256:
  `339799b218d4f8846cec1114a9756dda96a51744a72eb975bb9b632c4e349726`
- autonomous conformance manifest Git blob:
  `97b04caeda45d6ff334bd0c2168c1c333b270edb`
- autonomous conformance manifest SHA-256:
  `a53ae10a76dcca1f1e426f16385cb1487c9a1f690e2ab5ebb21463ec74cbea73`

`TestAutonomousEngineConformance` reads the manifest from the embedded package
at runtime, rejects missing, duplicate, extra, or unanchored case identities,
runs the real-binary walking-skeleton, dependency-base, and topology/recovery
scenarios, and emits a separate Sworn PASS result for each of the 12 cases. It
does not modify Baton's immutable `NOT RUN` source manifest.

## Driver and journey boundary

The runtime and readiness CLI share one canonical
`sworn.driver-config/v1` loader and host factory. Production manifests bind the
configuration digest and four explicit role/model selections; scripted
manifests and production configuration are mutually exclusive. The all-driver
gate requires Codex CLI, Claude Code CLI, OpenAI-compatible HTTP, DeepSeek,
Gemini, Bedrock Runtime Converse, and Bedrock Mantle to pass with their exact
configured models. There is no skip, fake, fallback, or substitution path.

The deterministic production journey uses a disposable repository, three
tracks, a dependency, and two serial slices on one track. It reopens the same
journal with byte-identical configuration, performs fresh read-only work and
assembly verification, and requires the final target tree to equal the passed
assembly tree. Credential-backed provider certification is an additional
explicit gate and is never replaced by deterministic local servers.

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
