package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTUIOptionsDefaultsAndCleanAbsoluteOverrides(t *testing.T) {
	t.Parallel()

	defaults, ok := parseTUIOptions(nil)
	if !ok || defaults != (tuiOptions{}) {
		t.Fatalf("default options = %#v, ok = %t", defaults, ok)
	}

	root := t.TempDir()
	want := tuiOptions{
		projectPath: filepath.Join(root, "project"),
		journalPath: filepath.Join(root, "sworn.db"),
		configPath:  filepath.Join(root, "drivers.json"),
		manifestDir: filepath.Join(root, "runs"),
	}
	got, ok := parseTUIOptions([]string{
		"--config", want.configPath,
		"--project", want.projectPath,
		"--manifest-dir", want.manifestDir,
		"--journal", want.journalPath,
	})
	if !ok || got != want {
		t.Fatalf("override options = %#v, ok = %t, want %#v", got, ok, want)
	}
}

func TestParseTUIOptionsRejectsOpenOrAmbiguousShapes(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	clean := filepath.Join(root, "project")
	unclean := root + string(os.PathSeparator) + "nested" +
		string(os.PathSeparator) + ".." + string(os.PathSeparator) + "project"
	tests := []struct {
		name string
		args []string
	}{
		{name: "unknown", args: []string{"--other", clean}},
		{
			name: "duplicate",
			args: []string{"--project", clean, "--project", clean},
		},
		{name: "relative", args: []string{"--journal", "sworn.db"}},
		{name: "unclean", args: []string{"--config", unclean}},
		{name: "missing", args: []string{"--manifest-dir"}},
		{name: "option as value", args: []string{"--project", "--journal"}},
		{name: "empty", args: []string{"--project", ""}},
		{name: "NUL", args: []string{"--project", clean + "\x00other"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got, ok := parseTUIOptions(test.args); ok || got != (tuiOptions{}) {
				t.Fatalf("parseTUIOptions(%q) = %#v, %t", test.args, got, ok)
			}
		})
	}
}

func TestTerminalIsInteractiveRejectsNilAndNonTTYFiles(t *testing.T) {
	t.Parallel()

	if terminalIsInteractive(nil, nil) {
		t.Fatal("nil terminals reported interactive")
	}
	stdin, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close() })
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdout.Close() })

	for _, test := range []struct {
		name          string
		stdin, stdout *os.File
	}{
		{name: "nil stdin", stdin: nil, stdout: stdout},
		{name: "nil stdout", stdin: stdin, stdout: nil},
		{name: "regular files", stdin: stdin, stdout: stdout},
	} {
		t.Run(test.name, func(t *testing.T) {
			if terminalIsInteractive(test.stdin, test.stdout) {
				t.Fatal("non-TTY files reported interactive")
			}
		})
	}
}

func TestRunWithoutArgumentsAndUnknownCommandKeepCLIBehavior(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 0 ||
		stdout.String() != usage || stderr.Len() != 0 {
		t.Fatalf(
			"run(nil) = %d, stdout %q, stderr %q",
			code,
			stdout.String(),
			stderr.String(),
		)
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"not-a-command"}, &stdout, &stderr); code != 2 ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), `unknown command "not-a-command"`) ||
		!strings.Contains(stderr.String(), `Run "sworn help"`) {
		t.Fatalf(
			"unknown command = %d, stdout %q, stderr %q",
			code,
			stdout.String(),
			stderr.String(),
		)
	}
}

func TestExplicitTUIRejectsNonInteractiveProcess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"tui"}, &stdout, &stderr); code != 1 ||
		stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "interactive terminal is required") {
		t.Fatalf(
			"noninteractive tui = %d, stdout %q, stderr %q",
			code,
			stdout.String(),
			stderr.String(),
		)
	}
}
