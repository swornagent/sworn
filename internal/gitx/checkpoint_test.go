package gitx

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCaptureAndRestoreCheckpointFidelity(t *testing.T) {
	t.Parallel()

	repo, base := newRepository(t, SHA1)
	key := TrackKey{Release: "rel-fidelity", Track: "T1"}
	createTrack(t, repo, key, base)

	workspaces, err := NewRunWorkspaces(repo, "run-fidelity", testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer workspaces.Close()

	lease, err := workspaces.OpenTrack(key, ImplementationView)
	if err != nil {
		t.Fatal(err)
	}

	// Make changes:
	// 1. Write one scoped source file
	file1 := filepath.Join(lease.Path(), "scoped1.txt")
	if err := os.WriteFile(file1, []byte("content 1"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Create a second file
	file2 := filepath.Join(lease.Path(), "scoped2.txt")
	if err := os.WriteFile(file2, []byte("content 2"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 3. Executable file
	fileExec := filepath.Join(lease.Path(), "script.sh")
	if err := os.WriteFile(fileExec, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Add scratch directory tmp/ that should be excluded
	tmpFile := filepath.Join(lease.Path(), "tmp", "scratch.log")
	if err := os.MkdirAll(filepath.Dir(tmpFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmpFile, []byte("ephemeral diagnostics"), 0o644); err != nil {
		t.Fatal(err)
	}

	attempt := CheckpointAttempt{WorkID: "S1", Epoch: 1, Try: 1}
	scope := CheckpointScope{Include: []string{"scoped1.txt", "scoped2.txt", "script.sh"}}

	result, err := workspaces.CaptureCheckpoint(lease, attempt, "S1", scope, 0, nil)
	if err != nil {
		t.Fatalf("capture checkpoint failed: %v", err)
	}
	if result.Empty {
		t.Fatal("expected non-empty checkpoint result")
	}
	if result.Commit.IsZero() || result.Tree.IsZero() {
		t.Fatal("expected non-zero commit and tree OIDs")
	}

	// Close the original lease
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}

	// Open a new workspace for retry under same authority
	retryLease, err := workspaces.OpenTrack(key, ImplementationView)
	if err != nil {
		t.Fatal(err)
	}
	defer retryLease.Close()

	// Restore checkpoint
	if err := workspaces.RestoreCheckpoint(retryLease, result.Tree); err != nil {
		t.Fatalf("restore checkpoint failed: %v", err)
	}

	// Verify restored file1
	rFile1 := filepath.Join(retryLease.Path(), "scoped1.txt")
	content1, err := os.ReadFile(rFile1)
	if err != nil || string(content1) != "content 1" {
		t.Fatalf("file1 not restored properly: %s, %v", string(content1), err)
	}

	// Verify restored file2
	rFile2 := filepath.Join(retryLease.Path(), "scoped2.txt")
	content2, err := os.ReadFile(rFile2)
	if err != nil || string(content2) != "content 2" {
		t.Fatalf("file2 not restored properly: %s, %v", string(content2), err)
	}

	// Verify restored executable bit
	rFileExec := filepath.Join(retryLease.Path(), "script.sh")
	stat, err := os.Stat(rFileExec)
	if err != nil {
		t.Fatalf("script.sh not found: %v", err)
	}
	if stat.Mode()&0o111 == 0 {
		t.Fatalf("expected executable mode, got %v", stat.Mode())
	}

	// Verify tmp/ was NOT part of the restored checkpoint
	rTmp := filepath.Join(retryLease.Path(), "tmp")
	if _, err := os.Lstat(rTmp); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected scratch tmp/ to not be restored, but found: %v", err)
	}
}

func TestCheckpointIgnoreSemantics(t *testing.T) {
	t.Parallel()

	repo, base := newRepository(t, SHA1)
	key := TrackKey{Release: "rel-ignore", Track: "T1"}
	createTrack(t, repo, key, base)

	workspaces, err := NewRunWorkspaces(repo, "run-ignore", testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	defer workspaces.Close()

	lease, err := workspaces.OpenTrack(key, ImplementationView)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()

	// Write a .gitignore ignoring *.log and build/
	gitignore := filepath.Join(lease.Path(), ".gitignore")
	if err := os.WriteFile(gitignore, []byte("*.log\nbuild/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Untracked file matching ignore pattern
	ignoredUntracked := filepath.Join(lease.Path(), "test.log")
	if err := os.WriteFile(ignoredUntracked, []byte("ignore me"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Tracked file: modify it
	trackedLog := filepath.Join(lease.Path(), "tracked.txt")
	if err := os.WriteFile(trackedLog, []byte("tracked content"), 0o644); err != nil {
		t.Fatal(err)
	}

	attempt := CheckpointAttempt{WorkID: "S1", Epoch: 1, Try: 1}
	scope := CheckpointScope{Include: []string{".gitignore", "tracked.txt"}}

	result, err := workspaces.CaptureCheckpoint(lease, attempt, "S1", scope, 0, nil)
	if err != nil {
		t.Fatalf("capture checkpoint failed: %v", err)
	}

	for _, p := range result.ChangedPaths {
		if p == "test.log" {
			t.Fatal("untracked ignored file test.log was included in checkpoint")
		}
	}
}
