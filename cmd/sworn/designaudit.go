package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/swornagent/sworn/internal/config"
	"github.com/swornagent/sworn/internal/designaudit"
)

// cmdDesignaudit implements `sworn designaudit <project-dir>`.
//
// It runs the deterministic design-conformance first-pass (hardcoded hex,
// off-scale spacing, recreated components) against the declared design system
// (S08). When the deterministic pass is clean, a human cohesion verdict must be
// supplied via --cohesion to reach exit 0.
//
// Returns exit 0 on PASS, exit 1 on violations or missing cohesion verdict,
// exit 2 on unrecoverable error.
func cmdDesignaudit(args []string) int {
	fs := flag.NewFlagSet("designaudit", flag.ExitOnError)
	cohesion := fs.String("cohesion", "", "human cohesion verdict: on-brand|off-brand (required when deterministic pass is clean)")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "sworn designaudit: project directory is required")
		fmt.Fprintln(os.Stderr, "usage: sworn designaudit <project-dir> [--cohesion on-brand|off-brand]")
		return 64
	}

	projectDir := fs.Arg(0)

	// Load config. SWORN_CONFIG_PATH takes precedence; if not set, look for
	// config.json in the project directory before falling back to the standard path.
	cfg, err := loadDesignauditConfig(projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn designaudit: loading config: %v\n", err)
		return 2
	}

	report, err := designaudit.Run(projectDir, cfg, *cohesion)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn designaudit: %v\n", err)
		return 2
	}

	fmt.Print(designaudit.Print(report))
	fmt.Fprintln(os.Stderr, designaudit.PrintCompact(report))

	if report.Exempt {
		return 0
	}
	if report.HasViolations() || report.NeedsCohesionVerdict() {
		return 1
	}
	return 0
}

// loadDesignauditConfig loads sworn config for the given project directory.
// Resolution order:
//  1. $SWORN_CONFIG_PATH env var (standard override)
//  2. <projectDir>/config.json (project-local config)
//  3. Standard platform path (~/.config/sworn/config.json)
func loadDesignauditConfig(projectDir string) (config.Config, error) {
	// If SWORN_CONFIG_PATH is already set, config.Load() uses it.
	if os.Getenv("SWORN_CONFIG_PATH") != "" {
		return config.Load()
	}

	// Try project-local config.json first.
	projectCfgPath := projectDir + "/config.json"
	if _, err := os.Stat(projectCfgPath); err == nil {
		old := os.Getenv("SWORN_CONFIG_PATH")
		os.Setenv("SWORN_CONFIG_PATH", projectCfgPath)
		cfg, loadErr := config.Load()
		if old == "" {
			os.Unsetenv("SWORN_CONFIG_PATH")
		} else {
			os.Setenv("SWORN_CONFIG_PATH", old)
		}
		return cfg, loadErr
	}

	// Fall back to standard path.
	return config.Load()
}
