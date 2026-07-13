package design

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
)

// fakeCaptainDriver is a minimal driver.Driver whose captain dispatch returns a
// canned structured emission (S02: Generate now dispatches Role=captain with a
// StructuredSchema and reads Result.StructuredJSON, not prose ResultText).
// resultText carries prose that may deliberately LACK §1–§6 headers, to prove
// the gate reads the structured object and not the prose shape (AC-01).
type fakeCaptainDriver struct {
	structuredJSON string
	resultText     string
	errKind        string
	err            error
	last           *driver.DispatchInput
}

func (f fakeCaptainDriver) Name() string { return "fake-captain-driver" }
func (f fakeCaptainDriver) Roles() driver.RoleSet {
	return driver.RoleSet{driver.RoleCaptain: true}
}
func (f fakeCaptainDriver) Dispatch(_ context.Context, in driver.DispatchInput) (driver.Result, error) {
	if f.last != nil {
		*f.last = in
	}
	if f.err != nil {
		return driver.Result{Status: driver.StatusError, ErrKind: f.errKind}, f.err
	}
	return driver.Result{
		Status:         driver.StatusOK,
		ResultText:     f.resultText,
		StructuredJSON: json.RawMessage(f.structuredJSON),
	}, nil
}

// validDesignJSON is a well-formed design-tldr emission: all six sections
// present and non-empty.
const validDesignJSON = `{
  "user_visible_change": "Users see design.md appear in the slice dir before any code changes.",
  "design_decisions": "- single-shot schema-constrained model call\n- deterministic render from typed fields",
  "files_touched": "- internal/design/tldr.go — Generate reads the structured object and writes design.md",
  "not_doing": "- NOT implementing the captain review stage",
  "reachability_plan": "Run Generate with a spec and observe design.md created in the slice directory.",
  "open_questions": "None."
}`

func TestGenerateWritesSixSections(t *testing.T) {
	dir := t.TempDir()
	spec := "# Test spec\n\n## User outcome\n\nA test outcome.\n"

	fa := fakeCaptainDriver{structuredJSON: validDesignJSON}
	got, err := Generate(context.Background(), dir, spec, fa, "fake/model", "/tmp/wt", 0, GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got == "" {
		t.Fatal("Generate returned empty text")
	}

	// The rendered design.md carries all six generated §N headers.
	for _, marker := range []string{"## §1", "## §2", "## §3", "## §4", "## §5", "## §6"} {
		if !strings.Contains(got, marker) {
			t.Errorf("rendered design missing %q header", marker)
		}
	}

	// Verify design.md was written with the rendered content.
	designPath := filepath.Join(dir, "design.md")
	data, err := os.ReadFile(designPath)
	if err != nil {
		t.Fatalf("design.md not written: %v", err)
	}
	if string(data) != got {
		t.Fatalf("design.md content mismatch\nwant:\n%s\ngot:\n%s", got, string(data))
	}
	if !strings.Contains(string(data), "Users see design.md appear") {
		t.Errorf("rendered design.md missing §1 content")
	}
}

// TestGenerateGrokCase is the exact failure this slice fixes (AC-01): a model
// whose PROSE lacks the literal §1–§6 headers but whose STRUCTURED output is
// valid must PASS — the schema is the contract, not the prose shape.
func TestGenerateGrokCase(t *testing.T) {
	dir := t.TempDir()
	spec := "# Test spec\n\n## User outcome\n\nGrok outcome.\n"

	// Prose with NO §1–§6 headers at all — the old hasSixSections scrape would
	// have rejected this. Structured output is valid.
	fa := fakeCaptainDriver{
		resultText:     "Sure, here is my design. I looked at the spec and here is the approach without any section markers whatsoever.",
		structuredJSON: validDesignJSON,
	}
	got, err := Generate(context.Background(), dir, spec, fa, "grok/model", "/tmp/wt", 0, GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate (Grok case) should PASS on valid structured output, got: %v", err)
	}
	if got == "" {
		t.Fatal("Generate returned empty text for a valid structured emission")
	}
	// design.md exists and carries the generated prose headers.
	data, err := os.ReadFile(filepath.Join(dir, "design.md"))
	if err != nil {
		t.Fatalf("design.md not written on the Grok case: %v", err)
	}
	if !strings.Contains(string(data), "## §1") || !strings.Contains(string(data), "## §6") {
		t.Errorf("rendered design.md missing generated § N headers:\n%s", data)
	}
}

// TestGenerateCapabilityAbsentDeferral is AC-03 for the design gate: a model
// that cannot emit structured output yields ErrStructuredUnsupported (a
// declared Rule 2 deferral), NOT a crash and NOT a written design.md.
func TestGenerateCapabilityAbsentDeferral(t *testing.T) {
	dir := t.TempDir()
	spec := "# Test spec\n\n## User outcome\n\nNo structured capability.\n"

	fa := fakeCaptainDriver{
		errKind: driver.ErrKindUnsupported,
		err:     errors.New("client for \"legacy/model\" does not support structured output"),
	}
	_, err := Generate(context.Background(), dir, spec, fa, "legacy/model", "/tmp/wt", 0, GenerateOptions{})
	if err == nil {
		t.Fatal("expected ErrStructuredUnsupported for a capability-absent model, got nil")
	}
	if !errors.Is(err, ErrStructuredUnsupported) {
		t.Errorf("err = %v, want errors.Is(err, ErrStructuredUnsupported)", err)
	}
	// Must NOT write design.md — capability-absent is a deferral, not a design.
	if _, statErr := os.Stat(filepath.Join(dir, "design.md")); statErr == nil {
		t.Error("design.md was written on a capability-absent deferral; want no artefact")
	}
}

func TestGenerateRespectsExisting(t *testing.T) {
	dir := t.TempDir()
	spec := "# Test spec\n\n## User outcome\n\nA test outcome.\n"

	// Pre-create a design.md.
	existing := "existing content"
	designPath := filepath.Join(dir, "design.md")
	if err := os.WriteFile(designPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without Regenerate, Generate should skip.
	fa := fakeCaptainDriver{structuredJSON: validDesignJSON}
	got, err := Generate(context.Background(), dir, spec, fa, "fake/model", "/tmp/wt", 0, GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != "" {
		t.Fatal("Generate should have returned empty (skipped)")
	}

	// design.md should be untouched.
	data, err := os.ReadFile(designPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != existing {
		t.Fatalf("design.md was overwritten: got %q, want %q", string(data), existing)
	}

	// With Regenerate, Generate should overwrite with the rendered structured design.
	got, err = Generate(context.Background(), dir, spec, fa, "fake/model", "/tmp/wt", 0, GenerateOptions{Regenerate: true})
	if err != nil {
		t.Fatalf("Generate with Regenerate: %v", err)
	}
	if got == "" {
		t.Fatal("Generate with Regenerate returned empty")
	}
	data, err = os.ReadFile(designPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != got || !strings.Contains(string(data), "## §1") {
		t.Fatalf("design.md was not regenerated from the structured emission: got %q", string(data))
	}
}

// TestMissingSections exercises the non-empty section check (D4) that replaces
// the old hasSixSections substring scrape: a schema-present-but-empty field
// fails.
func TestMissingSections(t *testing.T) {
	full := designTLDR{
		UserVisibleChange: "a",
		DesignDecisions:   "b",
		FilesTouched:      "c",
		NotDoing:          "d",
		ReachabilityPlan:  "e",
		OpenQuestions:     "f",
	}
	if m := full.missingSections(); len(m) != 0 {
		t.Errorf("full designTLDR reports missing sections: %v", m)
	}

	empty := full
	empty.NotDoing = "   " // whitespace-only counts as empty
	m := empty.missingSections()
	if len(m) != 1 || !strings.Contains(m[0], "§4") {
		t.Errorf("want §4 reported missing, got %v", m)
	}

	// render produces all six headers in order.
	rendered := full.render()
	last := -1
	for _, marker := range []string{"## §1", "## §2", "## §3", "## §4", "## §5", "## §6"} {
		idx := strings.Index(rendered, marker)
		if idx < 0 {
			t.Errorf("render missing %q", marker)
		}
		if idx < last {
			t.Errorf("render header %q out of order", marker)
		}
		last = idx
	}
}

func TestGenerateModelError(t *testing.T) {
	dir := t.TempDir()
	spec := "# Test\n\n## User outcome\n\nTest.\n"

	fa := fakeCaptainDriver{err: errors.New("model unavailable")}
	_, err := Generate(context.Background(), dir, spec, fa, "fake/model", "/tmp/wt", 0, GenerateOptions{})
	if err == nil {
		t.Fatal("expected error from model, got nil")
	}
	// A generic dispatch error is NOT a capability-absent deferral.
	if errors.Is(err, ErrStructuredUnsupported) {
		t.Errorf("generic model error misclassified as ErrStructuredUnsupported: %v", err)
	}
}

func TestGenerateMissingSections(t *testing.T) {
	dir := t.TempDir()
	spec := "# Test\n\n## User outcome\n\nTest.\n"

	// Structured output parses but §3 (files_touched) is empty — fail-closed.
	partial := `{
  "user_visible_change": "x",
  "design_decisions": "y",
  "files_touched": "",
  "not_doing": "z",
  "reachability_plan": "r",
  "open_questions": "None."
}`
	fa := fakeCaptainDriver{structuredJSON: partial}
	_, err := Generate(context.Background(), dir, spec, fa, "fake/model", "/tmp/wt", 0, GenerateOptions{})
	if err == nil {
		t.Fatal("expected error for missing sections, got nil")
	}
	if !strings.Contains(err.Error(), "missing required sections") {
		t.Errorf("wrong error: %v", err)
	}
}
