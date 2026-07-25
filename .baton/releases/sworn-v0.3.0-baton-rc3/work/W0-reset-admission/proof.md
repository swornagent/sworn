# Candidate binding

- Repository: `/home/brad/projects/sworn`
- Work: `W0-reset-admission`
- Base commit: `67d792bb09eb26e79ea3f8bff628d25caeaccab6`
- Candidate commit: `2f17a01cbeedbb181a2143cf9ffd725476ef65ac`
- Candidate tree: `722a8879d7bbee354f269f921d61e2acda6f36c9`
- Product-tree digest: `sha256:9518e8b737a60198c768fa354d688e7775f2862cfe20d8d011369c5995b99285`
- Plan digest: `sha256:66dd2c09b538b4eb41783128b3c4d110d10d04124a15f46b2d354858b2409d74`
- Approval digest: `sha256:35415561708d3421c1272fbebada8e56748166077313ba1b540ec96736bc7602`
- Design digest or assembly components: `sha256:1635961bcba03cdf7bd98b61d230d0bd0c9e690a1c9e2a263686ab97f52caca9`
- Captain invocation: `codex:/root/w0-design-captain/sworn-v0.3.0-baton-rc3/T0-admission/W0-reset-admission/review/2`
- Producer invocation: `codex:/root/w0-product-implementer/sworn-v0.3.0-baton-rc3/T0-admission/W0-reset-admission/implement/1`

# Acceptance evidence

| Acceptance | Result | Evidence reference |
| --- | --- | --- |
| A-W0-base | pass | `/tmp/sworn-w0-impl.EGREsl/A-W0-base-lineage.txt` (`sha256:878714f60c98441f0388d7fca27965f3df6e91b8f8bf8ea4c9585b75e3e5733f`); `/tmp/sworn-w0-impl.EGREsl/A-W0-product-inertness.txt` (`sha256:5daaafc129dcf604f040de403ed1bb946d44bd048d070ebd79eedb43ed9de194`) |
| A-W0-lineage | pass | `/tmp/sworn-w0-impl.EGREsl/A-W0-base-lineage.txt` (`sha256:878714f60c98441f0388d7fca27965f3df6e91b8f8bf8ea4c9585b75e3e5733f`); `/tmp/sworn-w0-impl.EGREsl/A-W0-candidate-closure.txt` (`sha256:e84b6c61ecf925431dff6accd3a1642ff98ca5e2524ce0f28e8e21028c415e23`) |
| A-W0-assets | pass | `/tmp/sworn-w0-impl.EGREsl/A-W0-release-identity.txt` (`sha256:5504975726967d5f91b261416e3f86f2a64e5a8d798355c45acc6d433a753fac`); `/tmp/sworn-w0-impl.EGREsl/A-W0-assets-snapshot.txt` (`sha256:3159ec1fbf62873c3b9087172d1c99f9866d2206321018de738983c57a6d27fb`); `/tmp/sworn-w0-impl.EGREsl/A-W0-support-generation.txt` (`sha256:cd05577b22e1dbe9ca57d1589b08c294375c4399aee3687acdce627b67681c18`) |
| A-W0-budget-authority | pass | `/tmp/sworn-w0-impl.EGREsl/A-W0-budget-authority.txt` (`sha256:d280a316560e5b6c1a9f85bdbe3178d2312489dc421100dc711d3efe30c2b858`); `/tmp/sworn-w0-impl.EGREsl/A-W0-budget-category-manifest.tsv` (`sha256:13cc3db894aa8958262243570e89f6de29f80bb6d2c72a637ec02998eca132b9`); `/tmp/sworn-w0-impl.EGREsl/A-W0-budget-runtime-path-counts.tsv` (`sha256:c0693e0dc0fda9878ae536c6947dbaad870d80c685962d1c00b19c69f050638b`); `/tmp/sworn-w0-impl.EGREsl/A-W0-budget-summary.txt` (`sha256:d3d04ef05d1a0e7a4397b7ef85d3cd34f7e7dbca36c613a48ac447736278cadb`) |
| Candidate closure | pass | Candidate has parent `62fb274dc539e3ff138aa9cb914f356dff361e22`, 206 product-only changed paths, zero `.baton/releases/**` or `docs/**` changes, exact independently generated snapshot bytes, unchanged release/target/old-lineage refs, and a clean worktree: `/tmp/sworn-w0-impl.EGREsl/A-W0-candidate-closure.txt` (`sha256:e84b6c61ecf925431dff6accd3a1642ff98ca5e2524ce0f28e8e21028c415e23`) |

# Checks

| Command or check | Exit status | Raw evidence reference |
| --- | --- | --- |
| `GOFLAGS=-buildvcs=false go test -count=1 ./tools/batonassets/... ./tools/batongolden/... ./cmd/sworn/...` | 0 | `/tmp/sworn-w0-impl.EGREsl/check-focused.log` (`sha256:b8d71459e538e907b72fb45439b4da60d0a094c7f2cb263ef6cc67b4f58645f1`) |
| `GOFLAGS=-buildvcs=false go test -count=1 ./internal/baton/...` | 0 | `/tmp/sworn-w0-impl.EGREsl/check-internal-baton.log` (`sha256:5e11aafbd614e72bcd198d739a0f8b432b5538bb2f79ddb2073335830f3fa9b1`) |
| `GOFLAGS=-buildvcs=false go test -count=1 ./...` | 0 | `/tmp/sworn-w0-impl.EGREsl/check-full.log` (`sha256:16802ba3b1bba81f9b2ed04a411e4486b402a6080f555ba65f8aef129adfc321`) |
| `GOFLAGS=-buildvcs=false go test -count=1 -race ./...` | 0 | `/tmp/sworn-w0-impl.EGREsl/check-race.log` (`sha256:6127e4392001cf1aa2df47606933e88737e6c670905cb8dbe28fa8cba17b9289`) |
| `GOFLAGS=-buildvcs=false go vet ./...` | 0 | `/tmp/sworn-w0-impl.EGREsl/check-vet.log` (`sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`) |
| Go formatting gate | 0 | `/tmp/sworn-w0-impl.EGREsl/check-format.log` (`sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`) |
| `GOWORK=off GOFLAGS=-buildvcs=false go mod tidy -diff` | 0 | `/tmp/sworn-w0-impl.EGREsl/check-tidy.log` (`sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`) |
| Product-only `git diff --check` | 0 | `/tmp/sworn-w0-impl.EGREsl/check-diff-candidate.log` (`sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`) |
| `CGO_ENABLED=0 GOFLAGS=-buildvcs=false go build -mod=readonly -buildvcs=false -trimpath -o /tmp/sworn-v0.3.0 ./cmd/sworn` plus build-info/symbol inspection | 0 | `/tmp/sworn-w0-impl.EGREsl/check-build.log` (`sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`); `/tmp/sworn-w0-impl.EGREsl/check-build-summary.txt` (`sha256:0ee68c89f1b2920fc9299e7d8c9872f7ea967fd2fd93177362b2988b5f85cf6a`) |
| `/tmp/sworn-v0.3.0 version --json` exact RC3 identity | 0 | `/tmp/sworn-w0-impl.EGREsl/check-version-json.log` (`sha256:4b0ffb46bb919cc0a6fbb19933f88606527bc142ce5a72f5fb9c07e4ec148d45`) |
| `batongolden verify` exact RC3 identity | 0 | `/tmp/sworn-w0-impl.EGREsl/check-batongolden.log` (`sha256:76083896e343530858e4047431d90b41179a8d0b0c67fab9061a66d8a8521dbb`) |
| Four named reproducibility and isolation tests, verbose | 0 | `/tmp/sworn-w0-impl.EGREsl/check-isolation-named.log` (`sha256:3b7c4d0bd27154980b056e0409d78d6c7e6dbe0b08733f5e6248a2ae7217ce55`) |
| Module package-set and dependency gate | 0 | `/tmp/sworn-w0-impl.EGREsl/check-module-summary.txt` (`sha256:49cbda5f31e927114b3f2a296b00e1f6160d7c284a4d83d7f6d6c67d1951f27e`) |
| Twin fresh-cache product-only builds, one with record-only history | 0 | `/tmp/sworn-w0-impl.EGREsl/check-repro-summary.txt` (`sha256:3688c2c5f28e0d732a6e3ccb3d97e6f3e8d8f84cc76c9ac58d1b9aa7a373599e`) |

# Deviations

None.

# Not delivered

None.

# Verification handoff

This proof records the frozen Implementer candidate and does not claim an
independent Verifier verdict. A fresh Verifier must assess the exact candidate
commit, ordinary tree and product-tree digest bound above.
