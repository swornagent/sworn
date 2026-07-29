package driver

import (
	"context"
	"errors"
	"sort"
)

// ProfileFamily is a closed description of one production transport family.
// It is informational only: selection and authority remain bound by
// ProfileConfig, AdapterIdentity, and the exact requested model.
type ProfileFamily string

const (
	ProfileFake       ProfileFamily = "fake"
	ProfileCodex      ProfileFamily = "codex_cli"
	ProfileClaude     ProfileFamily = "claude_code_cli"
	ProfileOpenAIHTTP ProfileFamily = "openai_compatible_http"
	ProfileDeepSeek   ProfileFamily = "deepseek"
	ProfileGemini     ProfileFamily = "gemini_generate_content"
	ProfileBedrock    ProfileFamily = "bedrock"
)

func (family ProfileFamily) valid() bool {
	switch family {
	case ProfileFake, ProfileCodex, ProfileClaude, ProfileOpenAIHTTP,
		ProfileDeepSeek, ProfileGemini, ProfileBedrock:
		return true
	default:
		return false
	}
}

// ProfileSurface distinguishes closed endpoint dialects within one family.
// It is empty for families with only one admitted production surface.
type ProfileSurface string

const (
	ProfileSurfaceOpenAIResponses        ProfileSurface = "openai_responses"
	ProfileSurfaceOpenAIChat             ProfileSurface = "openai_chat_completions"
	ProfileSurfaceBedrockRuntimeConverse ProfileSurface = "bedrock_runtime_converse"
	ProfileSurfaceBedrockMantleChat      ProfileSurface = "bedrock_mantle_chat_completions"
)

func (surface ProfileSurface) validFor(family ProfileFamily) bool {
	if family == ProfileOpenAIHTTP {
		return surface == ProfileSurfaceOpenAIResponses ||
			surface == ProfileSurfaceOpenAIChat
	}
	if family == ProfileBedrock {
		return surface == ProfileSurfaceBedrockRuntimeConverse ||
			surface == ProfileSurfaceBedrockMantleChat
	}
	return surface == ""
}

type ReadinessState string

const (
	ReadinessPass         ReadinessState = "PASS"
	ReadinessFail         ReadinessState = "FAIL"
	ReadinessNotCertified ReadinessState = "NOT_CERTIFIED"
)

func (state ReadinessState) valid() bool {
	return state == ReadinessPass || state == ReadinessFail ||
		state == ReadinessNotCertified
}

// ProfileReport is intentionally content- and secret-free. Code is a closed
// diagnostic token; adapters must not return provider errors, paths, argv,
// endpoints, credential references, or response content through this seam.
type ProfileReport struct {
	Profile             string         `json:"profile"`
	Model               string         `json:"model"`
	Family              ProfileFamily  `json:"family"`
	Surface             ProfileSurface `json:"surface,omitempty"`
	AdapterID           string         `json:"adapter_id"`
	AdapterVersion      string         `json:"adapter_version"`
	ConfigurationDigest string         `json:"configuration_digest"`
	State               ReadinessState `json:"state"`
	Code                string         `json:"code"`
}

type profileCheckKind uint8

const (
	checkInspect profileCheckKind = iota + 1
	checkDoctor
	checkCertify
)

type profileChecker interface {
	profileFamily() ProfileFamily
	checkProfile(context.Context, profileCheckKind, ProfileConfig, string) (ReadinessState, string)
}

type profileSurfaceReporter interface {
	profileSurface() ProfileSurface
}

// certificationFailureCode exposes only a small stage vocabulary. Provider
// text, stderr, request content, paths, credentials, and arbitrary error codes
// never cross the readiness boundary.
func certificationFailureCode(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded),
		IsCode(err, "INVOCATION_TIMEOUT"):
		return "certification_timeout"
	case errors.Is(err, context.Canceled),
		IsCode(err, "INVOCATION_CANCELLED"):
		return "certification_cancelled"
	}
	var contractErr *ContractError
	if !errors.As(err, &contractErr) {
		return "certification_contract_failed"
	}
	switch contractErr.Code {
	case "LIVE_PROBE_FAILED":
		return "certification_setup_failed"
	case "CREDENTIAL_UNAVAILABLE", "CREDENTIAL_NOT_CERTIFIED",
		"CREDENTIAL_IDENTITY_CHANGED", "AWS_CREDENTIAL_EXPORT_INVALID":
		return "certification_credential_failed"
	case "PROCESS_START_FAILED", "PROCESS_FAILED", "ISOLATION_UNAVAILABLE",
		"PROCESS_TREE_NOT_QUIESCENT", "INVALID_WORKSPACE",
		"WORKSPACE_INSPECTION_FAILED", "UNSAFE_WORKSPACE_SYMLINK",
		"UNSAFE_WORKSPACE_SURFACE", "WORKSPACE_IDENTITY_CHANGED",
		"WORKSPACE_MUTATED", "INPUT_BINDING_MISMATCH", "INPUT_STAGE_FAILED",
		"INVALID_PRODUCTION_INPUT_PATH", "INVALID_PROJECTION",
		"INPUT_CLEANUP_FAILED":
		return "certification_runtime_failed"
	case "PROVIDER_TRANSPORT_FAILED", "TRANSPORT_FAILURE",
		"HTTP_REDIRECT_REFUSED", "AWS_RESOLUTION_FAILED", "AWS_SIGNING_FAILED":
		return "certification_provider_transport_failed"
	case "PROVIDER_ERROR":
		return "certification_provider_rejected"
	case "PROVIDER_AUTHORIZATION_FAILED":
		return "certification_provider_authorization_failed"
	case "PROVIDER_LIMITED":
		return "certification_provider_limited"
	case "PROVIDER_REQUEST_REJECTED":
		return "certification_provider_request_rejected"
	case "PROVIDER_UNAVAILABLE":
		return "certification_provider_unavailable"
	case "MISSING_SUBMISSION", "SUBMISSION_REJECTED", "SUBMISSION_CONFLICT",
		"SUBMISSION_PROTOCOL_FAILED", "SUBMISSION_BINDING_MISMATCH",
		"SUBMISSION_SHAPE_MISMATCH", "INVALID_HANDOFF", "INVALID_SUBMISSION",
		"INVALID_IDENTITY", "INVALID_RESPONSIBILITY", "INVALID_SUMMARY",
		"INVALID_DETAIL", "INVALID_EXACT_BYTES", "INVALID_PLAN_BYTES",
		"INVALID_DECISION":
		return "certification_submission_failed"
	case "CONTINUATION_INVALID", "INVALID_JSON", "MISSING_JSON",
		"TRAILING_JSON", "NONCANONICAL_JSON":
		return "certification_response_contract_failed"
	case "INVALID_USAGE", "PARTIAL_USAGE", "PARTIAL_COST",
		"INVALID_COST_OBSERVATION":
		return "certification_usage_failed"
	case "TOOL_NOT_ALLOWED", "INVALID_TOOL_ARGUMENT", "TOOL_PATH_INVALID",
		"TOOL_READ_FAILED", "TOOL_WRITE_FAILED", "TOOL_EDIT_FAILED":
		return "certification_tool_failed"
	case "RESOURCE_LIMIT", "OUTPUT_OVERFLOW":
		return "certification_resource_limited"
	default:
		return "certification_contract_failed"
	}
}

// NewProductionRegistry admits the complete W5 production-family set as one
// common registry. The deterministic fake remains available to subset
// registries and scripted manifests, but is not required for production
// readiness. This does not create role or model choices; callers must still
// provide all four explicit RoleSelections for each dispatch configuration.
func NewProductionRegistry(
	configs []ProfileConfig,
	adapters []Adapter,
) (SelectionRegistry, error) {
	registry, err := NewSelectionRegistry(configs, adapters)
	if err != nil {
		return SelectionRegistry{}, err
	}
	families := make(map[ProfileFamily]int)
	surfaces := make(map[ProfileSurface]int)
	for _, registered := range registry.profiles {
		checker, ok := registered.adapter.(profileChecker)
		if !ok {
			return SelectionRegistry{}, fail("ADAPTER_NOT_CERTIFIABLE")
		}
		family := checker.profileFamily()
		if !family.valid() {
			return SelectionRegistry{}, fail("INVALID_ADAPTER")
		}
		surface := ProfileSurface("")
		if reporter, ok := registered.adapter.(profileSurfaceReporter); ok {
			surface = reporter.profileSurface()
		}
		if !surface.validFor(family) {
			return SelectionRegistry{}, fail("INVALID_ADAPTER")
		}
		families[family]++
		if surface != "" {
			surfaces[surface]++
		}
		if family == ProfileFake {
			if registered.config.Network != NetworkNone ||
				registered.config.CredentialRef != nil {
				return SelectionRegistry{}, fail("INVALID_PROFILE")
			}
		} else if registered.config.Network != NetworkRequired ||
			registered.config.CredentialRef == nil {
			return SelectionRegistry{}, fail("INVALID_PROFILE")
		}
	}
	for _, family := range []ProfileFamily{
		ProfileCodex, ProfileClaude, ProfileOpenAIHTTP,
		ProfileDeepSeek, ProfileGemini, ProfileBedrock,
	} {
		if families[family] < 1 {
			return SelectionRegistry{}, fail("MISSING_PROFILE_FAMILY")
		}
	}
	for _, surface := range []ProfileSurface{
		ProfileSurfaceBedrockRuntimeConverse,
		ProfileSurfaceBedrockMantleChat,
	} {
		if surfaces[surface] < 1 {
			return SelectionRegistry{}, fail("MISSING_PROFILE_SURFACE")
		}
	}
	return registry, nil
}

func (registry SelectionRegistry) Inspect(
	ctx context.Context,
	profile string,
	model string,
) ProfileReport {
	return registry.check(ctx, checkInspect, profile, model)
}

func (registry SelectionRegistry) Doctor(
	ctx context.Context,
	profile string,
	model string,
) ProfileReport {
	return registry.check(ctx, checkDoctor, profile, model)
}

func (registry SelectionRegistry) Certify(
	ctx context.Context,
	profile string,
	model string,
) ProfileReport {
	return registry.check(ctx, checkCertify, profile, model)
}

func (registry SelectionRegistry) Profiles() []string {
	keys := make([]string, 0, len(registry.profiles))
	for key := range registry.profiles {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (registry SelectionRegistry) check(
	ctx context.Context,
	kind profileCheckKind,
	profile string,
	model string,
) ProfileReport {
	report := ProfileReport{
		Profile: profile,
		Model:   model,
		State:   ReadinessNotCertified,
		Code:    "profile_not_certified",
	}
	if ctx == nil || ctx.Err() != nil ||
		!providerKeyPattern.MatchString(profile) ||
		validateText(model, 500, false) != nil {
		report.State = ReadinessFail
		report.Code = "invalid_check_request"
		return report
	}
	registered, ok := registry.profiles[profile]
	if !ok {
		report.Code = "unknown_profile"
		return report
	}
	identity := registered.adapter.Identity()
	report.AdapterID = identity.ID
	report.AdapterVersion = identity.Version
	report.ConfigurationDigest = identity.ConfigurationDigest
	checker, ok := registered.adapter.(profileChecker)
	if !ok {
		report.Code = "adapter_not_certifiable"
		return report
	}
	report.Family = checker.profileFamily()
	if reporter, ok := registered.adapter.(profileSurfaceReporter); ok {
		report.Surface = reporter.profileSurface()
	}
	if !report.Family.valid() || validateAdapterIdentity(identity) != nil ||
		!report.Surface.validFor(report.Family) ||
		identity.Key != registered.config.Adapter {
		report.State = ReadinessFail
		report.Code = "adapter_identity_invalid"
		return report
	}
	state, code := checker.checkProfile(ctx, kind, cloneProfileConfig(registered.config), model)
	if !state.valid() || !driverIdentityPattern.MatchString(code) {
		report.State = ReadinessFail
		report.Code = "invalid_check_result"
		return report
	}
	report.State, report.Code = state, code
	return report
}

func (adapter *ProcessAdapter) profileFamily() ProfileFamily {
	if adapter != nil && adapter.identity.ID == FakeDriverID {
		return ProfileFake
	}
	return ""
}

func (adapter *ProcessAdapter) checkProfile(
	ctx context.Context,
	_ profileCheckKind,
	profile ProfileConfig,
	model string,
) (ReadinessState, string) {
	if ctx.Err() != nil || adapter == nil ||
		adapter.identity.ID != FakeDriverID ||
		profile.Adapter != adapter.identity.Key ||
		profile.Network != NetworkNone ||
		profile.CredentialRef != nil ||
		validateText(model, 500, false) != nil {
		return ReadinessFail, "fake_profile_invalid"
	}
	return ReadinessPass, "fake_profile_ready"
}
