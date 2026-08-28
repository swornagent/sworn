package driver

import (
	"encoding/json"
	"errors"
	"strings"
	"syscall"
)

// maxSandboxStartCauseBytes bounds the kernel-reported reason a Bash-tool
// sandbox start PROCESS_START_FAILED refusal may carry, well under the
// funnel's detail re-validation bound once wrapped in the envelope.
const maxSandboxStartCauseBytes = 256

// sandboxStartDetail is the canonical-JSON envelope a Bash-tool sandbox
// start PROCESS_START_FAILED carries as Detail: the specific site that
// refused, and the kernel's own bounded, secret-free reason when one was
// extractable. It is a distinct closed vocabulary from NATIVE_SURFACE_INVALID's
// nativeSurfaceDetail (native_surface_check.go), mirroring its idiom for a
// different code and family rather than sharing its check names.
type sandboxStartDetail struct {
	Check string `json:"check"`
	Cause string `json:"cause,omitempty"`
}

// validSandboxStartCheck is the closed vocabulary of the six Bash-tool
// sandbox start sites a PROCESS_START_FAILED carrying Detail may name. An
// unrecognized or misspelled name fails safe: revalidateSandboxStartDetail
// drops the whole Detail, so a raise-site bug can only lose evidence on
// that one site, never leak an unnamed one.
func validSandboxStartCheck(check string) bool {
	switch check {
	case
		// newToolSession's per-invocation scratch surface.
		"sandbox_start.invocation_scratch_create",
		"sandbox_start.home_tmp_surface_create",
		// runToolBash's sandbox launch.
		"sandbox_start.mask_devnull_open",
		"sandbox_start.status_pipe_create",
		"sandbox_start.bwrap_exec_start",
		// Materially distinct from bwrap_exec_start: the process has
		// already started and failed to report its group, not failed to
		// exec at all.
		"sandbox_start.process_group_handshake_read":
		return true
	default:
		return false
	}
}

// sandboxStartCause renders the kernel's own reason for a sandbox-start
// failure, bounded and secret-free by construction rather than by
// redaction: it extracts nothing but the innermost syscall.Errno's own
// short message (e.g. "permission denied", "file name too long"), never
// the wrapping os/exec error, which typically carries a host path. There is
// deliberately no fallback to err.Error() on any path - an error that does
// not wrap a syscall.Errno (a malformed status read, a nil argument) yields
// no cause at all rather than a best-effort rendering that might carry one.
// The redaction set applied elsewhere in this package (redactionSecretSet,
// fed by credentials runNative holds) is not in hand at these sites, and
// would be the wrong tool regardless: a held credential is never present in
// an errno string, but a host path routinely is, and no redaction pass
// scrubs paths.
func sandboxStartCause(err error) string {
	if err == nil {
		return ""
	}
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return ""
	}
	cause := errno.Error()
	if validateText(cause, maxSandboxStartCauseBytes, false) != nil {
		return ""
	}
	return cause
}

// sandboxStartDetailBytes encodes the check-plus-bounded-cause envelope as
// compact canonical JSON.
func sandboxStartDetailBytes(check string, cause string) string {
	envelope := sandboxStartDetail{Check: check, Cause: cause}
	body, err := json.Marshal(envelope)
	if err != nil {
		return ""
	}
	return string(body)
}

// failSandboxStart builds a PROCESS_START_FAILED naming one of the six
// Bash-tool sandbox start sites, with the kernel's own bounded reason when
// cause yields one. The code PROCESS_START_FAILED itself never changes:
// this only adds a named, structured Detail to a refusal already raised.
func failSandboxStart(check string, cause error) error {
	return &ContractError{
		Code:   "PROCESS_START_FAILED",
		Detail: sandboxStartDetailBytes(check, sandboxStartCause(cause)),
	}
}

// revalidateSandboxStartDetail structurally re-validates a PROCESS_START_FAILED
// Detail at the normalizeAdapterError funnel: it must decode as the closed
// envelope shape, name a check in the closed vocabulary, and (when present)
// carry a cause within maxSandboxStartCauseBytes. Anything else - including
// the empty Detail every PROCESS_START_FAILED site outside the six Bash-tool
// sandbox sites still raises - drops the Detail entirely, so those sites'
// behavior is unchanged.
func revalidateSandboxStartDetail(detail string) (string, bool) {
	if detail == "" || len(detail) > 2*maxSandboxStartCauseBytes {
		return "", false
	}
	var envelope sandboxStartDetail
	decoder := json.NewDecoder(strings.NewReader(detail))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || decoder.More() {
		return "", false
	}
	if !validSandboxStartCheck(envelope.Check) {
		return "", false
	}
	if envelope.Cause != "" && validateText(envelope.Cause, maxSandboxStartCauseBytes, false) != nil {
		return "", false
	}
	canonical, err := json.Marshal(envelope)
	if err != nil {
		return "", false
	}
	return string(canonical), true
}
