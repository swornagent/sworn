package model

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotEnv loads API keys from .env files into the process environment.
// It reads ~/.sworn/.env and a .env file in the current working directory.
//
// Load order: CWD .env first, then ~/.sworn/.env. Because os.Setenv is called
// only when the key is not already set in the environment, CWD files "win" on
// collision — local project keys override global user keys. This achieves the
// spec's stated contract ("CWD wins") while using the simpler
// set-only-if-unset guard rather than unconditional overwrite.
//
// Explicit environment variables (set before the process started) are never
// overwritten by .env file values.
//
// Skips blank lines, lines starting with '#', and malformed lines.
// Parses KEY=VALUE and KEY="VALUE" (with optional surrounding quotes).
// Returns nil on success. Missing .env files are not errors.
//
// Idempotent — calling LoadDotEnv multiple times has the same effect as
// calling it once (keys already set are not overwritten).
func LoadDotEnv() error {
	// CWD .env first so it "sticks" and home .env is skipped on collision.
	if wd, err := os.Getwd(); err == nil {
		loadFile(filepath.Join(wd, ".env"))
	}

	if home, err := os.UserHomeDir(); err == nil {
		loadFile(filepath.Join(home, ".sworn", ".env"))
	}
	return nil
}

// loadFile reads a single .env file and sets keys via os.Setenv.
// Errors are silently ignored — a missing .env file is not an error.
func loadFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // no .env file is fine
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines and comments.
		if line == "" || line[0] == '#' {
			continue
		}

		// Split on first '='.
		idx := strings.IndexByte(line, '=')
		if idx < 1 {
			continue // malformed (empty key)
		}

		key := strings.TrimSpace(line[:idx])
		rawVal := line[idx+1:]

		// Strip optional surrounding double quotes.
		val := strings.TrimSpace(rawVal)
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}

		// Only set if not already present in the environment.
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}