package tui

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/swornagent/sworn/internal/db"
	"github.com/swornagent/sworn/internal/tracklog"
)

// logTickMsg drives the LogView tail-follow poll. It is a DISTINCT type from
// LiveView's tickMsg (Captain pin M1 — tick-chain multiplication guard): the
// root Model services a logTickMsg only when state == viewLog and a tickMsg
// only when state == viewLive, so a stray in-flight tick from the other view
// (scheduled before a view switch) is dropped rather than adopted — it never
// spawns a second chain. Each view therefore keeps exactly one tick cadence.
type logTickMsg struct{}

// LogView is a Bubble Tea component that renders a worker log — either a single
// track's <track>.log or the consolidated interleave of every log in the
// release's log dir — with tail-follow while the loop runs and scrollback.
type LogView struct {
	ReleaseName string
	LogDir      string // .sworn/logs/<release>
	Track       string // "" => consolidated; else a single <track-id>
	Lines       []string

	offset int  // top line of the viewport (scrollback position)
	follow bool // true while pinned to the tail
	Height int  // viewport rows, from Model.Height

	// origin is the view to return to on esc (viewLive when opened from the
	// live table, viewBoard when opened via L from the board) — keeps the
	// back-stack consistent (Captain pin M4).
	origin viewState
}

// StartLogView opens the log(s) for a release and does the first read
// synchronously so the initial View() is populated (mirrors StartLiveView).
// track == "" opens the consolidated view.
func StartLogView(repoRoot, release, track string, origin viewState, height int) *LogView {
	lv := &LogView{
		ReleaseName: release,
		LogDir:      filepath.Join(repoRoot, db.DefaultDir, "logs", release),
		Track:       track,
		follow:      true,
		Height:      height,
		origin:      origin,
	}
	lv.reload()
	return lv
}

// Init starts this view's own tick chain (logTickMsg).
func (lv *LogView) Init() tea.Cmd {
	return lv.tickCmd()
}

// Update handles the tail-follow tick. Only logTickMsg advances the view; any
// other message (including a stray LiveView tickMsg forwarded in error) is a
// no-op and, crucially, does NOT re-arm — so it cannot start a second chain.
func (lv *LogView) Update(msg tea.Msg) (*LogView, tea.Cmd) {
	switch msg.(type) {
	case logTickMsg:
		lv.reload()
		return lv, lv.tickCmd()
	}
	return lv, nil
}

func (lv *LogView) tickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg { return logTickMsg{} })
}

// reload re-reads the underlying file(s) and, when following, re-pins the
// viewport to the tail. When NOT following (the user scrolled up) the offset is
// left untouched so the viewport stays frozen even as new lines stream in.
func (lv *LogView) reload() {
	if lv.Track != "" {
		lv.Lines = readLogFile(filepath.Join(lv.LogDir, tracklog.LogFileName(lv.Track)))
	} else {
		lv.Lines = readConsolidated(lv.LogDir)
	}
	if lv.follow {
		lv.pinBottom()
	}
}

func (lv *LogView) viewportHeight() int {
	if lv.Height > 0 {
		return lv.Height
	}
	return 20
}

func (lv *LogView) pinBottom() {
	h := lv.viewportHeight()
	if len(lv.Lines) > h {
		lv.offset = len(lv.Lines) - h
	} else {
		lv.offset = 0
	}
}

func (lv *LogView) scrollUp() {
	lv.follow = false
	if lv.offset > 0 {
		lv.offset--
	}
}

func (lv *LogView) scrollDown() {
	h := lv.viewportHeight()
	max := len(lv.Lines) - h
	if max < 0 {
		max = 0
	}
	if lv.offset < max {
		lv.offset++
	}
	if lv.offset >= max {
		lv.follow = true // scrolled back to the tail → resume following
	}
}

func (lv *LogView) top() {
	lv.follow = false
	lv.offset = 0
}

func (lv *LogView) bottom() {
	lv.follow = true
	lv.pinBottom()
}

// View renders the log viewport.
func (lv *LogView) View() string {
	var sb strings.Builder

	title := "Log: " + lv.ReleaseName + " / "
	if lv.Track != "" {
		title += lv.Track
	} else {
		title += "consolidated"
	}
	sb.WriteString(LiveTitle.Render(title))
	sb.WriteString("\n\n")

	if len(lv.Lines) == 0 {
		sb.WriteString(EmptyMessage.Render("No logs yet"))
		return sb.String()
	}

	h := lv.viewportHeight()
	start := lv.offset
	if start < 0 {
		start = 0
	}
	if start > len(lv.Lines) {
		start = len(lv.Lines)
	}
	end := start + h
	if end > len(lv.Lines) {
		end = len(lv.Lines)
	}
	for _, line := range lv.Lines[start:end] {
		sb.WriteString(LiveRow.Render(line))
		sb.WriteString("\n")
	}
	return sb.String()
}

// readLogFile reads a single log file, dropping the "# sworn-log" version
// header line(s). Missing/unreadable file → nil (graceful empty state).
func readLogFile(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "# sworn-log") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// readConsolidated reads every *.log in dir (excluding rotated *.log.1) and
// merges the lines by their leading timestamp prefix. Because each file is
// already chronological the union sorted by timestamp is the interleave; a
// stable sort keeps same-timestamp lines in a deterministic per-file order.
// Lines without a parseable timestamp sort by their raw prefix, stably.
func readConsolidated(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	type keyed struct {
		key  string
		line string
		seq  int
	}
	var all []keyed
	seq := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".log") {
			continue // excludes <track>.log.1 (suffix .1) and non-log files
		}
		for _, line := range readLogFile(filepath.Join(dir, name)) {
			all = append(all, keyed{key: timestampKey(line), line: line, seq: seq})
			seq++
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].key == all[j].key {
			return all[i].seq < all[j].seq
		}
		return all[i].key < all[j].key
	})
	out := make([]string, len(all))
	for i := range all {
		out[i] = all[i].line
	}
	return out
}

// timestampKey returns the leading whitespace-delimited token of a line — the
// "2006-01-02T15:04:05.000" prefix the tee writes. Full date+time means the key
// is lexicographically chronological across midnight (Captain pin M2).
func timestampKey(line string) string {
	if i := strings.IndexByte(line, ' '); i >= 0 {
		return line[:i]
	}
	return line
}
