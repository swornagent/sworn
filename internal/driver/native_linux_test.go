//go:build linux

package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/gitx"
)

const (
	exactCodexBinary  = "/home/brad/.nvm/versions/node/v24.14.0/lib/node_modules/@openai/codex/node_modules/@openai/codex-linux-x64/vendor/x86_64-unknown-linux-musl/bin/codex"
	exactClaudeBinary = "/home/brad/snap/code/253/.local/share/claude/versions/2.1.241"
)

var (
	nativeProbeOnce          sync.Once
	nativeProbeBinary        string
	nativeProbeError         error
	nativeContinuationOnce   sync.Once
	nativeContinuationBinary string
	nativeContinuationError  error
)

// nativeMemoryRoot returns a tmpfs directory for tests that require
// memory-backed crash recovery. It probes the effective process temp
// directory first (honoring TMPDIR) and then the conventional /dev/shm
// test-environment surface; it is test-environment discovery, not a product
// host-location literal. It must be called before t.Parallel.
func nativeMemoryRoot(t *testing.T) string {
	t.Helper()
	for _, candidate := range []string{os.TempDir(), "/dev/shm"} {
		if nativeMemoryBackedPath(candidate) {
			return candidate
		}
	}
	t.Skip("no memory-backed (tmpfs) directory available for native-session tests")
	return ""
}

// setNativeMemoryRootEnv points the configured temp root at a tmpfs so the
// lazily resolved nativeSessionMemoryRoot is memory-backed for this test.
func setNativeMemoryRootEnv(t *testing.T) {
	t.Helper()
	t.Setenv(gitx.EnvTempRoot, nativeMemoryRoot(t))
}

// TestNativeSessionMemoryRootResolvesMemoryBacked proves A2 for the native
// session memory root: it resolves from the SWORN_NATIVE_SESSION_ROOT
// override, follows the configured temp root when that root is itself
// memory-backed, and otherwise discovers a memory-backed (tmpfs) directory —
// never a hardcoded literal and never the general disk-backed temp root.
func TestNativeSessionMemoryRootResolvesMemoryBacked(t *testing.T) {
	memoryRoot := nativeMemoryRoot(t)

	// SWORN_NATIVE_SESSION_ROOT override wins verbatim.
	override := filepath.Join(memoryRoot, "sworn-native-override")
	if err := os.MkdirAll(override, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(override) })
	t.Setenv(gitx.EnvNativeSessionRoot, override)
	t.Setenv(gitx.EnvTempRoot, "")
	got, err := nativeSessionMemoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != override {
		t.Fatalf("override native session memory root = %q, want %q", got, override)
	}

	// A memory-backed configured temp root is followed.
	t.Setenv(gitx.EnvNativeSessionRoot, "")
	t.Setenv(gitx.EnvTempRoot, memoryRoot)
	got, err = nativeSessionMemoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != memoryRoot {
		t.Fatalf("temp-root native session memory root = %q, want %q", got, memoryRoot)
	}

	// A disk-backed configured temp root is never used for native sessions:
	// the resolver either discovers a memory-backed directory or fails closed
	// (never degrading to the disk-backed root).
	disk := filepath.Join(t.TempDir(), "sworn-tmp")
	t.Setenv(gitx.EnvNativeSessionRoot, "")
	t.Setenv(gitx.EnvTempRoot, disk)
	if got, err = nativeSessionMemoryRoot(); err != nil {
		if !IsCode(err, "CONTINUATION_INVALID") {
			t.Fatalf("disk-backed temp root error = %v", err)
		}
	} else {
		if !nativeMemoryBackedPath(got) {
			t.Fatalf("native session memory root %q is not memory-backed", got)
		}
		if got == disk {
			t.Fatalf("disk-backed temp root %q became the native session root", disk)
		}
	}
}

// TestNativeSessionMemoryRootCreatesConfiguredFreshRoots proves A2's fresh
// configured tmpfs child support: a valid configured override (or an XDG
// state home on tmpfs) whose path does not yet exist is securely created and
// admitted as the native session memory root instead of being silently
// ignored.
func TestNativeSessionMemoryRootCreatesConfiguredFreshRoots(t *testing.T) {
	memoryRoot := nativeMemoryRoot(t)

	// A fresh SWORN_NATIVE_SESSION_ROOT child under a tmpfs is created,
	// owned by the current user, mode 0700, and memory-backed.
	t.Setenv(gitx.EnvNativeSessionRoot, "")
	t.Setenv(gitx.EnvTempRoot, "")
	nativeFresh := filepath.Join(memoryRoot, "sworn-native-fresh-"+randomSuffix())
	info, err := os.Lstat(nativeFresh)
	if !os.IsNotExist(err) {
		t.Fatalf("fresh native root unexpectedly exists: %v", info)
	}
	t.Setenv(gitx.EnvNativeSessionRoot, nativeFresh)
	t.Cleanup(func() { _ = os.RemoveAll(nativeFresh) })
	got, err := nativeSessionMemoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != nativeFresh {
		t.Fatalf("fresh native session memory root = %q, want %q", got, nativeFresh)
	}
	stat, err := os.Lstat(nativeFresh)
	if err != nil || !stat.IsDir() || stat.Mode().Perm() != 0o700 ||
		!nativeMemoryBackedPath(nativeFresh) {
		t.Fatalf("fresh native root not admitted as private tmpfs: %v, %v", stat, err)
	}
	// The fresh configured root is immediately usable by the session reaper,
	// which previously rejected a fresh (absent) configured root.
	if err := reapNativeSessionRoots(0); err != nil {
		t.Fatalf("reap on fresh configured native root: %v", err)
	}

	// A fresh SWORN_TEMP_ROOT child under a tmpfs is created and, being
	// memory-backed, becomes the native session memory root.
	t.Setenv(gitx.EnvNativeSessionRoot, "")
	tempFresh := filepath.Join(memoryRoot, "sworn-tmp-fresh-"+randomSuffix())
	if _, err := os.Lstat(tempFresh); !os.IsNotExist(err) {
		t.Fatalf("fresh temp root unexpectedly exists: %v", err)
	}
	t.Setenv(gitx.EnvTempRoot, tempFresh)
	t.Cleanup(func() { _ = os.RemoveAll(tempFresh) })
	got, err = nativeSessionMemoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != tempFresh {
		t.Fatalf("fresh temp-root native session memory root = %q, want %q", got, tempFresh)
	}
	if !nativeMemoryBackedPath(tempFresh) {
		t.Fatalf("fresh temp root %q is not memory-backed", tempFresh)
	}

	// A fresh XDG state home on tmpfs makes the unconfigured default temp
	// root itself memory-backed: it is created and used, so an unconfigured
	// host with tmpfs state still gets a memory-backed default.
	t.Setenv(gitx.EnvNativeSessionRoot, "")
	t.Setenv(gitx.EnvTempRoot, "")
	stateFresh := filepath.Join(memoryRoot, "sworn-state-fresh-"+randomSuffix())
	t.Setenv("XDG_STATE_HOME", stateFresh)
	t.Cleanup(func() { _ = os.RemoveAll(stateFresh) })
	got, err = nativeSessionMemoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(stateFresh, "sworn", "tmp")
	if got != expected {
		t.Fatalf("xdg default native session memory root = %q, want %q", got, expected)
	}
	if !nativeMemoryBackedPath(expected) {
		t.Fatalf("xdg default temp root %q is not memory-backed", expected)
	}
}

// TestNativeSessionMemoryRootRefusesConfiguredFailures proves A2's
// fail-closed configured-value handling: a malformed or unavailable
// configured temp root is refused rather than silently replaced by discovery,
// and a configured native session root that is not a private tmpfs is refused
// rather than honoured.
func TestNativeSessionMemoryRootRefusesConfiguredFailures(t *testing.T) {
	t.Setenv(gitx.EnvNativeSessionRoot, "")
	t.Setenv(gitx.EnvTempRoot, "relative-root")
	if _, err := nativeSessionMemoryRoot(); err == nil {
		t.Fatal("relative SWORN_TEMP_ROOT admitted")
	}
	t.Setenv(gitx.EnvTempRoot, "/workspace")
	if _, err := nativeSessionMemoryRoot(); err == nil {
		t.Fatal("guest-path SWORN_TEMP_ROOT admitted")
	}

	// A configured native session root on ordinary disk is refused: crash
	// recovery never trusts a non-tmpfs root.
	t.Setenv(gitx.EnvNativeSessionRoot, "")
	t.Setenv(gitx.EnvTempRoot, "")
	t.Setenv(gitx.EnvNativeSessionRoot, filepath.Join(t.TempDir(), "sworn-native-on-disk"))
	if _, err := nativeSessionMemoryRoot(); err == nil {
		t.Fatal("disk-backed SWORN_NATIVE_SESSION_ROOT admitted")
	}

	// An unavailable configured native session root (whose parent is a plain
	// file) is refused rather than replaced by discovery.
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(gitx.EnvNativeSessionRoot, filepath.Join(parent, "child"))
	if _, err := nativeSessionMemoryRoot(); err == nil {
		t.Fatal("unavailable SWORN_NATIVE_SESSION_ROOT admitted")
	}
}

// TestNativeSessionMemoryRootRefusesLooseNativeOverride proves a configured
// native session root that is writable by group or world is refused: session
// state must live in a private tmpfs directory.
func TestNativeSessionMemoryRootRefusesLooseNativeOverride(t *testing.T) {
	memoryRoot := nativeMemoryRoot(t)
	loose := filepath.Join(memoryRoot, "sworn-native-loose-"+randomSuffix())
	if err := os.MkdirAll(loose, 0o777); err != nil {
		t.Fatal(err)
	}
	// MkdirAll's mode is umask-filtered, so make the looseness explicit or
	// the test's meaning varies by machine.
	if err := os.Chmod(loose, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(loose) })
	t.Setenv(gitx.EnvNativeSessionRoot, loose)
	t.Setenv(gitx.EnvTempRoot, "")
	if _, err := nativeSessionMemoryRoot(); err == nil {
		t.Fatal("world-writable SWORN_NATIVE_SESSION_ROOT admitted")
	}
}

// TestNativeSessionMemoryRootNoOverrideDiscovery proves the discovery step
// only runs when no relevant override was supplied: with no overrides it
// returns a discovered writable tmpfs when one is discoverable, and fails
// closed with CONTINUATION_INVALID (never a disk path) when the host exposes
// none.
func TestNativeSessionMemoryRootNoOverrideDiscovery(t *testing.T) {
	t.Setenv(gitx.EnvNativeSessionRoot, "")
	t.Setenv(gitx.EnvTempRoot, "")
	got, err := nativeSessionMemoryRoot()
	if err != nil {
		if !IsCode(err, "CONTINUATION_INVALID") {
			t.Fatalf("no-override native session memory root error = %v", err)
		}
		// No writable tmpfs is discoverable on this host: resolution fails
		// closed rather than degrading to ordinary disk.
		return
	}
	if !nativeMemoryBackedPath(got) || !writableDirectory(got) {
		t.Fatalf("discovered native session memory root %q is not a writable tmpfs", got)
	}
}

// TestNativeSessionMemoryRootDefaultCreatesAndValidatesSession is the
// end-to-end sensible-default proof: an unconfigured host whose XDG state
// home is memory-backed gets a fresh created default temp root, and that
// root can create a native continuation session whose root passes the same
// memory-backing and ownership validation the engine trusts during crash
// recovery.
func TestNativeSessionMemoryRootDefaultCreatesAndValidatesSession(t *testing.T) {
	memoryRoot := nativeMemoryRoot(t) // skip when no tmpfs exists for the default to use
	t.Setenv(gitx.EnvNativeSessionRoot, "")
	t.Setenv(gitx.EnvTempRoot, "")
	stateFresh := filepath.Join(memoryRoot, "sworn-state-session-"+randomSuffix())
	t.Setenv("XDG_STATE_HOME", stateFresh)
	t.Cleanup(func() { _ = os.RemoveAll(stateFresh) })
	config := NativeAdapterConfig{
		Family:           ProfileCodex,
		CredentialTarget: CodexCredentialTarget,
	}
	state, err := newNativeContinuationState(config, 0)
	if err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	root := state.root
	state.mu.Unlock()
	if !validNativeSessionRoot(root) || !nativeMemoryBackedPath(root) {
		t.Fatalf("default-created native session root %q invalid", root)
	}
	if err := state.closeContinuation(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("default-created native session root survived cleanup")
	}
}

// TestNativeSessionMemoryRootRefusesInvalidOverride proves a bad
// SWORN_NATIVE_SESSION_ROOT override is refused rather than silently
// replaced by a fallback.
func TestNativeSessionMemoryRootRefusesInvalidOverride(t *testing.T) {
	t.Setenv(gitx.EnvNativeSessionRoot, "relative-root")
	if _, err := nativeSessionMemoryRoot(); err == nil {
		t.Fatal("relative SWORN_NATIVE_SESSION_ROOT admitted")
	}
	t.Setenv(gitx.EnvNativeSessionRoot, "/workspace")
	if _, err := nativeSessionMemoryRoot(); err == nil {
		t.Fatal("guest-path SWORN_NATIVE_SESSION_ROOT admitted")
	}
}

func randomSuffix() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

// TestExactNativeProfilesCertifyReportsCredentialPreflightHonestly pins A2:
// certify no longer claims native_preflight_not_required for a credential it
// never evaluated. The fixture resolver points at a nonexistent path (as it
// always has - the resolver return value was never used by readiness before
// this slice), so the bounded read cannot open it and certify reports
// unevaluated, honestly, rather than a silent pass.
func TestExactNativeProfilesCertifyReportsCredentialPreflightHonestly(t *testing.T) {
	for _, family := range []ProfileFamily{ProfileCodex, ProfileClaude} {
		family := family
		t.Run(string(family), func(t *testing.T) {
			config := exactNativeConfigFixture(t, family)
			ref := string(family) + "-credential"
			adapterValue, err := NewNativeAdapter(
				config,
				func(context.Context, string) (string, error) {
					return "/not-used-by-readiness", nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			adapter := adapterValue.(*nativeAdapter)
			profile := ProfileConfig{
				Key: string(family) + "-profile", Adapter: config.Key,
				Network: NetworkRequired, CredentialRef: &ref,
			}
			if state, code := adapter.checkProfile(
				context.Background(),
				checkInspect,
				profile,
				"exact-native-model",
			); state != ReadinessPass || code != "native_closure_exact" {
				t.Fatalf("inspect = %s %s", state, code)
			}
			if state, code := adapter.checkProfile(
				context.Background(),
				checkDoctor,
				profile,
				"exact-native-model",
			); state != ReadinessPass || code != "native_binary_ready" {
				t.Fatalf("doctor = %s %s", state, code)
			}
			if state, code := adapter.checkProfile(
				context.Background(),
				checkCertify,
				profile,
				"exact-native-model",
			); state != ReadinessNotCertified ||
				code != "native_credential_preflight_unevaluated" {
				t.Fatalf("certify = %s %s", state, code)
			}
			registry, err := NewSelectionRegistry(
				[]ProfileConfig{profile},
				[]Adapter{adapter},
			)
			if err != nil {
				t.Fatal(err)
			}
			report := registry.Inspect(
				context.Background(),
				profile.Key,
				"exact-native-model",
			)
			body, _ := json.Marshal(report)
			for _, forbidden := range []string{
				config.CLI.Path, ref, config.CredentialTarget,
			} {
				if bytes.Contains(body, []byte(forbidden)) {
					t.Fatalf("readiness report leaked %q: %s", forbidden, body)
				}
			}
		})
	}
}

// certifyLivenessAdapterFixture builds a nativeAdapter with a scripted, live
// CLI (so checkProfile's version gate passes without a real host CLI) and a
// resolver bound to credentialPath, for exercising checkCertify's
// credential-liveness reporting (A2) directly.
func certifyLivenessAdapterFixture(
	t *testing.T,
	credentialPath string,
) (*nativeAdapter, ProfileConfig) {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(
		executable,
		[]byte("#!/usr/bin/sh\necho certify-fixture-version\nexit 0\n"),
		0o700,
	); err != nil {
		t.Fatal(err)
	}
	digest, err := executableDigest(executable)
	if err != nil {
		t.Fatal(err)
	}
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
		Key: "claude-certify-fixture", ID: "sworn.claude", Version: "1.0.0",
		Family:                 ProfileClaude,
		CLI:                    ExecutableIdentity{Path: executable, Digest: digest},
		CLIVersion:             "2.1.999",
		VersionOutput:          "certify-fixture-version",
		RuntimeFiles:           files,
		RequiredRuntimeTargets: required,
		CredentialTarget:       ClaudeCredentialTarget,
		CredentialRefs:         []string{"claude-certify-credential"},
		MaxCredentialBytes:     1_048_576,
	}
	body, err := canonicalJSON(config)
	if err != nil {
		t.Fatal(err)
	}
	identity := AdapterIdentity{
		Key: config.Key, ID: config.ID, Version: config.Version,
		ConfigurationDigest: Digest(body),
	}
	adapter := &nativeAdapter{
		identity: identity,
		config:   config,
		resolve: func(context.Context, string) (string, error) {
			return credentialPath, nil
		},
		refs: map[string]struct{}{"claude-certify-credential": {}},
	}
	ref := "claude-certify-credential"
	profile := ProfileConfig{
		Key: "claude-certify-profile", Adapter: identity.Key,
		Network: NetworkRequired, CredentialRef: &ref,
	}
	return adapter, profile
}

// TestNativeCertifyRefusesPositivelyStaleCredential pins A2's other half:
// certify fails a positively-expired credential with CREDENTIAL_STALE,
// rather than the pre-slice silent pass.
func TestNativeCertifyRefusesPositivelyStaleCredential(t *testing.T) {
	credential := filepath.Join(t.TempDir(), "credential")
	expired := time.Now().UnixMilli() - 60_000
	body := []byte(
		`{"claudeAiOauth":{"accessToken":"a","expiresAt":` +
			strconv.FormatInt(expired, 10) + `}}`,
	)
	if err := os.WriteFile(credential, body, 0o600); err != nil {
		t.Fatal(err)
	}
	adapter, profile := certifyLivenessAdapterFixture(t, credential)
	state, code := adapter.checkProfile(
		context.Background(), checkCertify, profile, "certify-model",
	)
	if state != ReadinessFail || code != "CREDENTIAL_STALE" {
		t.Fatalf("certify = %s %s, want FAIL CREDENTIAL_STALE", state, code)
	}
}

func TestNativeCertificationFailureCasesRemainFailClosed(t *testing.T) {
	probe := buildNativeContinuation(t)
	digest, err := executableDigest(probe)
	if err != nil {
		t.Fatal(err)
	}
	config := nativeContinuationConfigFixture(
		t,
		ProfileCodex,
		probe,
		digest,
	)
	invocation, _, _ := memoryInvocationFixture(t)
	invocation.Selected.Model = "native-continuation-model"
	invocation.Request.Limits.TimeoutMillis = 2_000
	certificate := nativeContinuationCertificateFixture(invocation, config)

	t.Run("missing-credential", func(t *testing.T) {
		_, err := platformInvokeNative(
			context.Background(),
			invocation,
			config,
			filepath.Join(t.TempDir(), "missing"),
			certificate,
		)
		if !IsCode(err, "CREDENTIAL_NOT_CERTIFIED") ||
			certificationFailureCode(err) !=
				"certification_credential_failed" {
			t.Fatalf("missing credential error = %v", err)
		}
	})

	t.Run("unreachable-provider", func(t *testing.T) {
		credential := filepath.Join(t.TempDir(), "credential")
		if err := os.WriteFile(
			credential,
			[]byte(`{"offline_provider":"unreachable"}`),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		_, err := platformInvokeNative(
			context.Background(),
			invocation,
			config,
			credential,
			certificate,
		)
		if !IsCode(err, "PROVIDER_TRANSPORT_FAILED") ||
			certificationFailureCode(err) !=
				"certification_provider_transport_failed" {
			t.Fatalf("unreachable provider error = %v", err)
		}
	})
}

func TestNativeCommandSurfacesAreExactAndCapabilityIsSingleSeam(t *testing.T) {
	for _, family := range []ProfileFamily{ProfileCodex, ProfileClaude} {
		family := family
		t.Run(string(family), func(t *testing.T) {
			config := exactNativeConfigFixture(t, family)
			invocation, _, _ := memoryInvocationFixture(t)
			invocation.Selected.Model = "exact-native-model"
			invocation.Request.FreshContext = true
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
			capability := broker.capability()
			defer clearBytes(capability)
			credentialPath := filepath.Join(t.TempDir(), "credential")
			const credentialCanary = "native-credential-canary"
			if err := os.WriteFile(
				credentialPath,
				[]byte(credentialCanary),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
			credential, err := acquireFileCredential(
				credentialPath,
				invocation.HostWorkspace,
				config.MaxCredentialBytes,
			)
			if err != nil {
				t.Fatal(err)
			}
			defer credential.Close()
			closure, err := openNativeClosure(config)
			if err != nil {
				t.Fatal(err)
			}
			defer closeNativeFiles(closure)
			configFiles, err := nativeConfigFiles(
				config,
				invocation,
				broker.URL(),
				capability,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			defer closeNativeFiles(configFiles)
			arguments, environment, extraFiles, err := nativeCommand(
				config,
				invocation,
				credential,
				closure,
				configFiles,
				nil,
				nil,
				toolDefinitions(
					invocation.Request.Workspace.Access,
				),
			)
			if err != nil {
				t.Fatal(err)
			}
			defer clearEnvironment(environment)
			argumentBody := []byte(strings.Join(arguments, "\x00"))
			environmentBody := bytes.Join(environment, []byte{0})
			if bytes.Contains(argumentBody, capability) ||
				bytes.Contains(argumentBody, []byte(credentialCanary)) ||
				bytes.Contains(argumentBody, []byte(config.CLI.Path)) ||
				bytes.Contains(argumentBody, []byte(invocation.HostWorkspace)) ||
				!slicesContain(arguments, "--die-with-parent") ||
				!slicesContain(arguments, "--share-net") ||
				len(extraFiles) != len(config.RuntimeFiles)+4 {
				t.Fatalf(
					"native surface = args %q env %q extra=%d",
					arguments,
					environment,
					len(extraFiles),
				)
			}
			for index := 0; index < len(arguments)-1; index++ {
				if arguments[index] == "--ro-bind" &&
					(arguments[index+1] == "/usr" ||
						arguments[index+1] == "/bin") {
					t.Fatalf("host toolchain bind = %q", arguments[index:index+2])
				}
			}
			mcpBody := readOpenFile(t, configFiles[0])
			catalogBody := readOpenFile(t, configFiles[1])
			switch family {
			case ProfileCodex:
				if bytes.Contains(environmentBody, capability) ||
					bytes.Count(mcpBody, capability) != 1 ||
					slicesContain(arguments, "--output-schema") ||
					slicesContain(arguments, "-o") ||
					slicesContain(arguments, "-c") ||
					!containsArgumentSequence(arguments, codexArguments(
						invocation.Selected.Model,
						true,
						nil,
					)) ||
					!bytes.Contains(catalogBody, []byte(`"shell_type":"disabled"`)) ||
					!bytes.Contains(catalogBody, []byte(`"input_modalities":["text"]`)) ||
					!bytes.Contains(catalogBody, []byte(`"supports_search_tool":false`)) {
					t.Fatalf(
						"Codex surface = args %q env %q mcp=%s catalog=%s",
						arguments,
						environment,
						mcpBody,
						catalogBody,
					)
				}
			case ProfileClaude:
				if bytes.Contains(environmentBody, capability) ||
					bytes.Count(mcpBody, capability) != 1 ||
					!bytes.Contains(mcpBody, []byte(`"alwaysLoad":true`)) ||
					slicesContain(arguments, "--json-schema") ||
					!containsArgumentSequence(
						arguments,
						claudeArguments(
							invocation.Selected.Model,
							invocation.Request.Workspace.Access,
							nil,
						),
					) {
					t.Fatalf(
						"Claude surface = args %q env %q mcp=%s",
						arguments,
						environment,
						mcpBody,
					)
				}
			}
			pair, err := nativeAutomationCertificationInvocations(
				invocation.Selected,
			)
			if err != nil {
				t.Fatal(err)
			}
			_, automationTools, err := nativeAutomationSurface(
				pair.Recovery,
			)
			if err != nil {
				t.Fatal(err)
			}
			automationLaunch, err := nativeAutomationLaunchInvocation(
				pair.Recovery,
			)
			if err != nil {
				t.Fatal(err)
			}
			automationArguments, automationEnvironment, _, err :=
				nativeCommand(
					config,
					automationLaunch,
					credential,
					closure,
					configFiles,
					nil,
					nil,
					automationTools,
				)
			if err != nil {
				t.Fatal(err)
			}
			defer clearEnvironment(automationEnvironment)
			if !containsArgumentSequence(
				automationArguments,
				[]string{"--remount-ro", GuestWorkspacePath},
			) ||
				nativeToolDefinitionsDigest(automationTools) ==
					nativeToolSurfaceDigest(ReadOnly) ||
				nativeToolDefinitionsDigest(automationTools) ==
					nativeToolSurfaceDigest(ReadWrite) {
				t.Fatalf(
					"automation containment = args %q tools %#v",
					automationArguments,
					automationTools,
				)
			}
			if family == ProfileClaude &&
				!containsArgumentSequence(
					automationArguments,
					claudeArgumentsWithTools(
						invocation.Selected.Model,
						automationTools,
						nil,
					),
				) {
				t.Fatalf(
					"Claude automation argv = %q",
					automationArguments,
				)
			}
			clearBytes(mcpBody)
			clearBytes(catalogBody)
		})
	}
}

func TestNativeRuntimeCertificationInspectsLiveNamespaceAndDescriptors(t *testing.T) {
	probe := buildNativeProbe(t)
	digest, err := executableDigest(probe)
	if err != nil {
		t.Fatal(err)
	}
	config := exactNativeConfigFixture(t, ProfileCodex)
	config.CLI = ExecutableIdentity{Path: probe, Digest: digest}
	config.CLIVersion = "test"
	config.VersionOutput = "test"
	invocation, _, _ := memoryInvocationFixture(t)
	invocation.Selected.Model = "native-probe-model"
	invocation.Request.Limits.TimeoutMillis = 300
	credentialPath := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(
		credentialPath,
		[]byte(`{"token":"namespace-canary"}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	observation, err := platformInvokeNative(
		context.Background(),
		invocation,
		config,
		credentialPath,
		nativeCertificateFixture(invocation, config),
	)
	if !errors.Is(err, context.DeadlineExceeded) ||
		observation.Handoff != nil ||
		time.Since(started) > 3*time.Second {
		t.Fatalf(
			"runtime certification result = %#v, elapsed=%s, error=%v",
			observation,
			time.Since(started),
			err,
		)
	}
}

func TestExactNativeCLIsCertifyFreshAndContinuationProviderRequests(t *testing.T) {
	models := map[ProfileFamily]string{
		ProfileCodex:  "sworn-capture-model",
		ProfileClaude: "claude-sonnet-4-20250514",
	}
	for _, family := range []ProfileFamily{ProfileCodex, ProfileClaude} {
		family := family
		t.Run(string(family), func(t *testing.T) {
			config := exactNativeConfigFixture(t, family)
			invocation, _, _ := memoryInvocationFixture(t)
			invocation.Selected.Model = models[family]
			invocation.Request.Model = models[family]
			invocation.Request.Limits.TimeoutMillis = 20_000
			permission, err := NewSubmissionPermission(
				invocation.Request,
				invocation.Selected,
				ContainmentReadWrite,
				PlannerProposal,
			)
			if err != nil {
				t.Fatal(err)
			}
			invocation.Permission = permission
			certificate, err := platformCaptureNativeSurface(
				context.Background(),
				nativeSmokeInvocationsFixture(t, invocation),
				config,
			)
			if err != nil {
				t.Fatalf("capture = %v", err)
			}
			if err := validateNativeSurfaceCertificate(
				certificate,
				invocation,
				config,
			); err != nil ||
				certificate.FreshReadOnly.CaptureEvidenceDigest == "" ||
				certificate.FreshReadWrite.CaptureEvidenceDigest == "" ||
				certificate.ContinuationStart.CaptureEvidenceDigest == "" ||
				certificate.Resume.CaptureEvidenceDigest == "" ||
				certificate.FreshReadOnly.ToolDigest !=
					nativeToolSurfaceDigest(ReadOnly) ||
				certificate.FreshReadWrite.ToolDigest !=
					nativeToolSurfaceDigest(ReadWrite) ||
				certificate.ContinuationStart.ToolDigest !=
					nativeToolSurfaceDigest(ReadOnly) ||
				certificate.Resume.ToolDigest != nativeToolSurfaceDigest(ReadWrite) {
				t.Fatalf("certificate = %#v, error=%v", certificate, err)
			}
		})
	}
}

func TestExactNativeCLIsKeepModelPromptOffOrdinaryDisk(t *testing.T) {
	setNativeMemoryRootEnv(t)
	models := map[ProfileFamily]string{
		ProfileCodex:  "sworn-capture-model",
		ProfileClaude: "claude-sonnet-4-20250514",
	}
	for _, family := range []ProfileFamily{ProfileCodex, ProfileClaude} {
		family := family
		t.Run(string(family), func(t *testing.T) {
			config := exactNativeConfigFixture(t, family)
			invocation, _, _ := memoryInvocationFixture(t)
			invocation.Selected.Model = models[family]
			invocation.Request.Model = invocation.Selected.Model
			invocation.Request.Limits.TimeoutMillis = 20_000
			smoke := nativeSmokeInvocationsFixture(t, invocation)
			state, err := newNativeContinuationState(config, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer state.closeContinuation()
			launch := &nativeContinuationLaunch{state: state}
			defer clearBytes(launch.capturedID)
			if _, err := platformCaptureNativeStage(
				context.Background(),
				smoke.ContinuationStart,
				config,
				launch,
			); err != nil {
				t.Fatalf("offline pinned CLI probe = %v", err)
			}
			state.mu.Lock()
			root := state.root
			state.mu.Unlock()
			found, err := nativeTreeContains(
				filepath.Join(root, "home"),
				[]byte("sworn.model-prompt/v1"),
			)
			if err != nil {
				t.Fatal(err)
			}
			if !nativeMemoryBackedPath(root) {
				if found {
					t.Fatal("complete Sworn model prompt persisted to ordinary disk")
				}
				t.Fatalf("native session home is not memory-backed: %q", root)
			}
		})
	}
}

func TestCodexFirstProviderRequestRejectsToolSurfaceMutation(t *testing.T) {
	const model = "sworn-capture-model"
	validate := func(t *testing.T, request map[string]any) error {
		t.Helper()
		body, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		defer clearBytes(body)
		_, _, err = validateNativeProviderRequest(
			body,
			ProfileCodex,
			model,
			toolDefinitions(ReadWrite),
		)
		return err
	}
	if err := validate(
		t,
		codexFirstProviderRequestFixture(t, model, ReadWrite),
	); err != nil {
		t.Fatalf("exact surface = %v", err)
	}
	t.Run("additional tool", func(t *testing.T) {
		request := codexFirstProviderRequestFixture(t, model, ReadWrite)
		request["tools"] = append(request["tools"].([]any), map[string]any{
			"type": "function", "name": "shell",
			"description": "ambient shell", "strict": false,
			"parameters": map[string]any{
				"type": "object", "properties": map[string]any{},
			},
		})
		if err := validate(t, request); !IsCode(err, "NATIVE_SURFACE_INVALID") {
			t.Fatalf("additional tool error = %v", err)
		}
	})
	t.Run("parallel calls", func(t *testing.T) {
		request := codexFirstProviderRequestFixture(t, model, ReadWrite)
		request["parallel_tool_calls"] = true
		if err := validate(t, request); !IsCode(err, "NATIVE_SURFACE_INVALID") {
			t.Fatalf("parallel calls error = %v", err)
		}
	})
	t.Run("missing broker tool", func(t *testing.T) {
		request := codexFirstProviderRequestFixture(t, model, ReadWrite)
		for _, raw := range request["tools"].([]any) {
			tool := raw.(map[string]any)
			if tool["type"] == "namespace" {
				tools := tool["tools"].([]any)
				tool["tools"] = tools[:len(tools)-1]
			}
		}
		if err := validate(t, request); !IsCode(err, "NATIVE_SURFACE_INVALID") {
			t.Fatalf("missing broker tool error = %v", err)
		}
	})
	t.Run("changed inert tool", func(t *testing.T) {
		request := codexFirstProviderRequestFixture(t, model, ReadWrite)
		for _, raw := range request["tools"].([]any) {
			tool := raw.(map[string]any)
			if tool["name"] == "update_plan" {
				tool["description"] = "changed"
			}
		}
		if err := validate(t, request); !IsCode(err, "NATIVE_SURFACE_INVALID") {
			t.Fatalf("changed inert tool error = %v", err)
		}
	})
}

func codexFirstProviderRequestFixture(
	t *testing.T,
	model string,
	access WorkspaceAccess,
) map[string]any {
	t.Helper()
	inert := codexInertProviderTools()
	names := []string{
		"list_mcp_resource_templates",
		"list_mcp_resources",
		"read_mcp_resource",
		"update_plan",
	}
	tools := make([]any, 0, len(names)+1)
	for _, name := range names {
		definition, present := inert[name]
		if !present {
			t.Fatalf("missing inert tool fixture %s", name)
		}
		schema, err := decodeStrict(
			definition.InputSchema,
			MaxToolArgumentBytes,
		)
		if err != nil {
			t.Fatal(err)
		}
		tools = append(tools, map[string]any{
			"type": "function", "name": name,
			"description": definition.Description,
			"strict":      false,
			"parameters":  schema,
		})
	}
	namespaceTools := make([]any, 0, len(toolDefinitions(access)))
	for _, definition := range toolDefinitions(access) {
		schema, err := decodeStrict(
			definition.InputSchema,
			MaxToolArgumentBytes,
		)
		if err != nil {
			t.Fatal(err)
		}
		normalizeCodexProviderSchema(schema)
		namespaceTools = append(namespaceTools, map[string]any{
			"type": "function", "name": definition.Name,
			"description": definition.Description,
			"strict":      false,
			"parameters":  schema,
		})
	}
	tools = append(tools, map[string]any{
		"type": "namespace", "name": "mcp__sworn",
		"description": "Tools in the mcp__sworn namespace.",
		"tools":       namespaceTools,
	})
	return map[string]any{
		"model":               model,
		"tools":               tools,
		"tool_choice":         "auto",
		"parallel_tool_calls": false,
	}
}

func TestClaudeFirstProviderRequestRejectsToolSurfaceMutation(t *testing.T) {
	const model = "sworn-capture-model"
	validate := func(t *testing.T, request map[string]any) error {
		t.Helper()
		body, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		defer clearBytes(body)
		_, _, err = validateNativeProviderRequest(
			body,
			ProfileClaude,
			model,
			toolDefinitions(ReadWrite),
		)
		return err
	}
	if err := validate(
		t,
		claudeFirstProviderRequestFixture(t, model, ReadWrite),
	); err != nil {
		t.Fatalf("exact surface = %v", err)
	}
	t.Run("additional tool", func(t *testing.T) {
		request := claudeFirstProviderRequestFixture(t, model, ReadWrite)
		request["tools"] = append(request["tools"].([]any), map[string]any{
			"name":        "mcp__sworn__shell",
			"description": "ambient shell",
			"input_schema": map[string]any{
				"type": "object", "properties": map[string]any{},
			},
		})
		if err := validate(t, request); !IsCode(err, "NATIVE_SURFACE_INVALID") {
			t.Fatalf("additional tool error = %v", err)
		}
	})
	t.Run("changed description", func(t *testing.T) {
		request := claudeFirstProviderRequestFixture(t, model, ReadWrite)
		tools := request["tools"].([]any)
		tools[0].(map[string]any)["description"] = "changed"
		if err := validate(t, request); !IsCode(err, "NATIVE_SURFACE_INVALID") {
			t.Fatalf("changed description error = %v", err)
		}
	})
	t.Run("missing tool", func(t *testing.T) {
		request := claudeFirstProviderRequestFixture(t, model, ReadWrite)
		tools := request["tools"].([]any)
		request["tools"] = tools[:len(tools)-1]
		if err := validate(t, request); !IsCode(err, "NATIVE_SURFACE_INVALID") {
			t.Fatalf("missing tool error = %v", err)
		}
	})
	t.Run("unexpected field", func(t *testing.T) {
		request := claudeFirstProviderRequestFixture(t, model, ReadWrite)
		tools := request["tools"].([]any)
		tools[0].(map[string]any)["cache_control"] = map[string]any{
			"type": "ephemeral",
		}
		if err := validate(t, request); !IsCode(err, "NATIVE_SURFACE_INVALID") {
			t.Fatalf("unexpected field error = %v", err)
		}
	})
}

func claudeFirstProviderRequestFixture(
	t *testing.T,
	model string,
	access WorkspaceAccess,
) map[string]any {
	t.Helper()
	definitions := toolDefinitions(access)
	tools := make([]any, 0, len(definitions))
	for _, definition := range definitions {
		schema, err := decodeStrict(
			definition.InputSchema,
			MaxToolArgumentBytes,
		)
		if err != nil {
			t.Fatal(err)
		}
		tools = append(tools, map[string]any{
			"name":         "mcp__sworn__" + definition.Name,
			"description":  definition.Description,
			"input_schema": schema,
		})
	}
	return map[string]any{
		"model": model,
		"tools": tools,
	}
}

func TestNativeInitializationCaptureRejectsAmbientCapabilities(t *testing.T) {
	t.Run("Claude", func(t *testing.T) {
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
		tools := make([]string, 0)
		for _, definition := range toolDefinitions(ReadWrite) {
			tools = append(tools, "mcp__sworn__"+definition.Name)
		}
		event := map[string]any{
			"type": "system", "subtype": "init",
			"model": "exact-native-model", "permissionMode": "dontAsk",
			"slash_commands": []any{}, "skills": []any{}, "plugins": []any{},
			"tools": tools, "capabilities": []any{
				"interrupt_receipt_v1", "interrupt_cancel_queued_v1", "msg_lifecycle_v1",
			},
			"analytics_disabled":        true,
			"product_feedback_disabled": true,
			"mcp_servers": []any{map[string]any{
				"name": "sworn", "status": "connected",
			}},
		}
		state := &nativeEventState{
			family: ProfileClaude, model: "exact-native-model",
			definitions: toolDefinitions(ReadWrite), broker: broker,
		}
		body, _ := json.Marshal(event)
		capability := broker.capability()
		defer clearBytes(capability)
		if err := completeBrokerHandshake(
			t,
			broker,
			capability,
			"claude-code",
			ClaudeCLIVersion,
		); err != nil {
			t.Fatal(err)
		}
		if broker.Ready() {
			t.Fatal("broker opened before Claude init event")
		}
		if err := state.accept(body); err != nil || !state.nativeSeen ||
			!broker.Ready() {
			t.Fatalf("Claude init = %v, state=%#v", err, state)
		}
		ambient := make([]any, len(tools), len(tools)+1)
		for index, name := range tools {
			ambient[index] = name
		}
		ambient = append(ambient, "StructuredOutput")
		if exactClaudeTools(ambient, toolDefinitions(ReadWrite)) {
			t.Fatal("competing StructuredOutput tool was accepted")
		}
	})

	t.Run("Codex", func(t *testing.T) {
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
			family: ProfileCodex, model: "exact-native-model",
			definitions: toolDefinitions(ReadWrite), broker: broker,
		}
		if err := state.accept([]byte(
			`{"type":"thread.started","thread_id":"thread-1"}`,
		)); err != nil || !state.nativeSeen || broker.Ready() {
			t.Fatalf("Codex init = %v, state=%#v", err, state)
		}
		capability := broker.capability()
		defer clearBytes(capability)
		if err := completeBrokerHandshake(
			t,
			broker,
			capability,
			"codex",
			CodexCLIVersion,
		); err != nil {
			t.Fatal(err)
		}
		if !broker.Ready() {
			t.Fatal("broker did not open after exact Codex event and handshake")
		}
		if err := state.accept([]byte(
			`{"type":"item.started","item":{"type":"command_execution"}}`,
		)); !IsCode(err, "NATIVE_SURFACE_INVALID") {
			t.Fatalf("ambient Codex shell error = %v", err)
		}
	})
}

func nativeCertificateFixture(
	invocation Invocation,
	config NativeAdapterConfig,
) nativeSurfaceCertificate {
	digest := "sha256:" + strings.Repeat("a", 64)
	return nativeSurfaceCertificateFixture(
		invocation,
		config,
		"codex",
		CodexCLIVersion,
		digest,
	)
}

func nativeSurfaceCertificateFixture(
	invocation Invocation,
	config NativeAdapterConfig,
	clientName string,
	clientVersion string,
	captureDigest string,
) nativeSurfaceCertificate {
	initializeBody, _ := canonicalJSON(map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name": clientName, "version": clientVersion,
		},
	})
	emptyBody, _ := canonicalJSON(map[string]any{})
	stage := func(
		access WorkspaceAccess,
		invocationStage nativeInvocationStage,
	) nativeSurfaceStageCertificate {
		return nativeSurfaceStageCertificate{
			Access:                access,
			InvocationStage:       invocationStage,
			ToolDigest:            nativeToolSurfaceDigest(access),
			CaptureEvidenceDigest: captureDigest,
			ArgumentDigest: nativeCLIArgumentDigest(
				config.Family,
				invocation.Selected.Model,
				access,
				invocationStage,
			),
			Protocol:           "2025-06-18",
			ClientName:         clientName,
			ClientVersion:      clientVersion,
			InitializeDigest:   Digest(initializeBody),
			NotificationDigest: Digest(emptyBody),
			ListDigest:         Digest(emptyBody),
		}
	}
	return nativeSurfaceCertificate{
		Family:              config.Family,
		ProfileDigest:       nativeProfileDigest(invocation.Selected.Profile),
		Model:               invocation.Selected.Model,
		AdapterConfigDigest: invocation.Selected.Adapter.ConfigurationDigest,
		ExecutableDigest:    config.CLI.Digest,
		CLIVersion:          config.CLIVersion,
		FreshReadOnly:       stage(ReadOnly, nativeInvocationStageFresh),
		FreshReadWrite:      stage(ReadWrite, nativeInvocationStageFresh),
		ContinuationStart: stage(
			ReadOnly,
			nativeInvocationStageContinuationStart,
		),
		ContinuationStartRW: stage(
			ReadWrite,
			nativeInvocationStageContinuationStart,
		),
		ResumeReadOnly: stage(ReadOnly, nativeInvocationStageResume),
		Resume:         stage(ReadWrite, nativeInvocationStageResume),
	}
}

func nativeSmokeInvocationsFixture(
	t *testing.T,
	base Invocation,
) NativeSmokeInvocations {
	t.Helper()
	build := func(
		suffix string,
		responsibility Responsibility,
		access WorkspaceAccess,
		fresh bool,
	) Invocation {
		request, err := NewRequest(
			base.Request.InvocationID+"-"+suffix,
			RoleImplementer,
			base.Selected.Profile.Key,
			base.Selected.Model,
			Workspace{Path: GuestWorkspacePath, Access: access},
			base.Request.Inputs,
			fresh,
			base.Request.Limits,
		)
		if err != nil {
			t.Fatal(err)
		}
		containment := ContainmentReadOnly
		if access == ReadWrite {
			containment = ContainmentReadWrite
		}
		permission, err := NewSubmissionPermission(
			request,
			base.Selected,
			containment,
			responsibility,
		)
		if err != nil {
			t.Fatal(err)
		}
		value := base
		value.Request = request
		value.Permission = permission
		return value
	}
	return NativeSmokeInvocations{
		FreshReadOnly: build(
			"fresh-read-only",
			ImplementerDesign,
			ReadOnly,
			true,
		),
		FreshReadWrite: build(
			"fresh-read-write",
			ImplementerDesign,
			ReadWrite,
			true,
		),
		ContinuationStart: build(
			"continuation-start",
			ImplementerDesign,
			ReadOnly,
			true,
		),
		Resume: build(
			"resume",
			ImplementerImplementation,
			ReadWrite,
			false,
		),
	}
}

func nativeSameDutyInvocationFixture(
	t *testing.T,
	base Invocation,
	invocationID string,
) Invocation {
	t.Helper()
	descriptor, err := base.Permission.Describe()
	if err != nil {
		t.Fatal(err)
	}
	request, err := NewRequest(
		invocationID,
		base.Request.Role,
		base.Selected.Profile.Key,
		base.Selected.Model,
		base.Request.Workspace,
		base.Request.Inputs,
		base.Request.FreshContext,
		base.Request.Limits,
	)
	if err != nil {
		t.Fatal(err)
	}
	permission, err := NewSubmissionPermission(
		request,
		base.Selected,
		descriptor.Containment,
		descriptor.Responsibility,
	)
	if err != nil {
		t.Fatal(err)
	}
	result := base
	result.Request = request
	result.Permission = permission
	return result
}

func nativeVerifierInvocationFixture(
	t *testing.T,
	base Invocation,
	invocationID string,
	fresh bool,
) Invocation {
	t.Helper()
	request, err := NewRequest(
		invocationID, RoleVerifier, base.Selected.Profile.Key,
		base.Selected.Model,
		Workspace{Path: GuestWorkspacePath, Access: ReadOnly},
		base.Request.Inputs, fresh, base.Request.Limits,
	)
	if err != nil {
		t.Fatal(err)
	}
	permission, err := NewSubmissionPermission(
		request, base.Selected, ContainmentReadOnly, WorkVerification,
	)
	if err != nil {
		t.Fatal(err)
	}
	base.Request, base.Permission = request, permission
	return base
}

func completeBrokerHandshake(
	t *testing.T,
	broker *nativeBroker,
	capability []byte,
	clientName string,
	clientVersion string,
) error {
	t.Helper()
	status, body := brokerRequest(
		t,
		broker,
		capability,
		map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]any{},
				"clientInfo": map[string]any{
					"name": clientName, "version": clientVersion,
				},
			},
		},
	)
	clearBytes(body)
	if status != http.StatusOK {
		return fail("INVALID_BROKER")
	}
	status, body = brokerRequest(
		t,
		broker,
		capability,
		map[string]any{
			"jsonrpc": "2.0", "method": "notifications/initialized",
			"params": map[string]any{},
		},
	)
	clearBytes(body)
	if status != http.StatusAccepted {
		return fail("INVALID_BROKER")
	}
	status, body = brokerRequest(
		t,
		broker,
		capability,
		map[string]any{
			"jsonrpc": "2.0", "id": 2, "method": "tools/list",
			"params": map[string]any{},
		},
	)
	clearBytes(body)
	if status != http.StatusOK {
		return fail("INVALID_BROKER")
	}
	return nil
}

func TestSecretGuardFindsCapabilityAcrossWriteBoundaries(t *testing.T) {
	t.Parallel()
	capability := []byte("capability-canary")
	guard := newSecretGuard(capability, 1_024)
	_, _ = guard.Write([]byte("prefix-capability-"))
	_, _ = guard.Write([]byte("canary-suffix"))
	if !guard.leaked() {
		t.Fatal("split capability was not detected")
	}
}

func TestNativeContinuationResumesExactPrivateSessionWithFreshAuthority(
	t *testing.T,
) {
	setNativeMemoryRootEnv(t)
	for _, family := range []ProfileFamily{ProfileCodex, ProfileClaude} {
		family := family
		t.Run(string(family), func(t *testing.T) {
			binary := buildNativeContinuation(t)
			digest, err := executableDigest(binary)
			if err != nil {
				t.Fatal(err)
			}
			config := nativeContinuationConfigFixture(
				t,
				family,
				binary,
				digest,
			)
			configBody, err := canonicalJSON(config)
			if err != nil {
				t.Fatal(err)
			}
			identity := AdapterIdentity{
				Key: config.Key, ID: config.ID, Version: config.Version,
				ConfigurationDigest: Digest(configBody),
			}
			ref := config.CredentialRefs[0]
			credential := filepath.Join(t.TempDir(), "credential")
			const credentialCanary = "native-continuation-credential-canary"
			if err := os.WriteFile(
				credential,
				[]byte(`{"token":"`+credentialCanary+`"}`),
				0o600,
			); err != nil {
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
				Key:     "native-continuation-profile-" + string(family),
				Adapter: identity.Key, Network: NetworkRequired,
				CredentialRef: &ref,
			}
			selected := SelectedProfile{
				Profile: profile,
				Adapter: identity,
				Model:   "native-continuation-model",
				adapter: adapter,
			}
			base, _, _ := memoryInvocationFixture(t)
			base.Selected = selected
			pair := nativeSmokeInvocationsFixture(t, base)
			binding := continuationContractBinding()
			observation, handle, result, err := (Dispatcher{}).InvokeTurn(
				context.Background(),
				pair.ContinuationStart,
				binding,
				nil,
			)
			if err != nil || observation.Handoff == nil || handle == nil ||
				result.Mode != ContinuationModeNativeSession ||
				result.Status != ContinuationStatusSuspended {
				t.Fatalf(
					"design = observation %#v, handle %p, result %#v, error %v",
					observation,
					handle,
					result,
					err,
				)
			}
			cell := continuationCellFor(handle)
			cell.mu.Lock()
			nativeState, ok := cell.state.(*nativeContinuationState)
			cell.mu.Unlock()
			if !ok || nativeState.continuationBytes() < 1 {
				t.Fatal("design did not retain bounded native state")
			}
			nativeState.mu.Lock()
			root := nativeState.root
			sessionID := append([]byte(nil), nativeState.sessionID...)
			nativeState.mu.Unlock()
			if !validNativeSessionID(sessionID) ||
				!validNativeSessionRoot(root) ||
				!nativeMemoryBackedPath(root) {
				clearBytes(sessionID)
				t.Fatal("native state identity or root invalid")
			}
			clearBytes(sessionID)
			observation, next, result, err := (Dispatcher{}).InvokeTurn(
				context.Background(),
				pair.Resume,
				binding,
				handle,
			)
			if err != nil || observation.Handoff == nil || next != nil ||
				result.Mode != ContinuationModeNativeSession ||
				result.Status != ContinuationStatusResumed {
				t.Fatalf(
					"resume = observation %#v, next %p, result %#v, error %v",
					observation,
					next,
					result,
					err,
				)
			}
			if _, err := os.Lstat(root); !os.IsNotExist(err) {
				t.Fatal("consumed native session root still exists")
			}
			if family == ProfileCodex {
				verifierStart := nativeVerifierInvocationFixture(
					t, pair.ContinuationStart,
					"native-verifier-start", true,
				)
				observation, handle, result, err = (Dispatcher{}).InvokeTurn(
					context.Background(), verifierStart, binding, nil,
				)
				if err != nil || observation.Handoff == nil ||
					handle == nil ||
					result.Status != ContinuationStatusSuspended {
					t.Fatalf(
						"verifier start = %#v, %p, %#v, %v",
						observation, handle, result, err,
					)
				}
				verifierResume := nativeVerifierInvocationFixture(
					t, pair.Resume,
					"native-verifier-repair", false,
				)
				repairBinding := binding
				repairBinding.Attempt = 2
				observation, next, result, err = (Dispatcher{}).InvokeTurn(
					context.Background(), verifierResume,
					repairBinding, handle,
				)
				if err != nil || observation.Handoff == nil ||
					next == nil ||
					result.Status != ContinuationStatusSuspended {
					t.Fatalf(
						"verifier repair = %#v, %p, %#v, %v",
						observation, next, result, err,
					)
				}
				if err := next.Close(); err != nil {
					t.Fatal(err)
				}
			}

			promotionStart := nativeSameDutyInvocationFixture(
				t,
				pair.ContinuationStart,
				"native-recoverable-promote-"+string(family),
			)
			observation, handle, result, err = (Dispatcher{}).InvokeTurn(
				context.Background(),
				promotionStart,
				binding,
				nil,
			)
			if err != nil || observation.Yield == nil || handle == nil ||
				result.Mode != ContinuationModeNativeSession ||
				result.Status != ContinuationStatusSuspended {
				t.Fatalf(
					"promotion start = observation %#v, handle %p, result %#v, error %v",
					observation,
					handle,
					result,
					err,
				)
			}
			promotionBinding := binding
			promotionBinding.ToolContractDigest =
				Digest([]byte("native-promoted-design-contract"))
			promotionInput := &RecoverableTurnInput{
				SchemaVersion: RecoverableTurnInputSchemaVersion,
				Kind:          RecoverableInputAnswer,
				Answer:        "Use the exact admitted design evidence.",
				TargetBinding: &promotionBinding,
			}
			observation, next, result, err =
				(Dispatcher{}).InvokeRecoverableTurn(
					context.Background(),
					promotionStart,
					binding,
					handle,
					promotionInput,
				)
			if err != nil || observation.Handoff == nil ||
				observation.Yield != nil || next == nil ||
				result.Mode != ContinuationModeNativeSession ||
				result.Status != ContinuationStatusSuspended {
				t.Fatalf(
					"native design promotion = observation %#v, next %p, result %#v, error %v",
					observation,
					next,
					result,
					err,
				)
			}
			promotedCell := continuationCellFor(next)
			promotedCell.mu.Lock()
			promotedState, ok :=
				promotedCell.state.(*nativeContinuationState)
			promotedCell.mu.Unlock()
			if !ok {
				t.Fatal("promotion did not retain native state")
			}
			promotedState.mu.Lock()
			promotedRoot := promotedState.root
			promotedState.mu.Unlock()
			promotionResume := nativeSameDutyInvocationFixture(
				t,
				pair.Resume,
				"native-promoted-implementation-"+string(family),
			)
			observation, handle, result, err = (Dispatcher{}).InvokeTurn(
				context.Background(),
				promotionResume,
				promotionBinding,
				next,
			)
			if err != nil || observation.Handoff == nil ||
				observation.Yield != nil || handle != nil ||
				result.Mode != ContinuationModeNativeSession ||
				result.Status != ContinuationStatusResumed {
				t.Fatalf(
					"promoted native resume = observation %#v, handle %p, result %#v, error %v",
					observation,
					handle,
					result,
					err,
				)
			}
			if _, err := os.Lstat(promotedRoot); !os.IsNotExist(err) {
				t.Fatal("promoted native session root still exists")
			}

			plain, err := (Dispatcher{}).Invoke(
				context.Background(),
				pair.FreshReadOnly,
			)
			if err != nil || plain.Handoff == nil {
				t.Fatalf("plain ephemeral invocation = %#v, %v", plain, err)
			}

			_, failedHandle, _, err := (Dispatcher{}).InvokeTurn(
				context.Background(),
				pair.ContinuationStart,
				binding,
				nil,
			)
			if err != nil || failedHandle == nil {
				t.Fatalf("failed-resume setup = %p, %v", failedHandle, err)
			}
			failedCell := continuationCellFor(failedHandle)
			failedCell.mu.Lock()
			failedState, ok := failedCell.state.(*nativeContinuationState)
			failedCell.mu.Unlock()
			if !ok {
				t.Fatal("failed-resume setup did not retain native state")
			}
			failedState.mu.Lock()
			failedRoot := failedState.root
			failedState.mu.Unlock()
			if err := os.Remove(
				filepath.Join(failedRoot, "home", ".native-session-id"),
			); err != nil {
				t.Fatal(err)
			}
			observation, next, result, err = (Dispatcher{}).InvokeTurn(
				context.Background(),
				pair.Resume,
				binding,
				failedHandle,
			)
			if err != nil || !reflect.DeepEqual(observation, Observation{}) ||
				next != nil ||
				result.Mode != ContinuationModeFreshRehydrate ||
				result.Status != ContinuationStatusMismatch {
				t.Fatalf(
					"failed resume = observation %#v, next %p, result %#v, error %v",
					observation,
					next,
					result,
					err,
				)
			}
			if _, err := os.Lstat(failedRoot); !os.IsNotExist(err) {
				t.Fatal("failed native session root still exists")
			}

			reservations := 0
			prose := nativeSameDutyInvocationFixture(
				t,
				pair.ContinuationStart,
				"native-prose-nudge-"+string(family),
			)
			prose.RecoveryStepHook = func(
				_ context.Context,
				kind RecoveryStepKind,
			) error {
				if kind != RecoveryStepProseNudge {
					t.Fatalf("native recovery kind = %s", kind)
				}
				reservations++
				return nil
			}
			observation, next, result, err = (Dispatcher{}).InvokeTurn(
				context.Background(),
				prose,
				binding,
				nil,
			)
			if err != nil || observation.Handoff == nil ||
				observation.Yield != nil || next == nil ||
				result.Mode != ContinuationModeNativeSession ||
				result.Status != ContinuationStatusSuspended ||
				reservations != 1 {
				t.Fatalf(
					"native prose nudge = observation %#v, next %p, result %#v, reservations %d, error %v",
					observation,
					next,
					result,
					reservations,
					err,
				)
			}
			if err := next.Close(); err != nil {
				t.Fatal(err)
			}

			reservations = 0
			twice := nativeSameDutyInvocationFixture(
				t,
				pair.ContinuationStart,
				"native-prose-nudge-twice-"+string(family),
			)
			twice.RecoveryStepHook = prose.RecoveryStepHook
			observation, next, result, err = (Dispatcher{}).InvokeTurn(
				context.Background(),
				twice,
				binding,
				nil,
			)
			if !IsCode(err, "MISSING_SUBMISSION") ||
				observation.Diagnostic.Code != "adapter_failed" ||
				next != nil ||
				result.Status != ContinuationStatusCompleted ||
				reservations != 1 {
				t.Fatalf(
					"second native prose = observation %#v, next %p, result %#v, reservations %d, error %v",
					observation,
					next,
					result,
					reservations,
					err,
				)
			}

			start := nativeSameDutyInvocationFixture(
				t,
				pair.ContinuationStart,
				"native-recoverable-start-"+string(family),
			)
			observation, handle, result, err = (Dispatcher{}).
				InvokeRecoverableTurn(
					context.Background(),
					start,
					binding,
					nil,
					nil,
				)
			if err != nil || observation.Yield == nil ||
				observation.Handoff != nil || handle == nil ||
				result.Mode != ContinuationModeNativeSession ||
				result.Status != ContinuationStatusSuspended {
				t.Fatalf(
					"recoverable start = observation %#v, handle %p, result %#v, error %v",
					observation,
					handle,
					result,
					err,
				)
			}
			firstCell := continuationCellFor(handle)
			firstCell.mu.Lock()
			firstState, ok := firstCell.state.(*nativeContinuationState)
			firstCell.mu.Unlock()
			if !ok {
				t.Fatal("recoverable start did not retain native state")
			}
			firstState.mu.Lock()
			recoverableRoot := firstState.root
			firstState.mu.Unlock()

			yieldAgain := RecoverableTurnInput{
				SchemaVersion: RecoverableTurnInputSchemaVersion,
				Kind:          RecoverableInputAnswer,
				Answer:        "Please yield again.",
			}
			resumeYield := nativeSameDutyInvocationFixture(
				t,
				start,
				"native-recoverable-resume-yield-"+string(family),
			)
			observation, next, result, err = (Dispatcher{}).
				InvokeRecoverableTurn(
					context.Background(),
					resumeYield,
					binding,
					handle,
					&yieldAgain,
				)
			if err != nil || observation.Yield == nil || next == nil ||
				result.Mode != ContinuationModeNativeSession ||
				result.Status != ContinuationStatusSuspended {
				t.Fatalf(
					"recoverable yielded resume = observation %#v, next %p, result %#v, error %v",
					observation,
					next,
					result,
					err,
				)
			}
			nextCell := continuationCellFor(next)
			nextCell.mu.Lock()
			nextState, ok := nextCell.state.(*nativeContinuationState)
			nextCell.mu.Unlock()
			if !ok {
				t.Fatal("yielded resume did not transfer native state")
			}
			nextState.mu.Lock()
			nextRoot := nextState.root
			nextState.mu.Unlock()
			if nextRoot != recoverableRoot {
				t.Fatalf(
					"native session root changed: %q -> %q",
					recoverableRoot,
					nextRoot,
				)
			}

			complete := RecoverableTurnInput{
				SchemaVersion: RecoverableTurnInputSchemaVersion,
				Kind:          RecoverableInputAnswer,
				Answer:        "Complete the fixture now.",
			}
			resumeComplete := nativeSameDutyInvocationFixture(
				t,
				start,
				"native-recoverable-resume-complete-"+string(family),
			)
			observation, handle, result, err = (Dispatcher{}).
				InvokeRecoverableTurn(
					context.Background(),
					resumeComplete,
					binding,
					next,
					&complete,
				)
			if err != nil || observation.Handoff == nil ||
				observation.Yield != nil || handle != nil ||
				result.Mode != ContinuationModeNativeSession ||
				result.Status != ContinuationStatusResumed {
				t.Fatalf(
					"recoverable completion = observation %#v, handle %p, result %#v, error %v",
					observation,
					handle,
					result,
					err,
				)
			}
			if _, err := os.Lstat(recoverableRoot); !os.IsNotExist(err) {
				t.Fatal("completed recoverable native session root still exists")
			}

			resumeProseStart := nativeSameDutyInvocationFixture(
				t,
				pair.ContinuationStart,
				"native-recoverable-resume-prose-start-"+string(family),
			)
			observation, handle, result, err = (Dispatcher{}).
				InvokeRecoverableTurn(
					context.Background(),
					resumeProseStart,
					binding,
					nil,
					nil,
				)
			if err != nil || observation.Yield == nil || handle == nil {
				t.Fatalf(
					"resume prose setup = observation %#v, handle %p, result %#v, error %v",
					observation,
					handle,
					result,
					err,
				)
			}
			resumeProseCell := continuationCellFor(handle)
			resumeProseCell.mu.Lock()
			resumeProseState, ok :=
				resumeProseCell.state.(*nativeContinuationState)
			resumeProseCell.mu.Unlock()
			if !ok {
				t.Fatal("resume prose setup did not retain native state")
			}
			resumeProseState.mu.Lock()
			resumeProseRoot := resumeProseState.root
			resumeProseState.mu.Unlock()
			resumeCount, err := os.Open(filepath.Join(
				resumeProseRoot,
				"home",
				".native-resume-count",
			))
			if err != nil {
				t.Fatal(err)
			}
			defer resumeCount.Close()
			reservations = 0
			resumeProse := nativeSameDutyInvocationFixture(
				t,
				resumeProseStart,
				"native-recoverable-resume-prose-answer-"+string(family),
			)
			resumeProse.RecoveryStepHook = prose.RecoveryStepHook
			observation, next, result, err = (Dispatcher{}).
				InvokeRecoverableTurn(
					context.Background(),
					resumeProse,
					binding,
					handle,
					&complete,
				)
			if err != nil || observation.Handoff == nil ||
				observation.Yield != nil || next != nil ||
				result.Mode != ContinuationModeNativeSession ||
				result.Status != ContinuationStatusResumed ||
				reservations != 1 {
				t.Fatalf(
					"resume prose nudge = observation %#v, next %p, result %#v, reservations %d, error %v",
					observation,
					next,
					result,
					reservations,
					err,
				)
			}
			count := []byte{0}
			if read, readErr := resumeCount.ReadAt(count, 0); readErr != nil ||
				read != 1 || string(count) != "2" {
				t.Fatalf(
					"same native root resume count = %q, bytes=%d, error=%v",
					count,
					read,
					readErr,
				)
			}
			if _, err := os.Lstat(resumeProseRoot); !os.IsNotExist(err) {
				t.Fatal("completed resume-prose native root still exists")
			}
		})
	}
}

func TestNativeContinuationArgumentsAreExplicitAndNonInteractive(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	resume := &nativeContinuationLaunch{
		resume:     true,
		expectedID: []byte(id),
	}
	design := &nativeContinuationLaunch{}
	codexResume := codexArguments(
		"native-continuation-model",
		false,
		resume,
	)
	if !slices.Equal(
		codexResume[:4],
		[]string{"exec", "-C", GuestWorkspacePath, "resume"},
	) ||
		!containsArgumentSequence(codexResume, []string{id, "-"}) ||
		slicesContain(codexResume, "--ephemeral") ||
		slicesContain(codexResume, "--last") ||
		slicesContain(codexResume, "--all") {
		t.Fatalf("Codex resume argv = %q", codexResume)
	}
	codexDesign := codexArguments(
		"native-continuation-model",
		true,
		design,
	)
	if slicesContain(codexDesign, "--ephemeral") ||
		slicesContain(codexDesign, "resume") {
		t.Fatalf("Codex design argv = %q", codexDesign)
	}
	if !slicesContain(
		codexArguments("native-continuation-model", true, nil),
		"--ephemeral",
	) {
		t.Fatal("plain Codex invocation is not ephemeral")
	}

	claudeResume := claudeArguments(
		"native-continuation-model",
		ReadWrite,
		resume,
	)
	if !containsArgumentSequence(
		claudeResume,
		[]string{"--resume", id},
	) ||
		slicesContain(claudeResume, "--continue") ||
		slicesContain(claudeResume, "--fork-session") ||
		slicesContain(claudeResume, "--no-session-persistence") {
		t.Fatalf("Claude resume argv = %q", claudeResume)
	}
	claudeDesign := claudeArguments(
		"native-continuation-model",
		ReadOnly,
		design,
	)
	if slicesContain(claudeDesign, "--resume") ||
		slicesContain(claudeDesign, "--no-session-persistence") {
		t.Fatalf("Claude design argv = %q", claudeDesign)
	}
	if !slicesContain(
		claudeArguments("native-continuation-model", ReadOnly, nil),
		"--no-session-persistence",
	) {
		t.Fatal("plain Claude invocation is not ephemeral")
	}

	for _, family := range []ProfileFamily{ProfileCodex, ProfileClaude} {
		invocation, _, _ := memoryInvocationFixture(t)
		invocation.Selected.Model = "native-continuation-model"
		pair := nativeSmokeInvocationsFixture(t, invocation)
		certificate := nativeContinuationCertificateFixture(
			pair.ContinuationStart,
			NativeAdapterConfig{Family: family},
		)
		freshStage, err := nativeCertificateStage(
			certificate,
			pair.FreshReadWrite,
			nil,
		)
		if err != nil || freshStage != certificate.FreshReadWrite ||
			freshStage == certificate.Resume ||
			freshStage.ArgumentDigest == certificate.Resume.ArgumentDigest {
			t.Fatalf(
				"%s fresh read-write stage borrowed resume evidence",
				family,
			)
		}
		if _, err := nativeCertificateStage(
			certificate,
			pair.Resume,
			nil,
		); !IsCode(err, "NATIVE_NOT_CERTIFIED") {
			t.Fatalf("%s resume invocation certified as fresh: %v", family, err)
		}
		resumeStage, err := nativeCertificateStage(
			certificate,
			pair.Resume,
			resume,
		)
		if err != nil || resumeStage != certificate.Resume {
			t.Fatalf("%s explicit resume stage = %#v, %v", family, resumeStage, err)
		}
	}
}

func TestNativeContinuationIdentityMismatchPrecedesBrokerArm(t *testing.T) {
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
	launch := &nativeContinuationLaunch{
		resume: true,
		expectedID: []byte(
			"11111111-1111-4111-8111-111111111111",
		),
	}
	state := &nativeEventState{
		family:      ProfileCodex,
		model:       "native-continuation-model",
		definitions: toolDefinitions(ReadWrite),
		broker:      broker,
		launch:      launch,
	}
	err = state.accept([]byte(
		`{"type":"thread.started","thread_id":"22222222-2222-4222-8222-222222222222"}`,
	))
	if !IsCode(err, "NATIVE_SURFACE_INVALID") ||
		broker.Ready() || state.nativeSeen || state.identityAccepted {
		t.Fatalf("mismatch armed broker: state=%#v error=%v", state, err)
	}
}

func TestNativeContinuationCleanupBoundsAndStaleRecovery(t *testing.T) {
	setNativeMemoryRootEnv(t)
	config := NativeAdapterConfig{
		Family:           ProfileCodex,
		CredentialTarget: CodexCredentialTarget,
	}
	state, err := newNativeContinuationState(config, 0)
	if err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	root := state.root
	state.mu.Unlock()
	if err := state.retainSessionID(
		[]byte("11111111-1111-4111-8111-111111111111"),
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "home", ".codex", "session"),
		[]byte("bounded"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := reapNativeSessionRoots(0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal("locked live root was reaped")
	}
	if err := state.closeContinuation(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("closed root was not removed")
	}

	stale, err := newNativeContinuationState(config, 0)
	if err != nil {
		t.Fatal(err)
	}
	stale.mu.Lock()
	staleRoot, home, lease := stale.root, stale.home, stale.lease
	stale.home, stale.lease, stale.closed = nil, nil, true
	stale.mu.Unlock()
	_ = home.Close()
	_ = syscall.Flock(int(lease.Fd()), syscall.LOCK_UN)
	_ = lease.Close()
	if err := reapNativeSessionRoots(0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(staleRoot); !os.IsNotExist(err) {
		t.Fatal("unlocked stale root was not recovered")
	}

	memoryRoot, err := nativeSessionMemoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	incomplete, err := os.MkdirTemp(
		memoryRoot,
		nativeSessionRootPrefix,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(incomplete) })
	if err := os.Chmod(incomplete, 0o700); err != nil {
		t.Fatal(err)
	}
	defaultLifetime := time.Duration(DefaultContinuationLifetimeMillis) * time.Millisecond
	expired := time.Now().Add(-defaultLifetime - time.Minute)
	if err := os.Chtimes(incomplete, expired, expired); err != nil {
		t.Fatal(err)
	}
	if err := reapNativeSessionRoots(0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(incomplete); !os.IsNotExist(err) {
		t.Fatal("expired incomplete root was not recovered")
	}

	// A governed 1h lifetime reaps a root older than 1h and keeps one
	// newer than that bound (A1).
	customLifetime := time.Hour
	oldCustom, err := os.MkdirTemp(memoryRoot, nativeSessionRootPrefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(oldCustom) })
	if err := os.Chmod(oldCustom, 0o700); err != nil {
		t.Fatal(err)
	}
	oldStamp := time.Now().Add(-customLifetime - time.Minute)
	if err := os.Chtimes(oldCustom, oldStamp, oldStamp); err != nil {
		t.Fatal(err)
	}
	freshCustom, err := os.MkdirTemp(memoryRoot, nativeSessionRootPrefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(freshCustom) })
	if err := os.Chmod(freshCustom, 0o700); err != nil {
		t.Fatal(err)
	}
	freshStamp := time.Now().Add(-30 * time.Minute)
	if err := os.Chtimes(freshCustom, freshStamp, freshStamp); err != nil {
		t.Fatal(err)
	}
	if err := reapNativeSessionRoots(customLifetime); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(oldCustom); !os.IsNotExist(err) {
		t.Fatal("custom-lifetime stale root was not recovered")
	}
	if _, err := os.Lstat(freshCustom); err != nil {
		t.Fatal("custom-lifetime fresh root was reaped")
	}
}

func nativeTreeContains(root string, needle []byte) (bool, error) {
	found := false
	err := filepath.WalkDir(
		root,
		func(pathValue string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || found {
				return nil
			}
			body, err := os.ReadFile(pathValue)
			if err != nil {
				return err
			}
			found = bytes.Contains(body, needle)
			clearBytes(body)
			return nil
		},
	)
	return found, err
}

// hostBinaryDriftReason reports whether the file at pathValue no longer
// matches pinnedDigest, and if so, a reason string naming both digests for
// t.Skip. It takes no *testing.T so the exact-fixture skip path and its own
// coverage test exercise identical logic: a digest mismatch is host drift,
// never a silent pass-through, and identical bytes cannot produce a
// mismatch here regardless of what a version subprocess might report.
func hostBinaryDriftReason(pathValue, pinnedDigest string) (string, bool) {
	hostDigest, err := executableDigest(pathValue)
	if err == nil && hostDigest == pinnedDigest {
		return "", false
	}
	return fmt.Sprintf(
		"host binary at %s has drifted from its pinned certification identity %s (host digest %s, err=%v)",
		pathValue, pinnedDigest, hostDigest, err,
	), true
}

// TestExactNativeConfigFixtureSkipsOnHostBinaryDrift proves the skip path
// exactNativeConfigFixture relies on is reachable without depending on the
// operator's specific host paths: a throwaway local script can never match a
// pinned CLI digest, so the drift oracle must report a mismatch.
func TestExactNativeConfigFixtureSkipsOnHostBinaryDrift(t *testing.T) {
	script := filepath.Join(t.TempDir(), "drifted-binary")
	if err := os.WriteFile(
		script, []byte("#!/bin/sh\necho drifted\n"), 0o755,
	); err != nil {
		t.Fatal(err)
	}
	if _, drifted := hostBinaryDriftReason(script, CodexCLIDigest); !drifted {
		t.Fatal("drift not detected for a throwaway script pinned against CodexCLIDigest")
	}
	if _, drifted := hostBinaryDriftReason(script, ClaudeCLIDigest); !drifted {
		t.Fatal("drift not detected for a throwaway script pinned against ClaudeCLIDigest")
	}
	matching, err := executableDigest(script)
	if err != nil {
		t.Fatal(err)
	}
	if _, drifted := hostBinaryDriftReason(script, matching); drifted {
		t.Fatal("drift falsely reported when the host digest matches the pin")
	}
}

func exactNativeConfigFixture(
	t *testing.T,
	family ProfileFamily,
) NativeAdapterConfig {
	t.Helper()
	var pathValue, digest, version, versionOutput, target, key, id string
	switch family {
	case ProfileCodex:
		pathValue, digest = exactCodexBinary, CodexCLIDigest
		version, versionOutput = CodexCLIVersion, "codex-cli "+CodexCLIVersion
		target, key, id = CodexCredentialTarget, "codex-adapter", "sworn.codex"
	case ProfileClaude:
		pathValue, digest = exactClaudeBinary, ClaudeCLIDigest
		version, versionOutput = ClaudeCLIVersion, ClaudeCLIVersion+" (Claude Code)"
		target, key, id = ClaudeCredentialTarget, "claude-adapter", "sworn.claude"
	default:
		t.Fatalf("unknown native family %s", family)
	}
	if _, err := os.Stat(pathValue); err != nil {
		t.Skipf("exact %s fixture unavailable: %v", family, err)
	}
	if reason, drifted := hostBinaryDriftReason(pathValue, digest); drifted {
		t.Skip(reason)
	}
	runtimeFiles := systemRuntimeFiles(t)
	if family == ProfileClaude {
		for _, runtimeTarget := range []string{
			"/lib64/ld-linux-x86-64.so.2",
			"/lib/x86_64-linux-gnu/librt.so.1",
			"/lib/x86_64-linux-gnu/libc.so.6",
			"/lib/x86_64-linux-gnu/libpthread.so.0",
			"/lib/x86_64-linux-gnu/libdl.so.2",
			"/lib/x86_64-linux-gnu/libm.so.6",
		} {
			runtimeFiles = append(
				runtimeFiles,
				pinnedRuntimeFile(t, runtimeTarget, runtimeTarget),
			)
		}
	}
	required := make([]string, len(runtimeFiles))
	for index := range runtimeFiles {
		required[index] = runtimeFiles[index].Target
	}
	return NativeAdapterConfig{
		Key: key, ID: id, Version: "1.0.0", Family: family,
		CLI:        ExecutableIdentity{Path: pathValue, Digest: digest},
		CLIVersion: version, VersionOutput: versionOutput,
		RuntimeFiles: runtimeFiles, RequiredRuntimeTargets: required,
		CredentialTarget:   target,
		CredentialRefs:     []string{string(family) + "-credential"},
		MaxCredentialBytes: 1_048_576,
	}
}

func nativeContinuationConfigFixture(
	t *testing.T,
	family ProfileFamily,
	binary string,
	digest string,
) NativeAdapterConfig {
	t.Helper()
	target := CodexCredentialTarget
	if family == ProfileClaude {
		target = ClaudeCredentialTarget
	}
	runtimeFiles := systemRuntimeFiles(t)
	required := make([]string, len(runtimeFiles))
	for index := range runtimeFiles {
		required[index] = runtimeFiles[index].Target
	}
	return NativeAdapterConfig{
		Key:                    "native-continuation-" + string(family),
		ID:                     "sworn.native-continuation-" + string(family),
		Version:                "1.0.0",
		Family:                 family,
		CLI:                    ExecutableIdentity{Path: binary, Digest: digest},
		CLIVersion:             "1.0.0",
		VersionOutput:          "1.0.0",
		RuntimeFiles:           runtimeFiles,
		RequiredRuntimeTargets: required,
		CredentialTarget:       target,
		CredentialRefs:         []string{"native-continuation-credential"},
		MaxCredentialBytes:     65_536,
	}
}

func nativeContinuationCertificateFixture(
	invocation Invocation,
	config NativeAdapterConfig,
) nativeSurfaceCertificate {
	return nativeSurfaceCertificateFixture(
		invocation,
		config,
		"native-continuation",
		"1.0.0",
		Digest([]byte("native-continuation-capture")),
	)
}

func systemRuntimeFiles(t *testing.T) []PinnedRuntimeFile {
	t.Helper()
	files := make([]PinnedRuntimeFile, 0, 4)
	for _, target := range []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/resolv.conf",
		"/etc/hosts",
		"/etc/nsswitch.conf",
	} {
		files = append(files, pinnedRuntimeFile(t, target, target))
	}
	return files
}

func pinnedRuntimeFile(
	t *testing.T,
	source string,
	target string,
) PinnedRuntimeFile {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Skipf("host runtime file %s unavailable: %v", source, err)
	}
	digest, err := executableDigest(resolved)
	if err != nil {
		t.Skipf("host runtime file %s unavailable: %v", source, err)
	}
	return PinnedRuntimeFile{Path: resolved, Target: target, Digest: digest}
}

func readOpenFile(t *testing.T, file *os.File) []byte {
	t.Helper()
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	body, err := ioReadAllBounded(file, 65_536)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	return body
}

func containsArgumentSequence(arguments, expected []string) bool {
	if len(expected) > len(arguments) {
		return false
	}
	for start := 0; start <= len(arguments)-len(expected); start++ {
		matches := true
		for index := range expected {
			matches = matches && arguments[start+index] == expected[index]
		}
		if matches {
			return true
		}
	}
	return false
}

func buildNativeProbe(t *testing.T) string {
	t.Helper()
	nativeProbeOnce.Do(func() {
		directory, err := os.MkdirTemp("", "sworn-native-probe-")
		if err != nil {
			nativeProbeError = err
			return
		}
		nativeProbeBinary = filepath.Join(directory, "native-probe")
		command := exec.Command(
			"go",
			"build",
			"-o",
			nativeProbeBinary,
			"./testdata/nativeprobe",
		)
		command.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
		output, err := command.CombinedOutput()
		if err != nil {
			nativeProbeError = &buildFailure{output: string(output)}
		}
	})
	if nativeProbeError != nil {
		t.Fatal(nativeProbeError)
	}
	return nativeProbeBinary
}

func buildNativeContinuation(t *testing.T) string {
	t.Helper()
	nativeContinuationOnce.Do(func() {
		directory, err := os.MkdirTemp("", "sworn-native-continuation-")
		if err != nil {
			nativeContinuationError = err
			return
		}
		nativeContinuationBinary = filepath.Join(
			directory,
			"native-continuation",
		)
		command := exec.Command(
			"go",
			"build",
			"-o",
			nativeContinuationBinary,
			"./testdata/nativecontinuation",
		)
		command.Env = append(
			os.Environ(),
			"GOFLAGS=-buildvcs=false",
			"CGO_ENABLED=0",
		)
		output, err := command.CombinedOutput()
		if err != nil {
			nativeContinuationError = &buildFailure{output: string(output)}
		}
	})
	if nativeContinuationError != nil {
		t.Fatal(nativeContinuationError)
	}
	return nativeContinuationBinary
}

// A1: the native CLI capture becomes family-aware - the cache pair the
// claude/codex wires carry (cache_read_input_tokens,
// cache_creation_input_tokens) reaches the state usage beside the core pair,
// and every result/turn.completed event counts as one turn for A5.
func TestNativeEventStateCapturesFullWireSplitAndTurns(t *testing.T) {
	t.Parallel()
	state := &nativeEventState{family: ProfileClaude}
	body := []byte(
		`{"type":"result","usage":{"input_tokens":10,"output_tokens":3,` +
			`"cache_read_input_tokens":4,"cache_creation_input_tokens":5}}`,
	)
	if err := state.accept(body); err != nil {
		t.Fatal(err)
	}
	if !state.hasUsage ||
		state.usage.InputTokens != 10 ||
		state.usage.OutputTokens != 3 ||
		state.usage.CacheReadTokens == nil ||
		*state.usage.CacheReadTokens != 4 ||
		state.usage.CacheWriteTokens == nil ||
		*state.usage.CacheWriteTokens != 5 ||
		state.usage.ReasoningTokens != nil ||
		state.turns != 1 {
		t.Fatalf("claude capture = %#v", state.usage)
	}

	codex := &nativeEventState{family: ProfileCodex}
	if err := codex.accept([]byte(
		`{"type":"turn.completed","usage":{"input_tokens":7,"output_tokens":2,` +
			`"cache_read_input_tokens":1,"cache_creation_input_tokens":9}}`,
	)); err != nil {
		t.Fatal(err)
	}
	if !codex.hasUsage ||
		codex.usage.InputTokens != 7 ||
		codex.usage.OutputTokens != 2 ||
		codex.usage.CacheReadTokens == nil ||
		*codex.usage.CacheReadTokens != 1 ||
		codex.usage.CacheWriteTokens == nil ||
		*codex.usage.CacheWriteTokens != 9 ||
		codex.turns != 1 {
		t.Fatalf("codex capture = %#v", codex.usage)
	}
}

// A2: scanNativeEvents' cumulative-total branch fails
// ECONOMY_OUTPUT_BUDGET_EXCEEDED and stamps the crossing byte total onto
// state, never touching state.accept - nothing in the pre-slice suite pins
// this cumulative behavior.
func TestScanNativeEventsCumulativeByteBudgetCrossing(t *testing.T) {
	t.Parallel()
	state := &nativeEventState{family: ProfileClaude, model: "m"}
	line := []byte(`{"type":"result","usage":{"input_tokens":1,"output_tokens":1}}`)
	reader := bytes.NewReader(append(append([]byte(nil), line...), '\n'))
	capability := []byte("capability-not-present")
	err := scanNativeEvents(reader, capability, state, int64(len(line))-1)
	if !IsCode(err, "ECONOMY_OUTPUT_BUDGET_EXCEEDED") {
		t.Fatalf("error = %v, want ECONOMY_OUTPUT_BUDGET_EXCEEDED", err)
	}
	state.mu.Lock()
	spent := state.streamBytes
	state.mu.Unlock()
	if spent != int64(len(line)) {
		t.Fatalf("streamBytes = %d, want %d", spent, len(line))
	}
	if state.turns != 0 {
		t.Fatalf("turns = %d, want 0: the crossing must trip before accept runs", state.turns)
	}
}

// A2: a stream whose cumulative total stays under the budget keeps running
// every line through state.accept exactly as before.
func TestScanNativeEventsUnderBudgetContinues(t *testing.T) {
	t.Parallel()
	state := &nativeEventState{family: ProfileClaude, model: "m"}
	line := []byte(`{"type":"result","usage":{"input_tokens":1,"output_tokens":1}}` + "\n")
	var buf bytes.Buffer
	for i := 0; i < 5; i++ {
		buf.Write(line)
	}
	total := int64(buf.Len())
	capability := []byte("capability-not-present")
	if err := scanNativeEvents(&buf, capability, state, total+1); err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if state.turns != 5 {
		t.Fatalf("turns = %d, want 5", state.turns)
	}
}

// A2: a line that both crosses the cumulative budget and carries the
// capability secret still classifies as surface, not economy - the secret
// check runs first.
func TestScanNativeEventsSecretTripOutranksByteBudget(t *testing.T) {
	t.Parallel()
	state := &nativeEventState{family: ProfileClaude, model: "m"}
	capability := []byte("capability-secret-token")
	line := []byte(`{"type":"result","note":"` + string(capability) + `"}`)
	reader := bytes.NewReader(append(append([]byte(nil), line...), '\n'))
	if err := scanNativeEvents(reader, capability, state, 1); !IsCode(err, "NATIVE_SURFACE_INVALID") {
		t.Fatalf("error = %v, want NATIVE_SURFACE_INVALID", err)
	}
}
