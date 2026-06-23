package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/bench"
	"github.com/swornagent/sworn/internal/style"
)

// cmdBench implements `sworn bench` and `sworn bench overclaim`.
//
//	sworn bench [--task-set <dir>] [--models <comma-sep>] [--output <dir>]
//	sworn bench overclaim [--publish]
//
// The model benchmark runs each model against each task (spec + known-good diff)
// and produces a table to stdout and a JSON report to the output directory.
//
// The overclaim benchmark runs a deterministic 12-slice fixture through the
// concurrent scheduler at N=1, 2, 4 and reports overclaim/underclaim rates.
// With --publish, writes the Markdown report to docs/benchmark/.
//
// Model IDs use the "provider/model" format (e.g. "openai/gpt-4.1"). The
// benchmark constructs OAI clients directly with explicit model IDs, bypassing
// model.FromEnv's SWORN_OPENAI_MODEL override (Pin 3).
func cmdBench(args []string) int {
	// Dispatch to overclaim subcommand if first arg is "overclaim".
	if len(args) > 0 && args[0] == "overclaim" {
		return cmdBenchOverclaim(args[1:])
	}
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	taskSet := fs.String("task-set", "", "path to release directory containing slice specs (required)")
	modelsFlag := fs.String("models", defaultModelsCSV(), "comma-separated model IDs (provider/model)")
	outputDir := fs.String("output", "docs/benchmark", "output directory for JSON report")
	_ = fs.Parse(args)
	if *taskSet == "" {
		fmt.Fprintln(os.Stderr, "sworn bench: --task-set is required")
		return 64
	}

	apiKey := os.Getenv("SWORN_OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "sworn bench: SWORN_OPENAI_API_KEY not set")
		return 2
	}

	// Parse model list.
	var modelEntries []bench.ModelEntry
	for _, mid := range strings.Split(*modelsFlag, ",") {
		mid = strings.TrimSpace(mid)
		if mid == "" {
			continue
		}
		provider, model, err := parseBenchModelID(mid)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sworn bench: %v\n", err)
			return 2
		}
		modelEntries = append(modelEntries, bench.ModelEntry{
			ModelID:  mid,
			Provider: provider,
			Model:    model,
		})
	}
	if len(modelEntries) == 0 {
		fmt.Fprintln(os.Stderr, "sworn bench: no models specified")
		return 2
	}

	// Resolve task set.
	absTaskSet, err := filepath.Abs(*taskSet)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn bench: resolve task-set: %v\n", err)
		return 2
	}

	// Create a temp dir for generated diff files.
	tmpDir, err := os.MkdirTemp("", "sworn-bench-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn bench: create temp dir: %v\n", err)
		return 2
	}
	defer os.RemoveAll(tmpDir)

	tasks, err := bench.ResolveTaskSet(absTaskSet, tmpDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn bench: resolve task set: %v\n", err)
		return 2
	}
	if len(tasks) == 0 {
		fmt.Fprintln(os.Stderr, "sworn bench: no slice specs found in task set")
		return 2
	}

	fmt.Fprintf(os.Stderr, "sworn bench: %d models × %d tasks = %d cells\n",
		len(modelEntries), len(tasks), len(modelEntries)*len(tasks))

	// Run benchmark.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	start := time.Now()
	cells, err := bench.Run(ctx, modelEntries, tasks, apiKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn bench: %v\n", err)
		return 2
	}
	elapsed := time.Since(start)

	// Build report.
	taskNames := make([]string, len(tasks))
	for i, t := range tasks {
		taskNames[i] = t.Name
	}
	report := &bench.Report{
		Models: modelEntries,
		Tasks:  taskNames,
		Cells:  cells,
	}

	// Print table.
	fmt.Println(bench.Table(report))

	// Select default model.
	def, err := bench.SelectDefault(modelEntries, cells, taskNames)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn bench: select default: %v\n", err)
	} else {
		fmt.Println(style.Success(fmt.Sprintf("\nSafe-hosted default model: %s", def))+"\n")
	}

	fmt.Printf("Benchmark completed in %s (%d cells).\n", elapsed.Round(time.Millisecond), len(cells))

	// Write JSON report.
	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "sworn bench: create output dir: %v\n", err)
		return 2
	}
	jsonOut, err := bench.JSONReport(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn bench: generate JSON: %v\n", err)
		return 2
	}
	reportPath := filepath.Join(*outputDir, "benchmark-report.json")
	if err := os.WriteFile(reportPath, []byte(jsonOut), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "sworn bench: write report: %v\n", err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "sworn bench: JSON report written to %s\n", reportPath)

	// Write Markdown report.
	mdOut := markdownReport(report, def, elapsed)
	mdPath := filepath.Join(*outputDir, "benchmark-report.md")
	if err := os.WriteFile(mdPath, []byte(mdOut), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "sworn bench: write markdown report: %v\n", err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "sworn bench: Markdown report written to %s\n", mdPath)

	return 0
}

// defaultModelsCSV returns the default benchmark model list (Pin 5: 8 OpenAI
// models approved by Coach).
func defaultModelsCSV() string {
	return strings.Join([]string{
		"openai/gpt-4.1",
		"openai/gpt-4.1-mini",
		"openai/gpt-4.1-nano",
		"openai/gpt-4o",
		"openai/gpt-4o-mini",
		"openai/o4-mini",
		"openai/o3",
		"openai/o3-mini",
	}, ",")
}

// parseBenchModelID splits "provider/model" for benchmarking. Unlike the
// model package's parseModelID, this does not use env-override logic.
func parseBenchModelID(modelID string) (provider, model string, err error) {
	idx := strings.IndexByte(modelID, '/')
	if idx < 0 {
		return "", "", fmt.Errorf("invalid model ID %q (want provider/model)", modelID)
	}
	provider = modelID[:idx]
	model = modelID[idx+1:]
	if provider == "" || model == "" {
		return "", "", fmt.Errorf("invalid model ID %q (provider and model required)", modelID)
	}
	return provider, model, nil
}

// markdownReport generates a Markdown report for the benchmark.
func markdownReport(report *bench.Report, defaultModel string, elapsed time.Duration) string {
	var b strings.Builder
	b.WriteString("# SwornAgent Benchmark Report\n\n")
	b.WriteString("**Generated:** " + time.Now().UTC().Format(time.RFC3339) + "\n\n")
	b.WriteString("## Summary\n\n")
	b.WriteString(fmt.Sprintf("- **Models tested:** %d\n", len(report.Models)))
	b.WriteString(fmt.Sprintf("- **Tasks:** %d (S01–S09 slice specs with known-good diffs)\n", len(report.Tasks)))
	b.WriteString(fmt.Sprintf("- **Cells:** %d\n", len(report.Cells)))
	b.WriteString(fmt.Sprintf("- **Total time:** %s\n", elapsed.Round(time.Second)))
	if defaultModel != "" {
		b.WriteString(fmt.Sprintf("- **Safe-hosted default:** `%s`\n", defaultModel))
	}
	b.WriteString("\n## Notes\n\n")
	b.WriteString("- **Diff strategy:** known-good diffs (trivial comment addition to each spec). A PASS means the model correctly identified the change as non-violating.\n")
	b.WriteString("- **Single attempt** per model × task (first-pass success rate).\n")
	b.WriteString("- **Non-determinism:** model responses are inherently non-deterministic; re-running the benchmark may produce different pass-rates.\n")
	b.WriteString("- **Partial failure:** if a model errors (API failure, timeout), the cell is marked ERR and excluded from pass-rate calculation.\n")
	b.WriteString("- **Safe-hosted filter:** only models with provider `openai` + standard base URL are eligible for default selection (AC2).\n")
	b.WriteString("\n## Results Table\n\n")
	b.WriteString("```\n")
	b.WriteString(bench.Table(report))
	b.WriteString("\n```\n")
	return b.String()
}

// cmdBenchOverclaim implements `sworn bench overclaim [--publish]`.
//
// Runs the overclaim benchmark: a deterministic 12-slice fixture (8 PASS,
// 4 FAIL) through the concurrent scheduler at N=1, 2, 4 concurrent tracks.
// Reports overclaim and underclaim rates. With --publish, writes the
// Markdown report to docs/benchmark/overclaim-concurrent-1to4.md.
//
// No live API calls — all mock. The benchmark tests scheduler+gate
// correctness, not model quality.
func cmdBenchOverclaim(args []string) int {
	fs := flag.NewFlagSet("bench overclaim", flag.ExitOnError)
	publish := fs.Bool("publish", false, "write the Markdown report to docs/benchmark/overclaim-concurrent-1to4.md")
	_ = fs.Parse(args)

	report, err := bench.RunOverclaimBenchmark()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn bench overclaim: %v\n", err)
		return 2
	}

	// Print Markdown table to stdout.
	md := bench.FormatMarkdownTable(report)
	fmt.Print(md)

	// Print JSON to stderr for machine consumption.
	jsonOut, err := bench.FormatJSON(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn bench overclaim: format JSON: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "\nJSON:\n%s\n", jsonOut)
	}

	if *publish {
		outPath := filepath.Join("docs", "benchmark", "overclaim-concurrent-1to4.md")
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "sworn bench overclaim: create output dir: %v\n", err)
			return 2
		}
		if err := os.WriteFile(outPath, []byte(md), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "sworn bench overclaim: write report: %v\n", err)
			return 2
		}
		fmt.Fprintf(os.Stderr, "sworn bench overclaim: report written to %s\n", outPath)
	}

	return 0
}
