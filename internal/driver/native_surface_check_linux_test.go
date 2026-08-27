//go:build linux

package driver

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// decodeNativeSurfaceDetail asserts err is a NATIVE_SURFACE_INVALID whose
// Detail decodes as the closed envelope, and returns the decoded check/head.
func decodeNativeSurfaceDetail(t *testing.T, err error) nativeSurfaceDetail {
	t.Helper()
	var contractErr *ContractError
	if !errors.As(err, &contractErr) || contractErr.Code != "NATIVE_SURFACE_INVALID" {
		t.Fatalf("error = %#v, want NATIVE_SURFACE_INVALID", err)
	}
	var envelope nativeSurfaceDetail
	if err := json.Unmarshal([]byte(contractErr.Detail), &envelope); err != nil {
		t.Fatalf("detail %q did not decode: %v", contractErr.Detail, err)
	}
	if !validNativeSurfaceCheck(envelope.Check) {
		t.Fatalf("check %q is not in the closed vocabulary", envelope.Check)
	}
	return envelope
}

func TestValidateNativeProviderRequestNamesCheckWithBoundedRedactedHead(t *testing.T) {
	t.Parallel()
	token := []byte("capture-bearer-canary-0123456789")
	t.Run("malformed json names its check with a bounded head", func(t *testing.T) {
		t.Parallel()
		body := []byte(`not json, but carries a canary ` + string(token))
		_, _, err := validateNativeProviderRequest(
			body, ProfileClaude, "model", toolDefinitions(ReadWrite), token,
		)
		envelope := decodeNativeSurfaceDetail(t, err)
		if envelope.Check != "capture_request.malformed_json" {
			t.Fatalf("check = %q", envelope.Check)
		}
		head, decodeErr := base64.StdEncoding.DecodeString(envelope.Head)
		if decodeErr != nil {
			t.Fatalf("head did not decode: %v", decodeErr)
		}
		if strings.Contains(string(head), string(token)) {
			t.Fatalf("secret leaked into head: %q", head)
		}
		if len(head) > maxNativeSurfaceHeadBytes {
			t.Fatalf("head length %d exceeds bound %d", len(head), maxNativeSurfaceHeadBytes)
		}
	})
	t.Run("model mismatch names its check", func(t *testing.T) {
		t.Parallel()
		body := []byte(`{"model":"wrong-model","tools":[]}`)
		_, _, err := validateNativeProviderRequest(
			body, ProfileCodex, "expected-model", toolDefinitions(ReadWrite),
		)
		envelope := decodeNativeSurfaceDetail(t, err)
		if envelope.Check != "capture_request.model_mismatch" {
			t.Fatalf("check = %q", envelope.Check)
		}
	})
	t.Run("unsupported family names its check", func(t *testing.T) {
		t.Parallel()
		body := []byte(`{"model":"m","tools":[{}]}`)
		_, _, err := validateNativeProviderRequest(
			body, ProfileFamily("unsupported"), "m", toolDefinitions(ReadWrite),
		)
		envelope := decodeNativeSurfaceDetail(t, err)
		if envelope.Check != "capture_request.unsupported_family" {
			t.Fatalf("check = %q", envelope.Check)
		}
	})
}

func TestStateAcceptNamesCheckWithBoundedHead(t *testing.T) {
	t.Parallel()
	invocation, _, _ := memoryInvocationFixture(t)
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	broker, err := newNativeBroker(session)
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	state := &nativeEventState{
		family:      ProfileClaude,
		model:       "native-model",
		definitions: toolDefinitions(ReadWrite),
		broker:      broker,
	}
	t.Run("malformed json event names its check", func(t *testing.T) {
		line := []byte(`not json`)
		err := state.accept(line)
		envelope := decodeNativeSurfaceDetail(t, err)
		if envelope.Check != "event.malformed_json" {
			t.Fatalf("check = %q", envelope.Check)
		}
		head, decodeErr := base64.StdEncoding.DecodeString(envelope.Head)
		if decodeErr != nil || string(head) != string(line) {
			t.Fatalf("head = %q, err = %v, want %q", head, decodeErr, line)
		}
	})
	t.Run("not-object event names its check", func(t *testing.T) {
		line := []byte(`[1,2,3]`)
		err := state.accept(line)
		envelope := decodeNativeSurfaceDetail(t, err)
		if envelope.Check != "event.not_object" {
			t.Fatalf("check = %q", envelope.Check)
		}
	})
}

func TestAcceptSessionIDNamesCheck(t *testing.T) {
	t.Parallel()
	t.Run("no launch names identity_invalid", func(t *testing.T) {
		t.Parallel()
		state := &nativeEventState{}
		err := state.acceptSessionID([]byte("not-a-uuid"))
		envelope := decodeNativeSurfaceDetail(t, err)
		if envelope.Check != "session.identity_invalid" {
			t.Fatalf("check = %q", envelope.Check)
		}
	})
	t.Run("resume mismatch names identity_mismatch", func(t *testing.T) {
		t.Parallel()
		state := &nativeEventState{
			launch: &nativeContinuationLaunch{
				resume:     true,
				expectedID: []byte("11111111-1111-4111-8111-111111111111"),
			},
		}
		err := state.acceptSessionID([]byte("22222222-2222-4222-8222-222222222222"))
		envelope := decodeNativeSurfaceDetail(t, err)
		if envelope.Check != "session.identity_mismatch" {
			t.Fatalf("check = %q", envelope.Check)
		}
	})
}

func TestNativeSpontaneousExitFailureCarriesStderrTailThroughFunnel(t *testing.T) {
	t.Parallel()
	waitErr := &exec.ExitError{}
	t.Run("stale credential outranks everything", func(t *testing.T) {
		t.Parallel()
		err := nativeSpontaneousExitFailure(true, ProfileClaude, waitErr, []byte("tail"))
		if !IsCode(err, "CREDENTIAL_STALE") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("transport carries the stderr tail through the funnel", func(t *testing.T) {
		t.Parallel()
		tail := []byte("the CLI's own words about what went wrong")
		err := nativeSpontaneousExitFailure(false, ProfileClaude, waitErr, tail)
		if !IsCode(err, "PROVIDER_TRANSPORT_FAILED") {
			t.Fatalf("error = %v", err)
		}
		normalized := normalizeAdapterError(err)
		var contractErr *ContractError
		if !errors.As(normalized, &contractErr) ||
			contractErr.Code != "PROVIDER_TRANSPORT_FAILED" ||
			contractErr.Detail != string(tail) ||
			contractErr.Kind != KindTransport {
			t.Fatalf("normalized = %#v", normalized)
		}
	})
	t.Run("empty tail carries no detail through the funnel", func(t *testing.T) {
		t.Parallel()
		err := nativeSpontaneousExitFailure(false, ProfileClaude, waitErr, nil)
		normalized := normalizeAdapterError(err)
		var contractErr *ContractError
		if !errors.As(normalized, &contractErr) ||
			contractErr.Code != "PROVIDER_TRANSPORT_FAILED" ||
			contractErr.Detail != "" {
			t.Fatalf("normalized = %#v", normalized)
		}
	})
}
