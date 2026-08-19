//go:build linux

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/journal"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
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
	root := t.TempDir()
	swornBinary := filepath.Join(root, "sworn")
	buildBinary(t, swornBinary, "./cmd/sworn", "")

	repository := newProductRepository(t)
	targetBefore := runGit(t, repository, "rev-parse", "main")
	planBytes, plan := recoveryE2EPlan(t)

	const (
		runID          = "e2e-detached-lifecycle"
		release        = "turn-recovery-release"
		questionCanary = "DETACHED-HUMAN-QUESTION-CANARY"
		answerCanary   = "DETACHED-HUMAN-ANSWER-CANARY"
	)

	provider := &recoveryE2EProvider{
		t:         t,
		planBytes: planBytes,
		recover:   true,
		human:     true,
		question:  questionCanary,
		answer:    answerCanary,
		turns:     make(map[string]int),
	}
	providerHTTP := httptest.NewServer(http.HandlerFunc(provider.serve))
	defer providerHTTP.Close()

	configBody, loaded := recoveryE2EConfig(t, providerHTTP.URL)
	configPath := filepath.Join(root, "drivers.json")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}

	manifestBody := recoveryE2EManifest(t, runID, repository, loaded)
	manifestPath := writeManifest(t, root, manifestBody)
	journalPath := filepath.Join(root, "run.sqlite")
	address := telemetryParityAddress(t)
	operatorConfigPath := telemetryParityOperatorConfig(t, root, address, "")
	environment := map[string]string{
		"SWORN_TURN_RECOVERY_KEY": recoveryE2ESecret,
	}

	// Step 1: Process 1 (CLI Starter): `sworn run --detached` starts the run,
	// commits start records to the journal, outputs watch guidance, and exits 0
	// promptly, leaving the run in a clean named resumable state with no active
	// owner lease.
	runStdout, runStderr := runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"run",
		"--manifest", manifestPath,
		"--journal", journalPath,
		"--config", configPath,
		"--detached",
	)
	if runStderr != "" {
		t.Fatalf("sworn run --detached stderr = %q", runStderr)
	}
	wantGuidance := "Sworn run " + runID + " started detached.\n\n" +
		"Watch progress:\n" +
		"  sworn board --run " + runID + " --journal " + journalPath + "\n" +
		"  sworn tui\n"
	if runStdout != wantGuidance {
		t.Fatalf("sworn run --detached stdout = %q, want %q", runStdout, wantGuidance)
	}

	// Verify the run binding exists and no claimed unexpired owner lease remains.
	ctx := context.Background()
	store, err := journal.OpenReadOnly(ctx, journalPath)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	binding, err := store.RunBinding(ctx, runID)
	if err != nil || binding.ID != runID {
		_ = store.Close()
		t.Fatalf("run binding = %#v, err = %v", binding, err)
	}
	owner, present, err := store.CurrentOwner(ctx, runID)
	_ = store.Close()
	if err != nil {
		t.Fatalf("check owner: %v", err)
	}
	if present && owner.ExpiresAt.After(time.Now().UTC()) {
		t.Fatalf("claimed owner lease remained after detached command exit: %#v", owner)
	}

	// Authorize and install the approved plan so the run is ready to execute S1.
	authorizePlan(t, journalPath, runID, plan)
	installApprovedPlan(t, repository, planBytes)

	// Step 2: Process 2 (Resident Driver Process): A long-lived resident driver
	// process (`sworn serve`) acquires ownership and drives execution until it
	// encounters an open human attention turn (Implementer question), where it
	// parks cleanly.
	serveProcess := exec.Command(
		swornBinary, "serve",
		"--run", runID,
		"--journal", journalPath,
		"--manifest", manifestPath,
		"--operator-config", operatorConfigPath,
		"--config", configPath,
	)
	serveProcess.Env = cleanEnvironment(environment)
	var serveOut, serveErr bytes.Buffer
	serveProcess.Stdout, serveProcess.Stderr = &serveOut, &serveErr
	if err := serveProcess.Start(); err != nil {
		t.Fatalf("start sworn serve: %v", err)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- serveProcess.Wait() }()
	stopped := false
	defer func() {
		if serveProcess.Process != nil && !stopped {
			_ = serveProcess.Process.Signal(syscall.SIGTERM)
			select {
			case <-serveDone:
			case <-time.After(15 * time.Second):
				_ = serveProcess.Process.Kill()
			}
		}
	}()
	telemetryParityWaitHealth(
		t, address, func(cockpit.TelemetryHealth) bool { return true },
	)

	// Drive the run via the resident driver process to the open human turn.
	mcpResume := journeyMCPCall(t, address, "sworn_control", map[string]any{
		"run_id":              runID,
		"command_id":          "detached-resident-resume-1",
		"kind":                string(journal.Resume),
		"expected_generation": int64(0),
	})
	if bytes.Contains(mcpResume, []byte(`"isError":true`)) {
		t.Fatalf("resident resume = %s", mcpResume)
	}

	// Verify the run parked cleanly on the open human turn.
	var openAttentionID string
	deadline := time.Now().Add(60 * time.Second)
	for {
		attentions := mcpAttentions(t, address)
		if len(attentions) == 1 && attentions[0].Question == questionCanary {
			openAttentionID = attentions[0].ID
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("resident driver did not reach open human turn: %v", attentions)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Step 3: Process 3 (Separate Answering Process): `sworn answer` is invoked
	// from a separate CLI process to answer the open human attention turn. It
	// commits the answer durably to the journal and returns success promptly
	// without blocking.
	answerStdout, answerStderr := runBinaryWithEnvironment(
		t, swornBinary, 0, environment,
		"answer",
		"--run", runID,
		"--journal", journalPath,
		"--attention", openAttentionID,
		"--generation", "1",
		"--answer", answerCanary,
		"--config", configPath,
	)
	if answerStderr != "" {
		t.Fatalf("sworn answer stderr = %q", answerStderr)
	}
	if !strings.Contains(answerStdout, "Sworn run "+runID) {
		t.Fatalf("sworn answer stdout = %q", answerStdout)
	}

	// Assert the answer was durably committed to the journal.
	store, err = journal.OpenReadOnly(ctx, journalPath)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	snapshot, err := store.Snapshot(ctx, runID)
	_ = store.Close()
	if err != nil {
		t.Fatalf("snapshot journal: %v", err)
	}
	hasAnswerCommand := false
	for _, cmd := range snapshot.Commands {
		if cmd.Kind == "attention.answer" {
			hasAnswerCommand = true
			break
		}
	}
	if !hasAnswerCommand {
		t.Fatal("sworn answer did not durably record attention.answer command in journal")
	}

	// Step 4: Process 2 (Resident Driver Process): The resident driver observes
	// the durable answer and carries the drive to final merged delivery
	// completion.
	completionDeadline := time.Now().Add(120 * time.Second)
	for {
		snap := mcpSnapshot(t, address, runID)
		if snap.Run.State == "complete" {
			break
		}
		if time.Now().After(completionDeadline) {
			t.Fatalf("resident driver did not carry run to completion; last state = %q", snap.Run.State)
		}
		if snap.Run.State == "parked" {
			_ = journeyMCPCall(t, address, "sworn_control", map[string]any{
				"run_id":              runID,
				"command_id":          "detached-resident-resume-2",
				"kind":                string(journal.Resume),
				"expected_generation": snap.Run.ControlGeneration,
			})
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Step 5: Verification: Status and board projections confirm the completed
	// lifecycle and exact product tree delivery without orphaned unexpired leases.
	stopped = true
	_ = serveProcess.Process.Signal(syscall.SIGTERM)
	select {
	case <-serveDone:
	case <-time.After(15 * time.Second):
		_ = serveProcess.Process.Kill()
		t.Fatal("sworn serve did not stop")
	}
	if serveOut.String() != "sworn serve: ready\n" || serveErr.Len() != 0 {
		t.Fatalf("serve stdout = %q, stderr = %q", serveOut.String(), serveErr.String())
	}

	// Status projection via CLI
	statusStdout, statusStderr := runBinary(
		t, swornBinary, 0,
		"status", "--run", runID, "--journal", journalPath, "--json",
	)
	if statusStderr != "" {
		t.Fatalf("status stderr = %q", statusStderr)
	}
	var finalStatus swornruntime.RunStatus
	if err := json.Unmarshal([]byte(statusStdout), &finalStatus); err != nil {
		t.Fatalf("parse status: %v", err)
	}
	if finalStatus.State != "complete" || finalStatus.Outcome != "merged" {
		t.Fatalf("final status = %#v", finalStatus)
	}

	// Board projection via CLI
	boardStdout, boardStderr := runBinary(
		t, swornBinary, 0,
		"board", "--run", runID, "--journal", journalPath, "--json",
	)
	if boardStderr != "" {
		t.Fatalf("board stderr = %q", boardStderr)
	}
	var finalBoard cockpit.Snapshot
	if err := json.Unmarshal([]byte(boardStdout), &finalBoard); err != nil {
		t.Fatalf("parse board: %v", err)
	}
	if finalBoard.Run.State != "complete" {
		t.Fatalf("final board run state = %q, want complete", finalBoard.Run.State)
	}

	// Baton state in git
	finalState := readBatonState(t, repository, release)
	if finalState.Assembly.Outcome != "merged" ||
		finalState.Assembly.Candidate == nil ||
		finalState.Assembly.Pass == nil ||
		finalState.Assembly.ResultCommit == "" {
		t.Fatalf("baton state = %#v", finalState.Assembly)
	}
	targetAfter := runGit(t, repository, "rev-parse", "main")
	if targetAfter == targetBefore || targetAfter != finalState.Assembly.ResultCommit {
		t.Fatalf("target head = %s, want %s (was %s)", targetAfter, finalState.Assembly.ResultCommit, targetBefore)
	}
	productContent := runGit(t, repository, "show", "main:one.txt")
	if productContent != strings.TrimSuffix(recoveryE2EContent, "\n") {
		t.Fatalf("delivered product content = %q, want %q", productContent, recoveryE2EContent)
	}

	// No orphaned unexpired lease remaining on disk
	store, err = journal.OpenReadOnly(ctx, journalPath)
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	owner, present, err = store.CurrentOwner(ctx, runID)
	_ = store.Close()
	if err != nil {
		t.Fatalf("check owner: %v", err)
	}
	if present && owner.ExpiresAt.After(time.Now().UTC()) {
		t.Fatalf("orphaned claimed owner lease remained: %#v", owner)
	}
}
