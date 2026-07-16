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
