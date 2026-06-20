package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/swornagent/sworn/internal/state"
)

// TestReleasesListPopulates verifies that given a fixture docs/release/ directory
// with two index.md files, the releases list model contains exactly those two entries.
func TestReleasesListPopulates(t *testing.T) {
	dir := t.TempDir()
	fixtureReleaseDir(filepath.Join(dir, "docs", "release"))
	createIndex(t, dir, "release-alpha", "Release Alpha")
	createIndex(t, dir, "release-beta", "Release Beta")

	rl := &ReleasesList{}
	if err := rl.LoadReleases(dir); err != nil {
		t.Fatalf("LoadReleases: %v", err)
	}

	if len(rl.Releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(rl.Releases))
	}
	if rl.Releases[0].Name != "Release Alpha" {
		t.Errorf("expected first release to be 'Release Alpha', got %q", rl.Releases[0].Name)
	}
	if rl.Releases[1].Name != "Release Beta" {
		t.Errorf("expected second release to be 'Release Beta', got %q", rl.Releases[1].Name)
	}
}

// TestBoardViewShowsSlices verifies that given a fixture release with 3 slices
// at known states, the board view model contains those states after board.Load().
func TestBoardViewShowsSlices(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	// Create index.md with 2 tracks.
	indexContent := `---
tracks:
  - id: T1-core
    slices: [S01-first, S02-second]
    depends_on:
    state: in_progress
  - id: T2-extras
    slices: [S03-third]
    depends_on: T1-core
    state: planned
---`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0644)

	// Create slice directories with status.json files.
	createSliceStatus(t, releaseDir, "S01-first", "verified", "T1-core")
	createSliceStatus(t, releaseDir, "S02-second", "in_progress", "T1-core")
	createSliceStatus(t, releaseDir, "S03-third", "planned", "T2-extras")

	bv := &BoardView{}
	if err := bv.LoadBoard(dir, "test-release"); err != nil {
		t.Fatalf("LoadBoard: %v", err)
	}

	if !bv.Loaded {
		t.Fatal("expected Loaded=true")
	}
	if len(bv.Tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(bv.Tracks))
	}

	// Check slice states.
	checkSlice(t, bv, "S01-first", "verified")
	checkSlice(t, bv, "S02-second", "in_progress")
	checkSlice(t, bv, "S03-third", "planned")
}

// TestKeyNavigation simulates j, k, Enter, Esc keypresses on the model
// and asserts correct view transitions.
func TestKeyNavigation(t *testing.T) {
	dir := t.TempDir()
	fixtureReleaseDir(filepath.Join(dir, "docs", "release"))
	createIndex(t, dir, "release-alpha", "Release Alpha")
	createIndex(t, dir, "release-beta", "Release Beta")

	m := &Model{
		state:    viewReleases,
		repoRoot: dir,
		Releases: &ReleasesList{},
		Board:    &BoardView{},
	}
	if err := m.Releases.LoadReleases(dir); err != nil {
		t.Fatalf("LoadReleases: %v", err)
	}

	// Initial state: viewReleases, cursor at 0.
	if m.state != viewReleases {
		t.Fatalf("expected viewReleases state, got %d", m.state)
	}
	if m.Releases.Cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.Releases.Cursor)
	}

	// Press j to move down.
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m2 := upd.(*Model)
	if m2.Releases.Cursor != 1 {
		t.Fatalf("expected cursor 1 after j, got %d", m2.Releases.Cursor)
	}

	// Press k to move back up.
	upd, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m3 := upd.(*Model)
	if m3.Releases.Cursor != 0 {
		t.Fatalf("expected cursor 0 after k, got %d", m3.Releases.Cursor)
	}

	// Press Enter to select release and enter board view.
	upd, _ = m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m4 := upd.(*Model)
	if m4.state != viewBoard {
		t.Fatalf("expected viewBoard state after Enter, got %d", m4.state)
	}
	if !m4.Board.Loaded {
		t.Fatal("expected board to be loaded after Enter")
	}

	// Press Esc to go back to releases view.
	upd, _ = m4.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	m5 := upd.(*Model)
	if m5.state != viewReleases {
		t.Fatalf("expected viewReleases state after Esc, got %d", m5.state)
	}
}

// TestHelpToggle verifies that pressing ? toggles the help overlay.
func TestHelpToggle(t *testing.T) {
	m := &Model{
		state:    viewReleases,
		Releases: &ReleasesList{},
		Board:    &BoardView{},
	}

	if m.showHelp {
		t.Fatal("expected showHelp=false initially")
	}

	// Press ? to show help.
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m2 := upd.(*Model)
	if !m2.showHelp {
		t.Fatal("expected showHelp=true after pressing ?")
	}

	// Press ? again to hide.
	upd, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m3 := upd.(*Model)
	if m3.showHelp {
		t.Fatal("expected showHelp=false after second ?")
	}
}

// TestQuit verifies q quits the program.
func TestQuit(t *testing.T) {
	m := &Model{
		state:    viewReleases,
		Releases: &ReleasesList{},
		Board:    &BoardView{},
	}

	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m2 := upd.(*Model)
	if m2.state != viewQuit {
		t.Fatalf("expected viewQuit state after q, got %d", m2.state)
	}
	if cmd == nil {
		t.Fatal("expected a Quit command after q")
	}
}

// Helpers.

func fixtureReleaseDir(dir string) {
	os.MkdirAll(dir, 0o755)
}

func createIndex(t *testing.T, root string, releaseID, title string) {
	t.Helper()
	releaseDir := filepath.Join(root, "docs", "release", releaseID)
	os.MkdirAll(releaseDir, 0o755)
	content := "---\ntitle: " + title + "\n---\n"
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(content), 0644)
}
func createSliceStatus(t *testing.T, releaseDir, sliceID, sliceState, track string) {
	t.Helper()
	sliceDir := filepath.Join(releaseDir, sliceID)
	os.MkdirAll(sliceDir, 0o755)

	st := &state.Status{
		SliceID: sliceID,
		State:   state.State(sliceState),
		Track:   track,
	}
	if err := state.Write(filepath.Join(sliceDir, "status.json"), st); err != nil {
		t.Fatal(err)
	}
}

func checkSlice(t *testing.T, bv *BoardView, sliceID, expectedState string) {
	t.Helper()
	si, ok := bv.Slices[sliceID]
	if !ok {
		t.Fatalf("slice %s not found in board", sliceID)
	}
	if si.State != expectedState {
		t.Errorf("slice %s: expected state %q, got %q", sliceID, expectedState, si.State)
	}
}