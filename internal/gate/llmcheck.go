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
	"os"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/model"
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
	ID       string `json:"id"`       // e.g. "F-01"
	Severity string `json:"severity"` // "FAIL", "WARN", "INFO"
	Title    string `json:"title"`    // one-line summary
	Detail   string `json:"detail"`   // full explanation
}

// HasViolations returns true when the report contains FAIL findings.
func (r *LLMCheckReport) HasViolations() bool {
	for _, f := range r.Findings {
		if f.Severity == "FAIL" {
			return true
		}
	}
	return r.Verdict != "PASS"
}

// --- prompt templates ---

// systemPrompts holds the system prompt for each check type.
var systemPrompts = map[CheckType]string{
	CheckACSatisfaction: `You are a quality-assurance engineer verifying that a code change satisfies its acceptance criteria.

Your task is to read a slice specification (spec.md) containing acceptance checks, and a git diff showing the code changes. For each acceptance check (AC) in the spec, determine whether the code genuinely satisfies it.

Respond with a JSON object:
{
  "verdict": "PASS" or "FAIL",
  "findings": [
    {
      "id": "F-01",
      "severity": "FAIL" | "WARN" | "INFO",
      "title": "one-line summary",
      "detail": "what the check requires vs what the code delivers"
    }
  ]
}

Rules:
- Each AC must be checked individually. If an AC is not satisfied, emit a FAIL finding naming that AC.
- If the code change is unrelated to an AC, note it as INFO.
- Be specific: cite line ranges, function names, or file paths.
- If every AC is satisfied, verdict is PASS with zero FAIL findings.
- Temperature 0 — be deterministic and reproducible.`,
	CheckSpecAmbiguity: `You are a requirements engineer reviewing a slice specification for ambiguity.

Your task is to read a slice specification (spec.md) and identify any acceptance checks (ACs) that are vague, incomplete, or underspecified.

Respond with a JSON object:
{
  "verdict": "PASS" or "FAIL",
  "findings": [
    {
      "id": "F-01",
      "severity": "FAIL" | "WARN" | "INFO",
      "title": "one-line summary",
      "detail": "why the AC is ambiguous and what is missing"
    }
  ]
}

Rules:
- An AC is ambiguous if it lacks concrete artefacts (file paths, status codes, specific label strings, numeric thresholds).
- An AC is incomplete if it names a behaviour but not the condition or outcome.
- An AC is underspecified if it uses vague verbs ("fix", "handle", "address") without concrete deliverables.
- Severity: FAIL for truly ambiguous ACs, WARN for minor clarity issues.
- If all ACs are concrete, complete, and well-specified, verdict is PASS.
- Temperature 0 — be deterministic and reproducible.`,
	CheckDesignReview: `You are a software architect reviewing whether a code change conflicts with established project memory.

Your task is to read the project memory (provided below) and a git diff, and identify any design decisions in the code change that conflict with documented conventions, architecture decisions (ADRs), or infrastructure constraints.

Respond with a JSON object:
{
  "verdict": "PASS" or "FAIL",
  "findings": [
    {
      "id": "F-01",
      "severity": "FAIL" | "WARN" | "INFO",
      "title": "one-line summary",
      "detail": "the conflict: what the code does vs what the memory says"
    }
  ]
}

Rules:
- Check for violations of ADRs, branching models, naming conventions, dependency rules, and infrastructure constraints.
- A new dependency without an ADR is a FAIL.
- A deviation from documented architecture without justification is a FAIL.
- If the code change is fully consistent with project memory, verdict is PASS.
- Temperature 0 — be deterministic and reproducible.`,
	CheckSecurityReview: `You are a security engineer reviewing a code change for vulnerabilities.

Your task is to read a git diff and identify any security vulnerabilities introduced by the change.

Respond with a JSON object:
{
  "verdict": "PASS" or "FAIL",
  "findings": [
    {
      "id": "F-01",
      "severity": "critical" | "high" | "medium" | "low",
      "title": "one-line summary",
      "detail": "the vulnerability: what it is, where it is, and the risk"
    }
  ]
}

Rules:
- Severity scale: critical (remote code execution, auth bypass), high (data exposure, injection), medium (info leak, weak crypto), low (best-practice violations with no direct exploit).
- Check for: hardcoded secrets, SQL/command injection, missing auth checks, unsafe deserialization, path traversal, overly permissive CORS, logging sensitive data.
- If the diff introduces no security concerns, verdict is PASS.
- Temperature 0 — be deterministic and reproducible.`,
	CheckSemanticCoverage: `You are a test-quality reviewer checking whether tests genuinely verify their claimed acceptance criteria.

Your task is to read a slice specification (spec.md) containing acceptance checks with their associated tests, and the test file diffs. For each AC, determine whether the matching test genuinely verifies the AC's behaviour (not just imports or passes through the code).

Respond with a JSON object:
{
  "verdict": "PASS" or "FAIL",
  "findings": [
    {
      "id": "F-01",
      "severity": "FAIL" | "WARN" | "INFO",
      "title": "one-line summary",
      "detail": "what the test claims to verify vs what it actually asserts"
    }
  ]
}

Rules:
- A test that calls a function but never asserts its behaviour is a FAIL.
- A test that only checks "no error" without validating output is a FAIL.
- A test that exercises the wrong condition for its claimed AC is a FAIL.
- If every AC is genuinely verified by its tests, verdict is PASS.
- Temperature 0 — be deterministic and reproducible.`,
	CheckMaintainabilityReview: `You are a software maintainability reviewer assessing whether code will be understandable 12 months from now.

Your task is to read a git diff and assess its maintainability.

Respond with a JSON object:
{
  "verdict": "PASS" or "FAIL",
  "findings": [
    {
      "id": "F-01",
      "severity": "FAIL" | "WARN" | "INFO",
      "title": "one-line summary",
      "detail": "what the issue is and why it hurts future understanding"
    }
  ]
}

Rules:
- Check for: unclear naming (single-letter variables, misleading names), god objects (files >500 lines or functions >50 lines), missing package/function doc comments, overly clever abstractions, tight coupling without clear interfaces.
- Severity: FAIL for genuinely unmaintainable code (e.g. 300-line function with single-letter variables), WARN for minor clarity issues, INFO for suggestions.
- If the code is clean, well-named, and appropriately documented, verdict is PASS.
- Temperature 0 — be deterministic and reproducible.`,
}

// userPromptHeader is prepended to every user payload.
const userPromptHeader = `You are evaluating a slice in a release of the SwornAgent project (a Go CLI).

Below is the slice specification, followed by the git diff of the code change.

--- SPECIFICATION (spec.md) ---

`

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

	// Read spec.md.
	specPath := filepath.Join(sliceDir, "spec.md")
	specContent, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("llm-check: read spec.md: %w", err)
	}

	// Build the prompt.
	systemPrompt := systemPrompts[checkType]
	userPayload := buildUserPayload(checkType, string(specContent), diffContent)

	// Call the model.
	rawResponse, _, _, _, err := verifier.Verify(ctx, systemPrompt, userPayload) 
	if err != nil {
		return nil, fmt.Errorf("llm-check: model call failed: %w", err)
	}

	// Parse the response.
	result, parseErr := parseLLMResponse(rawResponse)
	if parseErr != nil {
		// Tolerant parse: if we can't extract JSON, treat the raw response
		// as a single INFO finding and fail closed.
		result = &llmResponseJSON{
			Verdict:  "FAIL",
			Findings: []LLMFinding{{ID: "F-00", Severity: "INFO", Title: "Unparseable model response", Detail: rawResponse}},
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
func buildUserPayload(_ CheckType, specContent, diffContent string) string {
	var b strings.Builder
	b.WriteString(userPromptHeader)
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

// parseLLMResponse extracts and parses the JSON verdict from a model response.
// It is tolerant: if the response wraps JSON in markdown code fences, it extracts
// the inner content first.
func parseLLMResponse(raw string) (*llmResponseJSON, error) {
	cleaned := extractJSON(raw)

	var result llmResponseJSON
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w (raw: %.200s)", err, raw)
	}

	// Normalise verdict.
	result.Verdict = strings.ToUpper(strings.TrimSpace(result.Verdict))
	if result.Verdict != "PASS" && result.Verdict != "FAIL" {
		// Unknown verdict — fail closed.
		result.Verdict = "FAIL"
		if len(result.Findings) == 0 {
			result.Findings = append(result.Findings, LLMFinding{
				ID:       "F-00",
				Severity: "INFO",
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
