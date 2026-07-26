package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/driver"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "info" {
		os.Exit(driver.RunFakeCommand(
			os.Args[1:], os.Stdin, os.Stdout, os.Stderr, driver.FakeCompleted,
		))
	}
	body, err := io.ReadAll(io.LimitReader(os.Stdin, driver.MaxRequestBytes+1))
	if err != nil || len(body) > driver.MaxRequestBytes {
		os.Exit(64)
	}
	request, err := driver.DecodeRequest(body)
	if err != nil {
		os.Exit(64)
	}
	if os.Getenv("SWORN_GITHUB_TOKEN") != "" ||
		os.Getenv("GITHUB_TOKEN") != "" {
		os.Exit(65)
	}
	for _, authority := range []string{".git", ".baton", ".sworn"} {
		entries, err := os.ReadDir(filepath.Join(request.Workspace.Path, authority))
		if err == nil && len(entries) != 0 {
			os.Exit(66)
		}
	}
	if strings.Contains(request.InvocationID, "/implementer_implementation/") {
		if err := os.WriteFile(
			filepath.Join(request.Workspace.Path, "one.txt"),
			[]byte("active track\n"),
			0o644,
		); err != nil {
			os.Exit(67)
		}
	}
	profile := driver.FakeCompleted
	if strings.Contains(request.Model, "transport-fail") &&
		strings.Contains(request.InvocationID, "/work_verification/") {
		profile = driver.FakeTransportError
	}
	os.Exit(driver.RunFakeCommand(
		os.Args[1:],
		bytes.NewReader(body),
		os.Stdout,
		os.Stderr,
		profile,
	))
}
