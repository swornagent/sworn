package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)
// ErrConfigExists is returned by Scaffold when the config file already exists
// and force is false.
var ErrConfigExists = errors.New("config file already exists")

// Scaffold writes a default config file at the standard path (see Path). If the
// file already exists and force is false, it returns ErrConfigExists — the CLI
// can then print a friendly message and exit 0 (idempotent). If force is true,
// the existing file is overwritten.
//
// The config file is written with mode 0600 (owner read/write only) because it
// may contain an API key.
func Scaffold(force bool) (path string, existed bool, err error) {
	p := Path()
	if p == "" {
		return "", false, fmt.Errorf("config: cannot determine home directory; set $SWORN_CONFIG_PATH")
	}

	// Check existence first — this is the idempotency gate.
	if _, statErr := os.Stat(p); statErr == nil {
		if !force {
			return p, true, ErrConfigExists
		}
		// force: overwrite below
	} else if !os.IsNotExist(statErr) {
		return "", false, fmt.Errorf("config: stat %s: %w", p, statErr)
	}

	cfg := DefaultConfig()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", false, fmt.Errorf("config: marshal default: %w", err)
	}
	// Append newline for human-readability.
	data = append(data, '\n')

	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", false, fmt.Errorf("config: mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(p, data, 0600); err != nil {
		return "", false, fmt.Errorf("config: write %s: %w", p, err)
	}
	return p, false, nil
}

// PromptDesignSystem returns a DesignSystem populated from interactive prompts
// (or defaults, in non-interactive mode). When the project is UI-bearing and
// no existing DesignSystem is set, the caller should use this to collect the
// declaration. Returns nil when nonInteractive is true and there is no current
// DesignSystem, indicating the user chose not to configure one now.
//
// In interactive mode, the prompts ask for:
//   - The design tokens source (e.g. "tokens.json", "design/tokens/")
//   - The component library location (e.g. "packages/ui", "src/components")
func PromptDesignSystem(current *DesignSystem, nonInteractive bool) (*DesignSystem, error) {
	if nonInteractive {
		if current != nil {
			return current, nil
		}
		return nil, nil
	}

	ds := &DesignSystem{}
	if current != nil {
		ds.TokenSource = current.TokenSource
		ds.ComponentLibrary = current.ComponentLibrary
	}

	fmt.Println()
	fmt.Println("Design system configuration (for UI-bearing projects):")
	fmt.Println()

	if ds.TokenSource == "" {
		fmt.Print("  Design tokens source (e.g. tokens.json): ")
		var tokenSrc string
		if _, err := fmt.Scanln(&tokenSrc); err != nil {
			// User entered nothing or non-interactive; leave empty.
			tokenSrc = ""
		}
		ds.TokenSource = tokenSrc
	}

	if ds.ComponentLibrary == "" {
		fmt.Print("  Component library location (e.g. packages/ui): ")
		var compLib string
		if _, err := fmt.Scanln(&compLib); err != nil {
			compLib = ""
		}
		ds.ComponentLibrary = compLib
	}

	// If both are still empty after prompting, the user declined to provide them.
	if ds.TokenSource == "" && ds.ComponentLibrary == "" {
		return nil, nil
	}

	return ds, nil
}
// PromptImplementer collects implementer model settings interactively. When
// nonInteractive is true, it returns the defaults without prompting.
//
// In interactive mode, the prompts show the default for each field and accept
// input. Empty input accepts the default.
func PromptImplementer(current ModelSetting, nonInteractive bool) ModelSetting {
	ms := ModelSetting{
		Model:            current.Model,
		EscalationModels: current.EscalationModels,
		MaxAttempts:      current.MaxAttempts,
	}
	if nonInteractive {
		return ms
	}

	fmt.Println()
	fmt.Println("Implementer model configuration:")
	fmt.Println()

	// Model
	fmt.Printf("  Model (provider/model) [%s]: ", ms.Model)
	var input string
	if _, err := fmt.Scanln(&input); err == nil && input != "" {
		ms.Model = input
	}

	// Escalation models
	defaultEsc := strings.Join(ms.EscalationModels, ", ")
	fmt.Printf("  Escalation models (comma-separated) [%s]: ", defaultEsc)
	input = ""
	// Scanln only reads one token; use a full line read for comma-separated input.
	var buf []byte
	var ch byte
	for {
		n, _ := fmt.Scanf("%c", &ch)
		if n == 0 || ch == '\n' {
			break
		}
		buf = append(buf, ch)
	}
	input = strings.TrimSpace(string(buf))
	if input != "" {
		var models []string
		for _, m := range strings.Split(input, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				models = append(models, m)
			}
		}
		if len(models) > 0 {
			ms.EscalationModels = models
		}
	}

	// Max attempts
	fmt.Printf("  Max attempts [%d]: ", ms.MaxAttempts)
	var n int
	if _, err := fmt.Scanln(&n); err == nil {
		ms.MaxAttempts = n
	}

	fmt.Println()
	return ms
}
