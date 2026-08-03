package driver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func automationBindingFixture() AutomationBinding {
	return AutomationBinding{
		RunID:                 "run-1",
		TrackID:               "track-1",
		Slice:                 "W4-turn-recovery",
		BatonAttempt:          1,
		PlanAuthorityDigest:   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		TargetAuthorityDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		WorkIdentity:          "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		ProgressIdentity:      "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
	}
}

func recoveryInvocationFixture(selection ModelSelection) RecoveryInvocation {
	return RecoveryInvocation{
		SchemaVersion: RecoveryInvocationSchemaVersion,
		InvocationID:  "recovery-1",
		Binding:       automationBindingFixture(),
		Selection:     selection,
		Facts: []AutomationFact{
			{Name: FactWorkerTerminal, Value: "question"},
			{Name: FactWorkerMessage, Value: "Which admitted base should I use?"},
			{Name: FactCurrentStatus, Value: "no sealed submission"},
			{
				Name:  FactProgressSummary,
				Value: "Use the exact admitted prepared base.",
			},
		},
	}
}

func TestModelSelectionResolvesOneExactRegisteredAdapter(t *testing.T) {
	t.Parallel()
	adapter := processAdapterFixture(t, "exact-adapter", "sworn.exact")
	registry, err := NewSelectionRegistry(
		[]ProfileConfig{{
			Key: "exact-profile", Adapter: "exact-adapter", Network: NetworkNone,
		}},
		[]Adapter{adapter},
	)
	if err != nil {
		t.Fatal(err)
	}
	selection := ModelSelection{Profile: "exact-profile", Model: "exact-model"}
	selected, err := registry.ResolveSelection(selection)
	if err != nil ||
		selected.Profile.Key != selection.Profile ||
		selected.Model != selection.Model ||
		selected.Adapter != adapter.Identity() {
		t.Fatalf("selection = %#v, error = %v", selected, err)
	}
	if _, err := registry.ResolveSelection(ModelSelection{
		Profile: "missing", Model: "exact-model",
	}); !IsCode(err, "UNKNOWN_PROFILE") {
		t.Fatalf("missing profile error = %v", err)
	}
	if _, err := registry.ResolveSelection(ModelSelection{
		Profile: "exact-profile",
	}); !IsCode(err, "INVALID_MODEL") {
		t.Fatalf("implicit model error = %v", err)
	}
}

func TestAutomationContractsAreClosedBoundedAndNonBaton(t *testing.T) {
	t.Parallel()
	selection := ModelSelection{Profile: "automation", Model: "small-model"}
	recovery := recoveryInvocationFixture(selection)
	body, err := EncodeRecoveryInvocation(recovery)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRecoveryInvocation(body)
	if err != nil || decoded.SchemaVersion != recovery.SchemaVersion ||
		decoded.InvocationID != recovery.InvocationID ||
		decoded.Binding != recovery.Binding ||
		decoded.Selection != recovery.Selection ||
		len(decoded.Facts) != len(recovery.Facts) {
		t.Fatalf("decoded = %#v, error = %v", decoded, err)
	}
	for _, forbidden := range []string{
		`"role"`, `"responsibility"`, `"workspace"`, `"ref"`,
		`"submission"`, `"permission"`, `"merge"`, `"shell"`,
	} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("recovery invocation exposes %s: %s", forbidden, body)
		}
	}
	unknown := []byte(strings.Replace(
		string(body),
		`"schema_version":"sworn.recovery-invocation/v1"`,
		`"schema_version":"sworn.recovery-invocation/v1","role":"captain"`,
		1,
	))
	if _, err := DecodeRecoveryInvocation(unknown); !IsCode(err, "UNKNOWN_FIELD") {
		t.Fatalf("role field error = %v", err)
	}

	answer := "Use the already admitted prepared base."
	decision := RecoveryDecision{
		SchemaVersion: RecoveryDecisionSchemaVersion,
		InvocationID:  recovery.InvocationID,
		Action:        RecoveryResumeWorker,
		Answer:        &answer,
	}
	decisionBody, err := EncodeRecoveryDecision(decision)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := DecodeRecoveryDecision(decisionBody); err != nil ||
		decoded.Action != RecoveryResumeWorker ||
		decoded.Answer == nil || *decoded.Answer != answer {
		t.Fatalf("decision = %#v, error = %v", decoded, err)
	}
	decision.Action = RecoveryAskCaptain
	if err := ValidateRecoveryDecision(decision); !IsCode(
		err,
		"INVALID_RECOVERY_DECISION",
	) {
		t.Fatalf("answer-bearing Captain action error = %v", err)
	}

	advisory := AdvisoryInvocation{
		SchemaVersion: AdvisoryInvocationSchemaVersion,
		InvocationID:  "advisory-1",
		Binding:       recovery.Binding,
		Selection:     selection,
		Question:      "Which admitted fact answers the worker?",
		Facts:         recovery.Facts,
	}
	advisoryBody, err := EncodeAdvisoryInvocation(advisory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeAdvisoryInvocation(advisoryBody); err != nil {
		t.Fatal(err)
	}
	result := AdvisoryResult{
		SchemaVersion: AdvisoryResultSchemaVersion,
		InvocationID:  advisory.InvocationID,
		Outcome:       AdvisoryCannotAnswer,
	}
	resultBody, err := EncodeAdvisoryResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeAdvisoryResult(resultBody); err != nil {
		t.Fatal(err)
	}
	result.Answer = &answer
	if err := ValidateAdvisoryResult(result); !IsCode(
		err,
		"INVALID_ADVISORY_RESULT",
	) {
		t.Fatalf("cannot-answer payload error = %v", err)
	}
}

func TestRecoveryResumeAnswerRequiresExactInvocationFact(t *testing.T) {
	t.Parallel()
	adapter := processAdapterFixture(
		t,
		"automation-adapter",
		"sworn.automation.test",
	)
	selection := ModelSelection{Profile: "automation", Model: "small-model"}
	registry, err := NewSelectionRegistry(
		[]ProfileConfig{{
			Key:     selection.Profile,
			Adapter: adapter.Identity().Key,
			Network: NetworkNone,
		}},
		[]Adapter{adapter},
	)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := registry.ResolveSelection(selection)
	if err != nil {
		t.Fatal(err)
	}
	recovery := recoveryInvocationFixture(selection)
	invocation := AutomationInvocation{
		Selected: selected,
		Recovery: &recovery,
	}

	observationFor := func(answer string) AutomationObservation {
		return AutomationObservation{
			TransportStatus: Completed,
			Usage: UsageReceipt{
				TokenStatus: UsageUnavailable,
				CostStatus:  UsageUnavailable,
			},
			Diagnostic: Diagnostic{Code: "none"},
			Recovery: &RecoveryDecision{
				SchemaVersion: RecoveryDecisionSchemaVersion,
				InvocationID:  recovery.InvocationID,
				Action:        RecoveryResumeWorker,
				Answer:        &answer,
			},
		}
	}
	exact := recovery.Facts[3].Value
	observation := observationFor(exact)
	if err := ValidateAutomationObservation(
		invocation,
		observation,
	); err != nil {
		t.Fatalf("exact fact observation = %v", err)
	}
	matched, err := RecoveryAnswerForInvocation(
		recovery,
		*observation.Recovery,
	)
	if err != nil || matched != recovery.Facts[3].Value {
		t.Fatalf("matched answer = %q, %v", matched, err)
	}

	for _, test := range []struct {
		name   string
		answer string
	}{
		{
			name:   "transformed eligible fact",
			answer: recovery.Facts[2].Value + ".",
		},
		{
			name:   "invented answer",
			answer: "Use a newly inferred prepared base.",
		},
		{
			name:   "worker message is context only",
			answer: recovery.Facts[1].Value,
		},
		{
			name:   "worker terminal is context only",
			answer: recovery.Facts[0].Value,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateAutomationObservation(
				invocation,
				observationFor(test.answer),
			); !IsCode(err, "INVALID_AUTOMATION_OBSERVATION") {
				t.Fatalf("provenance error = %v", err)
			}
		})
	}
}

type automationTestConversation struct {
	calls       [][]providerToolCall
	index       int
	definitions []providerToolDefinition
	results     [][]providerToolResult
}

func (conversation *automationTestConversation) request() (providerRequest, error) {
	return providerRequest{Body: []byte(`{"test":true}`)}, nil
}

func (conversation *automationTestConversation) accept([]byte) (providerTurn, error) {
	calls := conversation.calls[conversation.index]
	conversation.index++
	return providerTurn{Calls: calls, Usage: &Usage{
		InputTokens: 2, OutputTokens: 1,
	}}, nil
}

func (conversation *automationTestConversation) appendResults(
	results []providerToolResult,
) error {
	conversation.results = append(conversation.results, results)
	return nil
}

func (*automationTestConversation) appendInstruction([]byte) error {
	return fail("CONTINUATION_INVALID")
}

func (*automationTestConversation) resume(
	[]byte,
	[]providerToolDefinition,
) error {
	return fail("CONTINUATION_INVALID")
}

func (*automationTestConversation) close() {}

type automationTestTransport struct{}

func (automationTestTransport) roundTrip(
	context.Context,
	*string,
	providerRequest,
) ([]byte, error) {
	return []byte(`{"ok":true}`), nil
}

func (automationTestTransport) check(
	context.Context,
	profileCheckKind,
	*string,
	string,
) (ReadinessState, string) {
	return ReadinessPass, "test"
}

func TestProviderAutomationUsesOneRoleNeutralTerminalWithoutSubmissionAuthority(
	t *testing.T,
) {
	t.Parallel()
	answer := "Use the exact admitted prepared base."
	arguments, err := json.Marshal(map[string]any{
		"decision": RecoveryDecision{
			SchemaVersion: RecoveryDecisionSchemaVersion,
			InvocationID:  "recovery-1",
			Action:        RecoveryResumeWorker,
			Answer:        &answer,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := &automationTestConversation{calls: [][]providerToolCall{{{
		ID: "decision-1", Name: "sworn_recovery_decide", Arguments: arguments,
	}}}}
	var prompt []byte
	adapter, err := newLoopAdapter(
		"automation-adapter",
		"sworn.automation.test",
		"1.0.0",
		ProfileOpenAIHTTP,
		ProfileSurfaceOpenAIChat,
		providerDialectOpenAIChat,
		map[string]string{"fixture": "automation"},
		func(
			body []byte,
			model string,
			definitions []providerToolDefinition,
		) (providerConversation, error) {
			prompt = append([]byte(nil), body...)
			conversation.definitions = append(
				[]providerToolDefinition(nil),
				definitions...,
			)
			if model != "small-model" {
				t.Fatalf("model = %q", model)
			}
			return conversation, nil
		},
		automationTestTransport{},
	)
	if err != nil {
		t.Fatal(err)
	}
	credentialRef := "automation-credential"
	registry, err := NewSelectionRegistry(
		[]ProfileConfig{{
			Key: "automation", Adapter: "automation-adapter",
			Network: NetworkRequired, CredentialRef: &credentialRef,
		}},
		[]Adapter{adapter},
	)
	if err != nil {
		t.Fatal(err)
	}
	selection := ModelSelection{Profile: "automation", Model: "small-model"}
	selected, err := registry.ResolveSelection(selection)
	if err != nil {
		t.Fatal(err)
	}
	invocation := AutomationInvocation{
		Selected: selected,
		Recovery: pointerTo(recoveryInvocationFixture(selection)),
	}
	observation, err := (Dispatcher{}).InvokeAutomation(
		context.Background(),
		invocation,
	)
	if err != nil || observation.Recovery == nil ||
		observation.Advisory != nil ||
		observation.Recovery.Action != RecoveryResumeWorker ||
		observation.Usage.InputTokens == nil ||
		*observation.Usage.InputTokens != 2 {
		t.Fatalf("observation = %#v, error = %v", observation, err)
	}
	if len(conversation.definitions) != 1 ||
		conversation.definitions[0].Name != "sworn_recovery_decide" {
		t.Fatalf("automation tools = %#v", conversation.definitions)
	}
	surface, _ := json.Marshal(conversation.definitions)
	for _, forbidden := range []string{
		"sworn_submit", "sworn_yield", "Bash", "Read", "Write", "Edit",
		"Glob", "Grep",
	} {
		if strings.Contains(string(surface), forbidden) {
			t.Fatalf("automation surface exposes %q: %s", forbidden, surface)
		}
	}
	if strings.Contains(string(prompt), `"role"`) ||
		strings.Contains(string(prompt), `"responsibility"`) ||
		strings.Contains(string(prompt), `"workspace"`) {
		t.Fatalf("automation prompt exposes Baton authority: %s", prompt)
	}
	for _, required := range []string{
		"byte-for-byte",
		"worker_terminal and worker_message are context only",
		"Use ask_captain for judgment",
		"pause_track_for_human when uncertain",
	} {
		if !strings.Contains(string(prompt), required) {
			t.Fatalf("automation prompt omits %q: %s", required, prompt)
		}
	}
}

func TestNativeRuntimeDoesNotRequirePreflightCertificate(t *testing.T) {
	t.Parallel()
	if _, admitted := any((*nativeAdapter)(nil)).(automationAdapter); !admitted {
		t.Fatal("native adapter does not expose its certified automation surface")
	}
	ref := "native-credential"
	identity := AdapterIdentity{
		Key:                 "native-closed",
		ID:                  "sworn.native.closed",
		Version:             "1.0.0",
		ConfigurationDigest: Digest([]byte("native-closed")),
	}
	native := &nativeAdapter{
		identity: identity,
		refs:     map[string]struct{}{ref: {}},
		resolve: func(context.Context, string) (string, error) {
			return "/not-reached", nil
		},
	}
	nativeProfile := ProfileConfig{
		Key:           "native-profile",
		Adapter:       identity.Key,
		Network:       NetworkRequired,
		CredentialRef: &ref,
	}
	nativeSelected := SelectedProfile{
		Profile: nativeProfile,
		Adapter: identity,
		Model:   "native-model",
		adapter: native,
	}
	nativeSelection := ModelSelection{
		Profile: nativeProfile.Key,
		Model:   nativeSelected.Model,
	}
	certificate, credentialPath, err := native.nativeRuntime(
		context.Background(),
		Invocation{Selected: nativeSelected},
	)
	if err != nil || hasNativeSurfaceCertificate(certificate) ||
		credentialPath != "/not-reached" {
		t.Fatalf(
			"native runtime = certificate %#v, path %q, error %v",
			certificate,
			credentialPath,
			err,
		)
	}
	automationCertificate, credentialPath, err := native.nativeAutomationRuntime(
		context.Background(),
		AutomationInvocation{
			Selected: nativeSelected,
			Recovery: pointerTo(
				recoveryInvocationFixture(nativeSelection),
			),
		},
	)
	if err != nil ||
		hasNativeAutomationSurfaceCertificate(automationCertificate) ||
		credentialPath != "/not-reached" {
		t.Fatalf(
			"native automation runtime = certificate %#v, path %q, error %v",
			automationCertificate,
			credentialPath,
			err,
		)
	}

	adapter := processAdapterFixture(t, "closed-adapter", "sworn.closed")
	registry, err := NewSelectionRegistry(
		[]ProfileConfig{{
			Key: "closed-profile", Adapter: "closed-adapter", Network: NetworkNone,
		}},
		[]Adapter{adapter},
	)
	if err != nil {
		t.Fatal(err)
	}
	selection := ModelSelection{
		Profile: "closed-profile",
		Model:   "closed-model",
	}
	selected, err := registry.ResolveSelection(selection)
	if err != nil {
		t.Fatal(err)
	}
	_, err = (Dispatcher{}).InvokeAutomation(
		context.Background(),
		AutomationInvocation{
			Selected: selected,
			Recovery: pointerTo(recoveryInvocationFixture(selection)),
		},
	)
	if !IsCode(err, "AUTOMATION_UNSUPPORTED") {
		t.Fatalf("uncertified automation error = %v", err)
	}
}

func pointerTo[T any](value T) *T { return &value }
