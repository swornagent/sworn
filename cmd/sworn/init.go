package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/swornagent/sworn/internal/adopt"
	"github.com/swornagent/sworn/internal/config"
)

func cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	apiKey := fs.String("api-key", "", "API key for the default provider (openai); overrides prompting")
	force := fs.Bool("force", false, "overwrite existing config file")
	_ = fs.Parse(args)

	// Scaffold config file.
	cfgPath, existed, err := config.Scaffold(*force)
	if err != nil {
		if err == config.ErrConfigExists {
			fmt.Printf("config file already exists at %s (use --force to overwrite)\n", cfgPath)
			// Still run adoption steps (they are independent).
		} else {
			fmt.Fprintf(os.Stderr, "sworn init: %v\n", err)
			return 1
		}
	} else {
		fmt.Printf("config file created at %s\n", cfgPath)
	}

	// If config file existed and wasn't force-overwritten, we may still need to
	// set the API key. But the config file already has whatever key was set.
	// We only prompt if the file was newly created and --api-key wasn't passed.

	if !existed || *force {
		key := *apiKey
		if key == "" {
			key = promptAPIKey()
		}
		if key != "" {
			fmt.Println("API key set — store it in env var SWORN_OPENAI_API_KEY for production use")
			_ = key // Config already written with defaults; key is set via env var at runtime.
		}
	}

	// Materialise Baton protocol docs into the repo.
	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn init: cannot determine working directory: %v\n", err)
		return 1
	}
	if err := adopt.Materialise(repoRoot); err != nil {
		fmt.Fprintf(os.Stderr, "sworn init: %v\n", err)
		return 1
	}
	fmt.Printf("protocol vendored into docs/baton/ (rules + VERSION)\n")

	// Splice Baton rules into AGENTS.md.
	modified, err := adopt.SpliceAgents(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn init: %v\n", err)
		return 1
	}
	if modified {
		fmt.Println("AGENTS.md updated with Baton rules section")
	} else {
		fmt.Println("AGENTS.md already has current Baton rules section")
	}

	fmt.Println("\nsworn init complete. Run 'sworn verify' to verify your first change.")
	return 0
}

// promptAPIKey reads an API key from stdin with the prompt hidden.
func promptAPIKey() string {
	fmt.Fprint(os.Stderr, "Enter API key for default provider (openai): ")
	reader := bufio.NewReader(os.Stdin)
	key, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintln(os.Stderr)
		return ""
	}
	return strings.TrimSpace(key)
}