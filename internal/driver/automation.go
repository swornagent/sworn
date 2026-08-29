package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	RecoveryInvocationSchemaVersion = "sworn.recovery-invocation/v1"
	RecoveryDecisionSchemaVersion   = "sworn.recovery-decision/v1"
	AdvisoryInvocationSchemaVersion = "sworn.advisory-invocation/v1"
	AdvisoryResultSchemaVersion     = "sworn.advisory-result/v1"

	MaxAutomationBytes        = 65_536
	MaxAutomationFacts        = 8
	MaxAutomationFactBytes    = 4_096
	MaxAutomationMessageBytes = 8_192
	MaxAutomationTurns        = 4
	MaxAutomationCorrections  = 2
)

const swornRecoveryDecisionInputSchema = `{"type":"object","properties":{"decision":{"type":"object","properties":{"schema_version":{"type":"string","enum":["sworn.recovery-decision/v1"]},"invocation_id":{"type":"string"},"action":{"type":"string","enum":["resume_worker","ask_captain","retry_operationally","pause_track_for_human"]},"answer":{"type":"string"}},"required":["schema_version","invocation_id","action"],"additionalProperties":false}},"required":["decision"],"additionalProperties":false}`
const swornAdvisoryResultInputSchema = `{"type":"object","properties":{"result":{"type":"object","properties":{"schema_version":{"type":"string","enum":["sworn.advisory-result/v1"]},"invocation_id":{"type":"string"},"outcome":{"type":"string","enum":["answer","cannot_answer"]},"answer":{"type":"string"}},"required":["schema_version","invocation_id","outcome"],"additionalProperties":false}},"required":["result"],"additionalProperties":false}`

// AutomationBinding is exact engine authority expressed only as opaque
// identities. It intentionally contains no Git ref, workspace, Baton role,
// responsibility, submission permission, or tool authority.
type AutomationBinding struct {
	RunID                 string `json:"run_id"`
	TrackID               string `json:"track_id"`
	Slice                 string `json:"slice"`
	BatonAttempt          int64  `json:"baton_attempt"`
	PlanAuthorityDigest   string `json:"plan_authority_digest"`
	TargetAuthorityDigest string `json:"target_authority_digest"`
	WorkIdentity          string `json:"work_identity"`
	ProgressIdentity      string `json:"progress_identity"`
}

type AutomationFactName string

const (
	FactWorkerTerminal  AutomationFactName = "worker_terminal"
	FactWorkerMessage   AutomationFactName = "worker_message"
	FactCurrentStatus   AutomationFactName = "current_status"
	FactLastDiagnostic  AutomationFactName = "last_diagnostic"
	FactProgressSummary AutomationFactName = "progress_summary"
	FactOperatorAnswer  AutomationFactName = "operator_answer"
	FactCaptainAdvice   AutomationFactName = "captain_advice"
)

type AutomationFact struct {
	Name  AutomationFactName `json:"name"`
	Value string             `json:"value"`
}

type RecoveryInvocation struct {
	SchemaVersion string            `json:"schema_version"`
	InvocationID  string            `json:"invocation_id"`
	Binding       AutomationBinding `json:"binding"`
	Selection     ModelSelection    `json:"selection"`
	Facts         []AutomationFact  `json:"facts"`
}

type RecoveryAction string

const (
	RecoveryResumeWorker       RecoveryAction = "resume_worker"
	RecoveryAskCaptain         RecoveryAction = "ask_captain"
	RecoveryRetryOperationally RecoveryAction = "retry_operationally"
	RecoveryPauseForHuman      RecoveryAction = "pause_track_for_human"
)

type RecoveryDecision struct {
	SchemaVersion string         `json:"schema_version"`
	InvocationID  string         `json:"invocation_id"`
	Action        RecoveryAction `json:"action"`
	Answer        *string        `json:"answer,omitempty"`
}

type AdvisoryInvocation struct {
	SchemaVersion string            `json:"schema_version"`
	InvocationID  string            `json:"invocation_id"`
	Binding       AutomationBinding `json:"binding"`
	Selection     ModelSelection    `json:"selection"`
	Question      string            `json:"question"`
	Facts         []AutomationFact  `json:"facts"`
}

type AdvisoryOutcome string

const (
	AdvisoryAnswer       AdvisoryOutcome = "answer"
	AdvisoryCannotAnswer AdvisoryOutcome = "cannot_answer"
)

type AdvisoryResult struct {
	SchemaVersion string          `json:"schema_version"`
	InvocationID  string          `json:"invocation_id"`
	Outcome       AdvisoryOutcome `json:"outcome"`
	Answer        *string         `json:"answer,omitempty"`
}

// AutomationInvocation admits exactly one non-Baton operation.
type AutomationInvocation struct {
	Selected SelectedProfile
	Recovery *RecoveryInvocation
	Advisory *AdvisoryInvocation
}

type AutomationObservation struct {
	TransportStatus TransportStatus   `json:"transport_status"`
	DurationMillis  int64             `json:"duration_ms"`
	Usage           UsageReceipt      `json:"usage"`
	Diagnostic      Diagnostic        `json:"diagnostic"`
	Recovery        *RecoveryDecision `json:"recovery,omitempty"`
	Advisory        *AdvisoryResult   `json:"advisory,omitempty"`
}

type AutomationDriver interface {
	InvokeAutomation(
		context.Context,
		AutomationInvocation,
	) (AutomationObservation, error)
}

var _ AutomationDriver = Dispatcher{}

type automationAdapter interface {
	invokeAutomation(
		context.Context,
		AutomationInvocation,
	) (AutomationObservation, error)
}

func (Dispatcher) InvokeAutomation(
	ctx context.Context,
	invocation AutomationInvocation,
) (AutomationObservation, error) {
	if ctx == nil {
		return AutomationObservation{}, fail("INVALID_CONTEXT")
	}
	if err := ctx.Err(); err != nil {
		return AutomationObservation{}, normalizeAdapterError(err)
	}
	if err := validateAutomationInvocation(invocation); err != nil {
		return AutomationObservation{}, err
	}
	adapter, ok := invocation.Selected.adapter.(automationAdapter)
	if !ok {
		return AutomationObservation{}, fail("AUTOMATION_UNSUPPORTED")
	}
	observation, err := adapter.invokeAutomation(ctx, invocation)
	if err != nil {
		return AutomationObservation{}, normalizeAdapterError(err)
	}
	if err := ValidateAutomationObservation(invocation, observation); err != nil {
		return AutomationObservation{}, err
	}
	return observation, nil
}

func ValidateRecoveryInvocation(value RecoveryInvocation) error {
	if value.SchemaVersion != RecoveryInvocationSchemaVersion ||
		validateIdentity(value.InvocationID) != nil {
		return fail("INVALID_RECOVERY_INVOCATION")
	}
	if err := validateAutomationBinding(value.Binding); err != nil {
		return err
	}
	if err := ValidateModelSelection(value.Selection); err != nil {
		return err
	}
	if err := validateAutomationFacts(value.Facts); err != nil {
		return err
	}
	return nil
}

func ValidateRecoveryDecision(value RecoveryDecision) error {
	if value.SchemaVersion != RecoveryDecisionSchemaVersion {
		return failRecoveryDecision("recovery_decision.schema_version")
	}
	if validateIdentity(value.InvocationID) != nil {
		return failRecoveryDecision("recovery_decision.invocation_id")
	}
	switch value.Action {
	case RecoveryResumeWorker:
		if value.Answer == nil ||
			validateAutomationMessage(*value.Answer, false) != nil {
			return failRecoveryDecision("recovery_decision.resume_worker_answer_required")
		}
	case RecoveryAskCaptain, RecoveryRetryOperationally, RecoveryPauseForHuman:
		if value.Answer != nil {
			return failRecoveryDecision("recovery_decision.action_forbids_answer")
		}
	default:
		return failRecoveryDecision("recovery_decision.action_unknown")
	}
	return nil
}

// validRecoveryDecisionCheck is the closed vocabulary of
// ValidateRecoveryDecision's named refusal reasons, in the S1/S2 family
// idiom (validSandboxStartCheck, submissionRefusalDetail): a dotted
// <family>.<check> name, never derived from submitted bytes. This Detail
// never crosses normalizeAdapterError's funnel, so unlike those families it
// carries no structured envelope and needs no funnel-side re-validation.
// resume_worker_answer_required names one refusal reason for two distinct
// causes - value.Answer == nil (no answer given) and a non-nil Answer
// failing validateAutomationMessage (a malformed answer) - deliberately,
// per the Captain's ruling that a single name for both is acceptable as
// long as it is documented rather than implied to distinguish them.
func validRecoveryDecisionCheck(check string) bool {
	switch check {
	case
		"recovery_decision.schema_version",
		"recovery_decision.invocation_id",
		"recovery_decision.resume_worker_answer_required",
		"recovery_decision.action_forbids_answer",
		"recovery_decision.action_unknown":
		return true
	default:
		return false
	}
}

func failRecoveryDecision(check string) error {
	return &ContractError{Code: "INVALID_RECOVERY_DECISION", Detail: check}
}

// RecoveryAnswerForInvocation validates a direct recovery answer against the
// exact eligible fact bytes admitted to the same invocation and returns those
// fact bytes for forwarding to the worker.
func RecoveryAnswerForInvocation(
	invocation RecoveryInvocation,
	decision RecoveryDecision,
) (string, error) {
	if ValidateRecoveryInvocation(invocation) != nil ||
		ValidateRecoveryDecision(decision) != nil ||
		decision.InvocationID != invocation.InvocationID ||
		decision.Action != RecoveryResumeWorker ||
		decision.Answer == nil {
		return "", fail("INVALID_RECOVERY_DECISION")
	}
	for _, fact := range invocation.Facts {
		switch fact.Name {
		case FactCurrentStatus, FactLastDiagnostic, FactProgressSummary,
			FactOperatorAnswer, FactCaptainAdvice:
		default:
			continue
		}
		if bytes.Equal([]byte(*decision.Answer), []byte(fact.Value)) {
			return fact.Value, nil
		}
	}
	return "", fail("INVALID_RECOVERY_DECISION")
}

func ValidateAdvisoryInvocation(value AdvisoryInvocation) error {
	if value.SchemaVersion != AdvisoryInvocationSchemaVersion ||
		validateIdentity(value.InvocationID) != nil ||
		validateAutomationMessage(value.Question, false) != nil {
		return fail("INVALID_ADVISORY_INVOCATION")
	}
	if err := validateAutomationBinding(value.Binding); err != nil {
		return err
	}
	if err := ValidateModelSelection(value.Selection); err != nil {
		return err
	}
	if err := validateAutomationFacts(value.Facts); err != nil {
		return err
	}
	return nil
}

func ValidateAdvisoryResult(value AdvisoryResult) error {
	if value.SchemaVersion != AdvisoryResultSchemaVersion {
		return failAdvisoryResult("advisory_result.schema_version")
	}
	if validateIdentity(value.InvocationID) != nil {
		return failAdvisoryResult("advisory_result.invocation_id")
	}
	switch value.Outcome {
	case AdvisoryAnswer:
		if value.Answer == nil ||
			validateAutomationMessage(*value.Answer, false) != nil {
			return failAdvisoryResult("advisory_result.outcome_answer_required")
		}
	case AdvisoryCannotAnswer:
		if value.Answer != nil {
			return failAdvisoryResult("advisory_result.outcome_forbids_answer")
		}
	default:
		return failAdvisoryResult("advisory_result.outcome_unknown")
	}
	return nil
}

// validAdvisoryResultCheck is ValidateAdvisoryResult's counterpart to
// validRecoveryDecisionCheck, same idiom and same caveat:
// outcome_answer_required names one refusal reason for both a missing and a
// malformed answer.
func validAdvisoryResultCheck(check string) bool {
	switch check {
	case
		"advisory_result.schema_version",
		"advisory_result.invocation_id",
		"advisory_result.outcome_answer_required",
		"advisory_result.outcome_forbids_answer",
		"advisory_result.outcome_unknown":
		return true
	default:
		return false
	}
}

func failAdvisoryResult(check string) error {
	return &ContractError{Code: "INVALID_ADVISORY_RESULT", Detail: check}
}

func EncodeRecoveryInvocation(value RecoveryInvocation) ([]byte, error) {
	return encodeAutomationValue(value, ValidateRecoveryInvocation(value))
}

func EncodeRecoveryDecision(value RecoveryDecision) ([]byte, error) {
	return encodeAutomationValue(value, ValidateRecoveryDecision(value))
}

func EncodeAdvisoryInvocation(value AdvisoryInvocation) ([]byte, error) {
	return encodeAutomationValue(value, ValidateAdvisoryInvocation(value))
}

func EncodeAdvisoryResult(value AdvisoryResult) ([]byte, error) {
	return encodeAutomationValue(value, ValidateAdvisoryResult(value))
}

func DecodeRecoveryInvocation(body []byte) (RecoveryInvocation, error) {
	var value RecoveryInvocation
	root, err := decodeAutomationValue(
		body,
		[]string{"schema_version", "invocation_id", "binding", "selection", "facts"},
		nil,
		&value,
	)
	if err != nil {
		return RecoveryInvocation{}, err
	}
	if err := closeAutomationInvocationObjects(root); err != nil {
		return RecoveryInvocation{}, err
	}
	if err := ValidateRecoveryInvocation(value); err != nil {
		return RecoveryInvocation{}, err
	}
	if err := requireCanonicalAutomation(body, value); err != nil {
		return RecoveryInvocation{}, err
	}
	return value, nil
}

func DecodeRecoveryDecision(body []byte) (RecoveryDecision, error) {
	var value RecoveryDecision
	if _, err := decodeAutomationValue(
		body,
		[]string{"schema_version", "invocation_id", "action"},
		[]string{"answer"},
		&value,
	); err != nil {
		return RecoveryDecision{}, err
	}
	if err := ValidateRecoveryDecision(value); err != nil {
		return RecoveryDecision{}, err
	}
	if err := requireCanonicalAutomation(body, value); err != nil {
		return RecoveryDecision{}, err
	}
	return value, nil
}

func DecodeAdvisoryInvocation(body []byte) (AdvisoryInvocation, error) {
	var value AdvisoryInvocation
	root, err := decodeAutomationValue(
		body,
		[]string{
			"schema_version", "invocation_id", "binding", "selection",
			"question", "facts",
		},
		nil,
		&value,
	)
	if err != nil {
		return AdvisoryInvocation{}, err
	}
	if err := closeAutomationInvocationObjects(root); err != nil {
		return AdvisoryInvocation{}, err
	}
	if err := ValidateAdvisoryInvocation(value); err != nil {
		return AdvisoryInvocation{}, err
	}
	if err := requireCanonicalAutomation(body, value); err != nil {
		return AdvisoryInvocation{}, err
	}
	return value, nil
}

func DecodeAdvisoryResult(body []byte) (AdvisoryResult, error) {
	var value AdvisoryResult
	if _, err := decodeAutomationValue(
		body,
		[]string{"schema_version", "invocation_id", "outcome"},
		[]string{"answer"},
		&value,
	); err != nil {
		return AdvisoryResult{}, err
	}
	if err := ValidateAdvisoryResult(value); err != nil {
		return AdvisoryResult{}, err
	}
	if err := requireCanonicalAutomation(body, value); err != nil {
		return AdvisoryResult{}, err
	}
	return value, nil
}

func validateAutomationInvocation(value AutomationInvocation) error {
	if err := validateSelectedProfile(value.Selected); err != nil {
		return err
	}
	if (value.Recovery == nil) == (value.Advisory == nil) {
		return fail("INVALID_AUTOMATION_INVOCATION")
	}
	selection := ModelSelection{}
	if value.Recovery != nil {
		if err := ValidateRecoveryInvocation(*value.Recovery); err != nil {
			return err
		}
		selection = value.Recovery.Selection
	} else {
		if err := ValidateAdvisoryInvocation(*value.Advisory); err != nil {
			return err
		}
		selection = value.Advisory.Selection
	}
	if selection.Profile != value.Selected.Profile.Key ||
		selection.Model != value.Selected.Model {
		return fail("AUTOMATION_SELECTION_MISMATCH")
	}
	return nil
}

// ValidateAutomationObservation binds an automation result to its exact
// invocation. In particular, direct worker answers must come from an eligible
// fact in that recovery invocation.
func ValidateAutomationObservation(
	invocation AutomationInvocation,
	observation AutomationObservation,
) error {
	if err := validateAutomationInvocation(invocation); err != nil {
		return err
	}
	if observation.TransportStatus != Completed ||
		observation.DurationMillis < 0 ||
		observation.DurationMillis > MaxSafeInteger ||
		observation.Diagnostic.Code != "none" ||
		(observation.Recovery == nil) == (observation.Advisory == nil) {
		return fail("INVALID_AUTOMATION_OBSERVATION")
	}
	if _, err := EncodeUsageReceipt(observation.Usage); err != nil {
		return err
	}
	if invocation.Recovery != nil {
		if observation.Advisory != nil ||
			ValidateRecoveryDecision(*observation.Recovery) != nil {
			return fail("INVALID_AUTOMATION_OBSERVATION")
		}
		if observation.Recovery.InvocationID != invocation.Recovery.InvocationID {
			return fail("AUTOMATION_INVOCATION_ID_MISMATCH")
		}
		if observation.Recovery.Action == RecoveryResumeWorker {
			if _, err := RecoveryAnswerForInvocation(
				*invocation.Recovery,
				*observation.Recovery,
			); err != nil {
				return fail("INVALID_AUTOMATION_OBSERVATION")
			}
		}
		return nil
	}
	if observation.Recovery != nil ||
		ValidateAdvisoryResult(*observation.Advisory) != nil {
		return fail("INVALID_AUTOMATION_OBSERVATION")
	}
	if observation.Advisory.InvocationID != invocation.Advisory.InvocationID {
		return fail("AUTOMATION_INVOCATION_ID_MISMATCH")
	}
	return nil
}

func validateAutomationBinding(value AutomationBinding) error {
	if validateIdentity(value.RunID) != nil ||
		validateIdentity(value.TrackID) != nil ||
		validateIdentity(value.Slice) != nil ||
		value.BatonAttempt < 1 || value.BatonAttempt > MaxSafeInteger ||
		!digestPattern.MatchString(value.PlanAuthorityDigest) ||
		!digestPattern.MatchString(value.TargetAuthorityDigest) ||
		!digestPattern.MatchString(value.WorkIdentity) ||
		!digestPattern.MatchString(value.ProgressIdentity) {
		return fail("INVALID_AUTOMATION_BINDING")
	}
	return nil
}

func validateAutomationFacts(values []AutomationFact) error {
	if len(values) > MaxAutomationFacts {
		return fail("INVALID_AUTOMATION_FACTS")
	}
	seen := make(map[AutomationFactName]struct{}, len(values))
	total := 0
	for _, value := range values {
		switch value.Name {
		case FactWorkerTerminal, FactWorkerMessage, FactCurrentStatus,
			FactLastDiagnostic, FactProgressSummary, FactOperatorAnswer,
			FactCaptainAdvice:
		default:
			return fail("INVALID_AUTOMATION_FACTS")
		}
		if _, duplicate := seen[value.Name]; duplicate ||
			validateAutomationMessage(value.Value, false) != nil ||
			len([]byte(value.Value)) > MaxAutomationFactBytes {
			return fail("INVALID_AUTOMATION_FACTS")
		}
		seen[value.Name] = struct{}{}
		total += len([]byte(value.Value))
		if total > MaxAutomationBytes/2 {
			return fail("INVALID_AUTOMATION_FACTS")
		}
	}
	return nil
}

func validateAutomationMessage(value string, allowEmpty bool) error {
	if !utf8.ValidString(value) ||
		len([]byte(value)) > MaxAutomationMessageBytes ||
		strings.ContainsRune(value, '\x00') ||
		strings.ContainsRune(value, '\r') ||
		(!allowEmpty && strings.TrimSpace(value) == "") {
		return fail("INVALID_AUTOMATION_MESSAGE")
	}
	return nil
}

func encodeAutomationValue(value any, validation error) ([]byte, error) {
	if validation != nil {
		return nil, validation
	}
	body, err := json.Marshal(value)
	if err != nil || len(body)+1 > MaxAutomationBytes {
		return nil, fail("RESOURCE_LIMIT")
	}
	return append(body, '\n'), nil
}

func decodeAutomationValue(
	body []byte,
	required, optional []string,
	target any,
) (map[string]any, error) {
	if len(body) < 2 || len(body) > MaxAutomationBytes ||
		body[len(body)-1] != '\n' {
		return nil, fail("INVALID_AUTOMATION_VALUE")
	}
	return decodeTyped(body, MaxAutomationBytes, required, optional, target)
}

func closeAutomationInvocationObjects(root map[string]any) error {
	if _, err := closedObject(
		root["binding"],
		[]string{
			"run_id", "track_id", "slice", "baton_attempt",
			"plan_authority_digest", "target_authority_digest",
			"work_identity", "progress_identity",
		},
		nil,
	); err != nil {
		return err
	}
	if _, err := closedObject(
		root["selection"],
		[]string{"profile", "model"},
		nil,
	); err != nil {
		return err
	}
	facts, ok := root["facts"].([]any)
	if !ok {
		return fail("INVALID_AUTOMATION_FACTS")
	}
	for _, value := range facts {
		if _, err := closedObject(value, []string{"name", "value"}, nil); err != nil {
			return err
		}
	}
	return nil
}

func requireCanonicalAutomation(body []byte, value any) error {
	canonical, err := json.Marshal(value)
	if err != nil {
		return fail("INVALID_JSON")
	}
	if !bytes.Equal(append(canonical, '\n'), body) {
		return fail("NONCANONICAL_JSON")
	}
	return nil
}

func automationToolDefinitions(
	invocation AutomationInvocation,
) ([]providerToolDefinition, error) {
	if err := validateAutomationInvocation(invocation); err != nil {
		return nil, err
	}
	if invocation.Recovery != nil {
		return []providerToolDefinition{{
			Name:        "sworn_recovery_decide",
			Description: "Choose exactly one bounded recovery action. resume_worker must copy one eligible exact fact value byte-for-byte; use ask_captain for judgment and pause_track_for_human when uncertain.",
			InputSchema: json.RawMessage(swornRecoveryDecisionInputSchema),
		}}, nil
	}
	return []providerToolDefinition{{
		Name:        "sworn_advisory_respond",
		Description: "Return bounded non-gate advice or cannot_answer. This is not a Baton Captain decision.",
		InputSchema: json.RawMessage(swornAdvisoryResultInputSchema),
	}}, nil
}

func automationPrompt(invocation AutomationInvocation) ([]byte, error) {
	if err := validateAutomationInvocation(invocation); err != nil {
		return nil, err
	}
	var operation string
	var request any
	var instruction string
	if invocation.Recovery != nil {
		operation = "recovery"
		request = invocation.Recovery
		instruction = "Use only sworn_recovery_decide and call it exactly once. resume_worker.answer must copy byte-for-byte one current_status, last_diagnostic, progress_summary, operator_answer, or captain_advice fact from this request. worker_terminal and worker_message are context only. Use ask_captain for judgment and pause_track_for_human when uncertain. Do not claim Baton authority or invent facts."
	} else {
		operation = "advisory"
		request = invocation.Advisory
		instruction = "Use only sworn_advisory_respond and call it exactly once. Return answer only as bounded non-gate advice; return cannot_answer when uncertain. This is not a Baton Captain decision."
	}
	body, err := json.Marshal(struct {
		SchemaVersion string `json:"schema_version"`
		Operation     string `json:"operation"`
		Request       any    `json:"request"`
		Instruction   string `json:"instruction"`
	}{
		SchemaVersion: "sworn.automation-prompt/v1",
		Operation:     operation,
		Request:       request,
		Instruction:   instruction,
	})
	if err != nil || len(body) > MaxAutomationBytes {
		return nil, fail("RESOURCE_LIMIT")
	}
	return body, nil
}

func (adapter *loopAdapter) invokeAutomation(
	ctx context.Context,
	invocation AutomationInvocation,
) (AutomationObservation, error) {
	started := time.Now()
	if adapter == nil ||
		invocation.Selected.Adapter != adapter.identity ||
		invocation.Selected.Profile.Network != NetworkRequired {
		return AutomationObservation{}, fail("INVALID_ADAPTER")
	}
	prompt, err := automationPrompt(invocation)
	if err != nil {
		return AutomationObservation{}, err
	}
	definitions, err := automationToolDefinitions(invocation)
	if err != nil {
		clearBytes(prompt)
		return AutomationObservation{}, err
	}
	conversation, err := adapter.new(
		prompt,
		invocation.Selected.Model,
		definitions,
		Limits{},
	)
	clearBytes(prompt)
	if err != nil {
		return AutomationObservation{}, err
	}
	defer conversation.close()

	var total Usage
	usageAvailable := false
	corrections := 0
	var turnCount, toolCallCount int64
	seenIDs := make(map[string]struct{})
	expected := definitions[0].Name
	for turn := 0; turn < MaxAutomationTurns; turn++ {
		request, err := conversation.request()
		if err != nil || len(request.Body) < 1 ||
			len(request.Body) > MaxProviderRequestBytes {
			clearBytes(request.Body)
			return AutomationObservation{}, fail("INVALID_PROVIDER_REQUEST")
		}
		response, err := adapter.transport.roundTrip(
			ctx,
			invocation.Selected.Profile.CredentialRef,
			request,
		)
		clearBytes(request.Body)
		if err != nil {
			return AutomationObservation{}, err
		}
		providerTurn, err := conversation.accept(response)
		clearBytes(response)
		if err != nil {
			return AutomationObservation{}, err
		}
		turnCount++
		if len(providerTurn.Calls) == 1 {
			toolCallCount++
		}
		if providerTurn.Usage != nil {
			if providerTurn.Usage.InputTokens < 0 ||
				providerTurn.Usage.OutputTokens < 0 ||
				total.InputTokens > math.MaxInt64-providerTurn.Usage.InputTokens ||
				total.OutputTokens > math.MaxInt64-providerTurn.Usage.OutputTokens {
				return AutomationObservation{}, fail("INVALID_USAGE")
			}
			total.InputTokens += providerTurn.Usage.InputTokens
			total.OutputTokens += providerTurn.Usage.OutputTokens
			usageAvailable = true
		}
		if len(providerTurn.Calls) != 1 {
			return AutomationObservation{}, fail("AUTOMATION_PROTOCOL_FAILED")
		}
		call := providerTurn.Calls[0]
		if validateText(call.ID, MaxCorrelationIDBytes, false) != nil ||
			call.Name != expected || len(call.Arguments) == 0 ||
			len(call.Arguments) > MaxToolArgumentBytes {
			return AutomationObservation{}, fail("AUTOMATION_PROTOCOL_FAILED")
		}
		if _, duplicate := seenIDs[call.ID]; duplicate {
			return AutomationObservation{}, fail("AUTOMATION_PROTOCOL_FAILED")
		}
		seenIDs[call.ID] = struct{}{}
		observation, terminalErr := decodeAutomationTerminal(
			started,
			invocation,
			call.Arguments,
			total,
			usageAvailable,
			turnCount,
			toolCallCount,
		)
		if terminalErr == nil {
			return observation, nil
		}
		corrections++
		if corrections > MaxAutomationCorrections {
			return AutomationObservation{}, fail("AUTOMATION_CORRECTIONS_EXHAUSTED")
		}
		if err := conversation.appendResults([]providerToolResult{{
			ID: call.ID, Name: call.Name,
			Content: toolErrorContent(terminalErr),
			Failed:  true,
		}}); err != nil {
			return AutomationObservation{}, err
		}
	}
	return AutomationObservation{}, fail("AUTOMATION_CORRECTIONS_EXHAUSTED")
}

func decodeAutomationTerminal(
	started time.Time,
	invocation AutomationInvocation,
	arguments []byte,
	total Usage,
	usageAvailable bool,
	turns, toolCalls int64,
) (AutomationObservation, error) {
	value, err := decodeStrict(arguments, MaxToolArgumentBytes)
	if err != nil {
		return AutomationObservation{}, err
	}
	usage, err := NormalizeUsage(nil, nil, invocation.Selected.Adapter.ID)
	if usageAvailable {
		usage, err = NormalizeUsage(&total, nil, invocation.Selected.Adapter.ID)
	}
	if err != nil {
		return AutomationObservation{}, err
	}
	applyTurnEconomics(&usage, turns, toolCalls, nil)
	observation := AutomationObservation{
		TransportStatus: Completed,
		DurationMillis:  time.Since(started).Milliseconds(),
		Usage:           usage,
		Diagnostic:      Diagnostic{Code: "none"},
	}
	if observation.DurationMillis < 0 {
		observation.DurationMillis = 0
	}
	if invocation.Recovery != nil {
		root, err := closedObject(value, []string{"decision"}, nil)
		if err != nil {
			return AutomationObservation{}, err
		}
		decisionObject, err := closedObject(
			root["decision"],
			[]string{"schema_version", "invocation_id", "action"},
			[]string{"answer"},
		)
		if err != nil {
			return AutomationObservation{}, err
		}
		body, err := canonicalJSON(decisionObject)
		if err != nil {
			return AutomationObservation{}, err
		}
		var decision RecoveryDecision
		err = json.Unmarshal(body, &decision)
		clearBytes(body)
		if err != nil {
			return AutomationObservation{}, fail("INVALID_FIELD")
		}
		if err := ValidateRecoveryDecision(decision); err != nil {
			return AutomationObservation{}, err
		}
		if decision.InvocationID != invocation.Recovery.InvocationID {
			return AutomationObservation{}, fail("AUTOMATION_INVOCATION_ID_MISMATCH")
		}
		observation.Recovery = &decision
		return observation, nil
	}
	root, err := closedObject(value, []string{"result"}, nil)
	if err != nil {
		return AutomationObservation{}, err
	}
	resultObject, err := closedObject(
		root["result"],
		[]string{"schema_version", "invocation_id", "outcome"},
		[]string{"answer"},
	)
	if err != nil {
		return AutomationObservation{}, err
	}
	body, err := canonicalJSON(resultObject)
	if err != nil {
		return AutomationObservation{}, err
	}
	var result AdvisoryResult
	err = json.Unmarshal(body, &result)
	clearBytes(body)
	if err != nil {
		return AutomationObservation{}, fail("INVALID_FIELD")
	}
	if err := ValidateAdvisoryResult(result); err != nil {
		return AutomationObservation{}, err
	}
	if result.InvocationID != invocation.Advisory.InvocationID {
		return AutomationObservation{}, fail("AUTOMATION_INVOCATION_ID_MISMATCH")
	}
	observation.Advisory = &result
	return observation, nil
}
