package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/adopt"
	"github.com/swornagent/sworn/internal/config"
)

func cmdInit(args []string) int {	fs := flag.NewFlagSet("init", flag.ExitOnError)
	apiKey := fs.String("api-key", "", "API key for the default provider (openai); overrides prompting")
	force := fs.Bool("force", false, "overwrite existing config and customized Baton sections")
	yes := fs.Bool("yes", false, "skip confirmation prompt (non-interactive)")
	uiBearer := fs.Bool("ui-bearing", false, "mark project as UI-bearing (requires design system declaration)")
	_ = fs.Parse(args)

	// Shared stdin reader — avoids multiple bufio.NewReader(os.Stdin)
	// instances fighting over buffered pipe/test data.
	in := bufio.NewReader(os.Stdin)

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

	// Design system declaration (S08) — check current project config.
	existingCfg, _ := config.Load()
	if existingCfg.UIBearing && existingCfg.DesignSystem == nil {
		informational = append(informational, change{
			label:  "design_system",
			warn:   true,
			reason: "ui_bearing is true but no design_system declared — run with --ui-bearing to configure",
		})
	} else if !existingCfg.UIBearing && !*yes {
		informational = append(informational, change{
			label:  "design_system",
			reason: "project is not UI-bearing (use --ui-bearing to declare design system)",
		})
	}

	// Agent config files
	spliceResults, err := adopt.PlanSplice(repoRoot, *force)
	if err != nil {		fmt.Fprintf(os.Stderr, "sworn init: %v\n", err)
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
		resp, _ := in.ReadString('\n')
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

	// Design system prompt (S08): ask about UI-bearing and design system.
	// This runs after the config file exists so we can re-load and modify it.
	if cfgErr == nil && !cfgExisted {
		ds, err := config.PromptDesignSystem(existingCfg.DesignSystem, *yes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sworn init: design system prompt: %v\n", err)
			return 1
		}
		if ds != nil {
			// Re-load the config (just created by Scaffold) and add design system.
			cfg, loadErr := config.Load()
			if loadErr != nil {
				fmt.Fprintf(os.Stderr, "sworn init: re-load config: %v\n", loadErr)
				return 1
			}
			cfg.UIBearing = *uiBearer || true
			cfg.DesignSystem = ds
			if writeErr := writeConfig(cfgPath, &cfg); writeErr != nil {
				fmt.Fprintf(os.Stderr, "sworn init: write design system: %v\n", writeErr)
				return 1
			}
			fmt.Printf("  updated  %s (design system: token_source=%s, component_library=%s)\n",
				cfgPath, ds.TokenSource, ds.ComponentLibrary)
		} else if *uiBearer {
			// User explicitly wants UI-bearing but didn't provide design system.
			cfg, loadErr := config.Load()
			if loadErr != nil {
				fmt.Fprintf(os.Stderr, "sworn init: re-load config: %v\n", loadErr)
				return 1
			}
			cfg.UIBearing = true
			if writeErr := writeConfig(cfgPath, &cfg); writeErr != nil {
				fmt.Fprintf(os.Stderr, "sworn init: write ui_bearing: %v\n", writeErr)
				return 1
			}
			fmt.Printf("  updated  %s (ui_bearing: true — design system not yet configured; run 'sworn init --ui-bearing --force' to configure)\n", cfgPath)
		}

		// Implementer model prompt (S09): collect implementer model + escalation + retry cap.
		// When --yes is set, defaults are used without prompting (Coach Pin 2).
		cfg, loadErr := config.Load()
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "sworn init: re-load config: %v\n", loadErr)
			return 1
		}
		impl := config.PromptImplementer(cfg.Implementer, *yes)
		cfg.Implementer = impl
		if writeErr := writeConfig(cfgPath, &cfg); writeErr != nil {
			fmt.Fprintf(os.Stderr, "sworn init: write implementer config: %v\n", writeErr)
			return 1
		}
		fmt.Printf("  updated  %s (implementer: model=%s, escalation_models=%v, max_attempts=%d)\n",
			cfgPath, impl.Model, impl.EscalationModels, impl.MaxAttempts)
	} else if cfgErr == config.ErrConfigExists && *uiBearer {		// Config exists; update it with UI-bearing / design system.
		cfg, loadErr := config.Load()
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "sworn init: load existing config: %v\n", loadErr)
			return 1
		}
		ds, err := config.PromptDesignSystem(cfg.DesignSystem, *yes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sworn init: design system prompt: %v\n", err)
			return 1
		}
		cfg.UIBearing = true
		if ds != nil {
			cfg.DesignSystem = ds
			if writeErr := writeConfig(cfgPath, &cfg); writeErr != nil {
				fmt.Fprintf(os.Stderr, "sworn init: write design system: %v\n", writeErr)
				return 1
			}
			fmt.Printf("  updated  %s (ui_bearing: true, design_system configured)\n", cfgPath)
		} else {
			if writeErr := writeConfig(cfgPath, &cfg); writeErr != nil {
				fmt.Fprintf(os.Stderr, "sworn init: write ui_bearing: %v\n", writeErr)
				return 1
			}
			fmt.Printf("  updated  %s (ui_bearing: true — design system not yet configured)\n", cfgPath)
		}
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

	// --- Consideration catalog prompt ---
	// After the implementer-model prompt, offer to scaffold the consideration
	// catalog (docs/considerations.md) and decision registry (docs/decisions.md).
	// These are plain markdown templates — no template engine, no interpolation.
	if !*yes {
		fmt.Print("Set up consideration catalog? (y/n) [y]: ")
		resp, _ := in.ReadString('\n')
		resp = strings.TrimSpace(strings.ToLower(resp))
			if resp == "n" || resp == "no" {
			fmt.Println("  skipped  catalog — run 'sworn induction' later to set it up")
			goto done
		}
	}
	if err := materialiseCatalog(repoRoot, in); err != nil {
			fmt.Fprintf(os.Stderr, "sworn init: catalog: %v\n", err)
		return 1
	}

done:
	fmt.Println()
	fmt.Println("Done. Run 'sworn verify' to verify your first change.")
	return 0
}

// materialiseCatalog copies the consideration catalog and decision registry
// templates from docs/templates/ into the project root. If either target file
// already exists, it prompts before overwriting (defaulting to no).
func materialiseCatalog(repoRoot string, in *bufio.Reader) error {
	templates := []struct {
		src  string
		dst  string
		name string
	}{
		{"docs/templates/considerations.md", "docs/considerations.md", "consideration catalog"},
		{"docs/templates/decisions.md", "docs/decisions.md", "decision registry"},
	}

	for _, t := range templates {
		srcPath := filepath.Join(repoRoot, t.src)
		dstPath := filepath.Join(repoRoot, t.dst)

		// Ensure destination directory exists.
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return fmt.Errorf("create docs/: %w", err)
		}

		// Check if destination exists — prompt before overwriting.
		if _, err := os.Stat(dstPath); err == nil {
			fmt.Printf("  File exists — overwrite %s? [y/N]: ", t.name)
			resp, _ := in.ReadString('\n')
			resp = strings.TrimSpace(strings.ToLower(resp))
			if resp != "y" && resp != "yes" {
				fmt.Printf("  skipped  %s (already exists)\n", t.name)
				continue
			}
		}

		data, err := os.ReadFile(srcPath)
			if err != nil {
			return fmt.Errorf("read template %s: %w", t.src, err)
		}
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			return fmt.Errorf("write %s: %w", t.name, err)
		}
		fmt.Printf("  created  %s\n", t.dst)
	}

	return nil
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

// writeConfig marshals cfg and writes it to path with mode 0600.
func writeConfig(path string, cfg *config.Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0600)
}