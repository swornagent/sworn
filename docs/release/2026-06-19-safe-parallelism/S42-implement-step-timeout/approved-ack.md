<!-- Coach-extractable section: `coach ack <slice>` reads everything between
     this heading and the next ## heading (or EOF). Keep this content
     verbatim-pasteable into the Implementer session — no surrounding prose. -->

Solid design with 3 apply-inline pins:

1. **Config tier missing from AC5 precedence chain (critical).** `internal/config/config.go` exists and has a JSON reader; `resolveVerifierModel()` in `cmd/sworn/run.go` already calls `config.Load()`. Add `Implementer.Timeout string \`json:"timeout"\`` to `Config`, parse it with `time.ParseDuration`, add `internal/config/config.go` to `planned_files`, and implement `resolveImplementTimeout()` with precedence flag > env > config.Load() > default. This brings AC5 into compliance.

2. **Populate `design_decisions` in `status.json` before transitioning to `in_progress`.** Mirror the 5 §2 decisions using S41's structure (choice, stake_class "Type-2", options, rationale). `sworn designfit` reads this field; absent = trivially-passes, which defeats the gate.

3. **S44 DeadlineExceeded interaction — add one-line ack.** In design.md §2.D2 or §4, note that `context.DeadlineExceeded` is a sworn-local signal (not a `model.Error{Kind}`), so S44's Kind-based routing will leave it on the existing "escalate to next model" path. This forward-documents the seam for the S44 implementer.

Flags (not pins): (a) Strike "matches RetryCap: -1 pattern" from Decision 3 — use "Go zero-value-as-unset (`time.Duration`)." (b) S44 shares `slice.go` and `slice_test.go` — second-lander confines hunks; no action for S42.

§2 decisions D1/D3/D4/D5 ack (all Type-2, mechanical). D2 ack per [[project_provider_error_taxonomy]] — orthogonal to model.Error{Kind} as documented above. §6 open questions: none — ack.

Address pins 1–3 inline during implementation, then proceed to in_progress.

<!-- CAPTAIN-VERDICT
DECISION: PROCEED
CONSTITUTIONAL: no
REASON: Both critical pins are mechanical apply-inline corrections (add config tier to AC5 chain, populate design_decisions); no design re-check needed before code.
-->
