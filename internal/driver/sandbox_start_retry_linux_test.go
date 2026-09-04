//go:build linux

package driver

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestReadSandboxProcessGroupNamesItsFailureModes pins the handshake's
// distinguishable outcomes (sworn#251, sworn#277): a report that never
// arrived (the launcher died during setup, pre-fork - the only mode the
// launch loop may retry), a reported child already gone when probed (it
// ran to completion first - a completed start, not a refusal), and the
// healthy path. The group-unchanged mode is not provoked here: it
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

	// Reported child is already gone: it ran to completion and was reaped
	// before the probe (sworn#277). That is a completed start, reported
	// without waiting out any grace window, with the child's own pid
	// standing in for its group.
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
	gone, goneGroup, goneErr := readSandboxProcessGroup(reader, os.Getpid())
	_ = reader.Close()
	if goneErr != nil || gone != deadPID || goneGroup != deadPID {
		t.Fatalf("gone child handshake = (%d, %d, %v), want (%d, %d, nil)",
			gone, goneGroup, goneErr, deadPID, deadPID)
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

// TestToolBashFastExitSurvivesStarvedHandshakeProbe reproduces the hosted-
// runner class (sworn#277): bubblewrap reports its child the instant it
// forks, a short script finishes and is reaped before a starved engine gets
// to probe the child's group, and the launch used to refuse as a dead child
// (PROCESS_START_FAILED "sandbox child process group unprobeable") although
// the command ran to completion. The probe delay stands in for the starved
// scheduler; the result must be the command's own exit code and output.
func TestToolBashFastExitSurvivesStarvedHandshakeProbe(t *testing.T) {
	requireTrustedContainment(t)
	previous := testSandboxHandshakeProbeDelay
	testSandboxHandshakeProbeDelay = 500 * time.Millisecond
	t.Cleanup(func() { testSandboxHandshakeProbeDelay = previous })

	invocation, _, _ := memoryInvocationFixture(t)
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result := executeToolJSON(t, session, "bash-fast-exit", "Bash", map[string]any{
		"script": `printf 'fast-evidence\n'
exit 42`,
	})
	content := string(result.Content)
	if !result.Failed || !strings.HasPrefix(content, "error:PROCESS_FAILED exit_code=42\n") ||
		!strings.Contains(content, "fast-evidence") {
		t.Fatalf("fast-exit result under a starved probe = failed=%v content=%q",
			result.Failed, content)
	}
	success := executeToolJSON(t, session, "bash-fast-ok", "Bash", map[string]any{
		"script": `printf 'ok-evidence\n'`,
	})
	if success.Failed || string(success.Content) != "ok-evidence" {
		t.Fatalf("fast clean exit under a starved probe = failed=%v content=%q",
			success.Failed, string(success.Content))
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
