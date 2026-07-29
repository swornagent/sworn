# Sworn

Sworn is a local autonomous-delivery engine for the Baton protocol. It runs
Planner, Implementer, Captain, and fresh Verifier turns through one bounded
driver contract, while Sworn—not a model—owns scheduling, recovery, exact Git
composition, and target integration.

```text
Planner
   |
   v
Implementer --> Captain
   ^              |
   +---- revise --+
   |
   +---- proceed --> Implementer --> fresh Verifier --> deterministic Merge
```

Sworn supports parallel dependency-ready tracks with one serial writer per
track. Commands and external effects are journaled before execution, so pause,
resume, takeover, bounded retry, and crash recovery converge on recorded
authority. A successful process or model response is never a Baton verdict.

## Release candidate

The public binary identifies as Sworn `1.0.0-rc.1`. It embeds and validates
Baton `1.0.0-rc.8`; `sworn version --json` reports both identities.

This candidate provides:

- the real `sworn run`, control, status, board, and local operator surfaces;
- deterministic scripted manifests for tests and recovery compatibility;
- production Codex CLI, Claude Code CLI, OpenAI-compatible, DeepSeek, Gemini,
  and Bedrock drivers behind the same role-neutral contract;
- both Bedrock Runtime Converse/SigV4 and Bedrock Mantle Chat Completions
  surfaces, with no provider or model fallback;
- secret-free driver inspect, doctor, and live-certification reports; and
- an executable gate derived from all 12 autonomous-engine cases in the
  embedded Baton RC8 conformance manifest.

Technical readiness does not grant authority to tag, merge to `main`, deploy,
or publish the binary.

## Commands

```text
sworn version [--json]
sworn run --manifest ABS --journal ABS [--config ABS]
sworn pause|cancel --run ID --journal ABS --command ID --generation N
sworn resume|takeover --run ID --journal ABS --command ID --generation N [--config ABS]
sworn retry --run ID --journal ABS --command ID --generation N --work SHA256 --epoch N [--config ABS]
sworn status --run ID --journal ABS --json
sworn board --run ID --journal ABS [--json]
sworn serve --run ID --journal ABS [--manifest ABS] [--config ABS] [--operator-config ABS]
sworn driver inspect|doctor|certify --config ABS (--profile PROFILE --model MODEL | --all) --json
```

Production manifests contain no scripted submissions. They bind one canonical,
secret-free `sworn.driver-config/v1` digest and four explicit profile/model
selections. The same canonical configuration must be supplied to `run` and to
every driving restart command. Supplying it to a scripted manifest, omitting it
from a production manifest, or changing its digest fails closed.

Driver configuration names exact adapters, endpoints, credential references,
and certification models. Credential values remain in the selected environment
or owner-only files and never enter configuration, manifests, journals,
diagnostics, evidence, or telemetry. `driver certify --all` exits nonzero for
any missing, failed, or not-certified production family or Bedrock surface.

## Build and verify

Go 1.26.5 or newer is required. The release gates are:

```sh
GOFLAGS=-buildvcs=false go test ./...
GOFLAGS=-buildvcs=false go test -race ./...
GOFLAGS=-buildvcs=false go vet ./...
test -z "$(git ls-files -z -- '*.go' \
  ':(exclude,top).baton/releases/**' | xargs -0 -r gofmt -l)"
go mod tidy -diff
CGO_ENABLED=0 GOFLAGS=-buildvcs=false go build \
  -mod=readonly -buildvcs=false -trimpath -ldflags='-s -w' \
  -o /tmp/sworn-v0.3.0 ./cmd/sworn
test -n "$SWORN_DRIVER_CONFIG"
/tmp/sworn-v0.3.0 driver certify --all \
  --config "$SWORN_DRIVER_CONFIG" --json
git diff --check
```

See [the v0.3 release evidence](docs/releases/v0.3.0/README.md) for the exact
scope, conformance identity, measurements, and known readiness state.

## Source layout

```text
cmd/sworn         command-line and local operator surfaces
internal/baton    embedded protocol and deterministic Baton authority
internal/runtime  the single scheduler, reducer, and recovery owner
internal/journal  durable commands, effects, receipts, and events
internal/gitx     exact Git facts and mutations
internal/driver   common selection, invocation, tools, and credentials
internal/cockpit  truthful terminal and browser projections
internal/observe  local evaluation and opt-in telemetry projection
```

`.baton/releases` is delivery authority, not product source. Product copies,
archives, and binary identity deliberately exclude it. Earlier releases and
abandoned protocol lines remain immutable Git archaeology.
