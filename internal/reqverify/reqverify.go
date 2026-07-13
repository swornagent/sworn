// Package reqverify implements the requirements-quality verification gate.
//
// It grades every acceptance criterion in a release against the ISO/IEC/IEEE
// 29148:2018 quality characteristics (singular, unambiguous, complete,
// consistent, feasible, verifiable, necessary) using a fresh-context model
// pass.  It judges well-formedness only — never intent-correctness (that is
// S05 / validation).
//
// Fail-closed: any characteristic breach on any AC yields a non-zero exit.
package reqverify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/spec"
	"github.com/swornagent/sworn/internal/style"
)

// ErrStructuredUnsupported is returned by a Verifier whose underlying model
// cannot emit structured output (no StructuredOutput capability). It is a
// DECLARED Rule 2 deferral signal (S02 AC-03): Run surfaces it as
// Report.Deferred with a capability-naming reason so the DoR gate records a
// declared deferral (routed through CheckDoR's "not evaluated" arm), never a
// silent pass and never a hard prose-format failure. Every other Verify error
// stays a hard, fail-closed dispatch error.
var ErrStructuredUnsupported = errors.New("reqverify: verifier model does not support structured output (capability absent)")

// reqverifyResultsSchema is the sworn-local emit schema handed to the Verifier
// for the DoR requirements-grading call (S02 D2, Coach-confirmed inline emit +
// a lightweight sworn-local validate — reqverify is a fail-closed gate, so its
// structured output is validated before being trusted). It stays inside
// OpenAI's strict-mode keyword subset — no minLength/pattern/format (those
// break a strict response_format target; see internal/model/structured.go
// strict-projection). The "title" sets the OpenAI json_schema name.
var reqverifyResultsSchema = []byte(`{
  "title": "reqverify-results",
  "type": "object",
  "additionalProperties": false,
  "required": ["results"],
  "properties": {
    "results": {
      "type": "array",
      "description": "One grade per acceptance criterion evaluated.",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["slice_id", "ac_index", "status"],
        "properties": {
          "slice_id": { "type": "string", "description": "The slice id the AC belongs to." },
          "ac_index": { "type": "integer", "description": "1-based index of the AC within its slice." },
          "status": { "type": "string", "enum": ["PASS", "FAIL"], "description": "PASS if the AC satisfies every 29148 quality characteristic; else FAIL." },
          "characteristic": { "type": "string", "description": "On FAIL, the breached 29148 characteristic (singular, ambiguous, incomplete, ...)." },
          "reason": { "type": "string", "description": "On FAIL, a one-sentence reason." }
        }
      }
    }
  }
}`)

// Characteristic is a 29148 quality characteristic for requirements.
type Characteristic string

const (
	CharSingular    Characteristic = "singular"
	CharUnambiguous Characteristic = "unambiguous"
	CharAmbiguous   Characteristic = "ambiguous"
	CharComplete    Characteristic = "complete"
	CharConsistent  Characteristic = "consistent"
	CharFeasible    Characteristic = "feasible"
	CharVerifiable  Characteristic = "verifiable"
	CharNecessary   Characteristic = "necessary"
)

// AllCharacteristics lists the seven quality characteristics in definition order.
var AllCharacteristics = []Characteristic{
	CharSingular,
	CharUnambiguous,
	CharComplete,
	CharConsistent,
	CharFeasible,
	CharVerifiable,
	CharNecessary,
}

// Violation records a characteristic breach for one acceptance criterion.
type Violation struct {
	SliceID        string
	ACIndex        int // 1-based within the slice
	ACContent      string
	Characteristic Characteristic
	Reason         string
}

// Grade is the per-AC result after model grading.
type Grade struct {
	SliceID   string
	ACIndex   int
	ACContent string
	Passed    bool
	Violation *Violation // non-nil when Passed is false
}

// Report aggregates grades across all slices in the release.
type Report struct {
	Grades       []Grade
	Violations   []Violation
	TotalACs     int
	PassedACs    int
	FailedACs    int
	FreshContext bool // records that a fresh-context model pass was used
	// Deferred is set when the requirements-grading model could not emit
	// structured output (capability absent). It is a DECLARED Rule 2 deferral
	// (S02 AC-03): the gate was not evaluated, so the caller (CheckDoR) fails
	// closed on it (ReqverifyPassed=false) with DeferredReason as the
	// capability-naming explanation — never a silent pass, never a crash.
	Deferred       bool
	DeferredReason string
}

// HasViolations returns true when at least one characteristic breach exists.
func (r Report) HasViolations() bool { return len(r.Violations) > 0 }

// AC is an individual acceptance criterion extracted from a slice spec
// (spec.json acceptance_criteria, or legacy spec.md checkboxes).
type AC struct {
	SliceID string
	Index   int    // 1-based within the slice
	Content string // the AC text without the checkbox marker
}

// Verifier is the model interface reqverify needs. The grading call is
// schema-constrained (S02 migration): the model emits a JSON object conforming
// to schema (reqverifyResultsSchema) and Verify returns that structured JSON —
// not a `## RESULTS` prose section. A verifier whose model cannot emit
// structured output returns ErrStructuredUnsupported so the DoR gate records a
// declared Rule 2 deferral (AC-03). The package keeps its own interface (a
// superset shape of the old model.Verifier signature plus the schema arg) so it
// has no dependency on the model package.
type Verifier interface {
	Verify(ctx context.Context, systemPrompt, userPayload string, schema []byte) (structuredJSON string, costUSD float64, inputTokens, outputTokens int64, err error)
}

// Run executes requirements verification over a release directory.
//
// It discovers every slice's spec (spec.json preferred, spec.md fallback),
// extracts all acceptance criteria, builds a payload, dispatches it to the
// model with the requirements-verifier prompt, parses the per-AC grades, and
// returns the aggregated Report. A release yielding zero ACs is an error.
//
// The releaseDir is the path to docs/release/<name>.
func Run(ctx context.Context, releaseDir string, verifier Verifier, systemPrompt string) (Report, error) {
	report := Report{FreshContext: true}

	// 1. Discover slices and extract ACs.
	acs, err := extractACs(releaseDir)
	if err != nil {
		return report, fmt.Errorf("reqverify: extracting ACs: %w", err)
	}
	report.TotalACs = len(acs)
	if len(acs) == 0 {
		// Fail closed: a release with no evaluable ACs must never read as a
		// vacuous PASS — spec-v1 (spec.json) releases carried real ACs that an
		// earlier spec.md-only reader silently missed.
		return report, fmt.Errorf("reqverify: no evaluable acceptance criteria in %s (no spec.json acceptance_criteria or spec.md acceptance checks)", releaseDir)
	}

	// 2. Build the model payload.
	payload := buildPayload(acs)

	// 3. Dispatch to model, constrained to emit the structured results object.
	reply, _, _, _, err := verifier.Verify(ctx, systemPrompt, payload, reqverifyResultsSchema)
	if err != nil {
		// Capability-absent is a DECLARED Rule 2 deferral, not a hard dispatch
		// failure (S02 AC-03): the model genuinely cannot emit structured
		// output. Surface it as Report.Deferred so CheckDoR fails closed on it
		// with a capability-naming reason — never a silent pass, never a crash.
		if errors.Is(err, ErrStructuredUnsupported) {
			report.Deferred = true
			report.DeferredReason = "requirements verification not evaluated (verifier model lacks structured-output capability): " + err.Error()
			return report, nil
		}
		return report, fmt.Errorf("reqverify: model dispatch: %w", err)
	}

	// 4. Parse per-AC grades from the model's structured response.
	grades, err := parseStructuredGrades(reply, acs)
	if err != nil {
		return report, fmt.Errorf("reqverify: parsing model response: %w", err)
	}

	// 5. Aggregate.
	report.Grades = grades
	for _, g := range grades {
		if g.Passed {
			report.PassedACs++
		} else {
			report.FailedACs++
			if g.Violation != nil {
				report.Violations = append(report.Violations, *g.Violation)
			}
		}
	}

	return report, nil
}

// extractACs extracts acceptance criteria from every slice under the release
// directory. It prefers the spec-v1 record (spec.json acceptance_criteria —
// the canonical current format) and falls back to scraping spec.md checkbox
// lines under "## Acceptance checks" for legacy releases.
func extractACs(releaseDir string) ([]AC, error) {
	entries, err := os.ReadDir(releaseDir)
	if err != nil {
		return nil, err
	}

	// Sort slice directories for deterministic order.
	var sliceDirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Accept only names starting with "S" (slice pattern).
		if !strings.HasPrefix(e.Name(), "S") {
			continue
		}
		sliceDirs = append(sliceDirs, e.Name())
	}
	sort.Strings(sliceDirs)

	var allACs []AC
	for _, sliceID := range sliceDirs {
		sliceDir := filepath.Join(releaseDir, sliceID)

		// Prefer the spec-v1 record.
		rec, err := spec.ReadRecord(sliceDir)
		if err != nil {
			return nil, err
		}
		if rec != nil && len(rec.AcceptanceCriteria) > 0 {
			for i, r := range rec.AcceptanceCriteria {
				allACs = append(allACs, AC{
					SliceID: sliceID,
					Index:   i + 1,
					Content: r.Text,
				})
			}
			continue
		}

		// Legacy fallback: spec.md checkbox scrape.
		specPath := filepath.Join(sliceDir, "spec.md")
		data, err := os.ReadFile(specPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue // slice dir with no spec artefact yet
			}
			return nil, fmt.Errorf("reading %s: %w", specPath, err)
		}
		acs := parseACs(string(data), sliceID)
		allACs = append(allACs, acs...)
	}

	return allACs, nil
}

// checkboxRe matches markdown checkbox lines: "- [ ] ..." or "- [x] ...".
var checkboxRe = regexp.MustCompile(`^- \[[ xX]\]\s+(.*)`)

// acceptanceChecksHeader matches the "## Acceptance checks" section header.
var acceptanceChecksHeader = regexp.MustCompile(`(?i)^##\s+acceptance\s+checks`)

// parseACs extracts checkbox lines from within the "## Acceptance checks"
// section of a spec.md.
func parseACs(spec string, sliceID string) []AC {
	lines := strings.Split(spec, "\n")
	var inSection bool
	var acs []AC
	idx := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect section header.
		if acceptanceChecksHeader.MatchString(trimmed) {
			inSection = true
			continue
		}

		// Stop at the next top-level heading (## ...) that is NOT acceptance checks.
		if inSection && strings.HasPrefix(trimmed, "## ") && !acceptanceChecksHeader.MatchString(trimmed) {
			break
		}

		if !inSection {
			continue
		}

		// Match checkbox line.
		m := checkboxRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}

		idx++
		content := strings.TrimSpace(m[1])
		if content == "" {
			continue
		}

		acs = append(acs, AC{
			SliceID: sliceID,
			Index:   idx,
			Content: content,
		})
	}

	return acs
}

// buildPayload constructs the model payload from extracted ACs.
func buildPayload(acs []AC) string {
	var b strings.Builder

	// Group ACs by slice.
	type sliceGroup struct {
		SliceID string
		ACs     []AC
	}
	groupMap := make(map[string][]AC)
	var sliceOrder []string
	for _, ac := range acs {
		if _, ok := groupMap[ac.SliceID]; !ok {
			sliceOrder = append(sliceOrder, ac.SliceID)
		}
		groupMap[ac.SliceID] = append(groupMap[ac.SliceID], ac)
	}

	for _, sliceID := range sliceOrder {
		group := groupMap[sliceID]
		fmt.Fprintf(&b, "### Slice: %s\n\n", sliceID)
		for _, ac := range group {
			fmt.Fprintf(&b, "AC %d: %s\n", ac.Index, ac.Content)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// resultRecord is one per-AC grade in the model's structured emission
// (reqverifyResultsSchema). Parsed from validated JSON, never scraped from
// prose.
type resultRecord struct {
	SliceID        string `json:"slice_id"`
	ACIndex        int    `json:"ac_index"`
	Status         string `json:"status"`
	Characteristic string `json:"characteristic"`
	Reason         string `json:"reason"`
}

// resultsEnvelope is the top-level structured results object.
type resultsEnvelope struct {
	Results []resultRecord `json:"results"`
}

// validateReqverifyResults is the lightweight sworn-local validate (S02 D2,
// Coach-confirmed) applied before the structured output is trusted: the
// emission must be a JSON object carrying a `results` array, and every record
// must name a slice, a positive AC index, and a PASS/FAIL status. A breach is
// the structured equivalent of the old "missing ## RESULTS section" BLOCK —
// the model did not follow the contract at all — so it fails closed (returns a
// non-nil error that Run surfaces as a parse BLOCK). Per-AC completeness (an AC
// absent from a well-formed results array) is NOT a validation breach here — it
// is handled fail-closed as a FAIL grade in parseStructuredGrades.
func validateReqverifyResults(reply string) (resultsEnvelope, error) {
	var env resultsEnvelope
	// Distinguish "no results key" from "empty results array": a valid
	// emission always carries the key. Decode into a probe that keeps the key
	// presence.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(reply), &probe); err != nil {
		return env, fmt.Errorf("structured results not a JSON object: %w", err)
	}
	raw, ok := probe["results"]
	if !ok {
		return env, fmt.Errorf("structured results object missing required \"results\" array")
	}
	if err := json.Unmarshal(raw, &env.Results); err != nil {
		return env, fmt.Errorf("structured results \"results\" is not an array of grades: %w", err)
	}
	for i, r := range env.Results {
		if strings.TrimSpace(r.SliceID) == "" {
			return env, fmt.Errorf("structured results[%d] missing slice_id", i)
		}
		if r.ACIndex < 1 {
			return env, fmt.Errorf("structured results[%d] has non-positive ac_index %d", i, r.ACIndex)
		}
		if r.Status != "PASS" && r.Status != "FAIL" {
			return env, fmt.Errorf("structured results[%d] has invalid status %q (want PASS|FAIL)", i, r.Status)
		}
	}
	return env, nil
}

// parseStructuredGrades interprets the model's structured results object and
// assigns a Grade per AC (S02 migration, replacing the `## RESULTS` prose
// scrape). Acceptance semantics are preserved verbatim (D4):
//
//   - the whole emission failing the lightweight validate BLOCKS (the
//     structured equivalent of the old "missing ## RESULTS section"),
//   - an AC absent from a well-formed results array is a fail-closed FAIL,
//   - a per-AC FAIL carries its characteristic and reason.
func parseStructuredGrades(reply string, acs []AC) ([]Grade, error) {
	env, err := validateReqverifyResults(reply)
	if err != nil {
		return nil, err
	}

	resultMap := make(map[string]bool)         // "sliceID:index" -> passed
	violationMap := make(map[string]Violation) // "sliceID:index" -> violation
	for _, r := range env.Results {
		key := fmt.Sprintf("%s:%d", r.SliceID, r.ACIndex)
		if r.Status == "PASS" {
			resultMap[key] = true
			continue
		}
		resultMap[key] = false
		violationMap[key] = Violation{
			SliceID:        r.SliceID,
			ACIndex:        r.ACIndex,
			Characteristic: Characteristic(strings.TrimSpace(r.Characteristic)),
			Reason:         strings.TrimSpace(r.Reason),
		}
	}

	// Map grades back to the ACs in order.
	var grades []Grade
	for _, ac := range acs {
		key := fmt.Sprintf("%s:%d", ac.SliceID, ac.Index)
		passed, ok := resultMap[key]
		if !ok {
			// AC not in model output — fail-closed.
			grades = append(grades, Grade{
				SliceID:   ac.SliceID,
				ACIndex:   ac.Index,
				ACContent: ac.Content,
				Passed:    false,
				Violation: &Violation{
					SliceID:        ac.SliceID,
					ACIndex:        ac.Index,
					ACContent:      ac.Content,
					Characteristic: "verifiable",
					Reason:         "AC missing from model response — fail-closed",
				},
			})
			continue
		}
		if passed {
			grades = append(grades, Grade{
				SliceID:   ac.SliceID,
				ACIndex:   ac.Index,
				ACContent: ac.Content,
				Passed:    true,
			})
		} else {
			v := violationMap[key]
			v.ACContent = ac.Content
			grades = append(grades, Grade{
				SliceID:   ac.SliceID,
				ACIndex:   ac.Index,
				ACContent: ac.Content,
				Passed:    false,
				Violation: &v,
			})
		}
	}

	return grades, nil
}

// Print formats the report for human-readable output.
func Print(report Report) string {
	var b strings.Builder

	fmt.Fprint(&b, style.Heading("Requirements verification report")+"\n")
	fmt.Fprint(&b, style.Dim("===============================")+"\n\n")

	if report.TotalACs == 0 {
		fmt.Fprintf(&b, "No acceptance criteria to verify.\n")
		return b.String()
	}

	fmt.Fprint(&b, style.Accent(fmt.Sprintf("Total ACs: %d | Passed: %d | Failed: %d",
		report.TotalACs, report.PassedACs, report.FailedACs))+"\n\n")

	if report.FreshContext {
		fmt.Fprintf(&b, "Verifier mode: fresh-context (requirements-verifier prompt)\n\n")
	}

	if report.HasViolations() {
		fmt.Fprint(&b, style.Danger("Violations:")+"\n")
		for _, v := range report.Violations {
			fmt.Fprintf(&b, "  AC %d (%s): %s — %s\n",
				v.ACIndex, v.SliceID, v.Characteristic, v.Reason)
		}
		fmt.Fprintln(&b)
	}

	// Per-AC grade table.
	fmt.Fprintf(&b, "Per-AC grades:\n")
	for _, g := range report.Grades {
		if g.Passed {
			fmt.Fprint(&b, style.Success(fmt.Sprintf("  AC %d (%s): PASS", g.ACIndex, g.SliceID))+"\n")
		} else if g.Violation != nil {
			fmt.Fprint(&b, style.Danger(fmt.Sprintf("  AC %d (%s): FAIL — %s", g.ACIndex, g.SliceID, g.Violation.Characteristic))+"\n")
		} else {
			fmt.Fprint(&b, style.Danger(fmt.Sprintf("  AC %d (%s): FAIL", g.ACIndex, g.SliceID))+"\n")
		}
	}

	return b.String()
}

// PrintCompact formats a one-line summary for use by the CLI.
func PrintCompact(report Report) string {
	if report.TotalACs == 0 {
		return "reqverify: no acceptance criteria to verify"
	}
	summary := fmt.Sprintf("reqverify: %d ACs — %d passed, %d failed",
		report.TotalACs, report.PassedACs, report.FailedACs)
	if report.HasViolations() {
		summary += " — FAILED"
	} else {
		summary += " — PASSED"
	}
	return summary
}
