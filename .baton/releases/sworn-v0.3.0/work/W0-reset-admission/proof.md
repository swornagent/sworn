# Candidate binding

- Repository: `swornagent/sworn`
- Base commit: `b58fdab7ea912e41c7097d91540035b043358205`
- Candidate commit: `fb6e56b7900bc55055364ee3033f24ef9cf02551`
- Candidate tree: `b7ede415fc5d276e46fbf9512000f16e1c570b28`
- Product-tree digest: `sha256:050a3afe0c5b44db9cf66d269b869f95237a243794a7b2b919debcf49b70814c`
- Plan digest: `sha256:cf6e9103219c76a12834fbaf1eb9da8576b765dfe0602ebe416e756ed8ca10f8`
- Approval digest: `sha256:1cf79386fa391d93c19e03abe322e0425455967bec5a03785a046356b8aa2a0c`
- Design digest: `sha256:efa65370e9bfa2f19a6a1ec87da61d3aebffbc2f2f1868ac69d0f0a51bf80675`
- Captain invocation: `codex:/root/w0-captain/sworn-v0.3.0/T0-admission/W0-reset-admission/review/1`
- Producer invocation: `codex:/root/baton-implement/sworn-v0.3.0/T0-admission/W0-reset-admission/implement/1`

# Acceptance evidence

| Acceptance | Result | Evidence reference |
| --- | --- | --- |
| A-W0-base | pass | Live `origin/release/v0.3.0` remained `2c9ce0493971e0e833d4dec6c562b030315e33c9`, tree `a10d213da750ece28a6dc066e2170c76fc959def`, with ancestor `c32d6846a98aef59a33d0a4bca89a4fde434a1d1`; the installed plan and protected issue-157 approval retained the digests above; only T0 existed remotely; the committed product tree contained none of the conservative rehearsal target/release-head, prior plan/approval digest, approval-marker, or superseded-proposal values. |
| A-W0-assets-product | pass | Candidate `fb6e56b7900bc55055364ee3033f24ef9cf02551` removes every named v0.2 production package and leaves exactly `cmd/sworn`, `internal/{baton,driver,gitx,journal,runtime}`, and development tools `tools/{batonassets,batongolden}`. Live release evidence resolved annotated tag object `b80f3e27f0e0a71a4883bcc282e4843e085f0e04` to commit `890238ef063bb53cf51fb3359f1ff527f14846c6` and tree `97513f3e6f798f3ad04d5b510a49496a605a8ea4`; the downloaded release archive was `sha256:968088ede0c3bfbafb0a9372d3abbf6853556cc2a6e85ffc25615d6332977e63` and embedded that commit; generated support was `sha256:676c630c6a4ef3f752d604efaa5e51958adec0d8580b74cec7fb1e689b1d3436`. Two independent offline snapshotter runs were byte-identical to each other and the admitted 14-asset, 50,387-byte snapshot with manifest `sha256:74243a42dcbaa65eadac161126e9cfa8710803a136b827dd7001f5648459986c`. Separate product copies and fresh caches built byte-identically while ignoring record-only history. |

# Checks

| Command or check | Exit status | Raw evidence reference |
| --- | --- | --- |
| `GOFLAGS=-buildvcs=false go test -count=1 ./tools/batonassets/... ./tools/batongolden/... ./cmd/sworn/...` | 0 | Fresh committed-candidate execution; all three packages passed. |
| `GOFLAGS=-buildvcs=false go test -count=1 ./...` | 0 | Fresh committed-candidate execution; all admitted packages passed. |
| `GOFLAGS=-buildvcs=false go test -count=1 -race ./...` | 0 | Fresh committed-candidate execution; all admitted packages passed under the race detector. |
| `GOFLAGS=-buildvcs=false go vet ./...`, product-only `gofmt`, committed-range `git diff --check`, and `go mod tidy -diff` | 0 | Fresh committed-candidate execution; no finding or diff. |
| `TestModuleHasOnlyTheAdmittedPackageSet`, `TestBuiltBinaryHasNoLegacySymbolsOrVCSSettings`, `TestTwinProductBuildsIgnoreRecordOnlyHistory`, and `TestProductCopyAndArchiveExcludeBatonRecords` | 0 | Fresh targeted verbose execution; every boundary passed. |
| `CGO_ENABLED=0 GOFLAGS=-buildvcs=false go build -mod=readonly -buildvcs=false -trimpath ./cmd/sworn` | 0 | Binary `sha256:131782d07356544e2f31a8eb02c9ef023b39be269fa221b724c55f779ea3cd12`; `go version -m` contained no `vcs.*` setting or temporary root. |
| Pinned Baton tag/archive/support checks and two offline `tools/batonassets snapshot` runs | 0 | Exact identities recorded under A-W0-assets-product; both outputs matched the checked-in snapshot recursively. |
| GitHub Actions CI | 0 | Push run `30132514148` passed at exact candidate `fb6e56b7900bc55055364ee3033f24ef9cf02551`. |

# Deviations

None.

# Not delivered

None.
