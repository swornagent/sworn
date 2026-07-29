package driver

import (
	"context"
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
	ProfileSurfaceBedrockRuntimeConverse ProfileSurface = "bedrock_runtime_converse"
	ProfileSurfaceBedrockMantleChat      ProfileSurface = "bedrock_mantle_chat_completions"
)

func (surface ProfileSurface) validFor(family ProfileFamily) bool {
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

// NewProductionRegistry admits the complete W5 family set as one common
// registry. It does not create role or model choices; callers must still
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
		ProfileFake, ProfileCodex, ProfileClaude, ProfileOpenAIHTTP,
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
