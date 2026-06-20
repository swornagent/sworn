package main

import (
	"flag"
	"fmt"
	"github.com/swornagent/sworn/internal/memory"
	"os"
)

// cmdMemory dispatches the "sworn memory" command tree.
func cmdMemory(args []string) int {
	fs := flag.NewFlagSet("memory", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Fprint(os.Stderr, `sworn memory — SwornAgent's memory system configuration

usage:
  sworn memory status    show current memory configuration

See 'sworn memory status --help' for details.
`)
		return 64
	}

	switch fs.Arg(0) {
	case "status":
		return cmdMemoryStatus(fs.Args()[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown memory subcommand %q\n\n", fs.Arg(0))
		fmt.Fprint(os.Stderr, "usage: sworn memory status\n")
		return 64
	}
}

// cmdMemoryStatus prints the current memory configuration.
func cmdMemoryStatus(args []string) int {
	fs := flag.NewFlagSet("memory status", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "usage: sworn memory status\n")
		return 64
	}

	cfg, err := memory.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: loading memory config: %v\n", err)
		return 1
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: getting current directory: %v\n", err)
		return 1
	}

	harnesses := memory.ListHarnesses(cfg, cwd)

	// Determine if using defaults (no config files loaded).
	usingDefaults := len(cfg.LoadedPaths()) == 0

	if usingDefaults {
		fmt.Println("memory config: using defaults (no config file found)")
	} else {
		fmt.Println("memory config:")
		for _, p := range cfg.LoadedPaths() {
			fmt.Printf("  loaded: %s\n", p)
		}
	}

	fmt.Println()
	fmt.Println("Harnesses:")
	if len(harnesses) == 0 {
		fmt.Println("  (none configured)")
	} else {
		for _, h := range harnesses {
			status := "✓ exists"
			if !h.Exists {
				status = "✗ not found"
			}
			path := h.Path
			if path == "" {
				path = "(no native memory path)"
			}
			fmt.Printf("  %-15s %-8s %s\n", h.Name+":", status, path)
		}
	}

	fmt.Println()
	fmt.Println("Embedding:")
	fmt.Printf("  provider:  %s\n", cfg.Embedding.Provider)
	fmt.Printf("  model:     %s\n", cfg.Embedding.Model)
	fmt.Printf("  api key:   %s (%s)\n", cfg.Embedding.APIKeyEnv, apiKeyStatus(cfg.Embedding.APIKeyEnv))
	if cfg.Embedding.BaseURL != "" {
		fmt.Printf("  base url:  %s\n", cfg.Embedding.BaseURL)
	}
	fmt.Println()
	fmt.Printf("Index path: %s\n", cfg.IndexPath)

	return 0
}

// apiKeyStatus returns "<set>" or "<not set>" for the env var named by key.
// The resolved value is never printed or logged.
func apiKeyStatus(key string) string {
	if key == "" {
		return "<not set>"
	}
	if os.Getenv(key) != "" {
		return "<set>"
	}
	return "<not set>"
}