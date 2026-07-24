# Sworn S0 Baton RC2 asset scope

Date: 2026-07-24
Status: Captain-revised release closure admitted; snapshot verified; runtime not started
Sworn design head: `f1d90e902238040eaaf175e82aff69fb3b32f1a4`
Sworn admission base: `9e6a81da0034b578e98d4bd18354f66c2d5b12dc`
Baton annotated tag object: `b80f3e27f0e0a71a4883bcc282e4843e085f0e04`
Baton peeled commit: `890238ef063bb53cf51fb3359f1ff527f14846c6`
Baton peeled tree: `97513f3e6f798f3ad04d5b510a49496a605a8ea4`
Baton release archive SHA-256: `968088ede0c3bfbafb0a9372d3abbf6853556cc2a6e85ffc25615d6332977e63`
Baton support-package digest: `sha256:676c630c6a4ef3f752d604efaa5e51958adec0d8580b74cec7fb1e689b1d3436`

## Decision

S0 embeds only the Baton bytes that define its runtime-facing identity. The
JavaScript reference, fixtures, and portable-kit runners remain development
and CI evidence. Sworn does not ship a second copy of Baton's implementation
or make a generated manifest a competing runtime authority.

The publication gate is now satisfied. `internal/baton/release.json` records
the immutable release identity and `internal/baton/snapshot/manifest.json`
records the closed generated asset inventory. This work unit adds admission
data and its checks only; it does not implement a Baton transition, scheduler,
journal, driver, startup path, or database behavior.

## Production embed

The compiled inventory is closed to these 14 path-preserving blobs, sorted
lexicographically. The published scope is 50,387 bytes.

| Path | Released SHA-256 |
| --- | --- |
| `VERSION` | `0c654c00f94741d78169de333d4d9e866be0667b41f9c54cafe3c6b700b15a43` |
| `baton/PROTOCOL.md` | `8e6eb570b2eeb27d84b64fb182d71f7591995ba6a1318500769a9db9144eca5a` |
| `conformance/engine-adapter.md` | `dbb3d5c3d22b79a3da4e98fb96f4db1eaa16d2bda04567f4d181bda001705450` |
| `conformance/manifest.json` | `3bf2535cc1e92ac132576dd0c646062b9d33a0ba33201823f1d92409a6387a92` |
| `operations/baton-design-review.md` | `ead3a7d0e22a794ca5430fdbaca5c29f3ae5d5f6fad7c102d1f2bd878f28e356` |
| `operations/baton-implement.md` | `2444bead5b1a32188003ce515ac8862bd04d373b740bd89646a86ac5341c2f88` |
| `operations/baton-merge.md` | `94b8fb6026c903569cd375cafd11d27868759072dde256265556c710387ae62c` |
| `operations/baton-plan.md` | `e5c3ace4177cb10c9b0d3b5e569aa7cbe43bfdb3b7f4a17071a925a5ba3b77d3` |
| `operations/baton-verify.md` | `a6f0e9b9bf95cb59e5030b7f95f72d8d3545b52ef771c7d20e7be44a20e45bed` |
| `reference/driver/contract.md` | `660a1ce7b44cdd150d902fddc80043814b5d6dc4fc28c29a7daed9973abe60bf` |
| `schemas/work-status-v1.json` | `70219641e954afefa35fe20cf702eeabac3ce7c9290d09d5ce29082bf4a497c1` |
| `templates/design.md` | `10e4a2097bffab99464454f9389b5c72f8e3cb12680943ae945401e7b0ebc146` |
| `templates/plan.md` | `7caac5f8fc8baccacb2787902c1f86d97a92728db0a42b63a4674444886a276c` |
| `templates/proof.md` | `0bc58a34505859792ac734ff50a23420ad9f24e0227aee19c4e71d84ef9fd225` |

The later startup implementation must recompute every digest and require the
exact closed inventory. The checked-in release identity separately binds:

- annotated tag object, peeled commit, and peeled tree;
- deterministic published package archive digest;
- five operation-document digests and versions;
- the three canonical action-template digests;
- schema and conformance-manifest digests; and
- generated support-package digest admitted from release evidence.

The generated manifest is itself bound as
`sha256:74243a42dcbaa65eadac161126e9cfa8710803a136b827dd7001f5648459986c`.
The admission test treats the 14 paths, regular non-executable file shape,
sizes, bytes, operation names and versions, schema bindings, manifest digest,
and publication identity as golden metadata. The generator replay separately
proves the tagged Git blob modes. Checkout umask or shared-repository group
write bits are host policy, not Baton identity.

### Captain revision

The first unpushed local Implementer candidate contained the originally
approved 11-blob inventory. A fresh Captain returned `REVISE`: the authored
operations require `templates/plan.md`, `templates/design.md`, and
`templates/proof.md`, so those templates are part of the self-contained engine
contract rather than optional explanatory content. The candidate had not been
pushed or integrated. Implementation reopened, regenerated the
path-preserving snapshot from the same tagged commit, and expanded the closed
inventory by exactly 3,013 bytes. No runtime code entered the repair.

### Canonical operation paths

The admission handoff initially abbreviated the operation paths as
`operations/{plan,implement,design-review,verify,merge}.md`. None of those
paths exists in the tagged commit. The published generated-adapter manifest and
Git tree both name
`operations/baton-{plan,implement,design-review,verify,merge}.md`. Admission
therefore preserves those five real path names byte-for-byte; it creates no
aliases and performs no content transformation.

## Development-only closure

Golden-vector generation may execute these exact reference modules, but they
are never embedded in or executed by the production binary:

- `reference/records/actions.mjs`;
- `reference/records/git.mjs`;
- `reference/records/records.mjs`; and
- `reference/records/transition.mjs`.

The portable conformance corpus includes:

- all raw, valid-status, invalid-schema, and invalid-semantic fixtures named by
  `conformance/manifest.json`;
- `conformance/fixtures/driver/**` and
  `reference/driver/fake-driver.mjs`;
- `conformance/fixtures/board/**` and `reference/board/{oracle,terminal,web}.mjs`;
  and
- all five operation documents consumed by the fake driver.

The exact release archive remains authoritative for the Python checker, pinned
requirements, overhead baseline, manifest-named Node suites, helpers,
installers, generated adapters, and generator scripts. CI runs that archive
directly instead of copying its transitive test tree into Sworn.

## Intentional omissions

Production does not embed:

- any `reference/**/*.mjs` implementation;
- fixtures or generated golden vectors;
- Baton tests, Python, installers, adapter generators, generated Skills,
  examples, baselines, or other explanatory documents; or
- raw `adapters/generated/generated-manifest.json` bytes.

The generated adapter manifest is release-admission evidence. Sworn validates
it and copies its declared identity into the compiled release identity; it does
not consult the manifest as a mutable runtime authority. Its support-package
digest is not the published archive digest.

## Admission evidence

| Gate | Exact evidence | Result |
| --- | --- | --- |
| Annotated publication | local and remote `refs/tags/v1.0.0-rc.2` resolve to tag object `b80f3e27…`, commit `890238ef…`, tree `97513f3e…` | PASS |
| Published-tag protection | active GitHub tag ruleset `19678047`, `Protect published Baton version tags`, targets `refs/tags/v*`, blocks update and deletion, has no bypass actors, and reports current-user bypass `never`; Captain re-audit returned `PROCEED` | PASS |
| Release asset | GitHub prerelease published `2026-07-24T09:46:02Z`; downloaded `baton-1.0.0-rc.2.tar.gz` matches its checksum and `sha256:968088ed…`; its Git archive header embeds `890238ef…` | PASS |
| Production snapshot | two independent `tools/batonassets snapshot` runs and the checked-in 14-blob output are byte-identical; every emitted blob is byte-identical to its exact tagged Git blob | PASS |
| Portable corpus | tagged checkout: Python reports 7 strict JSON cases, 1 Draft 2020-12 schema, 2 positive and 6 negative fixtures; Node reports 132/132 tests passing | PASS |
| Generated support | tagged `adapters/generated/generated-manifest.json` reports package version `1.0.0-rc.2`, operation version `baton.operation/v1`, and package digest `sha256:676c630c…` | PASS |
| Sworn admission checks | `GOFLAGS=-buildvcs=false go test ./...`, `GOFLAGS=-buildvcs=false go vet ./...`, formatting, and diff checks validate the closed release metadata and generated snapshot | PASS |

In this shared multi-worktree checkout, Go VCS stamping fails with Git exit
128, including inside the two existing built-binary tests. The
repository-standard `GOFLAGS=-buildvcs=false` disables build metadata stamping
only; those tests still compile and execute their binaries and all packages
pass.

The release archive and the exact tagged checkout remain authoritative for the
development-only corpus. Checked-in Go action/transition vectors, portable
fixtures through the future Go facade, startup self-check, and old-database
rejection are still `NOT RUN`; they belong to the subsequent R0/R1 runtime
units and cannot inherit `PASS` from this admission.

The tag-ruleset observation is human publication evidence only. Runtime
admission is pinned to immutable Git and digest identities above and does not
call or trust mutable GitHub API policy state.
