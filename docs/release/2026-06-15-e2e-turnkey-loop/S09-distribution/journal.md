# S09-distribution — journal

## 2026-06-16 — Implementation session

**State transition: design_review → in_progress → implemented**

### Coach pins addressed

1. **Docker smoke test now includes `sworn verify`** — added `docker run --rm sworn-test verify --spec ... --diff ...` alongside `version` in the release workflow. Non-zero exit is expected without API key (fail-closed), confirming the binary is intact.
2. **Windows deferral has concrete tracking** — filed https://github.com/swornagent/sworn/issues/1. Updated `design.md` §4 and `status.json` `open_deferrals`.

### Decisions

- **Goreleaser v2** — used v2 schema (`version: 2`). Cleaner config, supported by goreleaser-action v6.
- **Docker multi-arch via manifests** — two single-arch images (amd64, arm64) + a manifest list combining them. Simpler than a single buildx cross-compile inside Dockerfile (which would need QEMU in the build stage). The release workflow already sets up QEMU + buildx for the goreleaser docker support.
- **`scratch` base image** — no shell, no CA certs, no package manager. User mounts spec/diff files via volume. Standard for static Go binaries.
- **No goreleaser installed locally** — couldn't run `goreleaser release --snapshot`. Validated via `make build` + Docker build + smoke tests instead.

### Deferrals

- **Windows builds** — tracked in #1. Acknowledged by Coach.
- **macOS notarization** — requires Apple Developer account. Out of scope per spec.
- **GitHub Action gate** — out of scope per spec.

### Reachability

- `make build` + `./bin/sworn version`: PASS (prints `sworn 0.0.0-dev`)
- `docker build -t sworn-test . && docker run --rm sworn-test version`: PASS
- `docker run --rm sworn-test verify ...`: exits 2 (Unconfigured — expected fail-closed)
- `go test ./...`: all pass
- `go vet ./...`: clean
### Skeptic panel

Skipped — no Agent/Workflow tool available in this harness. Advisory QA not run.

## 2026-06-16 — Verifier session

**State transition: implemented → verified**

### Verifier verdicts received

PASS

**Gate 1 (User-reachable outcome):** PASS — goreleaser `brews` stanza wires to `swornagent/homebrew-tap`; `builds` stanza compiles `./cmd/sworn` for `go install`; `dockers` stanza pushes to `ghcr.io/swornagent/sworn`. Release workflow triggers on `v*` tags.

**Gate 2 (Planned touchpoints):** PASS — All four planned touchpoints changed: `.goreleaser.yaml`, `.github/workflows/release.yml`, `Dockerfile`, `packaging/README.md` (subdir of `packaging/`). Extra docs artefacts are expected slice overhead.

**Gate 3 (Required tests):** PASS — Independently verified: `go test ./...` all pass; `go vet ./...` clean; `make build && ./bin/sworn version` prints `sworn 0.0.0-dev`; `docker build + docker run sworn-test version` passes; `docker run sworn-test verify` exits 2 (fail-closed, expected). Release workflow includes both CI smoke steps.

**Gate 4 (Reachability artefact):** PASS — Three manual-smoke gestures described and independently reproduced: binary version, container version, container verify (exit 2 fail-closed).

**Gate 5 (No silent deferrals):** PASS — No dark-code markers in any production file. Windows deferral is fully surfaced with all three Rule 2 elements: Why + Tracking (issue #1) + Acknowledged inline.

**Gate 6 (Claimed scope):** PASS — AC1 goreleaser config covers both install channels; AC2 Docker smoke confirms container runs `sworn verify`; AC3 `-X main.version={{.Version}}` ldflags in goreleaser config wires version to release tag.
