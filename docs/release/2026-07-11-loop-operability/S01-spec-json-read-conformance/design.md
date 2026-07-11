# Design TL;DR — S01-spec-json-read-conformance

**Slice:** S01-spec-json-read-conformance · **Track:** T1-conformance · **Release:** 2026-07-11-loop-operability
**State:** design_review (Rule 9 gate — no production code written)
**User outcome:** Every site that reads a slice's machine contract reads `spec.json` (spec-v1) as authoritative, with `spec.md` as legacy fallback only. Most importantly the engine implement leg (`implement.Run`, via `RunSlice`) reads `spec.json`, so `sworn run --parallel` can implement a spec.json-only release instead of hard-failing on a missing `spec.md` (sworn#97).

---

## 1. Approach

Apply the **already-established** ears.go/reqverify.go precedence exactly (ADR-0009): *spec.json-preferred, spec.md legacy fallback, spec.json authoritative on disagreement.* The reference block is `internal/ears/ears.go:238-257` — call `spec.ReadRecord(sliceDir)`; if it returns a non-nil record with ACs, use it; if it returns `(nil, nil)` (spec.json absent = legacy), read and parse `spec.md`; a malformed spec.json (`err != nil`) fails closed.

The read primitive already exists: **`internal/spec.ReadRecord(sliceDir string) (*spec.Record, error)`** (`internal/spec/spec.go:52`) — the "single reader for spec.json" built for sworn#22, returning `(nil, nil)` on absence precisely so callers can fall back. This slice does **not** invent a new reader; it routes the un-migrated sites through this one.

**Single-source of the precedence (AC-04).** The *fallback branch itself* is what is currently duplicated (ears.go inlines it; trace.go:191-195 inlines a spec.md-first variant). To make the precedence one implementation rather than N copies, add one thin helper to `internal/spec`:

```
// LoadSpec resolves a slice's machine contract, spec.json-preferred with
// spec.md legacy fallback. Returns (rec, "", nil) when spec.json exists,
// or (nil, mdText, nil) when only spec.md exists. Fails closed on a
// malformed spec.json or a spec.md read error.
func LoadSpec(sliceDir string) (rec *Record, mdText string, err error)
```

Every site branches on the same two-valued result and keeps its existing spec.md parser for the legacy branch only (the md parsers return package-local types — `[]string`, `[]AcceptanceCriterion`, `[]Result` — so they cannot be collapsed, but the *precedence + file read* now lives in one function). This is the "reused ears.go path" AC-04 accepts, promoted to a named primitive.

## 2. Per-site plan (audit sites → change)

| # | Site (live symbol) | Today | Change | ACs |
|---|---|---|---|---|
| 1 | `internal/implement/implement.go` `Run` :46, spec read :90, prompt build :107-111 | `os.ReadFile(specPath)` (spec.md) → raw bytes injected into implementer prompt; **this is the sworn#97 hard-fail** | `LoadSpec(sliceDir)`; when rec present, render a readable spec block (UserOutcome + AcceptanceCriteria + InScope/OutOfScope) into the prompt; spec.md bytes only on legacy fallback | AC-01, AC-02 |
| 2 | `internal/implement/spec_record.go` `WriteSpecRecord` :51 | Parses spec.md → **writes/overwrites** spec.json | When spec.json already exists (planner-authored), **validate + no-op** — do NOT regenerate from spec.md (R-02); only synthesise spec.json from spec.md on a legacy slice with none | AC-03 |
| 3 | `internal/implement/proof_record.go` `deliveredFromSpec` :170 **and** implement.go `generateProof`/`deliveredItems` :178-269 | Both scrape spec.md checkboxes for `delivered[]` | Derive `delivered[]` from `rec.AcceptanceCriteria` when spec.json present; spec.md scrape on legacy fallback | AC-02 |
| 4 | `internal/run/run.go` `SpecPath` :154/:298; spec.md **write** :285 | Hard-codes `spec.md`; `--task` path writes a spec.md template | `SpecPath` resolves spec.json-preferred; `--task` `setupSlice` emits a **spec.json** record (not spec.md) so the engine stays uniformly on the spec.json path | AC-02, AC-05 |
| 5 | `internal/scheduler/worker.go` :331/:396/:488 build `specPath = .../spec.md` → `RunSliceFn` | Constructs spec.md path | Keep the slice-dir-anchored path (implement.Run resolves via `filepath.Dir(specPath)`), but flip construction to prefer spec.json when present so the passed path is truthful; **no `RunSliceFn` signature change** (see §3 pin) | AC-01 |
| 6 | `internal/gate/coverage.go` `RunCoverage` :243-251 | `os.ReadFile(spec.md)` → `parseAcceptanceChecks` | `LoadSpec`; ACs from rec when present, else spec.md parse | AC-02, AC-04 |
| 7 | `internal/gate/trace.go` :186-196 (AC parse) | **spec.md-first**, spec.json fallback (inverted from the rule) | Flip to spec.json-first via `LoadSpec` (aligns with `resolveNeeds` :406 which already prefers spec.json) | AC-02 |
| 8 | `internal/specquality/specquality.go` :122-152 | `parseExamples` + `extractCriteriaText` from spec.md | Criteria text from `rec.AcceptanceCriteria` when present; **Examples stay spec.md-only** (spec-v1 has no examples field — PIN-2) | AC-02 |
| 9 | `internal/rtm/rtm.go` :139-147 | `parseAcceptanceChecks` + `parseRequiredTests` from spec.md | ACs from rec when present; **Required tests need `test_refs`** which `spec.Record.AC` does not expose today (PIN-1) | AC-02 |

## 3. Design choices + pins for the reviewer

- **PIN-1 (escalate — touches shared read contract): `rtm` required-tests on a spec.json-only release.** `spec.Record.AC` (spec.go:24-28) exposes only `id/text/ears_pattern` — **not** `test_refs`, although the spec-v1 schema permits it (spec.go:19 comment). rtm builds the golden thread AC→test from spec.md's "Required tests" section (`parseRequiredTests` rtm.go:463). On a spec.json-**only** release there is no spec.md, so rtm would see zero required tests → a trace break on exactly the releases this slice must make work. **Proposed:** extend `spec.Record.AC` with `TestRefs []string \`json:"test_refs,omitempty"\`` and source rtm's required tests from it (spec.json-preferred), keeping spec.md fallback. This is additive and reversible (Type-2) but modifies the shared `spec.Record` contract consumed by ears/trace/coverage — flagging for a reviewer nod before I touch it.
- **PIN-2 (mechanical, noted): `specquality` Examples have no spec.json equivalent.** spec-v1 has no `examples` field; `parseExamples` (specquality.go:521) is inherently spec.md-only. On a spec.json-only slice specquality simply finds no examples (examples are optional — not a gate failure). Keeping Examples on the spec.md fallback is correct; only the **criteria text** migrates to spec.json. No contract change.
- **CHOICE-A (Type-2): worker/RunSliceFn signature unchanged.** `implement.Run` derives `sliceDir = filepath.Dir(specPath)` (implement.go:47) and resolves spec.json from the directory, so worker.go/run.go can keep passing a slice-dir-anchored path without a signature ripple through `RunSliceFn` (worker.go:104). Minimal-churn default; noted rather than escalated because it is local and reversible.
- **CHOICE-B (Type-2): `--task` `setupSlice` emits spec.json.** run.go:285 currently writes a spec.md template for the `sworn run --task` on-ramp. To keep the engine on one contract path it will write a spec.json record instead. In-scope item 4 ("stop requiring/writing spec.md as the source of truth") authorises this.
- **Fallback is preserved, precedence is inverted (R-01).** spec.md support is **not** deleted — a pre-spec-v1 (legacy) slice with spec.md and no spec.json still reads via the fallback branch. Dual tests (json-authoritative + md-legacy-fallback) guard the inversion (AC-02).

## 4. Reachability artefact (Rule 1)

`sworn run --parallel` on a spec.json-only release (the `contract-edge-gates` fixture / a freshly `/plan-release`-authored release) reaches the implement dispatch **without** the `implement: read spec: open .../spec.md: no such file or directory` error (implement.go:90-92 today). Captured in proof.json as the exact dogfood failure now cleared (AC-05). Backed by a Go test driving `implement.Run` on a spec.json-only slice (AC-01) at the integration point that owns the affordance (the engine implement leg), not a leaf parser.

## 5. Files to touch (matches spec `touchpoints`)

Production: `internal/spec/spec.go` (add `LoadSpec` + `AC.TestRefs` pending PIN-1), `internal/implement/implement.go`, `internal/implement/spec_record.go`, `internal/implement/proof_record.go`, `internal/run/run.go`, `internal/scheduler/worker.go`, `internal/gate/coverage.go`, `internal/gate/trace.go`, `internal/specquality/specquality.go`, `internal/rtm/rtm.go`.
Tests: `internal/implement/implement_test.go` (+ dual json/md fixtures per package touched).

## 6. AC → change traceability

- **AC-01** (engine implement leg reads spec.json-only) → site 1 + implement_test.go driving `implement.Run`.
- **AC-02** (json authoritative; md legacy fallback, both proven) → sites 1,3,4,6,7,8,9 via `LoadSpec`; dual tests.
- **AC-03** (planner-authored spec.json byte-unchanged after implement run) → site 2 (`WriteSpecRecord` no-op-when-present); byte-equality test.
- **AC-04** (build+targeted tests pass; single shared precedence helper) → `spec.LoadSpec` primitive; `go test ./internal/implement/... ./internal/run/... ./internal/scheduler/... ./internal/gate/... ./internal/specquality/... ./internal/rtm/...`.
- **AC-05** (`sworn run --parallel` reaches implement without spec.md-missing error) → §4 reachability artefact.

## 7. Effort/complexity confirmation

Spec's `high/low/grind` **confirmed**: breadth (~9 sites, one established pattern) drives effort; depth is low **except** PIN-1 (the `spec.Record` contract extension) which is the one non-mechanical decision. Re-scoped nothing.

## 8. Hazards heeded

Newline-eating edit corruption (grep changed `.go` for fused `//`+code, `gofmt -l`, `go vet`); full `go test -count=1 -timeout 300s ./...` before any state transition; release-verify.sh false-FAILs "spec.md missing" on spec-v1 slices (canonical gate is `sworn verify`); boundary_mock scanner false-positives on prose (sworn#87).
