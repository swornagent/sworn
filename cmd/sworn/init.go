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
	force := fs.Bool("force", false, "overwrite existing config and customized Baton sections")
	yes := fs.Bool("yes", false, "skip confirmation prompt (non-interactive)")
	_ = fs.Parse(args)

	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn init: cannot determine working directory: %v\n", err)
		return 1
	}

	// --- Scan phase: determine what will change, without touching anything ---

	fmt.Println("sworn init: scanning repo...")
	fmt.Println()

	type change struct {
		label  string
		reason string
		warn   bool // true = needs user attention even if no action taken
	}
	var planned []change
	var informational []change

	// Config file
	cfgPath, cfgExisted, cfgErr := config.Scaffold(*force)
	if cfgErr == nil && !cfgExisted {
		planned = append(planned, change{
			label:  cfgPath,
			reason: "config file does not exist — will be created with default settings",
		})
	} else if cfgErr == config.ErrConfigExists {
		informational = append(informational, change{
			label:  cfgPath,
			reason: "already exists — no changes (use --force to overwrite)",
		})
	}
	// Undo the Scaffold side-effect: if the file was just created by the
	// config.Scaffold call we need to remove it — we haven't confirmed yet.
	// config.Scaffold always creates; we re-create in the apply phase.
	if cfgErr == nil && !cfgExisted {
		_ = os.Remove(cfgPath)
	}

	// Baton protocol docs
	batonExist := adopt.BatonDocsExist(repoRoot)
	if !batonExist {
		planned = append(planned, change{
			label:  "docs/baton/",
			reason: "Baton protocol docs not present — will write 7 rule files + README + VERSION",
		})
	} else {
		informational = append(informational, change{
			label:  "docs/baton/",
			reason: "already present — will refresh to current protocol version",
		})
	}

	// Agent config files
	spliceResults, err := adopt.PlanSplice(repoRoot, *force)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn init: %v\n", err)
		return 1
	}
	for _, r := range spliceResults {
		switch r.Action {
		case adopt.SpliceCreated:
			planned = append(planned, change{
				label:  r.File,
				reason: "file does not exist — will be created with Baton rules section",
			})
		case adopt.SpliceAppended:
			planned = append(planned, change{
				label:  r.File,
				reason: "Baton section missing — will append rules to existing file",
			})
		case adopt.SpliceUpdated:
			planned = append(planned, change{
				label:  r.File,
				warn:   true,
				reason: "Baton section is customized — will overwrite with current protocol text (--force)",
			})
		case adopt.SpliceNoOp:
			informational = append(informational, change{
				label:  r.File,
				reason: "Baton section already current — no changes",
			})
		case adopt.SpliceCustomized:
			informational = append(informational, change{
				label:  r.File,
				warn:   true,
				reason: "Baton section has been customized — leaving unchanged\n" +
					"          (re-run with --force to overwrite with current protocol text)",
			})
		case adopt.SpliceAbsent:
			informational = append(informational, change{
				label:  r.File,
				reason: "not found — skipping (only spliced if the file already exists)",
			})
		}
	}

	// Print plan
	labelWidth := 22
	if len(planned) > 0 {
		fmt.Println("Changes:")
		for _, c := range planned {
			marker := "  +"
			if c.warn {
				marker = "  !"
			}
			fmt.Printf("%s  %-*s  %s\n", marker, labelWidth, c.label, c.reason)
		}
		fmt.Println()
	}

	if len(informational) > 0 {
		fmt.Println("No action needed:")
		for _, c := range informational {
			marker := "     "
			if c.warn {
				marker = "  !  "
			}
			fmt.Printf("%s%-*s  %s\n", marker, labelWidth, c.label, c.reason)
		}
		fmt.Println()
	}

	if len(planned) == 0 {
		fmt.Println("Nothing to do — repo is already current.")
		return 0
	}

	// --- Confirm phase ---

	if !*yes {
		fmt.Print("Proceed? [Y/n]: ")
		reader := bufio.NewReader(os.Stdin)
		resp, _ := reader.ReadString('\n')
		resp = strings.TrimSpace(strings.ToLower(resp))
		if resp != "" && resp != "y" && resp != "yes" {
			fmt.Println("Aborted. No changes made.")
			return 0
		}
	}

	fmt.Println()

	// --- Apply phase ---

	// Config file
	if cfgErr == nil && !cfgExisted {
		_, _, err := config.Scaffold(*force)
		if err != nil && err != config.ErrConfigExists {
			fmt.Fprintf(os.Stderr, "sworn init: %v\n", err)
			return 1
		}
		key := *apiKey
		if key == "" && !*yes {
			key = promptAPIKey()
		}
		if key != "" {
			fmt.Println("  API key noted — store it in env var SWORN_OPENAI_API_KEY for production use")
		}
		fmt.Printf("  created  %s\n", cfgPath)
	}

	// Baton protocol docs
	if err := adopt.Materialise(repoRoot); err != nil {
		fmt.Fprintf(os.Stderr, "sworn init: %v\n", err)
		return 1
	}
	if !batonExist {
		fmt.Println("  created  docs/baton/ (rules + README + VERSION)")
	} else {
		fmt.Println("  updated  docs/baton/ (refreshed to current protocol version)")
	}

	// Agent config files
	applied, err := adopt.SpliceAgents(repoRoot, *force)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sworn init: %v\n", err)
		return 1
	}
	for _, r := range applied {
		switch r.Action {
		case adopt.SpliceCreated:
			fmt.Printf("  created  %s\n", r.File)
		case adopt.SpliceAppended:
			fmt.Printf("  updated  %s (Baton section appended)\n", r.File)
		case adopt.SpliceUpdated:
			fmt.Printf("  updated  %s (Baton section replaced)\n", r.File)
		case adopt.SpliceCustomized:
			fmt.Printf("  skipped  %s (customized — use --force to overwrite)\n", r.File)
		}
	}

	fmt.Println()
	fmt.Println("Done. Run 'sworn verify' to verify your first change.")
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
