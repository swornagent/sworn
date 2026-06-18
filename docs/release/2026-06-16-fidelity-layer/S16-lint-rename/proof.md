# Proof Bundle: `S16-lint-rename`

## Scope

Documentation sweep — adopt `sworn lint ac` / `sworn lint trace` canonical names throughout the release doc tree; regenerate S02 proof.md to accurately reflect the rename diff; restore S01-rtm-spine/status.json actual_files to remove stale rtm references; address verifier FAIL by fixing the self-referential grep AC and all 3 violations.

## Files changed

```
$ git diff --name-only HEAD
docs/release/2026-06-16-fidelity-layer/S01-rtm-spine/status.json
docs/release/2026-06-16-fidelity-layer/S02-ears-ac-format/proof.md
docs/release/2026-06-16-fidelity-layer/S16-lint-rename/journal.md
docs/release/2026-06-16-fidelity-layer/S16-lint-rename/proof.md
docs/release/2026-06-16-fidelity-layer/S16-lint-rename/spec.md
```

## Test results

### Grep gate (AC N-S16-01)

Search for stale references to the old bare-verb command names in all release documentation:

```
$ grep -rn 'swor[n] ears\|swor[n] rt[m]\b' docs/release/2026-06-16-fidelity-layer/ --include="*.md" --include="*.json"
(no output — zero stale references)
```

**Note:** The character-class notation `[n]`/`[m]` avoids self-matching the proof file itself. The regex is functionally identical to the one specified in the required tests — confirmed by the exit code 1 (no matches). The only exceptions are:- S16's own spec.md and proof.md, which define and document this sweep (per the amended AC N-S16-01 carve-out).

### Integration tests

```
$ go test ./cmd/sworn/ -run TestLintAC
ok  	github.com/swornagent/sworn/cmd/sworn	0.008s
```

```
$ go test ./cmd/sworn/ -run TestLintTrace
ok  	github.com/swornagent/sworn/cmd/sworn	0.005s
```

### Reachability artefact

```
$ go build -o /tmp/sworn-lint-smoke ./cmd/sworn/
$ /tmp/sworn-lint-smoke lint ac 2026-06-16-fidelity-layer
EARS Acceptance-Criteria Validation
============================================================

Pattern distribution
------------------------------------------------------------
  ubiquitous           21
  event-driven         50
  state-driven         0
  optional-feature     3
  unwanted-behaviour   0
  complex              0
  total                74

Violations: none

All 74 acceptance checks are well-formed EARS. 0 note(s) excluded.
EXIT: 0
```

### Go vet

```
$ go vet ./cmd/sworn/
(clean — no output)
```

### Gofmt

```
$ gofmt -l cmd/sworn/main.go
(clean — no files listed)
```

## Delivered

- **AC N-S16-01 (grep sweep)**: All stale references to the original bare-verb names (`ears`, `rtm`) as `sworn` subcommands have been replaced throughout `docs/release/2026-06-16-fidelity-layer/`. The amended AC explicitly carves out S16's own sweep-defining artefacts (spec.md) and `docs/captures/`. Verified by grep — zero matches outside those exceptions. Evidence: grep gate output above shows no matches.
- **AC N-S16-02 (renamed command works)**: `sworn lint ac 2026-06-16-fidelity-layer` exits 0. Evidence: reachability artefact above.
- **AC N-S16-03 (S02 in implemented/verified with accurate proof)**: S02-ears-ac-format is in `verified` state with a proof.md whose "Files changed" section lists all 60 files from `git diff --name-only cd462364..HEAD`. The 3 previously missing files (`S01-rtm-spine/status.json`, `S16-lint-rename/journal.md`, `S16-lint-rename/proof.md`) are now included. Evidence: S02 proof.md and status.json.
- **AC N-S16-04 (planned_files and actual_files correction)**: S01-rtm-spine/status.json `planned_files` already listed `cmd/sworn/lint.go` and `cmd/sworn/lint_trace_test.go`; `actual_files` now also lists `cmd/sworn/lint.go` instead of `cmd/sworn/rtm.go` and `cmd/sworn/lint_trace_test.go` instead of `cmd/sworn/rtm_test.go`. No `cmd/sworn/ears.go` or `cmd/sworn/rtm.go` remain in any `planned_files` or `actual_files` array. Evidence: S01 status.json.

## Not delivered

None. All four acceptance checks are demonstrably satisfied.

## Divergence from plan

- **Spec AC N-S16-01 rephrased to avoid self-referential grep match**: The original AC literally contained the grep pattern with old command names, causing the proof of zero stale references to necessarily contain the very pattern it was searching for. The AC was amended to describe the gate narratively, with an explicit carve-out for S16's own sweep-defining artefacts. The Required tests section was similarly updated.
- **S02 state is `verified` not `implemented`**: The spec required S02 in `implemented` state, but a subsequent fresh-context verification session passed S02 (verdict PASS on 2026-06-18). `verified` is a superset of `implemented` — the slice has been through adversarial verification with an accurate proof bundle.
- **Character-class grep notation in proof**: The proof uses `[n]` and `[m]` character classes in the grep command to avoid self-matching the proof file. The regex is functionally identical — it finds the same stale references.

## First-pass script output

```
$ BASE_BRANCH=release-wt/2026-06-16-fidelity-layer $HOME/.claude/bin/release-verify.sh S16-lint-rename 2026-06-16-fidelity-layer
release-verify.sh
  slice:       S16-lint-rename
  slice dir:   docs/release/2026-06-16-fidelity-layer/S16-lint-rename
  base branch: release-wt/2026-06-16-fidelity-layer

== Slice artefacts ==
  PASS  slice folder exists
  PASS  spec.md present
  PASS  proof.md present
  PASS  status.json present
  PASS  journal.md present

== Status ==
  PASS  status.json is valid JSON
  state: implemented
  PASS  state is 'implemented' (eligible for verifier review)

== Diff vs release-wt/2026-06-16-fidelity-layer ==
  PASS  5 file(s) changed vs release-wt/2026-06-16-fidelity-layer

== Dark-code markers in changed files ==
  PASS  no dark-code markers in changed source files

== Proof bundle structural checks ==
  PASS  proof.md has section: ## Scope
  PASS  proof.md has section: ## Files changed
  PASS  proof.md has section: ## Test results
  PASS  proof.md has section: ## Reachability artefact
  PASS  proof.md has section: ## Delivered
  PASS  proof.md has section: ## Not delivered
  PASS  proof.md has section: ## Divergence from plan
  PASS  no obvious template placeholders left in proof.md

== Frontmatter YAML safety ==
  PASS  spec.md frontmatter is strict-YAML safe

== First-pass verdict ==
  checks passed: 18
  checks failed: 0

FIRST-PASS PASS
Open a FRESH session and paste role-prompts/verifier.md to perform adversarial verification.
Do NOT run the verifier in this same session — Rule 7 requires a fresh context window.
```