package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/swornagent/sworn/internal/tracklog"
)

// logDirFor mirrors StartLogView's own path computation so fixtures land where
// the reader looks: <repoRoot>/.sworn/logs/<release>.
func logDirFor(repoRoot, release string) string {
	return filepath.Join(repoRoot, ".sworn", "logs", release)
}

// writeLog writes a log fixture in the on-disk format the engine tee produces:
// a "# sworn-log v1" header followed by timestamp-prefixed lines. It overwrites
// (used both to seed and to simulate more lines having streamed in).
func writeLog(t *testing.T, repoRoot, release, track string, lines []string) {
	t.Helper()
	dir := logDirFor(repoRoot, release)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	content := tracklog.FormatHeader + "\n" + strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, tracklog.LogFileName(track)), []byte(content), 0o644); err != nil {
		t.Fatalf("write log fixture: %v", err)
	}
}

func runeKey(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func liveModel(repoRoot, release string, rows []TrackRow, height int) *Model {
	return &Model{
		state:    viewLive,
		repoRoot: repoRoot,
		Height:   height,
		Board:    &BoardView{ReleaseName: release},
		Live:     &LiveView{ReleaseName: release, Rows: rows},
	}
}

// TestLiveEnterOpensTrackLog — the dive-into-a-worker journey: from the live
// table, enter on the cursor row opens that track's log through the root Model.
func TestLiveEnterOpensTrackLog(t *testing.T) {
	repo := t.TempDir()
	writeLog(t, repo, "rel", "T1", []string{"2026-07-03T10:00:00.000 [T1] router: S01 → implement"})

	m := liveModel(repo, "rel", []TrackRow{{ID: "T1"}}, 20)

	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := upd.(*Model)

	if m2.state != viewLog {
		t.Fatalf("expected viewLog after enter, got %d", m2.state)
	}
	if m2.Log == nil || m2.Log.Track != "T1" {
		t.Fatalf("expected Log.Track == T1, got %+v", m2.Log)
	}
	if !strings.Contains(m2.View(), "router: S01 → implement") {
		t.Errorf("rendered view missing seeded T1 line:\n%s", m2.View())
	}
}

// TestConsolidatedLogInterleaves — L opens the k-way merge of every track's log
// in global timestamp order across files.
func TestConsolidatedLogInterleaves(t *testing.T) {
	repo := t.TempDir()
	writeLog(t, repo, "rel", "T1", []string{
		"2026-07-03T10:00:00.000 [T1] alpha",
		"2026-07-03T10:00:02.000 [T1] gamma",
	})
	writeLog(t, repo, "rel", "T3", []string{
		"2026-07-03T10:00:01.000 [T3] beta",
		"2026-07-03T10:00:03.000 [T3] delta",
	})

	// Enter via the board's L entry point.
	m := &Model{state: viewBoard, repoRoot: repo, Height: 50, Board: &BoardView{ReleaseName: "rel"}}
	upd, _ := m.Update(runeKey('L'))
	m2 := upd.(*Model)

	if m2.state != viewLog || m2.Log == nil || m2.Log.Track != "" {
		t.Fatalf("expected consolidated viewLog, got state=%d log=%+v", m2.state, m2.Log)
	}

	want := []string{"alpha", "beta", "gamma", "delta"}
	var idx []int
	for _, tok := range want {
		found := -1
		for i, line := range m2.Log.Lines {
			if strings.Contains(line, tok) {
				found = i
				break
			}
		}
		if found < 0 {
			t.Fatalf("consolidated view missing %q: %v", tok, m2.Log.Lines)
		}
		idx = append(idx, found)
	}
	for i := 1; i < len(idx); i++ {
		if idx[i] <= idx[i-1] {
			t.Errorf("consolidated order wrong: %v not ascending (%v)", idx, m2.Log.Lines)
		}
	}
}

// TestBoardLOpensConsolidated — the second entry point: L from the board reaches
// the consolidated log without first entering the live table.
func TestBoardLOpensConsolidated(t *testing.T) {
	repo := t.TempDir()
	writeLog(t, repo, "rel", "loop", []string{"2026-07-03T10:00:00.000 RunParallel: loaded 2 tracks"})

	m := &Model{state: viewBoard, repoRoot: repo, Height: 20, Board: &BoardView{ReleaseName: "rel"}}
	upd, _ := m.Update(runeKey('L'))
	m2 := upd.(*Model)

	if m2.state != viewLog || m2.Log == nil || m2.Log.Track != "" {
		t.Fatalf("expected consolidated viewLog from board L, got state=%d log=%+v", m2.state, m2.Log)
	}
	if m2.Log.origin != viewBoard {
		t.Errorf("expected origin viewBoard, got %d", m2.Log.origin)
	}
	if !strings.Contains(m2.View(), "loaded 2 tracks") {
		t.Errorf("view missing coordinator line:\n%s", m2.View())
	}
}

// TestLogTailFollowOnTick — a new line appended to the file appears after a
// logTickMsg driven through the root Model, and follow keeps the tail pinned.
func TestLogTailFollowOnTick(t *testing.T) {
	repo := t.TempDir()
	writeLog(t, repo, "rel", "T1", []string{"2026-07-03T10:00:00.000 [T1] first line"})

	m := liveModel(repo, "rel", []TrackRow{{ID: "T1"}}, 20)
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := upd.(*Model)
	if !m2.Log.follow {
		t.Fatal("expected follow=true on open")
	}

	// Simulate more narration streaming to disk.
	writeLog(t, repo, "rel", "T1", []string{
		"2026-07-03T10:00:00.000 [T1] first line",
		"2026-07-03T10:00:05.000 [T1] freshly appended",
	})

	upd2, cmd := m2.Update(logTickMsg{})
	m3 := upd2.(*Model)
	if cmd == nil {
		t.Error("expected a re-arm cmd from the log tick (chain must stay alive)")
	}
	if !strings.Contains(m3.View(), "freshly appended") {
		t.Errorf("tail-follow did not surface the appended line:\n%s", m3.View())
	}
	if !m3.Log.follow {
		t.Error("expected follow to stay pinned after tick")
	}
}

// TestLogScrollbackFreezesFollow — scrolling up freezes the viewport: a tick
// grows Lines but the offset does not move; G re-pins to the tail.
func TestLogScrollbackFreezesFollow(t *testing.T) {
	repo := t.TempDir()
	six := make([]string, 6)
	for i := range six {
		six[i] = fmt.Sprintf("2026-07-03T10:00:0%d.000 [T1] line-%d", i, i)
	}
	writeLog(t, repo, "rel", "T1", six)

	m := liveModel(repo, "rel", []TrackRow{{ID: "T1"}}, 3)
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := upd.(*Model)
	if m2.Log.offset != 3 { // 6 lines - height 3 => pinned bottom at 3
		t.Fatalf("expected initial pinned offset 3, got %d", m2.Log.offset)
	}

	// Scroll up one → freezes follow.
	upd, _ = m2.Update(runeKey('k'))
	m3 := upd.(*Model)
	if m3.Log.follow || m3.Log.offset != 2 {
		t.Fatalf("expected follow=false offset=2 after k, got follow=%v offset=%d", m3.Log.follow, m3.Log.offset)
	}

	// More lines stream in; a tick must grow Lines but NOT move the frozen offset.
	eight := append(append([]string{}, six...),
		"2026-07-03T10:00:06.000 [T1] line-6",
		"2026-07-03T10:00:07.000 [T1] line-7")
	writeLog(t, repo, "rel", "T1", eight)

	upd, _ = m3.Update(logTickMsg{})
	m4 := upd.(*Model)
	if len(m4.Log.Lines) != 8 {
		t.Fatalf("expected Lines to grow to 8, got %d", len(m4.Log.Lines))
	}
	if m4.Log.offset != 2 {
		t.Errorf("expected frozen offset 2 after tick, got %d", m4.Log.offset)
	}

	// G re-pins to the tail.
	upd, _ = m4.Update(runeKey('G'))
	m5 := upd.(*Model)
	if !m5.Log.follow || m5.Log.offset != 5 { // 8 - 3
		t.Errorf("expected G to re-pin follow=true offset=5, got follow=%v offset=%d", m5.Log.follow, m5.Log.offset)
	}
}

// TestLogTickCadenceGuard — the M1 guard: while in viewLog, a stray LiveView
// tickMsg is dropped (no advance, no re-arm), while the view's own logTickMsg
// advances it and re-arms. Proves the two views never share a tick cadence.
func TestLogTickCadenceGuard(t *testing.T) {
	repo := t.TempDir()
	writeLog(t, repo, "rel", "T1", []string{"2026-07-03T10:00:00.000 [T1] one"})

	m := liveModel(repo, "rel", []TrackRow{{ID: "T1"}}, 20)
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := upd.(*Model)
	before := len(m2.Log.Lines)

	// Grow the file, then deliver a STRAY LiveView tickMsg while in viewLog.
	writeLog(t, repo, "rel", "T1", []string{
		"2026-07-03T10:00:00.000 [T1] one",
		"2026-07-03T10:00:01.000 [T1] two",
	})
	upd, strayCmd := m2.Update(tickMsg{})
	m3 := upd.(*Model)
	if strayCmd != nil {
		t.Error("stray tickMsg in viewLog must NOT re-arm (returned a cmd)")
	}
	if len(m3.Log.Lines) != before {
		t.Errorf("stray tickMsg must not advance LogView: was %d now %d", before, len(m3.Log.Lines))
	}

	// The view's OWN tick advances and re-arms.
	upd, ownCmd := m3.Update(logTickMsg{})
	m4 := upd.(*Model)
	if ownCmd == nil {
		t.Error("logTickMsg in viewLog must re-arm the chain")
	}
	if len(m4.Log.Lines) != before+1 {
		t.Errorf("logTickMsg must advance LogView: was %d now %d", before, len(m4.Log.Lines))
	}
}

// TestLogViewMissingDirGraceful — a release with no logs shows an empty state
// and never panics.
func TestLogViewMissingDirGraceful(t *testing.T) {
	repo := t.TempDir() // no .sworn/logs at all

	m := liveModel(repo, "rel", []TrackRow{{ID: "T1"}}, 20)
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := upd.(*Model)

	if m2.state != viewLog || m2.Log == nil {
		t.Fatalf("expected viewLog even with no logs, got state=%d", m2.state)
	}
	if len(m2.Log.Lines) != 0 {
		t.Errorf("expected zero lines for missing logs, got %d", len(m2.Log.Lines))
	}
	if !strings.Contains(m2.View(), "No logs yet") {
		t.Errorf("expected graceful empty state, got:\n%s", m2.View())
	}
}

// TestLogEscReturnsToOrigin — esc returns to the view the log was opened from
// (viewLive here), keeping the back-stack consistent (M4).
func TestLogEscReturnsToOrigin(t *testing.T) {
	repo := t.TempDir()
	writeLog(t, repo, "rel", "T1", []string{"2026-07-03T10:00:00.000 [T1] x"})

	m := liveModel(repo, "rel", []TrackRow{{ID: "T1"}}, 20)
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := upd.(*Model)

	upd, _ = m2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m3 := upd.(*Model)
	if m3.state != viewLive {
		t.Errorf("expected esc to return to viewLive origin, got %d", m3.state)
	}
}

// TestReachabilityLogSmoke — the Rule 1 reachability artefact: fixture log files
// are written via the REAL engine tee (tracklog.NewWriter — the exact seam
// RunTrack uses), then the TUI model is driven programmatically to prove the
// pane renders them. Both per-track (enter) and consolidated (L) journeys.
func TestReachabilityLogSmoke(t *testing.T) {
	repo := t.TempDir()
	logDir := logDirFor(repo, "rel")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write via the production tee seam, exactly as a worker would.
	w1, c1 := tracklog.NewWriter(logDir, "T1")
	fmt.Fprintf(w1, "[T1] starting\n")
	fmt.Fprintf(w1, "[T1] router: S01 → implement (ready)\n")
	c1()
	w2, c2 := tracklog.NewWriter(logDir, "merge:T3")
	fmt.Fprintf(w2, "[merge:T3] auto-merging into release-wt\n")
	c2()

	// Confirm the sanitised filename landed on disk (no raw colon).
	if _, err := os.Stat(filepath.Join(logDir, "merge__T3.log")); err != nil {
		t.Fatalf("sanitised merge log not written: %v", err)
	}

	// Drive the TUI: enter on T1 opens its log.
	m := liveModel(repo, "rel", []TrackRow{{ID: "T1"}, {ID: "merge:T3"}}, 20)
	upd, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	perTrack := upd.(*Model)
	pane := perTrack.View()
	if !strings.Contains(pane, "router: S01 → implement (ready)") {
		t.Fatalf("per-track pane did not render the teed line:\n%s", pane)
	}
	t.Logf("PER-TRACK PANE (T1):\n%s", pane)

	// esc back to live, then L opens consolidated (T1 + merge:T3 interleaved).
	upd, _ = perTrack.Update(tea.KeyMsg{Type: tea.KeyEsc})
	back := upd.(*Model)
	upd, _ = back.Update(runeKey('L'))
	consolidated := upd.(*Model)
	cpane := consolidated.View()
	if !strings.Contains(cpane, "[T1] starting") || !strings.Contains(cpane, "[merge:T3] auto-merging") {
		t.Fatalf("consolidated pane missing lines from both files:\n%s", cpane)
	}
	t.Logf("CONSOLIDATED PANE:\n%s", cpane)

	// Dump the raw on-disk files as part of the artefact.
	for _, name := range []string{"T1.log", "merge__T3.log"} {
		b, _ := os.ReadFile(filepath.Join(logDir, name))
		t.Logf("FILE %s:\n%s", name, string(b))
	}
}
