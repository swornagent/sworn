package driver

import (
	"context"
	"errors"
	"sort"
	"strings"
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
	ProfileSurfaceOpenRouterChat         ProfileSurface = "openrouter_chat_completions"
	ProfileSurfaceBedrockRuntimeConverse ProfileSurface = "bedrock_runtime_converse"
	ProfileSurfaceBedrockMantleChat      ProfileSurface = "bedrock_mantle_chat_completions"
)

func (surface ProfileSurface) validFor(family ProfileFamily) bool {
	if family == ProfileOpenAIHTTP {
		return surface == ProfileSurfaceOpenAIResponses ||
			surface == ProfileSurfaceOpenAIChat ||
			surface == ProfileSurfaceOpenRouterChat
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
	case "CREDENTIAL_UNAVAILABLE", "CREDENTIAL_MALFORMED",
		"CREDENTIAL_NOT_CERTIFIED", "CREDENTIAL_IDENTITY_CHANGED",
		"CREDENTIAL_STALE", "AWS_CREDENTIAL_EXPORT_INVALID":
		return "certification_credential_failed"
	case "PROCESS_START_FAILED", "PROCESS_FAILED", "ISOLATION_UNAVAILABLE",
		"PROCESS_TREE_NOT_QUIESCENT", "INVALID_WORKSPACE",
		"WORKSPACE_INSPECTION_FAILED", "UNSAFE_WORKSPACE_SYMLINK",
		"UNSAFE_WORKSPACE_SURFACE", "WORKSPACE_IDENTITY_CHANGED",
		"WORKSPACE_MUTATED", "INPUT_BINDING_MISMATCH", "INPUT_STAGE_FAILED",
		"INVALID_PRODUCTION_INPUT_PATH", "INVALID_PROJECTION",
		"INPUT_CLEANUP_FAILED", "SCRATCH_CLEANUP_FAILED":
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
		"SUBMISSION_SHAPE_MISMATCH", "YIELD_FIRST_REQUIRED", "INVALID_HANDOFF", "INVALID_SUBMISSION",
		"INVALID_IDENTITY", "INVALID_RESPONSIBILITY", "INVALID_SUMMARY",
		"INVALID_DETAIL", "INVALID_EXACT_BYTES", "INVALID_PLAN_BYTES",
		"INVALID_DECISION", "SUBMISSION_DECLARED_PROBE", "SUBMISSION_UNATTACHED_POINTER":
		return "certification_submission_failed"
	case "CONTINUATION_INVALID", "INVALID_JSON", "MISSING_JSON",
		"TRAILING_JSON", "NONCANONICAL_JSON", "CONTINUATION_STATE_UNPLAYABLE":
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

// ProductionRequiredFamilies and ProductionRequiredSurfaces are the single
// production roster declaration (S4-lane-live-probe A4): NewProductionRegistry
// (registry build, below) and completeProductionReadiness (the readiness
// report, cmd/sworn/driver.go) both read these vars, so a roster that
// builds can never then be reported not ready for a family or surface the
// build itself did not require.
var ProductionRequiredFamilies = []ProfileFamily{
	ProfileCodex, ProfileClaude, ProfileOpenAIHTTP, ProfileGemini, ProfileBedrock,
}

var ProductionRequiredSurfaces = []ProfileSurface{
	ProfileSurfaceBedrockRuntimeConverse,
}

// ProductionRosterNote distinguishes --all's whole-roster readiness check
// from a single --profile/--model lane check; it is appended to every
// MISSING_PROFILE_FAMILY/SURFACE refusal detail.
const ProductionRosterNote = "--all checks the complete production roster " +
	"while --profile P --model M checks one lane."

// MissingProductionMembers reports which members of the one production
// roster declaration a built registry's observed families and surfaces
// omit, in roster order.
func MissingProductionMembers(
	presentFamilies map[ProfileFamily]bool,
	presentSurfaces map[ProfileSurface]bool,
) (missingFamilies []ProfileFamily, missingSurfaces []ProfileSurface) {
	for _, family := range ProductionRequiredFamilies {
		if !presentFamilies[family] {
			missingFamilies = append(missingFamilies, family)
		}
	}
	for _, surface := range ProductionRequiredSurfaces {
		if !presentSurfaces[surface] {
			missingSurfaces = append(missingSurfaces, surface)
		}
	}
	return missingFamilies, missingSurfaces
}

// productionRosterDetail renders the named-refusal detail A4 requires, for
// example "missing families: bedrock; missing surfaces:
// bedrock_runtime_converse (--all checks the complete production roster
// while --profile P --model M checks one lane)".
func productionRosterDetail(
	missingFamilies []ProfileFamily,
	missingSurfaces []ProfileSurface,
) string {
	var parts []string
	if len(missingFamilies) > 0 {
		names := make([]string, len(missingFamilies))
		for index, family := range missingFamilies {
			names[index] = string(family)
		}
		parts = append(parts, "missing families: "+strings.Join(names, ", "))
	}
	if len(missingSurfaces) > 0 {
		names := make([]string, len(missingSurfaces))
		for index, surface := range missingSurfaces {
			names[index] = string(surface)
		}
		parts = append(parts, "missing surfaces: "+strings.Join(names, ", "))
	}
	return strings.Join(parts, "; ") + " (" + ProductionRosterNote + ")"
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
		} else if registered.config.Network != NetworkRequired {
			return SelectionRegistry{}, fail("INVALID_PROFILE")
		} else if registered.config.AuthMode != AuthModeNone &&
			registered.config.CredentialRef == nil {
			return SelectionRegistry{}, fail("INVALID_PROFILE")
		} else if registered.config.AuthMode == AuthModeNone &&
			registered.config.CredentialRef != nil {
			return SelectionRegistry{}, fail("INVALID_PROFILE")
		}
	}
	presentFamilies := make(map[ProfileFamily]bool, len(families))
	for family, count := range families {
		presentFamilies[family] = count > 0
	}
	presentSurfaces := make(map[ProfileSurface]bool, len(surfaces))
	for surface, count := range surfaces {
		presentSurfaces[surface] = count > 0
	}
	missingFamilies, missingSurfaces := MissingProductionMembers(
		presentFamilies, presentSurfaces,
	)
	if len(missingFamilies) > 0 {
		return SelectionRegistry{}, failWithDetail(
			"MISSING_PROFILE_FAMILY",
			productionRosterDetail(missingFamilies, missingSurfaces),
		)
	}
	if len(missingSurfaces) > 0 {
		return SelectionRegistry{}, failWithDetail(
			"MISSING_PROFILE_SURFACE",
			productionRosterDetail(missingFamilies, missingSurfaces),
		)
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
