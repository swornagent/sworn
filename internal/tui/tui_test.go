package tui

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/swornagent/sworn/internal/board"
	"github.com/swornagent/sworn/internal/db"
	"github.com/swornagent/sworn/internal/git"
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
	if rl.Releases[0].Name != "release-alpha" {
		t.Errorf("expected first release identity to be 'release-alpha', got %q", rl.Releases[0].Name)
	}
	if rl.Releases[1].Name != "release-beta" {
		t.Errorf("expected second release identity to be 'release-beta', got %q", rl.Releases[1].Name)
	}
}

// TestBoardViewShowsSlices verifies that given a fixture release with 3 slices
// at known states, the board view model contains those states after board.Load().
// This is the exact reproduction test for the originally reported bug
// (AC-01): fixture is built via writeBoardFixture (the real board.WriteBoard
// path), not a hand-authored `tracks:` YAML string literal (AC-04).
func TestBoardViewShowsSlices(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeBoardFixture(t, dir, "test-release", []board.BoardTrack{
		{
			ID:     "T1-core",
			Slices: []string{"S01-first", "S02-second"},
		},
		{
			ID:        "T2-extras",
			Slices:    []string{"S03-third"},
			DependsOn: board.StringList{"T1-core"},
		},
	})

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

// TestBoardViewLegacyIndexFallback verifies a release with NO board.json still
// renders through the shared, read-only catalog fallback.
func TestBoardViewLegacyIndexFallback(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "legacy-release")
	os.MkdirAll(releaseDir, 0o755)

	indexContent := `---
tracks:
  - id: T1-legacy
    slices: [S01-only]
    depends_on:
    worktree_branch: track/legacy-release/T1-legacy
    state: in_progress
---`
	os.WriteFile(filepath.Join(releaseDir, "index.md"), []byte(indexContent), 0644)
	createSliceStatus(t, releaseDir, "S01-only", "in_progress", "T1-legacy")

	bv := &BoardView{}
	if err := bv.LoadBoard(dir, "legacy-release"); err != nil {
		t.Fatalf("LoadBoard: %v", err)
	}

	if !bv.Loaded {
		t.Fatal("expected Loaded=true")
	}
	if len(bv.Tracks) != 1 || bv.Tracks[0].ID != "T1-legacy" {
		t.Fatalf("expected 1 track T1-legacy via legacy fallback, got %+v", bv.Tracks)
	}
	checkSlice(t, bv, "S01-only", "in_progress")

	// Catalog discovery is read-only and must not lazily migrate index.md.
	if _, err := os.Stat(filepath.Join(releaseDir, "board.json")); !os.IsNotExist(err) {
		t.Errorf("expected no board.json write during catalog fallback, stat err=%v", err)
	}
}

// TestBoardViewResolvesStateFromTrackBranch reproduces sworn#81: the primary
// checkout's working-tree status.json is stale (design_review) while the
// slice's owning track branch already carries the authoritative, more
// advanced state (verified) — the exact "S01 verified on the oracle, planned
// on the TUI" shape from the live driver-contract bug report. LoadBoard must
// resolve the slice's state via the git-ref oracle (same ownership-keyed
// path `sworn board`/the MCP ops tools use), not the primary working tree.
func TestBoardViewResolvesStateFromTrackBranch(t *testing.T) {
	dir := t.TempDir()
	release := "oracle-release"
	releaseDir := filepath.Join(dir, "docs", "release", release)
	os.MkdirAll(releaseDir, 0o755)

	repo := git.New(dir)
	if err := repo.Init(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := repo.Config("user.email", "test@example.com"); err != nil {
		t.Fatalf("git config user.email: %v", err)
	}
	if err := repo.Config("user.name", "Test"); err != nil {
		t.Fatalf("git config user.name: %v", err)
	}
	primaryBranch := currentBranch(t, dir)

	writeBoardFixture(t, dir, release, []board.BoardTrack{
		{
			ID:     "T1-core",
			Slices: []string{"S01-first"},
		},
	})
	// Stale copy: what the primary working tree still has on disk.
	createSliceStatus(t, releaseDir, "S01-first", "design_review", "T1-core")

	if err := repo.Stage("."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := repo.Commit("initial: stale design_review state"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	// The owning track branch has moved on: verified, committed there, but
	// never synced back into the primary checkout's filesystem.
	if err := repo.Branch("track/" + release + "/T1-core"); err != nil {
		t.Fatalf("git branch: %v", err)
	}
	createSliceStatus(t, releaseDir, "S01-first", "verified", "T1-core")
	if err := repo.Stage("."); err != nil {
		t.Fatalf("git add (track branch): %v", err)
	}
	if err := repo.Commit("track: S01 verified"); err != nil {
		t.Fatalf("git commit (track branch): %v", err)
	}

	// Return to the primary branch — the working tree now reflects the
	// stale design_review status.json again, exactly like the live bug.
	if err := repo.Checkout(primaryBranch); err != nil {
		t.Fatalf("git checkout %s: %v", primaryBranch, err)
	}
	if got := mustReadSliceState(t, releaseDir, "S01-first"); got != "design_review" {
		t.Fatalf("test setup: expected working tree to show stale design_review, got %q", got)
	}

	bv := &BoardView{}
	if err := bv.LoadBoard(dir, release); err != nil {
		t.Fatalf("LoadBoard: %v", err)
	}

	// The authoritative state lives on the track branch (verified), not the
	// stale primary-checkout filesystem copy (design_review).
	checkSlice(t, bv, "S01-first", "verified")
}

// TestBoardViewLiveWorktreeStateNotMaskedByLastCommit is the regression test
// for the fresh-verifier FAIL on sworn#81's first attempt: when the slice's
// owning track branch IS the branch currently checked out in repoRoot (the
// common serial/solo `sworn run` shape — one worktree doing everything, no
// separate track worktree), the oracle's git-ref read must not shadow an
// uncommitted state.Write() to the live status.json. internal/run/slice.go
// writes state repeatedly and only commits at specific milestones, so the
// working tree is routinely ahead of the last commit on the slice's own
// branch during a live run.
func TestBoardViewLiveWorktreeStateNotMaskedByLastCommit(t *testing.T) {
	dir := t.TempDir()
	release := "live-release"
	releaseDir := filepath.Join(dir, "docs", "release", release)
	os.MkdirAll(releaseDir, 0o755)

	repo := git.New(dir)
	if err := repo.Init(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := repo.Config("user.email", "test@example.com"); err != nil {
		t.Fatalf("git config user.email: %v", err)
	}
	if err := repo.Config("user.name", "Test"); err != nil {
		t.Fatalf("git config user.name: %v", err)
	}
	// board-v1 is a pure plan: the track branch is DERIVED as track/<release>/T1-core.
	// No such ref exists in this single-branch setup, so the oracle resolves the
	// slice via the release-wt/HEAD fallback (working tree), which is the point.
	writeBoardFixture(t, dir, release, []board.BoardTrack{
		{
			ID:     "T1-core",
			Slices: []string{"S01-first"},
		},
	})
	createSliceStatus(t, releaseDir, "S01-first", "planned", "T1-core")

	if err := repo.Stage("."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := repo.Commit("initial: planned"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	// Rewrite status.json on disk WITHOUT committing — exactly what
	// internal/run/slice.go's state.Write() calls do between commit
	// milestones during a live run.
	createSliceStatus(t, releaseDir, "S01-first", "in_progress", "T1-core")

	bv := &BoardView{}
	if err := bv.LoadBoard(dir, release); err != nil {
		t.Fatalf("LoadBoard: %v", err)
	}

	// The live, uncommitted working-tree state must win: the primary
	// checkout IS the track's own branch here, so the last commit is stale
	// relative to the filesystem, not authoritative over it.
	checkSlice(t, bv, "S01-first", "in_progress")
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

	// Press Enter to select release and enter board view. Board loading is
	// dispatched as a tea.Cmd (sworn#82) — drive it to completion the same
	// way the bubbletea runtime would: run the returned Cmd and feed its
	// msg back through Update.
	upd, cmd := m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m4 := upd.(*Model)
	if m4.state != viewBoard {
		t.Fatalf("expected viewBoard state after Enter, got %d", m4.state)
	}
	if cmd == nil {
		t.Fatal("expected a board-load Cmd after Enter")
	}
	upd, _ = m4.Update(cmd())
	m4 = upd.(*Model)
	if !m4.Board.Loaded {
		t.Fatal("expected board to be loaded after the async load completes")
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
	createIndex(t, dir, "test-release", "Test Release")
	writeBoardFixture(t, dir, "test-release", []board.BoardTrack{
		{ID: "T1-core", Slices: []string{"S01-first"}},
	})
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

func TestRefOnlyReleaseListAndBoardUsesCatalogSnapshot(t *testing.T) {
	dir := refOnlyCatalogFixture(t)

	m := newReleaseModel(t, dir)
	if len(m.Releases.Releases) != 1 {
		t.Fatalf("releases = %+v, want one ref-only release", m.Releases.Releases)
	}
	selected := m.Releases.Releases[0]
	if selected.ID != "ref-only-release" || selected.SourceRef != "refs/heads/release-wt/ref-only-release" {
		t.Fatalf("selected release = %+v", selected)
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*Model)
	if cmd == nil || m.Board.Loaded || !m.Board.Loading {
		t.Fatalf("Enter state: cmd=%v loaded=%v loading=%v", cmd != nil, m.Board.Loaded, m.Board.Loading)
	}
	updated, _ = m.Update(cmd())
	m = updated.(*Model)
	if !m.Board.Loaded || m.Board.SourceRef != selected.SourceRef {
		t.Fatalf("loaded board = %+v", m.Board)
	}
	if len(m.Board.Tracks) != 1 || m.Board.Tracks[0].ID != "T1-core" {
		t.Fatalf("tracks = %+v", m.Board.Tracks)
	}
	got := m.Board.Slices["S01-alpha"]
	if got.State != "verified" || got.StateSource != "refs/heads/track/ref-only-release/T1-core" || got.StateDurability != "committed" {
		t.Fatalf("catalog state did not reach TUI board unchanged: %+v", got)
	}
}

func TestRefOnlyBoardAndCLIStateEvidenceAgree(t *testing.T) {
	dir := refOnlyCatalogFixture(t)
	catalog, err := board.DiscoverCatalog(git.New(dir))
	if err != nil {
		t.Fatalf("DiscoverCatalog (sworn board authority): %v", err)
	}
	if len(catalog) != 1 {
		t.Fatalf("catalog = %+v", catalog)
	}
	want := catalog[0].Board.Tracks[0].Slices[0]

	m := newReleaseModel(t, dir)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*Model)
	updated, _ = m.Update(cmd())
	m = updated.(*Model)
	got := m.Board.Slices[want.ID]
	if got.State != string(want.State) || got.StateSource != want.StateSource || got.StateDurability != want.StateDurability || m.Board.SourceRef != catalog[0].SourceRef {
		t.Fatalf("TUI state %+v differs from board catalog %+v (sourceRef %q)", got, want, catalog[0].SourceRef)
	}
}

func TestBoardViewRendersCatalogStateDurability(t *testing.T) {
	record := testCatalogRecord("alpha", "refs/heads/release-wt/alpha", "uncommitted")
	bv := &BoardView{}
	if err := bv.LoadBoardFromCatalog(t.TempDir(), record); err != nil {
		t.Fatal(err)
	}
	if got := bv.View(); !strings.Contains(got, "S01-alpha") || !strings.Contains(got, "[uncommitted]") {
		t.Fatalf("uncommitted board marker missing:\n%s", got)
	}

	record.Board.Tracks[0].Slices[0].StateDurability = "committed"
	if err := bv.LoadBoardFromCatalog(t.TempDir(), record); err != nil {
		t.Fatal(err)
	}
	if got := bv.View(); strings.Contains(got, "[uncommitted]") {
		t.Fatalf("committed board rendered uncommitted marker:\n%s", got)
	}
}

func TestReleasesListRendersCatalogUncommittedAggregate(t *testing.T) {
	uncommitted := releaseInfoFromCatalog(testCatalogRecord("alpha", "", "uncommitted"))
	committed := releaseInfoFromCatalog(testCatalogRecord("zeta", "", "committed"))
	rl := &ReleasesList{Releases: []ReleaseInfo{uncommitted, committed}}
	if got := rl.View(); !strings.Contains(got, "alpha") || !strings.Contains(got, " [uncommitted]") {
		t.Fatalf("selected uncommitted release marker missing:\n%s", got)
	}
	rl.Cursor = 1
	if got := rl.View(); strings.Contains(got, "[uncommitted]") {
		t.Fatalf("unselected evidence or committed selection rendered marker:\n%s", got)
	}
}

func TestReleasesListCatalogFailureIsVisible(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "broken")
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(releaseDir, "board.json"), []byte(`{"release":{"name":"other"},"tracks":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	m := &Model{state: viewReleases, repoRoot: dir, Releases: &ReleasesList{}, Board: &BoardView{}}
	err := m.Releases.LoadReleases(dir)
	if err == nil {
		t.Fatal("expected identity-mismatched board error")
	}
	m.errMsg = err.Error()
	if len(m.Releases.Releases) != 0 {
		t.Fatalf("catalog failure populated partial releases: %+v", m.Releases.Releases)
	}
	if got := m.View(); !strings.Contains(got, "Error: "+err.Error()) {
		t.Fatalf("error not visible unchanged, want %q in:\n%s", err.Error(), got)
	}
}

func TestReleasesListUsesSharedNonGitCatalogFallback(t *testing.T) {
	dir := t.TempDir()
	writeBoardFixture(t, dir, "local-alpha", []board.BoardTrack{{ID: "T1-core", Slices: []string{"S01-alpha"}}})
	createSliceStatus(t, filepath.Join(dir, "docs", "release", "local-alpha"), "S01-alpha", "verified", "T1-core")

	m := newReleaseModel(t, dir)
	if got := m.Releases.Releases[0]; got.ID != "local-alpha" || got.SourceRef != "" || !got.HasUncommittedEvidence {
		t.Fatalf("filesystem catalog release = %+v", got)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*Model)
	updated, _ = m.Update(cmd())
	m = updated.(*Model)
	if !m.Board.Loaded || m.Board.Tracks[0].ID != "T1-core" || m.Board.Slices["S01-alpha"].StateDurability != "uncommitted" {
		t.Fatalf("filesystem catalog did not load through Model: %+v", m.Board)
	}
}

func TestRefAwareReleaseInteractionContract(t *testing.T) {
	dir := t.TempDir()
	for _, release := range []string{"zeta", "alpha"} {
		writeBoardFixture(t, dir, release, []board.BoardTrack{{ID: "T1-core", Slices: []string{"S01-alpha"}}})
		createSliceStatus(t, filepath.Join(dir, "docs", "release", release), "S01-alpha", "planned", "T1-core")
	}
	m := newReleaseModel(t, dir)
	if m.Releases.Releases[0].ID != "alpha" || m.Releases.Cursor != 0 {
		t.Fatalf("initial releases = %+v cursor=%d", m.Releases.Releases, m.Releases.Cursor)
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("j")}, {Type: tea.KeyDown}, {Type: tea.KeyRunes, Runes: []rune("k")}, {Type: tea.KeyUp}, {Type: tea.KeyDown}} {
		updated, _ := m.Update(key)
		m = updated.(*Model)
	}
	if m.Releases.Cursor != 1 || m.Releases.Releases[m.Releases.Cursor].ID != "zeta" {
		t.Fatalf("navigation cursor=%d releases=%+v", m.Releases.Cursor, m.Releases.Releases)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*Model)
	if cmd == nil || m.Board.ReleaseName != "zeta" || m.Board.Loaded || !m.Board.Loading || !strings.Contains(m.Board.View(), "Loading…") {
		t.Fatalf("async Enter contract failed: board=%+v cmd=%v view=%q", m.Board, cmd != nil, m.Board.View())
	}
	updated, _ = m.Update(cmd())
	m = updated.(*Model)
	if !m.Board.Loaded || m.Board.ReleaseName != "zeta" {
		t.Fatalf("async result did not load zeta: %+v", m.Board)
	}
}

func TestRefOnlyBoardLoadRejectsStaleDifferentSourceRef(t *testing.T) {
	m := &Model{
		state: viewBoard,
		Board: &BoardView{ReleaseName: "ref-only-release", SourceRef: "refs/heads/release-wt/ref-only-release", Loading: true},
	}
	stale := &BoardView{ReleaseName: "ref-only-release", SourceRef: "refs/heads/topic", Loaded: true}
	updated, _ := m.Update(boardLoadedMsg{releaseName: "ref-only-release", sourceRef: "refs/heads/topic", board: stale})
	m = updated.(*Model)
	if m.Board == stale || m.Board.Loaded || !m.Board.Loading || m.Board.SourceRef != "refs/heads/release-wt/ref-only-release" {
		t.Fatalf("stale different-sourceRef result replaced selection: %+v", m.Board)
	}
}

func testCatalogRecord(release, sourceRef, durability string) board.CatalogRecord {
	return board.CatalogRecord{
		Release:   release,
		SourceRef: sourceRef,
		Board: &board.BoardState{Release: release, Tracks: []board.TrackState{{
			ID:    "T1-core",
			State: "verified",
			Slices: []board.SliceState{{
				ID:              "S01-alpha",
				Track:           "T1-core",
				State:           "verified",
				StateSource:     sourceRef,
				StateDurability: durability,
			}},
		}}},
	}
}

func newReleaseModel(t *testing.T, dir string) *Model {
	t.Helper()
	m := &Model{state: viewReleases, repoRoot: dir, Releases: &ReleasesList{}, Board: &BoardView{}}
	if err := m.Releases.LoadReleases(dir); err != nil {
		t.Fatalf("LoadReleases: %v", err)
	}
	return m
}

func refOnlyCatalogFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runFixtureGit(t, dir, "init", "-b", "main")
	runFixtureGit(t, dir, "config", "user.email", "test@example.com")
	runFixtureGit(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runFixtureGit(t, dir, "add", "README.md")
	runFixtureGit(t, dir, "commit", "-m", "initial")
	runFixtureGit(t, dir, "checkout", "-b", "release-wt/ref-only-release")
	releaseDir := filepath.Join(dir, "docs", "release", "ref-only-release")
	writeBoardFixture(t, dir, "ref-only-release", []board.BoardTrack{{ID: "T1-core", Slices: []string{"S01-alpha"}}})
	createSliceStatus(t, releaseDir, "S01-alpha", "planned", "T1-core")
	runFixtureGit(t, dir, "add", "docs")
	runFixtureGit(t, dir, "commit", "-m", "release plan")
	runFixtureGit(t, dir, "checkout", "-b", "track/ref-only-release/T1-core")
	createSliceStatus(t, releaseDir, "S01-alpha", "verified", "T1-core")
	runFixtureGit(t, dir, "add", "docs")
	runFixtureGit(t, dir, "commit", "-m", "verified state")
	runFixtureGit(t, dir, "checkout", "main")
	return dir
}

func runFixtureGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
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

// writeBoardFixture writes a board.json fixture via the real board.WriteBoard
// write path (validated against the board-v1 schema) — AC-04: test fixtures
// are built through the real render/internal/board machinery, not a
// hand-authored legacy `tracks:` YAML string literal.
func writeBoardFixture(t *testing.T, root, release string, tracks []board.BoardTrack) {
	t.Helper()
	br := &board.BoardRecord{
		Release: board.StringRelease(release),
		Tracks:  tracks,
	}
	if err := board.WriteBoard(root, release, br); err != nil {
		t.Fatalf("writeBoardFixture: board.WriteBoard: %v", err)
	}
}

func createSliceStatus(t *testing.T, releaseDir, sliceID, sliceState, track string) {
	t.Helper()
	sliceDir := filepath.Join(releaseDir, sliceID)
	os.MkdirAll(sliceDir, 0o755)

	st := &state.Status{
		SliceID: sliceID,
		Release: filepath.Base(releaseDir), // required by validate-on-write
		State:   state.State(sliceState),
		Track:   track,
	}
	if err := state.Write(filepath.Join(sliceDir, "status.json"), st); err != nil {
		t.Fatal(err)
	}
}

// currentBranch returns the short name of the branch currently checked out
// in dir (equivalent to `git symbolic-ref --short HEAD`).
func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git symbolic-ref: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// mustReadSliceState reads a slice's status.json directly off disk (bypassing
// the oracle) so the test can assert the fixture setup produced the intended
// stale-on-disk / ahead-on-branch shape before exercising LoadBoard.
func mustReadSliceState(t *testing.T, releaseDir, sliceID string) string {
	t.Helper()
	st, err := state.Read(filepath.Join(releaseDir, sliceID, "status.json"))
	if err != nil {
		t.Fatalf("mustReadSliceState: %v", err)
	}
	return string(st.State)
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

// TestBlockedPanelExtractsViolations verifies AC-03: violations come from
// proof.json's not_delivered array, not a proof.md regex scrape. A proof.md
// with "## Violations"/"## Not delivered" sections in the SAME fixture is
// deliberately ignored — proving the scraper is gone, not just unused.
func TestBlockedPanelExtractsViolations(t *testing.T) {
	proofJSON, err := json.Marshal(map[string]any{
		"$schema":        "https://baton.sawy3r.net/schemas/proof-v1.json",
		"schema_version": 1,
		"slice_id":       "S01-first",
		"release":        "test-release",
		"not_delivered": []string{
			"Deferral 1: out of scope",
			"Deferral 2: needs follow-up",
		},
	})
	if err != nil {
		t.Fatalf("marshal proof.json fixture: %v", err)
	}

	violations := ExtractViolations(proofJSON)
	if len(violations) != 2 {
		t.Fatalf("expected 2 violations, got %d: %v", len(violations), violations)
	}
	if violations[0] != "Deferral 1: out of scope" {
		t.Errorf("expected 'Deferral 1: out of scope', got %q", violations[0])
	}
	if violations[1] != "Deferral 2: needs follow-up" {
		t.Errorf("expected 'Deferral 2: needs follow-up', got %q", violations[1])
	}
}

// TestBlockedPanelViolationsFromProofJSONNotProofMD verifies LoadBlockedView
// (the integration point) reads violations from proof.json.not_delivered
// even when a stray proof.md with "## Violations" bullets sits in the same
// slice directory — proving the proof.md scrape path is fully retired
// (AC-03), not just deprioritised.
func TestBlockedPanelViolationsFromProofJSONNotProofMD(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeBoardFixture(t, dir, "test-release", []board.BoardTrack{
		{ID: "T1-core", Slices: []string{"S01-first"}},
	})
	createSliceStatus(t, releaseDir, "S01-first", "failed_verification", "T1-core")

	// Decoy proof.md — must NOT be scraped for violations.
	decoyProofMD := `# Proof Bundle

## Violations
- LEGACY-SCRAPE-MARKER: this must never surface as a violation
`
	os.WriteFile(filepath.Join(releaseDir, "S01-first", "proof.md"), []byte(decoyProofMD), 0644)

	proofJSON, err := json.Marshal(map[string]any{
		"not_delivered": []string{"Real violation from proof.json"},
	})
	if err != nil {
		t.Fatalf("marshal proof.json fixture: %v", err)
	}
	os.WriteFile(filepath.Join(releaseDir, "S01-first", "proof.json"), proofJSON, 0644)

	bv, err := LoadBlockedView(dir, "test-release", "S01-first")
	if err != nil {
		t.Fatalf("LoadBlockedView: %v", err)
	}

	if len(bv.violations) != 1 || bv.violations[0] != "Real violation from proof.json" {
		t.Fatalf("expected violations from proof.json.not_delivered only, got: %v", bv.violations)
	}
	for _, v := range bv.violations {
		if strings.Contains(v, "LEGACY-SCRAPE-MARKER") {
			t.Errorf("proof.md was scraped for violations — the legacy path must be fully retired, got: %v", bv.violations)
		}
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

	writeBoardFixture(t, tmpDir, "test-release", []board.BoardTrack{
		{ID: "T1-core", Slices: []string{"S01-first"}},
	})

	createSliceStatus(t, releaseDir, "S01-first", "failed_verification", "T1-core")

	bv, err := LoadBlockedView(tmpDir, "test-release", "S01-first")
	if err != nil {
		t.Fatalf("LoadBlockedView: %v", err)
	}
	// board-v1 is a pure plan: the worktree path is DERIVED for the owning track
	// (found via Slices membership), a sibling of the release worktree (sworn#80).
	wantWT := board.TrackWorktreePathFrom(board.ReleaseWorktreePathFrom(tmpDir, "test-release"), "test-release", "T1-core")
	if bv.worktreePath != wantWT {
		t.Fatalf("expected derived worktreePath %q, got %q", wantWT, bv.worktreePath)
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

// TestBlockedPanelWorktreeSurvivesStaleTrackField verifies AC-02's real
// robustness requirement: worktree_path resolution must survive a stale
// status.json.track field (e.g. left behind by a /replan-release track
// rename), not just resolve correctly when the two happen to agree.
// status.json.track is a hint, never the authoritative match key — the
// authoritative key is the slice's membership in a board track's Slices
// list, matching S04's AssembleSliceContext (internal/mcp/context.go)
// pattern exactly. Board fixture: the track's ID has been renamed to
// "T1-core-renamed" (as /replan-release would do) but still lists the
// target slice in Slices; status.json.track still says the OLD id
// "T1-core". A match on t.ID == st.Track would silently fall back to
// repoRoot here — the exact silently-wrong-fallback behaviour AC-02 exists
// to eliminate, just reached via a different trigger (stale track field)
// than the slice's original bug (frontmatter parse failure).
func TestBlockedPanelWorktreeSurvivesStaleTrackField(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeBoardFixture(t, dir, "test-release", []board.BoardTrack{
		{ID: "T1-core-renamed", Slices: []string{"S01-first"}},
	})
	// status.json.track deliberately stale: still points at the track's
	// pre-rename ID, which no longer exists in board.json.
	createSliceStatus(t, releaseDir, "S01-first", "failed_verification", "T1-core")

	bv, err := LoadBlockedView(dir, "test-release", "S01-first")
	if err != nil {
		t.Fatalf("LoadBlockedView: %v", err)
	}

	// The path is DERIVED for the track that OWNS the slice via Slices membership
	// (T1-core-renamed), NOT the stale status.json.track ("T1-core"): the derived
	// path carries "T1-core-renamed", proving the owning track was resolved by
	// membership and never fell back to repoRoot (the silently-wrong-fallback AC-02 kills).
	wantWT := board.TrackWorktreePathFrom(board.ReleaseWorktreePathFrom(dir, "test-release"), "test-release", "T1-core-renamed")
	if bv.worktreePath != wantWT {
		t.Fatalf("expected derived worktreePath %q resolved via Slices membership despite stale status.json.track, got %q", wantWT, bv.worktreePath)
	}
	if !strings.Contains(bv.worktreePath, "T1-core-renamed") {
		t.Fatalf("worktreePath %q must derive from the owning track T1-core-renamed, not the stale track field T1-core", bv.worktreePath)
	}
	if bv.worktreePath == dir {
		t.Fatalf("worktreePath fell back to repoRoot %q — the exact silently-wrong-fallback AC-02 requires eliminating", dir)
	}
}

func TestBoardEnterTransitionsToBlocked(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	createIndex(t, dir, "test-release", "Test Release")

	writeBoardFixture(t, dir, "test-release", []board.BoardTrack{
		{ID: "T1-core", Slices: []string{"S01-first"}},
	})

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

	// Board loading is dispatched as a tea.Cmd (sworn#82) — drive it to
	// completion before pressing Enter again on the (now-populated) board.
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := upd.(*Model)
	if m2.state != viewBoard {
		t.Fatalf("expected viewBoard state, got %d", m2.state)
	}
	if cmd == nil {
		t.Fatal("expected a board-load Cmd after Enter")
	}
	upd, _ = m2.Update(cmd())
	m2 = upd.(*Model)

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

	createIndex(t, dir, "test-release", "Test Release")
	writeBoardFixture(t, dir, "test-release", []board.BoardTrack{
		{ID: "T1-core", Slices: []string{"S01-blocked"}},
	})

	// Create a slice at "implemented" with verification.result == "blocked"
	sliceDir := filepath.Join(releaseDir, "S01-blocked")
	os.MkdirAll(sliceDir, 0o755)
	st := &state.Status{
		SliceID: "S01-blocked",
		Release: filepath.Base(releaseDir), // required by validate-on-write
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

	// Enter to select release → board view. Board loading is dispatched as
	// a tea.Cmd (sworn#82) — drive it to completion before the next Enter.
	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := upd.(*Model)
	if m2.state != viewBoard {
		t.Fatalf("expected viewBoard state, got %d", m2.state)
	}
	if cmd == nil {
		t.Fatal("expected a board-load Cmd after Enter")
	}
	upd, _ = m2.Update(cmd())
	m2 = upd.(*Model)

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

	writeBoardFixture(t, dir, "test-release", []board.BoardTrack{
		{ID: "T1-core", Slices: []string{"S01-first"}},
	})

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

// TestLiveViewRendersMergeActorRow verifies that a live-status snapshot with
// a merge:<track> acquired event renders a distinct, highlighted merge row
// in the live concurrent-status view.
func TestLiveViewRendersMergeActorRow(t *testing.T) {
	dir := t.TempDir()

	// Create a sworn DB with a merge:T1-engine acquired event.
	dbPath := db.DefaultPath(dir)
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer conn.Close()

	_, err = conn.Exec(
		"INSERT INTO events (track_id, release, event, detail, ts) VALUES (?, ?, ?, ?, ?)",
		"merge:T1-engine", "test-release", "acquired", "PID 12345", "2026-06-28T12:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}

	lv, err := StartLiveView(dir, "test-release")
	if err != nil {
		t.Fatalf("StartLiveView: %v", err)
	}
	defer lv.Close()

	// Verify the merge row is present.
	var mergeRow *TrackRow
	for i := range lv.Rows {
		if lv.Rows[i].IsMerge && lv.Rows[i].ID == "merge:T1-engine" {
			mergeRow = &lv.Rows[i]
			break
		}
	}
	if mergeRow == nil {
		t.Fatalf("expected a merge:T1-engine row in lv.Rows, got: %+v", lv.Rows)
	}
	if mergeRow.State != "merging" {
		t.Errorf("expected merge row state 'merging', got %q", mergeRow.State)
	}

	// Verify the rendered view contains the merge row.
	view := lv.View()
	if !strings.Contains(view, "merge:T1-engine") {
		t.Errorf("expected view to contain 'merge:T1-engine', got:\n%s", view)
	}
}

// TestLiveViewNoMergeActorNoRow verifies that a snapshot with only
// worker/coordinator actors (no merge events) renders no merge row.
func TestLiveViewNoMergeActorNoRow(t *testing.T) {
	dir := t.TempDir()

	// Create a sworn DB with a regular track but no merge events.
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
	defer lv.Close()

	// No merge rows should be present.
	for _, row := range lv.Rows {
		if row.IsMerge {
			t.Errorf("expected no merge rows, found: %+v", row)
		}
	}

	// View should not contain "merge:".
	view := lv.View()
	if strings.Contains(view, "merge:") {
		t.Errorf("expected view to NOT contain 'merge:', got:\n%s", view)
	}
}

// TestLiveViewNoMergeActorAfterRelease verifies that a merge:<track> actor
// whose most-recent event is 'released-done' (not 'acquired') does NOT render
// a merge row. This is the critical test for the MAX(id) subquery pattern —
// a naive WHERE event='acquired' query would show stale completed merges.
func TestLiveViewNoMergeActorAfterRelease(t *testing.T) {
	dir := t.TempDir()

	dbPath := db.DefaultPath(dir)
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer conn.Close()

	// Insert acquired first, then released-done — the merge is complete.
	_, err = conn.Exec(
		"INSERT INTO events (track_id, release, event, detail, ts) VALUES (?, ?, ?, ?, ?)",
		"merge:T1-engine", "test-release", "acquired", "PID 12345", "2026-06-28T12:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert acquired event: %v", err)
	}
	_, err = conn.Exec(
		"INSERT INTO events (track_id, release, event, detail, ts) VALUES (?, ?, ?, ?, ?)",
		"merge:T1-engine", "test-release", "released-done", "PID 12345", "2026-06-28T12:05:00Z",
	)
	if err != nil {
		t.Fatalf("insert released-done event: %v", err)
	}

	lv, err := StartLiveView(dir, "test-release")
	if err != nil {
		t.Fatalf("StartLiveView: %v", err)
	}
	defer lv.Close()

	// No merge rows should be present — the latest event is released-done.
	for _, row := range lv.Rows {
		if row.IsMerge {
			t.Errorf("expected no merge rows after release, found: %+v", row)
		}
	}

	view := lv.View()
	if strings.Contains(view, "merge:T1-engine") {
		t.Errorf("expected view to NOT contain 'merge:T1-engine' after release, got:\n%s", view)
	}
}

// TestBoardViewShowsMergeBadge verifies that the board view renders a merge
// badge next to a track header when that track has an active merge in flight.
// This test sets up both a filesystem fixture (index.md + status.json) AND
// a SQLite DB with a merge:T1-core acquired event.
func TestBoardViewShowsMergeBadge(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeBoardFixture(t, dir, "test-release", []board.BoardTrack{
		{ID: "T1-core", Slices: []string{"S01-first"}},
	})
	createSliceStatus(t, releaseDir, "S01-first", "in_progress", "T1-core")

	// Create a sworn DB with a merge:T1-core acquired event.
	dbPath := db.DefaultPath(dir)
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer conn.Close()
	_, err = conn.Exec(
		"INSERT INTO events (track_id, release, event, detail, ts) VALUES (?, ?, ?, ?, ?)",
		"merge:T1-core", "test-release", "acquired", "PID 99999", "2026-06-28T12:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}

	bv := &BoardView{}
	if err := bv.LoadBoard(dir, "test-release"); err != nil {
		t.Fatalf("LoadBoard: %v", err)
	}

	if !bv.MergeActive["T1-core"] {
		t.Errorf("expected MergeActive[T1-core]=true, got: %v", bv.MergeActive)
	}

	view := bv.View()
	if !strings.Contains(view, "T1-core") {
		t.Errorf("expected view to contain 'T1-core', got:\n%s", view)
	}
	if !strings.Contains(view, "merge") {
		t.Errorf("expected view to contain merge badge 'merge', got:\n%s", view)
	}
}

// TestBoardViewNoMergeBadge verifies that the board view does NOT render a
// merge badge when no active merges exist.
func TestBoardViewNoMergeBadge(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeBoardFixture(t, dir, "test-release", []board.BoardTrack{
		{ID: "T1-core", Slices: []string{"S01-first"}},
	})
	createSliceStatus(t, releaseDir, "S01-first", "in_progress", "T1-core")

	// No DB created — no merge events.

	bv := &BoardView{}
	if err := bv.LoadBoard(dir, "test-release"); err != nil {
		t.Fatalf("LoadBoard: %v", err)
	}

	if len(bv.MergeActive) != 0 {
		t.Errorf("expected no active merges, got: %v", bv.MergeActive)
	}

	view := bv.View()
	if strings.Contains(view, "merge") {
		t.Errorf("expected view to NOT contain 'merge', got:\n%s", view)
	}
}

// TestBoardViewShowsDependsBadge verifies a track header shows a "needs: ..."
// badge derived from board.json's depends_on, and that a root track (no
// depends_on) shows no badge.
func TestBoardViewShowsDependsBadge(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	writeBoardFixture(t, dir, "test-release", []board.BoardTrack{
		{ID: "T1-contract", Slices: []string{"S01-first"}},
		{ID: "T2-subprocess", Slices: []string{"S02-second"}, DependsOn: []string{"T1-contract"}},
	})
	createSliceStatus(t, releaseDir, "S01-first", "verified", "T1-contract")
	createSliceStatus(t, releaseDir, "S02-second", "planned", "T2-subprocess")

	bv := &BoardView{}
	if err := bv.LoadBoard(dir, "test-release"); err != nil {
		t.Fatalf("LoadBoard: %v", err)
	}

	view := bv.View()
	if !strings.Contains(view, "needs: T1-contract") {
		t.Errorf("expected view to contain 'needs: T1-contract' badge, got:\n%s", view)
	}
	// T1-contract has no depends_on — its own header line must not claim to
	// need anything (checking the header specifically, not the whole view:
	// T2's "needs: T1-contract" badge legitimately contains the substring
	// "T1-contract" too).
	for l := range strings.SplitSeq(view, "\n") {
		if strings.Contains(l, "▸ T1-contract") && strings.Contains(l, "needs:") {
			t.Errorf("expected T1-contract header to have no 'needs:' badge, got line: %q", l)
		}
	}
}

// TestBoardViewToggleSortReordersTracksAndOrderedSlices verifies that 'o'
// (ToggleSort) switches the board from declaration order to dependency
// (topological) order, and that orderedSlices (which drives j/k cursor
// navigation) is rebuilt to match — otherwise cursor movement would desync
// from the visual layout the moment sort mode changes.
func TestBoardViewToggleSortReordersTracksAndOrderedSlices(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)

	// Declared out of dependency order: T2 (depends on T1) declared first.
	writeBoardFixture(t, dir, "test-release", []board.BoardTrack{
		{ID: "T2-subprocess", Slices: []string{"S02-second"}, DependsOn: []string{"T1-contract"}},
		{ID: "T1-contract", Slices: []string{"S01-first"}},
	})
	createSliceStatus(t, releaseDir, "S01-first", "verified", "T1-contract")
	createSliceStatus(t, releaseDir, "S02-second", "planned", "T2-subprocess")

	bv := &BoardView{}
	if err := bv.LoadBoard(dir, "test-release"); err != nil {
		t.Fatalf("LoadBoard: %v", err)
	}

	// Declaration order (default): T2 renders before T1, exactly as declared.
	view := bv.View()
	if strings.Index(view, "T2-subprocess") > strings.Index(view, "T1-contract") {
		t.Errorf("expected declaration order (T2 before T1), got:\n%s", view)
	}
	if got, want := bv.orderedSlices, []string{"S02-second", "S01-first"}; !slicesEqual(got, want) {
		t.Errorf("expected orderedSlices %v in declaration order, got %v", want, got)
	}

	bv.ToggleSort()

	// Dependency order: T1 (the dependency) must now render before T2.
	view = bv.View()
	if !strings.Contains(view, "sorted: dependency order") {
		t.Errorf("expected title to advertise dependency-order sort, got:\n%s", view)
	}
	if strings.Index(view, "T1-contract") > strings.Index(view, "T2-subprocess") {
		t.Errorf("expected dependency order (T1 before T2), got:\n%s", view)
	}
	if got, want := bv.orderedSlices, []string{"S01-first", "S02-second"}; !slicesEqual(got, want) {
		t.Errorf("expected orderedSlices %v in dependency order after ToggleSort, got %v", want, got)
	}

	bv.ToggleSort()
	if got, want := bv.orderedSlices, []string{"S02-second", "S01-first"}; !slicesEqual(got, want) {
		t.Errorf("expected ToggleSort to flip back to declaration order, got %v want %v", got, want)
	}
}

// TestBoardOKeyTogglesSort verifies pressing 'o' in the board view flips
// BoardView.SortMode via the same Model.Update dispatch path a real
// keypress takes (handleBoardKey), rather than only unit-testing ToggleSort
// in isolation.
func TestBoardOKeyTogglesSort(t *testing.T) {
	m := &Model{
		state: viewBoard,
		Board: &BoardView{Loaded: true, Tracks: []TrackInfo{{ID: "T1-a"}}},
	}

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m2 := upd.(*Model)
	if m2.state != viewBoard {
		t.Fatalf("expected to remain in viewBoard, got %d", m2.state)
	}
	if m2.Board.SortMode != trackSortDeps {
		t.Errorf("expected SortMode=%q after one 'o' press, got %q", trackSortDeps, m2.Board.SortMode)
	}

	upd, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m3 := upd.(*Model)
	if m3.Board.SortMode != "" {
		t.Errorf("expected SortMode=\"\" after second 'o' press, got %q", m3.Board.SortMode)
	}
}

// TestTopoSortTracksHandlesCycleAndDanglingRef verifies topoSortTracks never
// drops a track: a dependency cycle or a depends_on reference to a track ID
// absent from the board must still produce every track exactly once, rather
// than looping forever or silently omitting the unresolvable tracks.
func TestTopoSortTracksHandlesCycleAndDanglingRef(t *testing.T) {
	tracks := []TrackInfo{
		{ID: "T1-a", DependsOn: []string{"T2-b"}}, // cycle: T1 -> T2 -> T1
		{ID: "T2-b", DependsOn: []string{"T1-a"}},
		{ID: "T3-c", DependsOn: []string{"T99-ghost"}}, // dangling ref, no such track
	}
	got := topoSortTracks(tracks)
	if len(got) != len(tracks) {
		t.Fatalf("expected topoSortTracks to preserve all %d tracks, got %d: %v", len(tracks), len(got), got)
	}
	seen := map[string]bool{}
	for _, tr := range got {
		seen[tr.ID] = true
	}
	for _, tr := range tracks {
		if !seen[tr.ID] {
			t.Errorf("expected track %s to survive topoSortTracks, it was dropped", tr.ID)
		}
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestBoardViewDependencyOrderOnRealDriverContractRelease is the reachability
// test for the dependency-order sort feature: it loads THIS repo's real,
// committed 2026-06-28-driver-contract release (the exact release shown in
// the bug report screenshot) rather than a synthetic fixture, and verifies
// dependency badges render and that dependency-order sort places every track
// after all tracks it depends on — the real T1->T2/T3->T4->T5/T6->T7 chain
// recorded in that release's board.json.
func TestBoardViewDependencyOrderOnRealDriverContractRelease(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("skipping live-repo reachability test: findRepoRoot: %v", err)
	}
	const release = "2026-06-28-driver-contract"
	boardPath := filepath.Join(repoRoot, "docs", "release", release, "board.json")
	if _, err := os.Stat(boardPath); err != nil {
		t.Skipf("skipping live-repo reachability test: %s not found in this checkout: %v", boardPath, err)
	}

	bv := &BoardView{}
	if err := bv.LoadBoard(repoRoot, release); err != nil {
		t.Fatalf("LoadBoard against real repo release %q: %v", release, err)
	}
	if !bv.Loaded {
		t.Fatal("expected Loaded=true")
	}

	view := bv.View()
	t.Logf("declaration-order view:\n%s", view)
	for _, want := range []string{"needs: T1-contract", "needs: T2-subprocess, T3-inprocess"} {
		if !strings.Contains(view, want) {
			t.Errorf("expected declaration-order view to contain %q, got:\n%s", want, view)
		}
	}

	bv.ToggleSort()
	view = bv.View()
	t.Logf("dependency-order view:\n%s", view)
	pos := map[string]int{}
	for _, tr := range bv.displayTracks() {
		pos[tr.ID] = len(pos)
	}
	for id, deps := range map[string][]string{
		"T2-subprocess":      {"T1-contract"},
		"T3-inprocess":       {"T1-contract"},
		"T4-resolution-loop": {"T2-subprocess", "T3-inprocess"},
		"T7-baton-revendor":  {"T4-resolution-loop", "T5-catalog", "T6-proof"},
	} {
		for _, dep := range deps {
			if pos[dep] >= pos[id] {
				t.Errorf("dependency-order violated: %s (pos %d) must render before %s (pos %d)", dep, pos[dep], id, pos[id])
			}
		}
	}
}

// TestBoardViewLoadsRealOperationalReadinessRelease is the AC-05 reachability
// test: it drives the integration point that owns the affordance
// (BoardView.LoadBoard, called from Model.handleReleasesKey at the "enter"
// case) rooted at THIS repo's real, live checkout — not a synthetic
// t.TempDir() fixture — and loads the real, committed
// 2026-06-30-sworn-operational-readiness release (a genuine board.json-backed,
// 5-track release). This is the originally reported bug's exact repro:
// before this slice, BoardView.Tracks would silently come back empty for
// this release because sworn render no longer emits the `tracks:` frontmatter
// LoadBoard used to hand-parse.
func TestBoardViewLoadsRealOperationalReadinessRelease(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("skipping live-repo reachability test: findRepoRoot: %v", err)
	}
	const release = "2026-06-30-sworn-operational-readiness"
	boardPath := filepath.Join(repoRoot, "docs", "release", release, "board.json")
	if _, err := os.Stat(boardPath); err != nil {
		t.Skipf("skipping live-repo reachability test: %s not found in this checkout: %v", boardPath, err)
	}

	bv := &BoardView{}
	if err := bv.LoadBoard(repoRoot, release); err != nil {
		t.Fatalf("LoadBoard against real repo release %q: %v", release, err)
	}

	if !bv.Loaded {
		t.Fatal("expected Loaded=true")
	}

	wantTracks := []string{
		"T1-operational-unblock",
		"T2-board-render",
		"T3-consumer-repo-hygiene",
		"T4-board-record-reconciliation",
		"T5-model-pricing-registry",
	}
	if len(bv.Tracks) != len(wantTracks) {
		t.Fatalf("expected %d tracks for %s, got %d: %+v", len(wantTracks), release, len(bv.Tracks), bv.Tracks)
	}
	got := map[string]bool{}
	for _, tr := range bv.Tracks {
		got[tr.ID] = true
	}
	for _, id := range wantTracks {
		if !got[id] {
			t.Errorf("expected track %q to be present, got tracks: %+v", id, bv.Tracks)
		}
	}

	// Every track must have at least one slice with a non-"unknown" state —
	// proving live status.json data was actually read, not just track shells.
	for _, tr := range bv.Tracks {
		for _, sliceID := range tr.Slices {
			si, ok := bv.Slices[sliceID]
			if !ok || si.State == "" || si.State == "unknown" {
				t.Errorf("track %s slice %s: expected a real state, got %+v (ok=%v)", tr.ID, sliceID, si, ok)
			}
		}
	}
}

// --- S03-tui-chrome-rework: responsive chrome (header, pane widths, help bar) ---

// newChromeModel builds a two-pane Model with the given release names in the
// left list and an empty board, for the S03 chrome tests.
func newChromeModel(names ...string) *Model {
	rl := &ReleasesList{}
	for _, n := range names {
		rl.Releases = append(rl.Releases, ReleaseInfo{
			Name:        n,
			TrackCount:  2,
			SliceStates: map[string]int{"planned": 1},
		})
	}
	return &Model{
		state:    viewReleases,
		Releases: rl,
		Board:    &BoardView{},
	}
}

// TestWindowSizeMsgStoresDimensions — AC-01: a tea.WindowSizeMsg is no longer
// discarded; the reported width AND height are stored on the Model.
func TestWindowSizeMsgStoresDimensions(t *testing.T) {
	m := newChromeModel("rel-a")
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m2 := upd.(*Model)
	if m2.Width != 120 || m2.Height != 40 {
		t.Fatalf("expected Width=120 Height=40 stored from WindowSizeMsg, got Width=%d Height=%d", m2.Width, m2.Height)
	}
}

// TestPaneWidthsReserveBorderColumns — AC-01 + review pin 2: paneWidths must
// reserve the 4 rounded-border columns (2 per pane; JoinHorizontal adds no
// gap, verified live against lipgloss v1.1.0) so left+right+4 <= total. Also
// asserts the legacy (30,80) fallback for the pre-WindowSizeMsg case.
func TestPaneWidthsReserveBorderColumns(t *testing.T) {
	if l, r := paneWidths(0); l != 30 || r != 80 {
		t.Fatalf("paneWidths(0) legacy fallback: expected (30,80), got (%d,%d)", l, r)
	}
	for _, total := range []int{80, 100, 120, 220} {
		l, r := paneWidths(total)
		if l <= 0 || r <= 0 {
			t.Fatalf("paneWidths(%d): expected positive pane widths, got (%d,%d)", total, l, r)
		}
		if l+r+4 > total {
			t.Fatalf("paneWidths(%d): left+right+4=%d exceeds total %d — 4 border columns not reserved", total, l+r+4, total)
		}
	}
}

// TestPaneWidthsLeftFloor — AC-02 (Coach decision, option b): the left pane
// gets a minimum-width floor so it stays legible at an 80-col terminal
// instead of being squeezed to near-nothing by a pure proportional split.
func TestPaneWidthsLeftFloor(t *testing.T) {
	left, _ := paneWidths(80)
	if left < minLeftPane {
		t.Fatalf("paneWidths(80): left pane %d is below the minimum-width floor %d", left, minLeftPane)
	}
}

// TestTwoPaneRenderFitsTerminalWidth — AC-01/AC-05 + review pin 2: the full
// rendered frame (header + two panes + help bar) never renders a line wider
// than the reported terminal width. A frame wider than the terminal forces
// the emulator to line-wrap, which is the spec-identified root cause of the
// VS Code integrated-terminal viewport bug (AC-05).
func TestTwoPaneRenderFitsTerminalWidth(t *testing.T) {
	m := newChromeModel("render-drift-reconciliation")
	m.Version = "1.0.0"
	for _, w := range []int{80, 100, 120, 220} {
		upd, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 40})
		m = upd.(*Model)
		view := m.View()
		if got := lipgloss.Width(view); got > w {
			t.Fatalf("at terminal width %d, rendered frame max line width is %d (> %d) — would force emulator wrap (AC-05 root cause)", w, got, w)
		}
	}
}

// TestReleasesListShowsBareIDNotFrontmatterTitle verifies the releases pane
// displays the bare release directory name (e.g. "2026-06-28-driver-contract"),
// not render.go's generated frontmatter title ("Release board — <release>").
// Every index.md's title carries that constant prefix, so displaying it
// verbatim meant every entry in the pane redundantly repeated "Release board —".
func TestReleasesListShowsBareIDNotFrontmatterTitle(t *testing.T) {
	dir := t.TempDir()
	fixtureReleaseDir(filepath.Join(dir, "docs", "release"))
	createIndex(t, dir, "2026-06-28-driver-contract", "Release board — 2026-06-28-driver-contract")

	rl := &ReleasesList{}
	if err := rl.LoadReleases(dir); err != nil {
		t.Fatalf("LoadReleases: %v", err)
	}

	out := rl.View()
	if !strings.Contains(out, "2026-06-28-driver-contract") {
		t.Fatalf("expected the bare release ID in the pane, got:\n%s", out)
	}
	if strings.Contains(out, "Release board") {
		t.Fatalf("expected no 'Release board —' prefix in the pane, got:\n%s", out)
	}
}

// TestReleasesListRealRepoReleasesShowBareID is the reachability test for the
// bare-ID fix: it loads THIS repo's real, live docs/release/ directory (every
// release actually committed here, each with a render.go-generated
// "Release board — <release>" frontmatter title) and verifies the pane shows
// none of that prefix for any of them.
func TestReleasesListRealRepoReleasesShowBareID(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("skipping live-repo reachability test: findRepoRoot: %v", err)
	}

	rl := &ReleasesList{}
	if err := rl.LoadReleases(repoRoot); err != nil {
		t.Fatalf("LoadReleases against real repo: %v", err)
	}
	if len(rl.Releases) == 0 {
		t.Fatal("expected at least one real release under docs/release/")
	}

	out := rl.View()
	t.Logf("releases pane:\n%s", out)
	if strings.Contains(out, "Release board") {
		t.Errorf("expected no 'Release board —' prefix in the real releases pane, got:\n%s", out)
	}
	for _, rel := range rl.Releases {
		if !strings.Contains(out, rel.ID) {
			t.Errorf("expected bare release ID %q to appear in the pane, got:\n%s", rel.ID, out)
		}
	}
}

// TestReleasesListNoWrapAtTypicalWidth — AC-02: a release name under 40 chars
// with a comfortably wide pane renders on exactly one line, untruncated.
func TestReleasesListNoWrapAtTypicalWidth(t *testing.T) {
	rl := &ReleasesList{
		Width: 100,
		Releases: []ReleaseInfo{
			{ID: "loop-cli-ux", Name: "Release board — loop-cli-ux", TrackCount: 3, SliceStates: map[string]int{"verified": 3}},
		},
	}
	out := rl.View()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (title + 1 release, no wrap) at pane width 100, got %d:\n%q", len(lines), out)
	}
	if strings.Contains(out, "…") {
		t.Fatalf("a <40-char release name at pane width 100 must not be truncated, got:\n%q", out)
	}
	if !strings.Contains(out, "loop-cli-ux") {
		t.Fatalf("expected the full release name present, got:\n%q", out)
	}
}

// TestReleasesListTruncatesLongNameAtNarrowPane — AC-02 (Coach decision): at
// an 80-col terminal (left pane ~28 cols) a long release name is truncated
// with an ellipsis on a single line, NOT wrapped illegibly across lines.
func TestReleasesListTruncatesLongNameAtNarrowPane(t *testing.T) {
	longName := "an-extremely-long-release-name-that-would-wrap-illegibly-at-eighty-cols"
	rl := &ReleasesList{
		Width: 28,
		Releases: []ReleaseInfo{
			{ID: longName, Name: "Release board — " + longName, TrackCount: 2, SliceStates: map[string]int{"planned": 1}},
		},
	}
	out := rl.View()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (title + 1 truncated release, no wrap) at pane width 28, got %d:\n%q", len(lines), out)
	}
	if !strings.Contains(out, "…") {
		t.Fatalf("a long release name at pane width 28 must be ellipsis-truncated, got:\n%q", out)
	}
	if strings.Contains(out, longName) {
		t.Fatalf("the full untruncated long name must not appear at pane width 28, got:\n%q", out)
	}
	for _, ln := range lines {
		if w := lipgloss.Width(ln); w > 28 {
			t.Fatalf("rendered line %q has width %d, exceeding pane width 28 (would wrap)", ln, w)
		}
	}
}

// TestHeaderShowsVersionAndNoReleaseSelected — AC-03: on the initial releases
// screen (never navigated into a release) the header shows the version and an
// explicit "no release selected" label.
func TestHeaderShowsVersionAndNoReleaseSelected(t *testing.T) {
	m := newChromeModel("rel-a")
	m.Version = "1.2.3"
	m.Width = 100
	m.state = viewReleases
	m.Board.ReleaseName = ""
	header := m.renderHeader()
	if !strings.Contains(header, "1.2.3") {
		t.Fatalf("header should show version 1.2.3, got:\n%q", header)
	}
	if !strings.Contains(header, "no release selected") {
		t.Fatalf("header on the initial releases screen should show 'no release selected', got:\n%q", header)
	}
}

// TestHeaderShowsSelectedRelease — AC-03: once the user has navigated into a
// release, the header shows the version and the selected release name.
func TestHeaderShowsSelectedRelease(t *testing.T) {
	m := newChromeModel("rel-a")
	m.Version = "9.9.9"
	m.Width = 100
	m.state = viewBoard
	m.Board.ReleaseName = "2026-07-01-render-drift-reconciliation"
	header := m.renderHeader()
	if !strings.Contains(header, "9.9.9") {
		t.Fatalf("header should show the version, got:\n%q", header)
	}
	if !strings.Contains(header, "2026-07-01-render-drift-reconciliation") {
		t.Fatalf("header should show the selected release name, got:\n%q", header)
	}
}

// TestViewRendersHeader — AC-03 through the integration point (Rule 1): the
// header is actually rendered by Model.View, not only by renderHeader in
// isolation.
func TestViewRendersHeader(t *testing.T) {
	m := newChromeModel("rel-a")
	m.Version = "2.0.0"
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = upd.(*Model)
	view := m.View()
	if !strings.Contains(view, "2.0.0") {
		t.Fatalf("Model.View should render the header with version 2.0.0, got:\n%q", view[:min(300, len(view))])
	}
	if !strings.Contains(view, "no release selected") {
		t.Fatalf("Model.View on the initial screen should render 'no release selected' in the header, got:\n%q", view[:min(300, len(view))])
	}
}

// TestHelpBarSpansFullWidth — AC-04: the help bar spans the full terminal
// width (background-styled bar), and falls back to the legacy 110 when no
// WindowSizeMsg has been received yet.
func TestHelpBarSpansFullWidth(t *testing.T) {
	m := newChromeModel("rel-a")
	m.Width = 137
	if got := lipgloss.Width(m.renderHelp()); got != 137 {
		t.Fatalf("help bar should span the full terminal width 137, got %d", got)
	}
	m.Width = 0
	if got := lipgloss.Width(m.renderHelp()); got != 110 {
		t.Fatalf("help bar fallback width should be the legacy 110 when Width is unset, got %d", got)
	}
}

// --- sworn#82: async board load + lazy on-demand gate results ---

// TestEnterDispatchesAsyncBoardLoad is the reachability test for the fix:
// pressing Enter on the releases list must return a tea.Cmd instead of
// running BoardView.LoadBoard inline in the key handler. Before the fix,
// LoadBoard (and the gates it re-ran on every call) executed synchronously
// inside handleReleasesKey, so Board.Loaded flipped to true in the SAME
// Update() call that handled the keypress — bubbletea never got a chance to
// repaint a loading indicator in between, which is the reported freeze.
func TestEnterDispatchesAsyncBoardLoad(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)
	createIndex(t, dir, "test-release", "Test Release")
	writeBoardFixture(t, dir, "test-release", []board.BoardTrack{
		{ID: "T1-core", Slices: []string{"S01-first"}},
	})
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

	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := upd.(*Model)

	// The key handler must return immediately — no inline LoadBoard.
	if m2.state != viewBoard {
		t.Fatalf("expected viewBoard state right after Enter, got %d", m2.state)
	}
	if m2.Board.Loaded {
		t.Fatal("expected Board.Loaded=false immediately after Enter — LoadBoard must not run inline in the key handler (this is the sworn#82 freeze)")
	}
	if cmd == nil {
		t.Fatal("expected Enter to return a non-nil tea.Cmd to load the board asynchronously")
	}

	// Executing the returned Cmd (as the bubbletea runtime would, off the
	// Update goroutine) and feeding the resulting msg back through Update is
	// what actually populates the board.
	msg := cmd()
	upd2, _ := m2.Update(msg)
	m3 := upd2.(*Model)
	if !m3.Board.Loaded {
		t.Fatal("expected Board.Loaded=true after the async load msg is delivered")
	}
	if len(m3.Board.Tracks) != 1 {
		t.Fatalf("expected 1 track after async load, got %d", len(m3.Board.Tracks))
	}
	checkSlice(t, m3.Board, "S01-first", "verified")
}

// TestLoadBoardDoesNotComputeGatesEagerly proves the lazy-gates half of
// sworn#82: LoadBoard must NOT call LoadGateResults (trace + per-slice
// coverage/design/mock, each shelling git diff) as part of every board load.
// Before the fix this was ~100% of the measured cost (up to 21.3s of a
// 21.5s load on a 73-slice release).
func TestLoadBoardDoesNotComputeGatesEagerly(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)
	writeBoardFixture(t, dir, "test-release", []board.BoardTrack{
		{ID: "T1-core", Slices: []string{"S01-first"}},
	})
	createSliceStatus(t, releaseDir, "S01-first", "verified", "T1-core")

	bv := &BoardView{}
	if err := bv.LoadBoard(dir, "test-release"); err != nil {
		t.Fatalf("LoadBoard: %v", err)
	}

	if bv.GatesLoaded {
		t.Fatal("expected GatesLoaded=false after LoadBoard — gates must be lazy, not computed eagerly")
	}
	if len(bv.GateResults) != 0 {
		t.Fatalf("expected no GateResults populated by LoadBoard, got %d entries", len(bv.GateResults))
	}
}

// TestGateKeyTriggersAsyncGateLoad verifies the on-demand path: pressing 'g'
// in board view dispatches a tea.Cmd (not an inline call) that computes gate
// results, and delivering the resulting msg populates BoardView.GateResults
// and flips GatesLoaded.
func TestGateKeyTriggersAsyncGateLoad(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "docs", "release", "test-release")
	os.MkdirAll(releaseDir, 0o755)
	writeBoardFixture(t, dir, "test-release", []board.BoardTrack{
		{ID: "T1-core", Slices: []string{"S01-first"}},
	})
	createSliceStatus(t, releaseDir, "S01-first", "verified", "T1-core")

	bv := &BoardView{}
	if err := bv.LoadBoard(dir, "test-release"); err != nil {
		t.Fatalf("LoadBoard: %v", err)
	}

	m := &Model{
		state:    viewBoard,
		repoRoot: dir,
		Releases: &ReleasesList{},
		Board:    bv,
	}

	upd, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	m2 := upd.(*Model)
	if !m2.Board.GatesLoading {
		t.Fatal("expected GatesLoading=true right after pressing 'g'")
	}
	if cmd == nil {
		t.Fatal("expected 'g' to return a non-nil tea.Cmd to compute gates asynchronously")
	}

	msg := cmd()
	upd2, _ := m2.Update(msg)
	m3 := upd2.(*Model)
	if m3.Board.GatesLoading {
		t.Fatal("expected GatesLoading=false after the gates-loaded msg is delivered")
	}
	if !m3.Board.GatesLoaded {
		t.Fatal("expected GatesLoaded=true after the gates-loaded msg is delivered")
	}
	if _, ok := m3.Board.GateResults["S01-first"]; !ok {
		t.Fatalf("expected GateResults to contain S01-first, got %+v", m3.Board.GateResults)
	}
}

// TestCatalogRefreshStartsFromModelInit proves the automatic refresh chain is
// reachable from Bubble Tea's root integration point.
func TestCatalogRefreshStartsFromModelInit(t *testing.T) {
	m := refreshModel(t, refreshCatalogRecord("alpha", "S01-old"))
	cmd := m.Init()
	if cmd == nil || m.refreshGeneration != 1 || m.refreshInFlight {
		t.Fatalf("Init did not arm generation 1: cmd=%v generation=%d inFlight=%v", cmd != nil, m.refreshGeneration, m.refreshInFlight)
	}
}

// TestCatalogRefreshAtomicallyUpdatesListAndSelectedBoard is the root-model
// reachability test for AC-01: one accepted result owns both replacements and
// preserves presentation-only state without consulting a secondary resolver.
func TestCatalogRefreshAtomicallyUpdatesListAndSelectedBoard(t *testing.T) {
	initial := refreshCatalogRecord("alpha", "S01-old")
	m := refreshModel(t, initial)
	m.Board.MergeActive["T1-core"] = true
	m.Board.GatesLoaded = true
	m.Board.GateResults["S01-old"] = GateResult{TraceVerdict: "PASS"}
	old := m.Board.Slices["S01-old"]
	old.Gate = m.Board.GateResults["S01-old"]
	m.Board.Slices["S01-old"] = old
	m.repoRoot = filepath.Join(t.TempDir(), "must-not-be-read")
	m.refreshGeneration = 1
	m.refreshInFlight = true

	updated := refreshCatalogRecord("alpha", "S01-old", "S02-new")
	next, cmd := m.Update(catalogRefreshResultMsg{generation: 1, catalog: []board.CatalogRecord{updated}})
	m = next.(*Model)

	if cmd == nil {
		t.Fatal("accepted refresh must schedule exactly one next refresh command")
	}
	if len(m.Releases.Releases) != 1 || m.Releases.Releases[0].Catalog.Board.Tracks[0].Slices[1].ID != "S02-new" {
		t.Fatalf("releases snapshot did not advance atomically: %+v", m.Releases.Releases)
	}
	if _, ok := m.Board.Slices["S02-new"]; !ok {
		t.Fatalf("selected board did not advance from the same snapshot: %+v", m.Board.Slices)
	}
	if !m.Board.MergeActive["T1-core"] || !m.Board.GatesLoaded || m.Board.Slices["S01-old"].Gate.TraceVerdict != "PASS" {
		t.Fatalf("presentation state was not preserved: board=%+v", m.Board)
	}
	if m.refreshGeneration != 2 || m.refreshInFlight {
		t.Fatalf("refresh lifecycle = generation %d inFlight %v", m.refreshGeneration, m.refreshInFlight)
	}
}

func TestCatalogRefreshRearmsOnlyAfterCompletion(t *testing.T) {
	calls := 0
	m := refreshModel(t, refreshCatalogRecord("alpha", "S01-old"))
	m.refreshGeneration = 1
	m.discoverCatalog = func(string) ([]board.CatalogRecord, error) {
		calls++
		return []board.CatalogRecord{refreshCatalogRecord("alpha", "S01-old", "S02-new")}, nil
	}

	updated, discoverCmd := m.Update(catalogRefreshDueMsg{generation: 1})
	m = updated.(*Model)
	if discoverCmd == nil || !m.refreshInFlight || calls != 0 {
		t.Fatalf("due handling started incorrectly: cmd=%v inFlight=%v calls=%d", discoverCmd != nil, m.refreshInFlight, calls)
	}
	updated, duplicateCmd := m.Update(catalogRefreshDueMsg{generation: 1})
	m = updated.(*Model)
	if duplicateCmd != nil || calls != 0 {
		t.Fatalf("duplicate due overlapped discovery: cmd=%v calls=%d", duplicateCmd != nil, calls)
	}

	result := discoverCmd()
	if calls != 1 {
		t.Fatalf("discovery calls=%d, want exactly one", calls)
	}
	updated, scheduleCmd := m.Update(result)
	m = updated.(*Model)
	if scheduleCmd == nil || m.refreshGeneration != 2 || m.refreshInFlight {
		t.Fatalf("completion did not re-arm serial chain: cmd=%v generation=%d inFlight=%v", scheduleCmd != nil, m.refreshGeneration, m.refreshInFlight)
	}
}

func TestCatalogRefreshNeverOverlapsAndRejectsStaleGeneration(t *testing.T) {
	m := refreshModel(t, refreshCatalogRecord("alpha", "S01-old"))
	m.refreshGeneration = 2
	m.refreshInFlight = true
	m.refreshErr = "newest visible error"
	beforeList := m.Releases.View()
	beforeBoard := m.Board.View()

	updated, cmd := m.Update(catalogRefreshResultMsg{
		generation: 1,
		catalog:    []board.CatalogRecord{refreshCatalogRecord("alpha", "S99-stale")},
	})
	m = updated.(*Model)
	if cmd != nil || m.Releases.View() != beforeList || m.Board.View() != beforeBoard || m.refreshErr != "newest visible error" {
		t.Fatalf("stale result changed visible state: cmd=%v error=%q", cmd != nil, m.refreshErr)
	}
	updated, cmd = m.Update(catalogRefreshDueMsg{generation: 2})
	m = updated.(*Model)
	if cmd != nil || !m.refreshInFlight {
		t.Fatal("an in-flight generation accepted an overlapping due message")
	}
}

func TestCatalogRefreshPreservesSelectionByIdentity(t *testing.T) {
	alpha := refreshCatalogRecord("alpha", "S01-alpha")
	zeta := refreshCatalogRecord("zeta", "S01-old", "S02-selected")
	m := refreshModel(t, alpha, zeta)
	m.Releases.Cursor = 1
	bv, err := boardViewFromCatalog(zeta)
	if err != nil {
		t.Fatal(err)
	}
	bv.Cursor = 1
	m.Board = bv
	m.state = viewBoard
	m.refreshGeneration = 1
	m.refreshInFlight = true

	aardvark := refreshCatalogRecord("aardvark", "S01-new-release")
	zetaReordered := refreshCatalogRecord("zeta", "S02-selected", "S01-old", "S03-new")
	updated, _ := m.Update(catalogRefreshResultMsg{generation: 1, catalog: []board.CatalogRecord{zetaReordered, alpha, aardvark}})
	m = updated.(*Model)
	if got := m.Releases.Releases[m.Releases.Cursor].ID; got != "zeta" {
		t.Fatalf("release selection moved to %q, want zeta", got)
	}
	if got := m.Board.orderedSlices[m.Board.Cursor]; got != "S02-selected" {
		t.Fatalf("slice selection moved to %q, want S02-selected", got)
	}
}

func TestCatalogRefreshHandlesRemovedSelection(t *testing.T) {
	alpha := refreshCatalogRecord("alpha", "S01-alpha")
	zeta := refreshCatalogRecord("zeta", "S01-zeta")
	m := refreshModel(t, alpha, zeta)
	m.Releases.Cursor = 1
	m.Board, _ = boardViewFromCatalog(zeta)
	m.state = viewBoard
	m.refreshGeneration = 1
	m.refreshInFlight = true

	updated, _ := m.Update(catalogRefreshResultMsg{generation: 1, catalog: []board.CatalogRecord{alpha}})
	m = updated.(*Model)
	if m.state != viewReleases || m.Board.ReleaseName != "" || m.Releases.Cursor != 0 {
		t.Fatalf("removed selection not cleared/clamped: state=%d board=%+v cursor=%d", m.state, m.Board, m.Releases.Cursor)
	}
}

func TestCatalogRefreshHandlesEmptySuccessfulCatalog(t *testing.T) {
	m := refreshModel(t, refreshCatalogRecord("alpha", "S01-old"))
	m.refreshGeneration = 1
	m.refreshInFlight = true

	updated, cmd := m.Update(catalogRefreshResultMsg{generation: 1, catalog: []board.CatalogRecord{}})
	m = updated.(*Model)
	if cmd == nil || len(m.Releases.Releases) != 0 || m.Releases.Cursor != 0 {
		t.Fatalf("empty catalog was not installed safely: cmd=%v releases=%+v cursor=%d", cmd != nil, m.Releases.Releases, m.Releases.Cursor)
	}
	if m.Board.ReleaseName != "" || m.state != viewReleases || m.refreshErr != ErrNoReleases.Error() {
		t.Fatalf("empty catalog semantics: board=%+v state=%d error=%q", m.Board, m.state, m.refreshErr)
	}
}

func TestCatalogRefreshErrorRetainsLastGoodAndRecovers(t *testing.T) {
	initial := refreshCatalogRecord("alpha", "S01-old")
	m := refreshModel(t, initial)
	m.state = viewLog
	m.Log = StartLogView(t.TempDir(), "alpha", "", viewBoard, 10)
	m.errMsg = "unrelated operator error"
	m.refreshGeneration = 1
	m.refreshInFlight = true
	beforeList := m.Releases.View()
	beforeBoard := m.Board.View()

	updated, retryCmd := m.Update(catalogRefreshResultMsg{generation: 1, err: errors.New("catalog unavailable")})
	m = updated.(*Model)
	if retryCmd == nil || m.Releases.View() != beforeList || m.Board.View() != beforeBoard {
		t.Fatal("refresh error did not retain the complete last-good snapshot and re-arm")
	}
	view := m.View()
	if !strings.Contains(view, "Error: catalog unavailable") || !strings.Contains(view, "Error: unrelated operator error") {
		t.Fatalf("alternate root view did not render both errors:\n%s", view)
	}

	m.refreshInFlight = true
	recovered := refreshCatalogRecord("alpha", "S01-old", "S02-new")
	updated, _ = m.Update(catalogRefreshResultMsg{generation: 2, catalog: []board.CatalogRecord{recovered}})
	m = updated.(*Model)
	if m.refreshErr != "" || m.errMsg != "unrelated operator error" {
		t.Fatalf("recovery cleared wrong error: refresh=%q root=%q", m.refreshErr, m.errMsg)
	}
	if _, ok := m.Board.Slices["S02-new"]; !ok {
		t.Fatal("recovery did not install the newer snapshot")
	}
}

func TestCatalogRefreshErrorVisibleInEveryRootView(t *testing.T) {
	base := refreshModel(t, refreshCatalogRecord("alpha", "S01-old"))
	base.refreshErr = "refresh failed"
	base.Live = &LiveView{ReleaseName: "alpha"}
	base.Log = StartLogView(t.TempDir(), "alpha", "", viewBoard, 10)
	base.Blocked = &BlockedView{}
	base.Settings = &SettingsView{fields: make([]settingsField, 4)}

	for _, state := range []viewState{viewReleases, viewBoard, viewLive, viewLog, viewBlocked, viewSettings} {
		base.state = state
		if got := base.View(); !strings.Contains(got, "Error: refresh failed") {
			t.Errorf("state %d hid root refresh error:\n%s", state, got)
		}
	}
}

func TestCatalogRefreshCoexistsWithLiveAndLogTicks(t *testing.T) {
	dir := t.TempDir()
	conn, err := db.Open(db.DefaultPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(
		"INSERT INTO tracks (id, release, state, current_slice, started_at) VALUES (?, ?, ?, ?, ?)",
		"T1-core", "alpha", "in_progress", "S01-old", "2026-07-18T00:00:00Z",
	); err != nil {
		t.Fatal(err)
	}
	conn.Close()

	live, err := StartLiveView(dir, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	m := refreshModel(t, refreshCatalogRecord("alpha", "S01-old"))
	m.repoRoot = dir
	m.refreshGeneration = 7
	m.state = viewLive
	m.Live = live
	beforeTicks := live.TickCount
	updated, liveCmd := m.Update(tickMsg{})
	m = updated.(*Model)
	if liveCmd == nil || m.Live.TickCount <= beforeTicks || m.refreshGeneration != 7 || m.refreshInFlight {
		t.Fatal("catalog scheduling interfered with the live tick chain")
	}

	m.state = viewLog
	m.Log = StartLogView(dir, "alpha", "", viewLive, 10)
	updated, logCmd := m.Update(logTickMsg{})
	m = updated.(*Model)
	if logCmd == nil || m.refreshGeneration != 7 || m.refreshInFlight {
		t.Fatal("catalog scheduling interfered with the log tick chain")
	}
	updated, refreshCmd := m.Update(catalogRefreshDueMsg{generation: 7})
	m = updated.(*Model)
	if refreshCmd == nil || !m.refreshInFlight {
		t.Fatal("catalog refresh chain did not remain active in log view")
	}
}

func TestCatalogRefreshRenderedFrames(t *testing.T) {
	m := refreshModel(t, refreshCatalogRecord("alpha", "S01-old"))
	m.Version = "test"
	m.Width = 80
	before := normalizeTerminalFrame(m.View())
	m.refreshGeneration = 1
	m.refreshInFlight = true
	updated, _ := m.Update(catalogRefreshResultMsg{
		generation: 1,
		catalog:    []board.CatalogRecord{refreshCatalogRecord("alpha", "S01-old", "S02-new")},
	})
	m = updated.(*Model)
	after := normalizeTerminalFrame(m.View())
	m.refreshGeneration = 2
	m.refreshInFlight = true
	updated, _ = m.Update(catalogRefreshResultMsg{generation: 2, err: errors.New("catalog unavailable")})
	m = updated.(*Model)
	errorFrame := normalizeTerminalFrame(m.View())

	goldenDir := filepath.Join("..", "..", "docs", "release", "2026-07-17-ref-aware-board-discovery", "screenshots", "S03-tui-live-board-refresh")
	frames := []struct {
		name string
		text string
	}{
		{name: "before.txt", text: before},
		{name: "after.txt", text: after},
		{name: "error.txt", text: errorFrame},
	}
	for _, frame := range frames {
		want, err := os.ReadFile(filepath.Join(goldenDir, frame.name))
		if err != nil {
			t.Errorf("read %s: %v; generated frame: %q", frame.name, err, frame.text)
			continue
		}
		wantFrame := strings.TrimSuffix(string(want), "\n")
		if frame.text != wantFrame {
			t.Errorf("%s differs from committed terminal frame\n--- want ---\n%s\n--- got ---\n%s", frame.name, want, frame.text)
		}
	}
}

func normalizeTerminalFrame(frame string) string {
	lines := strings.Split(frame, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	return strings.Join(lines, "\n")
}

func refreshModel(t *testing.T, catalog ...board.CatalogRecord) *Model {
	t.Helper()
	releases, err := releaseInfosFromCatalog(catalog)
	if err != nil {
		t.Fatal(err)
	}
	bv, err := boardViewFromCatalog(catalog[0])
	if err != nil {
		t.Fatal(err)
	}
	return &Model{
		state:    viewBoard,
		Releases: &ReleasesList{Releases: releases},
		Board:    bv,
	}
}

func refreshCatalogRecord(release string, sliceIDs ...string) board.CatalogRecord {
	slices := make([]board.SliceState, 0, len(sliceIDs))
	for _, id := range sliceIDs {
		slices = append(slices, board.SliceState{
			ID:              id,
			Track:           "T1-core",
			State:           "verified",
			StateSource:     "refs/heads/track/" + release + "/T1-core",
			StateDurability: "committed",
		})
	}
	return board.CatalogRecord{
		Release:   release,
		SourceRef: "refs/heads/release-wt/" + release,
		Board: &board.BoardState{Release: release, Tracks: []board.TrackState{{
			ID:     "T1-core",
			State:  "verified",
			Slices: slices,
		}}},
	}
}
