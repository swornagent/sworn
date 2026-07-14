// Package credentials owns the shared on-disk credential envelope.
//
// Provider keys, account sessions, and notification settings share one file,
// but each consumer owns only its own top-level fields. Updates therefore
// preserve fields they do not understand instead of rewriting the whole file.
package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// PathEnv overrides the credential file location outright.
const PathEnv = "SWORN_CREDENTIALS_PATH"

// Path returns the single platform-appropriate credential file path.
func Path() string {
	if path := os.Getenv(PathEnv); path != "" {
		return path
	}
	if dir := os.Getenv("SWORN_HOME"); dir != "" {
		return filepath.Join(dir, "credentials.json")
	}
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		return ""
	}
	return filepath.Join(base, "sworn", "credentials.json")
}

// Dir returns the directory containing Path.
func Dir() string {
	path := Path()
	if path == "" {
		return ""
	}
	return filepath.Dir(path)
}

// PathIn returns the shared credential filename within an explicit directory.
// It supports legacy/test callers without duplicating filename ownership.
func PathIn(dir string) string {
	return filepath.Join(dir, "credentials.json")
}

// UpdateAt atomically applies a field-preserving update to a credential file.
// A malformed existing file is an error: overwriting it would destroy secrets
// the caller cannot recover.
func UpdateAt(path string, update func(map[string]json.RawMessage) error) error {
	if path == "" {
		return os.ErrNotExist
	}

	fields := make(map[string]json.RawMessage)
	raw, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(raw, &fields); err != nil {
			return fmt.Errorf("parse credentials envelope: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read credentials envelope: %w", err)
	}

	if err := update(fields); err != nil {
		return err
	}
	if len(fields) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove empty credentials envelope: %w", err)
		}
		return nil
	}

	data, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials envelope: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create credentials directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".credentials-*.tmp")
	if err != nil {
		return fmt.Errorf("create credentials temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("secure credentials temporary file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write credentials temporary file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync credentials temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close credentials temporary file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace credentials envelope: %w", err)
	}
	return nil
}

// SetJSONAt marshals and sets one top-level field while preserving all others.
func SetJSONAt(path, name string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal credentials field %q: %w", name, err)
	}
	return UpdateAt(path, func(fields map[string]json.RawMessage) error {
		fields[name] = raw
		return nil
	})
}

// DeleteAt removes only the named fields, preserving other credential domains.
func DeleteAt(path string, names ...string) error {
	return UpdateAt(path, func(fields map[string]json.RawMessage) error {
		for _, name := range names {
			delete(fields, name)
		}
		return nil
	})
}
