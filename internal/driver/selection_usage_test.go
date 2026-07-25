package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func providerFixture(t *testing.T, key, driverID string) ProviderConfig {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "driver")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return ProviderConfig{
		Key:           key,
		DriverID:      driverID,
		DriverVersion: "1.2.3",
		Executable: ExecutableIdentity{
			Path:   executable,
			Digest: Digest([]byte("#!/bin/sh\nexit 0\n")),
		},
		Network:       NetworkNone,
		CredentialRef: pointer(key + "-credential"),
	}
}

func TestRoleSelectionsResolveOneExactProviderAndModel(t *testing.T) {
	t.Parallel()
	configs := []ProviderConfig{
		providerFixture(t, "planner-provider", "driver.planner"),
		providerFixture(t, "implementer-provider", "driver.implementer"),
		providerFixture(t, "captain-provider", "driver.captain"),
		providerFixture(t, "verifier-provider", "driver.verifier"),
	}
	registry, err := NewSelectionRegistry(configs)
	if err != nil {
		t.Fatal(err)
	}
	selections := RoleSelections{
		Planner:     RoleSelection{"planner-provider", "planner-model"},
		Implementer: RoleSelection{"implementer-provider", "implementer-model"},
		Captain:     RoleSelection{"captain-provider", "captain-model"},
		Verifier:    RoleSelection{"verifier-provider", "verifier-model"},
	}
	expected := map[Role]RoleSelection{
		RolePlanner:     selections.Planner,
		RoleImplementer: selections.Implementer,
		RoleCaptain:     selections.Captain,
		RoleVerifier:    selections.Verifier,
	}
	for role, want := range expected {
		got, err := registry.Resolve(selections, role)
		if err != nil {
			t.Fatal(err)
		}
		if got.Provider.Key != want.Provider || got.Model != want.Model {
			t.Fatalf("%s resolution = %#v", role, got)
		}
		result := Result{
			DriverID:      got.Provider.DriverID,
			DriverVersion: got.Provider.DriverVersion,
			ObservedModel: pointer(got.Model),
		}
		if result.DriverID != got.Provider.DriverID ||
			result.DriverVersion != got.Provider.DriverVersion ||
			result.ObservedModel == nil || *result.ObservedModel != got.Model {
			t.Fatal("result was not bound to selection")
		}
	}
	if _, err := registry.Resolve(selections, RoleMerge); !IsCode(err, "ROLE_NOT_DISPATCHABLE") {
		t.Fatalf("merge error = %v", err)
	}
	unknown := selections
	unknown.Verifier.Provider = "missing"
	if _, err := registry.Resolve(unknown, RoleVerifier); !IsCode(err, "UNKNOWN_PROVIDER") {
		t.Fatalf("unknown error = %v", err)
	}
	empty := selections
	empty.Captain.Model = ""
	if _, err := registry.Resolve(empty, RoleCaptain); !IsCode(err, "INVALID_MODEL") {
		t.Fatalf("empty error = %v", err)
	}
}

func TestRoleSelectionsCodecIsClosedAndRequiresAllFourRoles(t *testing.T) {
	t.Parallel()
	selections := RoleSelections{
		Planner:     RoleSelection{"planner", "planner-model"},
		Implementer: RoleSelection{"implementer", "implementer-model"},
		Captain:     RoleSelection{"captain", "captain-model"},
		Verifier:    RoleSelection{"verifier", "verifier-model"},
	}
	body, err := EncodeRoleSelections(selections)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRoleSelections(body)
	if err != nil || decoded != selections {
		t.Fatalf("decoded = %#v, error = %v", decoded, err)
	}
	unknown := append([]byte(nil), body...)
	unknown = []byte(strings.Replace(
		string(unknown),
		`"model":"planner-model"`,
		`"model":"planner-model","fallback_model":"forbidden"`,
		1,
	))
	if _, err := DecodeRoleSelections(unknown); !IsCode(err, "UNKNOWN_FIELD") {
		t.Fatalf("fallback error = %v", err)
	}
	missing := []byte(strings.Replace(string(body),
		`,"verifier":{"provider":"verifier","model":"verifier-model"}`,
		"",
		1,
	))
	if _, err := DecodeRoleSelections(missing); !IsCode(err, "MISSING_FIELD") {
		t.Fatalf("missing error = %v", err)
	}
}

func TestProviderConfigurationHasNoGenericSecretOrLaunchEscapeHatch(t *testing.T) {
	t.Parallel()
	config := providerFixture(t, "provider", "driver.one")
	registry, err := NewSelectionRegistry([]ProviderConfig{config})
	if err != nil {
		t.Fatal(err)
	}
	config.CredentialRef = pointer("changed-after-registration")
	selected, err := registry.Resolve(RoleSelections{
		Planner:     RoleSelection{"provider", "p"},
		Implementer: RoleSelection{"provider", "i"},
		Captain:     RoleSelection{"provider", "c"},
		Verifier:    RoleSelection{"provider", "v"},
	}, RolePlanner)
	if err != nil {
		t.Fatal(err)
	}
	if *selected.Provider.CredentialRef != "provider-credential" {
		t.Fatal("registry did not clone provider configuration")
	}
	bad := providerFixture(t, "provider-2", "driver.two")
	bad.Executable.Path = "relative"
	if _, err := NewSelectionRegistry([]ProviderConfig{bad}); !IsCode(err, "INVALID_EXECUTABLE") {
		t.Fatalf("bad executable error = %v", err)
	}
	bad = providerFixture(t, "provider-3", "driver.three")
	bad.Network = "fallback"
	if _, err := NewSelectionRegistry([]ProviderConfig{bad}); !IsCode(err, "INVALID_NETWORK_POLICY") {
		t.Fatalf("bad network error = %v", err)
	}
	fakeWithNetwork := providerFixture(t, "fake-provider", FakeDriverID)
	fakeWithNetwork.Network = NetworkRequired
	if _, err := NewSelectionRegistry([]ProviderConfig{fakeWithNetwork}); !IsCode(err, "INVALID_NETWORK_POLICY") {
		t.Fatalf("networked fake error = %v", err)
	}
}

func TestUsageNormalizationPreservesZeroAndUnavailable(t *testing.T) {
	t.Parallel()
	unavailable, err := NormalizeUsage(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if unavailable.TokenStatus != UsageUnavailable ||
		unavailable.CostStatus != UsageUnavailable ||
		unavailable.InputTokens != nil || unavailable.OutputTokens != nil ||
		unavailable.CostMicroUnits != nil || unavailable.Currency != nil ||
		unavailable.Source != nil {
		t.Fatalf("unavailable = %#v", unavailable)
	}
	unavailableBody, err := EncodeUsageReceipt(unavailable)
	if err != nil {
		t.Fatal(err)
	}
	if string(unavailableBody) != `{"token_status":"unavailable","input_tokens":null,"output_tokens":null,"cost_status":"unavailable","cost_micro_units":null,"currency":null,"source":null}` {
		t.Fatalf("unavailable receipt = %s", unavailableBody)
	}
	zero, err := NormalizeUsage(
		&Usage{InputTokens: 0, OutputTokens: 0},
		&CostObservation{
			MicroUnits: 0,
			Currency:   "USD",
			Source:     CostSourceProviderReported,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if zero.TokenStatus != UsageReported || zero.CostStatus != UsageReported ||
		zero.InputTokens == nil || *zero.InputTokens != 0 ||
		zero.OutputTokens == nil || *zero.OutputTokens != 0 ||
		zero.CostMicroUnits == nil || *zero.CostMicroUnits != 0 {
		t.Fatalf("zero = %#v", zero)
	}
	body, err := EncodeUsageReceipt(zero)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"token_status":"reported","input_tokens":0,"output_tokens":0,"cost_status":"reported","cost_micro_units":0,"currency":"USD","source":"provider_reported"}` {
		t.Fatalf("receipt = %s", body)
	}
	for name, cost := range map[string]CostObservation{
		"negative":  {MicroUnits: -1, Currency: "USD", Source: CostSourceProviderReported},
		"currency":  {MicroUnits: 1, Currency: "usd", Source: CostSourceProviderReported},
		"estimated": {MicroUnits: 1, Currency: "USD", Source: "estimated"},
	} {
		if _, err := NormalizeUsage(nil, &cost); !IsCode(err, "INVALID_COST_OBSERVATION") {
			t.Fatalf("%s error = %v", name, err)
		}
	}
	one := int64(1)
	usd := "USD"
	source := CostSourceProviderReported
	for name, receipt := range map[string]UsageReceipt{
		"reported tokens missing values": {
			TokenStatus: UsageReported,
			CostStatus:  UsageUnavailable,
		},
		"unavailable tokens with values": {
			TokenStatus:  UsageUnavailable,
			InputTokens:  &one,
			OutputTokens: &one,
			CostStatus:   UsageUnavailable,
		},
		"reported cost partial": {
			TokenStatus:    UsageUnavailable,
			CostStatus:     UsageReported,
			CostMicroUnits: &one,
			Currency:       &usd,
			Source:         nil,
		},
		"unavailable cost with values": {
			TokenStatus:    UsageUnavailable,
			CostStatus:     UsageUnavailable,
			CostMicroUnits: &one,
			Currency:       &usd,
			Source:         &source,
		},
	} {
		if _, err := EncodeUsageReceipt(receipt); err == nil {
			t.Fatalf("%s receipt was accepted", name)
		}
	}
}
