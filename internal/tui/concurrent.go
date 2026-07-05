package tui

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/swornagent/sworn/internal/db"
)

// TrackRow holds live state for one track row in the concurrent status view.
type TrackRow struct {
	ID           string
	CurrentSlice string
	State        string
	StartedAt    string
	Elapsed      string // computed relative time, updated each tick
	IsMerge      bool   // true for merge:<track> actor rows
}

// tickMsg is delivered every ~1 second to trigger a DB re-poll.
type tickMsg struct{}

// LiveView is a Bubble Tea component that polls the SQLite DB every second
// and renders a live concurrent status table. It is embedded in the root Model
// when the user selects a release with running tracks.
type LiveView struct {
	ReleaseName string
	Rows        []TrackRow
	TickCount   int // monotonic counter — increments each tick
	Cursor      int // selected row index — j/k move it; enter opens its log

	conn *sql.DB // DB connection via db.Open() — Coach option (a): read-write, WAL-safe
}

// StartLiveView opens a DB connection for the given release and performs the
// first poll synchronously so the initial render is populated.
func StartLiveView(repoRoot, releaseName string) (*LiveView, error) {
	dbPath := db.DefaultPath(repoRoot)
	conn, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("live: open db: %w", err)
	}

	lv := &LiveView{
		ReleaseName: releaseName,
		Rows:        nil,
		conn:        conn,
	}

	// First poll — synchronous, so the initial View() call has data.
	if err := lv.poll(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("live: initial poll: %w", err)
	}

	return lv, nil
}

// HasInProgressTracks checks whether the given release has at least one track
// row with state = 'in_progress' in the SQLite DB. Returns true without error
// if the DB doesn't exist or is empty (caller treats missing DB as "no live tracks").
func HasInProgressTracks(repoRoot, releaseName string) bool {
	dbPath := db.DefaultPath(repoRoot)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return false
	}

	conn, err := db.Open(dbPath)
	if err != nil {
		return false
	}
	defer conn.Close()

	var count int
	err = conn.QueryRow(
		"SELECT COUNT(*) FROM tracks WHERE release = ? AND state = 'in_progress'",
		releaseName,
	).Scan(&count)
	return err == nil && count > 0
}

// ActiveMerges returns the track_ids of all active merge actors for the given
// release. A merge actor is "active" if its most-recent event in the events
// table is 'acquired' (not 'released-*'). The query uses a MAX(id) subquery to
// find the latest event per merge:* track_id, then filters for 'acquired'.
//
// Returns nil (not an error) if the DB doesn't exist or no active merges are
// found. The board view calls this to render merge badges on track headers.
func ActiveMerges(repoRoot, releaseName string) []string {
	dbPath := db.DefaultPath(repoRoot)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil
	}

	conn, err := db.Open(dbPath)
	if err != nil {
		return nil
	}
	defer conn.Close()

	rows, err := conn.Query(
		`SELECT track_id FROM events
		 WHERE id IN (
		   SELECT MAX(id) FROM events
		   WHERE release = ? AND track_id LIKE 'merge:%'
		   GROUP BY track_id
		 ) AND event = 'acquired'`,
		releaseName,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var merges []string
	for rows.Next() {
		var trackID string
		if err := rows.Scan(&trackID); err != nil {
			continue
		}
		merges = append(merges, trackID)
	}
	return merges
}

// Init implements tea.Component. It starts the first tick.
func (lv *LiveView) Init() tea.Cmd {
	return lv.tickCmd()
}

// Update implements tea.Component.
func (lv *LiveView) Update(msg tea.Msg) (*LiveView, tea.Cmd) {
	switch msg.(type) {
	case tickMsg:
		lv.TickCount++
		if err := lv.poll(); err != nil {
			// Log error silently — keep stale rows rather than crash.
			return lv, lv.tickCmd()
		}
		return lv, lv.tickCmd()
	}
	return lv, nil
}

// View renders the live concurrent status table.
func (lv *LiveView) View() string {
	if lv.conn == nil {
		return "Live: no connection\n"
	}

	var sb strings.Builder
	sb.WriteString(LiveTitle.Render("Live: " + lv.ReleaseName))
	sb.WriteString("\n\n")

	if len(lv.Rows) == 0 {
		sb.WriteString(EmptyMessage.Render("No active tracks"))
		return sb.String()
	}

	// Table header.
	headerFmt := lipgloss.NewStyle().Bold(true).Foreground(colPrimary).Padding(0, 1)
	sb.WriteString(headerFmt.Render(fmt.Sprintf("%-14s %-20s %-12s %s",
		"Track", "Slice", "State", "Elapsed")))
	sb.WriteString("\n")
	sb.WriteString(DividerLine)
	sb.WriteString("\n")

	// Table rows.
	for i, row := range lv.Rows {
		stateDisplay := stateDisplay(row.State)
		sliceDisplay := row.CurrentSlice
		if sliceDisplay == "" {
			sliceDisplay = "—"
		}
		marker := "  "
		if i == lv.Cursor {
			marker = "> "
		}
		line := marker + fmt.Sprintf("%-14s %-20s %-12s %s",
			row.ID, sliceDisplay, stateDisplay, row.Elapsed)
		if row.IsMerge {
			sb.WriteString(MergeRowStyle.Render(line))
		} else {
			sb.WriteString(LiveRow.Render(line))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	sb.WriteString(EmptyMessage.Render("enter: open log   L: consolidated log   b: board"))
	return sb.String()
}

// Close closes the DB connection.
func (lv *LiveView) Close() error {
	if lv.conn != nil {
		return lv.conn.Close()
	}
	return nil
}

// poll queries the DB for all non-planned tracks of the selected release,
// plus any active merge:<track> actors from the events table.
func (lv *LiveView) poll() error {
	if lv.conn == nil {
		return fmt.Errorf("live: no connection")
	}

	rows, err := lv.conn.Query(
		"SELECT id, current_slice, state, started_at FROM tracks WHERE release = ? AND state != 'planned' AND state != 'verified'",
		lv.ReleaseName,
	)
	if err != nil {
		return fmt.Errorf("live: query: %w", err)
	}
	defer rows.Close()

	var results []TrackRow
	now := time.Now()
	for rows.Next() {
		var tr TrackRow
		if err := rows.Scan(&tr.ID, &tr.CurrentSlice, &tr.State, &tr.StartedAt); err != nil {
			continue
		}
		tr.Elapsed = computeElapsed(tr.StartedAt, now)
		results = append(results, tr)
	}

	// Query active merge:<track> actors from the events table.
	// A merge actor is active if its most-recent event is 'acquired'.
	mergeRows, err := lv.conn.Query(
		`SELECT e.track_id, e.detail, e.ts FROM events e
		 WHERE e.id IN (
		   SELECT MAX(id) FROM events
		   WHERE release = ? AND track_id LIKE 'merge:%'
		   GROUP BY track_id
		 ) AND e.event = 'acquired'`,
		lv.ReleaseName,
	)
	if err == nil {
		for mergeRows.Next() {
			var tr TrackRow
			var detail, ts string
			if err := mergeRows.Scan(&tr.ID, &detail, &ts); err != nil {
				continue
			}
			tr.IsMerge = true
			tr.State = "merging"
			tr.CurrentSlice = detail
			if tr.CurrentSlice == "" {
				tr.CurrentSlice = "—"
			}
			tr.StartedAt = ts
			tr.Elapsed = computeElapsed(ts, now)
			results = append(results, tr)
		}
		mergeRows.Close()
	}

	lv.Rows = results
	// Keep the row cursor in range as rows come and go across polls.
	if lv.Cursor >= len(lv.Rows) {
		lv.Cursor = len(lv.Rows) - 1
	}
	if lv.Cursor < 0 {
		lv.Cursor = 0
	}
	return rows.Err()
}

// tickCmd returns a tea.Cmd that fires one tickMsg after ~1 second.
func (lv *LiveView) tickCmd() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

// computeElapsed returns a human-readable elapsed time since startedAt.
func computeElapsed(startedAt string, now time.Time) string {
	if startedAt == "" {
		return "—"
	}
	t, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		// Try ISO 8601 without timezone.
		t, err = time.Parse("2006-01-02T15:04:05", startedAt)
		if err != nil {
			return startedAt
		}
	}
	d := now.Sub(t)
	if d < 0 {
		return "0s"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// stateDisplay renders a live status badge.
func stateDisplay(state string) string {
	switch state {
	case "in_progress":
		return SliceStateActive(state)
	case "implemented":
		return SliceStateActive("running") // running
	case "failed_verification":
		return SliceStateFailed("blocked")
	case "blocked":
		return SliceStateBlocked("blocked")
	default:
		return state
	}
}

// CreditFileBalance reads the credits balance from the user's credits file.
// Returns the balance string (e.g. "42") and a bool indicating whether the
// file existed and was parsed. If the file doesn't exist, returns ("–", false).
// If the file exists but is malformed, shows the error inline.
func CreditFileBalance() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "–", false
	}

	creditPath := filepath.Join(home, ".config", "sworn", "credits.json")
	data, err := os.ReadFile(creditPath)
	if os.IsNotExist(err) {
		return "–", false
	}
	if err != nil {
		return fmt.Sprintf("err: %v", err), true
	}

	var parsed struct {
		Balance int `json:"balance"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Sprintf("err: %v", err), true
	}

	return fmt.Sprintf("%d", parsed.Balance), true
}
