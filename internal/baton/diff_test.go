package baton

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffCleanWhenInSync(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("testdata", "fixture"))
	if err != nil {
		t.Fatal(err)
	}

	tmpRepo := t.TempDir()
	for _, m := range batonFileMappings {
		destDir := filepath.Join(tmpRepo, filepath.Dir(m.Dest))
		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Vendor first to populate the embed.
	_, err = Vendor(VendorOpts{
		SourceDir: fixture,
		RepoRoot:  tmpRepo,
		CheckOnly: false,
	})
	if err != nil {
		t.Fatalf("Vendor() error = %v", err)
	}

	// Diff against the freshly-vendored embed — should be clean.
	divs, err := Diff(DiffOpts{
		SourceDir: fixture,
		RepoRoot:  tmpRepo,
	})
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	if len(divs) != 0 {
		for _, d := range divs {
			t.Errorf("unexpected divergence: %s: %s", d.File, d.Reason)
		}
	}
}

func TestDiffDetectsHandEditedEmbed(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("testdata", "fixture"))
	if err != nil {
		t.Fatal(err)
	}

	tmpRepo := t.TempDir()
	for _, m := range batonFileMappings {
		destDir := filepath.Join(tmpRepo, filepath.Dir(m.Dest))
		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Vendor first.
	_, err = Vendor(VendorOpts{
		SourceDir: fixture,
		RepoRoot:  tmpRepo,
		CheckOnly: false,
	})
	if err != nil {
		t.Fatalf("Vendor() error = %v", err)
	}

	// Hand-edit an embedded rule file: inject text that diverges from the
	// transformed source.
	ruleFile := filepath.Join(tmpRepo, "internal/adopt/baton/rules/01-reachability-gate.md")
	orig, err := os.ReadFile(ruleFile)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(orig), "sworn verify", "sworn verify (FORKED)", 1)
	if err := os.WriteFile(ruleFile, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}
	divs, err := Diff(DiffOpts{
		SourceDir: fixture,
		RepoRoot:  tmpRepo,
	})
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	if len(divs) == 0 {
		t.Fatal("Diff() returned no divergences after hand-editing an embed file")
	}

	found := false
	for _, d := range divs {
		if d.File == "internal/adopt/baton/rules/01-reachability-gate.md" {
			found = true
			if d.Reason == "" {
				t.Error("Divergence has empty Reason")
			}
		}
	}
	if !found {
		t.Errorf("divergent file not in results; got: %+v", divs)
	}
}

func TestDiffDetectsMissingEmbedFile(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("testdata", "fixture"))
	if err != nil {
		t.Fatal(err)
	}

	tmpRepo := t.TempDir()
	for _, m := range batonFileMappings {
		destDir := filepath.Join(tmpRepo, filepath.Dir(m.Dest))
		if err := os.MkdirAll(destDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Vendor first.
	_, err = Vendor(VendorOpts{
		SourceDir: fixture,
		RepoRoot:  tmpRepo,
		CheckOnly: false,
	})
	if err != nil {
		t.Fatalf("Vendor() error = %v", err)
	}

	// Delete one embed file.
	missingFile := filepath.Join(tmpRepo, "internal/adopt/baton/rules/02-no-silent-deferrals.md")
	if err := os.Remove(missingFile); err != nil {
		t.Fatal(err)
	}

	divs, err := Diff(DiffOpts{
		SourceDir: fixture,
		RepoRoot:  tmpRepo,
	})
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}

	found := false
	for _, d := range divs {
		if d.File == "internal/adopt/baton/rules/02-no-silent-deferrals.md" {
			found = true
			if !strings.Contains(d.Reason, "missing") {
				t.Errorf("missing file Reason = %q, want 'missing'", d.Reason)
			}
		}
	}
	if !found {
		t.Errorf("missing file not in divergences; got: %+v", divs)
	}
}

func TestDiffFailsOnMissingSource(t *testing.T) {
	dir := t.TempDir()
	_, err := Diff(DiffOpts{
		SourceDir: dir,
		RepoRoot:  dir,
	})
	if err == nil {
		t.Fatal("Diff(empty dir) = nil, want error")
	}
	if !strings.Contains(err.Error(), "source file missing") {
		t.Errorf("error = %v, want 'source file missing'", err)
	}
}
