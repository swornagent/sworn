# SwornAgent distribution

Every channel produces the same `sworn` binary (linux/macOS/Windows, amd64/arm64),
built and published by GoReleaser on each `v*` tag (see `.goreleaser.yaml` and
`.github/workflows/release.yml`).

## Quick install (macOS / Linux)

```sh
curl -fsSL https://sworn.sh/install.sh | sh
```

Detects your OS/arch, downloads the latest release archive, verifies its checksum,
and installs to `/usr/local/bin` (or `~/.local/bin` without sudo). Pin a version with
`SWORN_VERSION=v0.1.0` or change the target with `SWORN_INSTALL_DIR=...`. Script:
[`packaging/install.sh`](./install.sh).

## Homebrew (macOS / Linux)

```sh
brew install swornagent/tap/sworn
```

Formula is generated into [swornagent/homebrew-tap](https://github.com/swornagent/homebrew-tap)
on each release.

## Scoop (Windows)

```powershell
scoop bucket add swornagent https://github.com/swornagent/scoop-bucket
scoop install sworn
```

Manifest is generated into [swornagent/scoop-bucket](https://github.com/swornagent/scoop-bucket)
on each release.

## Linux packages (.deb / .rpm / .apk)

Attached to each [GitHub release](https://github.com/swornagent/sworn/releases). Download
the file for your distro and install directly, e.g.:

```sh
sudo dpkg -i sworn_<version>_linux_amd64.deb     # Debian/Ubuntu
sudo rpm -i  sworn_<version>_linux_amd64.rpm      # Fedora/RHEL
sudo apk add --allow-untrusted sworn_<version>_linux_amd64.apk   # Alpine
```

## Go install (any platform with Go 1.26+)

```sh
go install github.com/swornagent/sworn/cmd/sworn@latest
```

Builds from source off the default branch.

## Container (any Docker host)

```sh
docker run --rm ghcr.io/swornagent/sworn version
docker run --rm -v "$PWD:/workspace" ghcr.io/swornagent/sworn verify \
  --spec /workspace/spec.md --diff /workspace/diff
```

Published to `ghcr.io/swornagent/sworn` (`:latest` and `:vX.Y.Z`). A `scratch` image —
only the binary, no shell or package manager. Volume-mount spec/diff files in.

---

## Prerequisites for maintainers (before the first `v*` tag)

Two publisher repos must exist, and one PAT must be able to write to both:

1. **Create the publish repos** (public, may be empty):
   - `swornagent/homebrew-tap`
   - `swornagent/scoop-bucket`
2. **Create one PAT** with `Contents: read/write` on *both* repos above (fine-grained
   PAT, or a classic PAT with `repo` scope), and set it on `swornagent/sworn` as the
   secret **`HOMEBREW_TAP_GITHUB_TOKEN`**. Both the Homebrew and Scoop publishers read
   this same token — one secret, two repos.
3. **GHCR** uses the workflow's built-in `secrets.GITHUB_TOKEN` — nothing to set up.
4. **nfpm** (.deb/.rpm/.apk) and the archives need no repo or token — GoReleaser attaches
   them to the GitHub release directly.

Then push a tag (`git tag v0.1.0 && git push origin v0.1.0`); the release workflow
re-runs the gates and publishes every channel above.

## Fast-follow channels (not yet wired)

- **winget** — GoReleaser can generate the manifest, but publishing opens a PR to
  `microsoft/winget-pkgs` and passes their review (a fork + token are required, and it
  is not instant like Scoop). Add a `winget:` block once the fork is set up.
- **AUR** — an `aurs:` block can push a `PKGBUILD` to an AUR git repo (needs an SSH deploy
  key). Good for the Arch audience.
- **Homebrew core / Nixpkgs / npm wrapper** — gated on notability or extra maintenance;
  pursue once traction justifies it.
