// Package gate provides lint gates for the SwornAgent CLI.
//
// llmcheck.go ports bin/release-llm-check.sh from bash to Go:
// `sworn llm-check` — six deterministic LLM-based quality checks with
// structured prompts and structured JSON output.
//
// Each check type builds a focused system prompt + user payload from the
// slice's spec.md and git diff, calls the model via the provider
// infrastructure, and parses a structured JSON verdict response.
//
// All model calls use temperature 0 (deterministic) and fail closed.
// Stdlib only — zero runtime dependencies beyond the model package.
package gate

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/prompt"
	"github.com/swornagent/sworn/internal/spec"
	"github.com/swornagent/sworn/internal/style"
)

// --- check types ---

// CheckType enumerates the six LLM-based quality checks.
type CheckType string

const (
	CheckACSatisfaction        CheckType = "ac-satisfaction"
	CheckSpecAmbiguity         CheckType = "spec-ambiguity"
	CheckDesignReview          CheckType = "design-review"
	CheckSecurityReview        CheckType = "security-review"
	CheckSemanticCoverage      CheckType = "semantic-coverage"
	CheckMaintainabilityReview CheckType = "maintainability-review"
)

// ValidCheckTypes is the set of recognised --type values.
var ValidCheckTypes = map[CheckType]bool{
	CheckACSatisfaction:        true,
	CheckSpecAmbiguity:         true,
	CheckDesignReview:          true,
	CheckSecurityReview:        true,
	CheckSemanticCoverage:      true,
	CheckMaintainabilityReview: true,
}

// --- data model ---

// LLMCheckReport holds the full structured result of an LLM check.
type LLMCheckReport struct {
	CheckType   CheckType    `json:"check_type"`
	Slice       string       `json:"slice"`
	Release     string       `json:"release"`
	Verdict     string       `json:"verdict"` // "PASS" or "FAIL"
	Findings    []LLMFinding `json:"findings"`
	RawResponse string       `json:"raw_response,omitempty"`
}

// LLMFinding is one structured finding from the model's response.
type LLMFinding struct {
	ID       string `json:"id"`                 // e.g. "F-01"
	Severity string `json:"severity"`           // impact: critical|high|medium|low|info (legacy: FAIL|WARN|INFO)
	Blocking *bool  `json:"blocking,omitempty"` // disposition (llm-check-report-v1, Baton v0.12.0+); nil on legacy payloads
	Title    string `json:"title"`              // one-line summary
	Detail   string `json:"detail"`             // full explanation
}

// IsBlocking reports whether this finding fails its check.
//
// Severity sets the floor and `blocking` may only ESCALATE, never de-escalate.
// Baton v0.12.0 lets a check mark, say, a medium finding as blocking; it does
// not let a model wave through a critical one by claiming blocking: false. A
// critical/high finding that arrives with blocking: false is a model contract
// violation (the security-review prompt states those always block), so we fail
// closed on it rather than believe it.
//
// Two grading vocabularies are recognised because they both exist in the wild:
// five checks graded FAIL/WARN/INFO and security-review graded
// critical/high/medium/low. An unrecognised grade fails closed — an ungradeable
// finding is not a pass.
func (f LLMFinding) IsBlocking() bool {
	if f.Blocking != nil && *f.Blocking {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(f.Severity)) {
	case "fail", "critical", "high":
		return true
	case "warn", "medium", "low", "info":
		return false
	default:
		return true
	}
}

// HasViolations returns true when the report contains a blocking finding, or
// when the model itself declared FAIL.
//
// The model's verdict is corroborating evidence, never the sole authority: a
// PASS verdict cannot clear a blocking finding. Trusting `verdict` alone is
// self-certification, which is precisely what Rule 7 exists to prevent.
//
// This previously string-matched severity == "FAIL" — a value security-review
// never emits, since it grades critical/high/medium/low. The loop was therefore
// dead code for the security check and blocking silently degraded to the model's
// own verdict, so a critical RCE finding alongside a self-declared PASS shipped
// the gate green (sworn#103).
func (r *LLMCheckReport) HasViolations() bool {
	for _, f := range r.Findings {
		if f.IsBlocking() {
			return true
		}
	}
	return r.Verdict != "PASS"
}

// --- prompt templates ---
//
// The six system prompts are NOT defined here. They are vendored from the Baton
// protocol (baton/llm-checks/<check>.md, v0.12.0) and read at runtime via
// prompt.LLMCheck. The prompt body IS the contract, the same way a schema is: a
// check whose wording drifted from the published spec would be a different check
// running under the same name, and no second engine could implement the protocol
// without them. They previously lived here as ~140 lines of inline Go constants,
// which is what made Baton's "canonical runner, not the only possible one" claim
// unsupportable.

// userPromptHeaderFor builds the per-check user-payload header for a project.
//
// projectContext is Baton v0.12.0's {{project_context}} — a REQUIRED substitution,
// not a default. This was previously a const hardcoded to "the SwornAgent project
// (a Go CLI)", sent on every check in every repo: running the checks against a
// TypeScript codebase told the model it was reading a Go CLI, so it graded against
// the wrong priors, silently.
func userPromptHeaderFor(projectContext string) string {
	return "You are evaluating a slice in a release of " + projectContext + ".\n\n" +
		"Below is the slice specification, followed by the git diff of the code change.\n\n" +
		"--- SPECIFICATION ---\n\n"
}

// userPromptDiffSeparator separates the spec from the diff.
const userPromptDiffSeparator = `

--- GIT DIFF ---

`

// --- main entry point ---

// RunLLMCheck executes an LLM-based quality check against a slice.
//
// checkType selects which of the six checks to run.
// sliceDir is the path to the slice's directory (containing spec.md).
// diffContent is the output of `git diff <base>..HEAD` for the slice.
// verifier is the model client to use.
//
// On success, returns a populated LLMCheckReport. The caller should check
// HasViolations() to determine the final verdict.
func RunLLMCheck(ctx context.Context, checkType CheckType, sliceDir string, diffContent string, verifier model.Verifier) (*LLMCheckReport, error) {
	if !ValidCheckTypes[checkType] {
		return nil, fmt.Errorf("llm-check: unknown check type %q (valid: %s)", checkType, validCheckTypeList())
	}

	// Read the machine contract — spec.json preferred (rendered to a readable
	// markdown body for the model), spec.md legacy fallback (ADR-0009). Without
	// this, `sworn llm-check` (including the design-review check the captain
	// flow runs) hard-failed on a spec.json-only slice (AC-02, Coach Pin 2).
	rec, specMD, err := spec.LoadSpec(sliceDir)
	if err != nil {
		return nil, fmt.Errorf("llm-check: read spec: %w", err)
	}
	specContent := specMD
	if rec != nil {
		specContent = spec.RenderMarkdown(rec)
	}

	// The system prompt is the vendored Baton contract, read verbatim — not a Go
	// constant that could drift from the published spec.
	systemPrompt, err := prompt.LLMCheck(string(checkType))
	if err != nil {
		return nil, fmt.Errorf("llm-check: %w", err)
	}

	// Tell the model what it is actually looking at. Detected from the repo, not
	// hardcoded — see DetectProjectContext.
	projectContext := DetectProjectContext(repoRootFrom(sliceDir))
	userPayload := buildUserPayload(projectContext, specContent, diffContent)

	// Call the model.
	rawResponse, _, _, _, err := verifier.Verify(ctx, systemPrompt, userPayload)
	if err != nil {
		return nil, fmt.Errorf("llm-check: model call failed: %w", err)
	}

	// Parse the response.
	result, parseErr := parseLLMResponse(rawResponse)
	if parseErr != nil {
		// Tolerant parse: if we can't extract JSON, fail closed on a blocking
		// finding. An unreadable response is not a pass.
		blocking := true
		result = &llmResponseJSON{
			Verdict: "FAIL",
			Findings: []LLMFinding{{
				ID: "F-00", Severity: "high", Blocking: &blocking,
				Title: "Unparseable model response", Detail: rawResponse,
			}},
		}
	}

	// Build the report.
	sliceName := filepath.Base(sliceDir)
	releaseName := filepath.Base(filepath.Dir(sliceDir))

	return &LLMCheckReport{
		CheckType:   checkType,
		Slice:       sliceName,
		Release:     releaseName,
		Verdict:     result.Verdict,
		Findings:    result.Findings,
		RawResponse: rawResponse,
	}, nil
}

// --- prompt building ---

// buildUserPayload constructs the user message for the model.
func buildUserPayload(projectContext, specContent, diffContent string) string {
	var b strings.Builder
	b.WriteString(userPromptHeaderFor(projectContext))
	b.WriteString(specContent)
	b.WriteString(userPromptDiffSeparator)
	if diffContent == "" {
		b.WriteString("(no diff available — evaluating spec only)")
	} else {
		b.WriteString(diffContent)
	}
	return b.String()
}

// --- response parsing ---

// llmResponseJSON is the expected JSON shape from the model.
type llmResponseJSON struct {
	Verdict  string       `json:"verdict"`
	Findings []LLMFinding `json:"findings"`
}

// parseLLMResponse extracts and parses the JSON verdict from a model response,
// then grades the payload against the published contract (llm-check-report-v1).
// It is tolerant of markdown code fences around the JSON, and intolerant of
// anything else.
func parseLLMResponse(raw string) (*llmResponseJSON, error) {
	cleaned := extractJSON(raw)

	var result llmResponseJSON
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w (raw: %.200s)", err, raw)
	}

	// Grade the payload against the published record BEFORE normalising, so the
	// contract is checked against what the model actually said.
	//
	// The schema is the fail-closed half of sworn#103. It splits severity (impact)
	// from blocking (disposition) and derives the verdict from the findings, so a
	// PASS carrying a blocking finding is schema-INVALID — the exact payload (a
	// critical finding beside a self-declared PASS) that used to pass the security
	// gate green cannot even be expressed. A payload that violates the contract is
	// a FAIL, never a silently-misread pass.
	if verr := baton.ValidateSchema("llm-check-report-v1", []byte(cleaned)); verr != nil {
		blocking := true
		result.Verdict = "FAIL"
		result.Findings = append(result.Findings, LLMFinding{
			ID:       "F-00",
			Severity: "high",
			Blocking: &blocking,
			Title:    "LLM check response violates llm-check-report-v1",
			Detail: fmt.Sprintf("The model's response does not satisfy the published check contract, "+
				"so its verdict cannot be trusted: %v", verr),
		})
		return &result, nil
	}

	// Normalise verdict.
	result.Verdict = strings.ToUpper(strings.TrimSpace(result.Verdict))
	if result.Verdict != "PASS" && result.Verdict != "FAIL" {
		// Unknown verdict — fail closed.
		result.Verdict = "FAIL"
		if len(result.Findings) == 0 {
			result.Findings = append(result.Findings, LLMFinding{
				ID:       "F-00",
				Severity: "info",
				Title:    "Unknown verdict value",
				Detail:   fmt.Sprintf("Model returned verdict %q — expected PASS or FAIL.", result.Verdict),
			})
		}
	}

	return &result, nil
}

// extractJSON attempts to extract a JSON object from text that may be wrapped
// in markdown code fences or have surrounding prose.
func extractJSON(raw string) string {
	s := strings.TrimSpace(raw)

	// Try to find JSON between ```json ... ``` fences.
	if idx := findJSONFence(s); idx >= 0 {
		s = s[idx:]
	}

	// Find the outermost { ... }.
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return s
	}

	// Walk forward counting braces.
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}

	// Unclosed brace — return the whole string.
	return s
}

// findJSONFence returns the start offset of content after a ```json fence,
// or -1 if no fence is found.
func findJSONFence(s string) int {
	idx := strings.Index(s, "```json")
	if idx < 0 {
		idx = strings.Index(s, "```")
	}
	if idx < 0 {
		return -1
	}
	// Find the newline after the opening fence.
	nl := strings.IndexByte(s[idx:], '\n')
	if nl < 0 {
		return -1
	}
	start := idx + nl + 1
	// Find the closing fence.
	end := strings.Index(s[start:], "```")
	if end < 0 {
		return start // no closing fence — take everything after opening
	}
	return start
}

// --- helpers ---

// validCheckTypeList returns a comma-separated list of valid check types.
func validCheckTypeList() string {
	var list []string
	for ct := range ValidCheckTypes {
		list = append(list, string(ct))
	}
	return strings.Join(list, ", ")
}

// --- human-readable output ---

// PrintLLMCheck renders the LLMCheckReport as human-readable text.
func PrintLLMCheck(r *LLMCheckReport) string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(style.Bold(fmt.Sprintf("LLM CHECK — %s", r.CheckType)))
	b.WriteString("\n")
	b.WriteString(style.Dim(fmt.Sprintf("slice: %s  release: %s\n", r.Slice, r.Release)))
	b.WriteString("\n")

	if r.Verdict == "PASS" {
		b.WriteString(style.Success("PASS — no findings\n"))
		b.WriteString("\n")
		return b.String()
	}

	// FAIL — list findings.
	failCount := 0
	warnCount := 0
	for _, f := range r.Findings {
		switch f.Severity {
		case "FAIL", "critical", "high", "medium":
			failCount++
		case "WARN", "low":
			warnCount++
		}
	}
	b.WriteString(style.Danger(fmt.Sprintf("FAIL — %d finding(s)\n", failCount+warnCount)))
	b.WriteString("\n")

	for i, f := range r.Findings {
		sevStyle := style.Danger
		switch f.Severity {
		case "FAIL", "critical", "high", "medium":
			sevStyle = style.Danger
		case "WARN", "low":
			sevStyle = style.Warn
		default:
			sevStyle = style.Dim
		}
		b.WriteString(fmt.Sprintf("  %d. [%s] %s ", i+1, f.Severity, f.Title))
		b.WriteString(sevStyle(f.Detail))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(style.Danger("NOT PASSED"))
	b.WriteString("\n\n")

	return b.String()
}

// JSONLLMCheck returns the report as pretty-printed JSON.
func JSONLLMCheck(r *LLMCheckReport) string {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}
