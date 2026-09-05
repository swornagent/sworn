package driver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	CodexCLIVersion        = "0.146.0"
	CodexCLIDigest         = "sha256:2e863156ed35ecc5253b1e2f907a9143077b9f7cb51942070c61996471ff6e04"
	ClaudeCLIVersion       = "2.1.241"
	ClaudeCLIDigest        = "sha256:0771bd866cff82b76581fc0499f6529e1a36845078f144f8c81dccb3bc7037b8"
	CodexCredentialTarget  = "/home/sworn/.codex/auth.json"
	ClaudeCredentialTarget = "/home/sworn/.claude/.credentials.json"
)

// Pin admission modes. Absent (empty string) and "exact" are synonyms for
// today's byte-for-byte closure; "minor" admits a CLI whose self-reported
// version shares the pinned major.minor with any patch.
const (
	NativePinModeExact = "exact"
	NativePinModeMinor = "minor"
)

type PinnedRuntimeFile struct {
	Path   string `json:"path"`
	Target string `json:"target"`
	Digest string `json:"digest"`
}

// nativeAuthExitCodes is the per-family native auth-exit vocabulary: a
// spontaneous clean exit with one of these codes at the terminal
// classification site is positively an auth-class failure and surfaces as
// PROVIDER_AUTHORIZATION_FAILED instead of masquerading as
// PROVIDER_TRANSPORT_FAILED. It is a package variable, not a constant, only
// so non-parallel fixture tests can pin the classification branch for their
// own duration and restore it afterwards - the same link-time-gate pattern as
// testUncontainedDispatch. Production ships only entries probed against the
// pinned CLIs on the operator host; families absent from the map stay
// fail-open (their clean exits classify as transport). Exit code 1 is barred
// by the Captain ruling: bubblewrap uses 1 for its own setup failures, and
// mislabeling a sandbox fault as an auth failure is the exact confusion this
// vocabulary exists to end.
var nativeAuthExitCodes = map[ProfileFamily]int{}

// nativeAuthExitCode reports whether family has a pinned auth exit code.
func nativeAuthExitCode(family ProfileFamily) (int, bool) {
	code, ok := nativeAuthExitCodes[family]
	return code, ok
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
	// PinMode is admission policy, additive and omitempty so every existing
	// document without it keeps today's canonical bytes and
	// ConfigurationDigest exactly. Absent or "exact" preserves the four
	// byte-for-byte comparisons below; "minor" admits a CLI whose
	// self-reported version shares the pinned major.minor. The credential
	// target stays an exact comparison in both modes.
	PinMode string `json:"pin_mode,omitempty"`
}

// NativeSmokeInvocations supplies the separately authorized invocations used
// by native certification. Fresh read-only and read-write launches are
// independent from the persistent continuation start and explicit resume
// launches, even when they carry the same tool surface.
type NativeSmokeInvocations struct {
	FreshReadOnly     Invocation
	FreshReadWrite    Invocation
	ContinuationStart Invocation
	Resume            Invocation
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
	InvocationStage       nativeInvocationStage
	ToolDigest            string
	CaptureEvidenceDigest string
	ArgumentDigest        string
	AuthorityDigest       string
	Protocol              string
	ClientName            string
	ClientVersion         string
	InitializeDigest      string
	NotificationDigest    string
	ListDigest            string
}

type nativeInvocationStage uint8

const (
	nativeInvocationStageFresh nativeInvocationStage = iota + 1
	nativeInvocationStageContinuationStart
	nativeInvocationStageResume
	nativeInvocationStageRecovery
	nativeInvocationStageAdvisory
)

type nativeSurfaceCertificate struct {
	Family              ProfileFamily
	ProfileDigest       string
	Model               string
	AdapterConfigDigest string
	ExecutableDigest    string
	CLIVersion          string
	FreshReadOnly       nativeSurfaceStageCertificate
	FreshReadWrite      nativeSurfaceStageCertificate
	ContinuationStart   nativeSurfaceStageCertificate
	ContinuationStartRW nativeSurfaceStageCertificate
	ResumeReadOnly      nativeSurfaceStageCertificate
	Resume              nativeSurfaceStageCertificate
}

// nativeAutomationSurfaceCertificate is deliberately disjoint from the
// Baton invocation certificate. An automation launch is admitted only after
// both of its one-tool surfaces have been observed from the pinned CLI.
type nativeAutomationSurfaceCertificate struct {
	Family              ProfileFamily
	ProfileDigest       string
	Model               string
	AdapterConfigDigest string
	ExecutableDigest    string
	CLIVersion          string
	Recovery            nativeSurfaceStageCertificate
	Advisory            nativeSurfaceStageCertificate
}

func hasNativeSurfaceCertificate(certificate nativeSurfaceCertificate) bool {
	return certificate.Family != ""
}

func hasNativeAutomationSurfaceCertificate(
	certificate nativeAutomationSurfaceCertificate,
) bool {
	return certificate.Family != ""
}

type nativeAutomationSmokeInvocations struct {
	Recovery AutomationInvocation
	Advisory AutomationInvocation
}

type nativeAdapter struct {
	identity AdapterIdentity
	config   NativeAdapterConfig
	resolve  FileCredentialResolver
	refs     map[string]struct{}
}

func NewNativeAdapter(
	config NativeAdapterConfig,
	resolver FileCredentialResolver,
) (Adapter, error) {
	if !providerKeyPattern.MatchString(config.Key) {
		return nil, failWithDetail("INVALID_ADAPTER", "adapter_key")
	}
	if !driverIdentityPattern.MatchString(config.ID) {
		return nil, failWithDetail("INVALID_ADAPTER", "adapter_id")
	}
	if !versionPattern.MatchString(config.Version) {
		return nil, failWithDetail("INVALID_ADAPTER", "adapter_version")
	}
	if resolver == nil {
		return nil, failWithDetail("INVALID_ADAPTER", "credential_resolver")
	}
	if err := validateNativeConfig(config); err != nil {
		var contractErr *ContractError
		if errors.As(err, &contractErr) && contractErr.Detail != "" {
			return nil, failWithDetail("INVALID_ADAPTER", contractErr.Detail)
		}
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
		config: config, resolve: resolver, refs: refs,
	}, nil
}

func validateNativeConfig(config NativeAdapterConfig) error {
	if validateExecutableIdentity(config.CLI) != nil {
		return failWithDetail("NATIVE_NOT_CERTIFIED", "cli_identity")
	}
	if config.MaxCredentialBytes < 1 || config.MaxCredentialBytes > 1_048_576 ||
		config.VersionOutput == "" || len(config.VersionOutput) > 256 {
		return failWithDetail("NATIVE_NOT_CERTIFIED", "cli_admission_bounds")
	}
	var minor bool
	switch config.PinMode {
	case "", NativePinModeExact:
		minor = false
	case NativePinModeMinor:
		minor = true
	default:
		return failWithDetail("NATIVE_NOT_CERTIFIED", "pin_mode")
	}
	switch config.Family {
	case ProfileCodex:
		if config.CredentialTarget != CodexCredentialTarget {
			return failWithDetail("NATIVE_NOT_CERTIFIED", "credential_target")
		}
		if minor {
			if !nativeVersionSatisfiesMinor(config.CLIVersion, CodexCLIVersion) {
				return failWithDetail("NATIVE_NOT_CERTIFIED", "version")
			}
			if config.VersionOutput != "codex-cli "+config.CLIVersion {
				return failWithDetail("NATIVE_NOT_CERTIFIED", "version_output")
			}
		} else {
			if config.CLIVersion != CodexCLIVersion {
				return failWithDetail("NATIVE_NOT_CERTIFIED", "version")
			}
			if config.CLI.Digest != CodexCLIDigest {
				return failWithDetail("NATIVE_NOT_CERTIFIED", "digest")
			}
			if config.VersionOutput != "codex-cli "+CodexCLIVersion {
				return failWithDetail("NATIVE_NOT_CERTIFIED", "version_output")
			}
		}
	case ProfileClaude:
		if config.CredentialTarget != ClaudeCredentialTarget {
			return failWithDetail("NATIVE_NOT_CERTIFIED", "credential_target")
		}
		if minor {
			if !nativeVersionSatisfiesMinor(config.CLIVersion, ClaudeCLIVersion) {
				return failWithDetail("NATIVE_NOT_CERTIFIED", "version")
			}
			if config.VersionOutput != config.CLIVersion+" (Claude Code)" {
				return failWithDetail("NATIVE_NOT_CERTIFIED", "version_output")
			}
		} else {
			if config.CLIVersion != ClaudeCLIVersion {
				return failWithDetail("NATIVE_NOT_CERTIFIED", "version")
			}
			if config.CLI.Digest != ClaudeCLIDigest {
				return failWithDetail("NATIVE_NOT_CERTIFIED", "digest")
			}
			if config.VersionOutput != ClaudeCLIVersion+" (Claude Code)" {
				return failWithDetail("NATIVE_NOT_CERTIFIED", "version_output")
			}
		}
	default:
		return failWithDetail("NATIVE_NOT_CERTIFIED", "family")
	}
	if !filepath.IsAbs(config.CredentialTarget) ||
		filepath.Clean(config.CredentialTarget) != config.CredentialTarget {
		return failWithDetail("NATIVE_NOT_CERTIFIED", "credential_target")
	}
	if err := validatePinnedRuntimeFiles(
		config.RuntimeFiles,
		config.RequiredRuntimeTargets,
		"NATIVE_NOT_CERTIFIED",
	); err != nil {
		return err
	}
	for _, runtimeFile := range config.RuntimeFiles {
		for _, toolchainRoot := range []string{
			"/bin", "/sbin", "/usr/bin", "/usr/sbin", "/usr/local/bin",
		} {
			if pathBeneath(toolchainRoot, runtimeFile.Target) {
				return failWithDetail("NATIVE_NOT_CERTIFIED", "toolchain_root")
			}
		}
	}
	return nil
}

// nativeVersionSatisfiesMinor reports whether declared shares its major and
// minor version components with pinned, admitting any patch. Both strings
// must already match versionPattern (major.minor.patch, optional prerelease
// suffix on the patch component only), so splitting on "." is safe.
func nativeVersionSatisfiesMinor(declared, pinned string) bool {
	if !versionPattern.MatchString(declared) || !versionPattern.MatchString(pinned) {
		return false
	}
	declaredMajor, declaredMinor, ok := nativeMajorMinor(declared)
	if !ok {
		return false
	}
	pinnedMajor, pinnedMinor, ok := nativeMajorMinor(pinned)
	return ok && declaredMajor == pinnedMajor && declaredMinor == pinnedMinor
}

func nativeMajorMinor(version string) (int, int, bool) {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil || major < 0 || minor < 0 {
		return 0, 0, false
	}
	return major, minor, true
}

func validatePinnedRuntimeFiles(
	runtimeFiles []PinnedRuntimeFile,
	requiredTargets []string,
	code string,
) error {
	if len(runtimeFiles) == 0 || len(requiredTargets) == 0 {
		return failWithDetail(code, "runtime_file")
	}
	targets := make(map[string]PinnedRuntimeFile, len(runtimeFiles))
	for _, runtimeFile := range runtimeFiles {
		if runtimeFile.Path == "" || !filepath.IsAbs(runtimeFile.Path) ||
			filepath.Clean(runtimeFile.Path) != runtimeFile.Path ||
			!filepath.IsAbs(runtimeFile.Target) ||
			filepath.Clean(runtimeFile.Target) != runtimeFile.Target ||
			runtimeFile.Target == GuestWorkspacePath ||
			pathBeneath(GuestWorkspacePath, runtimeFile.Target) ||
			pathBeneath("/home/sworn", runtimeFile.Target) ||
			pathBeneath("/sworn", runtimeFile.Target) {
			return failWithDetail(code, "runtime_file_shape")
		}
		if !digestPattern.MatchString(runtimeFile.Digest) {
			return failWithDetail(code, "runtime_file_digest")
		}
		if _, duplicate := targets[runtimeFile.Target]; duplicate {
			return failWithDetail(code, "runtime_file_duplicate")
		}
		targets[runtimeFile.Target] = runtimeFile
	}
	requiredSeen := make(map[string]struct{}, len(requiredTargets))
	for _, required := range requiredTargets {
		if _, duplicate := requiredSeen[required]; duplicate {
			return failWithDetail(code, "runtime_file_duplicate")
		}
		requiredSeen[required] = struct{}{}
		if _, present := targets[required]; !present {
			return failWithDetail(code, "runtime_file_missing")
		}
	}
	for _, required := range []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/resolv.conf",
		"/etc/hosts",
		"/etc/nsswitch.conf",
	} {
		if _, present := targets[required]; !present {
			return failWithDetail(code, "trust_anchor")
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
	case checkDoctor, checkCertify:
		body, runErr := nativeVersion(ctx, adapter.config)
		exact := runErr == nil && string(body) == adapter.config.VersionOutput+"\n"
		clearBytes(body)
		if !exact {
			return ReadinessNotCertified, "native_version_changed"
		}
		if kind == checkCertify {
			// A2: certify must not pass what it never evaluated. It runs
			// the same bounded, read-only credential liveness check the
			// dispatch-time gate uses and reports exactly what it did: a
			// positively stale credential fails certification; a credential
			// the check actually read and did not find expired passes on
			// that evaluation alone; and a reference it could not resolve
			// or read is reported as unevaluated, never as a silent pass.
			// Reporting "unevaluated" for a credential that was in fact
			// evaluated would be the same false claim this check replaced,
			// so the evaluated signal the liveness check already returns
			// decides between the last two.
			pathValue, resolveErr := adapter.resolve(ctx, *profile.CredentialRef)
			if resolveErr == nil {
				stale, evaluated := nativeCredentialLivenessCheck(
					adapter.config.Family, pathValue, adapter.config.MaxCredentialBytes,
				)
				if stale {
					return ReadinessFail, "CREDENTIAL_STALE"
				}
				if evaluated {
					return ReadinessPass, "native_credential_preflight_passed"
				}
			}
			return ReadinessNotCertified, "native_credential_preflight_unevaluated"
		}
		return ReadinessPass, "native_binary_ready"
	default:
		return ReadinessFail, "check_kind_invalid"
	}
}

func (adapter *nativeAdapter) invokeAutomation(
	ctx context.Context,
	invocation AutomationInvocation,
) (AutomationObservation, error) {
	certificate, credentialPath, err := adapter.nativeAutomationRuntime(
		ctx,
		invocation,
	)
	if err != nil {
		return AutomationObservation{}, err
	}
	return platformInvokeNativeAutomation(
		ctx,
		invocation,
		adapter.config,
		credentialPath,
		certificate,
	)
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
		return Observation{}, nil, failContinuation("continuation.native.start_source_invalid")
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

func (adapter *nativeAdapter) invokeRecoverableContinuation(
	ctx context.Context,
	invocation Invocation,
) (Observation, continuationState, error) {
	if validateInvocation(invocation) != nil {
		return Observation{}, nil, failContinuation("continuation.native.start_recoverable_invocation_invalid")
	}
	certificate, pathValue, err := adapter.nativeRuntime(ctx, invocation)
	if err != nil {
		return Observation{}, nil, err
	}
	return platformStartNativeRecoverableContinuation(
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
		return Observation{}, failContinuation("continuation.native.resume_invocation_invalid")
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

func (adapter *nativeAdapter) resumeRecoverableContinuation(
	ctx context.Context,
	invocation Invocation,
	state continuationState,
	retainDesignTerminal bool,
) (Observation, continuationState, error) {
	if validateInvocation(invocation) != nil {
		return Observation{}, nil, failContinuation("continuation.native.resume_recoverable_invocation_invalid")
	}
	certificate, pathValue, err := adapter.nativeRuntime(ctx, invocation)
	if err != nil {
		return Observation{}, nil, err
	}
	return platformResumeNativeRecoverableContinuation(
		ctx,
		invocation,
		adapter.config,
		pathValue,
		certificate,
		state,
		retainDesignTerminal,
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
	pathValue, err := adapter.resolve(ctx, ref)
	if err != nil {
		return nativeSurfaceCertificate{}, "", fail("CREDENTIAL_NOT_CERTIFIED")
	}
	if err := nativeCredentialPreflight(
		adapter.config.Family,
		pathValue,
		adapter.config.MaxCredentialBytes,
	); err != nil {
		return nativeSurfaceCertificate{}, "", err
	}
	return nativeSurfaceCertificate{}, pathValue, nil
}

func (adapter *nativeAdapter) nativeAutomationRuntime(
	ctx context.Context,
	invocation AutomationInvocation,
) (nativeAutomationSurfaceCertificate, string, error) {
	if adapter == nil ||
		invocation.Selected.Adapter != adapter.identity ||
		invocation.Selected.Profile.CredentialRef == nil {
		return nativeAutomationSurfaceCertificate{}, "", fail("INVALID_ADAPTER")
	}
	ref := *invocation.Selected.Profile.CredentialRef
	if _, admitted := adapter.refs[ref]; !admitted {
		return nativeAutomationSurfaceCertificate{}, "",
			fail("CREDENTIAL_NOT_CERTIFIED")
	}
	pathValue, err := adapter.resolve(ctx, ref)
	if err != nil {
		return nativeAutomationSurfaceCertificate{}, "",
			fail("CREDENTIAL_NOT_CERTIFIED")
	}
	if err := nativeCredentialPreflight(
		adapter.config.Family,
		pathValue,
		adapter.config.MaxCredentialBytes,
	); err != nil {
		return nativeAutomationSurfaceCertificate{}, "", err
	}
	return nativeAutomationSurfaceCertificate{}, pathValue, nil
}

func nativeAutomationCertificationInvocations(
	selected SelectedProfile,
) (nativeAutomationSmokeInvocations, error) {
	selection := ModelSelection{
		Profile: selected.Profile.Key,
		Model:   selected.Model,
	}
	binding := AutomationBinding{
		RunID:                 "native-certification-run",
		TrackID:               "native-certification-track",
		Slice:                 "native-certification-slice",
		BatonAttempt:          1,
		PlanAuthorityDigest:   Digest([]byte("native-certification-plan")),
		TargetAuthorityDigest: Digest([]byte("native-certification-target")),
		WorkIdentity:          Digest([]byte("native-certification-work")),
		ProgressIdentity:      Digest([]byte("native-certification-progress")),
	}
	recovery := RecoveryInvocation{
		SchemaVersion: RecoveryInvocationSchemaVersion,
		InvocationID:  "native-recovery-certification",
		Binding:       binding,
		Selection:     selection,
		Facts: []AutomationFact{{
			Name:  FactWorkerTerminal,
			Value: "question",
		}},
	}
	advisory := AdvisoryInvocation{
		SchemaVersion: AdvisoryInvocationSchemaVersion,
		InvocationID:  "native-advisory-certification",
		Binding:       binding,
		Selection:     selection,
		Question:      "Can the admitted facts answer this bounded question?",
		Facts: []AutomationFact{{
			Name:  FactCurrentStatus,
			Value: "certification",
		}},
	}
	pair := nativeAutomationSmokeInvocations{
		Recovery: AutomationInvocation{
			Selected: selected,
			Recovery: &recovery,
		},
		Advisory: AutomationInvocation{
			Selected: selected,
			Advisory: &advisory,
		},
	}
	if validateAutomationInvocation(pair.Recovery) != nil ||
		validateAutomationInvocation(pair.Advisory) != nil {
		return nativeAutomationSmokeInvocations{},
			fail("NATIVE_NOT_CERTIFIED")
	}
	return pair, nil
}

func validateNativeSmokeInvocations(
	invocations NativeSmokeInvocations,
	selected SelectedProfile,
	adapter *nativeAdapter,
) error {
	values := []Invocation{
		invocations.FreshReadOnly,
		invocations.FreshReadWrite,
		invocations.ContinuationStart,
		invocations.Resume,
	}
	if adapter == nil {
		return fail("NATIVE_NOT_CERTIFIED")
	}
	identities := make(map[string]struct{}, len(values))
	for _, invocation := range values {
		if invocation.Selected.adapter != adapter ||
			invocation.Selected.Adapter != selected.Adapter ||
			invocation.Selected.Model != selected.Model ||
			invocation.Selected.Profile.Key != selected.Profile.Key ||
			invocation.Selected.Profile.Adapter != selected.Profile.Adapter ||
			invocation.Selected.Profile.Network != selected.Profile.Network ||
			!sameOptionalString(
				invocation.Selected.Profile.CredentialRef,
				selected.Profile.CredentialRef,
			) ||
			invocation.HostWorkspace !=
				invocations.ContinuationStart.HostWorkspace ||
			invocation.Request.InvocationID == "" {
			return fail("NATIVE_NOT_CERTIFIED")
		}
		if _, duplicate := identities[invocation.Request.InvocationID]; duplicate {
			return fail("NATIVE_NOT_CERTIFIED")
		}
		identities[invocation.Request.InvocationID] = struct{}{}
	}
	if !nativeCertificationInvocationMatches(
		invocations.FreshReadOnly,
		ImplementerDesign,
		ReadOnly,
		true,
	) ||
		!nativeCertificationInvocationMatches(
			invocations.FreshReadWrite,
			ImplementerDesign,
			ReadWrite,
			true,
		) ||
		validateContinuationSource(invocations.ContinuationStart) != nil ||
		validateContinuationResume(invocations.Resume) != nil {
		return fail("NATIVE_NOT_CERTIFIED")
	}
	return nil
}

func nativeCertificationInvocationMatches(
	invocation Invocation,
	responsibility Responsibility,
	access WorkspaceAccess,
	fresh bool,
) bool {
	if validateInvocation(invocation) != nil {
		return false
	}
	descriptor, err := invocation.Permission.Describe()
	return err == nil &&
		invocation.Request.Role == RoleImplementer &&
		invocation.Request.Workspace.Access == access &&
		invocation.Request.FreshContext == fresh &&
		descriptor.Role == RoleImplementer &&
		descriptor.Responsibility == responsibility &&
		descriptor.WorkspaceAccess == access &&
		descriptor.FreshContext == fresh
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
	return nativeToolDefinitionsDigest(toolDefinitions(access))
}

func nativeToolDefinitionsDigest(definitions []providerToolDefinition) string {
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
