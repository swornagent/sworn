# Sworn

Sworn runs autonomous software delivery with the Baton protocol.

It gives coding agents clear responsibilities, isolated worktrees, durable
evidence and exact gates. The model or vendor can change without changing the
delivery rules.

```text
Planner
   |
   v
Implementer --> Captain
   ^              |
   +---- revise --+
   |
   +---- proceed --> Implementer --> Verifier --> Merge
```

- The Planner proposes the work.
- The Implementer designs and builds it.
- The Captain checks the design before code changes.
- The Verifier reviews the finished candidate with clean context.
- Merge is a deterministic engine action, not a model decision.

## Current v0.3 checkpoint

The old v0.2 engine has been removed from the active source tree. Sworn now
contains one small greenfield foundation and an exact embedded copy of Baton
`v1.0.0-rc.2`.

The supported product commands at this checkpoint are:

```sh
sworn version [--json]
sworn help
```

It validates the compiled Baton package before reporting its exact tag, commit,
tree, release-archive digest and generated-support digest. `sworn run` and
`sworn board` return in later v0.3 work after their authority and recovery
paths are proven.

The previous engine remains available from tag `v0.2.0`, branch `legacy/v0`
and Git history.

## Source layout

```text
cmd/sworn         command-line entry point
internal/baton    embedded protocol and deterministic Baton authority
internal/runtime  command service, scheduling and recovery
internal/journal  durable commands, effects, receipts and events
internal/gitx     exact Git facts and mutations
internal/driver   common invocation and sealed-submission boundary
```

The four future seams intentionally contain no placeholder behavior yet.

## Development

Go 1.26.5 or newer is required.

```sh
GOFLAGS=-buildvcs=false \
  go test ./tools/batonassets/... ./tools/batongolden/... ./cmd/sworn/...
GOFLAGS=-buildvcs=false go test ./...
GOFLAGS=-buildvcs=false go test -race ./...
GOFLAGS=-buildvcs=false go vet ./...
CGO_ENABLED=0 GOFLAGS=-buildvcs=false \
  go build -buildvcs=false -trimpath ./cmd/sworn
```

`.baton/releases` holds delivery authority. It is deliberately excluded from
product copies, archives and binary identity.
