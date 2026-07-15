# Journal — S01-vendor-boundary-readiness

## 2026-07-16 — Implementer design checkpoint

- Transitioned `planned -> design_review`; no production code or tests were
  written.
- Bootstrap oracle adapter explicitly authorised by the planning authority:
  invoked the installed pre-cutover read-only oracle as
  `sworn board --release 2026-07-15-baton-v0.15-conformance --json`, adapted its
  legacy top-level `{release, tracks}` result, and resolved S01 as the first,
  dependency-ready slice in `T1-foundation`.
- Governing role source adapter: used the clean Baton `v0.15.1` checkout at
  `/home/brad/projects/baton`, commit
  `3fb4d275ae8a151f6287e7b9279d71628b12eea0`, specifically
  `commands/implement-slice.md`, `baton/track-mode.md`, and
  `baton/role-prompts/implementer.md`, because the installed role docs remain
  pre-cutover until S02.
- Materialised the conventional release and T1 worktrees from clean
  `release/v0.2.0` commit `135a01e1c4e0e2825a40ddd93618c3cbc906fdea`.
- Readiness evidence: pinned v0.15.1 `spec-v1` and `slice-status-v1` validation
  passed; status history preserves null `start_commit` and the empty pending
  cycle-0 maintainability record; `sworn lint ac`, `sworn lint trace`,
  `sworn reqvalidate`, and `sworn designfit` all passed.
- Design-review pin: AC-03/AC-04 require CLI exit changes in
  `cmd/sworn/baton.go`, but that file is absent from the slice touchpoints. The
  Captain must require a planner correction or decline before implementation;
  the Implementer will not cross that ownership boundary.
