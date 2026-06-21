package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/swornagent/sworn/internal/db"
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

// TestConcurrentStatusPoll verifies that the LiveView polls the DB correctly
// on tick and populates track rows. It uses a real SQLite database in a temp dir.
func TestConcurrentStatusPoll(t *testing.T) {
	dir := t.TempDir()

	// Create a sworn DB with a track in in_progress state.
	dbPath := db.DefaultPath(dir)
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer conn.Close()

	_, err = conn.Exec(
		"INSERT INTO tracks (id, release, state, current_slice, started_at) VALUES (?, ?, ?, ?, ?)",
		"T1-engine", "test-release", "in_progress", "S02-oai-model-client", "2026-06-20T10:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert track: %v", err)
	}

	// Create LiveView pointing at the same DB.
	lv, err := StartLiveView(dir, "test-release")
	if err != nil {
		t.Fatalf("StartLiveView: %v", err)
	}
	defer lv.Close()

	// Verify initial poll populated rows.
	if len(lv.Rows) != 1 {
		t.Fatalf("expected 1 track row, got %d", len(lv.Rows))
	}
	if lv.Rows[0].ID != "T1-engine" {
		t.Errorf("expected track ID T1-engine, got %q", lv.Rows[0].ID)
	}
	if lv.Rows[0].CurrentSlice != "S02-oai-model-client" {
		t.Errorf("expected current_slice S02-oai-model-client, got %q", lv.Rows[0].CurrentSlice)
	}
	if lv.Rows[0].State != "in_progress" {
		t.Errorf("expected state in_progress, got %q", lv.Rows[0].State)
	}
	if lv.Rows[0].Elapsed == "" || lv.Rows[0].Elapsed == "—" {
		t.Errorf("expected non-empty elapsed time, got %q", lv.Rows[0].Elapsed)
	}

	// Advance one tick.
	tickCount := lv.TickCount
	lv2, _ := lv.Update(tickMsg{})
	if lv2.TickCount <= tickCount {
		t.Errorf("expected TickCount to increase after tick, was %d now %d", tickCount, lv2.TickCount)
	}

	// Rows should still be populated after tick.
	if len(lv2.Rows) != 1 {
		t.Fatalf("expected 1 track row after tick, got %d", len(lv2.Rows))
	}
}

// TestAutoTransitionToLive verifies that when a release has in-progress tracks
// in the DB, pressing Enter auto-transitions to viewLive.
func TestAutoTransitionToLive(t *testing.T) {
	dir := t.TempDir()

	// Create release structure.
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)
	createIndex(t, dir, "test-release", "Test Release")

	// Create a sworn DB with in-progress track.
	dbPath := db.DefaultPath(dir)
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	_, err = conn.Exec(
		"INSERT INTO tracks (id, release, state, current_slice, started_at) VALUES (?, ?, ?, ?, ?)",
		"T1-engine", "test-release", "in_progress", "S01-first", "2026-06-20T10:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert track: %v", err)
	}
	conn.Close()

	m := &Model{
		state:    viewReleases,
		repoRoot: dir,
		Releases: &ReleasesList{},
		Board:    &BoardView{},
	}
	if err := m.Releases.LoadReleases(dir); err != nil {
		t.Fatalf("LoadReleases: %v", err)
	}

	// Press Enter to select the release.
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := upd.(*Model)

	// Should auto-transition to live view.
	if m2.state != viewLive {
		t.Fatalf("expected viewLive after Enter (has in-progress tracks), got %d", m2.state)
	}
	if m2.Live == nil {
		t.Fatal("expected Live to be non-nil after auto-transition")
	}
	if m2.Live.ReleaseName != "test-release" {
		t.Fatalf("expected Live release test-release, got %q", m2.Live.ReleaseName)
	}
}

// TestAutoTransitionNoTracks verifies that when a release has NO in-progress tracks,
// pressing Enter goes to board view (no auto-transition).
func TestAutoTransitionNoTracks(t *testing.T) {
	dir := t.TempDir()

	// Create release structure with a sliced status but no SQLite DB.
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)
	indexContent := `---
tracks:
  - id: T1-core
    slices: [S01-first]
    depends_on:
    state: in_progress
---`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0644)
	createSliceStatus(t, releaseDir, "S01-first", "verified", "T1-core")

	m := &Model{
		state:    viewReleases,
		repoRoot: dir,
		Releases: &ReleasesList{},
		Board:    &BoardView{},
	}
	if err := m.Releases.LoadReleases(dir); err != nil {
		t.Fatalf("LoadReleases: %v", err)
	}

	// Press Enter to select the release.
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := upd.(*Model)

	// Should stay on board view (no DB with in-progress tracks).
	if m2.state != viewBoard {
		t.Fatalf("expected viewBoard after Enter (no in-progress tracks), got %d", m2.state)
	}
}

// TestLiveBoardToggle verifies that pressing l in board view and b in live view
// toggles correctly between the two views.
func TestLiveBoardToggle(t *testing.T) {
	dir := t.TempDir()

	// Create release structure.
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)
	createIndex(t, dir, "test-release", "Test Release")

	// Create a sworn DB with in-progress track.
	dbPath := db.DefaultPath(dir)
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	_, err = conn.Exec(
		"INSERT INTO tracks (id, release, state, current_slice, started_at) VALUES (?, ?, ?, ?, ?)",
		"T1-engine", "test-release", "in_progress", "S01-first", "2026-06-20T10:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert track: %v", err)
	}
	conn.Close()

	m := &Model{
		state:    viewReleases,
		repoRoot: dir,
		Releases: &ReleasesList{},
		Board:    &BoardView{},
	}
	if err := m.Releases.LoadReleases(dir); err != nil {
		t.Fatalf("LoadReleases: %v", err)
	}

	// Press Enter to auto-transition to live view.
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := upd.(*Model)
	if m2.state != viewLive {
		t.Fatalf("expected viewLive, got %d", m2.state)
	}

	// Press b to go back to board.
	upd, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	m3 := upd.(*Model)
	if m3.state != viewBoard {
		t.Fatalf("expected viewBoard after b, got %d", m3.state)
	}

	// Press l to go back to live.
	upd, _ = m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m4 := upd.(*Model)
	if m4.state != viewLive {
		t.Fatalf("expected viewLive after l, got %d", m4.state)
	}

	// Press Esc to go back to releases.
	upd, _ = m4.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	m5 := upd.(*Model)
	if m5.state != viewReleases {
		t.Fatalf("expected viewReleases after Esc from live, got %d", m5.state)
	}
}

// TestCreditBalanceDisplayed verifies that when a credits file exists with balance 42,
// the CreditFileBalance function returns "42".
func TestCreditBalanceDisplayed(t *testing.T) {
	// Create a temporary home directory.
	tmpHome := t.TempDir()
	creditDir := filepath.Join(tmpHome, ".config", "sworn")
	os.MkdirAll(creditDir, 0o755)
	creditFile := filepath.Join(creditDir, "credits.json")
	os.WriteFile(creditFile, []byte(`{"balance": 42}`), 0644)

	// Temporarily override HOME.
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	bal, ok := CreditFileBalance()
	if !ok {
		t.Fatal("expected CreditFileBalance to return ok=true")
	}
	if bal != "42" {
		t.Errorf("expected balance '42', got %q", bal)
	}
}

// TestCreditBalanceAbsent verifies that when no credits file exists,
// CreditFileBalance returns "–" and ok=false.
func TestCreditBalanceAbsent(t *testing.T) {
	// Create a temporary home directory with no credits file.
	tmpHome := t.TempDir()

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	bal, ok := CreditFileBalance()
	if ok {
		t.Fatal("expected CreditFileBalance to return ok=false when no file")
	}
	if bal != "–" {
		t.Errorf("expected balance '–', got %q", bal)
	}
}

// TestModelTickForwarding verifies that tickMsg sent through Model.Update()
// reaches LiveView, increments TickCount, and re-polls DB rows.
// This is the integration-level test the spec requires — it exercises the
// tick through the root Model.Update(), not directly on LiveView.Update().
func TestModelTickForwarding(t *testing.T) {
	dir := t.TempDir()

	// Create release structure.
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)
	createIndex(t, dir, "test-release", "Test Release")

	// Create a sworn DB with an in-progress track.
	dbPath := db.DefaultPath(dir)
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	_, err = conn.Exec(
		"INSERT INTO tracks (id, release, state, current_slice, started_at) VALUES (?, ?, ?, ?, ?)",
		"T1-engine", "test-release", "in_progress", "S02-oai-model-client", "2026-06-20T10:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert track: %v", err)
	}
	conn.Close()

	m := &Model{
		state:    viewReleases,
		repoRoot: dir,
		Releases: &ReleasesList{},
		Board:    &BoardView{},
	}
	if err := m.Releases.LoadReleases(dir); err != nil {
		t.Fatalf("LoadReleases: %v", err)
	}

	// Step 1: Press Enter to auto-transition to live view.
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := upd.(*Model)

	if m2.state != viewLive {
		t.Fatalf("expected viewLive after Enter (has in-progress tracks), got %d", m2.state)
	}
	if m2.Live == nil {
		t.Fatal("expected Live to be non-nil after auto-transition")
	}
	initialTickCount := m2.Live.TickCount
	if len(m2.Live.Rows) == 0 {
		t.Fatal("expected Live.Rows to be populated after initial poll")
	}

	// Step 2: Send a tickMsg through Model.Update() — this must reach LiveView
	// via the tickMsg case in Model.Update(), not directly via LiveView.Update().
	upd2, _ := m2.Update(tickMsg{})
	m3 := upd2.(*Model)

	if m3.Live.TickCount <= initialTickCount {
		t.Errorf("expected TickCount to increase after tickMsg sent through Model.Update(), "+
			"was %d now %d", initialTickCount, m3.Live.TickCount)
	}
	if len(m3.Live.Rows) == 0 {
		t.Fatal("expected Live.Rows to be populated after tick through Model.Update()")
	}
	if m3.Live.Rows[0].ID != "T1-engine" {
		t.Errorf("expected track ID T1-engine, got %q", m3.Live.Rows[0].ID)
	}

	// Step 3: Send a second tick to verify the chain stays alive (tick-command
	// returned by Live.Update() is consumed by Bubble Tea and produces the
	// next tickMsg, which our test sends manually).
	beforeSecondTick := m3.Live.TickCount
	upd3, _ := m3.Update(tickMsg{})
	m4 := upd3.(*Model)
	if m4.Live.TickCount <= beforeSecondTick {
		t.Errorf("expected TickCount to increase after second tick, "+
			"was %d now %d", beforeSecondTick, m4.Live.TickCount)
	}
}

// TestLiveViewClose verifies that Close() cleans up the connection.
func TestLiveViewClose(t *testing.T) {
	dir := t.TempDir()

	// Create a sworn DB.
	dbPath := db.DefaultPath(dir)
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer conn.Close()

	_, err = conn.Exec(
		"INSERT INTO tracks (id, release, state, current_slice, started_at) VALUES (?, ?, ?, ?, ?)",
		"T1-engine", "test-release", "in_progress", "S01-first", "2026-06-20T10:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert track: %v", err)
	}

	lv, err := StartLiveView(dir, "test-release")
	if err != nil {
		t.Fatalf("StartLiveView: %v", err)
	}

	if err := lv.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestElapsedTimeFormatting verifies the computeElapsed function formats
// durations correctly.
func TestElapsedTimeFormatting(t *testing.T) {
	// Use a fixed now time.
	now := mustParseTime(t, "2026-06-20T10:05:30Z")
	tests := []struct {
		startedAt string
		expected  string
	}{
		{"", "—"},
		{"2026-06-20T10:05:25Z", "5s"},
		{"2026-06-20T10:04:00Z", "1m30s"},
		{"2026-06-20T10:00:00Z", "5m30s"},
		{"2026-06-20T09:00:00Z", "1h5m30s"},
	}

	for _, tc := range tests {
		got := computeElapsed(tc.startedAt, now)
		if got != tc.expected {
			t.Errorf("computeElapsed(%q) = %q, want %q", tc.startedAt, got, tc.expected)
		}
	}
}

// TestHasInProgressTracks verifies the DB query function.
func TestHasInProgressTracks(t *testing.T) {
	dir := t.TempDir()

	// No DB yet.
	if HasInProgressTracks(dir, "test-release") {
		t.Fatal("expected false when no DB exists")
	}

	// Create DB with no in-progress tracks.
	dbPath := db.DefaultPath(dir)
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	_, err = conn.Exec(
		"INSERT INTO tracks (id, release, state, current_slice, started_at) VALUES (?, ?, ?, ?, ?)",
		"T1-engine", "test-release", "verified", "S01-first", "2026-06-20T10:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert track: %v", err)
	}
	conn.Close()

	if HasInProgressTracks(dir, "test-release") {
		t.Fatal("expected false when no in-progress tracks")
	}

	// Add an in-progress track and re-check.
	conn, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	_, err = conn.Exec(
		"INSERT INTO tracks (id, release, state, current_slice, started_at) VALUES (?, ?, ?, ?, ?)",
		"T2-engine", "test-release", "in_progress", "S02-second", "2026-06-20T10:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert track: %v", err)
	}
	conn.Close()

	if !HasInProgressTracks(dir, "test-release") {
		t.Fatal("expected true after adding in-progress track")
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

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	return parsed
}

func TestBlockedPanelExtractsViolations(t *testing.T) {
	proofContent := `---
title: Proof Bundle
---

## Violations
- Violation 1: spec mismatch
- Violation 2: test failed

## Not delivered
- Deferral 1: out of scope
`
	violations := ExtractViolations(proofContent)
	if len(violations) != 3 {
		t.Fatalf("expected 3 violations, got %d: %v", len(violations), violations)
	}
	if violations[0] != "Violation 1: spec mismatch" {
		t.Errorf("expected 'Violation 1: spec mismatch', got %q", violations[0])
	}
	if violations[1] != "Violation 2: test failed" {
		t.Errorf("expected 'Violation 2: test failed', got %q", violations[1])
	}
	if violations[2] != "Deferral 1: out of scope" {
		t.Errorf("expected 'Deferral 1: out of scope', got %q", violations[2])
	}
}

func TestOpenAIWritesContextFile(t *testing.T) {
	tmpDir := t.TempDir()
	spec := "Spec content"
	violations := "Violation 1\nViolation 2"
	diff := "Git diff content"

	path, err := WriteContextFile(tmpDir, spec, violations, diff)
	if err != nil {
		t.Fatalf("WriteContextFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading context file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "Spec content") {
		t.Errorf("expected context file to contain spec content")
	}
	if !strings.Contains(content, "Violation 1") {
		t.Errorf("expected context file to contain violations")
	}
	if !strings.Contains(content, "Git diff content") {
		t.Errorf("expected context file to contain diff")
	}
}

func TestLaunchMissingTool(t *testing.T) {
	os.Setenv("SWORN_CLAUDE_CODE_CMD", "non-existent-command-12345")
	defer os.Unsetenv("SWORN_CLAUDE_CODE_CMD")

	err := LaunchClaudeCode("/tmp")
	if err == nil {
		t.Fatal("expected error when launching missing tool, got nil")
	}

	bv := &BlockedView{
		sliceID:      "S01-test",
		releaseName:  "test-release",
		worktreePath: "/tmp",
		violations:   []string{"Violation 1"},
	}

	bv2, _ := bv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	if !strings.Contains(bv2.message, "Claude Code not found") {
		t.Errorf("expected message to contain 'Claude Code not found', got %q", bv2.message)
	}
}

func TestDeferWritesRuleTwo(t *testing.T) {
	tmpDir := t.TempDir()
	releaseDir := filepath.Join(tmpDir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	indexContent := `---
tracks:
  - id: T1-core
    slices: [S01-first]
    worktree_path: ` + tmpDir + `
---`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0644)

	createSliceStatus(t, releaseDir, "S01-first", "failed_verification", "T1-core")

	bv, err := LoadBlockedView(tmpDir, "test-release", "S01-first")
	if err != nil {
		t.Fatalf("LoadBlockedView: %v", err)
	}

	bv2, _ := bv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	if !bv2.deferring {
		t.Fatal("expected deferring=true")
	}

	reason := "Not enough time"
	for _, r := range reason {
		bv2, _ = bv2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	bv3, _ := bv2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if bv3.deferring {
		t.Fatal("expected deferring=false after confirm")
	}
	if bv3.errMessage != "" {
		t.Fatalf("unexpected error: %s", bv3.errMessage)
	}

	st, err := state.Read(filepath.Join(releaseDir, "S01-first", "status.json"))
	if err != nil {
		t.Fatalf("reading status.json: %v", err)
	}
	if st.State != state.Deferred {
		t.Errorf("expected state 'deferred', got %q", st.State)
	}

	intakeData, err := os.ReadFile(filepath.Join(releaseDir, "intake.md"))
	if err != nil {
		t.Fatalf("reading intake.md: %v", err)
	}
	intakeContent := string(intakeData)
	if !strings.Contains(intakeContent, "S01-first") {
		t.Errorf("expected intake.md to contain slice ID")
	}
	if !strings.Contains(intakeContent, "Not enough time") {
		t.Errorf("expected intake.md to contain reason")
	}
	if !strings.Contains(intakeContent, "Why") {
		t.Errorf("expected intake.md to contain 'Why'")
	}
	if !strings.Contains(intakeContent, "Acknowledged") {
		t.Errorf("expected intake.md to contain 'Acknowledged'")
	}
}

func TestBoardEnterTransitionsToBlocked(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	createIndex(t, dir, "test-release", "Test Release")

	indexContent := `---
title: Test Release
tracks:
  - id: T1-core
    slices: [S01-first]
    state: in_progress
---`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0644)

	createSliceStatus(t, releaseDir, "S01-first", "failed_verification", "T1-core")

	m := &Model{
		state:    viewReleases,
		repoRoot: dir,
		Releases: &ReleasesList{},
		Board:    &BoardView{},
	}
	if err := m.Releases.LoadReleases(dir); err != nil {
		t.Fatalf("LoadReleases: %v", err)
	}

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := upd.(*Model)
	if m2.state != viewBoard {
		t.Fatalf("expected viewBoard state, got %d", m2.state)
	}

	upd2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := upd2.(*Model)
	if m3.state != viewBlocked {
		t.Fatalf("expected viewBlocked state after Enter on failed slice, got %d", m3.state)
	}
	if m3.Blocked == nil {
		t.Fatal("expected Blocked view to be loaded")
	}
	if m3.Blocked.sliceID != "S01-first" {
		t.Errorf("expected Blocked slice ID 'S01-first', got %q", m3.Blocked.sliceID)
	}
}

// TestBoardEnterTransitionsToBlockedOnImplementedBlockedVerdict verifies Pin 1:
// a slice at state "implemented" with verification.result == "blocked" also
// transitions to the blocked panel when Enter is pressed. BLOCKED verdicts
// leave the slice at "implemented" — they are NOT "failed_verification".
func TestBoardEnterTransitionsToBlockedOnImplementedBlockedVerdict(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	indexContent := `---
title: Test Release
tracks:
  - id: T1-core
    slices: [S01-blocked]
    state: in_progress
---`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0644)

	// Create a slice at "implemented" with verification.result == "blocked"
	sliceDir := filepath.Join(releaseDir, "S01-blocked")
	os.MkdirAll(sliceDir, 0o755)
	st := &state.Status{
		SliceID: "S01-blocked",
		State:   state.Implemented,
		Track:   "T1-core",
		Verification: state.Verification{
			Result: "blocked",
		},
	}
	if err := state.Write(filepath.Join(sliceDir, "status.json"), st); err != nil {
		t.Fatal(err)
	}

	m := &Model{
		state:    viewReleases,
		repoRoot: dir,
		Releases: &ReleasesList{},
		Board:    &BoardView{},
	}
	if err := m.Releases.LoadReleases(dir); err != nil {
		t.Fatalf("LoadReleases: %v", err)
	}

	// Enter to select release → board view
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := upd.(*Model)
	if m2.state != viewBoard {
		t.Fatalf("expected viewBoard state, got %d", m2.state)
	}

	// Enter on the implemented+blocked slice → should go to blocked panel
	upd2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 := upd2.(*Model)
	if m3.state != viewBlocked {
		t.Fatalf("expected viewBlocked state after Enter on implemented+blocked slice, got %d", m3.state)
	}
	if m3.Blocked == nil {
		t.Fatal("expected Blocked view to be loaded")
	}
	if m3.Blocked.sliceID != "S01-blocked" {
		t.Errorf("expected Blocked slice ID 'S01-blocked', got %q", m3.Blocked.sliceID)
	}
}

// TestBlockedPanelViewProof verifies that pressing [4] on the blocked panel
// opens a scrollable view of the raw proof.md content, and Esc returns to
// the options panel.
func TestBlockedPanelViewProof(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	indexContent := `---
title: Test Release
tracks:
  - id: T1-core
    slices: [S01-first]
    state: in_progress
---`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0644)

	createSliceStatus(t, releaseDir, "S01-first", "failed_verification", "T1-core")

	// Write a proof.md
	proofContent := `# Proof Bundle

## Not delivered
- Something was not delivered
`
	os.WriteFile(filepath.Join(releaseDir, "S01-first", "proof.md"), []byte(proofContent), 0644)

	bv, err := LoadBlockedView(dir, "test-release", "S01-first")
	if err != nil {
		t.Fatalf("LoadBlockedView: %v", err)
	}

	// Press [4] to view proof
	bv2, _ := bv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("4")})
	if !bv2.viewingProof {
		t.Fatal("expected viewingProof=true after pressing [4]")
	}

	// View should contain the raw proof content
	view := bv2.View()
	if !strings.Contains(view, "Proof Bundle") {
		t.Errorf("expected proof view to contain 'Proof Bundle', got: %s", view)
	}

	// Press Esc to return to options
	bv3, _ := bv2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("esc")})
	if bv3.viewingProof {
		t.Fatal("expected viewingProof=false after Esc")
	}

	// View should now show the options menu
	view = bv3.View()
	if !strings.Contains(view, "Resolution Options") {
		t.Errorf("expected options menu after Esc, got: %s", view)
	}
}
