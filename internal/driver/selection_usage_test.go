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
	assertObservationOmitsExceptOriginalCode(t, observation, sentinel)
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
	assertObservationOmitsExceptOriginalCode(t, observation, sentinel)
}

// TestSanitizeFailedObservationPreservesBoundedNonAdmittedCode pins A1's
// bounded evidence and A2's sanitization marker directly at the sanitizer,
// including C4's re-validation-drop distinction from honest absence.
func TestSanitizeFailedObservationPreservesBoundedNonAdmittedCode(t *testing.T) {
	t.Parallel()
	const surface = "sworn.test"

	benign := failureObservation("benign_adapter_code", surface)
	sanitized := sanitizeFailedObservation(benign, surface)
	if !sanitized.Diagnostic.Sanitized ||
		sanitized.Diagnostic.Code != "adapter_failed" ||
		sanitized.Diagnostic.OriginalCode != "benign_adapter_code" ||
		sanitized.Diagnostic.OriginalCodeDropped {
		t.Fatalf("benign preservation = %#v", sanitized.Diagnostic)
	}

	oversize := failureObservation(strings.Repeat("a", 65), surface)
	sanitized = sanitizeFailedObservation(oversize, surface)
	if !sanitized.Diagnostic.Sanitized ||
		sanitized.Diagnostic.OriginalCode != "" ||
		!sanitized.Diagnostic.OriginalCodeDropped {
		t.Fatalf("oversize code drop = %#v", sanitized.Diagnostic)
	}

	controlBearing := failureObservation("bad\ncode", surface)
	sanitized = sanitizeFailedObservation(controlBearing, surface)
	if !sanitized.Diagnostic.Sanitized ||
		sanitized.Diagnostic.OriginalCode != "" ||
		!sanitized.Diagnostic.OriginalCodeDropped {
		t.Fatalf("control-character code drop = %#v", sanitized.Diagnostic)
	}

	absent := failureObservation("", surface)
	sanitized = sanitizeFailedObservation(absent, surface)
	if !sanitized.Diagnostic.Sanitized ||
		sanitized.Diagnostic.OriginalCode != "" ||
		sanitized.Diagnostic.OriginalCodeDropped {
		t.Fatalf("honest absence = %#v", sanitized.Diagnostic)
	}

	admittedWithHostileEvents := failureObservation("process_failed", surface)
	admittedWithHostileEvents.Diagnostic.StderrBytes = 12
	admittedWithHostileEvents.Events = []TerminalEvent{{Sequence: 1, Kind: "not-a-real-kind"}}
	sanitized = sanitizeFailedObservation(admittedWithHostileEvents, surface)
	if !sanitized.Diagnostic.Sanitized ||
		sanitized.Diagnostic.Code != "process_failed" ||
		sanitized.Diagnostic.StderrBytes != 12 ||
		sanitized.Diagnostic.OriginalCode != "" ||
		sanitized.Diagnostic.OriginalCodeDropped ||
		sanitized.Events != nil {
		t.Fatalf("admitted code survives hostile event wipe: diagnostic=%#v events=%#v", sanitized.Diagnostic, sanitized.Events)
	}
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

// assertObservationOmitsExceptOriginalCode is assertObservationOmits
// narrowed, by exact whole-field equality rather than a general substring
// allowance, for the one sanitizer-owned field A1's preservation mechanism
// legitimately carries a bounded adapter-chosen code into: it pins that
// Diagnostic.OriginalCode is exactly value, then asserts value appears
// nowhere else in the observation.
func assertObservationOmitsExceptOriginalCode(t *testing.T, observation Observation, value string) {
	t.Helper()
	if observation.Diagnostic.OriginalCode != value {
		t.Fatalf("observation.Diagnostic.OriginalCode = %q, want %q", observation.Diagnostic.OriginalCode, value)
	}
	scrubbed := observation
	scrubbed.Diagnostic.OriginalCode = ""
	assertObservationOmits(t, scrubbed, value)
}

func TestUsageNormalizationPreservesZeroAndUnavailable(t *testing.T) {
	t.Parallel()
	const surface = "sworn.test"
	unavailable, err := NormalizeUsage(nil, nil, surface)
	if err != nil {
		t.Fatal(err)
	}
	if unavailable.TokenStatus != UsageUnavailable ||
		unavailable.CostStatus != UsageUnavailable ||
		unavailable.CacheStatus != UsageUnavailable ||
		unavailable.SchemaVersion != UsageSchemaV2 ||
		unavailable.Surface != surface ||
		unavailable.UnavailableReason != UsageReasonWireLacked ||
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
	if string(unavailableBody) != `{"token_status":"unavailable","input_tokens":null,"output_tokens":null,"cost_status":"unavailable","cost_micro_units":null,"currency":null,"source":null,"cache_status":"unavailable","schema_version":"sworn.usage/v2","surface":"sworn.test","unavailable_reason":"wire-lacked-usage"}` {
		t.Fatalf("unavailable receipt = %s", unavailableBody)
	}
	zero, err := NormalizeUsage(
		&Usage{InputTokens: 0, OutputTokens: 0},
		&CostObservation{
			MicroUnits: 0,
			Currency:   "USD",
			Source:     CostSourceProviderReported,
		},
		surface,
	)
	if err != nil {
		t.Fatal(err)
	}
	if zero.TokenStatus != UsageReported || zero.CostStatus != UsageReported ||
		zero.CacheStatus != UsageUnavailable ||
		zero.SchemaVersion != UsageSchemaV2 ||
		zero.Surface != surface || zero.UnavailableReason != "" ||
		zero.InputTokens == nil || *zero.InputTokens != 0 ||
		zero.OutputTokens == nil || *zero.OutputTokens != 0 ||
		zero.CostMicroUnits == nil || *zero.CostMicroUnits != 0 {
		t.Fatalf("zero = %#v", zero)
	}
	body, err := EncodeUsageReceipt(zero)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"token_status":"reported","input_tokens":0,"output_tokens":0,"cost_status":"reported","cost_micro_units":0,"currency":"USD","source":"provider_reported","cache_status":"unavailable","schema_version":"sworn.usage/v2","surface":"sworn.test"}` {
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
		surface,
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
	if string(cachedBody) != `{"token_status":"reported","input_tokens":0,"output_tokens":0,"cost_status":"unavailable","cost_micro_units":null,"currency":null,"source":null,"cache_status":"reported","cache_read_tokens":40,"cache_write_tokens":60,"schema_version":"sworn.usage/v2","surface":"sworn.test"}` {
		t.Fatalf("cached receipt = %s", cachedBody)
	}
	// A read-only vocabulary (Gemini, the Responses API) reports the read
	// side alone; the missing write side stays nil instead of becoming zero.
	onlyRead, err := NormalizeUsage(&Usage{
		InputTokens:      5,
		OutputTokens:     5,
		CacheReadTokens:  &read,
		CacheWriteTokens: nil,
	}, nil, surface)
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
	if string(readOnlyBody) != `{"token_status":"reported","input_tokens":5,"output_tokens":5,"cost_status":"unavailable","cost_micro_units":null,"currency":null,"source":null,"cache_status":"reported","cache_read_tokens":40,"schema_version":"sworn.usage/v2","surface":"sworn.test"}` {
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
			surface,
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

// A2: an unavailable receipt is loud, never a silent default. The failure
// observation paths name the surface and a stable reason, a silent v2
// receipt is unencodable, and a legacy v1 blob still decodes and re-encodes
// byte-identically so old journals stay readable.
func TestUnavailableReceiptsAreLoudAndLegacyBlobsStayByteIdentical(t *testing.T) {
	t.Parallel()
	loud, err := UnavailableReceipt("sworn.test", UsageReasonCaptureFailed)
	if err != nil || loud.SchemaVersion != UsageSchemaV2 ||
		loud.Surface != "sworn.test" ||
		loud.UnavailableReason != UsageReasonCaptureFailed ||
		loud.TokenStatus != UsageUnavailable {
		t.Fatalf("loud receipt = %#v, %v", loud, err)
	}
	if _, err := EncodeUsageReceipt(loud); err != nil {
		t.Fatalf("loud receipt encode = %v", err)
	}

	failed := failureObservation("adapter_failed", "sworn.test")
	if failed.Usage.SchemaVersion != UsageSchemaV2 ||
		failed.Usage.Surface != "sworn.test" ||
		failed.Usage.UnavailableReason != UsageReasonCaptureFailed ||
		failed.Usage.TokenStatus != UsageUnavailable {
		t.Fatalf("failure observation = %#v", failed)
	}
	sanitized := sanitizeFailedObservation(failed, "sworn.test")
	if sanitized.Usage.Surface != "sworn.test" ||
		sanitized.Usage.UnavailableReason != UsageReasonCaptureFailed {
		t.Fatalf("sanitized observation = %#v", sanitized)
	}

	// A silent unavailable v2 receipt is unencodable, hence unjournalable.
	for name, receipt := range map[string]UsageReceipt{
		"missing surface": {
			SchemaVersion:     UsageSchemaV2,
			TokenStatus:       UsageUnavailable,
			CostStatus:        UsageUnavailable,
			CacheStatus:       UsageUnavailable,
			UnavailableReason: UsageReasonCaptureFailed,
		},
		"missing reason": {
			SchemaVersion: UsageSchemaV2,
			Surface:       "sworn.test",
			TokenStatus:   UsageUnavailable,
			CostStatus:    UsageUnavailable,
			CacheStatus:   UsageUnavailable,
		},
		"wrong schema version": {
			SchemaVersion:     "sworn.usage/v9",
			Surface:           "sworn.test",
			TokenStatus:       UsageUnavailable,
			CostStatus:        UsageUnavailable,
			CacheStatus:       UsageUnavailable,
			UnavailableReason: UsageReasonWireLacked,
		},
		"reported with reason": {
			SchemaVersion:     UsageSchemaV2,
			Surface:           "sworn.test",
			TokenStatus:       UsageReported,
			InputTokens:       int64Pointer(1),
			OutputTokens:      int64Pointer(1),
			CostStatus:        UsageUnavailable,
			CacheStatus:       UsageUnavailable,
			UnavailableReason: UsageReasonWireLacked,
		},
	} {
		if _, err := EncodeUsageReceipt(receipt); err == nil {
			t.Fatalf("%s receipt was accepted", name)
		}
	}

	// Legacy v1 blobs keep today's exact bytes on decode and re-encode.
	legacy := []byte(
		`{"token_status":"unavailable","input_tokens":null,` +
			`"output_tokens":null,"cost_status":"unavailable",` +
			`"cost_micro_units":null,"currency":null,"source":null}`,
	)
	var decoded UsageReceipt
	if json.Unmarshal(legacy, &decoded) != nil {
		t.Fatal("legacy decode failed")
	}
	reencoded, err := EncodeUsageReceipt(decoded)
	if err != nil || !bytes.Equal(reencoded, legacy) {
		t.Fatalf("legacy re-encode = %s, %v", reencoded, err)
	}
}

// Correction 2: the field-wise emptiness predicate preserves the exact
// zero-receipt semantics the fresh-rehydrate detection depends on, including
// the new fields, now that the receipt carries a slice and is no longer
// comparable.
func TestUsageReceiptZeroPreservesFreshRehydrateSemantics(t *testing.T) {
	t.Parallel()
	if !(UsageReceipt{}).Zero() {
		t.Fatal("zero receipt is not zero")
	}
	variants := []UsageReceipt{
		{TokenStatus: UsageUnavailable},
		{CacheStatus: UsageUnavailable},
		{SchemaVersion: UsageSchemaV2},
		{Surface: "sworn.test"},
		{UnavailableReason: UsageReasonCaptureFailed},
		{Turns: int64Pointer(1)},
		{ToolCalls: int64Pointer(1)},
		{ToolCallsByName: []ToolCallCount{{Name: "Bash", Count: 1}}},
		{DurationMillis: int64Pointer(1)},
		{Profile: textPointerForTest("p")},
		{Model: textPointerForTest("m")},
	}
	for _, receipt := range variants {
		if receipt.Zero() {
			t.Fatalf("receipt %#v reported zero", receipt)
		}
	}
}

func textPointerForTest(value string) *string { return &value }

func int64Pointer(value int64) *int64 { return &value }

// A2's usage-preservation gate: the executed-binary digest survives
// sanitizeFailedObservation independent of preservesUsageDiagnostic, so a
// post-closure native failure never loses attestation of what actually ran
// even for a diagnostic code the failure-usage gate does not otherwise
// preserve.
func TestSanitizeFailedObservationPreservesExecutedDigestForAnyDiagnosticCode(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + string(bytesRepeatForTest("d", 64))
	raw := Observation{
		Diagnostic: Diagnostic{Code: "process_failed"},
		Usage:      UsageReceipt{ExecutedDigest: &digest},
	}
	sanitized := sanitizeFailedObservation(raw, "sworn.test")
	if sanitized.Usage.ExecutedDigest == nil ||
		*sanitized.Usage.ExecutedDigest != digest {
		t.Fatalf(
			"sanitized executed digest = %#v, want %s",
			sanitized.Usage.ExecutedDigest, digest,
		)
	}
	if sanitized.Usage.Surface != "sworn.test" ||
		sanitized.Usage.TokenStatus != UsageUnavailable {
		t.Fatalf("sanitized usage shape unexpectedly changed: %#v", sanitized.Usage)
	}

	malformed := "not-a-digest"
	rawMalformed := Observation{
		Diagnostic: Diagnostic{Code: "process_failed"},
		Usage:      UsageReceipt{ExecutedDigest: &malformed},
	}
	if sanitized := sanitizeFailedObservation(rawMalformed, "sworn.test"); sanitized.Usage.ExecutedDigest != nil {
		t.Fatalf(
			"a malformed executed digest was preserved: %#v",
			sanitized.Usage.ExecutedDigest,
		)
	}
}

func bytesRepeatForTest(pattern string, count int) []byte {
	body := make([]byte, 0, count)
	for len(body) < count {
		body = append(body, pattern...)
	}
	return body[:count]
}
