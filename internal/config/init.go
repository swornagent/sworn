package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

	// Check existence first — this is the idempotency gate (Coach Pin 2).
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