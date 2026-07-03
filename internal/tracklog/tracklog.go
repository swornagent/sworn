// Package tracklog owns the durable per-track worker log seam: the io.Writer
// tee that mirrors a worker's stderr narration into an append-only, versioned
// .sworn/logs/<release>/<track>.log file, and the ONE filename sanitiser that
// both the writer (engine) and the reader (TUI) share.
//
// Contract (Coach decision E1, 2026-07-03): every log file opens with the
// format-version header FormatHeader ("# sworn-log v1"). The path
// .sworn/logs/<release>/<track>.log is the hard, ratified contract; the line
// format is a versioned Type-2 that future readers can evolve behind the
// version marker.
//
// Stdlib only (AGENTS.md non-negotiable).
package tracklog

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FormatHeader is the first line written to every freshly-created log file. It
// versions the on-disk line format so future readers can evolve it behind the
// marker (Coach decision E1).
const FormatHeader = "# sworn-log v1"

// tsLayout is the per-line timestamp prefix written to the file side. It is a
// full date+time so the consolidated k-way merge (internal/tui) sorts
// correctly ACROSS a midnight / multi-hour boundary — the merge key is
// lexicographically chronological (Captain pin M2). stderr stays bare.
const tsLayout = "2006-01-02T15:04:05.000"

// softCapBytes is the per-file soft size cap. On exceed the writer renames the
// live file to <name>.log.1 (one prior generation kept) and reopens. Type-2
// default (design C3); revisit if real runs show it wrong.
const softCapBytes = 8 << 20 // 8 MiB

// SanitiseTrackID maps a raw track ID to a filesystem-safe filename stem. It is
// the SINGLE sanitiser shared by the writer and the TUI reader (Captain pin
// M5): a track ID of "merge:T3" must produce the SAME "merge__T3.log" on both
// sides. The colon — illegal on some filesystems and awkward in shells — maps
// to "__"; any other non-portable rune maps to a single '_'. The raw track ID
// is still the in-file line prefix, so display is unaffected.
func SanitiseTrackID(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for _, r := range id {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			b.WriteRune(r)
		case r == ':':
			b.WriteString("__")
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// LogFileName returns the sanitised log filename (stem + ".log") for a track ID.
// Both the engine writer and the TUI reader call this so the filename is
// computed in exactly one place.
func LogFileName(trackID string) string {
	return SanitiseTrackID(trackID) + ".log"
}

// NewWriter returns an io.Writer that tees every write to os.Stderr (verbatim,
// byte-for-byte — today's behaviour is preserved exactly) and to
// logDir/<sanitised-track>.log (a timestamp-prefixed copy, append-only), plus a
// close func to release the file handle.
//
// When logDir == "" it returns os.Stderr and a no-op closer, so legacy callers
// and tests are byte-for-byte unaffected (the "empty dir = stderr only"
// invariant, design A.3). On any file-open error it also falls back to
// stderr-only (fail open): a log problem must never break the run.
func NewWriter(logDir, trackID string) (io.Writer, func() error) {
	if logDir == "" {
		return os.Stderr, func() error { return nil }
	}
	tl := &teeLogger{
		stderr: os.Stderr,
		path:   filepath.Join(logDir, LogFileName(trackID)),
	}
	if err := tl.open(); err != nil {
		return os.Stderr, func() error { return nil }
	}
	return tl, tl.close
}

// teeLogger writes p verbatim to stderr and a timestamp-prefixed copy to file.
type teeLogger struct {
	mu     sync.Mutex
	stderr io.Writer
	path   string
	file   *os.File
	size   int64
}

// open creates the parent dir (defensive), opens the log O_APPEND|O_CREATE, and
// writes the format-version header when the file is newly created (size 0) so
// each file begins with exactly one header even across append-reopens.
func (t *teeLogger) open() error {
	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(t.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	t.file = f
	t.size = fi.Size()
	if t.size == 0 {
		n, _ := f.WriteString(FormatHeader + "\n")
		t.size += int64(n)
	}
	return nil
}

// Write writes p verbatim to stderr (the preserved guarantee) and, best-effort,
// a timestamp-prefixed copy to the file. The stderr write is authoritative for
// the io.Writer contract: a file-side error after a successful stderr write is
// swallowed so it can never break the caller's narration.
func (t *teeLogger) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	n, err := t.stderr.Write(p)
	if err != nil {
		return n, err
	}
	if t.file != nil {
		t.writeFile(p)
	}
	return n, nil
}

// writeFile prepends the per-line timestamp to each newline-terminated line of
// p (the narration is line-oriented, always ending each message with '\n') and
// appends to the file, rotating on soft-cap exceed.
func (t *teeLogger) writeFile(p []byte) {
	ts := time.Now().Format(tsLayout)
	var b strings.Builder
	b.Grow(len(p) + len(ts) + 2)
	start := 0
	for i := 0; i < len(p); i++ {
		if p[i] == '\n' {
			b.WriteString(ts)
			b.WriteByte(' ')
			b.Write(p[start : i+1])
			start = i + 1
		}
	}
	if start < len(p) { // trailing partial line (no terminating newline)
		b.WriteString(ts)
		b.WriteByte(' ')
		b.Write(p[start:])
	}
	n, _ := t.file.WriteString(b.String())
	t.size += int64(n)
	if t.size >= softCapBytes {
		t.rotate()
	}
}

// rotate renames the live file to <name>.log.1 (replacing any prior .1) and
// reopens a fresh generation with its own header. One prior generation kept.
func (t *teeLogger) rotate() {
	if t.file != nil {
		t.file.Close()
	}
	_ = os.Rename(t.path, t.path+".1")
	f, err := os.OpenFile(t.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.file = nil
		return
	}
	t.file = f
	t.size = 0
	n, _ := f.WriteString(FormatHeader + "\n")
	t.size += int64(n)
}

// close releases the file handle. stderr is never closed (it is process-global).
func (t *teeLogger) close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.file != nil {
		err := t.file.Close()
		t.file = nil
		return err
	}
	return nil
}
