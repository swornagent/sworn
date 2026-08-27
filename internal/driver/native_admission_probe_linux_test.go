//go:build linux

package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func nativeAdmissionProbeTestSelection(
	t *testing.T,
	scriptBody string,
) SelectedProfile {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(executable, []byte(scriptBody), 0o700); err != nil {
		t.Fatal(err)
	}
	digest, err := executableDigest(executable)
	if err != nil {
		t.Fatal(err)
	}
	// The probe only opens and hashes these paths (openNativeClosure /
	// openPinnedRuntimeFile); it never inspects Target. Fabricated temp
	// files stand in for the real system runtime files so this test does
	// not depend on host paths like /etc being present.
	targets := []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/resolv.conf",
		"/etc/hosts",
		"/etc/nsswitch.conf",
	}
	files := make([]PinnedRuntimeFile, len(targets))
	required := make([]string, len(targets))
	for index, target := range targets {
		sourcePath := filepath.Join(t.TempDir(), filepath.Base(target))
		if err := os.WriteFile(sourcePath, []byte(target+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		sourceDigest, err := executableDigest(sourcePath)
		if err != nil {
			t.Fatal(err)
		}
		files[index] = PinnedRuntimeFile{Path: sourcePath, Target: target, Digest: sourceDigest}
		required[index] = target
	}
	config := NativeAdapterConfig{
		Key: "a-claude-probe", ID: "sworn.claude", Version: "1.0.0",
		Family:                 ProfileClaude,
		CLI:                    ExecutableIdentity{Path: executable, Digest: digest},
		CLIVersion:             "2.1.999",
		VersionOutput:          "probe-fixture-version",
		RuntimeFiles:           files,
		RequiredRuntimeTargets: required,
		CredentialTarget:       ClaudeCredentialTarget,
		CredentialRefs:         []string{"claude-file"},
		MaxCredentialBytes:     1_048_576,
	}
	identity := AdapterIdentity{
		Key: config.Key, ID: config.ID, Version: config.Version,
		ConfigurationDigest: "sha256:" + string(bytes.Repeat([]byte("c"), 64)),
	}
	adapter := &nativeAdapter{
		identity: identity,
		config:   config,
		resolve: func(context.Context, string) (string, error) {
			return "", nil
		},
		refs: map[string]struct{}{"claude-file": {}},
	}
	return SelectedProfile{
		Profile: ProfileConfig{Key: "probe-profile", Adapter: identity.Key, Network: NetworkRequired},
		Adapter: identity,
		Model:   "probe-model",
		adapter: adapter,
	}
}

func decodeNativeAdmissionProbeEvent(t *testing.T, body []byte) NativeAdmissionProbeEvent {
	t.Helper()
	var event NativeAdmissionProbeEvent
	if err := json.Unmarshal(body, &event); err != nil {
		t.Fatalf("decode probe event: %v", err)
	}
	return event
}

// TestProbeNativeAdmissionRefusesAScriptedDeadCLI pins A3's named-refusal
// half: a pinned binary that executes but cannot report its version (a
// nonzero exit) refuses at admission with NATIVE_PIN_DEAD, and the probe
// result is journaled as a refusal.
func TestProbeNativeAdmissionRefusesAScriptedDeadCLI(t *testing.T) {
	selected := nativeAdmissionProbeTestSelection(t, "#!/usr/bin/sh\nexit 7\n")
	body, err := ProbeNativeAdmission(context.Background(), selected, "run-dead-pin")
	if !IsCode(err, "NATIVE_PIN_DEAD") {
		t.Fatalf("dead CLI probe error = %v, want NATIVE_PIN_DEAD", err)
	}
	if body == nil {
		t.Fatal("expected a journaled probe event for a dead pin")
	}
	event := decodeNativeAdmissionProbeEvent(t, body)
	if event.Outcome != nativeAdmissionProbeRefused || event.RunID != "run-dead-pin" {
		t.Fatalf("probe event = %#v, want a refused outcome for run-dead-pin", event)
	}
}

// TestProbeNativeAdmissionBoundExpiryIsUnevaluableNotRefusal pins A3's
// honestly-unevaluable half: the probe's own bound expiring never refuses -
// it admits, and the journal says so.
func TestProbeNativeAdmissionBoundExpiryIsUnevaluableNotRefusal(t *testing.T) {
	selected := nativeAdmissionProbeTestSelection(
		t, "#!/usr/bin/sh\necho probe-fixture-version\nexit 0\n",
	)
	expired, cancel := context.WithDeadline(
		context.Background(), time.Now().Add(-time.Second),
	)
	defer cancel()
	body, err := ProbeNativeAdmission(expired, selected, "run-expired-bound")
	if err != nil {
		t.Fatalf("expired-bound probe refused instead of admitting: %v", err)
	}
	if body == nil {
		t.Fatal("expected a journaled probe event for an expired bound")
	}
	event := decodeNativeAdmissionProbeEvent(t, body)
	if event.Outcome != nativeAdmissionProbeUnevaluable {
		t.Fatalf("probe event = %#v, want unevaluable", event)
	}
}

// TestProbeNativeAdmissionPassesALiveCLI pins the healthy path: a pinned
// binary that executes and reports its version within bound admits and is
// journaled as passed.
func TestProbeNativeAdmissionPassesALiveCLI(t *testing.T) {
	selected := nativeAdmissionProbeTestSelection(
		t, "#!/usr/bin/sh\necho probe-fixture-version\nexit 0\n",
	)
	body, err := ProbeNativeAdmission(context.Background(), selected, "run-live-pin")
	if err != nil {
		t.Fatalf("live CLI probe refused: %v", err)
	}
	event := decodeNativeAdmissionProbeEvent(t, body)
	if event.Outcome != nativeAdmissionProbePassed {
		t.Fatalf("probe event = %#v, want passed", event)
	}
}

// TestProbeNativeAdmissionIsANoOpForNonNativeAdapters pins the applicability
// gate: every other adapter kind returns (nil, nil), so no other dispatch
// path gains a new journal event or a new refusal.
func TestProbeNativeAdmissionIsANoOpForNonNativeAdapters(t *testing.T) {
	adapter := processAdapterFixture(t, "a-fake", "sworn.fake-probe")
	selected := SelectedProfile{
		Profile: ProfileConfig{Key: "fake-profile", Adapter: adapter.Identity().Key, Network: NetworkNone},
		Adapter: adapter.Identity(),
		Model:   "fake-model",
		adapter: adapter,
	}
	body, err := ProbeNativeAdmission(context.Background(), selected, "run-fake")
	if body != nil || err != nil {
		t.Fatalf("non-native probe = (%v, %v), want (nil, nil)", body, err)
	}
}

// nativeCredentialLivenessProbeTestSelection builds a native SelectedProfile
// whose credential resolver is the caller's own, for exercising
// ProbeNativeCredentialLiveness (A3(a)) without needing an openable CLI or
// runtime files - the probe never touches them.
func nativeCredentialLivenessProbeTestSelection(
	resolve FileCredentialResolver,
) SelectedProfile {
	config := NativeAdapterConfig{
		Key: "a-claude-credential-liveness", ID: "sworn.claude", Version: "1.0.0",
		Family:             ProfileClaude,
		CLI:                ExecutableIdentity{Path: "/unused-by-this-probe"},
		CredentialTarget:   ClaudeCredentialTarget,
		CredentialRefs:     []string{"claude-file"},
		MaxCredentialBytes: 1_048_576,
	}
	identity := AdapterIdentity{
		Key: config.Key, ID: config.ID, Version: config.Version,
		ConfigurationDigest: "sha256:" + string(bytes.Repeat([]byte("d"), 64)),
	}
	ref := "claude-file"
	adapter := &nativeAdapter{
		identity: identity,
		config:   config,
		resolve:  resolve,
		refs:     map[string]struct{}{ref: {}},
	}
	return SelectedProfile{
		Profile: ProfileConfig{
			Key: "credential-liveness-probe-profile", Adapter: identity.Key,
			Network: NetworkRequired, CredentialRef: &ref,
		},
		Adapter: identity,
		Model:   "probe-model",
		adapter: adapter,
	}
}

func decodeNativeCredentialLivenessProbeEvent(
	t *testing.T,
	body []byte,
) NativeCredentialLivenessProbeEvent {
	t.Helper()
	var event NativeCredentialLivenessProbeEvent
	if err := json.Unmarshal(body, &event); err != nil {
		t.Fatalf("decode probe event: %v", err)
	}
	return event
}

// TestProbeNativeCredentialLivenessRefusesPositivelyStaleCredential pins
// A3(a)'s named-refusal half: a positively-expired credential refuses at
// admission with CREDENTIAL_STALE, journaled as a refusal, zero burn.
func TestProbeNativeCredentialLivenessRefusesPositivelyStaleCredential(t *testing.T) {
	credential := filepath.Join(t.TempDir(), "credential")
	expired := time.Now().UnixMilli() - 60_000
	body := []byte(
		`{"claudeAiOauth":{"accessToken":"a","expiresAt":` +
			strconv.FormatInt(expired, 10) + `}}`,
	)
	if err := os.WriteFile(credential, body, 0o600); err != nil {
		t.Fatal(err)
	}
	selected := nativeCredentialLivenessProbeTestSelection(
		func(context.Context, string) (string, error) { return credential, nil },
	)
	eventBody, err := ProbeNativeCredentialLiveness(
		context.Background(), selected, "run-stale-credential",
	)
	if !IsCode(err, "CREDENTIAL_STALE") {
		t.Fatalf("stale-credential probe error = %v, want CREDENTIAL_STALE", err)
	}
	event := decodeNativeCredentialLivenessProbeEvent(t, eventBody)
	if event.Outcome != nativeAdmissionProbeRefused ||
		event.RunID != "run-stale-credential" {
		t.Fatalf("probe event = %#v, want a refused outcome", event)
	}
}

// TestProbeNativeCredentialLivenessPassesFreshCredential pins the healthy
// path: a credential that positively reads as not expired admits.
func TestProbeNativeCredentialLivenessPassesFreshCredential(t *testing.T) {
	credential := filepath.Join(t.TempDir(), "credential")
	future := int64(8_000_000_000_000_000)
	body := []byte(
		`{"claudeAiOauth":{"accessToken":"a","expiresAt":` +
			strconv.FormatInt(future, 10) + `}}`,
	)
	if err := os.WriteFile(credential, body, 0o600); err != nil {
		t.Fatal(err)
	}
	selected := nativeCredentialLivenessProbeTestSelection(
		func(context.Context, string) (string, error) { return credential, nil },
	)
	eventBody, err := ProbeNativeCredentialLiveness(
		context.Background(), selected, "run-fresh-credential",
	)
	if err != nil {
		t.Fatalf("fresh-credential probe refused: %v", err)
	}
	event := decodeNativeCredentialLivenessProbeEvent(t, eventBody)
	if event.Outcome != nativeAdmissionProbePassed {
		t.Fatalf("probe event = %#v, want passed", event)
	}
}

// TestProbeNativeCredentialLivenessUnevaluableOnUnreadableCredential pins
// the honesty floor: a resolve/read failure never refuses - it admits, and
// the journal says so.
func TestProbeNativeCredentialLivenessUnevaluableOnUnreadableCredential(t *testing.T) {
	selected := nativeCredentialLivenessProbeTestSelection(
		func(context.Context, string) (string, error) {
			return filepath.Join(t.TempDir(), "does-not-exist"), nil
		},
	)
	eventBody, err := ProbeNativeCredentialLiveness(
		context.Background(), selected, "run-unreadable-credential",
	)
	if err != nil {
		t.Fatalf("unreadable-credential probe refused: %v", err)
	}
	event := decodeNativeCredentialLivenessProbeEvent(t, eventBody)
	if event.Outcome != nativeAdmissionProbeUnevaluable {
		t.Fatalf("probe event = %#v, want unevaluable", event)
	}
}

// TestProbeNativeCredentialLivenessIsANoOpForNonNativeAdapters pins the
// applicability gate shared with ProbeNativeAdmission.
func TestProbeNativeCredentialLivenessIsANoOpForNonNativeAdapters(t *testing.T) {
	adapter := processAdapterFixture(t, "a-fake", "sworn.fake-credential-probe")
	selected := SelectedProfile{
		Profile: ProfileConfig{Key: "fake-profile", Adapter: adapter.Identity().Key, Network: NetworkNone},
		Adapter: adapter.Identity(),
		Model:   "fake-model",
		adapter: adapter,
	}
	body, err := ProbeNativeCredentialLiveness(context.Background(), selected, "run-fake")
	if body != nil || err != nil {
		t.Fatalf("non-native credential-liveness probe = (%v, %v), want (nil, nil)", body, err)
	}
}

// TestProbeNativeCredentialLivenessIsANoOpForUnboundOrUnknownRef pins the
// same deferral-to-CREDENTIAL_NOT_CERTIFIED applicability gate for a native
// profile whose CredentialRef is absent or unrecognized.
func TestProbeNativeCredentialLivenessIsANoOpForUnboundOrUnknownRef(t *testing.T) {
	unbound := nativeCredentialLivenessProbeTestSelection(
		func(context.Context, string) (string, error) { return "", nil },
	)
	unbound.Profile.CredentialRef = nil
	if body, err := ProbeNativeCredentialLiveness(
		context.Background(), unbound, "run-unbound",
	); body != nil || err != nil {
		t.Fatalf("unbound-ref probe = (%v, %v), want (nil, nil)", body, err)
	}

	unknown := nativeCredentialLivenessProbeTestSelection(
		func(context.Context, string) (string, error) { return "", nil },
	)
	unknownRef := "not-a-registered-ref"
	unknown.Profile.CredentialRef = &unknownRef
	if body, err := ProbeNativeCredentialLiveness(
		context.Background(), unknown, "run-unknown-ref",
	); body != nil || err != nil {
		t.Fatalf("unknown-ref probe = (%v, %v), want (nil, nil)", body, err)
	}
}
