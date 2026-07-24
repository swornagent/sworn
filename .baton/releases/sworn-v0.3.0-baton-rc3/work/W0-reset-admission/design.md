# Approach

Invocation:
`codex:/root/w0-design-implementer/sworn-v0.3.0-baton-rc3/T0-admission/W0-reset-admission/design/1`.

W0 is a fresh RC3-bound physical product reset and immutable-package
admission. It is not a continuation of the RC2 control lineage and it does not
cherry-pick an RC2 candidate.

1. Bind implementation to the authoritative materialized state before any
   product edit:
   - target `refs/heads/release/v0.3.0` was, immediately before installation,
     commit `2c9ce0493971e0e833d4dec6c562b030315e33c9`, tree
     `a10d213da750ece28a6dc066e2170c76fc959def`, with parents
     `c32d6846a98aef59a33d0a4bca89a4fde434a1d1` then
     `045d6e9c56c5da523f5d8a149e29fefeb7c56f17`;
   - install commit
     `67d792bb09eb26e79ea3f8bff628d25caeaccab6` is the exact child of that
     target and changes only the new plan and its initial statuses;
   - materialization commit
     `0eccb99abb6a49497ae2e1dc89f9d09cf115a4c1` is the exact child of the
     install, changes only W0's status, and is the current head of both
     `refs/heads/release-wt/sworn-v0.3.0-baton-rc3` and
     `refs/heads/track/sworn-v0.3.0-baton-rc3/T0-admission`;
   - the raw plan digest is
     `sha256:66dd2c09b538b4eb41783128b3c4d110d10d04124a15f46b2d354858b2409d74`
     and its protected approval digest is
     `sha256:35415561708d3421c1272fbebada8e56748166077313ba1b540ec96736bc7602`.
   Any mismatch, dirty product start, foreign authority, unexpected RC3 owner
   ref, or changed approval stops before implementation.
2. Prove the install/materialization transition is behaviorally inert before
   using it as the W0 base. The materialized commit must have the same
   non-record product identity as the certified target/bridge product, and
   every changed path from that product identity must be confined to
   `.baton/releases/**`. Missing evidence, an ambiguous product identity, any
   non-record change, or any unconditional `inert` policy result stops. This
   classification is implementation evidence, not a product constant.
3. Reconstruct the reset as a new product delta on the RC3 materialization.
   The old W0 product delta from
   `b58fdab7ea912e41c7097d91540035b043358205` to
   `fb6e56b7900bc55055364ee3033f24ef9cf02551`, product-patch digest
   `sha256:be8c3c41ac356b19eda82f5de8fe81965f37dbf02710d26a44555a456ec7d93a`,
   may be inspected or replayed mechanically only after excluding
   `.baton/releases/**`. Its commit, design, proof, approval, statuses,
   verdicts and receipts are never copied or cited as current authority.
4. Remove the v0.2 production packages
   `internal/{adapter,app,board,buildinfo,config,control,effects,engine,executor,policy,producer,protocol,repo,store,workspace}`.
   Reduce `go.mod` to the module and approved Go version and remove `go.sum`;
   W0 retains no dormant dependency for later work.
5. Establish the reset product as exactly `cmd/sworn`,
   `internal/{baton,driver,gitx,journal,runtime}`, and
   `tools/{batonassets,batongolden}`. The four future packages contain
   ownership documentation only. `cmd/sworn` exposes only `help` and
   `version [--json]`; every delivery, board, executor-shim, or unknown command
   refuses before consuming a path, configuration, repository, database, or
   model input.
6. Generate the admitted asset snapshot only from the published RC3 commit.
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
7. Preserve the reviewed closed, path-ordered 14-file product inventory, but
   regenerate every selected byte, manifest entry, size, mode and digest from
   the exact published RC3 commit. Rewrite the admission lock, compiled
   identity, tests, version output and golden verifier to RC3. Runtime
   validation has no network, mutable checkout, Git, Node, home directory,
   Baton record, or old snapshot input. The JavaScript package is a
   development oracle only.
8. Keep `.baton/releases` control-only. Exclude it explicitly from product
   copies, archives, file walkers, formatter input, checks, model workspaces,
   candidates, builds and release packages. The final implementation candidate
   is product-only. A later Baton proof/status commit must preserve the exact
   candidate product-tree identity.
9. Preserve the approved budget authority as control evidence, not a runtime
   constant. Fresh approval of the exact raw RC3 plan ratifies the measured
   W0+W1+W2 baseline of 10,464 non-generated runtime-production physical lines
   across 29 `cmd/**` and `internal/**` files, the normal combined ceiling of
   18,064, and the two proof-backed Captain bands through 19,264. No issue-body
   target, conversation summary, old plan, or RC2 approval substitutes for the
   exact approved bytes.

# Surfaces

- Delete `go.sum` and the legacy `internal` package directories listed in
  Approach step 4.
- Rewrite `.gitattributes`, `.github/workflows/ci.yml`, `AGENTS.md`,
  `README.md`, `cmd/sworn/**`, and `go.mod`.
- Add or rewrite `internal/baton/{assets.go,assets_test.go,release.json,snapshot/**}`.
- Add `internal/{driver,gitx,journal,runtime}/doc.go`.
- Retain and, only where RC3 binding requires it, narrowly update
  `tools/batonassets/**`; add or rewrite `tools/batongolden/**`.
- Do not change `docs`, unrelated refs, any path outside the admitted W0
  include set, or `.baton/releases/**` in the product candidate. Baton actions
  alone own later design, proof and status record commits.

# Consequential decisions and risks

- **New control lineage:** `refs/heads/release-wt/sworn-v0.3.0` at
  `ade672c77a7861748767424290031ed3d0f9de6d` and the old
  `refs/heads/track/sworn-v0.3.0/**` refs remain immutable archaeology. Their
  plan digest
  `sha256:cf6e9103219c76a12834fbaf1eb9da8576b765dfe0602ebe416e756ed8ca10f8`
  remains old-lineage identity. W0 neither deletes nor rewrites those refs and
  copies no old record, approval, design, proof, verdict, or merge receipt.
- **Inertness is proved, not trusted:** a host callback that simply returns
  `inert` can mask a product mutation. Require both equal non-record product
  identity and a structurally record-only delta before accepting an
  installation/materialization transition.
- **Product-only reset:** the reset deliberately removes the v0.2 kernel
  instead of refactoring it. Recovery remains through Git history; no legacy
  package is hidden behind a new seam.
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
- **Budget scope:** the approved 10,464-line measurement is a downstream
  composed W0+W1+W2 baseline, not permission to add unrelated W0 behavior.
  Any changed outcome, authority, product scope, safety boundary or ownership
  requires a newly approved plan.

# Evidence plan

- **A-W0-base:** record exact commit, tree and ordered parents of
  `2c9ce0493971e0e833d4dec6c562b030315e33c9`; prove install
  `67d792bb09eb26e79ea3f8bff628d25caeaccab6` is its record-only child and
  materialization `0eccb99abb6a49497ae2e1dc89f9d09cf115a4c1` is the next
  record-only child; bind the exact plan and approval digests, current owner
  and authority refs, materialization base, empty dependencies, clean start,
  and absence of every other RC3 track ref. Prove equal non-record product
  identity across target, install and materialization.
- **A-W0-lineage:** capture the exact old release/track refs and their status
  bytes before and after implementation; require identical OIDs and bytes.
  Scan the candidate and new RC3 record namespace for copied RC2 plan,
  approval, status, design, proof, verdict and merge-receipt bytes. The old
  product patch is evidence input only; the candidate must be freshly based on
  the RC3 T0 materialization and must contain no `.baton/releases/**` change.
- **A-W0-assets:** independently reconstruct and compare all five RC3
  identities above. Run the snapshotter twice into separate empty outputs and
  compare complete manifests, modes and bytes with each other and with the
  committed closed 14-file snapshot. Admission mutation tests independently
  reject every identity, binding, missing/extra asset, size, digest, mode and
  path change. `sworn version --json` and `batongolden verify` must report the
  same canonical RC3 identity. Prove compiled admission does not read network,
  Git, Node, a checkout, home directory or `.baton/releases`.
- **A-W0-budget-authority:** retain the exact protected approval reference and
  digest, recompute the raw plan digest, and verify the approved bytes contain
  the explicit 15,000-line supersession plus the 10,464/29 measured baseline,
  18,064 normal ceiling and 19,264 Captain-band ceiling. Verify neither product
  source nor binary embeds the approval as runtime policy.
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
  include set, require a clean product-only candidate commit, record its exact
  commit, ordinary tree and product-tree digest, and retain raw check outputs
  by digest for the fresh Verifier. Any failed gate, hidden record input,
  product change after the candidate, or old-lineage ref movement stops.

# Revisions

Initial RC3-plan-bound W0 design. It uses the old W0 design, proof and product
patch only as archaeology; replaces all RC2 identity and authority with the
fresh admitted RC3 plan; adds the durable preflight's strict inertness
correction; and maps all four current W0 acceptance identifiers to fresh
evidence.
