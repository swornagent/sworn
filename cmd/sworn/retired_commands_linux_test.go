//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestBuiltBinaryClosedCommandsDoNotConsumePaths(t *testing.T) {
	binary := buildCurrentSworn(t)

	fifo := filepath.Join(t.TempDir(), "retired-input.fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "shim.marker")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			"legacy board shape",
			[]string{"board", "--store", fifo, "--json"},
			"usage: sworn board",
		},
		{
			"retired shim",
			[]string{"__executor-shim", "--sworn-start-marker", marker},
			"unknown command",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, binary, test.args...)
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			err := command.Run()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Fatal("retired command consumed a blocking path")
			}
			assertProcessExit(t, err, 2)
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q", stdout.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr = %q", stderr.String())
			}
			if strings.Contains(stderr.String(), fifo) || strings.Contains(stderr.String(), marker) {
				t.Fatalf("retired command exposed an ignored path: %q", stderr.String())
			}
		})
	}
	if _, err := os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired shim touched its marker: %v", err)
	}
}
