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

## Verifier verdicts received

### 2026-07-12 — PASS (fresh-context /verify-slice)

Verified against track HEAD 83fce7f (track/2026-07-11-loop-operability/T1-conformance).
Fresh-context, artefact-only session; no implementer context loaded.

All seven gates passed:
- Gate 1 (user-reachable outcome): implement.Run via RunSlice — the `sworn run --parallel`
  per-slice engine path — reads spec.json on a spec.json-only slice. AC-05 reachability test
  TestRunSlice_SpecJSONOnly_ReachesImplement drives RunSlice end-to-end and reaches implement
  with the spec.md-missing error asserted absent (the sworn#97 dogfood failure, now cleared).
- Gate 2 (touchpoints): 13 declared touchpoints migrated; the two extra changed files
  (internal/run/slice.go sibling, internal/spec/spec.go shared primitive) are surfaced in
  proof.json divergence with Rule 2 justification; implement_test.go's coverage delivered via
  the new spec_json_read_test.go (same package). No unexplained churn.
- Gate 3 (tests exercise integration point): all named tests re-run green in this session —
  TestRun_SpecJSONOnly_ReadsSpecJSON / _SpecJSONAuthoritative_OverSpecMD / _SpecMDLegacyFallback
  / _PlannerSpecJSON_ByteUnchanged (AC-01/02/03), TestRunSlice_SpecJSONOnly_ReachesImplement
  (AC-05), TestBuild_SpecJSONOnly_GoldenThread (AC-06), TestSetupSlice_WritesSpecJSON. Strong
  assertions driving the integration points, not leaf-only.
- Gate 3b/4b (LLM checks): no SWORN_ANTHROPIC_API_KEY — non-blocking skip; this fresh-context
  pass is the model-backed adversarial check.
- Gate 4 (reachability): spec_json_reach_test.go names the parallel-loop per-slice gesture and
  matches the spec outcome; corroborated live by the worktree-built binary —
  `sworn specquality 2026-07-11-loop-operability` PASS on S01/S02/S03 and `sworn lint ac` reading
  all 6 S01 ACs from spec.json (Violations: none). (The ambient PATH binary is stale and FAILs
  specquality — the stale-binary hazard; verified against a binary built from this worktree.)
- Gate 5 (no silent deferrals): no TODO/FIXME/placeholder in added lines; the one not_delivered
  item (model-backed sworn verify/llm-check, no API key) carries why+tracking(verifier handoff)
  +acknowledgement. gofmt/vet clean; newline-eating-corruption grep clean.
- Gate 6 (design conformance): backend engine slice, non-UI — n/a.
- Gate 7 (scope match): every delivered item maps to a passing test / real artefact.

Full `go test -count=1 -timeout 300s ./...` re-run green (0 failures). AC-04 single shared
precedence helper confirmed: spec.LoadSpec / spec.SpecFilePath / spec.RenderMarkdown /
spec.ReadRecord is the one implementation; all 13 read sites route through it, no N-copy
duplication. AC-03 R-02 guard confirmed: WriteSpecRecord validates (does not regenerate) an
existing spec.json; byte-equality test passes.

Next: /implement-slice S02-model-response-structured 2026-07-11-loop-operability (next slice in track T1-conformance).
