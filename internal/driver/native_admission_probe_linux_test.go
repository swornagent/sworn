//go:build linux

package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
