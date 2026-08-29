package driver

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestValidSandboxStartCheckClosesTheVocabulary(t *testing.T) {
	t.Parallel()
	admitted := []string{
		"sandbox_start.invocation_scratch_create",
		"sandbox_start.home_tmp_surface_create",
		"sandbox_start.mask_devnull_open",
		"sandbox_start.status_pipe_create",
		"sandbox_start.bwrap_exec_start",
		"sandbox_start.process_group_handshake_read",
	}
	for _, check := range admitted {
		if !validSandboxStartCheck(check) {
			t.Fatalf("admitted check rejected: %q", check)
		}
	}
	for _, rejected := range []string{
		"", "sandbox_start.", "sandbox_start.unknown",
		"dispatch.process_signaled", "SANDBOX_START.BWRAP_EXEC_START",
	} {
		if validSandboxStartCheck(rejected) {
			t.Fatalf("unadmitted check accepted: %q", rejected)
		}
	}
}

// TestSandboxStartCauseIsPathFreeByConstruction is the A2/correction-1 proof:
// a wrapped *os.PathError carrying a host path yields a cause containing no
// substring of that path, because sandboxStartCause extracts only the
// innermost syscall.Errno's own short message, never the wrapping error.
func TestSandboxStartCauseIsPathFreeByConstruction(t *testing.T) {
	t.Parallel()
	hostPath := "/home/sworn/very-secret-invocation-scratch-directory-name"
	wrapped := &os.PathError{
		Op: "mkdir", Path: hostPath, Err: syscall.ENOTDIR,
	}
	cause := sandboxStartCause(wrapped)
	if cause == "" {
		t.Fatal("no cause extracted from a wrapped syscall.Errno")
	}
	if strings.Contains(cause, hostPath) || strings.ContainsAny(cause, "/") {
		t.Fatalf("cause carries a path: %q", cause)
	}
}

func TestSandboxStartCauseHasNoFallback(t *testing.T) {
	t.Parallel()
	if cause := sandboxStartCause(nil); cause != "" {
		t.Fatalf("nil error yielded a cause: %q", cause)
	}
	if cause := sandboxStartCause(errors.New("opaque failure, not a kernel errno")); cause != "" {
		t.Fatalf("non-errno error yielded a fallback cause: %q", cause)
	}
	if cause := sandboxStartCause(&ContractError{Code: "PROCESS_START_FAILED"}); cause != "" {
		t.Fatalf("a ContractError with no wrapped errno yielded a cause: %q", cause)
	}
}

func TestFailSandboxStartRoundTripsEveryCheck(t *testing.T) {
	t.Parallel()
	checks := []string{
		"sandbox_start.invocation_scratch_create",
		"sandbox_start.home_tmp_surface_create",
		"sandbox_start.mask_devnull_open",
		"sandbox_start.status_pipe_create",
		"sandbox_start.bwrap_exec_start",
		"sandbox_start.process_group_handshake_read",
	}
	for _, check := range checks {
		check := check
		t.Run(check, func(t *testing.T) {
			t.Parallel()
			err := failSandboxStart(check, &os.PathError{
				Op: "open", Path: "/tmp/x", Err: syscall.ENOTDIR,
			})
			var contractErr *ContractError
			if !errors.As(err, &contractErr) || contractErr.Code != "PROCESS_START_FAILED" {
				t.Fatalf("err = %#v", err)
			}
			detail, ok := revalidateSandboxStartDetail(contractErr.Detail)
			if !ok {
				t.Fatalf("detail failed re-validation: %q", contractErr.Detail)
			}
			if !strings.Contains(detail, `"check":"`+check+`"`) {
				t.Fatalf("detail = %q, want check %q", detail, check)
			}
		})
	}
}

func TestRevalidateSandboxStartDetailStructural(t *testing.T) {
	t.Parallel()
	t.Run("valid envelope round-trips", func(t *testing.T) {
		t.Parallel()
		detail := sandboxStartDetailBytes("sandbox_start.bwrap_exec_start", "no such file or directory")
		got, ok := revalidateSandboxStartDetail(detail)
		if !ok || got != detail {
			t.Fatalf("revalidate = %q, %v", got, ok)
		}
	})
	t.Run("valid envelope with empty cause round-trips", func(t *testing.T) {
		t.Parallel()
		detail := sandboxStartDetailBytes("sandbox_start.process_group_handshake_read", "")
		got, ok := revalidateSandboxStartDetail(detail)
		if !ok || got != detail || strings.Contains(got, "cause") {
			t.Fatalf("revalidate = %q, %v", got, ok)
		}
	})
	t.Run("unknown check drops the detail", func(t *testing.T) {
		t.Parallel()
		detail := sandboxStartDetailBytes("sandbox_start.not_a_real_site", "")
		if _, ok := revalidateSandboxStartDetail(detail); ok {
			t.Fatal("unknown check accepted")
		}
	})
	t.Run("malformed json drops the detail", func(t *testing.T) {
		t.Parallel()
		if _, ok := revalidateSandboxStartDetail("not json"); ok {
			t.Fatal("malformed json accepted")
		}
	})
	t.Run("unknown field drops the detail", func(t *testing.T) {
		t.Parallel()
		if _, ok := revalidateSandboxStartDetail(
			`{"check":"sandbox_start.bwrap_exec_start","extra":"x"}`,
		); ok {
			t.Fatal("unknown field accepted")
		}
	})
	t.Run("oversize cause drops the detail", func(t *testing.T) {
		t.Parallel()
		if _, ok := revalidateSandboxStartDetail(
			`{"check":"sandbox_start.bwrap_exec_start","cause":"` +
				strings.Repeat("x", maxSandboxStartCauseBytes*2) + `"}`,
		); ok {
			t.Fatal("oversize cause accepted")
		}
	})
	t.Run("empty detail drops", func(t *testing.T) {
		t.Parallel()
		if _, ok := revalidateSandboxStartDetail(""); ok {
			t.Fatal("empty detail accepted")
		}
	})
}

// TestNormalizeAdapterErrorPreservesSandboxStartDetail is the A2 funnel
// proof: a valid envelope survives normalizeAdapterError with check and
// cause intact, an unknown check or oversize cause drops the whole Detail
// while keeping Code and Kind, and every other PROCESS_START_FAILED site
// (which still raises Detail=="") stays byte-identical to today.
func TestNormalizeAdapterErrorPreservesSandboxStartDetail(t *testing.T) {
	t.Parallel()
	t.Run("valid envelope survives with check and cause intact", func(t *testing.T) {
		t.Parallel()
		in := &ContractError{
			Code:   "PROCESS_START_FAILED",
			Detail: sandboxStartDetailBytes("sandbox_start.status_pipe_create", "too many open files"),
		}
		out := normalizeAdapterError(in)
		got, ok := out.(*ContractError)
		if !ok || got.Code != "PROCESS_START_FAILED" || got.Kind != KindTransport ||
			got.Detail != in.Detail {
			t.Fatalf("normalized = %#v", out)
		}
	})
	t.Run("unknown check drops detail, keeps code and kind", func(t *testing.T) {
		t.Parallel()
		in := &ContractError{
			Code:   "PROCESS_START_FAILED",
			Detail: sandboxStartDetailBytes("sandbox_start.not_a_real_site", "x"),
		}
		out := normalizeAdapterError(in)
		got, ok := out.(*ContractError)
		if !ok || got.Code != "PROCESS_START_FAILED" || got.Kind != KindTransport || got.Detail != "" {
			t.Fatalf("normalized = %#v", out)
		}
	})
	t.Run("oversize cause drops detail, keeps code and kind", func(t *testing.T) {
		t.Parallel()
		in := &ContractError{
			Code: "PROCESS_START_FAILED",
			Detail: `{"check":"sandbox_start.bwrap_exec_start","cause":"` +
				strings.Repeat("x", maxSandboxStartCauseBytes*2) + `"}`,
		}
		out := normalizeAdapterError(in)
		got, ok := out.(*ContractError)
		if !ok || got.Code != "PROCESS_START_FAILED" || got.Kind != KindTransport || got.Detail != "" {
			t.Fatalf("normalized = %#v", out)
		}
	})
	t.Run("bare fail at any other site stays byte-identical", func(t *testing.T) {
		t.Parallel()
		in := fail("PROCESS_START_FAILED")
		out := normalizeAdapterError(in)
		got, ok := out.(*ContractError)
		if !ok || got.Code != "PROCESS_START_FAILED" || got.Kind != KindTransport || got.Detail != "" {
			t.Fatalf("normalized = %#v, want bare code/kind with no detail", out)
		}
	})
}

func TestToolErrorContentAppendsDetailOnlyWhenPresent(t *testing.T) {
	t.Parallel()
	t.Run("no detail keeps the bare code", func(t *testing.T) {
		t.Parallel()
		if got := string(toolErrorContent(fail("INVALID_TOOL_ARGUMENT"))); got != "error:INVALID_TOOL_ARGUMENT" {
			t.Fatalf("content = %q", got)
		}
	})
	t.Run("a detail-carrying error appends it", func(t *testing.T) {
		t.Parallel()
		detail := sandboxStartDetailBytes("sandbox_start.mask_devnull_open", "too many open files")
		err := &ContractError{Code: "PROCESS_START_FAILED", Detail: detail}
		want := "error:PROCESS_START_FAILED detail=" + detail
		if got := string(toolErrorContent(err)); got != want {
			t.Fatalf("content = %q, want %q", got, want)
		}
	})
}
