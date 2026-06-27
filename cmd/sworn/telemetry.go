package main

import (
	"flag"
	"fmt"
	"github.com/swornagent/sworn/internal/style"
	"github.com/swornagent/sworn/internal/supervisor"
	"github.com/swornagent/sworn/internal/telemetry"
	"os"
	"path/filepath"
)

// cmdTelemetry implements the "sworn telemetry" subcommand.
// Sub-subcommands: on, off, status, events.
func cmdTelemetry(args []string) int {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "usage: sworn telemetry on|off|status|events\n")
		return 64
	}

	switch args[0] {
	case "on":
		return telemetryOn()
	case "off":
		return telemetryOff()
	case "status":
		return telemetryStatus()
	case "events":
		return telemetryEvents(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "usage: sworn telemetry on|off|status|events\n")
		return 64
	}
}

func telemetryEvents(args []string) int {
	fs := flag.NewFlagSet("events", flag.ExitOnError)
	releaseName := fs.String("release", "", "release name (required, e.g. 2026-06-27-conformance-foundation)")
	_ = fs.Parse(args)

	if *releaseName == "" {
		fmt.Fprintf(os.Stderr, "telemetry events: --release is required\n")
		return 64
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry events: getwd: %v\n", err)
		return 1
	}

	db, err := supervisor.Open(*releaseName, wd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry events: open event store: %v\n", err)
		return 1
	}
	defer db.Close()

	events, err := supervisor.QueryEvents(db, *releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry events: query: %v\n", err)
		return 1
	}

	if len(events) == 0 {
		fmt.Fprintln(os.Stdout, "No events found for release", *releaseName)
		return 0
	}

	// Print events as a simple table.
	fmt.Fprintf(os.Stdout, "%-6s %-12s %-15s %-25s %s\n", "ID", "TRACK", "EVENT", "DETAIL", "TIMESTAMP")
	for _, e := range events {
		fmt.Fprintf(os.Stdout, "%-6d %-12s %-15s %-25s %s\n", e.ID, e.TrackID, e.Event, e.Detail, e.TS)
	}
	return 0
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
