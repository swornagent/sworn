# W1 authority-core Implementer proof

This is an Implementer evidence handoff for fresh independent verification. It
does not record or claim a Verifier verdict.

## Candidate binding

- Release/work: `sworn-v0.3.0-baton-rc3` / `W1-authority-core`.
- Track: `refs/heads/track/sworn-v0.3.0-baton-rc3/T1-authority`.
- Repository worktree:
  `/home/brad/projects/sworn-worktrees/release-sworn-v0.3.0-baton-rc3-T1-authority`.
- Candidate: `42c305a4f747520c5d8c54d60da7f3c63c0f8dfe`.
- Parent: `dd8adbb9d9dc16e63c7df083b06276656a69e144`.
- Ordinary tree: `3b64024769da7235deb4b0fc34181b4f5b312914`.
- Product-tree identity:
  `sha256:6630f1fb5117d2de34c9116717106a0e4508f2b10e39ec82c56d0d9401873e50`.
- Plan:
  `sha256:66dd2c09b538b4eb41783128b3c4d110d10d04124a15f46b2d354858b2409d74`.
- Protected approval:
  `sha256:35415561708d3421c1272fbebada8e56748166077313ba1b540ec96736bc7602`.
- Approved design:
  `sha256:18be5ebbb7f350d0e6d3e44608b851e2abc303e4a597a4a2182f27791322d980`.
- Design Captain:
  `codex:/root/w1-rc3-design-captain/sworn-v0.3.0-baton-rc3/T1-authority/W1-authority-core/review/2`.
- Producer:
  `codex:/root/w1_rc3_product_implementer/sworn-v0.3.0-baton-rc3/T1-authority/W1-authority-core/implement/1`.
- Publication: the remote track ref resolves to the exact candidate.
- Scope: exactly 26 approved paths, `record_paths=0`, `module_paths=0`,
  `forbidden_paths=0`, and a clean worktree.

The closed candidate/product/scope record is
`/tmp/sworn-v0.3.0-rc3-W1-evidence.B2ADEy/candidate-product-scope.txt`
(1,032 bytes,
`sha256:7ca1078161643a12d47c74812fa0b56012aafd6c865b14246b80509c469f94fc`).
It binds the canonical 26-path manifest, product manifest and full-index
candidate patch:

- `candidate.paths`: 832 bytes,
  `sha256:378cc9b73fdba1a5d24b1f77787c17c41bd151def6ec73855c819f15fed948d7`;
- `candidate-product-tree.manifest`: 7,413 bytes,
  `sha256:6630f1fb5117d2de34c9116717106a0e4508f2b10e39ec82c56d0d9401873e50`;
- `candidate-commit.patch`: 462,912 bytes,
  `sha256:8013a88012c168d229b00675dd405c74c80d18bc1146bd7a61ccc90bd58ee72d`.

## Implemented authority core

The candidate provides the Plan-derived public Baton action facade, strict
records and lifecycle transitions, immutable exact Snapshot selection,
protected evidence binding, product-inert record preparation, and deterministic
fast-forward/two-parent composition.

The Git adapter admits one canonical absolute repository and Git executable,
uses a closed literal environment and bounded process runner, supports SHA-1
and SHA-256 repositories, captures exact refs as typed direct/absent/symbolic/
broken/non-commit/unreadable states, and performs one prepared no-deref CAS
against the captured vector. It rechecks beneath held locks, accounts for
child wait/reap/process-group/lock quiescence, and reconciles only as desired,
snapshot-scoped not-applied, or recovery-required ambiguity without an
internal retry.

The checked development corpus is bound to Baton `v1.0.0-rc.3`. Ordinary Go
verification consumes embedded vectors; explicit generation uses the pinned
JavaScript oracle.

## Acceptance evidence

| Acceptance | Implementer evidence |
| --- | --- |
| `A-W1-authority` | Strict record/lifecycle/action, ownership, composition, product identity and exact-ref behavior are covered by package/integration tests, the embedded RC3 corpus, and the eleven-case adversarial matrix. |
| `A-W1-rebind` | The candidate is a single product-only child of the fresh RC3 `PROCEED` head, reconstructs the exact old 26-path input only as implementation input, and binds fresh RC3 oracle, race, scope, budget and candidate evidence. No RC2 control artifact is included. |

Start authority is retained at
`start-authority.txt` (2,485 bytes,
`sha256:4d3e81f6f7f15c0ef07aceab7581baeae353f64df25d777a66a5d6a6393c9aae`).
Pinned RC3 identities are retained at `rc3-identities.txt` (825 bytes,
`sha256:73cab1e5991ace89c82c8b2018f4ed9fb0325e51507c51cce33954dd01abcd19`).

The old input reconstruction is `old-input-identities.txt` (375 bytes,
`sha256:ac56e06265f57327bc3ac58381f07d399b9908d942ad2bb6e7893222d8dcabb7`).
It proves 26 paths, path-list
`sha256:378cc9b73fdba1a5d24b1f77787c17c41bd151def6ec73855c819f15fed948d7`
and binary full-index patch
`sha256:0a07ef1d8a9d1ec59ea206c3af8b50004bd8244a052f2902e99db04ba78a6f5e`.

Two independent oracle generations have identical inventory, modes and bytes;
their raw diff is the empty `oracle-generation-diff.txt`. `golden-verify.json`
(805 bytes,
`sha256:0ace94fc3f28a6a8e1f6c79ee79c37badc7c9895838089fef1cbe1d638ca90be`)
binds tag object `34324784694696a38d951061c2313363b405c1e4`,
commit `affaf16cc37f845b5dc43b22988d8b680ff1f212`, tree
`a26078b7db4ee36bdae4f28a48447ff2df782f4f`, support package
`sha256:e5927a82f7c8a0daf3aa1196e7aa56231044449bb141cc2d7efd1cc8cca209bd`,
and corpus manifest
`sha256:f7fba6afbe74f64aa1829dca0b2692bc5e5f9bef4df5b5795fbc70043c2ed620`.

## Adversarial matrix

All eleven named top-level RC3 cases pass. The canonical case manifest is
`adversarial-case-manifest.tsv` (696 bytes,
`sha256:50c00379d441ca991ef09554f055fdb728ce8b4f65a6d2b8274ef40808352d2d`);
the verbose raw result is `adversarial-matrix.log` (22,104 bytes,
`sha256:1e03137d81e54ffddbeae240e538e137a36bc6320fd1007ee3c04aa4d1b57556`).
It includes SHA-1/SHA-256, typed invalid refs, exact create/update/verify,
resolving/dangling aliases and all six after-capture alias races, closed bounds,
pre-commit faults, post-commit/cleanup faults, pure-verify acknowledgement
loss, one-attempt mixed ambiguity and snapshot-scoped all-pre/ABA evidence.
Applicable rows record PID, process group, waited/reaped/quiet/lock state and
attempt count.

## Exact budget and W2 composition

| Measurement | Files | Lines |
| --- | ---: | ---: |
| W0 runtime | 6 | 673 |
| W1-owned `internal/baton` + `internal/gitx` | 15 | 7,091 |
| Post-W1 runtime | 19 | 7,190 |
| Pinned W2 `internal/driver` input | 11 | 3,277 |
| W2 delta against W0 driver placeholder | +10 | +3,274 |
| Scratch post-W1 plus W2 | 29 | 10,464 |

Canonical category and runtime-path manifests are retained with summaries
`w0.summary.txt`,
`post-w1.summary.txt`,
`w1-surfaces.summary.txt`,
`w2-input.summary.txt`, and
`post-w1-w2.summary.txt`. The machine-readable delta record is
`runtime-budget-deltas.txt` (243 bytes,
`sha256:81c2a3a45dc8191494521328b2869aa754438cd344ff9bd56fa2f6f9943ea03b`).

The independently reconstructed W2 full-index patch digests are:

- `a85668ef..3b30295e`:
  `sha256:aa00493645b22d5e90ec0199ee5e93bc54b8107dc6744286a3cdf4350a43a751`;
- `bd28ce7c..287f83d0`:
  `sha256:37a2ee63a67e5554d8070cfc110486b3871015cbcf4f31761a08cfac4523bf93`;
- `287f83d0..0136a96c`:
  `sha256:ade0486ab9784047026475e80d127ae82382c0f23d2c8254c7204afb4807bd44`;
- W0-to-W2 one-shot:
  `sha256:9bcaa63667fc4e0ac159439307c82a5c9192de538a7b76203db0898f600b3d52`.

Sequential and one-shot application are byte-identical and both yield
`internal/driver` subtree
`85eed1c35838c241f7edf6a3331cb771c59c66ce`. Their directory diff is empty;
see `w2-sequence-identities.txt` and `composition-tree-identities.txt`. W1 and
W2 paths are disjoint.

## Candidate checks

| Check | Result | Raw evidence |
| --- | --- | --- |
| Focused package tests | exit 0 | `candidate-package-tests.log`, `sha256:e1009fee72e92e75d4e3bfebd4e095038a8a1cb030626ca93b9cc08c25db20a1` |
| Required focused race test | exit 0 | `candidate-race-tests.log`, `sha256:ed3ac191a6c497f1fbbed3d7dfb825e04f570c363b310ac818b8e03261f3d3fb` |
| Full `go test -count=1 ./...` | exit 0 | `candidate-full-tests.log`, `sha256:8ea4ffd8af12251d2caa77248d4ebc88d2c9e335ec4ce098f7406be0d24209ab` |
| `go vet ./...` | exit 0 | empty `candidate-vet.log` |
| `go mod tidy -diff` | exit 0 | empty `candidate-mod-tidy-diff.log` |
| tracked-Go `gofmt -l` | exit 0 | empty `candidate-gofmt.log` |
| candidate `git diff --check` | exit 0 | empty `candidate-diff-check.log` |
| readonly, trimpath, CGO-disabled build | exit 0 | empty `candidate-build.log` |
| `batongolden verify` | exit 0 | `golden-verify.json` above |

The closed twin-build record is
`twin-build-reproducibility.txt` (1,254 bytes,
`sha256:df9f206e2093fe6f90df32c7d2ff5265aa7a27b5b74a88d1b2e33585e704ff3f`).
Two detached clones of the exact candidate used independent compile caches.
Both 2,662,562-byte binaries have
`sha256:dba032c34933587ac145a5989cdc679ccf0c3cb2008237cbd31a9379f029edef`;
their normalized build information is also identical. The record repeats the
candidate, parent, product identity and `record_paths=0`.

## Ref and cleanliness closure

After publication:

- target remains `2c9ce0493971e0e833d4dec6c562b030315e33c9`;
- release marker remains `5d4c5eb117087e306be6868103891dc685319528`;
- T0 dependency remains `7d925851dc91a4ee324d9fe29c33d631f44d1a56`;
- the W1 materialization marker remains an ancestor;
- all enumerated old-lineage refs remain unchanged; and
- independent T2 is reported only as observation
  `babb19d9c5c995d095915bc6eb79cd4540130d64`, not as a W1 gate.

The worktree is clean and the remote track ref is the exact candidate.
`postcommit-authority.txt`, `candidate-status.txt`, and `published-ref.txt`
retain the raw closure.

## Evidence index, deviations and verification handoff

The complete evidence directory is
`/tmp/sworn-v0.3.0-rc3-W1-evidence.B2ADEy`. Its digest-addressed inventory is
`evidence-index.sha256`.

Deviations: none.

Not delivered: no `.baton/releases/**` mutation, status/proof record commit,
ref transition beyond publishing the product candidate, or independent
Verifier verdict. A fresh Verifier must assess the exact candidate, ordinary
tree, product identity, tests and evidence above.
