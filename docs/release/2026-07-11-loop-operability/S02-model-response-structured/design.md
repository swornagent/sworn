# Design TL;DR — S02-model-response-structured

Slice: `S02-model-response-structured` · Track: `T1-conformance` · Release: `2026-07-11-loop-operability`
Covers: `N-02` · Effort/Complexity: **low / high (puzzle)** — confirmed (small surface, semantics-preserving transport swap that touches the driver contract; depth over breadth).

Migrate the two loop gates that still **scrape model prose** — the design-TL;DR gate (`internal/design`, which required literal `§1`–`§6` headers) and the reqverify Definition-of-Ready gate (`internal/reqverify`, which scraped a `## RESULTS` prose section) — to the **ADR-0011 schema-constrained structured-output transport** already proven by the verifier-verdict-v1 path (`internal/verify/verify.go` + `internal/driver/inprocess/inprocess_verify.go`). The schema becomes the contract; a capable model that returns valid structured content passes regardless of prose shape (the exact Grok failures). Acceptance semantics are preserved verbatim — only the transport changes.

---

## §1 User-visible change

`sworn run` becomes model-portable at the two gates ADR-0011 missed. Driving the loop with Grok (or any `StructuredOutput`-capable model) no longer fails with:

- `design: model response missing required sections (need §1–§6 headers)` (a Rule 2 deferral), or
- `reqverify: parsing model response: model response missing ## RESULTS section` (a DoR BLOCK)

because those gates emitted valid work in a prose shape tuned to sonnet/deepseek. After this slice both gates constrain the model to emit a JSON object against a schema and read the **typed object**, not prose headers. A model that genuinely lacks structured-output capability fails **closed to a declared Rule 2 deferral** naming the missing capability — never a silent pass, never a prose-format hard fail.

## §2 Design decisions not in the spec

- **D1 (Type-1, architecturally significant — ESCALATE):** The structured-output transport currently lives only on the **verifier** dispatch. `DispatchInput.VerdictSchema` is verifier-named and only `dispatchVerifier` calls `ChatStructured`; `dispatchCaptain` does a tool-less **prose** `Chat`. Both migrated gates are **captain-family** dispatches (`design.Generate` → `RoleCaptain`; `driverVerifier.Verify` for DoR → `RoleCaptain`). So the spine of this slice is: **generalise the schema field to be role-agnostic and make `dispatchCaptain` emit structured output when a schema is present.** This edits the `Driver` contract (ADR-0012). Chosen option: **rename `DispatchInput.VerdictSchema` → `StructuredSchema`** (the field doc already says "or another schema-constrained role"), have both `dispatchCaptain` and `dispatchVerifier` consume it, and return `Result.StructuredJSON` uniformly. Alternative considered: add a parallel `CaptainSchema` field (rejected — two fields for one concept invites drift; the verifier field's own doc already anticipated generalisation). This is a **Type-1** choice (hard to reverse, structural, contract-surface) and needs a recorded human decision + an ADR-0012 amendment note.

- **D2 (ESCALATE — schema home):** Follow the `verifierEmitSchema` precedent exactly: a **lenient, strict-mode-subset** emit schema (no `minLength`/`pattern`/`format`, which break OpenAI strict `response_format`; see `internal/model/structured.go` strict-projection constraint) defined inline as `[]byte` in each consuming package. Two schemas: a **design-tldr** object (the six sections as required string fields) and a **reqverify-results** object (a `results` array of per-AC grades). Chosen: keep these **sworn-local** (inline, `title`-named for the OpenAI json_schema name) rather than adding canonical Baton `*-v1.json` schemas, because — unlike verifier-verdict-v1 — these are **sworn-internal gate transports**, not a cross-tool Baton contract. Open question for the Coach: whether the DoR-results schema deserves a canonical schema for the engine-side `baton.ValidateSchema` fail-closed step (verifier-verdict-v1 has both an inline emit-schema AND a canonical validate-schema). Recommend inline-only for design, and inline + a lightweight sworn-local validate for reqverify. Pin for ratification.

- **D3 (AC-03 fail-closed deferral — ESCALATE):** The verifier path folds *both* "client not structured-capable" *and* "structured emission failed" into `ErrKindProtocol` → INCONCLUSIVE. AC-03 requires these to **diverge**: capability-absent must become a **declared Rule 2 deferral**, distinct from a hard failure. Chosen: `dispatchCaptain` type-asserts `client.(model.StructuredOutput)` (as `dispatchVerifier` does) and, when absent, returns a **new dedicated `ErrKindUnsupported`** (`"unsupported"`) — the gates map *that specific kind* to a deferral record; every other structured failure stays a hard error (fail-closed, never a silent pass). The deferral lands on the existing "not evaluated" precedent, not a new surface: reqverify's `CheckDoR` already has a non-pass/non-fail `ReqverifyPassed=false, "…not evaluated…"` arm (`internal/implement/ready.go`) for the nil-verifier case — the capability-absent deferral routes through that same arm with a capability-naming reason; the design gate returns a sentinel `ErrStructuredUnsupported` the loop caller (`internal/run/slice.go`) surfaces + journals as a deferral. Contract addition (new ErrKind) → Type-1-adjacent, pin for ack.

- **D4 (Type-2 — acceptance semantics preserved):** Design PASS = all six section fields present & non-empty (schema-enforced, replacing the `hasSixSections` substring scrape). DoR PASS = the **identical** per-AC grade logic (`AC missing from results → fail-closed FAIL`; unparseable → FAIL) applied to the parsed `results` array instead of scraped `## RESULTS` lines. `design.md` for humans is **rendered deterministically from the structured fields** (§1–§6 headers generated, not scraped) so `/design-review` still reads a prose doc.

- **D5 (Type-2 — test adaptation, not rewrite):** Existing gate tests keep their semantic assertions; the fake drivers switch from returning prose `ResultText` to returning `StructuredJSON`. Two new tests per gate: (a) the **Grok case** — a stub whose prose lacks literal `§1`–`§6` / `## RESULTS` but whose structured output is valid → PASS; (b) the **capability-absent** case — a stub advertising no structured output → a declared deferral, not a crash.

## §3 Files I'll touch by purpose

- `internal/driver/driver.go` — rename `DispatchInput.VerdictSchema` → `StructuredSchema` (role-agnostic) + update contract doc; ADR-0012 amendment note.
- `internal/driver/subprocess.go` — add `ErrKindUnsupported = "unsupported"` constant.
- `internal/driver/inprocess/inprocess.go` — `dispatchCaptain` consumes `StructuredSchema` via `ChatStructured` (structured path) with the `model.StructuredOutput` type-assert → `ErrKindUnsupported` when absent; prose path unchanged when schema is nil.
- `internal/driver/inprocess/inprocess_verify.go` — update the renamed field reference (`VerdictSchema` → `StructuredSchema`).
- `internal/verify/verify.go` — update the renamed field reference at the verifier dispatch site (no semantic change).
- `internal/design/tldr.go` — define `designEmitSchema`; dispatch captain with the schema; parse the typed design object; **deterministically render** `design.md` from the fields; map `ErrKindUnsupported` → `ErrStructuredUnsupported` deferral. Retire `hasSixSections` (or repurpose it to validate the structured object).
- `internal/design/tldr_test.go` — adapt fakes to `StructuredJSON`; add Grok-pass + capability-absent-deferral tests.
- `internal/reqverify/reqverify.go` — add the structured emit path + `reqverifyResultsSchema`; parse the typed `results` array (replacing `parseGrades`' `## RESULTS` scrape) with the identical fail-closed mapping; surface a `Report.Deferred`/reason for capability-absent.
- `internal/reqverify/reqverify_test.go` — adapt the `parseGrades`/`Run` fakes to structured; add Grok-pass + deferral tests.
- `internal/implement/ready.go` — `driverVerifier` grows the structured dispatch (set `StructuredSchema`, return `StructuredJSON`); map the capability-absent deferral through the existing "not evaluated" DoR arm with a capability-naming reason.
- `cmd/sworn/reqverify.go` — wiring only if the `reqverify.Verifier` interface signature changes.

## §4 Things I'm NOT doing

- Not touching the file-read (spec.json) conformance — **S01** owns that (already verified).
- Not changing the already-migrated verifier/captain **verdict** path (verifier-verdict-v1) beyond the mechanical field rename — it is the precedent, semantics unchanged.
- Not building the native xai driver — **S03** (already verified).
- Not re-tuning prose **prompts** — the fix is consuming structured **output**, not rewording prompts.
- Not sweeping interpreter/orchestrator scrapers off the design/DoR critical path — declared out of scope (spec); noted for a later audit.

## §5 Reachability plan

Both gates are reachable through their loop integration points, not leaf-only:
- **Design gate:** `internal/run/slice.go:335` calls `design.Generate` on the captain driver — the Grok-shaped structured stub exercises the real gate path (dispatch → StructuredJSON → typed parse → rendered `design.md`). Reachability artefact: `go test ./internal/design/... -run Structured` showing a prose-header-less-but-valid-structure input producing a written `design.md` and a PASS.
- **DoR gate:** `internal/implement/ready.go:106` (`CheckDoR` → `reqverify.Run`) is the integration point; the structured stub drives it end-to-end, and the capability-absent stub drives the deferral arm. Artefact: `go test ./internal/reqverify/... ./internal/implement/...` plus the AC-04 combined `go test ./internal/design/... ./internal/reqverify/... ./internal/model/...`.
- Full-suite `go test -count=1 -timeout 300s ./...` before the state transition (newline-eating-corruption + cross-package fixture-regression guard, per memory).

## §6 Open questions / pins for review

1. **[ESCALATE / Type-1] Driver-contract change (D1).** Rename `VerdictSchema` → `StructuredSchema` and give `dispatchCaptain` a structured path. This is architecturally significant (ADR-0012 driver contract). Needs a recorded human decision and an ADR-0012 amendment note. Alternative (parallel field) is available if the Coach prefers minimal contract churn.
2. **[ESCALATE] Schema home (D2).** Sworn-local inline emit schemas vs. adding canonical Baton `*-v1.json` schemas. Recommend sworn-local (these are internal gate transports, not a cross-tool contract); confirm whether the DoR-results emission also needs an engine-side `baton.ValidateSchema` canonical-schema check for parity with verifier-verdict-v1.
3. **[ESCALATE] New `ErrKindUnsupported` (D3).** Confirm a dedicated ErrKind (vs. reusing `ErrKindProtocol`) is the right way to keep capability-absent distinguishable from emission-failure — this is what lets AC-03's deferral be *declared* rather than a hard fail.
4. **[MECHANICAL] `reqverify.Verifier` interface.** Migrating the transport changes the interface method (prose `Verify` → a structured emission). Its callers (`internal/implement/ready.go`, `cmd/sworn/reqverify.go`) and the test fake update in lockstep — a compile-checked change, low risk, flagged for visibility.
5. **[MEMORY-CITED] Newline-eating edit corruption.** Deepseek-class edits have fused `//`-comment + code lines three times on this repo (worker.go, index.md, tools_test.go). After every `.go` edit: `grep -nE '//.*\t+(return|[a-z]+\()'` the changed files, `gofmt -l`, `go vet`, and always full `go test -count=1 -timeout 300s ./...` before any state transition.
6. **[SPLIT OPTION]** The spec's effort note flags a possible design-vs-reqverify split. Recommend **keeping one slice**: the driver-contract change (D1) is the *shared spine* both gates depend on, so splitting would duplicate or serialise the same contract edit. Flag for the Coach to confirm.
