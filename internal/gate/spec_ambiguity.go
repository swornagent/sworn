package gate

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/baton/schemas"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/project"
	"github.com/swornagent/sworn/internal/prompt"
	"github.com/swornagent/sworn/internal/spec"
	"github.com/swornagent/sworn/internal/style"
)

const specAmbiguitySchemaURL = "https://baton.sawy3r.net/schemas/spec-ambiguity-report-v1.json"

// SpecAmbiguityFinding preserves the dedicated fingerprint-keyed report shape.
// It must never be converted into the generic positional Findings slice.
type SpecAmbiguityFinding struct {
	ID                   string `json:"id"`
	Severity             string `json:"severity"`
	Title                string `json:"title"`
	Detail               string `json:"detail"`
	Evidence             string `json:"evidence,omitempty"`
	CriterionID          string `json:"criterion_id"`
	AmbiguityKind        string `json:"ambiguity_kind"`
	ObservableDivergence string `json:"observable_divergence"`
	ContractSurface      string `json:"contract_surface"`
	SemanticSubject      string `json:"semantic_subject"`
	SuggestedResolution  string `json:"suggested_resolution"`
}

// SpecAmbiguityReport is the only accepted shape for CheckSpecAmbiguity.
type SpecAmbiguityReport struct {
	Schema           string                          `json:"$schema"`
	SchemaVersion    int                             `json:"schema_version"`
	Check            CheckType                       `json:"check"`
	SliceID          string                          `json:"slice_id"`
	Release          string                          `json:"release"`
	Verdict          string                          `json:"verdict"`
	BlockingFindings map[string]SpecAmbiguityFinding `json:"blocking_findings"`
	AdvisoryFindings map[string]SpecAmbiguityFinding `json:"advisory_findings"`
	// RawResponse is retained only for in-process diagnostics. It is excluded
	// from the public renderer so JSONSpecAmbiguity remains an exact
	// spec-ambiguity-report-v1 object rather than an augmented generic wrapper.
	RawResponse string `json:"-"`
}

// HasViolations derives disposition from the authoritative map rather than
// trusting a model's stated verdict.
func (r *SpecAmbiguityReport) HasViolations() bool {
	return r == nil || len(r.BlockingFindings) != 0 || r.Verdict != "PASS"
}

func runSpecAmbiguity(ctx context.Context, sliceDir string, verifier model.Verifier) (*LLMCheckReport, error) {
	resolution, err := spec.ResolveReferences(filepath.Join(sliceDir, "spec.json"))
	if err != nil {
		return nil, fmt.Errorf("spec-ambiguity: %w", err)
	}

	systemPrompt, err := prompt.LLMCheck(string(CheckSpecAmbiguity))
	if err != nil {
		return nil, fmt.Errorf("spec-ambiguity: %w", err)
	}
	// This is the exact shared payload contract. The dedicated section is only
	// appended after unsafe resolution passed, so no escaped or arbitrary bytes
	// can reach the model.
	payload := buildUserPayload(project.Resolve(resolution.WorkspaceRoot), spec.RenderMarkdown(resolution.Record), "")
	payload += "\n\n--- REFERENCED ARTIFACTS ---\n\n" + resolution.Render()

	rawResponse, err := model.ChatStructuredJSON(ctx, verifier, systemPrompt, payload, schemas.SpecAmbiguityReportV1)
	if err != nil {
		return nil, fmt.Errorf("spec-ambiguity: schema-constrained model call failed: %w", err)
	}
	report := parseSpecAmbiguityResponse(rawResponse, resolution.Record.SliceID, resolution.Record.Release)

	return &LLMCheckReport{
		CheckType:    CheckSpecAmbiguity,
		EmittedCheck: report.Check,
		Slice:        resolution.Record.SliceID,
		Release:      resolution.Record.Release,
		Verdict:      report.Verdict,
		RawResponse:  rawResponse,
		Ambiguity:    report,
	}, nil
}

func parseSpecAmbiguityResponse(raw, expectedSlice, expectedRelease string) *SpecAmbiguityReport {
	var result SpecAmbiguityReport
	if err := spec.DecodeJSONNoDuplicate([]byte(raw), &result); err != nil {
		return specAmbiguityContractFailure(expectedSlice, expectedRelease, raw, "The model response is not one duplicate-safe JSON object: "+err.Error())
	}
	if err := baton.ValidateSchema("spec-ambiguity-report-v1", []byte(raw)); err != nil {
		return specAmbiguityContractFailure(expectedSlice, expectedRelease, raw, "The model response does not satisfy spec-ambiguity-report-v1: "+err.Error())
	}
	if result.SliceID != expectedSlice || result.Release != expectedRelease {
		return specAmbiguityContractFailure(expectedSlice, expectedRelease, raw,
			fmt.Sprintf("The model report identifies slice %q / release %q, not the reviewed slice %q / release %q.", result.SliceID, result.Release, expectedSlice, expectedRelease))
	}
	for fingerprint := range result.BlockingFindings {
		if _, overlap := result.AdvisoryFindings[fingerprint]; overlap {
			return specAmbiguityContractFailure(expectedSlice, expectedRelease, raw,
				fmt.Sprintf("The fingerprint %q appears in both blocking_findings and advisory_findings.", fingerprint))
		}
	}
	derived := "PASS"
	if len(result.BlockingFindings) != 0 {
		derived = "FAIL"
	}
	if result.Verdict != derived {
		return specAmbiguityContractFailure(expectedSlice, expectedRelease, raw,
			fmt.Sprintf("The model verdict %q conflicts with %d blocking findings.", result.Verdict, len(result.BlockingFindings)))
	}
	result.Verdict = derived
	result.RawResponse = raw
	return &result
}

func specAmbiguityContractFailure(sliceID, release, raw, detail string) *SpecAmbiguityReport {
	return &SpecAmbiguityReport{
		Schema:        specAmbiguitySchemaURL,
		SchemaVersion: 1,
		Check:         CheckSpecAmbiguity,
		SliceID:       sliceID,
		Release:       release,
		Verdict:       "FAIL",
		BlockingFindings: map[string]SpecAmbiguityFinding{
			"report-contract-error": {
				ID:                   "F-00",
				Severity:             "high",
				Title:                "Spec ambiguity report contract error",
				Detail:               detail,
				CriterionID:          "cross-AC",
				AmbiguityKind:        "structure-or-wording",
				ObservableDivergence: "A malformed or misidentified report cannot establish a trustworthy ambiguity verdict.",
				ContractSurface:      "verification-evidence",
				SemanticSubject:      "model-report-contract",
				SuggestedResolution:  "Emit one valid spec-ambiguity-report-v1 object for the reviewed slice and release.",
			},
		},
		AdvisoryFindings: map[string]SpecAmbiguityFinding{},
		RawResponse:      raw,
	}
}

// PrintSpecAmbiguity renders the dedicated maps deterministically rather than
// converting fingerprints into anonymous generic findings.
func PrintSpecAmbiguity(r *SpecAmbiguityReport) string {
	if r == nil {
		return "\nLLM CHECK — spec-ambiguity\n\nFAIL — missing dedicated report\n"
	}
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(style.Bold("LLM CHECK — spec-ambiguity"))
	b.WriteString("\n")
	b.WriteString(style.Dim(fmt.Sprintf("slice: %s  release: %s\n", r.SliceID, r.Release)))
	b.WriteString("\n")
	if !r.HasViolations() {
		b.WriteString(style.Success("PASS — no blocking ambiguity findings\n"))
		writeAmbiguityFindings(&b, "advisory", r.AdvisoryFindings)
		b.WriteString("\n")
		return b.String()
	}
	b.WriteString(style.Danger(fmt.Sprintf("FAIL — %d blocking ambiguity finding(s)\n\n", len(r.BlockingFindings))))
	writeAmbiguityFindings(&b, "blocking", r.BlockingFindings)
	writeAmbiguityFindings(&b, "advisory", r.AdvisoryFindings)
	b.WriteString("\n")
	b.WriteString(style.Danger("NOT PASSED"))
	b.WriteString("\n\n")
	return b.String()
}

func writeAmbiguityFindings(b *strings.Builder, disposition string, findings map[string]SpecAmbiguityFinding) {
	keys := make([]string, 0, len(findings))
	for fingerprint := range findings {
		keys = append(keys, fingerprint)
	}
	sort.Strings(keys)
	for _, fingerprint := range keys {
		finding := findings[fingerprint]
		fmt.Fprintf(b, "  [%s] %s — %s: %s\n", disposition, fingerprint, finding.Title, finding.Detail)
	}
}

// JSONSpecAmbiguity returns the map-preserving dedicated report as stable JSON.
func JSONSpecAmbiguity(r *SpecAmbiguityReport) string {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}
