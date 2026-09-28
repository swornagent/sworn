package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
)

func TestRunHostCommandPassFailAndOverflow(t *testing.T) {
	t.Parallel()

	t.Run("pass", func(t *testing.T) {
		result := runHostCommand(t.TempDir(), "printf 'ok\\n'", hostCheckOutputBytes, 5*time.Second)
		if result.Outcome != protocol.CheckOutcomePass || result.ExitCode != 0 {
			t.Fatalf("pass result = %#v", result)
		}
		if !strings.Contains(result.Output, "ok") {
			t.Fatalf("pass output = %q", result.Output)
		}
		if result.OutputDigest != protocol.DigestBytes([]byte(result.Output)) {
			t.Fatal("output digest does not match output")
		}
	})

	t.Run("fail", func(t *testing.T) {
		result := runHostCommand(t.TempDir(), "exit 7", hostCheckOutputBytes, 5*time.Second)
		if result.Outcome != protocol.CheckOutcomeFail || result.ExitCode != 7 {
			t.Fatalf("fail result = %#v", result)
		}
	})

	t.Run("overflow", func(t *testing.T) {
		result := runHostCommand(t.TempDir(), "yes x | head -c 1000000", 4096, 5*time.Second)
		if result.Outcome != protocol.CheckOutcomeOverflow {
			t.Fatalf("overflow result = %#v", result)
		}
		if !result.Truncated {
			t.Fatal("overflow was not marked truncated")
		}
		if !strings.Contains(result.Output, protocol.HostCheckTruncationPrefix) {
			t.Fatalf("overflow output lacks the truthful marker: %q", result.Output)
		}
		if len(result.Output) > 8192 {
			t.Fatalf("overflow output exceeds bound: %d", len(result.Output))
		}
	})
}

// TestRunHostCommandFailsLoudlyWithoutResolvableShell is the A3 consumer
// proof at the actual host-check runner: a missing shell on PATH or an
// invalid SWORN_SH override refuses the check with a diagnostic instead of
// silently restoring a hardcoded /bin/sh literal.
func TestRunHostCommandFailsLoudlyWithoutResolvableShell(t *testing.T) {
	t.Run("no shell on PATH", func(t *testing.T) {
		t.Setenv(gitx.EnvShell, "")
		t.Setenv("PATH", t.TempDir())
		result := runHostCommand(
			t.TempDir(), "true", hostCheckOutputBytes, 5*time.Second,
		)
		if result.Outcome != protocol.CheckOutcomeFail || result.ExitCode != -1 {
			t.Fatalf("no-shell result = %#v", result)
		}
		if !strings.Contains(result.Diagnostic, "POSIX shell") {
			t.Fatalf("no-shell diagnostic = %q", result.Diagnostic)
		}
	})
	t.Run("invalid override refused", func(t *testing.T) {
		t.Setenv(gitx.EnvShell, filepath.Join(t.TempDir(), "missing-shell"))
		result := runHostCommand(
			t.TempDir(), "true", hostCheckOutputBytes, 5*time.Second,
		)
		if result.Outcome != protocol.CheckOutcomeFail || result.ExitCode != -1 {
			t.Fatalf("invalid-override result = %#v", result)
		}
		if !strings.Contains(result.Diagnostic, "POSIX shell") {
			t.Fatalf("invalid-override diagnostic = %q", result.Diagnostic)
		}
	})
	t.Run("override honored at the runner", func(t *testing.T) {
		realShell, err := exec.LookPath("sh")
		if err != nil {
			t.Skip("no sh discoverable to override with")
		}
		if canonical, err := filepath.EvalSymlinks(realShell); err == nil {
			realShell = canonical
		}
		t.Setenv(gitx.EnvShell, realShell)
		// Discovery would fail here; only the override can satisfy the run.
		t.Setenv("PATH", t.TempDir())
		result := runHostCommand(
			t.TempDir(), "printf 'ok\\n'", hostCheckOutputBytes, 5*time.Second,
		)
		if result.Outcome != protocol.CheckOutcomePass || result.ExitCode != 0 {
			t.Fatalf("override result = %#v", result)
		}
	})
}

func TestRunHostCommandTimeoutIsRecordedAsTimeout(t *testing.T) {
	t.Parallel()
	result := runHostCommand(t.TempDir(), "sleep 10", hostCheckOutputBytes, 300*time.Millisecond)
	if result.Outcome != protocol.CheckOutcomeTimeout {
		t.Fatalf("timeout result = %#v", result)
	}
	if result.Diagnostic == "" {
		t.Fatal("timeout diagnostic is empty")
	}
}

func TestHostOutputExcerptKeepsDigestInvariantForFullOutput(t *testing.T) {
	t.Parallel()

	full := "all good\n"
	excerpt, truncated := hostOutputExcerpt(full, false)
	if truncated || excerpt != full {
		t.Fatalf("excerpt = %q truncated=%v", excerpt, truncated)
	}
	if protocol.DigestBytes([]byte(excerpt)) != protocol.DigestBytes([]byte(full)) {
		t.Fatal("excerpt digest differs")
	}

	big := strings.Repeat("x", protocol.HostCheckOutputManifestBytes+10)
	excerpt, truncated = hostOutputExcerpt(big, false)
	if !truncated {
		t.Fatal("large output was not marked truncated")
	}
	if !strings.Contains(excerpt, protocol.HostCheckTruncationPrefix) {
		t.Fatalf("large output excerpt lacks marker: %q", excerpt)
	}
}

func TestBuildHostCheckResultsManifestBindsHostAndRoleEntries(t *testing.T) {
	t.Parallel()

	pass := int(0)
	results := []hostCheckResult{{
		Slice: "S1", Candidate: strings.Repeat("1", 40),
		ContractDigest: "sha256:" + strings.Repeat("b", 64),
		Check:          "go test ./...", Outcome: protocol.CheckOutcomePass,
		ExitCode: 0, Output: "all good\n",
		OutputDigest: protocol.DigestBytes([]byte("all good\n")),
		EffectID:     "attempt/host/1/1",
	}}
	manifest, err := buildHostCheckResultsManifest(
		"release-1", "S1", 1, strings.Repeat("1", 40),
		"sha256:"+strings.Repeat("b", 64), results,
		"sha256:"+strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := protocol.ParseCheckResults(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Entries) != 2 {
		t.Fatalf("entries = %d", len(parsed.Entries))
	}
	host, role := parsed.Entries[0], parsed.Entries[1]
	if host.Provenance != protocol.CheckProvenanceHost ||
		host.Outcome != protocol.CheckOutcomePass ||
		host.HostEffect != "attempt/host/1/1" ||
		host.OutputDigest != results[0].OutputDigest {
		t.Fatalf("host entry = %#v", host)
	}
	if role.Provenance != protocol.CheckProvenanceRole ||
		role.RoleDigest != "sha256:"+strings.Repeat("c", 64) {
		t.Fatalf("role entry = %#v", role)
	}
	_ = pass
}

func TestParseHostCheckResultRejectsSubstitution(t *testing.T) {
	t.Parallel()

	original := hostCheckResult{
		Slice: "S1", Candidate: strings.Repeat("1", 40),
		ContractDigest: "sha256:" + strings.Repeat("b", 64),
		Check:          "go test ./...", Outcome: protocol.CheckOutcomePass,
		ExitCode: 0, Output: "all good\n",
		OutputDigest: protocol.DigestBytes([]byte("all good\n")),
		EffectID:     "attempt/host/1/1",
	}
	body := mustJSON(original)
	if _, err := parseHostCheckResult(
		original.Slice, original.Candidate, original.ContractDigest,
		original.Check, original.EffectID, body); err != nil {
		t.Fatalf("exact result rejected: %v", err)
	}
	substituted := original
	substituted.Output = "tampered\n"
	// The digest is not re-digested: an incoherent substitution (output that
	// does not match its claimed digest) must fail closed at parse time.
	if _, err := parseHostCheckResult(
		original.Slice, original.Candidate, original.ContractDigest,
		original.Check, original.EffectID, mustJSON(substituted)); err == nil {
		t.Fatal("incoherent substitution was accepted")
	}
}

var _ = context.Background

// TestHostCheckCommandWordSkipsLeadingAssignments is A1's parse-layer
// proof: the classifier's word is the check's first non-assignment token,
// exactly as the shell that runs the check would resolve it.
func TestHostCheckCommandWordSkipsLeadingAssignments(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"go test ./...":                                         "go",
		"GOFLAGS=-buildvcs=false go test ./...":                 "go",
		"GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build ./...": "go",
		"test -z \"$(gofmt -l ./cmd)\"":                         "test",
		"":                                                      "",
		"FOO=bar":                                               "",
		// S6-host-environment-park-projection A5(i): shell grouping
		// syntax - a subshell or a brace group - is honestly
		// unclassifiable, never a false environment-failure positive
		// naming the literal opener as a missing command.
		"(cd dir && make)":  "",
		"{ a; b; }":         "",
		"(cd dir && make) ": "",
		"{true;}":           "",
	}
	for check, want := range cases {
		if got := hostCheckCommandWord(check); got != want {
			t.Fatalf("hostCheckCommandWord(%q) = %q, want %q", check, got, want)
		}
	}
}

// TestClassifyHostCheckExecutionResolvesShellBuiltinsAndReservedWords is
// A1's exact required proof: the shell's own builtin/keyword/function/
// alias resolution, via `command -v`, never misclassifies a builtin or a
// reserved word as a missing command, with no maintained list.
func TestClassifyHostCheckExecutionResolvesShellBuiltinsAndReservedWords(t *testing.T) {
	t.Parallel()
	shell, err := hostShell()
	if err != nil {
		t.Skip("no POSIX shell discoverable in this sandbox")
	}
	for _, word := range []string{"cd", "export", "set", ":", "[", "if", "!"} {
		environmentFailure, missing := classifyHostCheckExecution(shell, word+" true")
		if environmentFailure {
			t.Fatalf("builtin/reserved word %q misclassified as missing (missing=%q)", word, missing)
		}
	}
}

// TestClassifyHostCheckExecutionRecognizesAGenuinelyAbsentCommand proves
// the negative case still refuses: a command that genuinely does not
// resolve is classified as an environment failure naming it.
func TestClassifyHostCheckExecutionRecognizesAGenuinelyAbsentCommand(t *testing.T) {
	t.Parallel()
	shell, err := hostShell()
	if err != nil {
		t.Skip("no POSIX shell discoverable in this sandbox")
	}
	const absent = "sworn-genuinely-absent-command-xyz"
	environmentFailure, missing := classifyHostCheckExecution(shell, absent+" ./...")
	if !environmentFailure || missing != absent {
		t.Fatalf("classify(%q) = (%v, %q), want (true, %q)", absent, environmentFailure, missing, absent)
	}
}

// TestClassifyHostCheckExecutionResolvesRealCommandsAndAssignmentPrefixes
// proves the positive case for a real, resolvable command, including one
// prefixed by NAME=value assignments, exactly like this release's own
// contract checks.
func TestClassifyHostCheckExecutionResolvesRealCommandsAndAssignmentPrefixes(t *testing.T) {
	t.Parallel()
	shell, err := hostShell()
	if err != nil {
		t.Skip("no POSIX shell discoverable in this sandbox")
	}
	for _, check := range []string{"true", "FOO=bar BAZ=qux true"} {
		if environmentFailure, missing := classifyHostCheckExecution(shell, check); environmentFailure {
			t.Fatalf("classify(%q) misclassified true as missing (missing=%q)", check, missing)
		}
	}
}

// TestHostCommandResolvesPathShapedWordUsesStat proves a path-shaped word
// (containing '/') is resolved by os.Stat, exactly as the shell would
// attempt to execute it directly, not by a PATH search.
func TestHostCommandResolvesPathShapedWordUsesStat(t *testing.T) {
	t.Parallel()
	shell, err := hostShell()
	if err != nil {
		t.Skip("no POSIX shell discoverable in this sandbox")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "present.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntrue\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !hostCommandResolves(shell, script) {
		t.Fatalf("path-shaped present script %q did not resolve", script)
	}
	if hostCommandResolves(shell, filepath.Join(dir, "absent.sh")) {
		t.Fatal("path-shaped absent script resolved")
	}
}

// TestExecuteHostCheckFailsClosedWithTypedCodeWhenHostShellIsUnavailable is
// A5(ii)'s exact required proof for the fresh-claim site: when the host
// shell itself cannot be resolved, executeHostCheck (via runHostChecks)
// fails closed with the identical typed code validateHostCheckEnvironment
// (the run-start gate) already uses, instead of silently skipping
// classification and running the command unclassified. Nothing is ever
// completed for this failure, so it cannot spend a try or become repair
// input: the check.host effect stays exactly Claimed, never Succeeded or
// OperationalFailed.
func TestExecuteHostCheckFailsClosedWithTypedCodeWhenHostShellIsUnavailable(t *testing.T) {
	check := "true"
	fixture := newHostCheckFixture(t, []string{check})
	t.Setenv("SWORN_SH", filepath.Join(t.TempDir(), "no-such-shell"))

	_, err := fixture.service.runHostChecks(
		fixture.ctx, fixture.engine, fixture.owner, fixture.plan,
		"S1", fixture.candidate, fixture.targetHead, fixture.releaseHead)
	if !IsCode(err, "HOST_SHELL_UNAVAILABLE") {
		t.Fatalf("runHostChecks() error = %v, want HOST_SHELL_UNAVAILABLE", err)
	}
	work := hostCheckWork("S1", fixture.candidate, fixture.contractDgst, check)
	effect, err := fixture.store.Effect(fixture.ctx, fixture.owner.RunID, hostCheckEffectID(work))
	if err != nil {
		t.Fatal(err)
	}
	if effect.State != journal.Claimed || len(effect.Result) != 0 || effect.ErrorCode != "" {
		t.Fatalf("effect after HOST_SHELL_UNAVAILABLE = %#v, want Claimed and never completed", effect)
	}
}

// TestRecoverHostCheckClaimsFailsClosedWithTypedCodeWhenHostShellIsUnavailable
// is A5(ii)'s exact required proof for the crash-recovery site: a claimed
// check.host effect from a crashed prior attempt fails closed with the
// identical typed code instead of silently skipping classification and
// falling through to executeHostCheckFromRecovery.
func TestRecoverHostCheckClaimsFailsClosedWithTypedCodeWhenHostShellIsUnavailable(t *testing.T) {
	fixture := newHostCheckFixture(t, []string{"true"})
	work := hostCheckWork("S1", fixture.candidate, fixture.contractDgst, "true")
	effectID := hostCheckEffectID(work)
	command := hostCheckCommand{
		SchemaVersion: hostCheckSchemaVersion, Slice: "S1",
		Candidate: fixture.candidate, ContractDigest: fixture.contractDgst,
		Check: "true", OutputBytes: hostCheckOutputBytes, TimeoutMillis: 30_000,
	}
	payload := mustJSON(command)
	if err := fixture.store.EnsureAttempt(fixture.ctx,
		journal.Command{RunID: fixture.owner.RunID, ReplayKey: effectID,
			Kind: "check.host", Payload: payload, CreatedAt: fixture.service.now().UTC()},
		journal.Effect{RunID: fixture.owner.RunID, ID: effectID, ReplayKey: effectID,
			Kind: "check.host", BeforeDigest: work,
			ExpectedDigest: sha256Digest(payload), UpdatedAt: fixture.service.now().UTC()},
		journal.EffectAttempt{WorkID: work, Epoch: 1, Try: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.ClaimOwned(
		fixture.ctx, fixture.owner, effectID, fixture.service.now().UTC(), effectLease,
	); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SWORN_SH", filepath.Join(t.TempDir(), "no-such-shell"))
	if _, err := fixture.service.recoverHostCheckClaims(
		fixture.ctx, fixture.engine, fixture.owner,
	); !IsCode(err, "HOST_SHELL_UNAVAILABLE") {
		t.Fatalf("recoverHostCheckClaims() error = %v, want HOST_SHELL_UNAVAILABLE", err)
	}
	effect, err := fixture.store.Effect(fixture.ctx, fixture.owner.RunID, effectID)
	if err != nil {
		t.Fatal(err)
	}
	if effect.State != journal.Claimed || len(effect.Result) != 0 || effect.ErrorCode != "" {
		t.Fatalf("effect after HOST_SHELL_UNAVAILABLE = %#v, want Claimed and never completed", effect)
	}
}
