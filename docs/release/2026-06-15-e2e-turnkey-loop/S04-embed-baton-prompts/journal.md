# Journal — S04-embed-baton-prompts

## Session 2026-06-16 (implementer)

**State transition:** `design_review` → `in_progress` → `implemented`

### Design directives incorporated

- **Coach Pin 1 (INCONCLUSIVE verdict):** Added `Inconclusive Verdict = "INCONCLUSIVE"` to `internal/verdict/verdict.go` with exit code 3 (fail-closed, distinct from BLOCKED's 2). Added INCONCLUSIVE case to `parseVerdict` in `internal/verify/verify.go`.
- **Coach Pin 2 (Memory ack — Baton protocol alignment):** Acknowledged. All four prompts vendored verbatim from `~/.claude/baton/role-prompts/` — open Baton protocol, MIT-licensed. "S21 stall" reference in captain prompt is generic enough for open-source.
- **Coach Pin 3 (VERSION.txt bump tracking):** Added `# Bump this version whenever prompt files are re-vendored from upstream Baton` comment at top of `internal/prompt/VERSION.txt`.
- **Coach Pin 4 (Negative check in test):** `TestVerifier_NotOldPlaceholder` asserts embedded prompt ≠ old inline const. `TestVerifier_ContainsInconclusive` asserts embedded prompt contains INCONCLUSIVE token (absent from old const).

### Implementation decisions

- Vendored all four role prompts (verifier, implementer, planner, captain) now — the planner/implementer/captain are inert until S06/S07 consume them.
- Single `prompt` package with four accessor functions + `BatonVersion()`. `go:embed` at package level with `init()` reading into package-level vars.
- Replaced `const systemPrompt` with `var systemPrompt = prompt.Verifier()` in `verify.go`.
- Extended `sworn version` to print `baton-protocol v1.0.0` (from VERSION.txt) as a second line.
- `cmd/sworn/main.go` is a documented shared file — S02 touched `verify` case, S04 touches `version` case; additive and region-separable per Captain flags.

### Server start

No servers needed — sworn is a pure Go CLI project. `baton-server-start.sh` skipped (designed for fired project with Next.js + Go API).

### Deferrals

None.

### Skeptic panel

Skipped — no Agent/Workflow tool available in this harness. Panel is an accelerant, not a gate per implementer prompt.
### Trade-offs

- `prompt.go` uses `panic` in `init()` if an embedded file is missing — build-time failure is correct (no degraded runtime fallback), but means a corrupted vendoring dir produces a confusing startup panic rather than a compile error. go:embed would catch most cases at compile time; the `init()` panic is a belt-and-suspenders backstop.
- `BatonVersion()` parses VERSION.txt by stripping comment lines and taking the last non-empty line. This is simple but fragile if the file format changes significantly.

### Test results

All `go test ./...` pass. `go vet ./...` clean. Binary smoke: `sworn version` outputs both `sworn 0.0.0-dev` and `baton-protocol v1.0.0`.

### Dark-code false positive

`release-verify.sh` flagged "placeholder" in `prompt_test.go` comments as dark-code markers. Reworded test comments to avoid the pattern — the word was legitimate (referring to the old inline const, not marking deferred work).

## Verifier verdicts received

### 2026-06-16T18:30:00Z — PASS

**Gate 1 — User-reachable outcome:** `internal/verify/verify.go:21` uses `prompt.Verifier()` (not the old inline placeholder). Confirmed live.

**Gate 2 — Planned touchpoints match actual changed files:** Diff matches spec touchpoints; out-of-plan files (`verdict.go`, `main.go`) are documented in proof.md "Divergence from plan" with justification.

**Gate 3 — Required tests exist and exercise integration point:** 8/8 prompt tests + 5/5 verify tests pass live, including `TestVerifier_ContainsVerdictContract` (PASS/FAIL/BLOCKED tokens), `TestVerifier_NotOldPlaceholder`, `TestVerifier_ContainsInconclusive`.

**Gate 4 — Reachability artefact:** Fresh build + `sworn version` produces `sworn 0.0.0-dev\nbaton-protocol v1.0.0` — matches proof.md.

**Gate 5 — No silent deferrals:** Only "placeholder" hit is test variable name (not a deferral). No TODO/FIXME in production code. spec: "Deferrals allowed? No" — zero deferrals found.

**Gate 6 — Claimed scope matches implemented scope:** All 3 ACs and 4 Coach Pins have verifiable evidence references.

Verdict: **PASS** — slice moves to `verified`.