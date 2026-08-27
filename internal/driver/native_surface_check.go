package driver

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// maxNativeSurfaceHeadBytes bounds the offending-request/event head a
// NATIVE_SURFACE_INVALID may carry, well under the funnel's overall
// maxProviderErrorDetailBytes re-validation bound once base64-encoded and
// wrapped in the envelope.
const maxNativeSurfaceHeadBytes = 256

// nativeSurfaceDetail is the canonical-JSON envelope a NATIVE_SURFACE_INVALID
// carries as Detail: the specific structural check that refused, and an
// optional bounded, secret-free head of the offending request or event.
// Detail-preservation at the normalizeAdapterError funnel re-validates this
// shape structurally rather than treating it as opaque provider text.
type nativeSurfaceDetail struct {
	Check string `json:"check"`
	Head  string `json:"head,omitempty"`
}

// validNativeSurfaceCheck is the closed vocabulary of structural checks a
// NATIVE_SURFACE_INVALID may name. An unrecognized or misspelled name fails
// safe: revalidateNativeSurfaceDetail drops the whole Detail, matching the
// existing "flattens outside the admitted set" doctrine, so a raise site
// bug can never leak anything, only lose evidence on that one site.
func validNativeSurfaceCheck(check string) bool {
	switch check {
	case
		// nativeContinuationState / crash-recovery home checks.
		"continuation.claim_size_unreadable",
		"continuation.credential_target_not_placeholder",
		"continuation.retained_home_secret_leak",
		"continuation.retained_home_invalid",
		"continuation.retained_home_secret_present",
		// runNative dispatch-terminal checks.
		"dispatch.sandbox_process_group_unreadable",
		"dispatch.stderr_secret_leak",
		"dispatch.event_stream_scan_failed",
		"dispatch.session_identity_not_established",
		"dispatch.tool_session_missing",
		"dispatch.process_signaled",
		// certification capture evidence checks.
		"capture.provider_evidence_invalid",
		"capture.handshake_evidence_unavailable",
		"capture.evidence_encoding_failed",
		// nativeConfigSurfaceDigest checks.
		"config_surface.file_count_mismatch",
		"config_surface.seek_failed",
		"config_surface.read_failed",
		"config_surface.seek_reset_failed",
		// certifyNativeRuntime checks.
		"runtime.preconditions_invalid",
		"runtime.namespace_not_isolated",
		"runtime.cmdline_leak_or_unreadable",
		"runtime.expected_arguments_unavailable",
		"runtime.cmdline_command_mismatch",
		"runtime.environment_leak_or_unreadable",
		"runtime.mountinfo_leak_or_unreadable",
		"runtime.workspace_not_empty",
		"runtime.automation_workspace_not_read_only",
		"runtime.input_path_present",
		"runtime.shell_present",
		"runtime.config_file_invalid",
		"runtime.codex_config_capability_count",
		"runtime.claude_config_capability_leak",
		"runtime.unsupported_family",
		"runtime.agent_or_credential_identity_mismatch",
		"runtime.continuation_home_identity_mismatch",
		"runtime.runtime_file_identity_mismatch",
		"runtime.fd_table_unreadable",
		"runtime.fd_leak_detected",
		"runtime.proc_file_unreadable",
		// validateNativeProcessEnvironment checks.
		"runtime.environment_claude_capability_leak",
		"runtime.environment_codex_capability_leak",
		"runtime.environment_unsupported_family",
		"runtime.environment_entry_count_mismatch",
		"runtime.environment_entry_malformed",
		"runtime.environment_entry_unexpected",
		"runtime.environment_entry_duplicate",
		// scanNativeEvents / state.accept / acceptSessionID checks.
		"stream.secret_leak_detected",
		"event.malformed_json",
		"event.not_object",
		"event.claude_init_shape_invalid",
		"event.claude_init_field_mismatch",
		"event.claude_session_id_invalid",
		"event.codex_thread_started_invalid",
		"event.codex_session_id_invalid",
		"event.codex_item_shape_invalid",
		"event.codex_item_type_disallowed",
		"event.codex_turn_completed_invalid",
		"event.unsupported_family",
		"session.identity_invalid",
		"session.identity_mismatch",
		// native_capture_linux.go validateNativeProviderRequest checks.
		"capture_request.malformed_json",
		"capture_request.not_object",
		"capture_request.model_mismatch",
		"capture_request.tools_not_array",
		"capture_request.codex_tools_mismatch",
		"capture_request.claude_tools_mismatch",
		"capture_request.unsupported_family":
		return true
	default:
		return false
	}
}

// nativeSurfaceDetailBytes encodes the check-name-plus-bounded-head envelope
// as compact canonical JSON. head is truncated to maxNativeSurfaceHeadBytes
// before encoding; callers that need redaction apply it before calling this.
func nativeSurfaceDetailBytes(check string, head []byte) string {
	envelope := nativeSurfaceDetail{Check: check}
	if len(head) > 0 {
		bounded := head
		if len(bounded) > maxNativeSurfaceHeadBytes {
			bounded = bounded[:maxNativeSurfaceHeadBytes]
		}
		envelope.Head = base64.StdEncoding.EncodeToString(bounded)
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return ""
	}
	return string(body)
}

// failNativeSurface builds a NATIVE_SURFACE_INVALID naming check, with no
// offending bytes: the shape for checks with no natural evidence to bound
// (process/namespace/proc-fs identity checks) or where the offending bytes
// are themselves exactly the material the check exists to keep off the
// durable record.
func failNativeSurface(check string) error {
	return &ContractError{Code: "NATIVE_SURFACE_INVALID", Detail: nativeSurfaceDetailBytes(check, nil)}
}

// failNativeSurfaceHead builds a NATIVE_SURFACE_INVALID naming check with a
// bounded, secret-redacted head of the offending request or event. secrets
// (when any) are redacted out of the head before truncation via the same
// redactToolResultSpan discipline the tool-result projection uses, so a
// held credential can never ride the durable record even truncated.
func failNativeSurfaceHead(check string, offending []byte, secrets ...[]byte) error {
	head := offending
	if len(head) > maxNativeSurfaceHeadBytes {
		head = head[:maxNativeSurfaceHeadBytes]
	}
	if set := redactionSecretSet(secrets...); len(set) > 0 {
		head, _ = redactToolResultSpan(head, set)
	}
	return &ContractError{Code: "NATIVE_SURFACE_INVALID", Detail: nativeSurfaceDetailBytes(check, head)}
}

// revalidateNativeSurfaceDetail structurally re-validates a NATIVE_SURFACE_INVALID
// Detail at the normalizeAdapterError funnel: it must decode as the closed
// envelope shape, name a check in the closed vocabulary, and (when present)
// carry a head that decodes as base64 within maxNativeSurfaceHeadBytes.
// Anything else drops the Detail entirely, matching the existing
// "flattens outside the admitted set" doctrine rather than truncating a
// malformed claim into a valid-looking one.
func revalidateNativeSurfaceDetail(detail string) (string, bool) {
	if detail == "" || len(detail) > 4*maxNativeSurfaceHeadBytes {
		return "", false
	}
	var envelope nativeSurfaceDetail
	decoder := json.NewDecoder(strings.NewReader(detail))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || decoder.More() {
		return "", false
	}
	if !validNativeSurfaceCheck(envelope.Check) {
		return "", false
	}
	if envelope.Head != "" {
		raw, err := base64.StdEncoding.DecodeString(envelope.Head)
		if err != nil || len(raw) > maxNativeSurfaceHeadBytes {
			return "", false
		}
	}
	canonical, err := json.Marshal(envelope)
	if err != nil {
		return "", false
	}
	return string(canonical), true
}
