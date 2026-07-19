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
	if info.Version != "1.0.0-dev" {
		t.Fatalf("version = %q, want 1.0.0-dev", info.Version)
	}
}

func TestUnknownCommandFailsExplicitly(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"run"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "not implemented") {
		t.Fatalf("stderr = %q, want explicit not implemented error", stderr.String())
	}
}
