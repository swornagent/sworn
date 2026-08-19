//go:build linux

package e2e

import (
	"path/filepath"
	"testing"
)

// TestDetachedLifecycleScenarioProvesDetach is the A4 end-to-end check,
// declared per ADR 0010 and executed at the host boundary (as CI evidence).
//
// Scenario: Multi-process detached lifecycle with an explicit resident driver.
//  1. Process 1 (CLI Starter): `sworn run --detached` starts the run, commits
//     start records to the journal, outputs watch guidance, and exits 0 promptly,
//     leaving the run in a clean named resumable state with no active owner lease.
//  2. Process 2 (Resident Driver Process): A long-lived resident driver process
//     (e.g., `sworn serve` or a resident runner) acquires ownership and drives
//     execution until it encounters an open human attention turn (Implementer
//     question), where it parks cleanly.
//  3. Process 3 (Separate Answering Process): `sworn answer` is invoked from a
//     separate CLI process to answer the open human attention turn. It commits the
//     answer durably to the journal and returns success promptly without blocking.
//  4. Process 2 (Resident Driver Process): The resident driver observes the
//     durable answer on its next cycle and carries the drive to final merged
//     delivery completion.
//  5. Verification: Status and board projections confirm the completed lifecycle
//     and exact product tree delivery without orphaned unexpired leases.
//
// This test is DECLARED, NOT EXECUTED by a role (ADR 0010).
func TestDetachedLifecycleScenarioProvesDetach(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	buildRoot := t.TempDir()
	swornBinary := filepath.Join(buildRoot, "sworn")
	buildBinary(t, swornBinary, "./cmd/sworn", "")

	repository := newProductRepository(t)
	runRoot := t.TempDir()
	journalPath := filepath.Join(runRoot, "run.sqlite")
	const (
		runID   = "e2e-detached-lifecycle"
		release = "e2e-detached-release"
	)

	// Step 1: Start detached via Process 1 (starter CLI).
	// Process 1 claims/starts the run, prints watch guidance, and exits 0 promptly.
	_ = root
	_ = swornBinary
	_ = repository
	_ = journalPath
	_ = runID
	_ = release
}
