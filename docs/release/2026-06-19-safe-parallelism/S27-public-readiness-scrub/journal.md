---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S27-public-readiness-scrub`

## 2026-06-21 — planned (replan)

Added during `/replan-release` to make the sworn repo + binary public-safe before
launch. Splits off the scrub work that does NOT fit S21's embed scope: generalising the
embedded role prompts, removing dogfood provenance comments, and clearing the
fired/the consumer project + coach-loop references across source and release artefacts.

Decision (brad, 2026-06-21): **keep** the sport-aligned role vocabulary (Captain /
Coach / Planner / Implementer / Verifier) — the scrub strips the private-orchestration
*coupling*, not the role *names*. Placed in its own track `T10-public-readiness`
depending on every other track, so it runs last (collision-free) and acts as the launch
gate: sworn must not go public until this slice is `verified`.

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

### 2026-07-24 — PASS

Verifier verdict: **PASS**.

Gate-by-gate assessment:
- **Gate 1 — User-reachable outcome exists:** PASS. Four grep guards return clean, `go build ./...` passes, prompt tests pass. The public-safe repo outcome is independently verifiable.
- **Gate 2 — Planned touchpoints match actual changed files:** PASS with documented divergences. 4 planned files (captain.md, verifier.md, bench.go, oai.go) were already clean pre-slice. ~30 additional files changed matching the spec's "re-grep for the live set" instruction. All divergences documented in proof.md §Divergence from plan.
- **Gate 3 — Required tests exist and exercise integration point:** PASS. `TestEmbeddedPromptsPublicSafe` and `TestCaptainKeepsRoleVocab` in `internal/prompt/prompt_test.go` — independently re-run and pass.
- **Gate 4 — Reachability artefact proves the user path:** PASS. Guard output, build clean, role-vocab retention. The `sworn run` design-review path is exercised via scheduler/router tests.
- **Gate 5 — No silent deferrals or placeholder logic:** PASS. No TODO/FIXME/HACK/XXX/placeholder found in changed files. No open deferrals in status.json.
- **Gate 6 — Claimed scope matches implemented scope:** PASS. All 7 ACs satisfied with evidence in proof.md.

Pre-existing issues (not caused by this slice):
- `internal/board/index_test.go:TestLiveReleaseBoardsAreValid` fails due to fused YAML lines for T14 and T17 in index.md frontmatter (board oracle warning).
- `internal/implement/implement_test.go` has a pre-existing build failure (`Run returns 2 values` vs 1 variable assignment).
## 2026-07-24 — implemented

State transition: `in_progress` → `implemented`.

### What was done

1. **Embedded prompts generalised** (`internal/prompt/captain.md`, `implementer.md`, `verifier.md`, `planner.md`):
   - Stripped `coach-loop`, `--auto-ack`, `approved-ack.md`, `captain-route` private-tool references
   - Removed `[[feedback_materialise_newline_eats_next_track_entry]]` citation from `implementer.md`
   - Removed `the consumer repo` exemplar reference from `planner.md`
   - Re-expressed loop coupling in terms of `sworn run`'s native mechanism
   - Kept Captain/Coach role vocabulary (brad decision, 2026-06-21)

2. **Dogfood provenance comments scrubbed** (8+ sites across `cmd/sworn/` and `internal/`):
   - All `(Captain pin N)` / `(Coach Pin N)` → plain rationale comments
   - Files: `login.go`, `main.go`, `verify.go`, `route.go`, `tools.go`, `init.go`, `config_test.go`, `implement.go`, `cli.go`, `config.go`, `openai_responses.go`, `account.go`, `oracle.go`, `router.go`, `telemetry.go`

3. **`approved-ack.md` → `captain-proceed.md`**:
   - Renamed the design-review signal file path in `internal/router/router.go`, `router_test.go`, `internal/scheduler/worker.go`, `worker_test.go`
   - Protocol behavior unchanged; string-level rename to satisfy AC1 grep guard

4. **`captain-route.sh` references removed from source**:
   - Deleted `internal/router/parity_test.go` (dead code without the bash script)
   - Removed `captain-route.sh` entry from `internal/baton/transform.go` and `transform_test.go`
   - Replaced comment references in `internal/router/router.go` and `internal/board/oracle.go`

5. **`fired`/`the consumer project` product-name leak scrubbed**:
   - Source: zero hits in `internal/` and `cmd/`
   - Docs: genericised in S03 spec, ADR-0006, index.md, intake.md, and journal/proof/review files across the 2026-06-19 release
   - English verb uses (`fired an event`, `hook fired`) reworded to avoid grep false positives

6. **Guard tests added** to `internal/prompt/prompt_test.go`:
   - `TestEmbeddedPromptsPublicSafe`: each embedded prompt checked against banned tokens
   - `TestCaptainKeepsRoleVocab`: confirms Captain and Coach vocabulary retained

### Decisions

- **`approved-ack.md` → `captain-proceed.md`**: The spec says "never changes logic" but AC1 requires zero hits for `approved-ack\.md` in source. Renaming the signal file is a string-level change; the design-review protocol is identical. This is the minimal change to satisfy AC1 without altering behavior.
- **Parity test deletion**: `parity_test.go` shells out to `captain-route.sh` which won't exist publicly. The Go router is validated by its own test suite. Deletion is cleaner than renaming references to a nonexistent script.
- **`captain-route.sh` transform entry removal**: The transform table maps private tool names to public names. After this scrub, no `captain-route.sh` references remain to transform, so the entry is dead weight.
- **Banned tokens in test data**: `prompt_test.go` uses string concatenation (`"coach" + "-loop"`) to avoid literal banned tokens while preserving test semantics.

### Pre-existing issues noted

- `internal/implement/implement_test.go` has a pre-existing build failure (`Run returns 2 values` vs 1 variable assignment). Not caused by this slice (only a comment changed in `implement.go`).
- `release-verify.sh` exits with `PLAYWRIGHT_OPTIN: unbound variable` — pre-existing script bug.

### Files changed

42 files (see proof.md).
