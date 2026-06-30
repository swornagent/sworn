// Package verdict defines SwornAgent's verification result contract.
//
// The contract is deliberately small and fail-closed: anything that is not an
// explicit PASS does not merge. This mirrors the Baton Rule 7 verifier contract
// (PASS / FAIL / BLOCKED / INCONCLUSIVE) that SwornAgent enforces.
package verdict

// Verdict is the outcome of an adversarial verification.
type Verdict string

const (
	// Pass: every gate satisfied; the change may merge.
	Pass Verdict = "PASS"
	// Fail: a concrete acceptance/gate violation was found.
	Fail Verdict = "FAIL"
	// Blocked: verification could not be completed (missing artefact, unrunnable
	// model, unresolved spec). Fail-closed — treated as not-mergeable.
	Blocked Verdict = "BLOCKED"
	// Inconclusive: the verifier could not reach a determinate PASS or FAIL
	// (e.g. ambiguous spec, contradictory evidence, model uncertainty).
	// Fail-closed — treated as not-mergeable, but signals re-verify rather than
	// replan (distinct from BLOCKED).
	Inconclusive Verdict = "INCONCLUSIVE"
)

// Result is the machine-readable verdict emitted by `swornagent verify`.
type Result struct {
	Verdict          Verdict `json:"verdict"`
	FailedGate       string  `json:"failed_gate,omitempty"`
	Rationale        string  `json:"rationale"`
	CostUSD          float64 `json:"cost_usd"`
	InputTokens      int64   `json:"input_tokens,omitempty"`
	OutputTokens     int64   `json:"output_tokens,omitempty"`
	DurationMS       int64   `json:"duration_ms,omitempty"`
	ModelIDConfirmed string  `json:"model_id_confirmed,omitempty"`
	// Violations carries the per-gate violation summaries the verifier emits
	// with a FAIL/BLOCKED verdict. Since ADR-0011 these come off the typed
	// verifier-verdict-v1 record (one string per emitted violation:
	// "gate: description"), replacing the prose-splitting extractViolations
	// scrape. Kept as []string to match state.Verification.Violations until the
	// D6/1b record reconciliation migrates both to the object shape (#37).
	Violations []string `json:"violations,omitempty"`
	// Routing is the blocked-routing owner the verifier may emit alongside a
	// non-PASS verdict ("needs_planner" | "needs_human" | "needs_implementer").
	// Consumed by the BLOCKED halt path to populate status.json verification.routing.
	Routing string `json:"routing,omitempty"`
}
// ExitCode maps a verdict to a process exit code. 0 only for PASS; everything
// else is non-zero so a CI required-check blocks the merge by default.
func (r Result) ExitCode() int {
	switch r.Verdict {
	case Pass:
		return 0
	case Fail:
		return 1
	case Inconclusive:
		return 3
	default: // Blocked or any unknown value -> fail closed
		return 2
	}
}
