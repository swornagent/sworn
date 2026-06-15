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

	// Check key files exist.
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

	// VERSION file should contain the Baton protocol version.
	ver, err := os.ReadFile(filepath.Join(dir, "docs/baton/VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.TrimSpace(string(ver))) == 0 {
		t.Error("VERSION file is empty")
	}
}

func TestSpliceAgentsNoExistingFile(t *testing.T) {
	dir := t.TempDir()

	modified, err := SpliceAgents(dir)
	if err != nil {
		t.Fatalf("SpliceAgents (no AGENTS.md): %v", err)
	}
	if !modified {
		t.Error("expected modified = true when creating AGENTS.md")
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

	// Create AGENTS.md without the Baton section.
	original := "# My Project\n\nSome content.\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	modified, err := SpliceAgents(dir)
	if err != nil {
		t.Fatalf("SpliceAgents (append): %v", err)
	}
	if !modified {
		t.Error("expected modified = true when appending section")
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

func TestSpliceAgentsExistingSectionReplace(t *testing.T) {
	dir := t.TempDir()

	// Create AGENTS.md with an existing, stale Baton section.
	existing := "# My Project\n\nSome content.\n\n" +
		BatonSectionHeading + " (old)\n\nOld rule text here.\n\n" +
		"## Another Section\n\nMore content.\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	modified, err := SpliceAgents(dir)
	if err != nil {
		t.Fatalf("SpliceAgents (replace): %v", err)
	}
	if !modified {
		t.Error("expected modified = true when replacing stale section")
	}

	content, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)

	// Original non-Baton content preserved.
	if !strings.Contains(s, "# My Project") {
		t.Error("project heading should be preserved")
	}
	if !strings.Contains(s, "## Another Section") {
		t.Error("subsequent sections should be preserved")
	}

	// Old section text removed, new section present.
	if strings.Contains(s, "Old rule text here") {
		t.Error("old rule text should be removed")
	}
	if !strings.Contains(s, "fresh-context session") {
		t.Error("new seven-rule content should be present")
	}

	// Baton heading appears exactly once.
	if strings.Count(s, BatonSectionHeading) != 1 {
		t.Errorf("Baton heading count = %d, want 1", strings.Count(s, BatonSectionHeading))
	}
}

func TestSpliceAgentsIdempotent(t *testing.T) {
	dir := t.TempDir()

	// First run creates the section.
	_, err := SpliceAgents(dir)
	if err != nil {
		t.Fatalf("first SpliceAgents: %v", err)
	}

	// Read what was written.
	first, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}

	// Second run should be a no-op.
	modified, err := SpliceAgents(dir)
	if err != nil {
		t.Fatalf("second SpliceAgents: %v", err)
	}
	if modified {
		t.Error("expected modified = false on identical re-run (idempotent)")
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

	// Create AGENTS.md with the current Baton section already present.
	existing := "# My Project\n\nSome content.\n\n" + batonAGENTSFragment + "\n\n## Another Section\n\nMore.\n"
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	modified, err := SpliceAgents(dir)
	if err != nil {
		t.Fatalf("SpliceAgents (already current): %v", err)
	}
	if modified {
		t.Error("expected modified = false when section is already current")
	}
}

func TestMaterialiseIdempotent(t *testing.T) {
	dir := t.TempDir()

	// First run.
	if err := Materialise(dir); err != nil {
		t.Fatalf("first Materialise: %v", err)
	}

	// Second run should succeed (overwrites with same content).
	if err := Materialise(dir); err != nil {
		t.Fatalf("second Materialise: %v", err)
	}

	// Files should still exist.
	p := filepath.Join(dir, "docs/baton/README.md")
	if _, err := os.Stat(p); err != nil {
		t.Errorf("README.md missing after re-run: %v", err)
	}
}