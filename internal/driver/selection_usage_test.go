package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func processAdapterFixture(t *testing.T, key, adapterID string) *ProcessAdapter {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "driver")
	body := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(executable, body, 0o700); err != nil {
		t.Fatal(err)
	}
	adapter, err := NewProcessAdapter(
		key,
		adapterID,
		"1.2.3",
		ExecutableIdentity{Path: executable, Digest: Digest(body)},
	)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

type memoryAdapter struct {
	identity    AdapterIdentity
	observation *Observation
	err         error
}

func (adapter *memoryAdapter) Identity() AdapterIdentity {
	return adapter.identity
}

func (adapter *memoryAdapter) invoke(
	context.Context,
	Invocation,
) (Observation, error) {
	if adapter.observation != nil {
		return *adapter.observation, adapter.err
	}
	return Observation{
		TransportStatus: Completed,
		Usage: UsageReceipt{
			TokenStatus: UsageUnavailable,
			CostStatus:  UsageUnavailable,
		},
		Diagnostic: Diagnostic{Code: "none"},
	}, adapter.err
}

func memoryInvocationFixture(
	t *testing.T,
) (Invocation, *memoryAdapter, SealedHandoff) {
	t.Helper()
	adapter := &memoryAdapter{identity: AdapterIdentity{
		Key:                 "memory-adapter",
		ID:                  "sworn.memory",
		Version:             "1.0.0",
		ConfigurationDigest: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
	}}
	selected := SelectedProfile{
		Profile: ProfileConfig{
			Key:     "memory-profile",
			Adapter: adapter.identity.Key,
			Network: NetworkNone,
		},
		Adapter: adapter.identity,
		Model:   "memory-model",
		adapter: adapter,
	}
	request, err := NewRequest(
		"memory-invocation",
		RolePlanner,
		selected.Profile.Key,
		selected.Model,
		Workspace{Path: GuestWorkspacePath, Access: ReadWrite},
		[]Input{},
		true,
		Limits{TimeoutMillis: 60_000, OutputBytes: 65_536},
	)
	if err != nil {
		t.Fatal(err)
	}
	permission, err := NewSubmissionPermission(
		request,
		selected,
		ContainmentReadWrite,
		PlannerProposal,
	)
	if err != nil {
		t.Fatal(err)
	}
	submissionBody, err := EncodeSubmission(submissionFixture(
		t,
		request.InvocationID,
		PlannerProposal,
		"",
	))
	if err != nil {
		t.Fatal(err)
	}
	server, err := newSubmissionServer(permission)
	if err != nil {
		t.Fatal(err)
	}
	seal, sealBytes, err := server.Submit(submissionBody)
	if err != nil {
		t.Fatal(err)
	}
	handoff := SealedHandoff{
		SubmissionBytes:  submissionBody,
		SubmissionDigest: Digest(submissionBody),
		SealBytes:        sealBytes,
		SealDigest:       Digest(sealBytes),
	}
	if !seal.Accepted {
		t.Fatal("fixture submission was not accepted")
	}
	return Invocation{
		Request:       request,
		HostWorkspace: t.TempDir(),
		Selected:      selected,
		Permission:    permission,
		Inputs:        []InputContent{},
	}, adapter, handoff
}

func unavailableObservation() Observation {
	return Observation{
		TransportStatus: Completed,
		Usage: UsageReceipt{
			TokenStatus: UsageUnavailable,
			CostStatus:  UsageUnavailable,
		},
		Diagnostic: Diagnostic{Code: "none"},
	}
}

func TestRoleSelectionsResolveOneExactProfileAdapterAndModel(t *testing.T) {
	t.Parallel()
	adapters := []Adapter{
		processAdapterFixture(t, "planner-adapter", "driver.planner"),
		processAdapterFixture(t, "implementer-adapter", "driver.implementer"),
		processAdapterFixture(t, "captain-adapter", "driver.captain"),
		processAdapterFixture(t, "verifier-adapter", "driver.verifier"),
	}
	configs := []ProfileConfig{
		{Key: "planner-profile", Adapter: "planner-adapter", Network: NetworkNone},
		{Key: "implementer-profile", Adapter: "implementer-adapter", Network: NetworkNone},
		{Key: "captain-profile", Adapter: "captain-adapter", Network: NetworkNone},
		{Key: "verifier-profile", Adapter: "verifier-adapter", Network: NetworkNone},
	}
	registry, err := NewSelectionRegistry(configs, adapters)
	if err != nil {
		t.Fatal(err)
	}
	selections := RoleSelections{
		Planner:     RoleSelection{"planner-profile", "planner-model"},
		Implementer: RoleSelection{"implementer-profile", "implementer-model"},
		Captain:     RoleSelection{"captain-profile", "captain-model"},
		Verifier:    RoleSelection{"verifier-profile", "verifier-model"},
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
		if got.Profile.Key != want.Profile || got.Model != want.Model ||
			got.Adapter.Key != got.Profile.Adapter ||
			got.Adapter.ConfigurationDigest == "" ||
			got.adapter == nil {
			t.Fatalf("%s resolution = %#v", role, got)
		}
	}
	if _, err := registry.Resolve(
		selections,
		Role("merge"),
	); !IsCode(err, "ROLE_NOT_DISPATCHABLE") {
		t.Fatalf("Merge error = %v", err)
	}
	unknown := selections
	unknown.Verifier.Profile = "missing"
	if _, err := registry.Resolve(
		unknown,
		RoleVerifier,
	); !IsCode(err, "UNKNOWN_PROFILE") {
		t.Fatalf("unknown profile error = %v", err)
	}
	empty := selections
	empty.Captain.Model = ""
	if _, err := registry.Resolve(
		empty,
		RoleCaptain,
	); !IsCode(err, "INVALID_MODEL") {
		t.Fatalf("empty model error = %v", err)
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
	unknown := []byte(strings.Replace(
		string(body),
		`"model":"planner-model"`,
		`"model":"planner-model","fallback_model":"forbidden"`,
		1,
	))
	if _, err := DecodeRoleSelections(unknown); !IsCode(err, "UNKNOWN_FIELD") {
		t.Fatalf("fallback error = %v", err)
	}
	missing := []byte(strings.Replace(
		string(body),
		`,"verifier":{"profile":"verifier","model":"verifier-model"}`,
		"",
		1,
	))
	if _, err := DecodeRoleSelections(missing); !IsCode(err, "MISSING_FIELD") {
		t.Fatalf("missing error = %v", err)
	}
	withMerge := []byte(strings.Replace(
		string(body),
		`"verifier":`,
		`"merge":{"profile":"merge","model":"merge-model"},"verifier":`,
		1,
	))
	if _, err := DecodeRoleSelections(withMerge); !IsCode(err, "UNKNOWN_FIELD") {
		t.Fatalf("Merge selection error = %v", err)
	}
}

func TestRegistryIsProviderNeutralAndHasNoFallbackOrLaunchEscape(t *testing.T) {
	t.Parallel()
	inMemory := &memoryAdapter{identity: AdapterIdentity{
		Key:                 "http-adapter",
		ID:                  "sworn.http",
		Version:             "1.0.0",
		ConfigurationDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}}
	config := ProfileConfig{
		Key:           "cloud-profile",
		Adapter:       "http-adapter",
		Network:       NetworkRequired,
		CredentialRef: pointer("cloud-credential"),
	}
	registry, err := NewSelectionRegistry([]ProfileConfig{config}, []Adapter{inMemory})
	if err != nil {
		t.Fatal(err)
	}
	config.CredentialRef = pointer("changed-after-registration")
	selected, err := registry.Resolve(RoleSelections{
		Planner:     RoleSelection{"cloud-profile", "p"},
		Implementer: RoleSelection{"cloud-profile", "i"},
		Captain:     RoleSelection{"cloud-profile", "c"},
		Verifier:    RoleSelection{"cloud-profile", "v"},
	}, RolePlanner)
	if err != nil {
		t.Fatal(err)
	}
	if *selected.Profile.CredentialRef != "cloud-credential" {
		t.Fatal("registry did not clone profile configuration")
	}
	if _, ok := selected.adapter.(*memoryAdapter); !ok {
		t.Fatalf("non-process adapter was not preserved: %T", selected.adapter)
	}
	if _, err := NewSelectionRegistry(
		[]ProfileConfig{{
			Key:     "unknown-profile",
			Adapter: "missing",
			Network: NetworkNone,
		}},
		[]Adapter{inMemory},
	); !IsCode(err, "UNKNOWN_ADAPTER") {
		t.Fatalf("unknown adapter error = %v", err)
	}
	badNetwork := config
	badNetwork.Key = "bad-network"
	badNetwork.Network = "fallback"
	if _, err := NewSelectionRegistry(
		[]ProfileConfig{badNetwork},
		[]Adapter{inMemory},
	); !IsCode(err, "INVALID_NETWORK_POLICY") {
		t.Fatalf("bad network error = %v", err)
	}
	fake := processAdapterFixture(t, "fake-adapter", FakeDriverID)
	if _, err := NewSelectionRegistry(
		[]ProfileConfig{{
			Key:     "networked-fake",
			Adapter: "fake-adapter",
			Network: NetworkRequired,
		}},
		[]Adapter{fake},
	); !IsCode(err, "INVALID_NETWORK_POLICY") {
		t.Fatalf("networked fake error = %v", err)
	}
	if _, err := NewProcessAdapter(
		"bad",
		"driver.bad",
		"1.0.0",
		ExecutableIdentity{Path: "relative", Digest: Digest(nil)},
	); !IsCode(err, "INVALID_EXECUTABLE") {
		t.Fatalf("relative executable error = %v", err)
	}
}

func TestDispatcherScrubsHostileAdapterObservationsAndErrors(t *testing.T) {
	invocation, adapter, handoff := memoryInvocationFixture(t)
	valid := unavailableObservation()
	valid.Handoff = &handoff
	adapter.observation = &valid
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if err != nil || observation.Handoff == nil ||
		observation.Diagnostic.Code != "none" {
		t.Fatalf("valid non-process observation=%#v error=%v", observation, err)
	}

	missing := unavailableObservation()
	adapter.observation = &missing
	observation, err = (Dispatcher{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "MISSING_SUBMISSION") || observation.Handoff != nil ||
		observation.Diagnostic.Code != "submission_absent" {
		t.Fatalf("missing handoff observation=%#v error=%v", observation, err)
	}

	const sentinel = "hostile-secret-sentinel"
	forged := unavailableObservation()
	forged.Handoff = &SealedHandoff{
		SubmissionBytes:  []byte(sentinel),
		SubmissionDigest: Digest([]byte(sentinel)),
		SealBytes:        []byte("{}"),
		SealDigest:       Digest([]byte("{}")),
	}
	adapter.observation = &forged
	observation, err = (Dispatcher{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "INVALID_HANDOFF") || observation.Handoff != nil ||
		observation.TransportStatus != RunnerError ||
		observation.Diagnostic.Code != "invalid_handoff" ||
		observation.Usage.TokenStatus != UsageUnavailable ||
		observation.Usage.CostStatus != UsageUnavailable {
		t.Fatalf("forged handoff observation=%#v error=%v", observation, err)
	}
	assertObservationOmits(t, observation, sentinel)

	misleading := valid
	misleading.Diagnostic.Code = "submission_absent"
	adapter.observation = &misleading
	observation, err = (Dispatcher{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "INVALID_OBSERVATION") ||
		observation.Diagnostic.Code != "invalid_observation" ||
		observation.Handoff != nil {
		t.Fatalf("misleading success observation=%#v error=%v", observation, err)
	}

	hostileDiagnostic := valid
	hostileDiagnostic.Diagnostic.Code = sentinel
	adapter.observation = &hostileDiagnostic
	observation, err = (Dispatcher{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "INVALID_OBSERVATION") ||
		observation.Diagnostic.Code != "invalid_observation" ||
		observation.TransportStatus != RunnerError ||
		observation.Usage.TokenStatus != UsageUnavailable ||
		observation.Usage.CostStatus != UsageUnavailable ||
		observation.Handoff != nil {
		t.Fatalf("hostile diagnostic observation=%#v error=%v", observation, err)
	}
	assertObservationOmits(t, observation, sentinel)

	hostileEvent := valid
	hostileEvent.Events = []TerminalEvent{{Sequence: 1, Kind: sentinel}}
	adapter.observation = &hostileEvent
	observation, err = (Dispatcher{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "INVALID_OBSERVATION") ||
		observation.Diagnostic.Code != "invalid_observation" ||
		len(observation.Events) != 0 || observation.Handoff != nil {
		t.Fatalf("hostile event observation=%#v error=%v", observation, err)
	}
	assertObservationOmits(t, observation, sentinel)

	hostileFailure := unavailableObservation()
	hostileFailure.Diagnostic.Code = sentinel
	hostileFailure.Events = []TerminalEvent{{Sequence: 1, Kind: sentinel}}
	hostileFailure.Handoff = &handoff
	adapter.observation = &hostileFailure
	adapter.err = errors.New(sentinel)
	observation, err = (Dispatcher{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "ADAPTER_FAILURE") ||
		observation.Diagnostic.Code != "adapter_failed" ||
		observation.TransportStatus != RunnerError ||
		observation.Usage.TokenStatus != UsageUnavailable ||
		observation.Usage.CostStatus != UsageUnavailable ||
		len(observation.Events) != 0 || observation.Handoff != nil {
		t.Fatalf("hostile adapter failure observation=%#v error=%v", observation, err)
	}
	assertObservationOmits(t, observation, sentinel)
	if strings.Contains(err.Error(), sentinel) {
		t.Fatalf("adapter error leaked sentinel: %v", err)
	}

	adapter.err = errors.Join(errors.New(sentinel), fail("PROCESS_FAILED"))
	observation, err = (Dispatcher{}).Invoke(context.Background(), invocation)
	if !IsCode(err, "PROCESS_FAILED") ||
		observation.Diagnostic.Code != "adapter_failed" ||
		strings.Contains(err.Error(), sentinel) {
		t.Fatalf("wrapped adapter failure observation=%#v error=%v", observation, err)
	}
	assertObservationOmits(t, observation, sentinel)
}

func assertObservationOmits(t *testing.T, observation Observation, value string) {
	t.Helper()
	body, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte(value)) {
		t.Fatalf("observation retained %q: %s", value, body)
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
		unavailable.CacheStatus != UsageUnavailable ||
		unavailable.InputTokens != nil || unavailable.OutputTokens != nil ||
		unavailable.CostMicroUnits != nil || unavailable.Currency != nil ||
		unavailable.Source != nil ||
		unavailable.CacheReadTokens != nil ||
		unavailable.CacheWriteTokens != nil {
		t.Fatalf("unavailable = %#v", unavailable)
	}
	unavailableBody, err := EncodeUsageReceipt(unavailable)
	if err != nil {
		t.Fatal(err)
	}
	if string(unavailableBody) != `{"token_status":"unavailable","input_tokens":null,"output_tokens":null,"cost_status":"unavailable","cost_micro_units":null,"currency":null,"source":null,"cache_status":"unavailable"}` {
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
		zero.CacheStatus != UsageUnavailable ||
		zero.InputTokens == nil || *zero.InputTokens != 0 ||
		zero.OutputTokens == nil || *zero.OutputTokens != 0 ||
		zero.CostMicroUnits == nil || *zero.CostMicroUnits != 0 {
		t.Fatalf("zero = %#v", zero)
	}
	body, err := EncodeUsageReceipt(zero)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"token_status":"reported","input_tokens":0,"output_tokens":0,"cost_status":"reported","cost_micro_units":0,"currency":"USD","source":"provider_reported","cache_status":"unavailable"}` {
		t.Fatalf("receipt = %s", body)
	}
	// A provider-reported cache pair surfaces as the canonical reported
	// family with both sides, never as zeros on the token side or absent.
	read := int64(40)
	write := int64(60)
	cached, err := NormalizeUsage(
		&Usage{
			InputTokens:      0,
			OutputTokens:     0,
			CacheReadTokens:  &read,
			CacheWriteTokens: &write,
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if cached.CacheStatus != UsageReported ||
		cached.CacheReadTokens == nil || *cached.CacheReadTokens != 40 ||
		cached.CacheWriteTokens == nil || *cached.CacheWriteTokens != 60 {
		t.Fatalf("cached = %#v", cached)
	}
	cachedBody, err := EncodeUsageReceipt(cached)
	if err != nil {
		t.Fatal(err)
	}
	if string(cachedBody) != `{"token_status":"reported","input_tokens":0,"output_tokens":0,"cost_status":"unavailable","cost_micro_units":null,"currency":null,"source":null,"cache_status":"reported","cache_read_tokens":40,"cache_write_tokens":60}` {
		t.Fatalf("cached receipt = %s", cachedBody)
	}
	// A read-only vocabulary (Gemini, the Responses API) reports the read
	// side alone; the missing write side stays nil instead of becoming zero.
	onlyRead, err := NormalizeUsage(&Usage{
		InputTokens:      5,
		OutputTokens:     5,
		CacheReadTokens:  &read,
		CacheWriteTokens: nil,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if onlyRead.CacheStatus != UsageReported ||
		onlyRead.CacheReadTokens == nil || *onlyRead.CacheReadTokens != 40 ||
		onlyRead.CacheWriteTokens != nil {
		t.Fatalf("read-only cache = %#v", onlyRead)
	}
	readOnlyBody, err := EncodeUsageReceipt(onlyRead)
	if err != nil {
		t.Fatal(err)
	}
	if string(readOnlyBody) != `{"token_status":"reported","input_tokens":5,"output_tokens":5,"cost_status":"unavailable","cost_micro_units":null,"currency":null,"source":null,"cache_status":"reported","cache_read_tokens":40}` {
		t.Fatalf("read-only receipt = %s", readOnlyBody)
	}
	for name, cost := range map[string]CostObservation{
		"negative":  {MicroUnits: -1, Currency: "USD", Source: CostSourceProviderReported},
		"currency":  {MicroUnits: 1, Currency: "usd", Source: CostSourceProviderReported},
		"estimated": {MicroUnits: 1, Currency: "USD", Source: "estimated"},
	} {
		if _, err := NormalizeUsage(
			nil,
			&cost,
		); !IsCode(err, "INVALID_COST_OBSERVATION") {
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
		"reported cache missing values": {
			TokenStatus: UsageUnavailable,
			CostStatus:  UsageUnavailable,
			CacheStatus: UsageReported,
		},
		"unavailable cache with values": {
			TokenStatus:     UsageUnavailable,
			CostStatus:      UsageUnavailable,
			CacheStatus:     UsageUnavailable,
			CacheReadTokens: &one,
		},
		"absent cache with values": {
			TokenStatus:     UsageUnavailable,
			CostStatus:      UsageUnavailable,
			CacheReadTokens: &one,
		},
		"invalid cache status": {
			TokenStatus: UsageUnavailable,
			CostStatus:  UsageUnavailable,
			CacheStatus: Availability("bogus"),
		},
		"negative cache value": {
			TokenStatus:     UsageUnavailable,
			CostStatus:      UsageUnavailable,
			CacheStatus:     UsageReported,
			CacheReadTokens: &one,
			CacheWriteTokens: func() *int64 {
				negative := int64(-1)
				return &negative
			}(),
		},
	} {
		if _, err := EncodeUsageReceipt(receipt); err == nil {
			t.Fatalf("%s receipt was accepted", name)
		}
	}
}
