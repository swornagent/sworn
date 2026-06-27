package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/db"
	"github.com/swornagent/sworn/internal/style"
	"github.com/swornagent/sworn/internal/supervisor"
	"github.com/swornagent/sworn/internal/telemetry"
)

// cmdTelemetry implements the "sworn telemetry" subcommand.
// Sub-subcommands: on, off, status, decisions.
func cmdTelemetry(args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "usage: sworn telemetry on|off|status|decisions\n")
		return 64
	}

	switch args[0] {
	case "on":
		return telemetryOn()
	case "off":
		return telemetryOff()
	case "status":
		return telemetryStatus()
	case "decisions":
		return telemetryDecisions(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "usage: sworn telemetry on|off|status|decisions\n")
		return 64
	}
}

// telemetryDecisions outputs the decision log for a release.
// Usage: sworn telemetry decisions --release <name>
func telemetryDecisions(args []string) int {
	fs := flag.NewFlagSet("decisions", flag.ExitOnError)
	release := fs.String("release", "", "release name (required)")
	_ = fs.Parse(args)

	if *release == "" {
		fmt.Fprintf(os.Stderr, "usage: sworn telemetry decisions --release <name>\n")
		return 64
	}

	db, err := openTelemetryDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry decisions: open database: %v\n", err)
		return 1
	}
	defer db.Close()

	rows, err := supervisor.QueryDecisions(db, *release)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry decisions: query: %v\n", err)
		return 1
	}

	if len(rows) == 0 {
		fmt.Printf("No decisions recorded for release %q.\n", *release)
		return 0
	}

	// Human-readable table.
	printDecisionsTable(rows)
	return 0
}

// openTelemetryDB opens the default sworn SQLite database (read-only path).
func openTelemetryDB() (*sql.DB, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}
	dbPath := wd + "/.sworn/sworn.db"
	return db.Open(dbPath)
}

// printDecisionsTable outputs a human-readable table of decision rows.
func printDecisionsTable(rows []supervisor.DecisionRow) {
	// Column headers.
	fmt.Printf("%-6s %-12s %-10s %-18s %s\n",
		"ID", "SLICE", "ROLE", "ACTION", "REASON")
	fmt.Println(strings.Repeat("-", 100))

	for _, r := range rows {
		reason := r.Reason
		if len(reason) > 55 {
			reason = reason[:52] + "..."
		}
		fmt.Printf("%-6d %-12s %-10s %-18s %s\n",
			r.ID, truncate(r.SliceID, 12), r.Role, truncate(r.Action, 18), reason)
	}

	fmt.Printf("\n%d row(s) for release %q.\n", len(rows), rows[0].Release)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func telemetryOn() int {
	dir, err := telemetry.ConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: cannot determine config directory: %v\n", err)
		return 1
	}

	// Create .telemetry-enabled.
	enabledPath := filepath.Join(dir, ".telemetry-enabled")
	if err := os.MkdirAll(dir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: cannot create config directory: %v\n", err)
		return 1
	}
	if err := os.WriteFile(enabledPath, []byte{}, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: cannot write telemetry-enabled sentinel: %v\n", err)
		return 1
	}

	// Remove .no-telemetry if present.
	noTelemetryPath := filepath.Join(dir, ".no-telemetry")
	os.Remove(noTelemetryPath) // best-effort

	fmt.Fprintln(os.Stderr, "telemetry: enabled")
	return 0
}

func telemetryOff() int {
	dir, err := telemetry.ConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: cannot determine config directory: %v\n", err)
		return 1
	}

	// Create .no-telemetry.
	noTelemetryPath := filepath.Join(dir, ".no-telemetry")
	if err := os.MkdirAll(dir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: cannot create config directory: %v\n", err)
		return 1
	}
	if err := os.WriteFile(noTelemetryPath, []byte{}, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: cannot write no-telemetry sentinel: %v\n", err)
		return 1
	}

	// Remove .telemetry-enabled if present.
	enabledPath := filepath.Join(dir, ".telemetry-enabled")
	os.Remove(enabledPath) // best-effort

	fmt.Fprintln(os.Stderr, "telemetry: disabled")
	return 0
}

func telemetryStatus() int {
	dir, err := telemetry.ConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: cannot determine config directory: %v\n", err)
		return 1
	}

	if os.Getenv("SWORN_NO_TELEMETRY") == "1" {
		fmt.Fprintln(os.Stdout, style.Dim("telemetry: disabled (SWORN_NO_TELEMETRY env var)"))
		return 0
	}

	enabledPath := filepath.Join(dir, ".telemetry-enabled")
	noTelemetryPath := filepath.Join(dir, ".no-telemetry")

	_, enabledErr := os.Stat(enabledPath)
	_, noTelErr := os.Stat(noTelemetryPath)
	enabledExists := enabledErr == nil
	noTelemetryExists := noTelErr == nil
	if noTelemetryExists {
		fmt.Fprintln(os.Stdout, style.Dim("telemetry: disabled (opted out)"))
	} else if enabledExists {
		fmt.Fprintln(os.Stdout, style.Success("telemetry: enabled"))
	} else {
		fmt.Fprintln(os.Stdout, style.Dim("telemetry: disabled (init not run)"))
	}

	return 0
}