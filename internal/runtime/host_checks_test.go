package runtime

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/gitx"
)

func TestRunHostCommandPassFailAndOverflow(t *testing.T) {
	t.Parallel()

	t.Run("pass", func(t *testing.T) {
		result := runHostCommand(t.TempDir(), "printf 'ok\\n'", hostCheckOutputBytes, 5*time.Second)
		if result.Outcome != baton.CheckOutcomePass || result.ExitCode != 0 {
			t.Fatalf("pass result = %#v", result)
		}
		if !strings.Contains(result.Output, "ok") {
			t.Fatalf("pass output = %q", result.Output)
		}
		if result.OutputDigest != baton.DigestBytes([]byte(result.Output)) {
			t.Fatal("output digest does not match output")
		}
	})

	t.Run("fail", func(t *testing.T) {
		result := runHostCommand(t.TempDir(), "exit 7", hostCheckOutputBytes, 5*time.Second)
		if result.Outcome != baton.CheckOutcomeFail || result.ExitCode != 7 {
			t.Fatalf("fail result = %#v", result)
		}
	})

	t.Run("overflow", func(t *testing.T) {
		result := runHostCommand(t.TempDir(), "yes x | head -c 1000000", 4096, 5*time.Second)
		if result.Outcome != baton.CheckOutcomeOverflow {
			t.Fatalf("overflow result = %#v", result)
		}
		if !result.Truncated {
			t.Fatal("overflow was not marked truncated")
		}
		if !strings.Contains(result.Output, baton.HostCheckTruncationPrefix) {
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
		if result.Outcome != baton.CheckOutcomeFail || result.ExitCode != -1 {
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
		if result.Outcome != baton.CheckOutcomeFail || result.ExitCode != -1 {
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
		if result.Outcome != baton.CheckOutcomePass || result.ExitCode != 0 {
			t.Fatalf("override result = %#v", result)
		}
	})
}

func TestRunHostCommandTimeoutIsRecordedAsTimeout(t *testing.T) {
	t.Parallel()
	result := runHostCommand(t.TempDir(), "sleep 10", hostCheckOutputBytes, 300*time.Millisecond)
	if result.Outcome != baton.CheckOutcomeTimeout {
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
	if baton.DigestBytes([]byte(excerpt)) != baton.DigestBytes([]byte(full)) {
		t.Fatal("excerpt digest differs")
	}

	big := strings.Repeat("x", baton.HostCheckOutputManifestBytes+10)
	excerpt, truncated = hostOutputExcerpt(big, false)
	if !truncated {
		t.Fatal("large output was not marked truncated")
	}
	if !strings.Contains(excerpt, baton.HostCheckTruncationPrefix) {
		t.Fatalf("large output excerpt lacks marker: %q", excerpt)
	}
}

func TestBuildHostCheckResultsManifestBindsHostAndRoleEntries(t *testing.T) {
	t.Parallel()

	pass := int(0)
	results := []hostCheckResult{{
		Slice: "S1", Candidate: strings.Repeat("1", 40),
		ContractDigest: "sha256:" + strings.Repeat("b", 64),
		Check:          "go test ./...", Outcome: baton.CheckOutcomePass,
		ExitCode: 0, Output: "all good\n",
		OutputDigest: baton.DigestBytes([]byte("all good\n")),
		EffectID:     "attempt/host/1/1",
	}}
	manifest, err := buildHostCheckResultsManifest(
		"release-1", "S1", 1, strings.Repeat("1", 40),
		"sha256:"+strings.Repeat("b", 64), results,
		"sha256:"+strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := baton.ParseCheckResults(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Entries) != 2 {
		t.Fatalf("entries = %d", len(parsed.Entries))
	}
	host, role := parsed.Entries[0], parsed.Entries[1]
	if host.Provenance != baton.CheckProvenanceHost ||
		host.Outcome != baton.CheckOutcomePass ||
		host.HostEffect != "attempt/host/1/1" ||
		host.OutputDigest != results[0].OutputDigest {
		t.Fatalf("host entry = %#v", host)
	}
	if role.Provenance != baton.CheckProvenanceRole ||
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
		Check:          "go test ./...", Outcome: baton.CheckOutcomePass,
		ExitCode: 0, Output: "all good\n",
		OutputDigest: baton.DigestBytes([]byte("all good\n")),
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
