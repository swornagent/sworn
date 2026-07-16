package gate

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/baton/schemas"
	"github.com/swornagent/sworn/internal/project"
	"github.com/swornagent/sworn/internal/prompt"
	"github.com/swornagent/sworn/internal/spec"
)

const testSpecAmbiguitySchemaURL = "https://baton.sawy3r.net/schemas/spec-ambiguity-report-v1.json"

type ambiguityFixture struct {
	root    string
	release string
	slice   string
}

func newAmbiguityFixture(t *testing.T, references string) *ambiguityFixture {
	t.Helper()
	f := &ambiguityFixture{
		root:    t.TempDir(),
		release: "2026-07-17-ambiguity",
		slice:   "S01-reviewed",
	}
	cmd := exec.Command("git", "init", "-q", f.root)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	f.write(t, f.specPath(), ambiguitySpecJSON(f.release, f.slice, references))
	return f
}

func (f *ambiguityFixture) specPath() string {
	return filepath.Join(f.root, "docs", "release", f.release, f.slice, "spec.json")
}

func (f *ambiguityFixture) sliceDir() string {
	return filepath.Dir(f.specPath())
}

func (f *ambiguityFixture) path(parts ...string) string {
	return filepath.Join(append([]string{f.root}, parts...)...)
}

func (f *ambiguityFixture) write(t *testing.T, filename, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func ambiguitySpecJSON(release, slice, references string) string {
	return `{
  "$schema": "https://baton.sawy3r.net/schemas/spec-v1.json",
  "slice_id": "` + slice + `",
  "release": "` + release + `",
  "user_outcome": "A planner sees only explicit evidence.",
  "covers_needs": ["N-01"],
  "acceptance_criteria": [{"id":"AC-01","text":"THE SYSTEM SHALL inspect explicit references only.","ears_pattern":"ubiquitous"}],
  "in_scope": [],
  "out_of_scope": [],
  "references": ` + references + `
}` + "\n"
}

func validAmbiguityFinding(title string) SpecAmbiguityFinding {
	return SpecAmbiguityFinding{
		ID:                   "F-01",
		Severity:             "high",
		Title:                title,
		Detail:               "The artifact needs an explicit resolution.",
		CriterionID:          "AC-01",
		AmbiguityKind:        "unresolvable-reference",
		ObservableDivergence: "Different planners could select incompatible evidence.",
		ContractSurface:      "reference-integrity",
		SemanticSubject:      "typed-reference",
		SuggestedResolution:  "Make the referenced artifact valid and available.",
	}
}

func ambiguityResponse(t *testing.T, release, slice, verdict string, blocking, advisory map[string]SpecAmbiguityFinding) string {
	t.Helper()
	data, err := json.Marshal(&SpecAmbiguityReport{
		Schema:           testSpecAmbiguitySchemaURL,
		SchemaVersion:    1,
		Check:            CheckSpecAmbiguity,
		SliceID:          slice,
		Release:          release,
		Verdict:          verdict,
		BlockingFindings: blocking,
		AdvisoryFindings: advisory,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAmbiguityCheckRendersSafeUnresolvedReferenceAndSkipsUnsafe(t *testing.T) {
	t.Run("safe unresolved is supplied without implicit bytes", func(t *testing.T) {
		f := newAmbiguityFixture(t, `[{"kind":"file","path":"docs/missing.txt"}]`)
		f.write(t, f.path("private-canary.txt"), "MUST-NOT-LEAK")
		mock := &mockVerifier{text: ambiguityResponse(t, f.release, f.slice, "FAIL", map[string]SpecAmbiguityFinding{
			"missing-reference": validAmbiguityFinding("Referenced artifact is missing"),
		}, map[string]SpecAmbiguityFinding{})}

		report, err := RunLLMCheck(context.Background(), CheckSpecAmbiguity, f.sliceDir(), "ignored generic diff", mock)
		if err != nil {
			t.Fatalf("RunLLMCheck: %v", err)
		}
		if mock.structuredCalls != 1 || mock.verifyCalls != 0 {
			t.Fatalf("structured/raw calls = %d/%d, want 1/0", mock.structuredCalls, mock.verifyCalls)
		}
		if len(mock.structuredSchema) != 1 || string(mock.structuredSchema[0]) != string(schemas.SpecAmbiguityReportV1) {
			t.Fatalf("dedicated ambiguity schema was not selected")
		}
		wantPrompt, err := prompt.LLMCheck(string(CheckSpecAmbiguity))
		if err != nil {
			t.Fatal(err)
		}
		if mock.systemPrompt != wantPrompt {
			t.Fatal("dedicated system prompt bytes drifted from the vendored prompt")
		}
		resolution, err := spec.ResolveReferences(f.specPath())
		if err != nil {
			t.Fatal(err)
		}
		wantPayload := buildUserPayload(project.Resolve(resolution.WorkspaceRoot), spec.RenderMarkdown(resolution.Record), "") + "\n\n--- REFERENCED ARTIFACTS ---\n\n" + resolution.Render()
		if mock.userPayload != wantPayload {
			t.Fatalf("dedicated common payload or artifact suffix drifted\nwant: %q\n got: %q", wantPayload, mock.userPayload)
		}
		if !strings.Contains(mock.userPayload, "UNRESOLVED file:docs/missing.txt: missing\n") {
			t.Fatalf("safe unresolved reason missing from model payload:\n%s", mock.userPayload)
		}
		if strings.Contains(mock.userPayload, "MUST-NOT-LEAK") || strings.Contains(mock.userPayload, "private-canary") {
			t.Fatalf("payload leaked an unreferenced artifact:\n%s", mock.userPayload)
		}
		if report.Ambiguity == nil || !report.HasViolations() || report.Verdict != "FAIL" {
			t.Fatalf("dedicated blocking report was not preserved: %+v", report)
		}
		jsonOut := JSONLLMCheck(report)
		if !strings.Contains(jsonOut, "blocking_findings") || !strings.Contains(jsonOut, "missing-reference") || strings.Contains(jsonOut, `"findings"`) {
			t.Fatalf("ambiguity JSON flattened or lost its fingerprint maps: %s", jsonOut)
		}
	})

	t.Run("unsafe path stops before model dispatch", func(t *testing.T) {
		f := newAmbiguityFixture(t, `[{"kind":"file","path":"../outside.txt"}]`)
		mock := &mockVerifier{text: ambiguityResponse(t, f.release, f.slice, "PASS", map[string]SpecAmbiguityFinding{}, map[string]SpecAmbiguityFinding{})}
		_, err := RunLLMCheck(context.Background(), CheckSpecAmbiguity, f.sliceDir(), "", mock)
		if err == nil || !strings.Contains(err.Error(), "reference-path-invalid") {
			t.Fatalf("RunLLMCheck error = %v, want reference-path-invalid", err)
		}
		if mock.structuredCalls != 0 || mock.verifyCalls != 0 {
			t.Fatalf("unsafe path dispatched structured/raw calls = %d/%d, want 0/0", mock.structuredCalls, mock.verifyCalls)
		}
	})
}

func TestDedicatedAmbiguityReportContractFailureMatrix(t *testing.T) {
	f := newAmbiguityFixture(t, "[]")
	validFinding := validAmbiguityFinding("Ambiguous reference")
	validBlocking := map[string]SpecAmbiguityFinding{"typed-reference": validFinding}

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "duplicate raw key",
			raw: strings.Replace(
				ambiguityResponse(t, f.release, f.slice, "PASS", map[string]SpecAmbiguityFinding{}, map[string]SpecAmbiguityFinding{}),
				`"check":"spec-ambiguity"`, `"check":"spec-ambiguity","check":"spec-ambiguity"`, 1),
		},
		{
			name: "generic schema impostor",
			raw:  `{"check":"ac-satisfaction","verdict":"PASS","findings":[]}`,
		},
		{
			name: "overlapping fingerprint",
			raw:  ambiguityResponse(t, f.release, f.slice, "FAIL", validBlocking, map[string]SpecAmbiguityFinding{"typed-reference": validFinding}),
		},
		{
			name: "contradictory stated verdict",
			raw:  ambiguityResponse(t, f.release, f.slice, "PASS", validBlocking, map[string]SpecAmbiguityFinding{}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockVerifier{text: tt.raw}
			report, err := RunLLMCheck(context.Background(), CheckSpecAmbiguity, f.sliceDir(), "", mock)
			if err != nil {
				t.Fatalf("RunLLMCheck: %v", err)
			}
			if report.Ambiguity == nil || !report.HasViolations() || report.Verdict != "FAIL" {
				t.Fatalf("contract violation must fail closed: %+v", report)
			}
			if _, ok := report.Ambiguity.BlockingFindings["report-contract-error"]; !ok {
				t.Fatalf("contract violation must retain the dedicated error fingerprint: %+v", report.Ambiguity.BlockingFindings)
			}
			if mock.structuredCalls != 1 || mock.verifyCalls != 0 {
				t.Fatalf("structured/raw calls = %d/%d, want 1/0", mock.structuredCalls, mock.verifyCalls)
			}
		})
	}
}

func TestPrintSpecAmbiguityRetainsAdvisoryFingerprintOnPass(t *testing.T) {
	report := &SpecAmbiguityReport{
		Schema:           testSpecAmbiguitySchemaURL,
		SchemaVersion:    1,
		Check:            CheckSpecAmbiguity,
		SliceID:          "S01-reviewed",
		Release:          "2026-07-17-ambiguity",
		Verdict:          "PASS",
		BlockingFindings: map[string]SpecAmbiguityFinding{},
		AdvisoryFindings: map[string]SpecAmbiguityFinding{
			"advisory-reference": validAmbiguityFinding("Advisory reference wording"),
		},
	}
	output := PrintSpecAmbiguity(report)
	if !strings.Contains(output, "PASS") || !strings.Contains(output, "advisory-reference") || !strings.Contains(output, "Advisory reference wording") {
		t.Fatalf("plain renderer lost advisory fingerprint on PASS: %s", output)
	}
	if err := baton.ValidateSchema("spec-ambiguity-report-v1", []byte(JSONSpecAmbiguity(report))); err != nil {
		t.Fatalf("dedicated JSON renderer must remain an exact schema object: %v", err)
	}
}

func TestSpecAmbiguityCannotUseGenericRenderer(t *testing.T) {
	flattened := &LLMCheckReport{
		CheckType: CheckSpecAmbiguity,
		Verdict:   "PASS",
		Findings: []LLMFinding{{
			ID: "F-01", Severity: "high", Title: "Generic finding", Detail: "This must not become an ambiguity report.",
		}},
	}
	if output := PrintLLMCheck(flattened); !strings.Contains(output, "missing dedicated report") || strings.Contains(output, "Generic finding") {
		t.Fatalf("generic report was accepted as ambiguity plain output: %s", output)
	}
	if output := JSONLLMCheck(flattened); !strings.Contains(output, "null") || strings.Contains(output, "Generic finding") {
		t.Fatalf("generic report was accepted as ambiguity JSON output: %s", output)
	}
}

func TestSpecAmbiguityContractFailureRendersAsDedicatedSchema(t *testing.T) {
	report := specAmbiguityContractFailure("S01-reviewed", "2026-07-17-ambiguity", `{"not":"a-report"}`, "fixture contract error")
	if err := baton.ValidateSchema("spec-ambiguity-report-v1", []byte(JSONSpecAmbiguity(report))); err != nil {
		t.Fatalf("contract failure JSON must remain a dedicated schema object: %v", err)
	}
}
