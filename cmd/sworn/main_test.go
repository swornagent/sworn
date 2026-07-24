package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/buildinfo"
)

func TestVersionJSON(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"version", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
	var info buildinfo.Info
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	if info.Version != "0.2.0-dev" {
		t.Fatalf("version = %q, want 0.2.0-dev", info.Version)
	}
}

func TestBoardUnavailableBeforeArgumentParsing(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"board"},
		{"board", "run-1"},
		{"board", "--store", ".baton/releases/demo/status.json", "--json"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 1 {
			t.Fatalf("run(%v) = %d, stderr = %q", args, code, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("run(%v) stdout = %q, want no output", args, stdout.String())
		}
		if got := stderr.String(); got != "sworn: board is unavailable while v0.3 delivery is in maintenance bootstrap\n" {
			t.Fatalf("run(%v) stderr = %q, want maintenance refusal", args, got)
		}
	}
}

func TestUnknownCommandFailsExplicitly(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"deliver"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "not implemented") {
		t.Fatalf("stderr = %q, want explicit not implemented error", stderr.String())
	}
}

func TestRunCommandUnavailableBeforeArgumentParsing(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"run"},
		{"run", "run-1"},
		{"run", "run-1", "--config", "relative.json"},
		{"run", "run-1", "--config", "/tmp/run.json", "--config", "/tmp/run.json"},
		{"run", "run-1", "--config", "/tmp/run.json", "--json", "--json"},
		{"run", "run-1", "work-1", "work-2", "--config", "/tmp/run.json"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 1 {
			t.Fatalf("run(%v) = %d, stderr = %q", args, code, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("run(%v) stdout = %q, want no output", args, stdout.String())
		}
		if got := stderr.String(); got != "sworn: run is unavailable while v0.3 delivery is in maintenance bootstrap\n" {
			t.Fatalf("run(%v) stderr = %q, want maintenance refusal", args, got)
		}
	}
}
