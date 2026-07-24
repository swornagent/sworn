# Sworn

Sworn is being rebuilt as the lean, vendor-agnostic engine for the Baton
protocol.

This branch is currently a maintenance bridge. The old v0.2 delivery kernel is
still present as archaeology, but it is not a supported delivery path:

- `sworn run` stops immediately with a maintenance message;
- the legacy SQLite `board` stops immediately;
- the hidden executor shim stops immediately;
- the old release workflow is disabled; and
- official builds ignore ordinary Git commit metadata.

This keeps Baton records under `.baton/releases` out of product builds while
the v0.3 engine replaces the old kernel.

## Available commands

```text
sworn version [--json]
sworn help
```

The old SQLite `board` is unavailable during the bridge. It will return as the
Baton release-and-track oracle rather than exposing the retired control store.

`sworn board` and `sworn run` will return when the v0.3 Baton loop is ready.
That loop is being built around five clear responsibilities: Planner,
Implementer, Captain, clean-context Verifier, and deterministic Merge.

## Development

Go 1.26.5 or newer is required. Use the VCS-free commands below so a record-only
commit cannot change the resulting binary:

```sh
GOFLAGS=-buildvcs=false go test ./...
GOFLAGS=-buildvcs=false go test -race ./...
GOFLAGS=-buildvcs=false go vet ./...
CGO_ENABLED=0 GOFLAGS=-buildvcs=false \
  go build -buildvcs=false -trimpath ./cmd/sworn
GOFLAGS=-buildvcs=false \
  go run -buildvcs=false -trimpath ./cmd/sworn version --json
```

The retained internal packages still have regression tests over isolated
fixtures. Those tests preserve useful archaeology; they do not make the old
delivery composition available through the shipped CLI.

## Rebuild record

The active scope and bootstrap reasoning are captured in:

- [Sworn Baton record-root bootstrap bridge](docs/captures/2026-07-24-sworn-baton-record-root-bootstrap.md)
- [Sworn RC2 asset scope](docs/captures/2026-07-24-sworn-s0-rc2-asset-scope.md)
- [Sworn v0.3 rebuild issue](https://github.com/swornagent/sworn/issues/157)

The v0.2 code remains recoverable from Git history and its release tag. It is
not the architecture base for v0.3.
