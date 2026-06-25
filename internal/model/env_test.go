package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv_SetsUnsetKeys(t *testing.T) {
	// Create a temp .env file with two keys.
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	writeFile(t, envFile, "TEST_KEY_ONE=value1\nTEST_KEY_TWO=value2\n")

	// Pre-set TEST_KEY_ONE in the environment.
	t.Setenv("TEST_KEY_ONE", "already-set")

	// Change to the temp dir so CWD .env is the temp file.
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	LoadDotEnv()

	// TEST_KEY_ONE should be unchanged (already set).
	if got := os.Getenv("TEST_KEY_ONE"); got != "already-set" {
		t.Errorf("TEST_KEY_ONE = %q, want %q (should not overwrite already-set key)", got, "already-set")
	}

	// TEST_KEY_TWO should now be set.
	if got := os.Getenv("TEST_KEY_TWO"); got != "value2" {
		t.Errorf("TEST_KEY_TWO = %q, want %q", got, "value2")
	}
}

func TestLoadDotEnv_SkipComments(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	writeFile(t, envFile, "# this is a comment\n\n  # indented comment\nREAL_KEY=real_value\n")

	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	LoadDotEnv()

	if got := os.Getenv("REAL_KEY"); got != "real_value" {
		t.Errorf("REAL_KEY = %q, want %q", got, "real_value")
	}
	// Comments and blank lines should not create env vars.
	if got := os.Getenv("# this is a comment"); got != "" {
		t.Errorf("comment line became env var: %q", got)
	}
}

func TestLoadDotEnv_CWDWins(t *testing.T) {
	// Create a home .env and a CWD .env with the same key.
	homeDir := t.TempDir()
	cwdDir := t.TempDir()

	// Set up ~/.sworn/.env with home=global
	swornDir := filepath.Join(homeDir, ".sworn")
	os.MkdirAll(swornDir, 0755)
	writeFile(t, filepath.Join(swornDir, ".env"), "COLLISION_KEY=global\n")

	// Set up CWD .env with collision_key=local
	writeFile(t, filepath.Join(cwdDir, ".env"), "COLLISION_KEY=local\n")

	// Override HOME so LoadDotEnv finds the temp home dir.
	t.Setenv("HOME", homeDir)

	origWd, _ := os.Getwd()
	os.Chdir(cwdDir)
	defer os.Chdir(origWd)

	LoadDotEnv()

	// CWD should win — local key was set first and home skipped.
	if got := os.Getenv("COLLISION_KEY"); got != "local" {
		t.Errorf("COLLISION_KEY = %q, want %q (CWD should override home)", got, "local")
	}
}

func TestLoadDotEnv_QuotedValues(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	writeFile(t, envFile, "QUOTED_KEY=\"value with spaces\"\n")

	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	LoadDotEnv()

	if got := os.Getenv("QUOTED_KEY"); got != "value with spaces" {
		t.Errorf("QUOTED_KEY = %q, want %q", got, "value with spaces")
	}
}
func TestLoadDotEnv_Idempotent(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	writeFile(t, envFile, "IDEM_KEY=first\n")

	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	LoadDotEnv()
	if got := os.Getenv("IDEM_KEY"); got != "first" {
		t.Fatalf("first call: IDEM_KEY = %q, want %q", got, "first")
	}

	// Overwrite the file with a different value.
	writeFile(t, envFile, "IDEM_KEY=second\n")

	// Second call — key is already set, so should not change.
	LoadDotEnv()
	if got := os.Getenv("IDEM_KEY"); got != "first" {
		t.Errorf("second call: IDEM_KEY = %q, want %q (idempotent)", got, "first")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeFile %s: %v", path, err)
	}
}