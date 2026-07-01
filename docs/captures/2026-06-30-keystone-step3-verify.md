---
title: 'Rule-7 verdict — ADR-0011 keystone step 3 (verifier-verdict-v1 pilot)'
description: 'Fresh-context adversarial verification of commit 869f07c: the agentic verifier emits a schema-constrained verifier-verdict-v1 object via ChatStructured, validated fail-closed, replacing the prose HasPrefix scrape. Verdict: PASS.'
date: 2026-06-30
---

# Rule-7 verdict — ADR-0011 keystone step 3

## Verdict

**PASS**

Every checked Delivered claim is satisfied against live repo state at HEAD
`869f07c` (working tree clean). `go build`, `go vet`, the targeted tests, and the
full `go test ./...` suite all pass. The prose verdict scrape and `extractViolations`
prose-split are genuinely deleted; `RunAgentic` emits/validates via the canonical
schema and fails closed to INCONCLUSIVE at every boundary; the schema's `allOf`
really enforces FAIL/BLOCKED⇒violations≥1 and is exercised by a non-tautological
test; the live sentinel/Err contract survives and `scheduler` compiles green.

## Files-changed reconciliation

`git show --stat 869f07c` lists 13 files; all 13 appear in the bundle's "Files
changed" section. The bundle additionally lists `docs/captures/2026-06-30-keystone-step2-verify.md`
as a working-tree `A` (the prior step-2 verdict doc), which is NOT part of this
commit — the bundle annotates it "(step-2 Rule-7 verdict, prior)", so this is an
accurate working-tree snapshot, not a mismatch. No undeclared file in the commit.

## Test results (regenerated live, not trusted from bundle)

- `go build ./...` → exit 0
- `go vet ./internal/verify/ ./internal/baton/... ./internal/orchestrator/ ./internal/run/ ./internal/verdict/` → exit 0
- `go test ./internal/verify/ -run RunAgentic -v` → 9/9 PASS (Pass/Fail/Blocked + 6 fail-closed INCONCLUSIVE)
- `go test ./internal/baton/ -run VerifierVerdict -v` → PASS
- `go test ./...` (full suite, 300s timeout) → exit 0; all packages `ok`, including
  `internal/scheduler`, `internal/orchestrator`, `internal/run`, `internal/verify`,
  `internal/baton`, `cmd/sworn` (24.6s). No newline-fusion / hang regression.

## Per-claim findings

1. **Prose scrape deleted.** `grep parseVerdict|firstVerdictLine|stripMarkdown
   internal/verify/verify.go` → only a NOTE comment at line 259-262, no func defs.
   `grep -rn HasPrefix internal/verify/ internal/orchestrator/` → all hits are
   comments OR the boundary-mock diff-line check at `verify.go:394`
   (`strings.HasPrefix(t,"+")`), NOT a verdict scrape. CONFIRMED.

2. **extractViolations deleted from slice.go.** `grep` → only NOTE comment at
   `slice.go:779`. The surviving `func extractViolations` is `internal/mcp/context.go:101`
   (out of scope per instructions). CONFIRMED.

3. **RunAgentic fail-closed structured path.** `verify.go:169` type-asserts
   `model.StructuredOutput` (→ INCONCLUSIVE `verifier_structured_unsupported` if
   not); `:175` calls `ChatStructured`; dispatch error `:177`, empty choices `:180`,
   malformed JSON `:198/214`, marshal error `:205`, schema-invalid `:208` all return
   `verdict.Inconclusive`. `acceptStructuredVerdict` (`:190`) stamps schema_version/$schema,
   marshals, calls `baton.ValidateSchema("verifier-verdict-v1", …)` BEFORE mapping
   the typed `structuredVerdict`. CONFIRMED.

4. **Schema allOf + non-tautological test.** `verifier-verdict-v1.json:14` verdict
   enum = 4 values; `:42-46` `allOf` if verdict∈{FAIL,BLOCKED} then
   `required:[violations]` + `violations.minItems:1`. `TestValidateSchema_VerifierVerdict`
   (validate_schema_test.go:47-68) calls real `ValidateSchema` with 4 cases: PASS
   accepted, FAIL+violations accepted, FAIL-no-violations REJECTED, bad-enum REJECTED.
   `TestRunAgenticFailWithoutViolationsInconclusive` (test:154) proves the end-to-end
   fail-closed. CONFIRMED — not a tautology.

5. **Dead interpreter gone, sentinel survives.** `grep func.*Interpret|parseInterpretResult|captureVerifier`
   → only NOTE comments (interpreter.go:6, slice.go:754). `ErrInterpretInconclusive`
   (interpreter.go:26) + `InterpreterInconclusiveSentinel` (`:21`) survive;
   `scheduler/worker.go:304,361,444` use the sentinel. Full suite green covers
   compilation. CONFIRMED.

6. **verdict.Result + slice.go wiring.** `verdict.go:42` `Violations []string`,
   `:46` `Routing string`. `slice.go:583` calls `RunAgentic`→`lastVerdict`;
   BLOCKED path `:671` `st.Verification.Violations = lastVerdict.Violations`, `:680`
   `st.Verification.Routing = lastVerdict.Routing` — both off the typed verdict,
   not a prose split. The `len==0` fallback (`:672`) is a guard, not a scrape.
   CONFIRMED.

## Not-delivered honesty

All four are genuine Rule-2 deferrals (why + tracking + ack), not hidden failures:
- No live provider dispatch (no keys; same boundary as Step 2; follow-up smoke). Genuine.
- `inconclusive` not in slice-status leaf enum → tracked **#37** (`gh issue view 37`
  → state OPEN, title matches "Complete D4: add 'inconclusive' to slice-status-v1
  leaf result enum"). Genuine, verified open.
- Routing not yet emitted by verifier prompt (plumbing carried end-to-end; prompt
  authoring is a sibling slice). Genuine.
- baton-web publishing of the `$id` URL (schema is embedded, not required for the
  in-binary path). Genuine.

## Divergences

All three are DECLARED in the bundle (acceptable):
(a) Retarget from dead `orchestrator.Interpret` to live `verify.RunAgentic` —
declared in the continuation-handshake note + commit body; Brad acknowledged.
(b) Canonical schema `required` omits the identity triple — declared, forced by
ADR-0011 §3.3(g) (judgement-only payload); documented in the schema's own
`description`. Confirmed at `verifier-verdict-v1.json:8`.
(c) Two `run_test.go` prose-tolerance tests updated to the structured/fail-closed
reality — declared; full `internal/run` suite green. No UNdeclared behaviour change found.

## Reachability

`TestRunAgenticPass` drives the integration point `RunAgentic`→`ChatStructured`
(not a leaf): it asserts the system message is the verifier role prompt, the user
payload carries SPEC/DIFF/PROOF, the emit schema handed to the driver carries the
`verifier-verdict-v1` title + verdict enum, and the verdict/rationale/token-split
(700/300) come off the typed object. The six INCONCLUSIVE boundary tests are real,
distinct, passing tests each asserting `verdict.Inconclusive` with the correct
`FailedGate`. Loop-level reachability is covered by the green `internal/run` suite
(RunSlice→RunAgentic).
