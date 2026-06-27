# SwornAgent distribution

Three install channels, all producing the same `sworn` binary.

## Homebrew (macOS)

```sh
brew install swornagent/tap/sworn
```

Updates on each release. The tap repository is at
[swornagent/homebrew-tap](https://github.com/swornagent/homebrew-tap).

## Go install (any platform with Go)

```sh
go install github.com/swornagent/sworn/cmd/sworn@latest
```

Pulls and builds from the default branch. Requires Go 1.26+.

## Container (any Docker host)

```sh
docker run --rm ghcr.io/swornagent/sworn version
docker run --rm -v "$PWD:/workspace" ghcr.io/swornagent/sworn verify \
  --spec /workspace/spec.md --diff /workspace/diff
```

Images are published to `ghcr.io/swornagent/sworn` on each release.
Both `:latest` and `:vX.Y.Z` tags are available. The container is a
`scratch` image containing only the `sworn` binary — no shell, no
package manager, no CA certs (Go bundles its own). Use volume mounts
to pass spec and diff files in.

## Prerequisites for maintainers

- The `swornagent/homebrew-tap` repository must exist and the
  `HOMEBREW_TAP_GITHUB_TOKEN` secret must be set on this repo for
  Homebrew formula publishing.
- GHCR is used via `secrets.GITHUB_TOKEN` (no separate registry
  credentials needed).