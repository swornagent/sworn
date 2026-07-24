package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

func TestVersionJSONReportsExactBatonAdmission(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"version", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr = %q", code, stderr.String())
	}
	var got versionInfo
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != swornVersion || got.State != swornState {
		t.Fatalf("version identity = %#v", got)
	}
	if got.Baton.PackageVersion != baton.PackageVersion ||
		got.Baton.TagObject != baton.TagObject ||
		got.Baton.Commit != baton.Commit ||
		got.Baton.Tree != baton.Tree ||
		got.Baton.ArchiveSHA256 != baton.ArchiveSHA256 ||
		got.Baton.SupportPackageSHA256 != baton.SupportPackageSHA256 ||
		got.Baton.ManifestSHA256 != baton.ManifestSHA256 ||
		got.Baton.AssetCount != baton.AssetCount ||
		got.Baton.AssetBytes != baton.AssetBytes {
		t.Fatalf("Baton identity = %#v", got.Baton)
	}
	if strings.Contains(stdout.String(), `"commit":"unknown"`) {
		t.Fatalf("version output reintroduced Sworn commit stamping: %s", stdout.String())
	}
}

func TestVersionTextIsSmallAndExplicit(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() = %d, stderr = %q", code, stderr.String())
	}
	want := "sworn 0.3.0-dev\nstate baton-rc2-admitted\nbaton 1.0.0-rc.2 (" + baton.Commit + ")\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestHelpIsTheOnlyArgumentFreeCommand(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{nil, {"help"}, {"--help"}, {"-h"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("run(%v) = %d, stderr = %q", args, code, stderr.String())
		}
		if stdout.String() != usage || stderr.Len() != 0 {
			t.Fatalf("run(%v) stdout = %q, stderr = %q", args, stdout.String(), stderr.String())
		}
	}
}

func TestRetiredAndUnknownCommandsShareOneClosedPath(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"run", "run-1", "--config", "/unreadable"},
		{"board", "--store", "/unreadable"},
		{"__executor-shim", "--marker", "/unwritable"},
		{"deliver"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Fatalf("run(%v) = %d, want 2", args, code)
		}
		if stdout.Len() != 0 {
			t.Fatalf("run(%v) stdout = %q", args, stdout.String())
		}
		if !strings.Contains(stderr.String(), "is not implemented at the v0.3 admission checkpoint") {
			t.Fatalf("run(%v) stderr = %q", args, stderr.String())
		}
		if strings.Contains(stderr.String(), "/unreadable") || strings.Contains(stderr.String(), "/unwritable") {
			t.Fatalf("run(%v) inspected or echoed a retired path: %q", args, stderr.String())
		}
	}
}

func TestVersionRejectsEveryOtherShape(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"version", "--json", "--json"}, {"version", "--text"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Fatalf("run(%v) = %d, want 2", args, code)
		}
		if stdout.Len() != 0 || stderr.String() != "usage: sworn version [--json]\n" {
			t.Fatalf("run(%v) stdout = %q, stderr = %q", args, stdout.String(), stderr.String())
		}
	}
}
