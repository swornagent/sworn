package driver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
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
	return failContinuation("test.fixture.append_instruction")
}

func (*automationTestConversation) resume(
	[]byte,
	[]providerToolDefinition,
) error {
	return failContinuation("test.fixture.resume")
}

func (*automationTestConversation) declaredReasoningEffort() string { return "" }

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
			_ Limits,
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

// TestValidRecoveryDecisionAndAdvisoryResultCheckCloseTheVocabulary pins
// A2's closed vocabulary of named refusal reasons, in the same idiom as
// TestValidSandboxStartCheckClosesTheVocabulary.
func TestValidRecoveryDecisionAndAdvisoryResultCheckCloseTheVocabulary(t *testing.T) {
	t.Parallel()
	admittedRecovery := []string{
		"recovery_decision.schema_version",
		"recovery_decision.invocation_id",
		"recovery_decision.resume_worker_answer_required",
		"recovery_decision.action_forbids_answer",
		"recovery_decision.action_unknown",
	}
	for _, check := range admittedRecovery {
		if !validRecoveryDecisionCheck(check) {
			t.Fatalf("admitted recovery check rejected: %q", check)
		}
	}
	admittedAdvisory := []string{
		"advisory_result.schema_version",
		"advisory_result.invocation_id",
		"advisory_result.outcome_answer_required",
		"advisory_result.outcome_forbids_answer",
		"advisory_result.outcome_unknown",
	}
	for _, check := range admittedAdvisory {
		if !validAdvisoryResultCheck(check) {
			t.Fatalf("admitted advisory check rejected: %q", check)
		}
	}
	for _, rejected := range []string{
		"", "recovery_decision.", "recovery_decision.unknown",
		"advisory_result.unknown", "RECOVERY_DECISION.SCHEMA_VERSION",
	} {
		if validRecoveryDecisionCheck(rejected) {
			t.Fatalf("unadmitted recovery check accepted: %q", rejected)
		}
		if validAdvisoryResultCheck(rejected) {
			t.Fatalf("unadmitted advisory check accepted: %q", rejected)
		}
	}
	for _, cross := range admittedRecovery {
		if validAdvisoryResultCheck(cross) {
			t.Fatalf("recovery check accepted by advisory vocabulary: %q", cross)
		}
	}
	for _, cross := range admittedAdvisory {
		if validRecoveryDecisionCheck(cross) {
			t.Fatalf("advisory check accepted by recovery vocabulary: %q", cross)
		}
	}
}

func automationSelectedFixture(t *testing.T) (SelectedProfile, ModelSelection) {
	t.Helper()
	adapter := processAdapterFixture(t, "automation-adapter", "sworn.automation.test")
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
	return selected, selection
}

// TestAutomationSelectionMismatchIsNoLongerReportedAsABinding pins A1's
// third collapsed site: a profile/model selection mismatch is not a genuine
// AutomationBinding mismatch and must stop reporting as one.
func TestAutomationSelectionMismatchIsNoLongerReportedAsABinding(t *testing.T) {
	t.Parallel()
	selected, selection := automationSelectedFixture(t)
	recovery := recoveryInvocationFixture(selection)
	recovery.Selection.Model = "wrong-model"
	err := validateAutomationInvocation(AutomationInvocation{
		Selected: selected,
		Recovery: &recovery,
	})
	if !IsCode(err, "AUTOMATION_SELECTION_MISMATCH") {
		t.Fatalf("selection mismatch error = %v", err)
	}
	if IsCode(err, "AUTOMATION_BINDING_MISMATCH") {
		t.Fatalf("selection mismatch still reports as a binding mismatch: %v", err)
	}
}

// TestDecodeAutomationTerminalDistinguishesCollapsedConditions pins A1's
// other two collapsed sites (recovery and advisory): a JSON type-decode
// failure, a shape/rule violation, and an invocation-id echo mismatch must
// report as distinct, accurate codes instead of one shared
// AUTOMATION_BINDING_MISMATCH.
func TestDecodeAutomationTerminalDistinguishesCollapsedConditions(t *testing.T) {
	t.Parallel()
	selected, selection := automationSelectedFixture(t)
	recovery := recoveryInvocationFixture(selection)
	advisory := AdvisoryInvocation{
		SchemaVersion: AdvisoryInvocationSchemaVersion,
		InvocationID:  "advisory-1",
		Binding:       recovery.Binding,
		Selection:     selection,
		Question:      "Which admitted fact answers the worker?",
		Facts:         recovery.Facts,
	}
	answer := recovery.Facts[3].Value

	t.Run("recovery", func(t *testing.T) {
		t.Parallel()
		invocation := AutomationInvocation{Selected: selected, Recovery: &recovery}

		malformed, err := json.Marshal(map[string]any{
			"decision": map[string]any{
				"schema_version": RecoveryDecisionSchemaVersion,
				"invocation_id":  123,
				"action":         "resume_worker",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeAutomationTerminal(
			time.Now(), invocation, malformed, Usage{}, false, 0, 0,
		); !IsCode(err, "INVALID_FIELD") {
			t.Fatalf("malformed invocation_id type error = %v", err)
		}

		ruleViolation, err := json.Marshal(map[string]any{
			"decision": RecoveryDecision{
				SchemaVersion: RecoveryDecisionSchemaVersion,
				InvocationID:  recovery.InvocationID,
				Action:        RecoveryAskCaptain,
				Answer:        &answer,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = decodeAutomationTerminal(
			time.Now(), invocation, ruleViolation, Usage{}, false, 0, 0,
		)
		var contractErr *ContractError
		if !errors.As(err, &contractErr) ||
			contractErr.Code != "INVALID_RECOVERY_DECISION" ||
			contractErr.Detail != "recovery_decision.action_forbids_answer" {
			t.Fatalf("rule-violation error = %v", err)
		}

		mismatched, err := json.Marshal(map[string]any{
			"decision": RecoveryDecision{
				SchemaVersion: RecoveryDecisionSchemaVersion,
				InvocationID:  "not-" + recovery.InvocationID,
				Action:        RecoveryResumeWorker,
				Answer:        &answer,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeAutomationTerminal(
			time.Now(), invocation, mismatched, Usage{}, false, 0, 0,
		); !IsCode(err, "AUTOMATION_INVOCATION_ID_MISMATCH") {
			t.Fatalf("invocation-id mismatch error = %v", err)
		}
	})

	t.Run("advisory", func(t *testing.T) {
		t.Parallel()
		invocation := AutomationInvocation{Selected: selected, Advisory: &advisory}

		malformed, err := json.Marshal(map[string]any{
			"result": map[string]any{
				"schema_version": AdvisoryResultSchemaVersion,
				"invocation_id":  123,
				"outcome":        "answer",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeAutomationTerminal(
			time.Now(), invocation, malformed, Usage{}, false, 0, 0,
		); !IsCode(err, "INVALID_FIELD") {
			t.Fatalf("malformed invocation_id type error = %v", err)
		}

		ruleViolation, err := json.Marshal(map[string]any{
			"result": AdvisoryResult{
				SchemaVersion: AdvisoryResultSchemaVersion,
				InvocationID:  advisory.InvocationID,
				Outcome:       AdvisoryCannotAnswer,
				Answer:        &answer,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = decodeAutomationTerminal(
			time.Now(), invocation, ruleViolation, Usage{}, false, 0, 0,
		)
		var contractErr *ContractError
		if !errors.As(err, &contractErr) ||
			contractErr.Code != "INVALID_ADVISORY_RESULT" ||
			contractErr.Detail != "advisory_result.outcome_forbids_answer" {
			t.Fatalf("rule-violation error = %v", err)
		}

		mismatched, err := json.Marshal(map[string]any{
			"result": AdvisoryResult{
				SchemaVersion: AdvisoryResultSchemaVersion,
				InvocationID:  "not-" + advisory.InvocationID,
				Outcome:       AdvisoryCannotAnswer,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := decodeAutomationTerminal(
			time.Now(), invocation, mismatched, Usage{}, false, 0, 0,
		); !IsCode(err, "AUTOMATION_INVOCATION_ID_MISMATCH") {
			t.Fatalf("invocation-id mismatch error = %v", err)
		}
	})
}

// TestAutomationInvocationIDMismatchReportsIdenticallyThroughDecodeAndObservation
// pins A3: the identity-echo condition must report the same way from
// decodeAutomationTerminal and from ValidateAutomationObservation's
// duplicate post-hoc check, for both the recovery and advisory branches.
func TestAutomationInvocationIDMismatchReportsIdenticallyThroughDecodeAndObservation(
	t *testing.T,
) {
	t.Parallel()
	selected, selection := automationSelectedFixture(t)
	recovery := recoveryInvocationFixture(selection)
	answer := recovery.Facts[3].Value

	t.Run("recovery", func(t *testing.T) {
		t.Parallel()
		invocation := AutomationInvocation{Selected: selected, Recovery: &recovery}
		decision := RecoveryDecision{
			SchemaVersion: RecoveryDecisionSchemaVersion,
			InvocationID:  "not-" + recovery.InvocationID,
			Action:        RecoveryResumeWorker,
			Answer:        &answer,
		}
		arguments, err := json.Marshal(map[string]any{"decision": decision})
		if err != nil {
			t.Fatal(err)
		}
		_, decodeErr := decodeAutomationTerminal(
			time.Now(), invocation, arguments, Usage{}, false, 0, 0,
		)
		observationErr := ValidateAutomationObservation(invocation, AutomationObservation{
			TransportStatus: Completed,
			Usage: UsageReceipt{
				TokenStatus: UsageUnavailable, CostStatus: UsageUnavailable,
			},
			Diagnostic: Diagnostic{Code: "none"},
			Recovery:   &decision,
		})
		if !IsCode(decodeErr, "AUTOMATION_INVOCATION_ID_MISMATCH") ||
			!IsCode(observationErr, "AUTOMATION_INVOCATION_ID_MISMATCH") {
			t.Fatalf(
				"decode error = %v, observation error = %v",
				decodeErr, observationErr,
			)
		}
	})

	t.Run("advisory", func(t *testing.T) {
		t.Parallel()
		advisory := AdvisoryInvocation{
			SchemaVersion: AdvisoryInvocationSchemaVersion,
			InvocationID:  "advisory-1",
			Binding:       recovery.Binding,
			Selection:     selection,
			Question:      "Which admitted fact answers the worker?",
			Facts:         recovery.Facts,
		}
		invocation := AutomationInvocation{Selected: selected, Advisory: &advisory}
		result := AdvisoryResult{
			SchemaVersion: AdvisoryResultSchemaVersion,
			InvocationID:  "not-" + advisory.InvocationID,
			Outcome:       AdvisoryCannotAnswer,
		}
		arguments, err := json.Marshal(map[string]any{"result": result})
		if err != nil {
			t.Fatal(err)
		}
		_, decodeErr := decodeAutomationTerminal(
			time.Now(), invocation, arguments, Usage{}, false, 0, 0,
		)
		observationErr := ValidateAutomationObservation(invocation, AutomationObservation{
			TransportStatus: Completed,
			Usage: UsageReceipt{
				TokenStatus: UsageUnavailable, CostStatus: UsageUnavailable,
			},
			Diagnostic: Diagnostic{Code: "none"},
			Advisory:   &result,
		})
		if !IsCode(decodeErr, "AUTOMATION_INVOCATION_ID_MISMATCH") ||
			!IsCode(observationErr, "AUTOMATION_INVOCATION_ID_MISMATCH") {
			t.Fatalf(
				"decode error = %v, observation error = %v",
				decodeErr, observationErr,
			)
		}
	})
}

// TestAutomationRecoveryRefusalNamesTheViolatedRule pins A2: the tool-result
// feedback a recovery worker receives names the violated rule (here,
// ask_captain carrying an answer, the exact sworn#250 shape), not a bare
// unnamed code.
func TestAutomationRecoveryRefusalNamesTheViolatedRule(t *testing.T) {
	t.Parallel()
	selection := ModelSelection{Profile: "automation", Model: "small-model"}
	corrected := recoveryInvocationFixture(selection)

	violatingAnswer := "unearned answer"
	violating, err := json.Marshal(map[string]any{
		"decision": RecoveryDecision{
			SchemaVersion: RecoveryDecisionSchemaVersion,
			InvocationID:  corrected.InvocationID,
			Action:        RecoveryAskCaptain,
			Answer:        &violatingAnswer,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	correctedAnswer := corrected.Facts[3].Value
	valid, err := json.Marshal(map[string]any{
		"decision": RecoveryDecision{
			SchemaVersion: RecoveryDecisionSchemaVersion,
			InvocationID:  corrected.InvocationID,
			Action:        RecoveryResumeWorker,
			Answer:        &correctedAnswer,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	conversation := &automationTestConversation{calls: [][]providerToolCall{
		{{ID: "decision-1", Name: "sworn_recovery_decide", Arguments: violating}},
		{{ID: "decision-2", Name: "sworn_recovery_decide", Arguments: valid}},
	}}
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
			_ Limits,
		) (providerConversation, error) {
			conversation.definitions = append(
				[]providerToolDefinition(nil),
				definitions...,
			)
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
	selected, err := registry.ResolveSelection(selection)
	if err != nil {
		t.Fatal(err)
	}
	invocation := AutomationInvocation{Selected: selected, Recovery: &corrected}
	observation, err := (Dispatcher{}).InvokeAutomation(
		context.Background(),
		invocation,
	)
	if err != nil || observation.Recovery == nil ||
		observation.Recovery.Action != RecoveryResumeWorker {
		t.Fatalf("observation = %#v, error = %v", observation, err)
	}
	if len(conversation.results) != 1 || len(conversation.results[0]) != 1 {
		t.Fatalf("fed-back results = %#v", conversation.results)
	}
	want := "error:INVALID_RECOVERY_DECISION detail=recovery_decision.action_forbids_answer"
	if got := string(conversation.results[0][0].Content); got != want {
		t.Fatalf("feedback = %q, want %q", got, want)
	}
	if !conversation.results[0][0].Failed {
		t.Fatal("feedback not marked failed")
	}
}
