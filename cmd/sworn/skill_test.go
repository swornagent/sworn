package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillInstallCommandUsesTheGivenIsolatedHome(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"skill", "install", "--home", home}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
	installed := filepath.Join(home, ".agents", "skills", "sworn", "SKILL.md")
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("sworn skill was not installed under the given home: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(installed)) {
		t.Fatalf("stdout = %q, want it to report the installed path", stdout.String())
	}
}

func TestSkillInstallCommandRejectsUnknownArguments(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"skill", "bogus"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run() = %d, want usage failure", code)
	}
}
