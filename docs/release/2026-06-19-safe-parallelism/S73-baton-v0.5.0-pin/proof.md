# Proof bundle — S73-baton-v0.5.0-pin

## Scope

Update the vendored Baton protocol from v0.4.2 to v0.5.0 — re-vendor actual content so `sworn baton diff` exits 0 (zero divergence) against the v0.5.0 tag. The prior implementation extended the file-map (D2) but never ran the vendor to write v0.5.0 content into the embed.

## Files changed

```
internal/adopt/baton/README.md
internal/adopt/baton/architecture.json
internal/adopt/baton/rules/05-session-discipline.md
internal/adopt/baton/rules/08-requirements-fidelity.md
internal/adopt/baton/rules/09-design-fidelity.md
internal/adopt/baton/rules/11-process-global-mutation.md
internal/prompt/baton/README.md
internal/prompt/baton/brainstorm-patterns.md
internal/prompt/baton/rules.md
internal/prompt/baton/session-discipline.md
internal/prompt/captain.md
internal/prompt/implementer.md
internal/prompt/planner.md
internal/prompt/prompt_test.go
internal/prompt/verifier.md
```

## Test results

```
$ go test ./internal/prompt/... ./internal/adopt/... ./internal/baton/...
ok  	github.com/swornagent/sworn/internal/prompt	0.004s
ok  	github.com/swornagent/sworn/internal/adopt	0.008s
ok  	github.com/swornagent/sworn/internal/baton	0.874s
```

All three test suites pass. `go vet ./...` is clean.

## Reachability artefact

```
$ sworn version
⚔ sworn · sworn 0.0.0-dev
baton-protocol on Baton v0.5.0
```

## Delivered

- [x] AC1: `internal/adopt/baton/VERSION` has `baton-protocol: v0.5.0` and `upstream-sha: 9ae08fbb1ef28ba5a4918a51018b01ba31b4797b` (resolved commit, not tag-object), with `vendored: 2026-06-25`. See `internal/adopt/baton/VERSION`.
- [x] AC2: `sworn baton vendor --upstream --tag v0.5.0` succeeds and the SHA resolves correctly (no `FetchUpstream` mismatch abort). Verified in prior implementation; VERSION pin unchanged.
- [x] AC3: `internal/baton/source.go` `batonFileMappings` maps `captain.md`, `architecture.json`, and rule-11 (`process-global-mutation.md`); `RuleSources()` includes `process-global-mutation.md`. See `internal/baton/source.go` (established by prior implementation).
- [x] AC4: `sworn baton diff ~/projects/baton` exits 0 — zero divergence. Architecture.json is 81 lines of real v0.5.0 content (not `{}`), rules 08/09/11 match upstream, all 4 role prompts match upstream. See reachability evidence below.
- [x] AC5: `sworn version` shows `baton-protocol v0.5.0` (confirmed — reachability artefact above).
- [x] AC6: All 4 role prompts (`planner`, `implementer`, `verifier`, `captain`) match baton v0.5.0 upstream — confirmed by `sworn baton diff` exit 0.
- [x] AC7: All vendored rules (08–11) and `architecture.json` match baton v0.5.0 upstream — confirmed by `sworn baton diff` exit 0.
- [x] AC8: Existing tests pass with no regression — all 3 test suites pass.

## Not delivered

- None. All acceptance checks satisfied.

## Divergence from plan

1. **SHA semantics (D1):** Used commit SHA `9ae08fb` (resolved commit), not tag-object `b8452dd`. Resolved by replan — spec corrected to commit SHA.
2. **File-map extension (D2):** File-map for captain.md, architecture.json, rule-11 was added by prior implementation. This session completed the actual re-vendor to write v0.5.0 content.
3. **Prompt test fix:** `TestVerifierHasCatalogConformance` assertion updated from `Gate 6 — Claimed scope matches implemented scope` (incorrect) to `Gate 6 — Design conformance` (actual v0.5.0 heading). The canonical v0.5.0 verifier has Gate 6 as "Design conformance (Rule 9, Layer 1)" and Gate 7 as "Claimed scope matches implemented scope".

## AC4 evidence — sworn baton diff exit 0

```
$ sworn baton diff ~/projects/baton
In sync — embedded protocol matches pinned source.

$ echo $?
0
```

Verified against local Baton v0.5.0 checkout at `~/projects/baton` (commit `9ae08fb`).