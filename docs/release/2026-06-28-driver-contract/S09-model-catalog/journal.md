# Journal — S09-model-catalog

## 2026-07-10 — Design TL;DR session

Track `T5-catalog` had no worktree yet; materialised
`track/2026-06-28-driver-contract/T5-catalog` from `release-wt/2026-06-28-driver-contract`
tip (cec87c7, T4-resolution-loop already merged, dependency gate clear —
`dependsOnTracks: [T4-resolution-loop]`, `blocked: false` per board oracle).

Read `spec.json`, the release `intake.md` N-11/A-05 decision history, the
landed `internal/driver/registry` (S05) and `internal/model.ProviderConfig`
(S08 pricing lives in `internal/model/client.go` `PriceForModel`), and the
existing `cmd/sworn/capabilities.go` verb this new one sits alongside.

**Key design finding**: AC-01's "determined via registry enumeration" can't
be read literally — `internal/driver/registry.Default()` only registers 4
driver entries and explicitly excludes Google/Ollama by design (its own
header comment: "verify-only providers... stay on the one-shot utility
path"), but S09's own touchpoints require exactly those two providers in the
catalog. Extending the registry is out of scope for this slice. Resolved as
design decision D1: `catalog.go` runs its own no-dispatch credential check
against `model.ProviderConfig` uniformly across all 7 providers, rather than
partially reusing the registry for 5 and side-channeling the other 2. Full
rationale in `design.md`.

Three more Type-2 decisions recorded (D2: Ollama's per-model `/api/show`
call, N+1 against a local daemon; D3: Ollama always attempted, keyless,
mirrors `claude-cli`'s registry precedent; D4: Google's `tools` annotation is
unconditionally `unknown` — `supportedGenerationMethods` has no explicit
tool-support signal per spec.json's own rationale). All four are narrow/local
to this one new package+command, self-classified Type-2, no Type-1 human
ratification required per Rule 9 — flagged to the Captain in `design.md` and
`status.json.design_decisions` for review anyway.

**Rule 2 deferral**: pricing display (OpenRouter's `pricing` block is
free/wire-honest and could annotate `$/1M`) is left out entirely — no AC
requires it, and `PriceForModel` is keyed by the registry's fully-resolved
`provider/model` ID, not catalog's raw per-provider wire IDs, so wiring it in
would need its own normalisation pass. Why: no AC requires it, adding an
unasked capability-shaped surface risks scope creep. Tracking: none filed —
raising to the Coach at design review whether a follow-on issue is wanted.
Acknowledgement: pending this design review.

No production code written this session (Rule 9 gate — design review
precedes implementation). `design.md` written; `status.json` ->
`state: design_review`.

Next: `/design-review S09-model-catalog 2026-06-28-driver-contract`, then
Coach acknowledgement (`DECISION: PROCEED`) before implementation resumes.

## 2026-07-11 — Implementation session

Verified the Coach ack was committed on the track branch
(`captain-proceed.md` @ 54203ed, `DECISION: PROCEED`) before proceeding —
per the task's own instruction, this is the resolved dispositions list I
implemented against, not just design.md as originally written.

**Dispositions applied:**

1. design.md's HTTP-client-convention section corrected: `anthropic.go` uses
   `anthropic-sdk-go` (an ADR-0007 exception), not stdlib `net/http` like
   `oai.go`/`ollama.go`. Documentation fix only — `catalog.go` was always
   designed stdlib-only (`models.list` needs no SDK dispatch machinery), so
   no code changed as a result. Landed in the start-implementation commit
   (`0c4d4e6`).
2. D1 (uniform no-dispatch credential check across all 7 providers) shipped
   as designed — `catalogProviderDefs` in `internal/model/catalog.go`.
3. Pricing-column deferral now carries all three Rule 2 legs: why (in
   design.md), tracking (`sworn#92`), acknowledgement (`captain-proceed.md`
   pin 3). Cited in `proof.json` `not_delivered` and `status.json`
   `open_deferrals`.
4. `cmd/sworn/main.go` left untouched — self-registration precedent
   (`init()` + `command.Register`, matching every other verb). Recorded as
   a touchpoint divergence in `proof.json`.
5. `TestModelsCommand` (`cmd/sworn/models_test.go`) named explicitly in
   `status.json` `reachability_artifacts` and `proof.json` `reachability` —
   drives the registered `models` command end to end (Rule 1), not a leaf
   `internal/model/catalog.go` unit test.

**Implementation notes / divergences found mid-session (all recorded in
`proof.json` `divergence`, not silently absorbed):**

- Ollama's `/api/show` call is implemented as the real Ollama API's
  documented `POST` + `{"name": <model>}` body shape, not design.md's
  table-prose "GET {host}/api/show". Mechanical correctness fix at the
  HTTP-verb level (D2 itself — "call /api/show per model" — is unchanged),
  not a design decision requiring re-review.
- `TestListCatalog_OllamaAlwaysAttempted` (D3) points `OllamaHost` at an
  explicit closed port (`http://127.0.0.1:1`) instead of relying on the
  env-default host design.md's test-plan prose implies. This dev machine
  runs a real local Ollama daemon (confirmed via `curl
  http://localhost:11434/api/tags` returning real models) — asserting on
  the env-default host would have made the test environment-dependent and
  flaky. Same behaviour under test (Ollama always attempted with zero
  configured credentials), deterministic failure mode instead.
- `sworn lint coverage` false-FAILs "read spec.md: no such file" on this
  spec-v1 (`spec.json`) slice — the documented `feedback_releaseverify_specmd_false_fail`
  hazard, not specific to this slice. `sworn llm-check -type
  ac-satisfaction` requires a configured model; this implementer
  environment has zero provider API keys. Both declared in `proof.json`
  divergence rather than contorted around.
- `sworn verify` (deterministic first-pass) required a resolvable
  `--verifier-model` even on the non-agentic path, because this machine's
  local `~/.config/sworn/config.json` already names a verifier model with
  no credentials here. Ran with `-verifier-model openai/gpt-4o-mini` and a
  dummy `SWORN_OPENAI_API_KEY` (client construction only checks
  non-empty; the deterministic path never dispatches — confirmed by
  reading `verify.go`'s own comment). Verdict: PASS, `cost_usd: 0`.
- Checked `proof.json`'s `delivered`/`not_delivered`/`divergence` arrays
  against the embedded `internal/baton/schemas/proof-v1.json` schema
  directly (`baton.ValidateSchema("proof-v1", ...)`): the schema wants
  plain strings, but every slice in this release (including
  already-verified S08) uses an `{item, evidence}` object shape and fails
  the same way. Pre-existing repo-wide drift, not this slice's to fix —
  kept the established convention for consistency rather than deviating
  unilaterally.

Full suite (`go test -count=1 -timeout 300s ./...`) green: 47 packages ok,
0 FAIL, zero regressions in any untouched package. `status.json` ->
`state: implemented`. Stopping here per role boundary — no verifier prompt
run in this session.

## Verifier verdicts received

### 2026-07-10 — Round 1: PASS (fresh-context verifier)

All six verification gates passed against `track/2026-06-28-driver-contract/T5-catalog`
HEAD `850ca98` (S09 code at `4fbeaac`; `850ca98` is the forward-sync merge of
`release-wt` that cleared drift before verify — its S10 spec.json delta is
expected merge noise, not S09 scope).

- **Gate 1 (user-reachable outcome):** PASS. `cmdModels` self-registers via
  `init()` + `command.Register` into the process command registry that
  `main.go:dispatch` resolves through (`main.go:93 command.Lookup(name)`) — the
  exact runtime integration point, not a test-only registry. `TestModelsCommand`
  drives `command.Lookup("models").Run(...)` end-to-end (Rule 1, not the leaf
  catalog unit).
- **Gate 2 (touchpoints):** PASS. Four code files added (`cmd/sworn/models.go`,
  `cmd/sworn/models_test.go`, `internal/model/catalog.go`,
  `internal/model/catalog_test.go`) match spec touchpoints; `cmd/sworn/main.go`
  intentionally untouched (self-registration precedent) — declared divergence,
  captain-proceed pin 4.
- **Gate 3 (tests exercise the integration point):** PASS. Re-ran all named
  tests in a fresh window — targeted, verbose subtests (7 provider-class
  `TestCatalogAnnotations` cases), and full `go test -count=1 -timeout 300s
  ./...` (all packages ok, 0 FAIL). `go build`, `go vet`, `gofmt -l` clean.
- **Gate 3b (ac-satisfaction LLM):** skipped, no LLM provider configured
  (non-blocking). Manual adversarial AC walk: AC-01..AC-04 all satisfied.
  AC-01's "registry enumeration" parenthetical is dispositioned by the
  Coach-ratified D1 (design review, captain-proceed pin 2) — an upstream gate
  the verifier trusts; the testable outcome (no-dispatch credential-based
  availability + grouped-by-prefix listing) is delivered.
- **Gate 4 (reachability):** PASS. `cli-run` artefact names the `sworn models`
  gesture and drives the real entrypoint; annotations + grouping asserted on
  captured stdout.
- **Gate 5 (no silent deferrals):** PASS. Changed-source grep clean; the sole
  deferral (pricing display) carries all three Rule 2 legs — why + tracking
  (`sworn#92`) + acknowledgement (`captain-proceed.md` pin 3, `acknowledged_by:
  Brad (Coach)`).
- **Gate 6 (design conformance):** auto-pass — no `docs/baton/design-fidelity.json`,
  non-UI Go CLI.
- **Gate 7 (scope matches):** PASS. Every `delivered` evidence reference
  resolves to real, working code/tests.

Fail-closed capability honesty independently confirmed: `annotateStringListTools`
returns `unknown` on an absent wire field and `yes` only on an explicit `tools`
entry; `annotateMistralTools` maps nil→unknown; bare-ID providers (OpenAI/Groq/
Anthropic) and Google are unconditionally `unknown` — no model renders capable
from absent/unknown metadata, and tests assert the absent-field→unknown and
Google-present-field→unknown edges. Zero model dispatches confirmed: availability
is credential-presence only, every HTTP path is a models/list-or-metadata
endpoint (no completion/chat/generate strings in `catalog.go`), and
`TestListCatalog_NoDispatchPaths` fails on any non-list path. Both declared
divergences (Ollama `/api/show` POST body shape; D3 closed-port `127.0.0.1:1`
test) match the code and mask no real gap.

`status.json` -> `state: verified`, `verification.result: pass`,
`verifier_was_fresh_context: true`.
