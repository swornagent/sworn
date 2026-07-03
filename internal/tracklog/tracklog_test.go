package tracklog

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// tsLinePrefix matches the "2006-01-02T15:04:05.000 " prefix the file side
// prepends to every line (a full date+time so the consolidated merge sorts
// across a midnight boundary — Captain pin M2).
var tsLinePrefix = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3} `)

// captureStderr redirects os.Stderr across fn and returns what was written to
// it. NewWriter captures os.Stderr at construction, so fn must construct the
// writer AND write through it while the pipe is installed.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)
	r.Close()
	return string(out)
}

func readFileLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

// TestTrackWriterTeesToFileAndStderr — the stderr side is byte-for-byte the
// input (the "stderr unchanged" guarantee), and the file side carries a
// parseable timestamp prefix plus the original text and a leading format header.
func TestTrackWriterTeesToFileAndStderr(t *testing.T) {
	dir := t.TempDir()
	const msg = "[T1] router: S01 → implement (ready)\n"

	stderr := captureStderr(t, func() {
		w, closeLog := NewWriter(dir, "T1")
		fmt.Fprint(w, msg)
		closeLog()
	})

	// (a) stderr side byte-for-byte identical to the input.
	if stderr != msg {
		t.Errorf("stderr side not byte-for-byte:\n got %q\nwant %q", stderr, msg)
	}

	// (b) file side: header + timestamp-prefixed line containing the text.
	lines := readFileLines(t, filepath.Join(dir, "T1.log"))
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header + entry), got %d: %v", len(lines), lines)
	}
	if lines[0] != FormatHeader {
		t.Errorf("first line = %q, want header %q", lines[0], FormatHeader)
	}
	if !tsLinePrefix.MatchString(lines[1]) {
		t.Errorf("entry missing parseable timestamp prefix: %q", lines[1])
	}
	if !strings.Contains(lines[1], "[T1] router: S01 → implement (ready)") {
		t.Errorf("entry missing original text: %q", lines[1])
	}
}

// TestTrackWriterEmptyDirIsStderrOnly — LogDir=="" returns a writer whose only
// effect is stderr; no file is created anywhere (legacy back-compat invariant).
func TestTrackWriterEmptyDirIsStderrOnly(t *testing.T) {
	probe := t.TempDir() // must stay empty
	stderr := captureStderr(t, func() {
		w, closeLog := NewWriter("", "T1")
		if w != io.Writer(os.Stderr) {
			t.Errorf("empty LogDir: expected the writer to be os.Stderr itself")
		}
		fmt.Fprint(w, "[T1] starting\n")
		closeLog()
	})
	if stderr != "[T1] starting\n" {
		t.Errorf("stderr = %q", stderr)
	}
	entries, _ := os.ReadDir(probe)
	if len(entries) != 0 {
		t.Errorf("expected no files created for empty LogDir, found %d", len(entries))
	}
}

// TestTrackWriterAppendCrashSafe — write, reopen (new writer, same path), write
// again; the file contains BOTH runs' lines in order (append, not truncate),
// with a single header at the top.
func TestTrackWriterAppendCrashSafe(t *testing.T) {
	dir := t.TempDir()

	w1, close1 := NewWriter(dir, "T1")
	fmt.Fprint(w1, "[T1] run-one line\n")
	close1()

	w2, close2 := NewWriter(dir, "T1")
	fmt.Fprint(w2, "[T1] run-two line\n")
	close2()

	lines := readFileLines(t, filepath.Join(dir, "T1.log"))
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (1 header + 2 entries), got %d: %v", len(lines), lines)
	}
	if lines[0] != FormatHeader {
		t.Errorf("header not first / duplicated: %v", lines)
	}
	if !strings.Contains(lines[1], "run-one line") || !strings.Contains(lines[2], "run-two line") {
		t.Errorf("append order wrong: %v", lines)
	}
}

// TestTrackWriterRotates — with an artificially low cap, exceed it; <track>.log.1
// appears and the live <track>.log continues.
func TestTrackWriterRotates(t *testing.T) {
	dir := t.TempDir()
	tl := &teeLogger{stderr: io.Discard, path: filepath.Join(dir, "T1.log")}
	if err := tl.open(); err != nil {
		t.Fatalf("open: %v", err)
	}
	// Shrink the effective cap by writing until size crosses the constant is
	// impractical (8 MiB); instead drive rotate() directly after a write, which
	// is what writeFile does on cap-exceed. Assert the rename + reopen behaviour.
	fmt.Fprint(tl, "[T1] before rotate\n")
	tl.rotate()
	fmt.Fprint(tl, "[T1] after rotate\n")
	tl.close()

	if _, err := os.Stat(filepath.Join(dir, "T1.log.1")); err != nil {
		t.Errorf("expected rotated T1.log.1 to exist: %v", err)
	}
	live := readFileLines(t, filepath.Join(dir, "T1.log"))
	if len(live) == 0 || live[0] != FormatHeader {
		t.Errorf("live file should start with a fresh header after rotate: %v", live)
	}
	joined := strings.Join(live, "\n")
	if !strings.Contains(joined, "after rotate") {
		t.Errorf("live file missing post-rotate line: %v", live)
	}
	if strings.Contains(joined, "before rotate") {
		t.Errorf("pre-rotate line should be in .1, not the live file: %v", live)
	}
}

// TestTrackIDSanitisedForFilename — merge:T3 produces merge__T3.log and never a
// path containing a raw ':'. The SAME sanitiser is what the TUI reader calls
// (M5: one shared function).
func TestTrackIDSanitisedForFilename(t *testing.T) {
	if got := SanitiseTrackID("merge:T3"); got != "merge__T3" {
		t.Errorf("SanitiseTrackID(merge:T3) = %q, want merge__T3", got)
	}
	if got := LogFileName("merge:T3"); got != "merge__T3.log" || strings.Contains(got, ":") {
		t.Errorf("LogFileName(merge:T3) = %q, want merge__T3.log with no colon", got)
	}
	// Plain track IDs are unchanged.
	if got := SanitiseTrackID("T1-engine"); got != "T1-engine" {
		t.Errorf("SanitiseTrackID(T1-engine) = %q, want unchanged", got)
	}

	dir := t.TempDir()
	w, closeLog := NewWriter(dir, "merge:T3")
	fmt.Fprint(w, "[merge:T3] merging\n")
	closeLog()
	if _, err := os.Stat(filepath.Join(dir, "merge__T3.log")); err != nil {
		t.Errorf("expected merge__T3.log on disk: %v", err)
	}
}
