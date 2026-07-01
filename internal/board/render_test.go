package board

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -update regenerates the golden file: go test ./internal/board/ -run TestRenderGolden -update
var updateGolden = flag.Bool("update", false, "regenerate render golden file")

const (
	fixtureRoot    = "testdata/render"
	fixtureRelease = "rel-fixture"
	goldenPath     = "testdata/render/rel-fixture.golden.md"
)

// TestRenderGolden pins the exact rendered output (AC-01) and proves the render
// is deterministic and idempotent (AC-02): rendering the same inputs twice is
// byte-identical, and matches the committed golden file.
func TestRenderGolden(t *testing.T) {
	got, err := Render(fixtureRoot, fixtureRelease)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	// Idempotency (AC-02): a second render of the same inputs is byte-identical.
	got2, err := Render(fixtureRoot, fixtureRelease)
	if err != nil {
		t.Fatalf("Render (2nd): %v", err)
	}
	if got != got2 {
		t.Fatalf("Render is not idempotent: two renders of the same inputs differ")
	}

	if *updateGolden {
		if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("golden regenerated at %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if got != string(want) {
		t.Errorf("rendered output does not match golden %s.\n--- got ---\n%s\n--- want ---\n%s",
			goldenPath, got, string(want))
	}
}

// TestRenderFrontmatterValidates asserts the generated frontmatter parses
// cleanly through the same validator that guards the board against the
// frontmatter-fusion failure class (AC-03).
func TestRenderFrontmatterValidates(t *testing.T) {
	got, err := Render(fixtureRoot, fixtureRelease)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if errs := ValidateIndex("rendered", got); len(errs) != 0 {
		t.Errorf("rendered index.md failed ValidateIndex: %v", errs)
	}
	// Frontmatter scalars must be single-quoted (AC-03).
	if !strings.Contains(got, "title: 'Release board") {
		t.Errorf("title is not a single-quoted YAML scalar")
	}
	if !strings.Contains(got, "description: '") {
		t.Errorf("description is not a single-quoted YAML scalar")
	}
}

// TestRenderReproducesTracks is the AC-05 property on the fixture: the tracks
// table contains every track id, and the touchpoint matrix marks no file under
// two tracks (the disjointness the matrix exists to prove).
func TestRenderReproducesTracks(t *testing.T) {
	got, err := Render(fixtureRoot, fixtureRelease)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, id := range []string{"TA-alpha", "TB-beta"} {
		if !strings.Contains(got, "`"+id+"`") {
			t.Errorf("tracks table missing track %q", id)
		}
	}
	// Matrix disjointness: no data row (a `path` row) may carry more than one ✓.
	for _, line := range strings.Split(got, "\n") {
		if !strings.HasPrefix(line, "| `internal/") {
			continue // only file rows begin with a backticked path
		}
		if n := strings.Count(line, "✓"); n > 1 {
			t.Errorf("touchpoint matrix marks a file under %d tracks (not disjoint): %s", n, line)
		}
	}
}

// TestRenderFailsClosedMissingBoard asserts AC-04 for the missing-board case:
// a release dir with no board.json returns an error, and RenderToFile writes no
// index.md (it must NOT fall through to ReadBoard's lazy migration-from-index).
func TestRenderFailsClosedMissingBoard(t *testing.T) {
	root := t.TempDir()
	relDir := filepath.Join(root, "docs", "release", "empty-rel")
	if err := os.MkdirAll(relDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Plant an index.md so a lazy-migration fallback would have something to read.
	if err := os.WriteFile(filepath.Join(relDir, "index.md"), []byte("---\ntitle: 'x'\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := Render(root, "empty-rel"); err == nil {
		t.Fatalf("Render: expected error for missing board.json, got nil")
	}
	if err := RenderToFile(root, "empty-rel"); err == nil {
		t.Fatalf("RenderToFile: expected error for missing board.json, got nil")
	}
	// The planted index.md must be untouched (no partial write); no new one created
	// elsewhere. Assert index.md content is exactly what we planted.
	data, err := os.ReadFile(filepath.Join(relDir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "---\ntitle: 'x'\n---\n" {
		t.Errorf("index.md was modified on a failed render (fail-closed violated): %q", string(data))
	}
}

// TestRenderFailsClosedStringBoard asserts AC-04 for a present-but-invalid
// board: a legacy bare-string `release` fails closed through the strict reader.
func TestRenderFailsClosedStringBoard(t *testing.T) {
	root := t.TempDir()
	relDir := filepath.Join(root, "docs", "release", "str-rel")
	if err := os.MkdirAll(relDir, 0755); err != nil {
		t.Fatal(err)
	}
	board := `{"$schema":"x","schema_version":1,"release":"str-rel","tracks":[{"id":"T1","slices":[],"worktree_branch":"b","state":"planned"}]}`
	if err := os.WriteFile(filepath.Join(relDir, "board.json"), []byte(board), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Render(root, "str-rel"); err == nil {
		t.Fatalf("Render: expected error for bare-string release board, got nil")
	}
	if _, err := os.Stat(filepath.Join(relDir, "index.md")); !os.IsNotExist(err) {
		t.Errorf("index.md was written despite a failed render (fail-closed violated)")
	}
}

// TestRenderFailsClosedMissingSliceRecord asserts AC-04 when a referenced slice
// is missing its spec/status record.
func TestRenderFailsClosedMissingSliceRecord(t *testing.T) {
	root := t.TempDir()
	relDir := filepath.Join(root, "docs", "release", "gap-rel")
	if err := os.MkdirAll(relDir, 0755); err != nil {
		t.Fatal(err)
	}
	board := `{"$schema":"x","schema_version":1,"release":{"name":"gap-rel"},"tracks":[{"id":"T1","slices":["S99-missing"],"worktree_branch":"b","state":"planned"}]}`
	if err := os.WriteFile(filepath.Join(relDir, "board.json"), []byte(board), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Render(root, "gap-rel"); err == nil {
		t.Fatalf("Render: expected error for missing slice record, got nil")
	}
}

// TestRenderToFileWritesGolden asserts RenderToFile writes exactly the Render
// output (build-then-write) to index.md.
func TestRenderToFileWritesGolden(t *testing.T) {
	// Copy the fixture into a temp root so the write does not dirty testdata.
	root := t.TempDir()
	src := filepath.Join(fixtureRoot, "docs", "release", fixtureRelease)
	dst := filepath.Join(root, "docs", "release", fixtureRelease)
	copyTree(t, src, dst)

	want, err := Render(root, fixtureRelease)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if err := RenderToFile(root, fixtureRelease); err != nil {
		t.Fatalf("RenderToFile: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "index.md"))
	if err != nil {
		t.Fatalf("read written index.md: %v", err)
	}
	if string(got) != want {
		t.Errorf("RenderToFile wrote content differing from Render output")
	}
}

// copyTree recursively copies a directory tree (test helper).
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			copyTree(t, s, d)
			continue
		}
		data, err := os.ReadFile(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(d, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}
