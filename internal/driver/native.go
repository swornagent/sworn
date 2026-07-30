package driver

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

const (
	CodexCLIVersion        = "0.146.0"
	CodexCLIDigest         = "sha256:2e863156ed35ecc5253b1e2f907a9143077b9f7cb51942070c61996471ff6e04"
	ClaudeCLIVersion       = "2.1.208"
	ClaudeCLIDigest        = "sha256:125372839bc827ca24dd72382627b291fbca615408d732fe3291bc16723ce7f3"
	CodexCredentialTarget  = "/home/sworn/.codex/auth.json"
	ClaudeCredentialTarget = "/home/sworn/.claude/.credentials.json"
)

type PinnedRuntimeFile struct {
	Path   string `json:"path"`
	Target string `json:"target"`
	Digest string `json:"digest"`
}

type NativeAdapterConfig struct {
	Key                    string              `json:"key"`
	ID                     string              `json:"id"`
	Version                string              `json:"version"`
	Family                 ProfileFamily       `json:"family"`
	CLI                    ExecutableIdentity  `json:"cli"`
	CLIVersion             string              `json:"cli_version"`
	VersionOutput          string              `json:"version_output"`
	RuntimeFiles           []PinnedRuntimeFile `json:"runtime_files"`
	RequiredRuntimeTargets []string            `json:"required_runtime_targets"`
	CredentialTarget       string              `json:"credential_target"`
	CredentialRefs         []string            `json:"credential_refs"`
	MaxCredentialBytes     int64               `json:"max_credential_bytes"`
}

// NativeSmokeInvocations supplies the separately authorized design and
// implementation invocations used by deterministic native certification.
type NativeSmokeInvocations struct {
	Design         Invocation
	Implementation Invocation
}

// NativeSmokeBuilder supplies only the already-authorized invocations used by
// native certification. The adapter owns the executable, loopback provider,
// broker configuration, launch, capture, and certification result.
type NativeSmokeBuilder func(
	context.Context,
	SelectedProfile,
) (NativeSmokeInvocations, error)

type nativeSurfaceStageCertificate struct {
	Access                WorkspaceAccess
	Resume                bool
	ToolDigest            string
	CaptureEvidenceDigest string
	ArgumentDigest        string
	Protocol              string
	ClientName            string
	ClientVersion         string
	InitializeDigest      string
	NotificationDigest    string
	ListDigest            string
}

type nativeSurfaceCertificate struct {
	Family              ProfileFamily
	ProfileDigest       string
	Model               string
	AdapterConfigDigest string
	ExecutableDigest    string
	CLIVersion          string
	Design              nativeSurfaceStageCertificate
	Resume              nativeSurfaceStageCertificate
}

type nativeAdapter struct {
	identity     AdapterIdentity
	config       NativeAdapterConfig
	resolve      FileCredentialResolver
	smokeBuilder NativeSmokeBuilder
	refs         map[string]struct{}
	certMu       sync.RWMutex
	certified    map[string]nativeSurfaceCertificate
}

func NewNativeAdapter(
	config NativeAdapterConfig,
	resolver FileCredentialResolver,
	builder NativeSmokeBuilder,
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
		config: config, resolve: resolver, smokeBuilder: builder, refs: refs,
		certified: make(map[string]nativeSurfaceCertificate),
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
		if adapter.smokeBuilder == nil {
			return ReadinessNotCertified, "native_smoke_not_configured"
		}
		selected := SelectedProfile{
			Profile: cloneProfileConfig(profile),
			Adapter: adapter.identity,
			Model:   model,
			adapter: adapter,
		}
		invocations, err := adapter.smokeBuilder(ctx, selected)
		if err != nil || validateNativeSmokeInvocations(
			invocations,
			selected,
			adapter,
		) != nil {
			return ReadinessFail, "native_smoke_invalid"
		}
		certificate, err := platformCaptureNativeSurface(
			ctx,
			invocations,
			adapter.config,
		)
		if err != nil {
			return ReadinessFail, "native_surface_failed"
		}
		adapter.certMu.Lock()
		adapter.certified[nativeCertificationKey(profile, model)] = certificate
		adapter.certMu.Unlock()
		return ReadinessPass, "live_smoke_passed"
	default:
		return ReadinessFail, "check_kind_invalid"
	}
}

func (adapter *nativeAdapter) invoke(
	ctx context.Context,
	invocation Invocation,
) (Observation, error) {
	certificate, pathValue, err := adapter.nativeRuntime(ctx, invocation)
	if err != nil {
		return Observation{}, err
	}
	return platformInvokeNative(
		ctx,
		invocation,
		adapter.config,
		pathValue,
		certificate,
	)
}

func (adapter *nativeAdapter) invokeContinuation(
	ctx context.Context,
	invocation Invocation,
) (Observation, continuationState, error) {
	if validateContinuationSource(invocation) != nil {
		return Observation{}, nil, fail("CONTINUATION_INVALID")
	}
	certificate, pathValue, err := adapter.nativeRuntime(ctx, invocation)
	if err != nil {
		return Observation{}, nil, err
	}
	return platformStartNativeContinuation(
		ctx,
		invocation,
		adapter.config,
		pathValue,
		certificate,
	)
}

func (adapter *nativeAdapter) resumeContinuation(
	ctx context.Context,
	invocation Invocation,
	state continuationState,
) (Observation, error) {
	if validateContinuationResume(invocation) != nil {
		return Observation{}, fail("CONTINUATION_INVALID")
	}
	certificate, pathValue, err := adapter.nativeRuntime(ctx, invocation)
	if err != nil {
		return Observation{}, err
	}
	return platformResumeNativeContinuation(
		ctx,
		invocation,
		adapter.config,
		pathValue,
		certificate,
		state,
	)
}

func (adapter *nativeAdapter) nativeRuntime(
	ctx context.Context,
	invocation Invocation,
) (nativeSurfaceCertificate, string, error) {
	if adapter == nil || invocation.Selected.Adapter != adapter.identity ||
		invocation.Selected.Profile.CredentialRef == nil {
		return nativeSurfaceCertificate{}, "", fail("INVALID_ADAPTER")
	}
	ref := *invocation.Selected.Profile.CredentialRef
	if _, admitted := adapter.refs[ref]; !admitted {
		return nativeSurfaceCertificate{}, "", fail("CREDENTIAL_NOT_CERTIFIED")
	}
	adapter.certMu.RLock()
	certificate, certified := adapter.certified[nativeCertificationKey(
		invocation.Selected.Profile,
		invocation.Selected.Model,
	)]
	adapter.certMu.RUnlock()
	if !certified {
		return nativeSurfaceCertificate{}, "", fail("NATIVE_NOT_CERTIFIED")
	}
	pathValue, err := adapter.resolve(ctx, ref)
	if err != nil {
		return nativeSurfaceCertificate{}, "", fail("CREDENTIAL_NOT_CERTIFIED")
	}
	return certificate, pathValue, nil
}

func validateNativeSmokeInvocations(
	invocations NativeSmokeInvocations,
	selected SelectedProfile,
	adapter *nativeAdapter,
) error {
	design := invocations.Design
	implementation := invocations.Implementation
	if adapter == nil || design.Selected.adapter != adapter ||
		implementation.Selected.adapter != adapter ||
		design.Selected.Adapter != selected.Adapter ||
		implementation.Selected.Adapter != selected.Adapter ||
		design.Selected.Model != selected.Model ||
		implementation.Selected.Model != selected.Model ||
		design.Selected.Profile.Key != selected.Profile.Key ||
		implementation.Selected.Profile.Key != selected.Profile.Key ||
		design.Selected.Profile.Adapter != selected.Profile.Adapter ||
		implementation.Selected.Profile.Adapter != selected.Profile.Adapter ||
		design.Selected.Profile.Network != selected.Profile.Network ||
		implementation.Selected.Profile.Network != selected.Profile.Network ||
		!sameOptionalString(
			design.Selected.Profile.CredentialRef,
			selected.Profile.CredentialRef,
		) ||
		!sameOptionalString(
			implementation.Selected.Profile.CredentialRef,
			selected.Profile.CredentialRef,
		) ||
		design.HostWorkspace != implementation.HostWorkspace ||
		design.Request.InvocationID == implementation.Request.InvocationID ||
		validateContinuationSource(design) != nil ||
		validateContinuationResume(implementation) != nil {
		return fail("NATIVE_NOT_CERTIFIED")
	}
	return nil
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func nativeCertificationKey(profile ProfileConfig, model string) string {
	body, err := canonicalJSON(profile)
	if err != nil {
		return ""
	}
	return Digest(body) + "\x00" + model
}

func nativeToolSurfaceDigest(access WorkspaceAccess) string {
	definitions := toolDefinitions(access)
	body, err := canonicalJSON(definitions)
	if err != nil {
		return ""
	}
	return Digest(body)
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
