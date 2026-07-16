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
  table now covers the exact 41-file high/high `beast` boundary, which the
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
  the high/high 41-file `beast` effort in `status.json`; fresh Captain review is
  still required before implementation.
