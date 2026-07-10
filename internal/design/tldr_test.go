package design

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
)

// fakeCaptainDriver is a minimal driver.Driver returning a canned captain
// response (S06: the wire-typed fakeAgent became a driver fake; Generate now
// dispatches Role=captain through the driver seam).
type fakeCaptainDriver struct {
	text string
	err  error
	last *driver.DispatchInput
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
		return driver.Result{Status: driver.StatusError, ErrKind: driver.ErrKindConfig}, f.err
	}
	return driver.Result{Status: driver.StatusOK, ResultText: f.text}, nil
}

const cannedTLDR = `## §1 User-visible change

Users will see a design.md file appear in the slice directory before any code changes.

## §2 Design decisions not in the spec

- Use a single-shot model call, not an agent tool loop
- Write design.md atomically with os.WriteFile
- Section check is substring-based on §1–§6 markers

## §3 Files I'll touch by purpose

- internal/design/tldr.go — Generate function that calls the model and writes design.md
- internal/design/tldr_test.go — unit tests for Generate

## §4 Things I'm NOT doing

- NOT implementing the captain review stage
- NOT blocking implementation on TL;DR content

## §5 Reachability plan

Run the Generate function with a spec and observe design.md created in the slice directory.

## §6 Open questions

None.
`

func TestGenerateWritesSixSections(t *testing.T) {
	dir := t.TempDir()
	spec := "# Test spec\n\n## User outcome\n\nA test outcome.\n"

	fa := fakeCaptainDriver{text: cannedTLDR}
	got, err := Generate(context.Background(), dir, spec, fa, "fake/model", "/tmp/wt", 0, GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got == "" {
		t.Fatal("Generate returned empty text")
	}
	if !hasSixSections(got) {
		t.Fatal("generated text missing six sections")
	}

	// Verify design.md was written.
	designPath := filepath.Join(dir, "design.md")
	data, err := os.ReadFile(designPath)
	if err != nil {
		t.Fatalf("design.md not written: %v", err)
	}
	if string(data) != cannedTLDR {
		t.Fatalf("design.md content mismatch\nwant:\n%s\ngot:\n%s", cannedTLDR, string(data))
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
	fa := fakeCaptainDriver{text: cannedTLDR}
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

	// With Regenerate, Generate should overwrite.
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
	if string(data) != cannedTLDR {
		t.Fatalf("design.md was not regenerated: got %q, want cannedTLDR", string(data))
	}
}

func TestHasSixSections(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{"all present", cannedTLDR, true},
		{"missing §4", strings.ReplaceAll(cannedTLDR, "§4", ""), false},
		{"empty", "", false},
		{"only §1", "§1 something", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasSixSections(tt.text); got != tt.want {
				t.Errorf("hasSixSections = %v, want %v", got, tt.want)
			}
		})
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
}

func TestGenerateMissingSections(t *testing.T) {
	dir := t.TempDir()
	spec := "# Test\n\n## User outcome\n\nTest.\n"

	fa := fakeCaptainDriver{text: "Just some text without section markers."}
	_, err := Generate(context.Background(), dir, spec, fa, "fake/model", "/tmp/wt", 0, GenerateOptions{})
	if err == nil {
		t.Fatal("expected error for missing sections, got nil")
	}
	if !strings.Contains(err.Error(), "missing required sections") {
		t.Errorf("wrong error: %v", err)
	}
}
