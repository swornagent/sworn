//go:build linux

package driver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestCredentialRotationEventAppendsOnlyWhenRotated pins the A3 loud
// recording: a rotated close appends one admitted credential_rotated event
// with a contiguous sequence, and an unrotated observation is untouched.
func TestCredentialRotationEventAppendsOnlyWhenRotated(t *testing.T) {
	base := Observation{
		TransportStatus: Completed,
		DurationMillis:  1,
		Diagnostic:      Diagnostic{Code: "none"},
		Events: []TerminalEvent{
			{Sequence: 1, Kind: "result_completed"},
			{Sequence: 2, Kind: "published"},
		},
	}
	if got := withCredentialRotationEvent(base, false); len(got.Events) != 2 {
		t.Fatalf("unrotated events = %#v", got.Events)
	}
	got := withCredentialRotationEvent(base, true)
	if len(got.Events) != 3 ||
		got.Events[2].Sequence != 3 ||
		got.Events[2].Kind != "credential_rotated" ||
		!validTerminalEventKind(got.Events[2].Kind) {
		t.Fatalf("rotated events = %#v", got.Events)
	}
	for index, event := range got.Events {
		if event.Sequence != uint64(index+1) {
			t.Fatalf("event %d sequence = %d", index, event.Sequence)
		}
	}
}

// withNativeAuthExitCodeFixture pins the family auth-exit vocabulary for one
// non-parallel test and restores it afterwards. It mirrors the
// enableUncontainedDispatch link-time-gate pattern: tests that use it own the
// vocabulary map for their own duration and never run in parallel.
func withNativeAuthExitCodeFixture(t *testing.T, family ProfileFamily, code int) {
	t.Helper()
	previous, present := nativeAuthExitCodes[family]
	nativeAuthExitCodes[family] = code
	t.Cleanup(func() {
		if present {
			nativeAuthExitCodes[family] = previous
		} else {
			delete(nativeAuthExitCodes, family)
		}
	})
}

// nativeCredentialGateAdapter builds an unvalidated nativeAdapter whose
// resolver returns the given credential path, for gate-level preflight tests
// that need no CLI, runtime files, or sandbox.
func nativeCredentialGateAdapter(
	t *testing.T,
	family ProfileFamily,
	credentialPath string,
) (*nativeAdapter, AdapterIdentity, string, SelectedProfile) {
	t.Helper()
	target := CodexCredentialTarget
	if family == ProfileClaude {
		target = ClaudeCredentialTarget
	}
	config := NativeAdapterConfig{
		Key:     "preflight-gate-" + string(family),
		ID:      "sworn.preflight-gate-" + string(family),
		Version: "1.0.0",
		Family:  family,
		CLI: ExecutableIdentity{
			Path:   "/nonexistent/native-preflight-gate-cli",
			Digest: "sha256:" + strings.Repeat("0", 64),
		},
		CLIVersion:             "1.0.0",
		VersionOutput:          "1.0.0",
		CredentialTarget:       target,
		CredentialRefs:         []string{"preflight-gate-credential"},
		MaxCredentialBytes:     65_536,
		RuntimeFiles:           []PinnedRuntimeFile{},
		RequiredRuntimeTargets: []string{},
	}
	body, err := canonicalJSON(config)
	if err != nil {
		t.Fatal(err)
	}
	identity := AdapterIdentity{
		Key:                 config.Key,
		ID:                  config.ID,
		Version:             config.Version,
		ConfigurationDigest: Digest(body),
	}
	ref := config.CredentialRefs[0]
	adapter := &nativeAdapter{
		identity: identity,
		config:   config,
		resolve: func(context.Context, string) (string, error) {
			return credentialPath, nil
		},
		refs: map[string]struct{}{ref: {}},
	}
	profile := ProfileConfig{
		Key:           "preflight-gate-profile-" + string(family),
		Adapter:       identity.Key,
		Network:       NetworkRequired,
		CredentialRef: &ref,
	}
	selected := SelectedProfile{
		Profile: profile,
		Adapter: identity,
		Model:   "preflight-gate-model",
		adapter: adapter,
	}
	return adapter, identity, ref, selected
}

// TestNativeCredentialPreflightGateRefusesOnlyExpired pins A1 at the two
// per-dispatch gates: a positively-expired credential is refused
// CREDENTIAL_STALE before any dispatch work, while fresh, expiry-less,
// unparseable, and other-family credentials pass unchanged.
func TestNativeCredentialPreflightGateRefusesOnlyExpired(t *testing.T) {
	expired := `{"claudeAiOauth":{"accessToken":"a","expiresAt":` +
		strconvI64(time.Now().UnixMilli()-60_000) + `}}`
	fresh := `{"claudeAiOauth":{"accessToken":"a","expiresAt":` +
		strconvI64(8_000_000_000_000_000) + `}}`
	expiryLess := `{"claudeAiOauth":{"accessToken":"a","refreshToken":"r"}}`
	malformed := `{"claudeAiOauth":{"expiresAt":`

	write := func(t *testing.T, body string) string {
		t.Helper()
		pathValue := filepath.Join(t.TempDir(), "credential")
		if err := os.WriteFile(pathValue, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return pathValue
	}

	gate := func(
		t *testing.T,
		family ProfileFamily,
		body string,
		wantCode string,
	) {
		t.Helper()
		pathValue := write(t, body)
		adapter, _, _, selected := nativeCredentialGateAdapter(
			t,
			family,
			pathValue,
		)
		invocation := Invocation{Selected: selected}
		_, gotPath, err := adapter.nativeRuntime(
			context.Background(),
			invocation,
		)
		if wantCode == "" {
			if err != nil {
				t.Fatalf("preflight error = %v", err)
			}
			if gotPath != pathValue {
				t.Fatalf("preflight path = %q, want %q", gotPath, pathValue)
			}
			return
		}
		if !IsCode(err, wantCode) {
			t.Fatalf("preflight error = %v, want %s", err, wantCode)
		}
		if gotPath != "" {
			t.Fatalf("refused preflight still returned path %q", gotPath)
		}
	}

	cases := []struct {
		family   ProfileFamily
		body     string
		wantCode string
	}{
		{ProfileClaude, expired, "CREDENTIAL_STALE"},
		{ProfileClaude, fresh, ""},
		{ProfileClaude, expiryLess, ""},
		{ProfileClaude, malformed, ""},
		{ProfileCodex, expired, ""},
	}
	for _, test := range cases {
		test := test
		t.Run(string(test.family)+"-"+test.wantCode, func(t *testing.T) {
			gate(t, test.family, test.body, test.wantCode)
		})
	}

	t.Run("automation gate", func(t *testing.T) {
		pathValue := write(t, expired)
		adapter, _, _, selected := nativeCredentialGateAdapter(
			t,
			ProfileClaude,
			pathValue,
		)
		_, gotPath, err := adapter.nativeAutomationRuntime(
			context.Background(),
			AutomationInvocation{Selected: selected},
		)
		if !IsCode(err, "CREDENTIAL_STALE") {
			t.Fatalf("automation preflight error = %v, want CREDENTIAL_STALE", err)
		}
		if gotPath != "" {
			t.Fatalf("refused automation preflight returned path %q", gotPath)
		}
	})

	t.Run("automation gate fresh passes", func(t *testing.T) {
		pathValue := write(t, fresh)
		adapter, _, _, selected := nativeCredentialGateAdapter(
			t,
			ProfileClaude,
			pathValue,
		)
		_, gotPath, err := adapter.nativeAutomationRuntime(
			context.Background(),
			AutomationInvocation{Selected: selected},
		)
		if err != nil || gotPath != pathValue {
			t.Fatalf("automation preflight = %q, %v", gotPath, err)
		}
	})
}

// TestNativeCredentialPreflightRefusesBeforeDispatchWork pins the C5
// delivered property: the refusal lands at dispatch preparation, before the
// CLI spawn, the sandbox, or the closure check. A fresh credential over the
// same adapter proceeds past the gate and fails later at the nonexistent
// closure - proving the gate is the first stop, while an expired credential
// never gets that far.
func TestNativeCredentialPreflightRefusesBeforeDispatchWork(t *testing.T) {
	expired := `{"claudeAiOauth":{"accessToken":"a","expiresAt":` +
		strconvI64(time.Now().UnixMilli()-60_000) + `}}`
	fresh := `{"claudeAiOauth":{"accessToken":"a","refreshToken":"r"}}`

	run := func(t *testing.T, body string) error {
		t.Helper()
		credential := filepath.Join(t.TempDir(), "credential")
		if err := os.WriteFile(credential, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		_, _, _, selected := nativeCredentialGateAdapter(
			t,
			ProfileClaude,
			credential,
		)
		base, _, _ := memoryInvocationFixture(t)
		base.Selected = selected
		pair := nativeSmokeInvocationsFixture(t, base)
		_, err := (Dispatcher{}).Invoke(
			context.Background(),
			pair.FreshReadWrite,
		)
		return err
	}

	t.Run("expired refused at gate", func(t *testing.T) {
		if err := run(t, expired); !IsCode(err, "CREDENTIAL_STALE") {
			t.Fatalf("expired dispatch error = %v, want CREDENTIAL_STALE", err)
		}
	})
	t.Run("fresh proceeds past gate", func(t *testing.T) {
		if err := run(t, fresh); !IsCode(err, "NATIVE_NOT_CERTIFIED") {
			t.Fatalf("fresh dispatch error = %v, want NATIVE_NOT_CERTIFIED", err)
		}
	})
}

// nativeCredentialFixtureAdapter builds the nativecontinuation test adapter
// over the given credential body, mirroring the established continuation
// fixture pattern. The config fixture skips the test when host runtime files
// are unavailable.
func nativeCredentialFixtureAdapter(
	t *testing.T,
	family ProfileFamily,
	binary string,
	digest string,
	credentialBody []byte,
) (*nativeAdapter, AdapterIdentity, string, SelectedProfile, string) {
	t.Helper()
	config := nativeContinuationConfigFixture(t, family, binary, digest)
	configBody, err := canonicalJSON(config)
	if err != nil {
		t.Fatal(err)
	}
	identity := AdapterIdentity{
		Key:                 config.Key,
		ID:                  config.ID,
		Version:             config.Version,
		ConfigurationDigest: Digest(configBody),
	}
	ref := config.CredentialRefs[0]
	credential := filepath.Join(t.TempDir(), "credential")
	if err := os.WriteFile(credential, credentialBody, 0o600); err != nil {
		t.Fatal(err)
	}
	adapter := &nativeAdapter{
		identity: identity,
		config:   config,
		resolve: func(context.Context, string) (string, error) {
			return credential, nil
		},
		refs: map[string]struct{}{ref: {}},
	}
	profile := ProfileConfig{
		Key:           "credential-fixture-profile-" + string(family),
		Adapter:       identity.Key,
		Network:       NetworkRequired,
		CredentialRef: &ref,
	}
	selected := SelectedProfile{
		Profile: profile,
		Adapter: identity,
		Model:   "native-continuation-model",
		adapter: adapter,
	}
	return adapter, identity, ref, selected, credential
}

func credentialFixtureInvoke(
	t *testing.T,
	family ProfileFamily,
	binary string,
	digest string,
	credentialBody []byte,
	timeoutMillis int64,
) error {
	t.Helper()
	_, _, _, selected, _ := nativeCredentialFixtureAdapter(
		t,
		family,
		binary,
		digest,
		credentialBody,
	)
	base, _, _ := memoryInvocationFixture(t)
	base.Selected = selected
	// The timeout must land before the permission is minted: the
	// permission digests the request, and a post-mint mutation fails
	// PERMISSION_BINDING_MISMATCH before any process launches.
	base.Request.Limits.TimeoutMillis = timeoutMillis
	pair := nativeSmokeInvocationsFixture(t, base)
	invocation := pair.FreshReadWrite
	_, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	return err
}

// TestNativeSpontaneousExitClassification pins A2 at the terminal
// classification site through the nativecontinuation fixture. The fixture
// emits the identity event and completes the full broker handshake before
// every exit (C2), so every case reaches the named site.
func TestNativeSpontaneousExitClassification(t *testing.T) {
	probe := buildNativeContinuation(t)
	digest, err := executableDigest(probe)
	if err != nil {
		t.Fatal(err)
	}
	unauthorized := []byte(`{"offline_provider":"unauthorized"}`)
	exitone := []byte(`{"offline_provider":"exitone"}`)
	expire := []byte(`{"offline_provider":"expire"}`)
	crash := []byte(`{"offline_provider":"crash"}`)

	t.Run("auth exit without vocabulary stays transport", func(t *testing.T) {
		for _, family := range []ProfileFamily{ProfileCodex, ProfileClaude} {
			family := family
			t.Run(string(family), func(t *testing.T) {
				err := credentialFixtureInvoke(
					t,
					family,
					probe,
					digest,
					unauthorized,
					2_000,
				)
				if !IsCode(err, "PROVIDER_TRANSPORT_FAILED") {
					t.Fatalf("auth-exit error = %v, want PROVIDER_TRANSPORT_FAILED", err)
				}
				var contractErr *ContractError
				if !errors.As(err, &contractErr) || contractErr.Kind != KindTransport {
					t.Fatalf("auth-exit Kind = %#v, want KindTransport", err)
				}
			})
		}
	})

	// A4: a self-signalled crash of the wrapped CLI - never the engine
	// stopping it - surfaces as NATIVE_SURFACE_INVALID naming
	// dispatch.process_signaled, classifying to KindSurfaceIntegrity.
	t.Run("signalled child surfaces as surface invalid", func(t *testing.T) {
		for _, family := range []ProfileFamily{ProfileCodex, ProfileClaude} {
			family := family
			t.Run(string(family), func(t *testing.T) {
				err := credentialFixtureInvoke(
					t,
					family,
					probe,
					digest,
					crash,
					2_000,
				)
				detail := decodeNativeSurfaceDetail(t, err)
				if detail.Check != "dispatch.process_signaled" {
					t.Fatalf("crash detail = %#v, want dispatch.process_signaled", detail)
				}
				var contractErr *ContractError
				if !errors.As(err, &contractErr) ||
					contractErr.Kind != KindSurfaceIntegrity {
					t.Fatalf("crash Kind = %#v, want KindSurfaceIntegrity", err)
				}
			})
		}
	})

	t.Run("pinned auth exit code surfaces authorization failure", func(t *testing.T) {
		withNativeAuthExitCodeFixture(t, ProfileCodex, 2)
		err := credentialFixtureInvoke(
			t,
			ProfileCodex,
			probe,
			digest,
			unauthorized,
			2_000,
		)
		if !IsCode(err, "PROVIDER_AUTHORIZATION_FAILED") {
			t.Fatalf("auth-exit error = %v, want PROVIDER_AUTHORIZATION_FAILED", err)
		}
	})

	t.Run("exit code one is barred from the vocabulary", func(t *testing.T) {
		withNativeAuthExitCodeFixture(t, ProfileCodex, 1)
		err := credentialFixtureInvoke(
			t,
			ProfileCodex,
			probe,
			digest,
			exitone,
			2_000,
		)
		if !IsCode(err, "PROVIDER_TRANSPORT_FAILED") {
			t.Fatalf("exit-one error = %v, want PROVIDER_TRANSPORT_FAILED", err)
		}
	})

	t.Run("expired credential outranks transport", func(t *testing.T) {
		err := credentialFixtureInvoke(
			t,
			ProfileClaude,
			probe,
			digest,
			expire,
			2_000,
		)
		if !IsCode(err, "CREDENTIAL_STALE") {
			t.Fatalf("expired-at-close error = %v, want CREDENTIAL_STALE", err)
		}
	})

	t.Run("expiry vocabulary is claude-only", func(t *testing.T) {
		err := credentialFixtureInvoke(
			t,
			ProfileCodex,
			probe,
			digest,
			expire,
			2_000,
		)
		if !IsCode(err, "PROVIDER_TRANSPORT_FAILED") {
			t.Fatalf("codex expired-at-close error = %v, want PROVIDER_TRANSPORT_FAILED", err)
		}
	})
}

// TestNativeSpontaneousExitFailureClassifiesSignalledExitCodes pins A4's
// discriminator directly against real exec.ExitError values, independent of
// the sandboxed nativecontinuation fixture (which needs host runtime files
// this environment may lack): 138 (128+SIGUSR1, this file's own crash-mode
// idiom, exercised end to end by "signalled child surfaces as surface
// invalid" above wherever the sandbox is available) and other
// signal-translated codes classify as a signalled surface, while the two
// values an engine-initiated group kill can itself produce (128+SIGTERM,
// 128+SIGKILL) stay excluded, preserving the pre-existing engine-stop
// reading unchanged. /usr/bin/sh's own "exit N" builtin produces the exact
// same exec.ExitError.ExitCode() shape bwrap's signal translation does, so
// this needs no sandbox, no bwrap, and no fixture binary.
func TestNativeSpontaneousExitFailureClassifiesSignalledExitCodes(t *testing.T) {
	exitWith := func(t *testing.T, code int) error {
		t.Helper()
		cmd := exec.Command("/usr/bin/sh", "-c", fmt.Sprintf("exit %d", code))
		err := cmd.Run()
		if err == nil {
			t.Fatalf("exit %d unexpectedly succeeded", code)
		}
		return err
	}

	cases := []struct {
		name          string
		code          int
		wantSignalled bool
	}{
		{"128+SIGUSR1 (this file's crash idiom) classifies as signalled", 138, true},
		{"128+SIGSEGV classifies as signalled", 139, true},
		{"128+SIGABRT classifies as signalled", 134, true},
		{"128+SIGTERM stays the engine-stop exclusion", 128 + int(syscall.SIGTERM), false},
		{"128+SIGKILL stays the engine-stop exclusion", 128 + int(syscall.SIGKILL), false},
		{"ordinary nonzero exit stays transport", 2, false},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			waitErr := exitWith(t, test.code)
			err := nativeSpontaneousExitFailure(false, ProfileClaude, waitErr, nil)
			if test.wantSignalled {
				detail := decodeNativeSurfaceDetail(t, err)
				if detail.Check != "dispatch.process_signaled" {
					t.Fatalf(
						"code %d detail = %#v, want dispatch.process_signaled",
						test.code, detail,
					)
				}
				var contractErr *ContractError
				if !errors.As(err, &contractErr) {
					t.Fatal("expected a *ContractError")
				}
				if classifyKind(contractErr.Code, contractErr.HardLimit) != KindSurfaceIntegrity {
					t.Fatalf("code %d Kind = %v, want KindSurfaceIntegrity", test.code,
						classifyKind(contractErr.Code, contractErr.HardLimit))
				}
				return
			}
			if !IsCode(err, "PROVIDER_TRANSPORT_FAILED") {
				t.Fatalf("code %d error = %v, want PROVIDER_TRANSPORT_FAILED", test.code, err)
			}
		})
	}
}

// TestNativeBenignRotationKeepsCompletedDispatch pins A3 end to end: the
// fixture signals through the bound credential, the host renames a valid
// same-shape replacement over the path mid-dispatch, and the completed
// dispatch keeps its handoff while recording the rotation loudly on the
// durable observation surface.
func TestNativeBenignRotationKeepsCompletedDispatch(t *testing.T) {
	probe := buildNativeContinuation(t)
	digest, err := executableDigest(probe)
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range []ProfileFamily{ProfileCodex, ProfileClaude} {
		family := family
		t.Run(string(family), func(t *testing.T) {
			marker := []byte(`{"rotation_marker":"running"}`)
			_, _, _, selected, credential :=
				nativeCredentialFixtureAdapter(
					t,
					family,
					probe,
					digest,
					[]byte(`{"offline_provider":"rotation"}`),
				)
			base, _, _ := memoryInvocationFixture(t)
			base.Selected = selected
			base.Request.Limits.TimeoutMillis = 30_000
			pair := nativeSmokeInvocationsFixture(t, base)
			invocation := pair.FreshReadWrite

			done := make(chan Observation, 1)
			errs := make(chan error, 1)
			go func() {
				observation, err := (Dispatcher{}).Invoke(
					context.Background(),
					invocation,
				)
				errs <- err
				done <- observation
			}()
			// Wait for the fixture's own signal through the bound credential,
			// then rotate exactly like ordinary host refresh: a new 0600 file
			// at a temporary name, atomically renamed over the path.
			deadline := time.Now().Add(15 * time.Second)
			for {
				body, readErr := os.ReadFile(credential)
				if readErr == nil && bytes.Contains(body, marker) {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("fixture never signalled through the credential")
				}
				time.Sleep(5 * time.Millisecond)
			}
			replacement := filepath.Join(
				filepath.Dir(credential),
				"replacement",
			)
			if err := os.WriteFile(
				replacement,
				[]byte(`{"token":"rotated"}`),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(replacement, credential); err != nil {
				t.Fatal(err)
			}
			if err := <-errs; err != nil {
				t.Fatalf("rotated dispatch error = %v", err)
			}
			observation := <-done
			if observation.Handoff == nil {
				t.Fatal("completed dispatch lost its handoff")
			}
			if len(observation.Events) == 0 ||
				observation.Events[len(observation.Events)-1].Kind !=
					"credential_rotated" {
				t.Fatalf(
					"completed dispatch did not record the rotation: %#v",
					observation.Events,
				)
			}
			last := observation.Events[len(observation.Events)-1]
			if last.Sequence != uint64(len(observation.Events)) {
				t.Fatalf(
					"credential_rotated sequence = %d, want %d",
					last.Sequence,
					len(observation.Events),
				)
			}
		})
	}
}

func strconvI64(value int64) string {
	return strconv.FormatInt(value, 10)
}

// TestNativeCertifyReportsWhatThePreflightActuallyDid pins all three
// certify outcomes for a native adapter. The regression this guards
// against shipped because nothing asserted that a native can pass at
// all: reporting "unevaluated" for a credential the check did read and
// did not find expired is the same false claim the honest preflight
// replaced, and it left sworn driver certify unable to certify any
// claude or codex lane (sworn#248).
func TestNativeCertifyReportsWhatThePreflightActuallyDid(t *testing.T) {
	fresh := `{"claudeAiOauth":{"accessToken":"a","expiresAt":` +
		strconvI64(8_000_000_000_000_000) + `}}`
	expired := `{"claudeAiOauth":{"accessToken":"a","expiresAt":` +
		strconvI64(time.Now().UnixMilli()-60_000) + `}}`

	write := func(t *testing.T, body string) string {
		t.Helper()
		pathValue := filepath.Join(t.TempDir(), "credential")
		if err := os.WriteFile(pathValue, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return pathValue
	}
	probe := buildNativeContinuation(t)
	digest, err := executableDigest(probe)
	if err != nil {
		t.Fatal(err)
	}
	// The in-repo probe, not a vendor binary: exactNativeConfigFixture
	// keys off a CLI that exists on one operator host, so a certify test
	// built on it would skip everywhere else - including CI, which is
	// exactly how an unguarded path ships (sworn#249).
	certify := func(t *testing.T, credentialPath string) (ReadinessState, string) {
		t.Helper()
		adapter, identity, ref, _, _ := nativeCredentialFixtureAdapter(
			t, ProfileClaude, probe, digest, []byte(`{}`),
		)
		adapter.resolve = func(context.Context, string) (string, error) {
			return credentialPath, nil
		}
		profile := ProfileConfig{
			Key: "certify-profile", Adapter: identity.Key,
			Network: NetworkRequired, CredentialRef: &ref,
		}
		return adapter.checkProfile(
			context.Background(),
			checkCertify,
			profile,
			"native-continuation-model",
		)
	}

	t.Run("evaluated and live passes on that evaluation", func(t *testing.T) {
		state, code := certify(t, write(t, fresh))
		if state != ReadinessPass ||
			code != "native_credential_preflight_passed" {
			t.Fatalf("certify = %s %s, want PASS native_credential_preflight_passed",
				state, code)
		}
	})
	t.Run("positively stale fails", func(t *testing.T) {
		state, code := certify(t, write(t, expired))
		if state != ReadinessFail || code != "CREDENTIAL_STALE" {
			t.Fatalf("certify = %s %s, want FAIL CREDENTIAL_STALE", state, code)
		}
	})
	t.Run("unreadable reference stays unevaluated", func(t *testing.T) {
		state, code := certify(t, "/not-used-by-readiness")
		if state != ReadinessNotCertified ||
			code != "native_credential_preflight_unevaluated" {
			t.Fatalf("certify = %s %s, want NOT_CERTIFIED unevaluated",
				state, code)
		}
	})
}
