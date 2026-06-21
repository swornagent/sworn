package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/swornagent/sworn/internal/memory"
)

// cmdMemory dispatches the "sworn memory" command tree.
func cmdMemory(args []string) int {
	fs := flag.NewFlagSet("memory", flag.ExitOnError)
	_ = fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Fprint(os.Stderr, `sworn memory — SwornAgent's memory system configuration

usage:
  sworn memory status    show current memory configuration
  sworn memory build     build or update the memory index

See 'sworn memory <command> --help' for details.
`)
		return 64
	}

	switch fs.Arg(0) {
	case "status":
		return cmdMemoryStatus(fs.Args()[1:])
	case "build":
		return cmdMemoryBuild(fs.Args()[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown memory subcommand %q\n\n", fs.Arg(0))
		fmt.Fprint(os.Stderr, "usage: sworn memory [status|build]\n")
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

func cmdMemoryBuild(args []string) int {
	fs := flag.NewFlagSet("memory build", flag.ExitOnError)
	force := fs.Bool("force", false, "re-embed all entries regardless of change detection")
	_ = fs.Parse(args)

	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "usage: sworn memory build [--force]\n")
		return 64
	}

	start := time.Now()

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

	entries, err := memory.DiscoverEntries(cfg, cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: discovering entries: %v\n", err)
		return 1
	}

	idx, err := memory.OpenIndex(cfg.IndexPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: opening index: %v\n", err)
		return 1
	}
	defer idx.Close()

	ctx := context.Background()
	var toEmbed []memory.DiscoveredEntry
	var unchanged int

	for _, e := range entries {
		id := memory.ComputeID(e.Path, e.Content)
		if !*force {
			exists, err := idx.HasEntry(ctx, id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: checking entry existence: %v\n", err)
				return 1
			}
			if exists {
				unchanged++
				continue
			}
		}
		toEmbed = append(toEmbed, e)
	}

	if len(toEmbed) > 0 {
		embedder, err := memory.NewEmbedder(cfg.Embedding)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: creating embedder: %v\n", err)
			return 1
		}

		texts := make([]string, len(toEmbed))
		for i, e := range toEmbed {
			texts[i] = e.Content
		}

		embeddings, err := embedder.Embed(ctx, texts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: embedding entries: %v\n", err)
			return 1
		}

		for i, e := range toEmbed {
			entry := memory.Entry{
				ID:        memory.ComputeID(e.Path, e.Content),
				Path:      e.Path,
				Harness:   e.Harness,
				Title:     e.Title,
				Content:   e.Content,
				Embedding: embeddings[i],
				Model:     embedder.Model(),
				IndexedAt: time.Now().UTC(),
			}
			if err := idx.UpsertEntry(ctx, entry); err != nil {
				fmt.Fprintf(os.Stderr, "error: upserting entry: %v\n", err)
				return 1
			}
		}
	}

	duration := time.Since(start)
	fmt.Printf("Indexed %d entries (%d new, %d unchanged) via %s in %s\n",
		len(entries), len(toEmbed), unchanged, cfg.Embedding.Model, duration.Round(time.Millisecond))

	return 0
}