package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
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
	workspacePath := driver.EffectiveWorkspacePath(request.Workspace.Path)
	// The reserved-authority visibility check is a defensive fixture guard on
	// the contained path, where /workspace masks .git/.baton/.sworn with empty
	// tmpfs. In an uncontained dispatch the real worktree (including
	// .baton/releases) is directly visible, so the check is skipped only when
	// the engine-set uncontained marker is present; the marker never appears in
	// a contained child environment.
	if !driver.UncontainedDispatchMarker() {
		for _, authority := range []string{".git", ".baton", ".sworn"} {
			entries, err := os.ReadDir(filepath.Join(workspacePath, authority))
			if err == nil && len(entries) != 0 {
				os.Exit(66)
			}
		}
	}
	if strings.Contains(request.InvocationID, "/implementer_implementation/") {
		name, content := "one.txt", "active track\n"
		if strings.Contains(request.Model, "composition-conflict") {
			switch {
			case strings.Contains(request.InvocationID, "/S1/"):
				name, content = "shared.txt", "producer product\n"
			case strings.Contains(request.InvocationID, "/S2/"):
				name, content = "shared.txt", "consumer-track product\n"
			case strings.Contains(request.InvocationID, "/S3/"):
				name, content = "consumer.txt", "consumer product\n"
			}
		} else if strings.Contains(request.InvocationID, "/implementer_implementation/2/") {
			content = "active track revised\n"
		} else if strings.Contains(request.InvocationID, "/S2/") {
			name, content = "two.txt", "second track\n"
		}
		if strings.Contains(request.Model, "scope-escape") {
			name = "outside.txt"
			content = "outside approved scope " + strconv.Itoa(os.Getpid()) + "\n"
		}
		if err := os.WriteFile(
			filepath.Join(workspacePath, name),
			[]byte(content),
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
