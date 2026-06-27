package adopt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaterialiseWritesDocs(t *testing.T) {
	dir := t.TempDir()

	if err := Materialise(dir); err != nil {
		t.Fatalf("Materialise: %v", err)
	}

	for _, rel := range []string{
		"docs/baton/README.md",
		"docs/baton/VERSION",
		"docs/baton/rules/01-reachability-gate.md",
		"docs/baton/rules/07-adversarial-verification.md",
	} {
		p := filepath.Join(dir, rel)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing file %s: %v", rel, err)
		}
	}

	ver, err := os.ReadFile(filepath.Join(dir, "docs/baton/VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.TrimSpace(string(ver))) == 0 {
		t.Error("VERSION file is empty")
	}
}

func TestMaterialiseIdempotent(t *testing.T) {
	dir := t.TempDir()

	if err := Materialise(dir); err != nil {
		t.Fatalf("first Materialise: %v", err)
	}
	if err := Materialise(dir); err != nil {
		t.Fatalf("second Materialise: %v", err)
	}

	p := filepath.Join(dir, "docs/baton/README.md")
	if _, err := os.Stat(p); err != nil {
		t.Errorf("README.md missing after re-run: %v", err)
	}
}

// resultFor returns the SpliceResult for the named file, or fails the test.
func resultFor(t *testing.T, results []SpliceResult, name string) SpliceResult {
	t.Helper()
	for _, r := range results {
		if r.File == name {
			return r
		}
	}
	t.Fatalf("no result found for %s", name)
	return SpliceResult{}
}

func TestSpliceAgentsNoExistingFile(t *testing.T) {
	dir := t.TempDir()

	results, err := SpliceAgents(dir, false)
	if err != nil {
		t.Fatalf("SpliceAgents (no AGENTS.md): %v", err)
	}

	r := resultFor(t, results, "AGENTS.md")
	if r.Action != SpliceCreated {
		t.Errorf("AGENTS.md action = %v, want SpliceCreated", r.Action)
	}

	content, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), BatonSectionHeading) {
		t.Error("AGENTS.md missing Baton heading")
	}
	if !strings.Contains(string(content), "fresh-context session") {
		t.Error("AGENTS.md missing seven-rule content")
	}
}

func TestSpliceAgentsExistingNoSection(t *testing.T) {
	dir := t.TempDir()

	original := "# My Project\n\nSome content.\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	results, err := SpliceAgents(dir, false)
	if err != nil {
		t.Fatalf("SpliceAgents (append): %v", err)
	}

	r := resultFor(t, results, "AGENTS.md")
	if r.Action != SpliceAppended {
		t.Errorf("AGENTS.md action = %v, want SpliceAppended", r.Action)
	}

	content, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), original) {
		t.Error("original content should be preserved")
	}
	if !strings.Contains(string(content), BatonSectionHeading) {
		t.Error("Baton section should be appended")
	}
}

func TestSpliceAgentsCustomizedSectionSkippedWithoutForce(t *testing.T) {
	dir := t.TempDir()

	// Write an AGENTS.md with a customized Baton section.
	existing := "# My Project\n\nSome content.\n\n" +
		BatonSectionHeading + "\n\nCustom rule text — tailored for this repo.\n\n" +
		"## Another Section\n\nMore content.\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	results, err := SpliceAgents(dir, false)
	if err != nil {
		t.Fatalf("SpliceAgents (customized, no force): %v", err)
	}

	r := resultFor(t, results, "AGENTS.md")
	if r.Action != SpliceCustomized {
		t.Errorf("AGENTS.md action = %v, want SpliceCustomized", r.Action)
	}

	// File must be unchanged.
	content, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != existing {
		t.Error("customized file should not be modified without --force")
	}
}

func TestSpliceAgentsCustomizedSectionReplacedWithForce(t *testing.T) {
	dir := t.TempDir()

	existing := "# My Project\n\nSome content.\n\n" +
		BatonSectionHeading + "\n\nCustom rule text — tailored for this repo.\n\n" +
		"## Another Section\n\nMore content.\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	results, err := SpliceAgents(dir, true)
	if err != nil {
		t.Fatalf("SpliceAgents (customized, force): %v", err)
	}

	r := resultFor(t, results, "AGENTS.md")
	if r.Action != SpliceUpdated {
		t.Errorf("AGENTS.md action = %v, want SpliceUpdated", r.Action)
	}

	content, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)

	if !strings.Contains(s, "# My Project") {
		t.Error("project heading should be preserved")
	}
	if !strings.Contains(s, "## Another Section") {
		t.Error("subsequent sections should be preserved")
	}
	if strings.Contains(s, "Custom rule text") {
		t.Error("custom rule text should be replaced with --force")
	}
	if !strings.Contains(s, "fresh-context session") {
		t.Error("new seven-rule content should be present after force")
	}
	if strings.Count(s, BatonSectionHeading) != 1 {
		t.Errorf("Baton heading count = %d, want 1", strings.Count(s, BatonSectionHeading))
	}
}

func TestSpliceAgentsIdempotent(t *testing.T) {
	dir := t.TempDir()

	if _, err := SpliceAgents(dir, false); err != nil {
		t.Fatalf("first SpliceAgents: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}

	results, err := SpliceAgents(dir, false)
	if err != nil {
		t.Fatalf("second SpliceAgents: %v", err)
	}

	r := resultFor(t, results, "AGENTS.md")
	if r.Action != SpliceNoOp {
		t.Errorf("AGENTS.md action = %v, want SpliceNoOp on re-run", r.Action)
	}

	second, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("file content changed on re-run (not idempotent)")
	}
}

func TestSpliceAgentsIdempotentWhenSectionAlreadyCurrent(t *testing.T) {
	dir := t.TempDir()

	existing := "# My Project\n\nSome content.\n\n" + batonAGENTSFragment + "\n\n## Another Section\n\nMore.\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	results, err := SpliceAgents(dir, false)
	if err != nil {
		t.Fatalf("SpliceAgents (already current): %v", err)
	}

	r := resultFor(t, results, "AGENTS.md")
	if r.Action != SpliceNoOp {
		t.Errorf("AGENTS.md action = %v, want SpliceNoOp when section already current", r.Action)
	}
}

func TestSpliceAgentsCLAUDEMDAbsent(t *testing.T) {
	dir := t.TempDir()

	results, err := SpliceAgents(dir, false)
	if err != nil {
		t.Fatalf("SpliceAgents: %v", err)
	}

	r := resultFor(t, results, "CLAUDE.md")
	if r.Action != SpliceAbsent {
		t.Errorf("CLAUDE.md action = %v, want SpliceAbsent when file does not exist", r.Action)
	}

	// CLAUDE.md must not be created.
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Error("CLAUDE.md should not be created when absent")
	}
}

func TestSpliceAgentsCLAUDEMDExisting(t *testing.T) {
	dir := t.TempDir()

	original := "# CLAUDE.md\n\nProject notes.\n"
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	results, err := SpliceAgents(dir, false)
	if err != nil {
		t.Fatalf("SpliceAgents: %v", err)
	}

	r := resultFor(t, results, "CLAUDE.md")
	if r.Action != SpliceAppended {
		t.Errorf("CLAUDE.md action = %v, want SpliceAppended", r.Action)
	}

	content, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), original) {
		t.Error("original CLAUDE.md content should be preserved")
	}
	if !strings.Contains(string(content), BatonSectionHeading) {
		t.Error("Baton section should be spliced into CLAUDE.md")
	}
}

func TestPlanSpliceDoesNotWrite(t *testing.T) {
	dir := t.TempDir()

	results, err := PlanSplice(dir, false)
	if err != nil {
		t.Fatalf("PlanSplice: %v", err)
	}

	r := resultFor(t, results, "AGENTS.md")
	if r.Action != SpliceCreated {
		t.Errorf("PlanSplice AGENTS.md action = %v, want SpliceCreated", r.Action)
	}

	// AGENTS.md must NOT have been created — PlanSplice is read-only.
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Error("PlanSplice must not write any files")
	}
}
