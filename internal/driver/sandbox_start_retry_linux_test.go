//go:build linux

package driver

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestReadSandboxProcessGroupNamesItsFailureModes pins the three
// distinguishable handshake failures (sworn#251): a report that never
// arrived (the launcher died during setup, pre-fork - the only mode the
// launch loop may retry), a reported child whose group cannot be probed,
// and the healthy path. The group-unchanged mode is not provoked here: it
// requires holding a live child inside the parent's group for the whole
// processStartHandshakeGrace, which is deliberately long.
func TestReadSandboxProcessGroupNamesItsFailureModes(t *testing.T) {
	t.Parallel()

	// Report never arrives: the writer closes with nothing written.
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	_, _, statusErr := readSandboxProcessGroup(reader, os.Getpid())
	_ = reader.Close()
	if statusErr != errSandboxGroupReportMissing {
		t.Fatalf("silent launcher error = %v, want errSandboxGroupReportMissing", statusErr)
	}

	// Reported child is already dead: Getpgid fails immediately, so this
	// must refuse as unprobeable without waiting out any grace window.
	dead := exec.Command("true")
	if err := dead.Start(); err != nil {
		t.Fatal(err)
	}
	deadPID := dead.Process.Pid
	if err := dead.Wait(); err != nil {
		t.Fatal(err)
	}
	reader, writer, err = os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(writer).Encode(map[string]int{"child-pid": deadPID}); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	_, _, statusErr = readSandboxProcessGroup(reader, os.Getpid())
	_ = reader.Close()
	if statusErr != errSandboxChildUnprobeable {
		t.Fatalf("dead child error = %v, want errSandboxChildUnprobeable", statusErr)
	}

	// Healthy: a live child in its own process group reports cleanly. A
	// bogus parent PID guarantees the group comparison passes immediately.
	alive := exec.Command("sleep", "5")
	alive.SysProcAttr = linuxSandboxProcessAttributes()
	if err := alive.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = alive.Process.Kill()
		_ = alive.Wait()
	}()
	reader, writer, err = os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(writer).Encode(map[string]int{"child-pid": alive.Process.Pid}); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	child, group, statusErr := readSandboxProcessGroup(reader, os.Getpid())
	_ = reader.Close()
	if statusErr != nil || child != alive.Process.Pid || group <= 0 {
		t.Fatalf("healthy handshake = (%d, %d, %v)", child, group, statusErr)
	}
}

// TestFailSandboxStartCauseCarriesStaticSentences pins the handshake site's
// engine-authored causes through the same envelope and revalidation the
// errno-extracted causes use: a bounded static sentence survives to the
// funnel, an overlong one drops to a bare named check rather than
// truncating.
func TestFailSandboxStartCauseCarriesStaticSentences(t *testing.T) {
	t.Parallel()
	err := failSandboxStartCause(
		"sandbox_start.process_group_handshake_read",
		errSandboxGroupReportMissing.Error()+"; launcher exit status 1 after 3 attempts",
	)
	var contractErr *ContractError
	if !errors.As(err, &contractErr) || contractErr.Code != "PROCESS_START_FAILED" {
		t.Fatalf("handshake refusal = %v", err)
	}
	canonical, ok := revalidateSandboxStartDetail(contractErr.Detail)
	if !ok || !strings.Contains(canonical, "launcher exit status 1 after 3 attempts") {
		t.Fatalf("revalidated detail = %q ok=%v", canonical, ok)
	}

	overlong := failSandboxStartCause(
		"sandbox_start.process_group_handshake_read",
		strings.Repeat("x", maxSandboxStartCauseBytes+1),
	)
	if !errors.As(overlong, &contractErr) {
		t.Fatalf("overlong refusal = %v", overlong)
	}
	canonical, ok = revalidateSandboxStartDetail(contractErr.Detail)
	if !ok || strings.Contains(canonical, "xxx") {
		t.Fatalf("overlong cause survived: %q ok=%v", canonical, ok)
	}
}
