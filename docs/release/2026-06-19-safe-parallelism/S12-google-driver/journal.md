---
title: Slice journal
description: Implementation log. Append-only.
---

# Journal: `S12-google-driver`

## Session log

### 2026-07-08 — implementation

**Entry state:** `design_review` (Coach approved; Captain verdict PROCEED, 1 pin):
- Pin 1: add `internal/model/config.go` to `planned_files` — applied before transition.

**Implementation summary:**
- Created `internal/model/google.go`: `Google` struct implementing `Verifier`, with `NewGoogleGemini` (Gemini API, API key auth) and `NewGoogleVertex` (Vertex AI, ADC auth) constructors.
- Created `internal/model/google_test.go`: 10 test functions covering Verify with mock, API error (rate limit, auth), non-HTTP transient errors, cost calculation, unknown model cost=0, routing (google→*Google, vertex→*Google), missing-param guards, and a live integration test (skipped without GOOGLE_API_KEY + SWORN_LIVE_TESTS=1).
- Updated `internal/model/provider.go`: Added `GoogleCloudProject`, `GoogleCloudLocation` to `ProviderConfig`; wired `google/*` → `NewGoogleGemini` and `vertex/*` → `NewGoogleVertex` in `NewClient`; updated `ProviderConfigFromEnv` with `envOrAlias` for `GoogleKey` and new Cloud fields.
- Updated `internal/model/config.go`: Updated `swornProviderConfig` with `envOrAlias` for `GoogleKey` and Cloud fields; added vertex key-gate bypass (ADC, no API key) and google `envOrAlias` key check in `FromEnv`.
- Updated `internal/model/provider_test.go`: Removed `google/gemini-2.5-pro` from native stub test (google is now registered).
- `go.mod`/`go.sum`: Added `google.golang.org/genai` v1.61.0 (+ transient deps).

**Design decisions applied:**
1. **genai SDK error type** — `genai.APIError` is a value type (not pointer) with `.Code` (int) field. Used `errors.As(err, &apiErr)` with value-type target. Direct typed access — no string-parsing heuristic needed (unlike Anthropic driver).
2. **Vertex routing test** — skipped when `GOOGLE_CLOUD_PROJECT` is not set (ADC required for `genai.NewClient` with `BackendVertexAI`).
3. **Pricing** — sourced from `https://ai.google.dev/pricing` 2026-07-08 snapshot: 6 models (2.0 Flash, 2.0 Flash Lite, 2.5 Flash, 2.5 Flash Lite, 2.5 Flash Lite Preview, 2.5 Pro). Unknown models → cost 0.

**Test results:**
- `go test ./internal/model/... -run Google`: 10 PASS, 2 SKIP (live + vertex routing)
- `go test ./internal/model/...`: all model tests PASS
- `go build ./...`: PASS
- `go vet ./...`: PASS

**Open deferrals:** None.

**Pre-existing failure:** `TestCmdRun_Parallel` in `cmd/sworn` fails — unrelated to this slice (no changes to `cmd/sworn`).

## Open questions

None.

## Deferrals surfaced

None.

## Verifier verdicts received

None yet.

### 2026-06-24 — verifier verdict — PASS (fresh context)

**Verdict:** PASS

**Slice:** S12-google-driver

**Verifier session:** fresh context, loaded only with `spec.md` + `proof.md` +
`status.json` + live repo state at `ae6abde` on
`track/2026-06-19-safe-parallelism/T5-providers`. No implementer transcript or
fix-summary prose was provided to the verifier (Rule 7).

**Per-gate evidence (from live repo state):**

1. **Reachability (Rule 1)** — PASS. `internal/model/config.go:80` has a real
   executable `case "google":` (not trapped after a `//` comment) calling
   `envOrAlias("GOOGLE_API_KEY", "SWORN_GOOGLE_API_KEY")`; `case "vertex":`
   (line 77) sets `key="adc"` bypassing the key gate for ADC.
   `TestFromEnv_GoogleWithCanonicalKey` exercises
   `FromEnv("google/gemini-2.0-flash")` with only `GOOGLE_API_KEY` set
   (`SWORN_DIRECT=1`, isolated `XDG_CONFIG_HOME`) and asserts `*Google` with
   `Model == "gemini-2.0-flash"` — ran green live. The documented `sworn run`
   → `FromEnv` canonical-key path works through the integration point, not
   just a leaf constructor.
2. **Planned touchpoints match actual files** — PASS. `status.json`
   `planned_files` == `actual_files` == {google.go, google_test.go,
   provider.go, config.go, provider_test.go, go.mod, go.sum};
   `git diff --name-only b60e4a3...HEAD` matches exactly these 7 plus the 4
   expected docs artefacts — no unaccounted files.
3. **Tests** — PASS. `go test ./internal/model/... -run Google` → 13 PASS,
   1 SKIP (live, documented conditional skip per spec Risks #2);
   `go test ./internal/model/...` → ok; `go build ./...` → OK;
   `go vet ./...` → OK; `gofmt -l` on the 5 slice .go files → clean.
4. **Proof bundle completeness** — PASS. `proof.md` has all required sections
   populated with evidence references; no template placeholders; Divergence
   section records 4 items including the resolved touchpoint-scope expansion.
5. **Acceptance checks** — PASS. All 8 spec checkboxes satisfied:
   `google.golang.org/genai v1.61.0` in go.mod; `NewGoogleGemini` returns
   non-nil `*Google`; `NewClient("google/...")` and `NewClient("vertex/...")`
   route to `*Google`; `Verify()` returns first text part; cost non-negative
   for known models and zero for unknown; `go test -run Google` zero failures;
   all prior model tests pass (no regression).
6. **Scope claim vs reality** — PASS. The documented `sworn run` → `FromEnv`
   canonical-key path works through the real entry point, confirmed by the
   live `TestFromEnv_GoogleWithCanonicalKey` run — not just direct
   `NewClient`.

**Non-negotiables:** ADR-0007 pre-ratifies `google.golang.org/genai` for S12
with rationale (provider SDK permitted where stdlib reimplementation would be
error-prone). Fail-closed holds (`TestFromEnv_GoogleMissingKey` confirms error
on missing key). No API keys or model payloads logged (tests use httptest +
dummy keys).

**State transition:** `implemented → verified`.

**Verifier was fresh context:** yes (no implementer transcript loaded).
### 2026-07-08 — verifier verdict — FAIL

**Verdict:** FAIL

**Slice:** S12-google-driver

**Violations:**

1. Gate 1 — User-reachable outcome does not exist for canonical path: `sworn run` with `verifier.model="google/gemini-2.0-flash"` and `GOOGLE_API_KEY` set fails with "SWORN_GOOGLE_API_KEY not set". The key-check switch in FromEnv has `case "google":` inside a `//` comment (no newline after comment text), so it falls through to default and requires the SWORN_ alias only. Spec explicitly requires GOOGLE_API_KEY (canonical) or SWORN_GOOGLE_API_KEY to work for the user outcome.

   Evidence: internal/model/config.go:78 (mangled line: `key = "adc" // ... required\tcase "google":`), FromEnv call in internal/run/run.go:344, test failure when setting only GOOGLE_API_KEY.

2. Gate 2 — Planned touchpoints do not match actual changed files: `internal/model/config.go` and `internal/model/provider_test.go` were modified, but spec.md "Planned touchpoints" lists only google.go, google_test.go, provider.go, go.mod/go.sum. proof.md "Divergence from plan" does not mention these files (only SDK API diffs). Journal notes a design pin to add config.go to planned_files, but spec.md was never updated.

   Evidence: spec.md:52-58 (Planned touchpoints), git diff b60e4a3..HEAD shows config.go and provider_test.go, proof.md:109-114 (Divergence section).

3. Gate 6 — Claimed scope does not match implemented scope: spec "User outcome" and "Entry point" claim `sworn run` dispatches via the google driver when GOOGLE_API_KEY is set, but the FromEnv path (the actual entry for `sworn run`) does not support it. Acceptance checks 3 and 4 ("model.NewClient... returns non-nil Verifier") pass only for direct NewClient, not the documented user path.

   Evidence: spec.md:10-18 (User outcome / Entry point), config.go:75-84 (broken key gate), proof.md:98-99 (claims delivery of NewClient routing).

**Required to address:**

1. Fix the switch in internal/model/config.go FromEnv so `case "google":` is a real case using envOrAlias (and vertex bypass stays). Ensure `GOOGLE_API_KEY` alone satisfies the key check for google prefix.

2. Update spec.md "Planned touchpoints" to include the actual files changed (config.go, provider_test.go) or document the divergence in proof.md with Rule 2 elements.

3. Add a unit test exercising FromEnv("google/...") with only GOOGLE_API_KEY set (no SWORN_ alias) to prevent regression on the user path.

4. Run `gofmt -l -w` on changed .go files (config.go, google.go, provider.go, google_test.go are not formatted).

**Gate checks summary:** Gate 1 FAIL (user path broken), Gate 2 FAIL (touchpoint mismatch), Gate 3 PASS (unit tests exist and pass), Gate 4 PASS (unit reachability), Gate 5 PASS (no silent deferrals), Gate 6 FAIL (scope claim vs reality).

**Next step for human:** Re-open `/implement-slice S12-google-driver 2026-06-19-safe-parallelism` in a fresh terminal to address the numbered violations. Do not re-verify until fixed.

**Verifier was fresh context:** yes (no implementer transcript loaded).

### 2026-06-24 — fix session (address verifier FAIL)

**Entry state:** `failed_verification` (verifier returned FAIL with 3 violations:
Gate 1 broken user path, Gate 2 touchpoint mismatch, Gate 6 scope claim vs
reality).

**Root cause:** The `FromEnv` key-check switch in `internal/model/config.go` had
a mangled line — `case "google":` was appended to the same line as a `//`
comment on the vertex case (`key = "adc" // ... required\tcase "google":`), so
the entire `case "google":` clause was swallowed into the comment and google
fell through to the default branch. `GOOGLE_API_KEY` alone therefore failed with
"SWORN_GOOGLE_API_KEY not set", breaking the documented `sworn run` user
outcome. Two further mangled lines existed in the same region
(`provider.go:43` CloudflareKey appended to GoogleCloudLocation line;
`provider.go:159` bedrock body appended to the case line; `config.go:136`
CloudflareKey appended to GoogleCloudLocation line) — all the same class of
comment/run-together mangling. `google.go:81` had a `}`-then-comment run-together
and `google_test.go:206` was missing a blank line between funcs.

**Fixes applied:**

1. **`internal/model/config.go`** — rewrote the `FromEnv` switch so `case "vertex"`
   and `case "google":` are real executable cases on their own lines; vertex uses
   ADC (key = "adc"), google uses `envOrAlias("GOOGLE_API_KEY",
   "SWORN_GOOGLE_API_KEY")`. Fixed the mangled `CloudflareKey` line in
   `swornProviderConfig`. `GOOGLE_API_KEY` alone now satisfies the pre-dispatch
   key check for the google prefix.
2. **`internal/model/provider.go`** — fixed the mangled `CloudflareKey` line in
   `ProviderConfigFromEnv` and the mangled `bedrock` case in `NewClient`.
3. **`internal/model/google.go`** — fixed the `}`-then-comment run-together in
   the error-handling block of `Verify`.
4. **`internal/model/google_test.go`** — added a blank line between
   `TestNewClient_VertexRouted` and `TestNewGoogleGemini_MissingKey`; added three
   regression tests covering the user path:
   - `TestFromEnv_GoogleWithCanonicalKey` — the exact broken path:
     `FromEnv("google/gemini-2.0-flash")` with only `GOOGLE_API_KEY` set
     (SWORN_DIRECT=1, isolated XDG_CONFIG_HOME) → `*Google`.
   - `TestFromEnv_GoogleWithAliasKey` — SWORN_GOOGLE_API_KEY alias fallback.
   - `TestFromEnv_GoogleMissingKey` — fail-closed when neither key is set.
5. **`gofmt -l -w`** on the four slice .go files flagged by the verifier
   (config.go, google.go, provider.go, google_test.go). Pre-existing unformatted
   files outside the slice scope (env.go, errors.go, oai.go) were left untouched
   — verified unformatted at start_commit too (not a regression).
6. **`spec.md` "Planned touchpoints"** — added `internal/model/config.go` and
   `internal/model/provider_test.go` explicitly (resolves Gate 2; the journal's
   earlier design pin to add config.go had never propagated to the spec).
7. **`status.json`** — added `provider_test.go` to `planned_files`; transitioned
   state `failed_verification → in_progress → implemented`.

**Test results (live, this session):**
- `go test ./internal/model/... -run Google`: 13 PASS, 1 SKIP (live), 0 FAIL
- `go test ./internal/model/...`: all model tests PASS
- `go build ./...`: PASS
- `go vet ./...`: PASS
- `gofmt -l` (slice files): clean

**Pre-existing failure note:** `TestCmdRun_Parallel` in `cmd/sworn` fails when
run in the full `go test ./...` suite. Confirmed pre-existing (fails at
start_commit b60e4a3 too — a cross-package HOME/config test-isolation issue in
the cmd/sworn harness). No changes to `cmd/sworn` in this slice.

**Open deferrals:** None.

**Divergence from plan:** see proof.md §Divergence from plan #4 (touchpoint scope
expanded to config.go + provider_test.go; spec updated to match, not deferred).
