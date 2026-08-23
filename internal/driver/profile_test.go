package driver

import (
	"context"
	"fmt"
	"testing"
)

type familyAdapter struct {
	identity AdapterIdentity
	family   ProfileFamily
	surface  ProfileSurface
	state    ReadinessState
	code     string
}

func (adapter *familyAdapter) Identity() AdapterIdentity { return adapter.identity }
func (adapter *familyAdapter) profileFamily() ProfileFamily {
	return adapter.family
}
func (adapter *familyAdapter) profileSurface() ProfileSurface {
	return adapter.surface
}
func (adapter *familyAdapter) checkProfile(
	context.Context,
	profileCheckKind,
	ProfileConfig,
	string,
) (ReadinessState, string) {
	return adapter.state, adapter.code
}
func (adapter *familyAdapter) invoke(context.Context, Invocation) (Observation, error) {
	return Observation{}, fail("TRANSPORT_FAILURE")
}

func TestProductionRegistryRequiresEveryFamilyAndExplicitRoleModels(t *testing.T) {
	t.Parallel()
	families := []ProfileFamily{
		ProfileFake, ProfileCodex, ProfileClaude, ProfileOpenAIHTTP,
		ProfileGemini, ProfileBedrock,
	}
	surfaces := []ProfileSurface{
		"", "", "", ProfileSurfaceOpenAIResponses, "",
		ProfileSurfaceBedrockRuntimeConverse,
	}
	var adapters []Adapter
	var configs []ProfileConfig
	for index, family := range families {
		key := "adapter-" + itoa(index)
		adapter := &familyAdapter{
			identity: AdapterIdentity{
				Key: key, ID: "driver." + key, Version: "1.0.0",
				ConfigurationDigest: Digest([]byte(key)),
			},
			family: family, surface: surfaces[index],
			state: ReadinessPass, code: "fixture_ready",
		}
		profile := ProfileConfig{
			Key: "profile-" + itoa(index), Adapter: key, Network: NetworkRequired,
		}
		if family == ProfileFake {
			profile.Network = NetworkNone
		} else {
			ref := "credential-" + itoa(index)
			profile.CredentialRef = &ref
		}
		adapters = append(adapters, adapter)
		configs = append(configs, profile)
	}
	registry, err := NewProductionRegistry(configs, adapters)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Profiles()) != len(families) {
		t.Fatalf("profiles = %v", registry.Profiles())
	}
	withoutFake, err := NewProductionRegistry(configs[1:], adapters)
	if err != nil || len(withoutFake.Profiles()) != len(families)-1 {
		t.Fatalf(
			"production registry without fake = %v, %v",
			withoutFake.Profiles(),
			err,
		)
	}
	selections := RoleSelections{
		Planner:     RoleSelection{Profile: "profile-1", Model: "planner-model"},
		Implementer: RoleSelection{Profile: "profile-2", Model: "implementer-model"},
		Captain:     RoleSelection{Profile: "profile-3", Model: "captain-model"},
		Verifier:    RoleSelection{Profile: "profile-4", Model: "verifier-model"},
	}
	for _, role := range []Role{RolePlanner, RoleImplementer, RoleCaptain, RoleVerifier} {
		selected, resolveErr := registry.Resolve(selections, role)
		if resolveErr != nil || selected.Model == "" ||
			selected.Profile.Key == "" || selected.Adapter.Key == "" {
			t.Fatalf("%s resolve = %#v, %v", role, selected, resolveErr)
		}
	}
	if _, err := registry.Resolve(selections, Role("merge")); !IsCode(err, "ROLE_NOT_DISPATCHABLE") {
		t.Fatalf("Merge error = %v", err)
	}
	mantleSurface := adapters[len(adapters)-1].(*familyAdapter)
	mantleSurface.surface = ProfileSurfaceBedrockMantleChat
	if _, err := NewProductionRegistry(
		configs,
		adapters,
	); !IsCode(err, "MISSING_PROFILE_SURFACE") {
		t.Fatalf("missing surface error = %v", err)
	}
	mantleSurface.surface = ProfileSurfaceBedrockRuntimeConverse
	missingCredential := append([]ProfileConfig(nil), configs...)
	missingCredential[1].CredentialRef = nil
	if _, err := NewProductionRegistry(missingCredential, adapters); !IsCode(err, "INVALID_PROFILE") {
		t.Fatalf("missing credential error = %v", err)
	}
}

func TestProfileReportsAreClosedSecretFreeAndDoNotSubstitute(t *testing.T) {
	t.Parallel()
	secret := "credential-secret-canary"
	ref := "opaque-ref"
	adapter := &familyAdapter{
		identity: AdapterIdentity{
			Key: "adapter", ID: "driver.adapter", Version: "1.0.0",
			ConfigurationDigest: Digest([]byte("configuration")),
		},
		family: ProfileGemini, state: ReadinessNotCertified,
		code: "live_probe_not_configured",
	}
	registry, err := NewSelectionRegistry(
		[]ProfileConfig{{
			Key: "profile", Adapter: "adapter", Network: NetworkRequired,
			CredentialRef: &ref,
		}},
		[]Adapter{adapter},
	)
	if err != nil {
		t.Fatal(err)
	}
	report := registry.Certify(context.Background(), "profile", "model")
	if report.State != ReadinessNotCertified ||
		report.Profile != "profile" || report.Model != "model" ||
		report.AdapterID != adapter.identity.ID ||
		report.Code != "live_probe_not_configured" {
		t.Fatalf("report = %#v", report)
	}
	body, err := canonicalJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	if bytesContains(body, []byte(secret)) || bytesContains(body, []byte(ref)) {
		t.Fatalf("report leaked private data: %s", body)
	}
	adapter.state, adapter.code = ReadinessFail, "live_probe_failed"
	failed := registry.Certify(context.Background(), "profile", "model")
	if failed.State != ReadinessFail || failed.Code != "live_probe_failed" {
		t.Fatalf("failed report = %#v", failed)
	}
	adapter.state, adapter.code = ReadinessPass, "live_probe_passed"
	passed := registry.Certify(context.Background(), "profile", "model")
	if passed.State != ReadinessPass || passed.Code != "live_probe_passed" {
		t.Fatalf("passed report = %#v", passed)
	}
	unknown := registry.Doctor(context.Background(), "other", "model")
	if unknown.State != ReadinessNotCertified || unknown.Code != "unknown_profile" {
		t.Fatalf("unknown report = %#v", unknown)
	}
}

func TestCertificationFailureCodesAreClosedAndSecretFree(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		code string
	}{
		{"setup", fail("LIVE_PROBE_FAILED"), "certification_setup_failed"},
		{"credential", fail("CREDENTIAL_UNAVAILABLE"), "certification_credential_failed"},
		{"runtime", fail("PROCESS_START_FAILED"), "certification_runtime_failed"},
		{"transport", fail("PROVIDER_TRANSPORT_FAILED"), "certification_provider_transport_failed"},
		{"rejected", fail("PROVIDER_ERROR"), "certification_provider_rejected"},
		{"authorization", providerHTTPStatusError(403, ""), "certification_provider_authorization_failed"},
		{"limited", providerHTTPStatusError(429, ""), "certification_provider_limited"},
		{"request", providerHTTPStatusError(400, ""), "certification_provider_request_rejected"},
		{"unavailable", providerHTTPStatusError(503, ""), "certification_provider_unavailable"},
		{"submission", fail("MISSING_SUBMISSION"), "certification_submission_failed"},
		{"response contract", failContinuation("test.fixture.response_contract"), "certification_response_contract_failed"},
		{"unplayable continuation state", fail("CONTINUATION_STATE_UNPLAYABLE"), "certification_response_contract_failed"},
		{"usage", fail("INVALID_USAGE"), "certification_usage_failed"},
		{"tool", fail("TOOL_NOT_ALLOWED"), "certification_tool_failed"},
		{"resource", fail("RESOURCE_LIMIT"), "certification_resource_limited"},
		{"timeout", context.DeadlineExceeded, "certification_timeout"},
		{"cancelled", context.Canceled, "certification_cancelled"},
		{"wrapped", fmt.Errorf("secret-canary: %w", fail("PROVIDER_ERROR")), "certification_provider_rejected"},
		{"arbitrary error", fmt.Errorf("secret-canary"), "certification_contract_failed"},
		{"arbitrary contract", fail("SECRET_CANARY"), "certification_contract_failed"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			code := certificationFailureCode(test.err)
			if code != test.code {
				t.Fatalf("code = %q, want %q", code, test.code)
			}
		})
	}
}

func TestSubmissionAdapterErrorsRemainClassifiable(t *testing.T) {
	for _, code := range []string{
		"INVALID_SUBMISSION", "INVALID_IDENTITY", "INVALID_RESPONSIBILITY",
		"INVALID_SUMMARY", "INVALID_DETAIL", "INVALID_EXACT_BYTES",
		"INVALID_PLAN_BYTES", "INVALID_DECISION", "SUBMISSION_REJECTED",
		"SUBMISSION_SHAPE_MISMATCH",
	} {
		normalized := normalizeAdapterError(fail(code))
		if !IsCode(normalized, code) ||
			certificationFailureCode(normalized) != "certification_submission_failed" {
			t.Fatalf("%s normalized to %v", code, normalized)
		}
	}
}

func bytesContains(body, value []byte) bool {
	for index := 0; index+len(value) <= len(body); index++ {
		match := true
		for offset := range value {
			if body[index+offset] != value[offset] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
