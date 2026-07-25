# Candidate binding

- Repository: `swornagent/sworn`
- Base commit: `48d640ba5cdd28f5f1f00e3cb58cec844d2f2a36`
- Candidate commit: `f1916044b8ddf8c54987e2f5f96187142d7c3ede`
- Candidate tree: `5a4c0bc37a69e45587d0ac20e7f015d9a13a0beb`
- Product-tree digest: `sha256:579cb7451587304dcadd324a58acd7bc4cafbf4e118588df12fac9068954dea3`
- Plan digest: `sha256:cf6e9103219c76a12834fbaf1eb9da8576b765dfe0602ebe416e756ed8ca10f8`
- Approval digest: `sha256:1cf79386fa391d93c19e03abe322e0425455967bec5a03785a046356b8aa2a0c`
- Design digest: `sha256:00e7fc17868c7176862f8208a5bf9e4101bff8d63e824e28e0c8d5eae1c6a11b`
- Captain invocation: `codex:/root/w1_authority_captain/sworn-v0.3.0/T1-authority/W1-authority-core/review/2`
- Producer invocation: `codex:/root/w1_authority_build/sworn-v0.3.0/T1-authority/W1-authority-core/implement/1`

# Acceptance evidence

| Acceptance | Result | Evidence reference |
| --- | --- | --- |
| A-W1-authority | pass | Candidate `f1916044b8ddf8c54987e2f5f96187142d7c3ede` implements strict admitted plan/status records, exact lifecycle and evidence bindings, immutable read-only authority snapshots, candidate/history validation, product identity, all seven typed Baton actions, exact retry reconciliation, track composition, assembly preparation and exact target integration in `internal/baton`. `internal/gitx` remains a Baton-agnostic mechanical boundary with one real-pathed absolute Git executable, fixed sanitized environment, closed OIDs for SHA-1/SHA-256, immutable object preparation and atomic compare-and-set ref transactions. External-package tests expose read-only projection and eligibility without repository handles, paths, evidence admissions or mutation APIs. |
| A-W1-authority | pass | The development-only golden generator imports the four digest-pinned Baton RC2 JavaScript record sources and invokes the real `createBatonActions` façade. Two independent clean generations in separate temporary roots were byte-identical to each other and the committed corpus in both SHA-1 and SHA-256 repositories. Go matches the oracle for strict records, lifecycle, all seven actions, immediate unchanged retries, pristine rebound, handoff digests, receipts, exact commit/tree OIDs and final refs. Candidate/history tests reject product-before-PROCEED, scope escape, mixed product/record commits, record-final candidates and product-after-candidate; Git tests prove product identity, deterministic fast-forward/two-parent composition, custom merge-driver rejection, replace-ref isolation, containment rejection and atomic no-partial ref movement. |

# Checks

| Command or check | Exit status | Raw evidence reference |
| --- | --- | --- |
| `GOFLAGS=-buildvcs=false go test -race -count=1 -timeout 300s ./internal/baton/... ./internal/gitx/... ./tools/batongolden/...` | 0 | Independent committed-candidate run; combined output `sha256:acbed707f92cca8d93b436e28047ff8338f883b177ca3a5e70426c6cd5dde090`. |
| `GOFLAGS=-buildvcs=false go test -count=1 -timeout 300s ./...` | 0 | Independent committed-candidate run; combined output `sha256:3fcb323936096bc4c7fc22c018e3289bc1facea2c8596adc9eb596295aee6fb0`. |
| Three race repetitions of the complete seven-action/retry loop, pristine rebound, product/CAS matrix and deterministic composition | 0 | Independent focused run; combined output `sha256:b30d98e5f4e1fd6d407a89baa6b86dd1d0e80654123b45bc3132da2ae1cb0f23`. |
| Twenty repetitions of strict-record errors, independent lifecycle gates and evidence admission | 0 | Independent focused run; combined output `sha256:21600d52301014f99dcbaf1a31d5228c086af9149146cfcaa6a99dcf9f4d1114`. |
| `GOFLAGS=-buildvcs=false go vet ./internal/baton/... ./internal/gitx/... ./tools/batongolden/...` | 0 | No output; `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`. |
| Two explicit `batongolden generate` runs from the pinned RC2 JavaScript reference, recursive comparison to each other and the committed corpus, then `batongolden verify` | 0 | Temporary evidence root `/tmp/sworn-w1-oracle-root.MDtnuY`; manifest `sha256:5351cba850ff0dafdb513a550796122dfdde04b216188488e269e74f49772953`; four vector files, 82,918 bytes. Vector SHA-256 values: actions `db80bb21...`, Git `cb49495f...`, lifecycle `bb24d0df...`, records `da581a2...`. |
| Product-only formatting, `git diff --check`, clean worktree and changed-path audit | 0 | Final product commit changes exactly 25 paths under `internal/baton`, `internal/gitx` and `tools/batongolden`; final-commit name-status digest `sha256:4c46856deccc5ec1f0a688270491b6227722ba0f32d64393d03386583460773e`. No runtime, journal, driver, configuration, dependency or Baton record path changed. |
| RC2 product identity through the pinned JavaScript development oracle | 0 | Candidate tree `5a4c0bc37a69e45587d0ac20e7f015d9a13a0beb`; 82 ordered product entries; product-tree digest `sha256:579cb7451587304dcadd324a58acd7bc4cafbf4e118588df12fac9068954dea3`. |

# Deviations

None.

# Not delivered

Scheduling, durable journal recovery, model invocation, provider adapters,
operator cockpit, evaluation and OTel are deliberately owned by later approved
work and are not W1 claims.
