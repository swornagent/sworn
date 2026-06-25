package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/command"
	"github.com/swornagent/sworn/internal/ledger"
	"github.com/swornagent/sworn/internal/state"
)

// ── Registry reachability ────────────────────────────────────────────────

func TestLedgerCommandRegistered(t *testing.T) {
	c, ok := command.Lookup("ledger")
	if !ok {
		t.Fatal("command.Lookup(\"ledger\") not found — init() in cmd/sworn/ledger.go did not register")
	}
	if c.Name != "ledger" {
		t.Errorf("Name: want ledger, got %s", c.Name)
	}
	if c.Summary == "" {
		t.Error("Summary must be non-empty")
	}
	if c.Run == nil {
		t.Error("Run must be non-nil")
	}
}

// ── Sync integration test ────────────────────────────────────────────────

// setupFixtureRelease creates a temporary fixture release tree with a verified
// slice that has a terminal PASS verdict and a spec with known gate count.
func setupFixtureRelease(t *testing.T, dir string) {
	t.Helper()

	// Build the release board structure.
	releaseDir := filepath.Join(dir, "docs", "release", "fixture-release")
	sliceDir := filepath.Join(releaseDir, "S01-test-slice")
	if err := os.MkdirAll(sliceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// spec.md with 4 acceptance checks.
	spec := `# Slice: S01-test-slice

## Acceptance checks

- [ ] First check
- [ ] Second check
- [ ] Third check
- [ ] Fourth check
`
	if err := os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	// status.json with a terminal PASS verdict.
	st := state.Status{
		SliceID: "S01-test-slice",
		Release: "fixture-release",
		Track:   "T5-providers",
		State:   state.Verified,
		Verification: state.Verification{
			Result: "pass",
			Model:  "claude-sonnet",
			Attempt: 1,
		},
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sliceDir, "status.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Create docs/ledger/ to ensure the parent dir exists.
	if err := os.MkdirAll(filepath.Join(dir, "docs", "ledger"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// chdirTemp changes the working directory to dir and restores it on test completion.
// The wrapped function runs with cwd = dir. This satisfies Rule 11 (process-global
// mutation: guaranteed restore).
func chdirTemp(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Errorf("chdir restore: %v", err)
		}
	})
}

func TestSync_AppendsRecord(t *testing.T) {
	dir := t.TempDir()

	// Create .git so findRepoRoot works from this tree.
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	setupFixtureRelease(t, dir)
	chdirTemp(t, dir)

	// First sync: should add 1 record.
	exit := cmdLedgerSync(nil)
	if exit != 0 {
		t.Fatalf("first sync: exit %d, want 0", exit)
	}

	// Verify the ledger file was created.
	ledgerPath := filepath.Join(dir, "docs", "ledger", "verdicts.jsonl")
	records, err := ledger.Load(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}
	r := records[0]
	if r.SliceID != "S01-test-slice" {
		t.Errorf("SliceID: want S01-test-slice, got %s", r.SliceID)
	}
	if r.Verdict != "pass" {
		t.Errorf("Verdict: want pass, got %s", r.Verdict)
	}
	if r.SliceKind != "provider" {
		t.Errorf("SliceKind: want provider (T5-providers), got %s", r.SliceKind)
	}
	if r.GateCount != 4 {
		t.Errorf("GateCount: want 4, got %d", r.GateCount)
	}
	if r.Model != "claude-sonnet" {
		t.Errorf("Model: want claude-sonnet, got %s", r.Model)
	}
}

func TestSync_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	setupFixtureRelease(t, dir)
	chdirTemp(t, dir)

	// Run sync twice.
	if exit := cmdLedgerSync(nil); exit != 0 {
		t.Fatalf("first sync: exit %d", exit)
	}
	if exit := cmdLedgerSync(nil); exit != 0 {
		t.Fatalf("second sync: exit %d", exit)
	}

	// Should still be exactly 1 record.
	ledgerPath := filepath.Join(dir, "docs", "ledger", "verdicts.jsonl")
	records, err := ledger.Load(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Errorf("idempotent sync: want 1 record, got %d", len(records))
	}
}

func TestSync_SkipsPendingSlice(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a release with a pending slice (no terminal verdict).
	releaseDir := filepath.Join(dir, "docs", "release", "fixture-release")
	sliceDir := filepath.Join(releaseDir, "S02-pending-slice")
	if err := os.MkdirAll(sliceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	spec := "# Slice: S02-pending-slice\n\n- [ ] One check\n"
	if err := os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	st := state.Status{
		SliceID: "S02-pending-slice",
		Release: "fixture-release",
		Track:   "T8-memory",
		State:   state.InProgress,
		Verification: state.Verification{
			Result: "pending",
		},
	}
	data, _ := json.MarshalIndent(st, "", "  ")
	if err := os.WriteFile(filepath.Join(sliceDir, "status.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "docs", "ledger"), 0o755); err != nil {
		t.Fatal(err)
	}
	chdirTemp(t, dir)

	exit := cmdLedgerSync(nil)
	if exit != 0 {
		t.Fatalf("sync: exit %d", exit)
	}

	// No records should be written (pending).
	ledgerPath := filepath.Join(dir, "docs", "ledger", "verdicts.jsonl")
	records, err := ledger.Load(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Errorf("pending-only sync: want 0 records, got %d", len(records))
	}
}

func TestSync_GateCountFromSpec(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	releaseDir := filepath.Join(dir, "docs", "release", "fixture-release")
	sliceDir := filepath.Join(releaseDir, "S03-gate-slice")
	if err := os.MkdirAll(sliceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 7 acceptance checks.
	spec := "# Slice: S03-gate-slice\n\n- [ ] a\n- [ ] b\n- [ ] c\n- [ ] d\n- [ ] e\n- [ ] f\n- [ ] g\n"
	if err := os.WriteFile(filepath.Join(sliceDir, "spec.md"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	st := state.Status{
		SliceID: "S03-gate-slice",
		Release: "fixture-release",
		Track:   "T12-harness-hardening",
		State:   state.FailedVerification,
		Verification: state.Verification{
			Result: "fail",
			Model:  "gpt-5",
			Attempt: 2,
			Violations: []string{"unreachable test"},
		},
	}
	data, _ := json.MarshalIndent(st, "", "  ")
	if err := os.WriteFile(filepath.Join(sliceDir, "status.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "docs", "ledger"), 0o755); err != nil {
		t.Fatal(err)
	}
	chdirTemp(t, dir)

	exit := cmdLedgerSync(nil)
	if exit != 0 {
		t.Fatalf("sync: exit %d", exit)
	}

	ledgerPath := filepath.Join(dir, "docs", "ledger", "verdicts.jsonl")
	records, err := ledger.Load(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record, got %d", len(records))
	}
	if records[0].GateCount != 7 {
		t.Errorf("GateCount: want 7, got %d", records[0].GateCount)
	}
}

// ── Report integration test ──────────────────────────────────────────────

func TestReport_Integration(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	setupFixtureRelease(t, dir)
	chdirTemp(t, dir)

	// Sync first to populate the ledger.
	if exit := cmdLedgerSync(nil); exit != 0 {
		t.Fatalf("sync: exit %d", exit)
	}

	// Capture stdout by running report. We can't easily capture os.Stdout in a
	// test, so we verify the side effects: ledger file exists + has correct data.
	ledgerPath := filepath.Join(dir, "docs", "ledger", "verdicts.jsonl")
	records, err := ledger.Load(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("want 1 record after sync, got %d", len(records))
	}

	// Verify report doesn't error. We redirect stdout to avoid clutter.
	old := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	exit := cmdLedgerReport(nil)
	os.Stdout = old
	if exit != 0 {
		t.Fatalf("report: exit %d, want 0", exit)
	}
}

// ── No-subcommand usage ──────────────────────────────────────────────────

func TestLedgerNoSubcommand(t *testing.T) {
	// runLedger with no args prints usage and returns non-zero.
	// Redirect stderr to avoid clutter.
	old := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	exit := runLedger(nil)
	os.Stderr = old
	if exit == 0 {
		t.Error("no subcommand: expected non-zero exit")
	}
}

func TestLedgerBadSubcommand(t *testing.T) {
	old := os.Stderr
	os.Stderr, _ = os.Open(os.DevNull)
	exit := runLedger([]string{"nonexistent"})
	os.Stderr = old
	if exit == 0 {
		t.Error("bad subcommand: expected non-zero exit")
	}
}

// ── Usage message content ────────────────────────────────────────────────

func TestLedgerUsageMentionsSyncAndReport(t *testing.T) {
	// Verify the usage string names both sync and report.
	// We'll check by constructing the string manually from the source.
	c, ok := command.Lookup("ledger")
	if !ok {
		t.Fatal("ledger not registered")
	}

	// Exercise the no-subcommand path via runLedger directly, capturing stderr.
	// Use a pipe to capture.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	_ = runLedger(nil)
	w.Close()
	os.Stderr = old

	var buf strings.Builder
	data := make([]byte, 4096)
	for {
		n, err := r.Read(data)
		if n > 0 {
			buf.Write(data[:n])
		}
		if err != nil {
			break
		}
	}
	out := buf.String()
	for _, want := range []string{"sync", "report", "docs/ledger/verdicts.jsonl"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage output missing %q", want)
		}
	}

	_ = c // keep c used
}