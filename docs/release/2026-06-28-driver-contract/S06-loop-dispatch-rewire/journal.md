# Journal — S06-loop-dispatch-rewire

## 2026-07-10 — Session 1 (Implementer): design TL;DR produced, halted at design_review

- Grounded the design against live worktree code: `internal/run/slice.go`
  (factory fields + nil-default patch at 57-63/193-201, terminal check at
  ~487, five dispatch sites), `internal/verify/verify.go` (RunAgentic +
  acceptStructuredVerdict), `internal/implement/{implement,ready}.go` (DoR
  agentVerifier seam), `internal/driver/*` (S01-S04 contract + drivers),
  `internal/driver/registry/registry.go` (S05), `internal/model/config.go`
  (FromEnv proxy block — the R-04 duplication source).
- Wrote `design.md`. Ten decisions D1-D10; the ones needing reviewer eyes
  are flagged inline and re-listed in "Design-level risks / pins":
  D2 (RoleCaptain only on in-process drivers; captain-leg resolve failure →
  existing design-gate deferral, not hard halt — an AC-02 interpretation),
  D7 (ProviderConfigFromEnv gains SWORN_* fallbacks so the loop keeps
  honouring SWORN_*-only setups), D9 (delete dead InterpretVerifier field).
- R-03 answered with `driver.TerminalErrKind` (set exactly {auth, credits},
  per the S04 Coach ack binding record) consumed at both the implement leg
  and the verify leg — one predicate, four halt tests planned.
- R-04 answered with extracted `model.ProxyRoute` (single predicate) +
  `model.ResolveLoopClient` as the in-process drivers' FromEnv-equivalent
  client default + registry delegation; three-part reachability test
  (advertise / observe proxy hit server-side / SWORN_DIRECT flips both).
- Discovery beyond the spec's touchpoints (named in design.md "Files to
  touch", to land in this slice because the seam forces them):
  cmd/sworn/verify.go's agentic path also calls RunAgentic;
  run.Options carries duplicate factory fields; inprocess default timeout
  (300s) is shorter than DefaultImplementTimeout (15m) so DispatchInput.Timeout
  must be passed explicitly or implement legs get silently capped.
- **R-02 memory note (named here per the spec's mitigation):** the
  S05-strict-reader lesson — a tightened reader/contract regressed board.json
  fixtures in OTHER packages (internal/board + cmd/sworn). Before any
  transition to `implemented`, run the FULL `go test -timeout 300s ./...`;
  the highest cross-package risk for this slice is cmd/sworn (three files
  compile against re-cut signatures) and internal/scheduler (worker fakes).
- No production code written this session (Rule 9 — design review before
  code). State: planned → design_review. Next: `/design-review
  S06-loop-dispatch-rewire 2026-06-28-driver-contract` (Captain), then Coach
  acknowledgement, before implementation resumes.

## 2026-07-10 — Session 2 (Implementer): implemented

- Gate walk-in: Captain review (review.md, NEEDS_COACH + constitutional
  flag) and Coach acknowledgement (captain-proceed.md, PROCEED, 6
  dispositions) verified on-branch @e266595 before any code. design_decisions
  (D1/D2/D3/D6/D7) populated per pin 6 BEFORE in_progress; start_commit
  16a8160.
- Implemented bottom-up: driver contract (ErrKindCredits + TerminalErrKind,
  pin 3 citation in the doc comment) → model (ProxyRoute + proxyClient +
  ResolveLoopClient, FromEnv refactored onto the shared predicate;
  ProviderConfigFromEnv widened per-key with envOrAlias) → inprocess
  (RoleCaptain + tool-less dispatchCaptain; newClient default →
  ResolveLoopClient; errKindCredits → contract const) → registry
  (proxyRouting delegates to model.ProxyRoute — the hand-synced duplicate
  deleted) → design.Generate / captain.Review / implement.Run /
  verify.RunAgentic re-cut onto driver dispatch → slice.go upfront
  resolution + leg rewires + Result-sourced telemetry → run.go field
  deletions → cmd/sworn/verify.go agentic path onto the registry seam.
- Pin dispositions honoured: (1) captain-leg Resolve failure → wrapped role
  error inside recordDesignGateDeferral, run proceeds
  (TestRunSliceCaptainResolutionFailureDefersAndProceeds); implement/verify
  hard-error pre-dispatch (TestRunSliceResolutionFailure, zero dispatches
  asserted). (2) every upfront Resolve failure wrapped `RunSlice: resolve %q
  for role %q` so model+role+alternatives all appear. (3) TerminalErrKind
  {auth, credits} with the S04 T3 captain-proceed.md citation; four halt
  tests + non-terminal-continues at both consumption points. (4) three-part
  R-04 reachability test (TestProxyRoutingAdvertisedEqualsActual): advertise
  / server-side-observed proxy dispatch via httptest SWORN_PROXY_URL /
  SWORN_DIRECT flips both. (5) canonical-wins + SWORN_-fallback precedence
  tests for all 14 widened keys. (6) design_decisions recorded pre-code.
- Flags honoured: InterpretVerifier deleted, no shim (a); fused comments
  repaired at slice.go:694, verify.go:123, run.go:27+229, corruption grep +
  gofmt clean after the sweep (b); verify acceptance tests fed driver.Result
  with assertions minimally diffed; the verifier_structured_unsupported arm
  is unrepresentable at the engine and its test adapted to the
  dispatch-error arm per design D4 (c).
- Test-harness consequence of AC-04 (scan includes _test.go): every
  wire-typed agent fake in internal/run + internal/verify tests became a
  driver fake injected via registry.New()+Register (fakeDriver/testRegistry
  in run_test.go; fakeVerifierDriver in verify_agentic_test.go;
  fakeImplDriver applying file effects in implement_test.go — the tool loop
  now lives behind the driver, so the fake writes files itself).
- S04/S05-era assertions that captain is UNDECLARED updated to the S06
  contract (TestInprocessIdentities, TestResolveRoleFailFast — the
  fail-fast case is now a subprocess prefix; in-process captain resolution
  succeeds).
- D7 ripple: ambient SWORN_MISTRAL_API_KEY leaked into the pre-existing
  TestProviderConfigFromEnv through the new fallback — the test now clears
  every widened SWORN_* alias first (the R-02 cross-package class, caught by
  the full-suite run).
- Divergences recorded in proof.json: negative ImplementTimeout opt-out is
  bounded at the in-process driver's 300s default (the driver contract has
  no unbounded mode); the verify-leg dispatch is now bounded by
  implementTimeout (pre-S06 the ChatStructured call had no explicit
  deadline).
- First-pass gate: initial run FAILed boundary_mock on a DELETED doc-comment
  line ('fake … authoring' → 'auth' substring) from the removed
  verifierAwareAgent — a scanner false positive (deleted-line scanning +
  substring keyword match), filed as swornagent/sworn#87; re-ran with the
  gate's --deferral declaration citing it → PASS with declared deferral.
  Fixing the scanner is out of S06 scope (R-01 minimal-diff on
  internal/verify).
- Full `go test -count=1 -timeout 300s ./...`: 45 packages ok, 0 FAIL
  (fresh cache, R-02 gate). go vet + gofmt -l clean.
- State: in_progress → implemented. Next: fresh-context
  `/verify-slice S06-loop-dispatch-rewire 2026-06-28-driver-contract`.

## Verifier verdicts received

### 2026-07-10 — PASS (fresh-context verifier, Rule 7)

PASS

Slice: `S06-loop-dispatch-rewire`
Verified against: `0e4265b` (track/2026-06-28-driver-contract/T4-resolution-loop)
Verifier session: `fresh, artefact-only`

Gate walk (all re-run live by the verifier, not recalled from proof.json):

- Gate 1 (user-reachable): RunSlice is the engine loop behind `sworn run`;
  cmd call sites inherit the nil->registry.Default seam
  (TestRunSliceDefaultsNilRegistry); `sworn verify --agentic` re-cut onto the
  same registry seam (cmd/sworn/verify.go:115-127). Reachability test drives
  captain+implement+verify legs end-to-end through a registry-injected fake
  driver — re-run verbose, PASS.
- Gate 2 (touchpoints): all spec touchpoints changed except
  internal/verify/verify_test.go, which holds ONLY the RunFirstPass/first-pass
  tests AC-05 requires unchanged; the verify acceptance tests the spec's R-01
  references live in verify_agentic_test.go (pre-existing at start_commit,
  adapted not rewritten — test-name diff confirms). Files beyond the literal
  touchpoints were all pre-declared in the Coach-ratified design.md "Required
  beyond the literal touchpoints" list. No smuggled or undelivered scope.
- Gate 3 (tests re-run): go build OK; slice packages OK; driver/model/
  captain/design packages OK; FULL `go test -count=1 -timeout 300s ./...`
  = 45 packages ok, 0 FAIL, exit 0; go vet + gofmt -l clean.
  TestRunSliceResolutionFailure asserts model ID + role + registered
  alternatives in the error text AND zero dispatches (strong assertions);
  TestRunSliceCaptainResolutionFailureDefersAndProceeds proves the amended
  AC-02 captain arm (recordDesignGateDeferral + proceed).
- Gate 3b/4b (LLM checks): skipped non-blocking — no LLM provider configured
  in the verification environment ($SWORN_MODEL unset).
- Gate 4 (reachability artefact): both cited cli-run commands re-run PASS.
  R-04 binding artefact TestProxyRoutingAdvertisedEqualsActual verified to
  contain all three parts: (a) Drivers() advertises openai/ ViaProxy under
  the login condition, (b) a registry-resolved REAL in-process driver's
  Dispatch observed server-side at the httptest proxy, (c) SWORN_DIRECT=1
  flips advertisement AND dispatch route together, plus the source predicate
  model.ProxyRoute asserted directly.
- Gate 5 (silent deferrals): grep over changed .go files clean except the
  nominalCostPerToken doc comment — an S08-owned deferral (owning slice
  exists in this release; spec out_of_scope names S08; Coach ack pin 5
  cited). open_deferrals entry (RoleCaptain on subprocess drivers) carries
  all three Rule 2 legs + acknowledged_by; tracking swornagent/sworn#86
  confirmed OPEN via gh. Scanner false-positive tracking swornagent/sworn#87
  confirmed OPEN. Newline-corruption sweep clean; the pre-existing fused
  comments (slice.go:694 region, verify.go:123) verified repaired in the diff.
- Gate 6 (design conformance): `sworn designaudit .` = EXEMPT (not
  ui_bearing). Type-1 decisions D2/D6 carry human decisions citing the Coach
  acknowledgement (captain-proceed.md 2026-07-10).
- Gate 7 (claimed scope): every delivered item's evidence spot-checked live —
  factory fields deleted (grep: only historical comments remain), upfront
  resolution block wraps errors with model+role (Coach pin 2),
  driver.TerminalErrKind = {auth, credits} single predicate consumed at
  slice.go:525 AND verify.go:226 (R-03: both kinds tested at both legs),
  TestNoWireImports scans ".", "../verify", "../scheduler" (AC-04),
  acceptStructuredVerdict retained with baton.ValidateSchema and economics
  sourced from driver.Result (AC-03/AC-05), appendDispatch call sites source
  captain/implementer/verifier records from the leg's Result (AC-05),
  ResolveLoopClient is the in-process newClient default and
  registry.proxyRouting delegates to model.ProxyRoute (R-04/D6),
  canonical-wins + SWORN_* fallback tests present (D7).

Amended AC-02 graded as directed: captain-leg Resolve failure defers via
recordDesignGateDeferral and proceeds; implement/verify legs hard-error
naming model, role, and alternatives BEFORE any dispatch. R-03 and R-04
binding mitigations explicitly verified.

state -> verified.
