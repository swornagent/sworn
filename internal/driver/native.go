package driver

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

const (
	CodexCLIVersion        = "0.145.0"
	CodexCLIDigest         = "sha256:a2a05dafaa1acb002a45eaec0a462de5b13694fcfcd7bc43305f14781ce7be14"
	ClaudeCLIVersion       = "2.1.208"
	ClaudeCLIDigest        = "sha256:125372839bc827ca24dd72382627b291fbca615408d732fe3291bc16723ce7f3"
	CodexCredentialTarget  = "/home/sworn/.codex/auth.json"
	ClaudeCredentialTarget = "/home/sworn/.claude/.credentials.json"
)

type PinnedRuntimeFile struct {
	Path   string
	Target string
	Digest string
}

type NativeAdapterConfig struct {
	Key                    string
	ID                     string
	Version                string
	Family                 ProfileFamily
	CLI                    ExecutableIdentity
	CLIVersion             string
	VersionOutput          string
	RuntimeFiles           []PinnedRuntimeFile
	RequiredRuntimeTargets []string
	CredentialTarget       string
	CredentialRefs         []string
	MaxCredentialBytes     int64
}

type nativeAdapter struct {
	identity  AdapterIdentity
	config    NativeAdapterConfig
	resolve   FileCredentialResolver
	liveProbe ProfileLiveProbe
	refs      map[string]struct{}
	certMu    sync.RWMutex
	certified map[string]struct{}
}

func NewNativeAdapter(
	config NativeAdapterConfig,
	resolver FileCredentialResolver,
	probe ProfileLiveProbe,
) (Adapter, error) {
	if !providerKeyPattern.MatchString(config.Key) ||
		!driverIdentityPattern.MatchString(config.ID) ||
		!versionPattern.MatchString(config.Version) ||
		resolver == nil || validateNativeConfig(config) != nil {
		return nil, fail("INVALID_ADAPTER")
	}
	refs := make(map[string]struct{}, len(config.CredentialRefs))
	for _, ref := range config.CredentialRefs {
		if !providerKeyPattern.MatchString(ref) {
			return nil, fail("INVALID_CREDENTIAL_REFERENCE")
		}
		if _, duplicate := refs[ref]; duplicate {
			return nil, fail("INVALID_CREDENTIAL_REFERENCE")
		}
		refs[ref] = struct{}{}
	}
	if len(refs) == 0 {
		return nil, fail("INVALID_CREDENTIAL_REFERENCE")
	}
	sort.Strings(config.CredentialRefs)
	sort.Slice(config.RuntimeFiles, func(left, right int) bool {
		return config.RuntimeFiles[left].Target < config.RuntimeFiles[right].Target
	})
	sort.Strings(config.RequiredRuntimeTargets)
	body, err := canonicalJSON(config)
	if err != nil {
		return nil, err
	}
	return &nativeAdapter{
		identity: AdapterIdentity{
			Key: config.Key, ID: config.ID, Version: config.Version,
			ConfigurationDigest: Digest(body),
		},
		config: config, resolve: resolver, liveProbe: probe, refs: refs,
		certified: make(map[string]struct{}),
	}, nil
}

func validateNativeConfig(config NativeAdapterConfig) error {
	if validateExecutableIdentity(config.CLI) != nil ||
		config.MaxCredentialBytes < 1 || config.MaxCredentialBytes > 1_048_576 ||
		config.VersionOutput == "" || len(config.VersionOutput) > 256 {
		return fail("NATIVE_NOT_CERTIFIED")
	}
	switch config.Family {
	case ProfileCodex:
		if config.CLIVersion != CodexCLIVersion ||
			config.CLI.Digest != CodexCLIDigest ||
			config.CredentialTarget != CodexCredentialTarget ||
			config.VersionOutput != "codex-cli "+CodexCLIVersion {
			return fail("NATIVE_NOT_CERTIFIED")
		}
	case ProfileClaude:
		if config.CLIVersion != ClaudeCLIVersion ||
			config.CLI.Digest != ClaudeCLIDigest ||
			config.CredentialTarget != ClaudeCredentialTarget ||
			config.VersionOutput != ClaudeCLIVersion+" (Claude Code)" {
			return fail("NATIVE_NOT_CERTIFIED")
		}
	default:
		return fail("NATIVE_NOT_CERTIFIED")
	}
	if !filepath.IsAbs(config.CredentialTarget) ||
		filepath.Clean(config.CredentialTarget) != config.CredentialTarget ||
		validatePinnedRuntimeFiles(
			config.RuntimeFiles,
			config.RequiredRuntimeTargets,
			"NATIVE_NOT_CERTIFIED",
		) != nil {
		return fail("NATIVE_NOT_CERTIFIED")
	}
	for _, runtimeFile := range config.RuntimeFiles {
		for _, toolchainRoot := range []string{
			"/bin", "/sbin", "/usr/bin", "/usr/sbin", "/usr/local/bin",
		} {
			if pathBeneath(toolchainRoot, runtimeFile.Target) {
				return fail("NATIVE_NOT_CERTIFIED")
			}
		}
	}
	return nil
}

func validatePinnedRuntimeFiles(
	runtimeFiles []PinnedRuntimeFile,
	requiredTargets []string,
	code string,
) error {
	if len(runtimeFiles) == 0 || len(requiredTargets) == 0 {
		return fail(code)
	}
	targets := make(map[string]PinnedRuntimeFile, len(runtimeFiles))
	for _, runtimeFile := range runtimeFiles {
		if runtimeFile.Path == "" || !filepath.IsAbs(runtimeFile.Path) ||
			filepath.Clean(runtimeFile.Path) != runtimeFile.Path ||
			!filepath.IsAbs(runtimeFile.Target) ||
			filepath.Clean(runtimeFile.Target) != runtimeFile.Target ||
			!digestPattern.MatchString(runtimeFile.Digest) ||
			runtimeFile.Target == GuestWorkspacePath ||
			pathBeneath(GuestWorkspacePath, runtimeFile.Target) ||
			pathBeneath("/home/sworn", runtimeFile.Target) ||
			pathBeneath("/sworn", runtimeFile.Target) {
			return fail(code)
		}
		if _, duplicate := targets[runtimeFile.Target]; duplicate {
			return fail(code)
		}
		targets[runtimeFile.Target] = runtimeFile
	}
	requiredSeen := make(map[string]struct{}, len(requiredTargets))
	for _, required := range requiredTargets {
		if _, duplicate := requiredSeen[required]; duplicate {
			return fail(code)
		}
		requiredSeen[required] = struct{}{}
		if _, present := targets[required]; !present {
			return fail(code)
		}
	}
	for _, required := range []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/resolv.conf",
		"/etc/hosts",
		"/etc/nsswitch.conf",
	} {
		if _, present := targets[required]; !present {
			return fail(code)
		}
	}
	return nil
}

func (adapter *nativeAdapter) Identity() AdapterIdentity {
	if adapter == nil {
		return AdapterIdentity{}
	}
	return adapter.identity
}

func (adapter *nativeAdapter) profileFamily() ProfileFamily {
	if adapter == nil {
		return ""
	}
	return adapter.config.Family
}

func (adapter *nativeAdapter) checkProfile(
	ctx context.Context,
	kind profileCheckKind,
	profile ProfileConfig,
	model string,
) (ReadinessState, string) {
	if adapter == nil || profile.Adapter != adapter.identity.Key ||
		profile.Network != NetworkRequired || profile.CredentialRef == nil ||
		validateText(model, 500, false) != nil {
		return ReadinessFail, "profile_binding_invalid"
	}
	if _, admitted := adapter.refs[*profile.CredentialRef]; !admitted {
		return ReadinessNotCertified, "credential_reference_unknown"
	}
	opened, err := openNativeClosure(adapter.config)
	if err != nil {
		return ReadinessNotCertified, "native_closure_changed"
	}
	closeNativeFiles(opened)
	switch kind {
	case checkInspect:
		return ReadinessPass, "native_closure_exact"
	case checkDoctor:
		body, runErr := nativeVersion(ctx, adapter.config)
		exact := runErr == nil && string(body) == adapter.config.VersionOutput+"\n"
		clearBytes(body)
		if !exact {
			return ReadinessNotCertified, "native_version_changed"
		}
		return ReadinessPass, "native_binary_ready"
	case checkCertify:
		if adapter.liveProbe == nil {
			return ReadinessNotCertified, "live_probe_not_configured"
		}
		if err := adapter.liveProbe(ctx, *profile.CredentialRef, model); err != nil {
			return ReadinessFail, "live_probe_failed"
		}
		adapter.certMu.Lock()
		adapter.certified[*profile.CredentialRef+"\x00"+model] = struct{}{}
		adapter.certMu.Unlock()
		return ReadinessPass, "live_probe_passed"
	default:
		return ReadinessFail, "check_kind_invalid"
	}
}

func (adapter *nativeAdapter) invoke(
	ctx context.Context,
	invocation Invocation,
) (Observation, error) {
	if adapter == nil || invocation.Selected.Adapter != adapter.identity ||
		invocation.Selected.Profile.CredentialRef == nil {
		return Observation{}, fail("INVALID_ADAPTER")
	}
	ref := *invocation.Selected.Profile.CredentialRef
	if _, admitted := adapter.refs[ref]; !admitted {
		return Observation{}, fail("CREDENTIAL_NOT_CERTIFIED")
	}
	adapter.certMu.RLock()
	_, certified := adapter.certified[ref+"\x00"+invocation.Selected.Model]
	adapter.certMu.RUnlock()
	if !certified {
		return Observation{}, fail("NATIVE_NOT_CERTIFIED")
	}
	pathValue, err := adapter.resolve(ctx, ref)
	if err != nil {
		return Observation{}, fail("CREDENTIAL_NOT_CERTIFIED")
	}
	return platformInvokeNative(ctx, invocation, adapter.config, pathValue)
}

func openNativeClosure(config NativeAdapterConfig) ([]*os.File, error) {
	binary, err := openPinnedExecutable(config.CLI)
	if err != nil {
		return nil, fail("NATIVE_NOT_CERTIFIED")
	}
	files := []*os.File{binary}
	for _, runtimeFile := range config.RuntimeFiles {
		file, openErr := openPinnedRuntimeFile(runtimeFile)
		if openErr != nil {
			closeNativeFiles(files)
			return nil, openErr
		}
		files = append(files, file)
	}
	return files, nil
}

func openPinnedRuntimeFile(identity PinnedRuntimeFile) (*os.File, error) {
	info, err := os.Lstat(identity.Path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fail("NATIVE_NOT_CERTIFIED")
	}
	file, err := os.Open(identity.Path)
	if err != nil {
		return nil, fail("NATIVE_NOT_CERTIFIED")
	}
	digest, err := streamDigest(file)
	if err != nil || digest != identity.Digest {
		_ = file.Close()
		return nil, fail("NATIVE_NOT_CERTIFIED")
	}
	if _, err := file.Seek(0, 0); err != nil {
		_ = file.Close()
		return nil, fail("NATIVE_NOT_CERTIFIED")
	}
	return file, nil
}

func closeNativeFiles(files []*os.File) {
	for _, file := range files {
		if file != nil {
			_ = file.Close()
		}
	}
}
