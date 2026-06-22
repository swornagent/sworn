# Design TL;DR — S38-verifier-blocked-violations

## §1. User-visible change

When a verifier issues a BLOCKED verdict, the concrete defect and proposed
amendment will now always appear in `status.json`'s `verification.violations`
array — not only in `journal.md` prose. A new deterministic first-pass gate in
`release-verify.sh` rejects any `status.json` with `result: blocked` and an
empty `violations` array, so a malformed BLOCKED can never be handed off to the
planner with a blank reason. The verifier role prompt is updated to make this
requirement explicit in the BLOCKED output branch.

## §2. Design decisions not in spec

1. **Gate location: `release-verify.sh` (status.json section) + Go unit in
   `internal/verify/`.** The spec leaves the gate's location open.
   `release-verify.sh` is the right surface because it already reads and
   validates `status.json` at every invocation and is the canonical first-pass
   gate for the harness. A Go function in `internal/verify/` makes the check
   callable from `sworn verify` or the run loop if pattern demands it later —
   but the harness path (`release-verify.sh` → baton) is the immediate delivery
   vehicle. The unit test validates the Go function.

2. **Verifier prompt change is additive only.** Two sentences added to the
   BLOCKED output format spec: one requiring non-empty
   `verification.violations`, one noting the gate will reject an
   empty-violations BLOCKED. No structural rewrite — keeps the prompt diff
   reviewable.

3. **Gate checks `status.json` verbatim, not a Go-typed struct.** The
   `release-verify.sh` gate inspects `verification.result` and
   `verification.violations` via `jq` — same pattern as the existing
   state/field checks in Check 2. No new parsing infrastructure needed.

4. **Gate is forward-looking only.** It validates at-write time, not
   retroactively. A slice already in `verified` state with a historical
   empty-violations BLOCKED does not retroactively fail — the check only fires
   on `verification.result == "blocked"`.

5. **Unit test and reachability artefact are the same thing.** The spec
   prescribes running the Go check against crafted JSON. The unit test
   (`TestBlockedRequiresViolations`) serves double duty as both the required
   unit test and the reachability artefact (a `go test` run proving the gate
   fires and clears correctly).

## §3. Files I'll touch grouped by purpose

- **Verifier prompt canonical source:** `internal/prompt/verifier.md` — add the
  BLOCKED-branch violation-population requirement (spec AC1).
- **Deterministic gate (Go):** `internal/verify/validate_blocked.go` (new) —
  `ValidateBlockedViolations()` function that checks `result == blocked →
  violations non-empty`.
- **Deterministic gate (test):** `internal/verify/verify_test.go` — add
  `TestBlockedRequiresViolations` — two sub-tests: empty-violations BLOCKED
  fails, populated-violations BLOCKED passes.
- **First-pass harness gate:** `scripts/release-verify.sh` — new check in the
  status.json section that fires when `verification.result == "blocked"` and
  `violations` is empty.
- **Slice artefacts:** `docs/release/.../S38-verifier-blocked-violations/` —
  update `status.json`, generate `proof.md`, append `journal.md`.

## §4. Things I'm NOT doing

- **Not changing the FAIL verdict recording path** — spec explicitly excludes
  this.
- **Not changing the loop's page-rendering format** — spec explicitly excludes
  this.
- **Not retroactively validating historical BLOCKED verdicts** — S24/S06a were
  already cleared by the planner; the gate is forward-looking per spec Risks
  section.
- **Not modifying `verify-slice.md` (the baton slash command)** — the verifier
  prompt is in `internal/prompt/verifier.md`; the slash command routes to the
  role prompt. No change needed there.

## §5. Reachability plan

1. `go test ./internal/verify/ -run TestBlockedRequiresViolations -v` — the
   unit test output is the reachability artefact. It proves: (a)
   empty-violations BLOCKED fails the gate, (b) populated-violations BLOCKED
   passes.
2. `$HOME/.claude/bin/release-verify.sh S38-verifier-blocked-violations
   2026-06-19-safe-parallelism` — the first-pass script run proves the bash
   gate fires in the harness.

## §6. Open questions for the Coach

None.