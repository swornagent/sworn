# Design TL;DR — S09-distribution

## §1. User-visible change

A developer will be able to install `sworn` through three channels: `brew install swornagent/tap/sworn` (macOS), `go install github.com/swornagent/sworn/cmd/sworn@latest` (any Go developer), or `docker run ghcr.io/swornagent/sworn` (any Docker host). Each produces a working binary that responds to `sworn version` with the release tag it was built from. The release process is automated: pushing a `v*` tag triggers a GitHub Actions workflow that uses goreleaser to cross-compile static binaries for linux/darwin on amd64/arm64, publish a GitHub Release with checksums, push a Homebrew formula to the tap repo, and push a multi-arch container image to GHCR.

## §2. Design decisions not in spec (max 5)

1. **Build matrix: linux/darwin × amd64/arm64, no Windows.** Windows is the only platform that would need different archive packaging (.zip) and carries the CI cost of runners that don't match our primary targets (macOS desktop + Linux CI/container). It's a one-line addition to the goreleaser matrix later.
   - Rationale: every distribution channel the spec names (`brew`, `go install`, container) targets macOS or Linux. Adding Windows now would mean shipping untested binaries.

2. **Container base image: `scratch` (empty).** `CGO_ENABLED=0` produces a fully static binary that needs no libc, no CA certs (Go bundles its own), no shell. `scratch` is the smallest possible image.
   - Rationale: minimal attack surface, standard idiom for static Go binaries.

3. **Homebrew formula via goreleaser `brews` stanza, not a hand-maintained formula.** goreleaser's Homebrew support auto-generates the Ruby formula, computes the SHA256 from the published archive, and pushes to the tap repo (`swornagent/homebrew-tap`) on each release. No manual formula maintenance.
   - Rationale: eliminates drift between the published binary and the formula's checksum/version. The tap repo itself must exist and be accessible via a `HOMEBREW_TAP_GITHUB_TOKEN` secret.

4. **Release trigger: `v*` tag push.** goreleaser reads the tag, strips the `v` prefix for the Homebrew formula version, and passes the full tag through ldflags as `main.version`. The Makefile's `VERSION ?= 0.0.0-dev` default stays for development builds.
   - Rationale: goreleaser's native model. The tag IS the version; no second source of truth.

5. **`packaging/` directory holds only a README describing the distribution channels.** goreleaser config lives at repo root (convention), Dockerfile at repo root (convention), and the workflow in `.github/workflows/`. `packaging/` exists as an umbrella for distribution-related docs but doesn't contain build config.
   - Rationale: the spec lists `packaging/` as a planned touchpoint; goreleaser+Dockerfile+workflow cover the actual distribution machinery.

## §3. Files I'll touch grouped by purpose

- **goreleaser config** — `.goreleaser.yaml` (new): build matrix, archive config, ldflags, GitHub Release, Homebrew formula, checksums, snapshot support.
- **Release automation** — `.github/workflows/release.yml` (new): tag-triggered workflow that runs goreleaser with the required secrets (GITHUB_TOKEN, HOMEBREW_TAP_GITHUB_TOKEN).
- **Container image** — `Dockerfile` (new): multi-stage build producing a minimal scratch image.
- **Distribution docs** — `packaging/README.md` (new): documents the three install channels per the spec entry points.
- **No changes to Go code** — `sworn version` already works (landed in S08-init-config); goreleaser's ldflags supply the real version at build time.

## §4. Things I'm NOT doing

- **Windows builds.** Not in the matrix. Deferred with tracking: add `windows/amd64` to goreleaser when a user asks for it.
- **macOS notarization / code signing.** Requires an Apple Developer account and notarization pipeline. Out of scope.
- **The GitHub Action gate (verify-on-top mode).** Explicitly out of scope per spec.
- **Creating the `swornagent/homebrew-tap` repository.** The goreleaser config assumes it exists. I'll document it as a prerequisite in `packaging/README.md`.
- **Automated `go install` testing in CI.** `go install` pulls from the default branch; testing it pre-release is nonsensical. The release workflow smoke-tests the built binary, which is equivalent.
- **Apt/RPM/yum packages.** Not in spec; Linux users get `go install` or the container.

## §5. Reachability plan

1. **Local goreleaser snapshot:** `goreleaser release --snapshot --skip-publish --clean` produces a dist/ binary. Run `dist/sworn_darwin_arm64/sworn version` — must print the snapshot version (not `0.0.0-dev`).
2. **Docker smoke test:** `docker build -t sworn-test . && docker run --rm sworn-test version` — must print the version.
3. **Workflow review:** the `.github/workflows/release.yml` is a static YAML file; correctness is verified by reviewing the goreleaser-action configuration and the secret references against goreleaser's documented interface.

## §6. Open questions for the Coach

(none)