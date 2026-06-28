---
title: Proof bundle — S20-role-revendor
description: Rule 6 proof bundle for re-vendoring planner, implementer, captain role prompts from canonical post-records-as-JSON source.
---

# Proof Bundle: `S20-role-revendor`

## Scope

Re-vendor `planner.md`, `implementer.md`, `captain.md` from canonical Baton source (`$HOME/.claude/baton/role-prompts/` post-records-as-JSON) to `internal/prompt/`. Bump `internal/prompt/VERSION.txt` to match the S22 pin SHA.

## Files changed

```
$ git diff --name-only aebefae..HEAD
docs/release/2026-06-27-conformance-foundation/S20-role-revendor/journal.md
docs/release/2026-06-27-conformance-foundation/S20-role-revendor/status.json
internal/prompt/VERSION.txt
internal/prompt/captain.md
internal/prompt/implementer.md
internal/prompt/planner.md
```

## Test results

```
$ go test ./internal/prompt/...
ok  	github.com/swornagent/sworn/internal/prompt	0.005s
```

Full test suite:
```
$ go test ./...
ok  	github.com/swornagent/sworn/cmd/sworn	10.231s
ok  	github.com/swornagent/sworn/internal/account	10.127s
ok  	github.com/swornagent/sworn/internal/adopt	0.024s
ok  	github.com/swornagent/sworn/internal/agent	0.067s
ok  	github.com/swornagent/sworn/internal/baton	1.199s
ok  	github.com/swornagent/sworn/internal/bench	1.138s
ok  	github.com/swornagent/sworn/internal/board	0.121s
ok  	github.com/swornagent/sworn/internal/captain	0.045s
ok  	github.com/swornagent/sworn/internal/command	0.027s
ok  	github.com/swornagent/sworn/internal/config	0.026s
ok  	github.com/swornagent/sworn/internal/db	0.764s
ok  	github.com/swornagent/sworn/internal/design	0.025s
ok  	github.com/swornagent/sworn/internal/designaudit	0.021s
ok  	github.com/swornagent/sworn/internal/designfit	0.024s
ok  	github.com/swornagent/sworn/internal/ears	0.013s
ok  	github.com/swornagent/sworn/internal/gate	0.101s
ok  	github.com/swornagent/sworn/internal/git	0.396s
ok  	github.com/swornagent/sworn/internal/implement	0.391s
ok  	github.com/swornagent/sworn/internal/journey	0.036s
ok  	github.com/swornagent/sworn/internal/ledger	0.022s
ok  	github.com/swornagent/sworn/internal/lint	0.174s
ok  	github.com/swornagent/sworn/internal/mcp	0.184s
ok  	github.com/swornagent/sworn/internal/memory	1.136s
ok  	github.com/swornagent/sworn/internal/model	2.099s
ok  	github.com/swornagent/sworn/internal/orchestrator	0.005s
ok  	github.com/swornagent/sworn/internal/prompt	(cached)
ok  	github.com/swornagent/sworn/internal/reqvalidate	0.032s
ok  	github.com/swornagent/sworn/internal/reqverify	0.010s
ok  	github.com/swornagent/sworn/internal/router	0.006s
ok  	github.com/swornagent/sworn/internal/rtm	0.034s
ok  	github.com/swornagent/sworn/internal/run	3.762s
ok  	github.com/swornagent/sworn/internal/scheduler	0.066s
ok  	github.com/swornagent/sworn/internal/specquality	0.021s
ok  	github.com/swornagent/sworn/internal/state	0.014s
ok  	github.com/swornagent/sworn/internal/style	0.006s
ok  	github.com/swornagent/sworn/internal/supervisor	0.705s
ok  	github.com/swornagent/sworn/internal/telemetry	0.319s
ok  	github.com/swornagent/sworn/internal/tui	0.835s
ok  	github.com/swornagent/sworn/internal/verify	0.027s
EXIT: 0
```

### Acceptance check verification

```
=== AC1: planner.md diff vs canonical ===
$ diff internal/prompt/planner.md ~/.claude/baton/role-prompts/planner.md
PASS (exit 0 — byte-identical)

=== AC2: implementer.md diff vs canonical ===
$ diff internal/prompt/implementer.md ~/.claude/baton/role-prompts/implementer.md
39c39
< ... (it vanishes from `coach top`; see a known issue with freehand multi-line replacement). ...
---
> ... (it vanishes from `coach top`; see [[feedback_materialise_newline_eats_next_track_entry]]). ...
DIFFERS by 1 line — public-safety scrub required by TestEmbeddedPromptsPublicSafe.
The [[feedback_]] wiki reference is banned in public repos.

=== AC3: captain.md diff vs canonical ===
$ diff internal/prompt/captain.md ~/.claude/baton/role-prompts/captain.md
PASS (exit 0 — byte-identical)

=== AC4: VERSION.txt matches S22 pin ===
$ cat internal/prompt/VERSION.txt
42eb48b
PASS — matches S22 pin SHA

=== AC5: Stale marker grep ===
$ grep -rn "v0.4.2\|proof.md-primary\|PROOF-optional\|scripts/release-verify.sh" internal/prompt/{planner,implementer,captain}.md
PASS (exit 1 — zero results, clean)

=== AC6: go build ===
$ go build ./...
PASS (exit 0)
```

## Reachability artefact

Manual smoke step:
- AC1: `diff internal/prompt/planner.md ~/.claude/baton/role-prompts/planner.md` exits 0
- AC2: `diff internal/prompt/implementer.md ~/.claude/baton/role-prompts/implementer.md` has exactly 1 line difference (public-safety scrub)
- AC3: `diff internal/prompt/captain.md ~/.claude/baton/role-prompts/captain.md` exits 0
- AC5: stale marker grep returns zero results
- `go test ./...` exits 0

## Delivered

- [x] **AC1**: `internal/prompt/planner.md` byte-identical to canonical — diff exits 0
- [x] **AC2**: `internal/prompt/implementer.md` matches canonical with 1 public-safety scrub (1 line; `[[feedback_` → generic description) — required by `TestEmbeddedPromptsPublicSafe`
- [x] **AC3**: `internal/prompt/captain.md` byte-identical to canonical — diff exits 0
- [x] **AC4**: `internal/prompt/VERSION.txt` contains S22 pin SHA `42eb48b`
- [x] **AC5**: Zero stale markers (`v0.4.2`, `proof.md-primary`, `PROOF-optional`, `scripts/release-verify.sh`) in re-vendored files
- [x] **AC6**: `go build ./...` exits 0; full test suite passes
- [x] **Public-safety**: `TestEmbeddedPromptsPublicSafe` passes — no banned tokens in embedded prompts

## Not delivered

None. All six acceptance checks satisfied (AC2 with documented 1-line public-safety deviation).

## Divergence from plan

- **implementer.md public-safety scrub**: Canonical `implementer.md` contains `[[feedback_materialise_newline_eats_next_track_entry]]` — an internal wiki reference banned by the public-safety test (`TestEmbeddedPromptsPublicSafe`). Replaced with "a known issue with freehand multi-line replacement". This is a 1-line, 1-token deviation required for public-repo compliance. `planner.md` and `captain.md` required no scrubbing.
- **VERSION.txt vs S23 removal**: S23 removed `internal/prompt/VERSION.txt` as dead (version centralized to `internal/adopt/baton/VERSION`). S20's spec explicitly requires creating it with the S22 pin SHA. Created per spec. The file is not embedded by `prompt.go` (go:embed directive was removed in S23). If this is a spec defect, the verifier should flag it.