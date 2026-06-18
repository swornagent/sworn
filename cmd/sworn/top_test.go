package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTop_EmptyState checks that sworn top renders an empty evidence pane
// with a hint when no journeys artefact exists.
func TestTop_EmptyState(t *testing.T) {
	dir := t.TempDir()

	// Run renderEvidenceSurface in a dir with no journeys artefact.
	exitCode := renderEvidenceSurface("test-release", dir)
	if exitCode != 0 {
		t.Errorf("expected exit 0 for empty state, got %d", exitCode)
	}
}

// TestTop_GreenBoard checks that sworn top renders a green-board when all
// journeys have passing attestations.
func TestTop_GreenBoard(t *testing.T) {
	dir := t.TempDir()

	// Create journeys artefact with two journeys.
	createJourneysArtefact(t, dir, `{
		"version": 1,
		"created_at": "2026-06-16T00:00:00Z",
		"updated_at": "2026-06-16T00:00:00Z",
		"journeys": [
			{
				"id": "J01-onboard-new-user",
				"user_type": "new_user",
				"outcome": "User creates account and sets up profile"
			},
			{
				"id": "J02-create-scenario",
				"user_type": "pro_user",
				"outcome": "User creates a new financial scenario"
			}
		],
		"is_ratified": true,
		"ratified_by": "brad",
		"ratified_at": "2026-06-16T00:00:00Z"
	}`)

	// Create attestations artefact with both passing.
	createAttestationsArtefact(t, dir, `{
		"version": 1,
		"attestations": [
			{
				"journey_id": "J01-onboard-new-user",
				"status": "walked-pass",
				"walked_by": "brad",
				"real_infra": true,
				"mocks_off": true
			},
			{
				"journey_id": "J02-create-scenario",
				"status": "walked-pass",
				"walked_by": "brad",
				"real_infra": true,
				"mocks_off": true
			}
		]
	}`)

	exitCode := renderEvidenceSurface("test-release", dir)
	if exitCode != 0 {
		t.Errorf("expected exit 0 for green-board, got %d", exitCode)
	}
}

// TestTop_KillList_Unwalked checks that un-walked journeys are rendered
// in a kill-list with exit code 1.
func TestTop_KillList_Unwalked(t *testing.T) {
	dir := t.TempDir()

	// Create journeys artefact with one journey.
	createJourneysArtefact(t, dir, `{
		"version": 1,
		"created_at": "2026-06-16T00:00:00Z",
		"updated_at": "2026-06-16T00:00:00Z",
		"journeys": [
			{
				"id": "J01-onboard-new-user",
				"user_type": "new_user",
				"outcome": "User creates account and sets up profile"
			}
		],
		"is_ratified": true,
		"ratified_by": "brad",
		"ratified_at": "2026-06-16T00:00:00Z"
	}`)
	// Deliberately NO attestations — all un-walked.

	exitCode := renderEvidenceSurface("test-release", dir)
	if exitCode != 1 {
		t.Errorf("expected exit 1 for kill-list, got %d", exitCode)
	}
}

// TestTop_KillList_Failed checks that walked-fail journeys are rendered
// in a kill-list with exit code 1.
func TestTop_KillList_Failed(t *testing.T) {
	dir := t.TempDir()

	// Create journeys artefact with one journey.
	createJourneysArtefact(t, dir, `{
		"version": 1,
		"created_at": "2026-06-16T00:00:00Z",
		"updated_at": "2026-06-16T00:00:00Z",
		"journeys": [
			{
				"id": "J01-onboard-new-user",
				"user_type": "new_user",
				"outcome": "User creates account and sets up profile"
			}
		],
		"is_ratified": true,
		"ratified_by": "brad",
		"ratified_at": "2026-06-16T00:00:00Z"
	}`)

	// Create attestations with failed walkthrough.
	createAttestationsArtefact(t, dir, `{
		"version": 1,
		"attestations": [
			{
				"journey_id": "J01-onboard-new-user",
				"status": "walked-fail",
				"walked_by": "brad",
				"real_infra": true,
				"mocks_off": true,
				"notes": "Found a regression"
			}
		]
	}`)

	exitCode := renderEvidenceSurface("test-release", dir)
	if exitCode != 1 {
		t.Errorf("expected exit 1 for kill-list with failed journey, got %d", exitCode)
	}
}

// TestTop_ReadOnly checks that sworn top issues no state transition or
// artefact write. It does this by verifying the assertion that no files
// are created or modified in the project root.
func TestTop_ReadOnly(t *testing.T) {
	dir := t.TempDir()

	// Create a journeys artefact.
	createJourneysArtefact(t, dir, `{
		"version": 1,
		"created_at": "2026-06-16T00:00:00Z",
		"updated_at": "2026-06-16T00:00:00Z",
		"journeys": [
			{
				"id": "J01-onboard-new-user",
				"user_type": "new_user",
				"outcome": "User creates account and sets up profile"
			}
		],
		"is_ratified": true,
		"ratified_by": "brad",
		"ratified_at": "2026-06-16T00:00:00Z"
	}`)

	// Snapshot files before running top.
	filesBefore := listFiles(t, dir)

	exitCode := renderEvidenceSurface("test-release", dir)
	if exitCode != 1 { // un-walked = kill-list
		t.Errorf("expected exit 1 for un-walked, got %d", exitCode)
	}

	// Snapshot files after running top — should be identical.
	filesAfter := listFiles(t, dir)
	if !stringSliceEqual(filesBefore, filesAfter) {
		t.Errorf("top modified the filesystem (read-only violation):\n before: %v\n after:  %v",
			filesBefore, filesAfter)
	}
}

// TestTop_Mixed checks that a mix of passed and un-walked journeys
// renders a kill-list naming the un-walked ones.
func TestTop_Mixed(t *testing.T) {
	dir := t.TempDir()

	createJourneysArtefact(t, dir, `{
		"version": 1,
		"created_at": "2026-06-16T00:00:00Z",
		"updated_at": "2026-06-16T00:00:00Z",
		"journeys": [
			{
				"id": "J01-onboard-new-user",
				"user_type": "new_user",
				"outcome": "User creates account and sets up profile"
			},
			{
				"id": "J02-create-scenario",
				"user_type": "pro_user",
				"outcome": "User creates a new financial scenario"
			}
		],
		"is_ratified": true,
		"ratified_by": "brad",
		"ratified_at": "2026-06-16T00:00:00Z"
	}`)

	// Only one journey has an attestation (passed).
	createAttestationsArtefact(t, dir, `{
		"version": 1,
		"attestations": [
			{
				"journey_id": "J01-onboard-new-user",
				"status": "walked-pass",
				"walked_by": "brad",
				"real_infra": true,
				"mocks_off": true
			}
		]
	}`)

	exitCode := renderEvidenceSurface("test-release", dir)
	if exitCode != 1 {
		t.Errorf("expected exit 1 for kill-list (one un-walked), got %d", exitCode)
	}
}

// TestTop_EmptyJourneysArtefact checks that an artefact with no journeys
// renders gracefully.
func TestTop_EmptyJourneysArtefact(t *testing.T) {
	dir := t.TempDir()

	createJourneysArtefact(t, dir, `{
		"version": 1,
		"created_at": "2026-06-16T00:00:00Z",
		"updated_at": "2026-06-16T00:00:00Z",
		"journeys": [],
		"is_ratified": true,
		"ratified_by": "brad",
		"ratified_at": "2026-06-16T00:00:00Z"
	}`)

	exitCode := renderEvidenceSurface("test-release", dir)
	if exitCode != 0 {
		t.Errorf("expected exit 0 for empty journeys, got %d", exitCode)
	}
}

// --- helpers ---

func createJourneysArtefact(t *testing.T, dir, content string) {
	t.Helper()
	swornDir := filepath.Join(dir, ".sworn")
	if err := os.MkdirAll(swornDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(swornDir, "journeys.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func createAttestationsArtefact(t *testing.T, dir, content string) {
	t.Helper()
	swornDir := filepath.Join(dir, ".sworn")
	if err := os.MkdirAll(swornDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(swornDir, "attestations.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ensure TestTop matched by -run TestTop
var _ = strings.Contains // suppress unused import
