# Approach

W0 is a physical source reset and immutable-package admission, not a partial
port of the v0.2 engine.

1. Reassert the approved target binding before implementation: target
   `2c9ce0493971e0e833d4dec6c562b030315e33c9`, ordinary tree
   `a10d213da750ece28a6dc066e2170c76fc959def`, ancestor
   `c32d6846a98aef59a33d0a4bca89a4fde434a1d1`, plan
   `sha256:cf6e9103219c76a12834fbaf1eb9da8576b765dfe0602ebe416e756ed8ca10f8`
   and approval
   `sha256:1cf79386fa391d93c19e03abe322e0425455967bec5a03785a046356b8aa2a0c`.
   The lineage assertion belongs in delivery evidence, never in product
   constants.
2. Delete the v0.2 production packages
   `internal/{adapter,app,board,buildinfo,config,control,effects,engine,executor,policy,producer,protocol,repo,store,workspace}`.
   Reduce `go.mod` to the module and Go version and delete `go.sum`. No
   dependency is retained for future work.
3. Retain the reviewed `internal/baton/release.json`,
   `internal/baton/snapshot/**` and `tools/batonassets/**`. Add one small
   `internal/baton` production package which embeds only those release bytes,
   strictly validates the closed release and manifest JSON, checks the exact
   ordered 14-file inventory and every size/digest, and exposes a copied
   release identity plus read-only asset access. Admission requires:
   annotated tag object `b80f3e27f0e0a71a4883bcc282e4843e085f0e04`,
   commit `890238ef063bb53cf51fb3359f1ff527f14846c6`, tree
   `97513f3e6f798f3ad04d5b510a49496a605a8ea4`, archive
   `sha256:968088ede0c3bfbafb0a9372d3abbf6853556cc2a6e85ffc25615d6332977e63`,
   generated support
   `sha256:676c630c6a4ef3f752d604efaa5e51958adec0d8580b74cec7fb1e689b1d3436`,
   manifest
   `sha256:74243a42dcbaa65eadac161126e9cfa8710803a136b827dd7001f5648459986c`
   and 50,387 embedded bytes. Validation has no network, Git, Node, home
   directory, checkout or Baton record-root input.
4. Establish only package ownership seams in
   `internal/{runtime,journal,gitx,driver}/doc.go`. They contain no placeholder
   lifecycle, interfaces or model behavior. W1 owns Baton/Git actions, W2 owns
   the driver contract, and W3 owns runtime and journal behavior.
5. Keep `cmd/sworn` deliberately small. `help` and `version [--json]` are the
   only supported commands. `version` first admits the compiled Baton package
   and reports Sworn version/state plus the exact Baton identities above.
   `run`, `board`, `__executor-shim` and every other name use the same
   unimplemented-command path without inspecting further arguments or paths.
   Sworn Git commit stamping is removed; product identity and provenance remain
   distinct.
6. Add `tools/batongolden` as a development-only `verify` command over the
   compiled admission lock with canonical JSON and focused tests. W1, not W0,
   adds vectors generated independently from the pinned JavaScript reference.
   The existing snapshotter remains an explicit offline upgrade/check tool; no
   build, test or generation step silently rewrites admitted assets.
7. Tighten product-only builds and CI. Exact Baton bytes are marked `-text
   diff`; `.baton/releases` remains export-ignored and is excluded from every
   product copy, archive, walker and format input. Two separate product copies
   use separate fresh `GOCACHE`, `GOMODCACHE` and `GOPATH`, with `GOWORK=off`,
   `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, `CGO_ENABLED=0`,
   `-buildvcs=false` and `-trimpath`; a record-only commit in one copy must not
   change a binary byte. CI runs the approved focused check, full tests, race,
   vet, formatting, module-tidiness and the official build.

# Surfaces

- Delete: `go.sum` and every legacy package directory named in step 2.
- Rewrite: `go.mod`, `.github/workflows/ci.yml`, `AGENTS.md`, `README.md` and
  `cmd/sworn/{main.go,main_test.go,binary_integration_test.go}`.
- Add: `internal/baton/{assets.go,assets_test.go}`,
  `internal/{runtime,journal,gitx,driver}/doc.go`, and
  `tools/batongolden/{main.go,main_test.go}`.
- Retain byte-for-byte:
  `internal/baton/{release.json,snapshot/**}`.
- Retain functionally: `tools/batonassets/{main.go,main_test.go}`.
- Amend `.gitattributes` only to preserve exact bytes under
  `internal/baton/release.json` and `internal/baton/snapshot/**`, while keeping
  the Baton record-root export exclusions.
- Do not touch `docs`, `LICENSE`, `.gitignore`, unrelated refs, or any path
  outside the approved W0 include set. Baton action commits alone update
  `.baton/releases`.

# Consequential decisions and risks

- **Incompatible cut:** v0.2 source is removed rather than refactored. It
  remains recoverable from tag `v0.2.0`, branch `legacy/v0` and Git history;
  no old package is copied into the new seams.
- **One protocol authority:** `internal/protocol/snapshot` is deleted with the
  legacy package. The 14-asset `internal/baton` snapshot is the only compiled
  Baton authority.
- **Digest-pinned publication:** Baton RC2's GitHub release is published but
  not immutable. Runtime trusts only checked-in, digest-verified bytes. The
  implementation proof independently rechecks the annotated tag, archive
  header/digest and generated-support digest; future upgrades require a new
  reviewed admission.
- **No speculative APIs:** the four future seams are documentation-only. This
  avoids locking W1–W3 into placeholder types while making package ownership
  explicit.
- **No dormant SQLite:** the current SQLite graph is removed. W3 may add one
  dependency only when its journal behavior and failure tests arrive.
- **Fail-closed startup:** a malformed embedded package makes `version` fail
  and no command becomes operational. Mutation-negative tests cover each
  identity and inventory boundary.
- **Archaeology refs remain visible:** W0 does not delete or rename old local
  release refs. Evidence validates the named `sworn-v0.3.0` projection and
  does not claim that the legacy all-release catalog is globally valid.
- **Golden independence:** W0 verifies package admission only. W1-generated
  lifecycle vectors must come from the pinned released JavaScript reference,
  never from the Go implementation being tested.

# Evidence plan

- **A-W0-base:** capture the exact target commit/tree, ancestry, merge parents,
  installed plan/approval digests, materialization base, absent T1–T6 refs and
  clean W0 start. Scan the product candidate for the rehearsal identity, prior
  plan digest and prior approval marker. Record the candidate's exact commit,
  ordinary tree and product tree in the implementation proof.
- **A-W0-assets-product:** production admission tests validate the exact tag,
  commit, tree, archive, support, manifest, 14 paths, 50,387 bytes, operation,
  template and contract bindings, plus mutation-negative cases. The
  snapshotter twice reproduces the committed snapshot from the pinned commit.
  `batongolden verify` emits the same canonical identity.
- **A-W0-assets-product / source cut:** an exact package allowlist proves the
  module contains only `cmd/sworn`, `internal/{baton,runtime,journal,gitx,driver}`
  and `tools/{batonassets,batongolden}`. Built-symbol and source-inventory
  canaries reject every legacy package. FIFO/marker canaries prove retired
  command names consume no arguments or paths.
- **A-W0-assets-product / reproducibility:** twin builds from separate
  product-only copies and fresh caches are byte-identical; neither temporary
  root occurs in the binary; build info contains no `vcs.*` setting; Git
  archive and copy inventories exclude `.baton/releases`.
- **Required and supporting checks:**
  `GOFLAGS=-buildvcs=false go test ./tools/batonassets/... ./tools/batongolden/... ./cmd/sworn/...`;
  `GOFLAGS=-buildvcs=false go test ./...`;
  `GOFLAGS=-buildvcs=false go test -race ./...`;
  `GOFLAGS=-buildvcs=false go vet ./...`;
  product-only `gofmt`, `git diff --check`, `go mod tidy -diff`; and
  `CGO_ENABLED=0 GOFLAGS=-buildvcs=false go build -buildvcs=false -trimpath ./cmd/sworn`.
- **Scope and record isolation:** compare changed paths with the approved
  include list, require a product-only candidate commit, then demonstrate that
  the later Baton proof/status commit preserves its product-tree identity.

# Revisions

Initial plan-bound W0 design. It replaces pre-plan architecture captures as
delivery authority; those captures remain archaeology only.
