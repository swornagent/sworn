package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverClaudeCode(t *testing.T) {
	dir := t.TempDir()

	// Create MEMORY.md
	memoryMD := `- [First Entry](first.md)
- [Second Entry](second.md)
- Just a bullet
`
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(memoryMD), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "first.md"), []byte("first content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "second.md"), []byte("second content"), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := discoverClaudeCode(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Title != "First Entry" || entries[0].Content != "first content" {
		t.Errorf("unexpected first entry: %+v", entries[0])
	}
	if entries[1].Title != "Second Entry" || entries[1].Content != "second content" {
		t.Errorf("unexpected second entry: %+v", entries[1])
	}
}

func TestDiscoverFlatFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".cursorrules")

	content := `# Rule 1
Do this.
---
# Rule 2
Do that.
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := discoverFlatFile(path, "cursor")
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Title != "Rule 1" || entries[0].Content != "# Rule 1\nDo this." {
		t.Errorf("unexpected first entry: %+v", entries[0])
	}
	if entries[1].Title != "Rule 2" || entries[1].Content != "# Rule 2\nDo that." {
		t.Errorf("unexpected second entry: %+v", entries[1])
	}
	if entries[0].Path != path+"#0" || entries[1].Path != path+"#1" {
		t.Errorf("unexpected paths: %s, %s", entries[0].Path, entries[1].Path)
	}
}

func TestDiscoverCustomPath(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("A content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("B content"), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := discoverCustomPath(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}
