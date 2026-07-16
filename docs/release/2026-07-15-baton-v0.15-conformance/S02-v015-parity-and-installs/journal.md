# Journal — S02-v015-parity-and-installs

## 2026-07-16 — Implementer design checkpoint

- Transitioned `planned -> design_review`; no production code, tests,
  vendoring, pin update, archive write, or Codex/Claude installation mutation
  was performed.
- Planning-authority bootstrap oracle adapter explicitly authorised by the
  orchestrator: invoked the installed pre-cutover read-only oracle as
  `/home/brad/go/bin/sworn board --release
  2026-07-15-baton-v0.15-conformance --json`, normalized only its legacy
  top-level `{release, tracks}` envelope, and derived S02 as the second slice of
  `T1-foundation`. The branch and worktree came only from exact track-mode
  convention and matched `git worktree list` at clean pushed head
  `0396d5d550acc1bfe8d0e5ccd8db681d4018f3f2`; predecessor S01 is `verified`.
- Governing upstream sources were read from the clean Baton `v0.15.1` checkout
  at `/home/brad/projects/baton`, exact commit
  `3fb4d275ae8a151f6287e7b9279d71628b12eea0`: `commands/implement-slice.md`,
  `baton/role-prompts/implementer.md`, `baton/track-mode.md`, both installer
  scripts, and the exact `baton/llm-checks/README.md` plus
  `maintainability-review.md` used by the planning-authority Gate-8 adapter.
- Readiness evidence: S02 status history preserves null `start_commit`, the
  empty pending cycle-0 maintainability ledger, and null adjudication;
  `bin/sworn lint ac`, `bin/sworn lint trace`, `bin/sworn reqvalidate`,
  `bin/sworn designfit`, and `bin/sworn specquality` all passed. The four S02
  ACs are concrete, traced EARS conditions.
- Exact-source evidence: the tag resolves to the C-01 commit; the normative Git
  archive operation reproduced 78 entries, SHA-256
  `27d5021cb3ec258a7fd7a5feb6eed92968be0e6cb439e2951da7c6b368e0ca15`,
  and Git blob `39ae650dfe0282b0fa8bda14e1a01e7084077702`. The current read-only
  `bin/sworn baton diff /home/brad/projects/baton` reported the expected 17
  mapped v0.13.1-to-v0.15.1 content divergences before S02 execution.
- Design-review ownership pin: `internal/adopt/adopt.go` owns the only relevant
  `go:embed` filesystem but is absent from every T1 touchpoint, so the shipped
  binary cannot consume the required tar within the current boundary.
- Design-review ownership pin: current `internal/baton/diff.go` and
  `internal/baton/content.go` cannot generate or compare the archive through the
  ordinary mapping, yet neither is an S02 touchpoint. If `baton diff` is an
  applicable C-01 archive proof surface, the planner must add an explicit owner.
- Design-review maintainability pin: archive extraction, exact installer-output
  generation, and three-root rollback/recovery are distinct responsibilities,
  but the fixed touchpoint list provides no dedicated production helper. The
  Captain should reject hiding them in unrelated mapping/manifest files and ask
  the Coach whether bounded new internal files are required.

## 2026-07-16 — Implementer design revision after Coach replan

- Re-read the exact Baton v0.15.1 Implementer contract, used the explicitly
  authorised pre-cutover board adapter, normalized its legacy top-level result,
  and confirmed S01 remains `verified` before S02 in `T1-foundation`. The
  conventional worktree and branch matched `git worktree list`; the worktree
  started clean at authoritative Coach replan merge `5e4a7b3410e975ce7507a13291d3411749bd680e`.
- Revised `design.md` against the amended spec, intake, normative
  clarifications, prior Captain `NEEDS_COACH` review, S01 live proof/status, and
  current code. No production code, test, vendored byte, archive, VERSION pin,
  or Codex/Claude installation was changed; S02 remains `design_review` with
  null `start_commit` and pending cycle-0 maintainability authority.
- Captain pins 1–4 now trace to the Coach-ratified owners:
  `internal/adopt/baton_archive.go` is the sole compile-time archive embed with
  no filesystem fallback; `internal/baton/diff.go` owns public archive parity
  while `content.go` remains excluded; `vendor.go` and
  `vendor_transaction.go` add validated archive bytes to mapped bytes plus
  VERSION in one repository transaction/recovery set; and focused
  `installer_archive.go` / `install_transaction.go` own native generation and
  three-root rollback/recovery while `cmd/sworn` stays thin.
- Captain pins 5–7 now trace explicitly: both native trees derive and assert
  all eight tagged commands including `design-review.md`; design.md mirrors all
  five Type-1 status decisions plus the path-only Type-2 default; exact
  normative JSON bytes and independent complete Codex and Claude script-oracle
  parity are mandatory, fail-closed prerequisites.
- Corrected the first public TDD red to
  `cmd/sworn/baton_test.go:TestBatonDiffV015BinaryReachability`, with
  `cmd/sworn/doctor_test.go:TestDoctorAndBatonDiffV015BinaryReachability` as the
  companion built-binary doctor/sync reachability proof. The planned-file/AC
  table now covers the exact 42-file high/high `beast` boundary, which the
  Implementer confirmed in `status.json`.

## 2026-07-16 — Implementer design revision after VERSION/recovery ratification

- Re-ran the explicitly authorised pre-cutover oracle as
  `/home/brad/go/bin/sworn board --release
  2026-07-15-baton-v0.15-conformance --json`, confirmed S01 remains `verified`
  before S02, and matched the clean conventional T1 worktree/branch at merged
  Coach correction `1ee6c319505b7dd466485bff406f11828cf0507f`.
- Revised only S02 design/status/journal planning artefacts. S02 remains
  `design_review`, owner `implementer`, with null `start_commit`, pending
  maintainability/verification, and no production, test, vendored, archive,
  proof, or real-home installation write.
- Captain pin 1 now has one exact two-identity design: `5f1dd...` names only the
  upstream root `VERSION` bytes `v0.15.1` plus LF, while every participating ref
  separately resolves and compares Sworn's committed multi-line manifest blob
  and parsed tag/SHA/digest against the marker and binary.
- Captain pin 2 now fixes both exact script oracles to explicit umask `0022`,
  makes native generation explicitly enforce directories `0755` and files
  `0644`, and requires a hostile inherited-umask fixture.
- Captain pin 3 now physically resolves and rejects equal, nested, aliased,
  symlinked, special-file, and recovery-overlapping target/recovery roots before
  mutation; complete owner-only snapshots/manifest/sentinel authority is durable
  before replacement, sentinel presence is recovery-only, and kill/fault tests
  cover every publish, replace, verify, rollback/recovery, and retire boundary.
- Preserved the ratified archive embed/public-diff/expanded-repository-
  transaction owners, bounded archive/install helpers, exact eight-command
  inventory including `design-review.md`, normative JSON and dual-oracle parity,
  thin CLI adapters, and built-binary first-red/reachability boundary. Confirmed
  the high/high 42-file `beast` effort in `status.json`; fresh Captain review is
  still required before implementation.

## 2026-07-16 — Coach acknowledgement and implementation start

TL;DR The twice-replanned C-01 design is implementation-ready. 0 pins + 9 flags.

Flags (not pins): (a) the archive embed, public parity, repository transaction, bounded helper, thin-adapter, eight-command, and binary-reachability owners are explicit; (b) the upstream commit, VERSION blob, 78-entry archive SHA/blob, and command inventory reproduce exactly; (c) upstream VERSION and Sworn adopting-manifest identities remain distinct; (d) script and native modes are fixed under `umask 0022` with hostile-umask coverage; (e) four-root topology fails closed before mutation; (f) complete owner-only recovery authority is durable before replacement; (g) publish/replace/verify/rollback/recovery/retire crash states are recovery-only; (h) no active sibling collision exists; (i) no project memory or real installation state entered this review.

All nine Rule-9 decisions are acknowledged. No open design question remains. Proceed to `in_progress` and implement the design exactly as written; the fresh Verifier remains the certification backstop.

## 2026-07-16 — Coach reconciliation of v0.15 positive validator fixtures

- Paused at clean pushed implementation checkpoint
  `7b57f64fbfe9d0540737034d1794100b80aeec3b` after exact schema parity made two
  pre-v0.15 positive `slice-status-v1` test literals fail on newly required
  `start_commit` and `maintainability` members.
- Coach added only `internal/baton/validate_schema_test.go` as S02's 42nd path
  with AC-01 trace. The permitted edit is limited to the two named conforming
  literals; schema weakening and active-record migration/interpretation remain
  prohibited and assigned to later slices.
- Implementer reconfirmed the high/high `beast` rating against the live code.
  S02 remains `in_progress`; immutable `start_commit`
  `e61cb190736ee7483fb4ed1a993442b26ce3574c`, pending cycle-0 maintainability,
  and pending fresh verification are preserved.

## 2026-07-16 — Explicit AC record-sweep closure

- Added the three exact test references named by AC-01, AC-03, and AC-04 in
  `internal/baton/records_conformance_test.go`. The positive sweep authenticates
  the C-01 tag/commit/digest, distinct upstream root `VERSION` object, complete
  78-node archive identity/inventory, mapped repository, compiled adopting
  manifest, every embedded schema byte, and complete classification set.
- The AC-03 companion independently extracts and runs both tagged installers in
  isolated fixed-`0022` homes, then asks the production read-only install checker
  to compare all three resulting roots plus separate Sworn `VERSION` sentinels
  against the native trees. The existing same-named
  `installer_archive_test.go` test remains the second independent script/native
  oracle rather than being weakened or replaced.
- The fail-closed sweep mutation-tests mapped repository bytes, the adopting
  manifest/binary identity, upstream `VERSION` bytes, archive repository and
  compiled consumers, source inventory, schema classification, all three managed
  roots, and all three sentinels. All mutations fail on their applicable public
  or production read-only surface; no real installation home is read or written.
- `internal/baton/diff_test.go` and `internal/baton/vendor_test.go` remain
  byte-unchanged: their existing tests already own generic mapped-drift,
  operational-source, schema-mapping, transformed-vendor, idempotence, and
  check-only roles. The new record sweep composes those production seams against
  the exact v0.15.1 authority without duplicating generic fixture coverage.
- Focused `go test ./internal/baton -run '^TestBatonV015' -count=1` passed in
  8.599s; full `go test ./internal/baton -count=1` passed in 227.880s; and
  `git diff --check` was clean. Real-home sync remains deliberately uninvoked.

## 2026-07-16 — Coach reconciliation of the repository-wide exact-schema gate

- At clean pushed semantic checkpoint
  `60dcd6291ddbe3491e16a05d2ed98d896d714165`, `gofmt`, `make build`,
  `go vet ./...`, S02-owned packages, public binary parity, and installer tests
  passed, but `go test ./... -count=1` failed in ten older positive tests across
  `internal/gate`, `internal/run`, and `internal/state`.
- A read-only independent scope audit reproduced every failure and returned
  `BLOCKED` for a fixture-only repair. The generic reports lacked the exact
  v0.15 `check` identity, while `state.Status` had no carrier for a supplied
  `maintainability` object, so production `Read`/`Write` could not honestly
  satisfy the existing exact-schema reachability assertions.
- Coach expanded S02 from 42 to exactly 47 paths: the three affected fixture
  owners plus `internal/state/state.go` and `internal/state/state_test.go`.
  The only production authority pulled forward is an optional opaque/lossless
  supplied maintainability carrier. Defaults, transition interpretation,
  complete null semantics, requested-check matching, validation weakening, and
  active-record migration remain prohibited.
- S03 retains the complete typed maintainability and absent-versus-null carrier,
  exact-schema atomic writers, board/spec carriers, record sweeps, rendering,
  and doctor surface. S04 retains requested/emitted check equality, mismatch
  rejection, ambiguity separation, and generic-maintainability retirement.
- Release-wide EARS (107/107), trace, requirements validation, design-fit, and
  spec-quality gates passed. A first fresh ambiguity session returned no verdict
  and was discarded fail-closed; a second bounded fresh session returned
  `PASS`. Release-wt replan commit `dcd6386` was then synchronized into T1.
- S02 stays `in_progress`; immutable `start_commit`, pending maintainability,
  pending verification, and the clean semantic checkpoint are preserved.
  Effort confirmation is reset until the Implementer accepts the 47-path beast
  boundary, and a fresh Captain must review this delta before edits resume.

## 2026-07-16 — Captain acknowledgement of the five-path delta

- Fresh no-history Captain review at track head `7696c9b` returned `PROCEED`
  with zero pins. The review classified the amendment as constitutional for the
  slice and required no further Coach escalation.
- The permitted implementation is exact: optional opaque supplied
  `json.RawMessage` maintainability carrier with `omitempty`; preservation and
  absence tests; canonical `check` only in canned valid reports; and explicit
  valid start/maintainability facts in state/run fixtures.
- Production check matching, typed/null semantics, lifecycle transitions,
  defaults, migrations, `state.Write` validation/atomicity, post-write JSON
  surgery, and any install-transaction refactor remain outside this delta.
- The Implementer may resume only after reconfirming high/high `beast`. The
  semantic implementation outside the five added paths remains frozen.

## 2026-07-16 — Gate-8 bootstrap provenance amendment synchronized

- Formal maintainability preflight stopped before model dispatch at semantic
  checkpoint `60097cfa65dc39d9a0ab174be7c627fde2d3f7d5` because historical syncs
  `b8df1857…` and `7696c9bf…` fail literal section-6 release-record composition;
  an independent audit proved S01's committed frontier also consumes the same
  pre-C12 class at `36d1bd56…` and `d062d055…`.
- Planner amendment `5eaa7aea39b5ce17f483647824ca538e316a5ac8` sealed exactly those four
  merges/eight exceptional paths at manifest SHA-256
  `f9e0de63c0a5ecf15cdb6058a52166ff0a609fa0d0cf2ecdf81d7955030b1943`.
  Every entry is bound to its exact S01/S02 consumer, immutable start, exact
  review head, start-exclusive/head-inclusive first-parent interval, and
  permitted purpose; all ordinary section-6 checks remain mandatory on the
  same merge. Three audited pre-start merges remain unrecognized.
- Two fresh ambiguity reviews identified interval-binding, positive-AC, and
  same-merge ordinary-validation gaps; all were corrected. A final fresh review
  returned `PASS`, Draft 2020-12 validation passed, and all 109 release ACs,
  trace, requirements validation, design-fit, and spec-quality gates passed.
- Ordinary two-parent synchronization `d54c102f9d244a316ae3f0301bec617b2fc6c6f3`
  composed all 77 affected paths by section 6, with `index.md` regenerated from
  the combined records. No implementation bytes changed, and S02's authorized
  semantic review head remains `60097cfa65dc39d9a0ab174be7c627fde2d3f7d5`.
- The previously computed 45-included/9-excluded scope and fingerprint
  `sha256:4d58ca4027c3919073f0c51daa1f5b3ca0ab33a87414c9aebe17372f5bd338a7`
  remain candidates only. No report, status, proof, model, or real-home
  installation mutation occurred. Gate 8 must reconstruct the sealed entry and
  authorization from this clean pushed head before dispatch.

## 2026-07-16 — Implementer Gate-8 cycle-0 preflight FAIL

- Resumed from clean pushed track head
  `8ca00899f8fa45a06c099d3e5a01095229e09fd8`, preserved immutable
  `start_commit` `e61cb190736ee7483fb4ed1a993442b26ce3574c`, and kept semantic
  review head `60097cfa65dc39d9a0ab174be7c627fde2d3f7d5` unchanged.
- Reconstructed the sealed bootstrap manifest from live repository state. Its
  SHA-256 is exactly
  `f9e0de63c0a5ecf15cdb6058a52166ff0a609fa0d0cf2ecdf81d7955030b1943`;
  Draft 2020-12 schema, duplicate-key rejection, exact S02 authorization, exact
  `b8df1857` / `7696c9bf` tuples and exceptional path sets, and ordinary
  section-6 validation for every other path all passed. Post-review
  synchronization `d54c102f` also passed ordinary composition, and record-only
  `8ca0089` introduced no semantic path.
- Recomputed the canonical semantic scope from Git: 45 included paths, 9
  excluded release-record paths, no generated or lockfile exclusion, exact
  `baton-maintainability-v1` fingerprint
  `sha256:4d58ca4027c3919073f0c51daa1f5b3ca0ab33a87414c9aebe17372f5bd338a7`,
  789,223 canonical diff bytes, and presentation SHA-256
  `1a6184fc611f1eaaa63468268fb9bfcbe8af0ebf6909ad6018b12d3a3091879f`.
- Dispatched exactly one fresh no-history role-isolated Implementer preflight,
  invocation `ff6145c0-2d33-46f7-af38-2ab742516425`. The reviewer consumed only
  the untouched tagged prompt/schema, Sworn raw-response constraint, supplied
  project context, and every path in the canonical diff. A fail-closed immutable
  input-path correction occurred before diff inspection and did not change the
  invocation, scope, or fingerprint.
- The exact raw response passed duplicate-key rejection, tagged Baton schema,
  Sworn raw-response overlay, identity/scope, unique-finding, structured-
  disposition, and derived-verdict checks. It returned `FAIL`: blocking `F-01`
  requires bounded phase-specific private state within
  `internal/baton/install_transaction.go`; advisory `F-02` identifies the inert
  `prepareInstall` `needManifest` parameter.
- Persisted the complete engine-provenance report at
  `reports/maintainability/implementer-cycle-0-ff6145c0-2d33-46f7-af38-2ab742516425.json`
  in commit `bce66675862c18286dfee5d59f462ee50359abb1`; its committed blob is
  `6f95a34a0273d95f8f9d2008b1f9355f4179f7c1`. Exact Baton plus persisted
  overlay validation and all synchronization provenance checks passed.
- Applied the mandatory cycle-0 in-scope FAIL transition only:
  `maintainability.state` remains `pending`, `cycle` remains 0,
  `implementation_head` remains null, the unique compact FAIL ledger entry
  pins blocking ID `F-01`, and adjudication remains null. The next authority is
  one bounded in-scope remediation followed by exactly one Implementer closure
  review.
- No remediation, closure review, Verifier dispatch, proof mutation,
  `implemented` transition, certification, real-home access, or
  `doctor --sync-baton` execution occurred in this checkpoint.

## 2026-07-16 — Captain authorizes bounded transaction typestate remediation

- A fresh read-only Captain reviewed formal blocker `F-01`, the live 2,645-line
  transaction owner, existing fault/recovery tests, and the exact one-file
  disposition boundary. Verdict: `PROCEED`; no Coach escalation or re-slice.
- The five mechanical pins require complete `installPreflight`,
  `capturedInstall`, `stagedInstall`, and `publishedInstall` private states;
  recovery-only reconstruction from committed authority; preservation of every
  contractual fault, durability, mode, topology, rollback, debris, diagnostic,
  and error behavior; unchanged public APIs; removal of inert `needManifest`;
  and authorship confined to `internal/baton/install_transaction.go`.
- The Implementer may perform exactly one bounded remediation and run the
  existing behavior suites. It must stop at the clean pushed semantic commit:
  closure dispatch is prohibited until planning authority repins the sealed S02
  authorization to that exact new review head.

## 2026-07-16 — Bounded remediation and closure authority synchronized

- The bounded remediation is committed and pushed at exact semantic head
  `4377d71a23a2252d4bbb6bb3784692171b0329da`. It authors only
  `internal/baton/install_transaction.go`, replaces the reusable transaction
  bag with complete preflight/captured/staged/published states, reconstructs
  recovery from validated published authority, and removes inert
  `needManifest` without changing public APIs, fault names, error classes,
  transaction identity, durability boundaries, rollback, or diagnostics.
- Final-byte verification passed: targeted `TestBatonSync|TestInstallIdentity`
  (118.840s), `go test ./internal/baton -count=1` (362.262s),
  `go test ./... -count=1`, `go vet ./...`, `make build`, `gofmt`, diff checks,
  and public-signature/fault-name/error-class comparisons.
- Planning amendment `af9426e0005e8f319acbf504fa63f76553dcd880`
  replaced only the two S02 sealed-manifest `review_head` values with
  `4377d71…`, updated the pinned digest to
  `2fabdcbf60ea0d81f77259bcaa08258a0e804f4cf1e23b8ba33eb2a7d47f5666`,
  and recorded the ratified endpoint decision. Both S01 envelopes, both S02
  starts, all merge/path/tuple evidence, purposes, interval semantics, schema,
  prohibitions, and the three unrecognized pre-start merges remain unchanged.
- A fresh no-history ambiguity review returned `PASS`. Draft 2020-12 manifest
  validation, digest reproduction, endpoint ancestry and one-file semantic
  delta checks, all 109 ACs, trace, requirements validation, design-fit,
  spec-quality, deterministic rendering, and diff checks passed.
- Ordinary two-parent synchronization
  `866bd2113e9530c8cd645006eb573fe11af4b3c1` has exact parents `4377d71…` and
  `af9426e…`. Every affected tuple satisfies section 6, the release-wt segment
  is the single planner commit under the release root, and deterministic
  `index.md` remains the current track version. The merge is pushed and clean.
- S02 remains `in_progress` with maintainability `pending`, cycle 0, and null
  `implementation_head`. One fresh role-isolated Implementer closure review is
  now authorized against exact semantic head `4377d71…`; its canonical scope,
  exclusions, fingerprint, and presentation digest must be recomputed from Git.
  No closure dispatch, new report, status transition, proof, Verifier action,
  real-home access, or `doctor --sync-baton` execution occurred in this record.

## 2026-07-16 — Corrected closure endpoint synchronized before dispatch

- Before any closure dispatch, independent final-byte review found and the
  Implementer corrected the sole `paths-ready` pre-capture crash-disposition
  regression in exact semantic commit
  `2a17443d67d39cf681dba117a57673714a916d7f`. The correction remains confined
  to `internal/baton/install_transaction.go`; final targeted, package,
  repository, vet, build, formatting, diff, reachability, and repeat
  independent-audit gates passed.
- Planner commits `4d9e70b6b2df5583a36a9dc350fb2abece07c647` and
  `727fbf1f2debbe3e2a46655ef390ceca7858b6c9` replace only the two S02 sealed
  `review_head` endpoints with `2a17443…`, pin manifest SHA-256
  `23ca47fe790e5f8d4e9022b5b0df819de9972938d581e014a7ffd9c0dc16227e`, and
  capture fresh authority PASS plus deterministic planner gates. The prior
  `4377d71…` authorization is superseded before use; all S01/S02 envelope
  evidence and prohibitions remain unchanged.
- Ordinary two-parent synchronization
  `d3acf943f666f52c0e575e9116f07766ff94a5f7` has first parent `2a17443…` and
  second parent `727fbf1…`. Its staged result was exactly the three planner
  paths, each equal to the second-parent blob; Draft 2020-12 validation,
  manifest digest, board reconciliation, and section-6 composition passed.
- S02 remains `in_progress`, maintainability remains `pending` at cycle 0 with
  null `implementation_head`, and no closure report, status transition, proof,
  Verifier action, real-home access, or `doctor --sync-baton` execution has
  occurred. One fresh role-isolated Implementer closure review is now
  authorized against exact semantic head `2a17443…` only.

## 2026-07-16 — Implementer Gate-8 cycle-0 closure PASS

- One fresh role-isolated Implementer closure review consumed the untouched
  v0.15.1 maintainability prompt and exact canonical semantic scope from
  immutable base `e61cb190736ee7483fb4ed1a993442b26ce3574c` through corrected
  head `2a17443d67d39cf681dba117a57673714a916d7f`. Its 45 included and 20
  excluded paths fingerprint to
  `sha256:c72341fa8bab5c4a9b7a548b7ffb3ba1d57955f5e322d527c5284a1eed54f8d2`.
- Invocation `f4ef4f75-4dc8-48d1-a37c-d37d5f83c5ff` returned `PASS` with zero
  findings. The full overlay-valid report is committed in
  `1c46bccc4ea8a2e360bd3e17d587a7963ed5d90e` at
  `reports/maintainability/implementer-cycle-0-f4ef4f75-4dc8-48d1-a37c-d37d5f83c5ff.json`,
  blob `31172c04c2063333d2cd041ded2fbe66b7e8f965`.
- The append-only ledger preserves the earlier preflight `FAIL` and records the
  permitted closure `PASS`; `maintainability.state` is now `passed` and
  `implementation_head` is pinned to the reviewed semantic head. Overall S02
  state remains `in_progress` with verification pending until a live proof bundle
  and deterministic proof gate are complete.
- No semantic source/test/configuration change, fresh Verifier dispatch,
  real-home access, or `doctor --sync-baton` execution occurred in this
  transition.

## 2026-07-16 — Planner invalidates premature closure authority

- A fresh independent Gate-8 reconstruction proved that historical merge
  `7696c9bf9c235fffb937d3ed7e4be5a8a2bbda2a` fails section 6's mandatory
  deterministic `index.md` check. Its committed projection omits S02 ownership
  of `internal/run/slice_test.go`; rendering the exact composed historical
  records produces a different blob. The other three sealed merges reproduce
  byte-identically.
- Invocation `f4ef4f75-4dc8-48d1-a37c-d37d5f83c5ff` was dispatched by a
  concurrent session before that provenance defect was repaired. Its committed
  PASS report remains in Git as forensic evidence, but it is non-authoritative:
  scope construction was required to fail before model dispatch, so the report
  is removed from the status ledger and cannot support lifecycle or Verifier
  authority.
- The current sealed manifest also binds the two S02 historical merges only to
  final review head `2a17443…`, while the preserved preflight report consumed
  them at exact head `60097cfa…`. A replacement manifest must authorize both
  exact historical consumers separately; ancestor or descendant substitution
  remains prohibited.
- Fail-closed state is restored: maintainability is `pending`, cycle 0,
  `implementation_head` is null, and only the original authoritative preflight
  FAIL remains in the ledger. Overall S02 stays `in_progress` with verification
  pending. No semantic code, proof, Verifier, merge, real-home, or installation
  action is authorized until a human-ratified v2 planning amendment is synced.

## 2026-07-16 — Crash-durable replacement authority synchronized

- Planner commit `7ceeb46107fa4371ee1e59d6c4bb882dd2da806c`
  truthfully seals the physical orphan dispatch and temporary historical ledger
  append while denying them C-06 authority, recognizes only the exact
  `0d593202…` to `3ac42c01…` hold correction, and keeps C-08 restricted to
  `live_review` freshness authority.
- The replacement remains planning-only through the manual bootstrap adapter.
  Its claim commit consumes the sole dispatch budget and issues one
  non-resumable permit; the claim stores only a random continuity-token hash.
  A valid result requires a receipt revealing the preimage and binding the
  exact claim commit, invocation, report blob and verdict. Any restart without
  that receipt routes to the Coach with zero redispatch.
- The sealed v2 schema, manifest, claim schema, and receipt schema SHA-256
  digests are respectively
  `daa13bd5cb8dd3d5c0f7473ee132b9d15d083405a1e89bfeedc7f8e298bbbbad`,
  `3d0e0da7fa57a0d754b8e0b6a0faae90f47bea72c100ea2dbf0ba4c68c486dc1`,
  `32023df8e953640b266d9113c9055b9cd601cceb26e842def080dc1491563746`,
  and `3678f1ac208e0d9a04a3bc01ad9d9e61fa8bc5472402a05ae376f21ca8022a52`.
- Fresh authority and consistency reviewers both returned `PASS` against exact
  commit `7ceeb46…`. Draft 2020-12, duplicate-key, 109-AC, trace, requirements,
  design-fit, spec-quality, deterministic render, full repository test, vet,
  and build gates passed.
- Ordinary two-parent synchronization
  `da8cc0664732bf3f5e0a043b8c397ce094ed4821` has fail-closed hold
  `3ac42c01…` as parent one and sealed planning authority `7ceeb46…` as parent
  two. It changes only the eleven ratified release-record paths and revalidates
  all four digests and the byte-identical rendered board.
- No replacement claim or receipt exists yet, so no replacement dispatch
  budget has been consumed. S02 remains `in_progress`, maintainability remains
  `pending` at cycle 0 with null `implementation_head`, and verification remains
  pending. No model, Verifier, real-home, or installation action occurred in
  this record.
