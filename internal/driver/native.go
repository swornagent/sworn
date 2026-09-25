package driver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

// The CLI versions and digests Sworn has been tested with. They are a
// compatibility statement, not an admission gate: a configured CLI of any
// well-formed version is admitted and then bound by its own configured digest
// and version output at every launch. A version other than the tested one is
// reported as untested: it may well work, but compatibility and stability are
// not guaranteed.
const (
	CodexCLIVersion        = "0.146.0"
	CodexCLIDigest         = "sha256:2e863156ed35ecc5253b1e2f907a9143077b9f7cb51942070c61996471ff6e04"
	ClaudeCLIVersion       = "2.1.241"
	ClaudeCLIDigest        = "sha256:0771bd866cff82b76581fc0499f6529e1a36845078f144f8c81dccb3bc7037b8"
	CodexCredentialTarget  = "/home/sworn/.codex/auth.json"
	ClaudeCredentialTarget = "/home/sworn/.claude/.credentials.json"
)

// Pin admission modes, retained so existing configurations keep decoding
// with unchanged canonical bytes. All three values ("", "exact" and "minor")
// now admit identically: the configured digest and version output are the
// pin, and the tested version above is reported, never enforced.
const (
	NativePinModeExact = "exact"
	NativePinModeMinor = "minor"
)

// NativeCLIResolutionRunSnapshot opts a native adapter in to a run-start
// snapshot of its host CLI instead of a hand-maintained pinned copy. The
// adapter then names the host binary by path alone; each run copies the bytes
// once into a content-addressed store, records the copy as a run fact, and
// every dispatch in that run executes the copy.
const NativeCLIResolutionRunSnapshot = "run_snapshot"

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
// by the Lead ruling: bubblewrap uses 1 for its own setup failures, and
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
	CLIVersion             string              `json:"cli_version,omitempty"`
	VersionOutput          string              `json:"version_output,omitempty"`
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
	// CLIResolution is additive and omitempty like PinMode. Absent, the CLI
	// fields above pin one exact binary. NativeCLIResolutionRunSnapshot
	// instead names the host binary by cli.path alone and leaves cli.digest,
	// cli_version and version_output absent: each run binds them from its
	// own snapshot fact (see NativeCLISnapshot).
	CLIResolution string `json:"cli_resolution,omitempty"`
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
// Protocol invocation certificate. An automation launch is admitted only after
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
	if err := admitNativeConfig(config); err != nil {
		return nil, err
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

// admitNativeConfig reports a validation refusal as INVALID_ADAPTER, keeping
// the refusal's own detail.
func admitNativeConfig(config NativeAdapterConfig) error {
	if err := validateNativeConfig(config); err != nil {
		var contractErr *ContractError
		if errors.As(err, &contractErr) && contractErr.Detail != "" {
			return failWithDetail("INVALID_ADAPTER", contractErr.Detail)
		}
		return fail("INVALID_ADAPTER")
	}
	return nil
}

func validateNativeConfig(config NativeAdapterConfig) error {
	snapshot := false
	switch config.CLIResolution {
	case "":
		if validateExecutableIdentity(config.CLI) != nil {
			return failWithDetail("NATIVE_NOT_CERTIFIED", "cli_identity")
		}
	case NativeCLIResolutionRunSnapshot:
		// The host path is the only CLI fact a snapshot declaration carries.
		// It must be absolute: a bare name would resolve through the PATH of
		// whichever process read the file, so doctor and serve could bind
		// different binaries. It need not exist yet; each run resolves it at
		// start. A digest, version or version output beside the opt-in is
		// refused, never ignored.
		if config.CLI.Path == "" || !filepath.IsAbs(config.CLI.Path) ||
			filepath.Clean(config.CLI.Path) != config.CLI.Path {
			return failWithDetail("NATIVE_NOT_CERTIFIED", "cli_identity")
		}
		if config.CLI.Digest != "" || config.CLIVersion != "" ||
			config.VersionOutput != "" {
			return failWithDetail("NATIVE_NOT_CERTIFIED", "cli_resolution_pinned")
		}
		snapshot = true
	default:
		return failWithDetail("NATIVE_NOT_CERTIFIED", "cli_resolution")
	}
	if config.MaxCredentialBytes < 1 || config.MaxCredentialBytes > 1_048_576 ||
		(!snapshot && (config.VersionOutput == "" || len(config.VersionOutput) > 256)) {
		return failWithDetail("NATIVE_NOT_CERTIFIED", "cli_admission_bounds")
	}
	switch config.PinMode {
	case "", NativePinModeExact, NativePinModeMinor:
	default:
		return failWithDetail("NATIVE_NOT_CERTIFIED", "pin_mode")
	}
	var credentialTarget, versionOutput string
	switch config.Family {
	case ProfileCodex:
		credentialTarget = CodexCredentialTarget
		versionOutput = "codex-cli " + config.CLIVersion
	case ProfileClaude:
		credentialTarget = ClaudeCredentialTarget
		versionOutput = config.CLIVersion + " (Claude Code)"
	default:
		return failWithDetail("NATIVE_NOT_CERTIFIED", "family")
	}
	if config.CredentialTarget != credentialTarget {
		return failWithDetail("NATIVE_NOT_CERTIFIED", "credential_target")
	}
	if !snapshot && !versionPattern.MatchString(config.CLIVersion) {
		return failWithDetail("NATIVE_NOT_CERTIFIED", "version")
	}
	if !snapshot && config.VersionOutput != versionOutput {
		return failWithDetail("NATIVE_NOT_CERTIFIED", "version_output")
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

// NativeCLICompatibility states how a configured native CLI relates to the
// version Sworn has been tested with. It is reported by doctor and certify
// and never gates admission.
type NativeCLICompatibility struct {
	CLIVersion    string `json:"cli_version"`
	TestedVersion string `json:"tested_version"`
	Status        string `json:"status"`
	Note          string `json:"note,omitempty"`
}

const (
	NativeCLITested   = "tested"
	NativeCLIUntested = "untested"
)

// CLICompatibilityOf reports how config's CLI relates to the tested version,
// or nil for a family that has no native CLI.
func CLICompatibilityOf(config NativeAdapterConfig) *NativeCLICompatibility {
	var tested string
	switch config.Family {
	case ProfileCodex:
		tested = CodexCLIVersion
	case ProfileClaude:
		tested = ClaudeCLIVersion
	default:
		return nil
	}
	compatibility := &NativeCLICompatibility{
		CLIVersion: config.CLIVersion, TestedVersion: tested,
		Status: NativeCLITested,
	}
	if config.CLIVersion != tested {
		compatibility.Status = NativeCLIUntested
		compatibility.Note = "Sworn is tested with " + tested +
			"; this CLI is " + config.CLIVersion +
			". It may work, but compatibility and stability are not guaranteed."
	}
	return compatibility
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

func (adapter *nativeAdapter) cliCompatibility() *NativeCLICompatibility {
	if adapter == nil {
		return nil
	}
	return CLICompatibilityOf(adapter.config)
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
		ProtocolAttempt:       1,
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
