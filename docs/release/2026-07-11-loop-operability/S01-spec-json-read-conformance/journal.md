# Journal — S01-spec-json-read-conformance

## 2026-07-12 — Implementer session (in_progress → implemented)

Design review acknowledged (captain-proceed.md, Verdict: PROCEED). Coach ratified
Pin 1 (extend spec.Record.AC with test_refs, AC-06), Pin 2 (fold in
gate/llmcheck.go, lint/touchpoints.go, cmd/sworn/task.go — "sweep ALL sites"),
Pin 3 (record design_decisions before in_progress). Recorded CHOICE-A/B (Type-2)
and the test_refs contract extension (Type-1, Coach-ratified) in status.json.

### Approach
Single-sourced the "spec.json-preferred, spec.md-legacy-fallback, spec.json
authoritative on disagreement" precedence (ADR-0009 / internal/ears) in
`internal/spec`:
- `LoadSpec(sliceDir) (*Record, mdText, error)` — the one precedence helper,
  fail-closed on malformed spec.json.
- `RenderMarkdown(*Record) string` — record → readable prompt/gate body.
- `SpecFilePath(sliceDir) string` — truthful spec path (spec.json if present).
- `AC.TestRefs []string` (`test_refs`) — the spec-v1 field rtm's golden thread
  resolves against (AC-06).

Every un-migrated read site now routes through these:
- implement.Run + generateProof + proof_record: prompt/scope/delivered from the
  record when present (AC-01/AC-02/AC-03).
- spec_record.WriteSpecRecord: VALIDATES an existing planner-authored spec.json
  rather than regenerating from spec.md (R-02 — ears_pattern/test_refs survive
  byte-for-byte, AC-03).
- run.RunSlice: design/captain/verify legs + the first-pass gate resolve
  spec.json via loadSpecText/resolveSpecPath; setupSlice emits authoritative
  spec.json (CHOICE-B).
- scheduler/worker + cmd/sworn/task.go: pass/emit the truthful spec.json path.
- gate/coverage + gate/trace (flipped to spec.json-first) + gate/llmcheck +
  lint/touchpoints: no longer hard-fail on a spec.json-only slice (Pin 2).
- specquality: a spec.json slice (spec-v1 has no examples section) is a no-op
  PASS, not a "no examples" violation (PIN-2).
- rtm: required tests sourced from AC.TestRefs (AC-06 golden thread).

### Key mid-session finding (surfaced, not absorbed silently — Rule 2)
The declared touchpoints list `internal/run/run.go`, but the spec-md-missing
HARD failure in the verify leg lives in `internal/run/slice.go` (:797) — a
sibling in the SAME `internal/run` package. RunSlice's first-pass gate also read
the raw specPath. Both were routed through the shared loader (no other track
owns internal/run; not a cross-track collision). Likewise `internal/spec/spec.go`
(the shared primitive the spec named under in_scope + the Coach-ratified
test_refs extension) was edited though not in the touchpoints array. Recorded in
proof.json `divergence`.

### Verification
- Full `go test -count=1 -timeout 300s ./...` — all packages PASS, 0 failures.
- gofmt -l clean; go vet clean; newline-eating-corruption grep clean.
- Live: `sworn specquality 2026-07-11-loop-operability` PASS on all 3
  spec.json-only slices (was a guaranteed violation pre-PIN-2); `sworn lint ac`
  reads 6 S01 ACs from spec.json (Violations: none).
- New tests: TestRun_SpecJSONOnly_ReadsSpecJSON (AC-01),
  TestRun_SpecJSONAuthoritative_OverSpecMD + TestRun_SpecMDLegacyFallback (AC-02),
  TestRun_PlannerSpecJSON_ByteUnchanged (AC-03),
  TestRunSlice_SpecJSONOnly_ReachesImplement (AC-05),
  TestBuild_SpecJSONOnly_GoldenThread (AC-06), TestSetupSlice_WritesSpecJSON.

### Deferrals (Rule 2)
- Model-backed `sworn verify` / `sworn llm-check --check ac-satisfaction` not run
  this session (no SWORN_ANTHROPIC_API_KEY). Deterministic evidence stands in;
  the fresh-context `/verify-slice` pass is the model-backed gate. Owned by the
  Rule 7 verifier handoff.
- `sworn lint trace` reports NOT TRACEABLE, but the violations are pre-existing
  requirements-fidelity gaps in the release artefacts (N-01/N-02/N-03 absent from
  intake.md; ACs do not cite needs inline) — a planner concern, independent of
  S01's spec-read scope, not a regression. Left for /replan-release or a planner
  pass on intake.md.
