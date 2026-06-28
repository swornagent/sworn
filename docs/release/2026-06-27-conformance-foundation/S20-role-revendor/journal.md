# Journal: S20-role-revendor

## Session 2026-06-28 — Initial implementation

### Decisions

- **Re-vendored from canonical source**: Copied `planner.md`, `implementer.md`, `captain.md` from `$HOME/.claude/baton/role-prompts/` (post-records-as-JSON canonical). 
- **Public-safety scrub**: `implementer.md` contained one `[[feedback_materialise_newline_eats_next_track_entry]]` reference banned by `TestEmbeddedPromptsPublicSafe`. Replaced with "a known issue with freehand multi-line replacement". This is a single-line deviation from canonical required by public-repo safety. `planner.md` and `captain.md` required no scrubbing.
- **VERSION.txt**: Created with S22 pin SHA `42eb48b`. Note: S23 removed `internal/prompt/VERSION.txt` as dead (version centralized to `internal/adopt/baton/VERSION`), but the spec explicitly requires it. Created anyway per spec contract. This file is not embedded by `prompt.go` (the go:embed directive was removed in S23).
- **No changes to `design-reviewer.md`**: Out of scope per spec; handled by S19.

### Trade-offs

- The `implementer.md` diff AC (byte-for-byte identical to canonical) cannot pass simultaneously with the public-safety test (`TestEmbeddedPromptsPublicSafe`). The canonical source contains internal wiki references that must be scrubbed for public consumption. The single-line deviation is the minimum scrub needed.
- VERSION.txt is a dead file (not embedded, removed by S23) but created per spec. If this is a spec defect, the verifier will flag it.

### Deferrals

None. All spec scope satisfied.
## Verifier verdicts received

### 2026-07-28 — Verifier session (fresh context)

**Verdict: PASS**

**Gates walked:**
- Gate 1 — User-reachable outcome: PASS. Re-vendored prompts are embedded via go:embed in `internal/prompt/prompt.go` and exported through `Planner()`, `Implementer()`, `Captain()` functions used by sworn binary dispatch.
- Gate 2 — Planned touchpoints match actual changes: PASS. All 4 planned files (`planner.md`, `implementer.md`, `captain.md`, `VERSION.txt`) changed in feat commit `a94e400`. Docs-only noise from forward-merge (T2 S08-S10 code) excluded — not S20 scope.
- Gate 3 — Required tests exist and exercise integration point: PASS. `go test ./internal/prompt/...` exits 0 (24 tests including `TestPlanner_NonEmpty`, `TestImplementer_NonEmpty`, `TestCaptain_NonEmpty`, `TestEmbeddedPromptsPublicSafe`). `go test ./...` exits 0. `go build ./...` exits 0. `go vet` clean.
- Gate 3b — LLM AC satisfaction check: SKIPPED (LLM provider not configured; non-blocking).
- Gate 4 — Reachability artefact: PASS. Manual smoke step validated: diff vs canonical exits 0 for planner.md and captain.md; implementer.md has exactly 1 documented public-safety line (replacing `[[feedback_` wiki reference).
- Gate 5 — No silent deferrals or placeholder logic: PASS. Grep for TODO/FIXME/deferred/placeholder/XXX/HACK in changed prompt files returns only Baton protocol terminology in instructional text, not implementation markers.
- Gate 6 — Design conformance: PASS (not UI-bearing; Go CLI project).
- Gate 7 — Claimed scope matches implemented scope: PASS. All 6 ACs verified with evidence: AC1 (planner.md byte-identical), AC2 (implementer.md with documented public-safety scrub), AC3 (captain.md byte-identical), AC4 (VERSION.txt = 42eb48b matches S22 pin), AC5 (zero stale markers), AC6 (go build passes). Public-safety scrub documented in divergence. VERSION.txt dead-file concern noted but is spec-compliant (spec explicitly requires creation).

**Verified against commit:** `a94e400beb072ce96ded6ce900c67b14b1d0e3dc`
**Verifier session:** fresh, artefact-only
