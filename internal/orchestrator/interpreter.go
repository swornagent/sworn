package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/verdict"
)

// interpreterSystemPrompt is the prompt that instructs a cheap model to classify
// raw model output into a determinate verdict. It is deliberately terse to
// minimise token cost — the interpreter is designed for a 200-token bounded call.
const interpreterSystemPrompt = `Classify the following agent output into exactly one of: PASS, FAIL, BLOCKED, or INCONCLUSIVE.

Rules:
- PASS: the output states that verification passed and all checks are satisfied
- FAIL: the output identifies specific violations or unmet acceptance checks
- BLOCKED: the output indicates verification cannot proceed (missing artefacts, broken tests, unconfigured model)
- INCONCLUSIVE: the output is ambiguous, self-contradictory, or does not state a clear verdict

Reply with ONLY the single word (PASS, FAIL, BLOCKED, or INCONCLUSIVE). No other text.`

// Interpret calls a bounded LLM to classify a raw model output into a verdict.
// It uses the provided model.Verifier for the classification call. If the
// classifier is nil or unconfigured, Interpret returns INCONCLUSIVE immediately
// (fail-closed — AC4).
//
// The interpreter makes exactly one call. It does not retry — if the result is
// INCONCLUSIVE or an error, the caller must handle the escalation (PAGE the Coach).
func Interpret(ctx context.Context, rawOutput string, classifier model.Verifier) (verdict.Result) {
	if classifier == nil {
		return verdict.Result{
			Verdict:   verdict.Inconclusive,
			Rationale: "interpreter: no classifier model configured (nil)",
		}
	}

	text, costUSD, err := classifier.Verify(ctx, interpreterSystemPrompt, rawOutput)
	if err != nil {
		return verdict.Result{
			Verdict:   verdict.Inconclusive,
			Rationale: fmt.Sprintf("interpreter: model error: %v", err),
			CostUSD:   costUSD,
		}
	}

	// Parse the classification response.
	result := parseInterpretResult(text)
	result.CostUSD = costUSD
	return result
}

// parseInterpretResult extracts a verdict from the interpreter model's reply.
// It tolerates leading whitespace, markdown emphasis, and bare code fences.
// Only a leading PASS/FAIL/BLOCKED/INCONCLUSIVE token on the first substantive
// line passes; anything else returns INCONCLUSIVE (fail-closed).
func parseInterpretResult(text string) verdict.Result {
	line := firstInterpretLine(text)
	t := strings.TrimSpace(line)
	// Strip common markdown wrapping.
	t = strings.TrimRight(t, ".*_` ")
	t = strings.TrimLeft(t, "*_` ")
	upper := strings.ToUpper(t)
	switch {
	case strings.HasPrefix(upper, "PASS"):
		return verdict.Result{Verdict: verdict.Pass, Rationale: text}
	case strings.HasPrefix(upper, "FAIL"):
		return verdict.Result{Verdict: verdict.Fail, Rationale: text}
	case strings.HasPrefix(upper, "BLOCKED"):
		return verdict.Result{Verdict: verdict.Blocked, Rationale: text}
	case strings.HasPrefix(upper, "INCONCLUSIVE"):
		return verdict.Result{Verdict: verdict.Inconclusive, Rationale: text}
	default:
		return verdict.Result{Verdict: verdict.Inconclusive, Rationale: text}
	}
}

// firstInterpretLine returns the first non-empty, non-code-fence line.
func firstInterpretLine(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if t == "```" {
			continue
		}
		return t
	}
	return ""
}

// IsInconclusive reports whether an error from RunSlice means the interpreter
// returned INCONCLUSIVE and the worker should PAGE the Coach.
//
// The sentinel is embedded in the error message so callers can detect it with
// strings.Contains without importing the orchestrator package from worker.
const InterpreterInconclusiveSentinel = "INTERPRETER_INCONCLUSIVE"

// ErrInterpretInconclusive is returned by the triage path when the interpreter
// classifies the raw output as INCONCLUSIVE. The worker/router detect this via
// the InterpreterInconclusiveSentinel substring.
func ErrInterpretInconclusive(sliceID string, rawPreview string) error {
	preview := rawPreview
	if len(preview) > 100 {
		preview = preview[:97] + "..."
	}
	return fmt.Errorf("%s: interpreter could not classify output for %s (raw preview: %s)",
		InterpreterInconclusiveSentinel, sliceID, preview)
}