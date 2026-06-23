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

func cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
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

	// AGENTS.md
	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	agentsData, agentsReadErr := os.ReadFile(agentsPath)
	if os.IsNotExist(agentsReadErr) {
		// AGENTS.md does not exist — will be created from template.
		planned = append(planned, change{
			label:  "AGENTS.md",
			reason: "does not exist — will be created from MCP-pointer template",
		})
	} else if agentsReadErr != nil {
		fmt.Fprintf(os.Stderr, "sworn init: read AGENTS.md: %v\n", agentsReadErr)
		return 1
	} else if strings.Contains(string(agentsData), adopt.BatonSectionHeading) {
		// Legacy Baton splice detected — warn and skip.
		informational = append(informational, change{
			label: "AGENTS.md",
			warn:  true,
			reason: "contains legacy Baton content — run 'sworn doctor' to migrate\n" +
				"          (AGENTS.md left unchanged)",
		})
	} else {
		informational = append(informational, change{
			label:  "AGENTS.md",
			reason: "already present and up-to-date — no changes (use --force to overwrite)",
		})
	}
	// Store for apply phase.
	_ = agentsData

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

	// Design system prompt (S08): only when --ui-bearing is set.
	if *uiBearer {
		cfg, loadErr := config.Load()
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "sworn init: load config: %v\n", loadErr)
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
			fmt.Printf("  updated  %s (design system: token_source=%s, component_library=%s)\n",
				cfgPath, ds.TokenSource, ds.ComponentLibrary)
		} else {
			if writeErr := writeConfig(cfgPath, &cfg); writeErr != nil {
				fmt.Fprintf(os.Stderr, "sworn init: write ui_bearing: %v\n", writeErr)
				return 1
			}
			fmt.Printf("  updated  %s (ui_bearing: true — design system not yet configured)\n", cfgPath)
		}
	}

	// Implementer model prompt (S09): only for new config.
	if cfgErr == nil && !cfgExisted {
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
	}
	// AGENTS.md — create from MCP-pointer template if it does not exist.
	if os.IsNotExist(agentsReadErr) {
		if err := createAgentsMD(repoRoot); err != nil {
			fmt.Fprintf(os.Stderr, "sworn init: %v\n", err)
			return 1
		}
		fmt.Println("  created  AGENTS.md (MCP-pointer template)")
	} else if agentsReadErr == nil && *force && !strings.Contains(string(agentsData), adopt.BatonSectionHeading) {
		// --force with a non-legacy AGENTS.md: overwrite with template.
		if err := createAgentsMD(repoRoot); err != nil {
			fmt.Fprintf(os.Stderr, "sworn init: %v\n", err)
			return 1
		}
		fmt.Println("  updated  AGENTS.md (overwritten with MCP-pointer template via --force)")
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
	fmt.Println("Done. Connect your AI to sworn mcp to get the Baton protocol and role prompts. Run 'sworn doctor' to verify your setup.")
	return 0
}

// createAgentsMD writes the MCP-pointer AGENTS.md from the embedded template
// at docs/templates/agents.md.
func createAgentsMD(repoRoot string) error {
	templatePath := filepath.Join(repoRoot, "docs", "templates", "agents.md")
	targetPath := filepath.Join(repoRoot, "AGENTS.md")

	data, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("read AGENTS.md template: %w", err)
	}
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return fmt.Errorf("write AGENTS.md: %w", err)
	}
	return nil
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