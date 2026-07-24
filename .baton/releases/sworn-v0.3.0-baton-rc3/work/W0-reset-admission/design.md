# Approach

Invocation:
`codex:/root/w0-design-implementer/sworn-v0.3.0-baton-rc3/T0-admission/W0-reset-admission/design/2`.

W0 is a fresh RC3-bound physical product reset and immutable-package
admission. It is not a continuation of the RC2 control lineage and it does not
cherry-pick an RC2 candidate.

1. Keep the product/materialization anchor distinct from the evolving owner
   record chain:
   - target `refs/heads/release/v0.3.0` was, immediately before installation,
     commit `2c9ce0493971e0e833d4dec6c562b030315e33c9`, tree
     `a10d213da750ece28a6dc066e2170c76fc959def`, with parents
     `c32d6846a98aef59a33d0a4bca89a4fde434a1d1` then
     `045d6e9c56c5da523f5d8a149e29fefeb7c56f17`;
   - install commit
     `67d792bb09eb26e79ea3f8bff628d25caeaccab6` is the exact child of that
     target and changes only the new plan and its initial statuses;
   - materialization commit
     `0eccb99abb6a49497ae2e1dc89f9d09cf115a4c1` is the immutable W0
     product/materialization base. It is the current head of
     `refs/heads/release-wt/sworn-v0.3.0-baton-rc3` and that release ref must
     remain there throughout design and implementation;
   - the revision responsibility starts from the published post-review owner
     head `e5e078011932d864fc291ba2ddca0396f08d6a9d` on
     `refs/heads/track/sworn-v0.3.0-baton-rc3/T0-admission`. Its chain from
     materialization is
     `0eccb99a` -> DESIGN_WRITTEN
     `ba927969797fe8a10459a5fae472bfaccc09df69` -> REVISE
     `e5e07801`;
   - the raw plan digest is
     `sha256:66dd2c09b538b4eb41783128b3c4d110d10d04124a15f46b2d354858b2409d74`
     and its protected approval digest is
     `sha256:35415561708d3421c1272fbebada8e56748166077313ba1b540ec96736bc7602`.
2. Before any product edit, capture the exact then-current owner head after
   this revised design and a new Captain `PROCEED`; call it
   `implementation_start_owner_head`. It must be a first-parent descendant of
   `e5e078011932d864fc291ba2ddca0396f08d6a9d`. Prove every commit from
   `0eccb99abb6a49497ae2e1dc89f9d09cf115a4c1` through
   `implementation_start_owner_head` changes only `.baton/releases/**`, has
   the same non-record product identity as `0eccb99a`, and is a valid W0
   design/review transition on the admitted plan. Simultaneously require the
   release ref to remain exactly `0eccb99a` and the target to remain exactly
   `2c9ce049`. The owner head is expected to advance for design and review
   records; it must never be required to equal the materialization or release
   head. Any foreign commit, non-record path, product-identity change,
   unexpected ref, dirty product start, or changed approval stops.
3. The record-only classification is proved, never trusted. Each intervening
   commit must have both equal non-record product identity and a structurally
   `.baton/releases/**`-only delta. Missing evidence, ambiguity, or an
   unconditional host-policy `inert` result stops. This classification is
   implementation evidence, not a product constant.
4. Reconstruct the reset as a new product delta on
   `implementation_start_owner_head`, whose pre-edit product identity is the
   immutable `0eccb99a` product identity. The old W0 product delta from
   `b58fdab7ea912e41c7097d91540035b043358205` to
   `fb6e56b7900bc55055364ee3033f24ef9cf02551`, product-patch digest
   `sha256:be8c3c41ac356b19eda82f5de8fe81965f37dbf02710d26a44555a456ec7d93a`,
   may be inspected or replayed mechanically only after excluding
   `.baton/releases/**`. Its commit, design, proof, approval, statuses,
   verdicts and receipts are never copied or cited as current authority.
5. Remove the v0.2 production packages
   `internal/{adapter,app,board,buildinfo,config,control,effects,engine,executor,policy,producer,protocol,repo,store,workspace}`.
   Reduce `go.mod` to the module and approved Go version and remove `go.sum`;
   W0 retains no dormant dependency for later work.
6. Establish the reset product as exactly `cmd/sworn`,
   `internal/{baton,driver,gitx,journal,runtime}`, and
   `tools/{batonassets,batongolden}`. The four future packages contain
   ownership documentation only. `cmd/sworn` exposes only `help` and
   `version [--json]`; every delivery, board, executor-shim, or unknown command
   refuses before consuming a path, configuration, repository, database, or
   model input.
7. Generate the admitted asset snapshot only from the published RC3 commit.
   Baton v1.0.0-rc.3 is fixed by:
   - annotated tag object
     `34324784694696a38d951061c2313363b405c1e4`;
   - peeled commit `affaf16cc37f845b5dc43b22988d8b680ff1f212`;
   - peeled tree `a26078b7db4ee36bdae4f28a48447ff2df782f4f`;
   - normalized release archive
     `sha256:4757078049d8e9f0ac3db2aee91e65f8df48f31b0cccf26478343ca3d79d5166`;
   - generated installed support package
     `sha256:e5927a82f7c8a0daf3aa1196e7aa56231044449bb141cc2d7efd1cc8cca209bd`.
   Independently resolve and compare all five. Verify the tag is annotated,
   its peel and tree are exact, the unique normalized archive embeds the exact
   commit and is safe to extract, and two isolated support-package generations
   match the installed package digest. Any unresolved or mismatched value
   fails closed.
8. Preserve the reviewed closed, path-ordered 14-file product inventory, but
   regenerate every selected byte, manifest entry, size, mode and digest from
   the exact published RC3 commit. Rewrite the admission lock, compiled
   identity, tests, version output and golden verifier to RC3. Runtime
   validation has no network, mutable checkout, Git, Node, home directory,
   Baton record, or old snapshot input. The JavaScript package is a
   development oracle only.
9. Keep `.baton/releases` control-only. Exclude it explicitly from product
   copies, archives, file walkers, formatter input, checks, model workspaces,
   candidates, builds and release packages. The final implementation candidate
   is a product-only child of `implementation_start_owner_head`. A later Baton
   proof/status commit must preserve the exact candidate product-tree identity.
10. Accept the approved budget figures only after exact independent
    reproduction. The pinned external planning evidence is commit
    `551449df923ed432073819b01395fbd99629820e`, path
    `docs/captures/2026-07-25-sworn-v0.3.0-rc3-replacement-plan-measurement.md`,
    raw-blob SHA-256
    `a2a7f73d6b47149a3ccb8d47bdfb479026a81c93d92d55a02dac9f3a11625f05`.
    That commit is not an ancestor of the target and is never product or
    control authority. Reproduce it only from these exact repository objects:
    - W0 product candidate
      `fb6e56b7900bc55055364ee3033f24ef9cf02551`, ordinary tree
      `b7ede415fc5d276e46fbf9512000f16e1c570b28`, product-tree digest
      `sha256:050a3afe0c5b44db9cf66d269b869f95237a243794a7b2b919debcf49b70814c`;
    - shared W0 materialized base
      `b2bb28778a4f73f5841bb936a993860d0b10d0ce`, tree
      `ddb4ebf0eb62c4c47233f4f38220b0b80907887b`;
    - W1 track head `c8ddcd89cbfdaab6c94d601631f87840bf7fd7c2`,
      tree `3ad90403b2dffed7eec0d99119f421444430dab5`, whose product delta from
      `b2bb2877` has 26 paths, path-list digest
      `sha256:378cc9b73fdba1a5d24b1f77787c17c41bd151def6ec73855c819f15fed948d7`
      and binary full-index patch digest
      `sha256:0a07ef1d8a9d1ec59ea206c3af8b50004bd8244a052f2902e99db04ba78a6f5e`;
    - W2 candidate `0136a96c4355e60c815b5cab043b54e860d00062`,
      tree `252f1a7c326cbc604600e7beb2393bb7f4f22fd1`, whose product delta from
      `b2bb2877` has 19 paths, path-list digest
      `sha256:c6e280555e27889b257e708fa26a1dddade52aa5371126aae4ffd3039b8568e2`
      and binary full-index patch digest
      `sha256:9bcaa63667fc4e0ac159439307c82a5c9192de538a7b76203db0898f600b3d52`.
    Require both product patches to exclude `.baton/releases/**`, have zero
    path intersection, apply cleanly in either order to the shared W0 product,
    and yield the same recorded product-manifest digest
    `sha256:abe8d9078e0daa0c7da6fd528c37071e4cef3afc94804a2c424a900f17a0471a`.
    Equivalently, start from the non-record product of the W1 track head and
    apply the exact W2 `b2bb2877..0136a96c` product patch.
11. Classify every tracked regular `*.go` file in raw path-byte order with
    exactly this first-match precedence:
    - `generated` when any line in the file matches the exact regular
      expression `^// Code generated .* DO NOT EDIT\.$`;
    - `fixture` for `test/*`, `*/testdata/*`, `*/fixtures/*`, `fixture/*`, or
      `fixtures/*`;
    - `test` for a path ending `_test.go`;
    - `tooling` for `tools/*`;
    - `runtime-production` for `cmd/*` or `internal/*`;
    - `other-production` otherwise.
    The generated match is not restricted to the first line and `.*` is not
    replaced by a non-empty-text rule. Blank and comment records count. Paths
    are repository-relative POSIX UTF-8 and sorted by raw bytes under
    `LC_ALL=C`. Every baseline file must end in LF. Physical lines are
    `awk 'END { print NR }'` records; an unterminated final non-empty record
    would count once but fails the baseline LF requirement. Each canonical
    category-manifest row is exactly
    `category<TAB>lines<TAB>bytes<TAB>sha256<TAB>path<LF>`.
12. Emit the exact canonical path/count manifest as
    `path<TAB>decimal-physical-lines<LF>` with no header:

    ```text
    cmd/sworn/main.go	90
    internal/baton/actions.go	769
    internal/baton/actions_tracks.go	692
    internal/baton/assets.go	571
    internal/baton/candidate.go	425
    internal/baton/evidence.go	214
    internal/baton/lifecycle.go	433
    internal/baton/plan.go	718
    internal/baton/reader.go	583
    internal/baton/repository.go	330
    internal/baton/status.go	642
    internal/baton/strictjson.go	271
    internal/driver/contract.go	873
    internal/driver/control.go	264
    internal/driver/doc.go	8
    internal/driver/fake.go	259
    internal/driver/invoke.go	174
    internal/driver/process_linux.go	486
    internal/driver/process_other.go	9
    internal/driver/projection.go	285
    internal/driver/selection.go	225
    internal/driver/submission.go	619
    internal/driver/usage.go	75
    internal/gitx/doc.go	7
    internal/gitx/prepare.go	435
    internal/gitx/refs.go	376
    internal/gitx/repository.go	625
    internal/journal/doc.go	3
    internal/runtime/doc.go	3
    ```

    Its raw-byte digest must be
    `sha256:c0693e0dc0fda9878ae536c6947dbaad870d80c685962d1c00b19c69f050638b`,
    with exactly 29 rows summing to exactly 10,464. Independently reproduce
    these exact category results from the canonical rows:
    - runtime-production: 29 files, 10,464 lines,
      `sha256:da994287128bd8efb294fbe0a6a656dff51f10ce7e7285363d5cd3ab2edda744`;
    - tooling: 2 files, 773 lines,
      `sha256:da34736c9ead2d9fa89716381b79613baca1f521cfc8e7a2ba652114148eb5dd`;
    - tests: 16 files, 5,247 lines,
      `sha256:075f08e3e5bac6d0cd68870ab0d53a621cf26af669d8024649aa0636f4ed24b6`;
    - fixtures: 2 files, 247 lines,
      `sha256:eaaed722847cfef5b68a1c1a4b1289f9af633d9cf24c97b845120a21c81a7e48`;
    - generated: 0 files, 0 lines, the empty-manifest digest
      `sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`;
    - other-production: 0 files and 0 lines; and
    - all Go: 49 files, 16,731 lines,
      `sha256:13cc3db894aa8958262243570e89f6de29f80bb6d2c72a637ec02998eca132b9`.
    Only then may W0 evidence repeat the 10,464/29 baseline, the normal combined
    ceiling of 18,064, or the two proof-backed Captain bands through 19,264.
    Any input, patch, path, row, digest, count, final-line, category or aggregate
    mismatch stops for corrected captured evidence and newly approved plan
    bytes; it must never be rounded, inferred or asserted from historic prose.

# Surfaces

- Delete `go.sum` and the legacy `internal` package directories listed in
  Approach step 5.
- Rewrite `.gitattributes`, `.github/workflows/ci.yml`, `AGENTS.md`,
  `README.md`, `cmd/sworn/**`, and `go.mod`.
- Add or rewrite
  `internal/baton/{assets.go,assets_test.go,release.json,snapshot/**}`.
- Add `internal/{driver,gitx,journal,runtime}/doc.go`.
- Retain and, only where RC3 binding requires it, narrowly update
  `tools/batonassets/**`; add or rewrite `tools/batongolden/**`.
- Do not change `docs`, unrelated refs, any path outside the admitted W0
  include set, or `.baton/releases/**` in the product candidate. Baton actions
  alone own later design, proof and status record commits.

# Consequential decisions and risks

- **Two heads, two purposes:** `0eccb99a` is the immutable release/product
  materialization; `e5e07801` is the bound post-review owner head for this
  revision and later record actions advance from it. Treating the owner head
  as if it must remain equal to the release head would reject valid Baton
  design/review history.
- **New control lineage:** `refs/heads/release-wt/sworn-v0.3.0` at
  `ade672c77a7861748767424290031ed3d0f9de6d` and the old
  `refs/heads/track/sworn-v0.3.0/**` refs remain immutable archaeology. Their
  plan digest
  `sha256:cf6e9103219c76a12834fbaf1eb9da8576b765dfe0602ebe416e756ed8ca10f8`
  remains old-lineage identity. W0 neither deletes nor rewrites those refs and
  copies no old record, approval, design, proof, verdict, or merge receipt.
- **Inertness is proved, not trusted:** require both equal non-record product
  identity and a record-only delta for every owner-chain commit.
- **Product-only reset:** the reset removes the v0.2 kernel rather than
  refactoring it. Recovery remains through Git history; no legacy package is
  hidden behind a new seam.
- **One compiled Baton authority:** the regenerated `internal/baton` snapshot
  is the only product-embedded Baton package. Old RC2 snapshots and
  `internal/protocol/snapshot` are not fallback inputs.
- **Published bytes, not working-copy bytes:** asset generation reads exact Git
  blobs from the peeled RC3 commit with replacements, inherited Git authority,
  implicit fetches, symlinks, non-blobs, oversized input and mutable
  worktree/index state rejected.
- **No speculative future APIs:** W1–W3 ownership packages are inert
  documentation-only seams. W0 does not pre-implement authority, driver,
  journal or runtime behavior.
- **Reproducibility boundary:** official builds use `CGO_ENABLED=0`,
  `GOFLAGS=-buildvcs=false`, explicit `-buildvcs=false`, and `-trimpath`.
  Separate product roots and fresh `GOCACHE`, `GOMODCACHE` and `GOPATH` use
  `GOWORK=off`, `GOTOOLCHAIN=local`, `GOPROXY=off`, and `GOSUMDB=off`.
  Temporary roots and `vcs.*` settings are forbidden in the binary.
- **External measurement is evidence only:** commit `551449df` is pinned by
  exact blob bytes but is outside target ancestry. Its claims gain no authority
  until independently reproduced from the exact product inputs above.

# Evidence plan

- **A-W0-base:** record exact target commit, tree and ordered parents; prove
  install `67d792bb` and materialization `0eccb99a` are the expected
  record-only commits; bind the exact plan and approval digests, materialization
  base and empty dependencies. Bind this revision to owner head `e5e07801`.
  At implementation start, capture the exact post-PROCEED owner head and prove
  its complete first-parent chain from `0eccb99a` is record-only, product-inert
  and valid while release remains `0eccb99a`. Do not require owner equality
  with the release/materialization head.
- **A-W0-lineage:** capture exact old release/track refs and status bytes before
  and after implementation; require identical OIDs and bytes. Scan the
  candidate and new RC3 namespace for copied RC2 plan, approval, status,
  design, proof, verdict and merge-receipt bytes. The old product patch is
  evidence input only; the fresh candidate contains no `.baton/releases/**`
  change.
- **A-W0-assets:** independently reconstruct and compare all five RC3
  identities. Run the snapshotter twice into separate empty outputs and compare
  complete manifests, modes and bytes with each other and the committed closed
  14-file snapshot. Admission mutation tests reject every identity, binding,
  missing/extra asset, size, digest, mode and path change. `sworn version
  --json` and `batongolden verify` report the same canonical RC3 identity.
- **A-W0-budget-authority:** verify the external measurement blob's exact
  commit/path/SHA-256 without treating it as authority; independently recreate
  both product patches in both orders from the pinned W0/W1/W2 inputs; compare
  patch, path-list, composed-product and category-manifest digests; then
  regenerate the exact 29-row path/count manifest above and require its digest,
  row count and 10,464 sum. Recompute the approved raw plan and approval bytes
  and verify they contain the explicit 15,000-line supersession. Derived
  18,064/19,264 ceilings are admissible only after every reproduction gate
  matches; otherwise stop for corrected evidence and reapproval.
- **Mandatory focused check:**
  `GOFLAGS=-buildvcs=false go test -count=1 ./tools/batonassets/... ./tools/batongolden/... ./cmd/sworn/...`.
- **Internal Baton admission gate:**
  `GOFLAGS=-buildvcs=false go test -count=1 ./internal/baton/...`, including
  exact RC3 identity/binding and mutation-negative tests.
- **Full gates:**
  `GOFLAGS=-buildvcs=false go test -count=1 ./...`;
  `GOFLAGS=-buildvcs=false go test -count=1 -race ./...`;
  and `GOFLAGS=-buildvcs=false go vet ./...`.
- **Formatting, module and diff gates:**
  `bash -o pipefail -c 'unformatted="$(git ls-files -z -- "*.go" ":(exclude,top).baton/releases/**" | xargs -0 -r gofmt -l)"; test -z "$unformatted"'`;
  `GOWORK=off GOFLAGS=-buildvcs=false go mod tidy -diff`;
  and product-only `git diff --check`.
- **Official build gate:**
  `CGO_ENABLED=0 GOFLAGS=-buildvcs=false go build -mod=readonly -buildvcs=false -trimpath -o /tmp/sworn-v0.3.0 ./cmd/sworn`.
  Inspect build information and symbols for no `vcs.*`, temporary roots, or
  legacy package links.
- **Reproducibility and isolation gate:** run
  `TestTwinProductBuildsIgnoreRecordOnlyHistory`,
  `TestProductCopyAndArchiveExcludeBatonRecords`,
  `TestModuleHasOnlyTheAdmittedPackageSet`, and
  `TestBuiltBinaryHasNoLegacySymbolsOrVCSSettings` verbosely. Then build from
  two separately materialized product-only copies with separate fresh caches,
  one containing an additional commit confined to `.baton/releases/**`;
  require byte-identical binaries and normalized product packages, unchanged
  product manifests, absent record paths, and no temporary-root bytes.
- **Candidate closure:** compare every changed product path with the admitted
  include set, require a clean product-only candidate commit based on
  `implementation_start_owner_head`, record its exact commit, ordinary tree
  and product-tree digest, and retain raw check outputs by digest for the fresh
  Verifier. Any failed gate, hidden record input, product change after the
  candidate, or old-lineage ref movement stops.

# Revisions

Revision 2 responds narrowly to Captain `REVISE` invocation
`codex:/root/w0-design-captain/sworn-v0.3.0-baton-rc3/T0-admission/W0-reset-admission/review/1`
over design
`sha256:bee4fbe46a16bde126b1e95776096bae68b7a9e0adc90dbce3294d2722802c61`.
It separates immutable materialization/release product base `0eccb99a` from the
record-only owner chain at `e5e07801`, adds a then-current post-PROCEED owner
binding and complete intervening inertness proof, and replaces the asserted
budget aggregate with pinned external bytes, exact W0/W1/W2 composition inputs,
a complete classifier, and a canonical 29-row path/count manifest. All other
RC3 identity, reset, scope, isolation, lineage and check content is retained.
