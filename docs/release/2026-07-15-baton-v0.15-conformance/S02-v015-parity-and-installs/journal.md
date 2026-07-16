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
