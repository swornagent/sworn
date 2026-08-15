# Install Sworn

Sworn currently supports Linux x86_64. macOS, Windows, and Linux ARM64 are not
release targets yet.

## Homebrew

On Linux x86_64:

```sh
brew install --cask swornagent/tap/sworn
sworn version
```

To move to a newer release candidate, or from an RC to the stable release:

```sh
brew update
brew upgrade --cask sworn
```

## Download the release archive

Set the version you want, download its archive and checksum, then verify it:

```sh
version=1.0.0-rc.2
base="https://github.com/swornagent/sworn/releases/download/v${version}"
curl -fLO "${base}/sworn-v${version}-linux-amd64.tar.gz"
curl -fLO "${base}/checksums.txt"
sha256sum --check --ignore-missing checksums.txt
tar -xzf "sworn-v${version}-linux-amd64.tar.gz"
```

Install the binary somewhere on your `PATH`:

```sh
mkdir -p "$HOME/.local/bin"
install -m 0755 \
  "sworn-v${version}-linux-amd64/sworn" \
  "$HOME/.local/bin/sworn"
sworn version
```

Repeat those steps with a newer version to upgrade a direct installation.

## Runtime requirements

The release binary does not need Go. Running AI model work currently needs:

- Git;
- root-owned `bwrap` discoverable on PATH (for example `/usr/bin/bwrap`); and
- unprivileged user namespaces enabled.

These requirements provide the Linux execution boundary Sworn uses around
model-driven work.
