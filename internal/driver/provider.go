package driver

import (
	"context"
	"encoding/json"
	"math"
	"time"
)

const (
	// MaxProviderTurns is a runaway-loop guard, not a work budget. A careful
	// implementer pass over this repo needs ~30 tool turns, so any value that
	// could bind honest work is too low; recap only from eval evidence.
	MaxProviderTurns         = 1_000
	MaxProviderRequestBytes  = 1_048_576
	MaxProviderResponseBytes = 1_048_576
)

type providerRequest struct {
	Method      string
	URL         string
	ContentType string
	Headers     map[string]string
	Body        []byte
}

type providerTurn struct {
	Calls []providerToolCall
	Usage *Usage
	Prose bool
	// ReasoningEffort is the value the provider reported for this turn, when
	// the response carries one; nil is honest absence.
	ReasoningEffort *string
	// FinishReason and Truncated are set by an adapter when the provider's
	// own output-ceiling finish reason ended the invocation, so the run
	// reports an explicit PROVIDER_TRUNCATED failure instead of an empty
	// successful result.
	FinishReason *string
	Truncated    bool
}

type providerConversation interface {
	request() (providerRequest, error)
	accept([]byte) (providerTurn, error)
	appendResults([]providerToolResult) error
	appendInstruction([]byte) error
	resume([]byte, []providerToolDefinition) error
	// declaredReasoningEffort returns the exact effort value this
	// conversation serializes into its requests; an empty string is honest
	// absence for dialects that have no reasoning-effort vocabulary.
	declaredReasoningEffort() string
	close()
}

type providerConversationFactory func(
	prompt []byte,
	model string,
	tools []providerToolDefinition,
) (providerConversation, error)

type providerTransport interface {
	roundTrip(context.Context, *string, providerRequest) ([]byte, error)
	check(context.Context, profileCheckKind, *string, string) (ReadinessState, string)
}

type providerDialect string

const (
	providerDialectOpenAIResponses providerDialect = "openai_responses"
	providerDialectOpenAIChat      providerDialect = "openai_chat"
	providerDialectOpenRouterChat  providerDialect = "openrouter_chat"
	providerDialectOpaqueChat      providerDialect = "opaque_chat"
	providerDialectGemini          providerDialect = "gemini"
	providerDialectBedrockConverse providerDialect = "bedrock_converse"
)

func (dialect providerDialect) continuationMode() ContinuationMode {
	switch dialect {
	case providerDialectOpenAIChat:
		return ContinuationModeTranscriptReplay
	case providerDialectOpenAIResponses,
		providerDialectOpenRouterChat,
		providerDialectOpaqueChat,
		providerDialectGemini,
		providerDialectBedrockConverse:
		return ContinuationModeOpaqueReplay
	default:
		return ""
	}
}

type loopAdapter struct {
	identity  AdapterIdentity
	family    ProfileFamily
	surface   ProfileSurface
	dialect   providerDialect
	new       providerConversationFactory
	transport providerTransport
}

func newLoopAdapter(
	key, id, version string,
	family ProfileFamily,
	surface ProfileSurface,
	dialect providerDialect,
	configuration any,
	factory providerConversationFactory,
	transport providerTransport,
) (*loopAdapter, error) {
	if !providerKeyPattern.MatchString(key) ||
		!driverIdentityPattern.MatchString(id) ||
		!versionPattern.MatchString(version) ||
		!family.valid() || family == ProfileFake ||
		!surface.validFor(family) ||
		dialect.continuationMode() == "" ||
		factory == nil || transport == nil {
		return nil, fail("INVALID_ADAPTER")
	}
	body, err := canonicalJSON(configuration)
	if err != nil {
		return nil, err
	}
	return &loopAdapter{
		identity: AdapterIdentity{
			Key:                 key,
			ID:                  id,
			Version:             version,
			ConfigurationDigest: Digest(body),
		},
		family:    family,
		surface:   surface,
		dialect:   dialect,
		new:       factory,
		transport: transport,
	}, nil
}

func (adapter *loopAdapter) Identity() AdapterIdentity {
	if adapter == nil {
		return AdapterIdentity{}
	}
	return adapter.identity
}

func (adapter *loopAdapter) profileFamily() ProfileFamily {
	if adapter == nil {
		return ""
	}
	return adapter.family
}

func (adapter *loopAdapter) profileSurface() ProfileSurface {
	if adapter == nil {
		return ""
	}
	return adapter.surface
}

func (adapter *loopAdapter) checkProfile(
	ctx context.Context,
	kind profileCheckKind,
	profile ProfileConfig,
	model string,
) (ReadinessState, string) {
	if adapter == nil || profile.Adapter != adapter.identity.Key ||
		profile.Network != NetworkRequired ||
		validateText(model, 500, false) != nil {
		return ReadinessFail, "profile_binding_invalid"
	}
	return adapter.transport.check(ctx, kind, profile.CredentialRef, model)
}

func (adapter *loopAdapter) invoke(
	ctx context.Context,
	invocation Invocation,
) (observation Observation, resultErr error) {
	started := time.Now()
	if adapter == nil || invocation.Selected.Adapter != adapter.identity ||
		invocation.Selected.Profile.Network != NetworkRequired {
		return Observation{}, fail("INVALID_ADAPTER")
	}
	prompt, err := modelPrompt(invocation)
	if err != nil {
		return Observation{}, err
	}
	conversation, err := adapter.new(
		prompt,
		invocation.Selected.Model,
		toolDefinitions(invocation.Request.Workspace.Access),
	)
	clearBytes(prompt)
	if err != nil {
		return Observation{}, err
	}
	defer conversation.close()
	observation, _, resultErr = adapter.runConversation(
		ctx,
		started,
		invocation,
		conversation,
		nil,
		false,
	)
	return observation, resultErr
}

type apiContinuationState struct {
	conversation providerConversation
	dialect      providerDialect
	mode         ContinuationMode
	bytes        int64
	closed       bool
}

func (state *apiContinuationState) continuationMode() ContinuationMode {
	if state == nil || state.closed {
		return ""
	}
	return state.mode
}

func (state *apiContinuationState) continuationBytes() int64 {
	if state == nil || state.closed {
		return 0
	}
	return state.bytes
}

func (state *apiContinuationState) closeContinuation() error {
	if state == nil || state.closed {
		return nil
	}
	state.closed = true
	if state.conversation != nil {
		state.conversation.close()
	}
	state.conversation = nil
	state.dialect = ""
	state.mode = ""
	state.bytes = 0
	return nil
}

func (adapter *loopAdapter) invokeContinuation(
	ctx context.Context,
	invocation Invocation,
) (Observation, continuationState, error) {
	started := time.Now()
	if adapter == nil || invocation.Selected.Adapter != adapter.identity ||
		invocation.Selected.Profile.Network != NetworkRequired {
		return Observation{}, nil, fail("INVALID_ADAPTER")
	}
	prompt, err := modelPrompt(invocation)
	if err != nil {
		return Observation{}, nil, err
	}
	conversation, err := adapter.new(
		prompt,
		invocation.Selected.Model,
		toolDefinitions(invocation.Request.Workspace.Access),
	)
	clearBytes(prompt)
	if err != nil {
		return Observation{}, nil, err
	}
	observation, state, resultErr := adapter.runConversation(
		ctx,
		started,
		invocation,
		conversation,
		nil,
		true,
	)
	if state == nil {
		conversation.close()
	}
	return observation, state, resultErr
}

func (adapter *loopAdapter) invokeRecoverableContinuation(
	ctx context.Context,
	invocation Invocation,
) (Observation, continuationState, error) {
	return adapter.invokeContinuation(ctx, invocation)
}

func (adapter *loopAdapter) resumeContinuation(
	ctx context.Context,
	invocation Invocation,
	prior continuationState,
) (Observation, error) {
	observation, _, err := adapter.resumeProviderContinuation(
		ctx, invocation, prior, false, false,
	)
	return observation, err
}

func (adapter *loopAdapter) resumeRecoverableContinuation(
	ctx context.Context,
	invocation Invocation,
	prior continuationState,
	retainDesignTerminal bool,
) (Observation, continuationState, error) {
	return adapter.resumeProviderContinuation(
		ctx, invocation, prior, true, retainDesignTerminal,
	)
}

func (adapter *loopAdapter) resumeProviderContinuation(
	ctx context.Context,
	invocation Invocation,
	prior continuationState,
	retainYield bool,
	retainDesignTerminal bool,
) (Observation, continuationState, error) {
	started := time.Now()
	state, ok := prior.(*apiContinuationState)
	if adapter == nil || !ok || state == nil || state.closed ||
		state.conversation == nil ||
		state.dialect != adapter.dialect ||
		state.mode != adapter.dialect.continuationMode() ||
		state.bytes < 1 || state.bytes > MaxProviderRequestBytes ||
		invocation.Selected.Adapter != adapter.identity ||
		invocation.Selected.Profile.Network != NetworkRequired {
		return Observation{}, nil, fail("CONTINUATION_INVALID")
	}
	prompt, err := modelPrompt(invocation)
	if err != nil {
		return Observation{}, nil, fail("CONTINUATION_INVALID")
	}
	err = state.conversation.resume(
		prompt,
		toolDefinitions(invocation.Request.Workspace.Access),
	)
	clearBytes(prompt)
	if err != nil {
		return Observation{}, nil, fail("CONTINUATION_INVALID")
	}
	request, err := state.conversation.request()
	if err != nil || len(request.Body) < 1 ||
		len(request.Body) > MaxProviderRequestBytes {
		clearBytes(request.Body)
		return Observation{}, nil, fail("CONTINUATION_INVALID")
	}
	defer clearBytes(request.Body)
	observation, retained, resultErr := adapter.runConversation(
		ctx,
		started,
		invocation,
		state.conversation,
		&request,
		retainYield,
	)
	// CONTINUATION_INVALID is reserved for rejection before provider or tool
	// effects so the dispatcher can safely request one fresh rehydration.
	if IsCode(resultErr, "CONTINUATION_INVALID") {
		resultErr = fail("PROTOCOL_FAILURE")
	}
	if retained != nil {
		state.conversation = nil
		state.dialect = ""
		state.mode = ""
		state.bytes = 0
		state.closed = true
	}
	if retained != nil &&
		(observation.Yield == nil || !retainYield) &&
		(observation.Handoff == nil || !retainDesignTerminal) {
		_ = closeContinuationState(retained)
		retained = nil
	}
	return observation, retained, resultErr
}

func (adapter *loopAdapter) runConversation(
	ctx context.Context,
	started time.Time,
	invocation Invocation,
	conversation providerConversation,
	initialRequest *providerRequest,
	retain bool,
) (observation Observation, state continuationState, resultErr error) {
	session, err := newToolSession(invocation)
	if err != nil {
		return Observation{}, nil, err
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil {
			observation.Handoff = nil
			observation.Yield = nil
			if state != nil {
				_ = closeContinuationState(state)
				state = nil
			}
			resultErr = joinErrors(resultErr, closeErr)
		}
	}()
	var total Usage
	usageAvailable := false
	effortRequested := conversation.declaredReasoningEffort()
	var effortReported *string
	seenIDs := make(map[string]struct{})
	proseNudges := 0
	for turn := 0; turn < MaxProviderTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return Observation{}, nil, err
		}
		var request providerRequest
		if initialRequest != nil {
			request = *initialRequest
			initialRequest = nil
		} else {
			request, err = conversation.request()
		}
		if err != nil || len(request.Body) > MaxProviderRequestBytes {
			clearBytes(request.Body)
			return Observation{}, nil, fail("CONTINUATION_INVALID")
		}
		response, err := adapter.transport.roundTrip(
			ctx,
			invocation.Selected.Profile.CredentialRef,
			request,
		)
		clearBytes(request.Body)
		if err != nil {
			return Observation{}, nil, err
		}
		providerTurn, err := conversation.accept(response)
		clearBytes(response)
		if err != nil {
			return Observation{}, nil, err
		}
		if providerTurn.Usage != nil {
			if err := addTurnUsage(&total, providerTurn.Usage); err != nil {
				return Observation{}, nil, err
			}
			usageAvailable = true
		}
		if providerTurn.ReasoningEffort != nil {
			value := *providerTurn.ReasoningEffort
			effortReported = &value
		}
		if providerTurn.Truncated {
			usage, err := NormalizeUsage(nil, nil)
			if usageAvailable {
				usage, err = NormalizeUsage(&total, nil)
			}
			if err != nil {
				return Observation{}, nil, err
			}
			applyInvocationFacts(
				&usage,
				effortRequested,
				effortReported,
				providerTurn.FinishReason,
				true,
			)
			observation := Observation{
				TransportStatus: RunnerError,
				DurationMillis:  time.Since(started).Milliseconds(),
				Usage:           usage,
				Diagnostic:      Diagnostic{Code: "provider_truncated"},
			}
			return observation, nil, fail("PROVIDER_TRUNCATED")
		}
		if len(providerTurn.Calls) == 0 {
			// A call-less turn is nudged, never failed: some models need
			// many nudges to land a tool call, and every nudge is durably
			// accounted as eval data. The turn budget and timeout are the
			// only bounds on how long that patience lasts.
			if err := reserveRecoveryStep(
				ctx,
				invocation.RecoveryStepHook,
				RecoveryStepProseNudge,
			); err != nil {
				return Observation{}, nil, err
			}
			if err := conversation.appendInstruction(
				[]byte(providerProseNudge),
			); err != nil {
				return Observation{}, nil, err
			}
			proseNudges++
			continue
		}
		if len(providerTurn.Calls) > MaxToolCalls {
			return Observation{}, nil, fail("RESOURCE_LIMIT")
		}
		terminalCalls := 0
		for _, call := range providerTurn.Calls {
			if call.Name == "sworn_submit" || call.Name == "sworn_yield" {
				terminalCalls++
			}
		}
		if terminalCalls > 0 &&
			(terminalCalls != 1 || len(providerTurn.Calls) != 1) {
			return Observation{}, nil, fail("SUBMISSION_PROTOCOL_FAILED")
		}
		results := make([]providerToolResult, 0, len(providerTurn.Calls))
		for _, call := range providerTurn.Calls {
			if validateProviderToolCall(call, seenIDs) != nil {
				return Observation{}, nil, fail("CONTINUATION_INVALID")
			}
			results = append(results, session.execute(ctx, call))
		}
		if terminated, terminalErr := session.terminated(); terminated {
			if terminalErr != nil {
				return Observation{}, nil, terminalErr
			}
			if retain {
				if err := conversation.appendResults(results); err != nil {
					return Observation{}, nil, err
				}
			}
			if closeErr := session.Close(); closeErr != nil {
				return Observation{}, nil, closeErr
			}
			usage, err := NormalizeUsage(nil, nil)
			if usageAvailable {
				usage, err = NormalizeUsage(&total, nil)
			}
			if err != nil {
				return Observation{}, nil, err
			}
			applyInvocationFacts(
				&usage,
				effortRequested,
				effortReported,
				nil,
				false,
			)
			observation := completedToolObservation(started, usage, session.handoff())
			if yielded := session.yielded(); yielded != nil {
				observation = completedYieldObservation(started, usage, yielded)
			}
			if !retain {
				return observation, nil, nil
			}
			replay, replayErr := conversation.request()
			if replayErr != nil || len(replay.Body) < 1 ||
				len(replay.Body) > MaxProviderRequestBytes {
				clearBytes(replay.Body)
				return observation, nil, nil
			}
			replayBytes := int64(len(replay.Body))
			clearBytes(replay.Body)
			mode := adapter.dialect.continuationMode()
			return observation, &apiContinuationState{
				conversation: conversation,
				dialect:      adapter.dialect,
				mode:         mode,
				bytes:        replayBytes,
			}, nil
		}
		if err := conversation.appendResults(results); err != nil {
			return Observation{}, nil, err
		}
	}
	return Observation{}, nil, fail("RESOURCE_LIMIT")
}

const providerProseNudge = "Finish this turn now by calling exactly one advertised Sworn terminal tool. Do not answer in prose."

func validateProviderToolCall(call providerToolCall, seen map[string]struct{}) error {
	if validateText(call.ID, MaxCorrelationIDBytes, false) != nil ||
		!providerKeyPattern.MatchString(call.Name) ||
		len(call.Arguments) == 0 || len(call.Arguments) > MaxToolArgumentBytes {
		return fail("CONTINUATION_INVALID")
	}
	if _, exists := seen[call.ID]; exists {
		return fail("CONTINUATION_INVALID")
	}
	seen[call.ID] = struct{}{}
	switch call.Name {
	case "Bash", "Read", "Write", "Edit", "Glob", "Grep",
		"sworn_submit", "sworn_yield":
		return nil
	default:
		return fail("TOOL_NOT_ALLOWED")
	}
}

func completedToolObservation(
	started time.Time,
	usage UsageReceipt,
	handoff *SealedHandoff,
) Observation {
	kinds := []string{
		"result_completed",
		"submit_accepted_pending",
		"submit_acknowledged",
		"engine_stop_after_submit",
		"process_waited",
		"process_group_quiescent",
		"workspace_postcheck",
		"input_projection_removed",
		"producers_joined",
		"published",
	}
	events := make([]TerminalEvent, len(kinds))
	for index, kind := range kinds {
		events[index] = TerminalEvent{Sequence: uint64(index + 1), Kind: kind}
	}
	duration := time.Since(started).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	return Observation{
		TransportStatus: Completed,
		DurationMillis:  duration,
		Usage:           usage,
		Diagnostic:      Diagnostic{Code: "none"},
		Handoff:         handoff,
		Events:          events,
	}
}

func completedYieldObservation(
	started time.Time,
	usage UsageReceipt,
	yield *Yield,
) Observation {
	kinds := []string{
		"result_completed",
		"yield_accepted",
		"engine_stop_after_yield",
		"process_waited",
		"process_group_quiescent",
		"workspace_postcheck",
		"input_projection_removed",
		"producers_joined",
		"completed_without_handoff",
	}
	events := make([]TerminalEvent, len(kinds))
	for index, kind := range kinds {
		events[index] = TerminalEvent{Sequence: uint64(index + 1), Kind: kind}
	}
	duration := time.Since(started).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	return Observation{
		TransportStatus: Completed,
		DurationMillis:  duration,
		Usage:           usage,
		Diagnostic:      Diagnostic{Code: "none"},
		Yield:           yield,
		Events:          events,
	}
}

// addTurnUsage accumulates one provider turn's reported accounting into the
// invocation total. Cache reads and writes are summed exactly like tokens;
// each side is carried independently so a read-only vocabulary (Gemini, the
// Responses API) never fabricates a zero on the side it does not report.
func addTurnUsage(total *Usage, turn *Usage) error {
	if turn == nil || total == nil ||
		turn.InputTokens < 0 || turn.OutputTokens < 0 ||
		total.InputTokens > math.MaxInt64-turn.InputTokens ||
		total.OutputTokens > math.MaxInt64-turn.OutputTokens {
		return fail("INVALID_USAGE")
	}
	total.InputTokens += turn.InputTokens
	total.OutputTokens += turn.OutputTokens
	if turn.CacheReadTokens != nil {
		if *turn.CacheReadTokens < 0 {
			return fail("INVALID_USAGE")
		}
		if total.CacheReadTokens == nil {
			read := *turn.CacheReadTokens
			total.CacheReadTokens = &read
		} else if *total.CacheReadTokens > math.MaxInt64-*turn.CacheReadTokens {
			return fail("INVALID_USAGE")
		} else {
			*total.CacheReadTokens += *turn.CacheReadTokens
		}
	}
	if turn.CacheWriteTokens != nil {
		if *turn.CacheWriteTokens < 0 {
			return fail("INVALID_USAGE")
		}
		if total.CacheWriteTokens == nil {
			write := *turn.CacheWriteTokens
			total.CacheWriteTokens = &write
		} else if *total.CacheWriteTokens > math.MaxInt64-*turn.CacheWriteTokens {
			return fail("INVALID_USAGE")
		} else {
			*total.CacheWriteTokens += *turn.CacheWriteTokens
		}
	}
	return nil
}

// applyInvocationFacts stamps the invocation-level facts onto a receipt. The
// requested effort is the exact value the conversation serialized; the
// reported effort is the last value a provider echoed; finish reason and
// truncation are carried on the explicit PROVIDER_TRUNCATED failure.
func applyInvocationFacts(
	usage *UsageReceipt,
	effortRequested string,
	effortReported *string,
	finishReason *string,
	truncated bool,
) {
	if usage == nil {
		return
	}
	if effortRequested != "" {
		value := effortRequested
		usage.EffortRequested = &value
	}
	if effortReported != nil {
		value := *effortReported
		usage.EffortReported = &value
	}
	if finishReason != nil {
		value := *finishReason
		usage.FinishReason = &value
	}
	if truncated {
		value := true
		usage.Truncated = &value
	}
}

func modelPrompt(invocation Invocation) ([]byte, error) {
	descriptor, err := invocation.Permission.Describe()
	if err != nil {
		return nil, err
	}
	type promptEnvelope struct {
		SchemaVersion  string         `json:"schema_version"`
		InvocationID   string         `json:"invocation_id"`
		Role           Role           `json:"role"`
		Operation      Operation      `json:"operation"`
		Workspace      Workspace      `json:"workspace"`
		Inputs         []Input        `json:"inputs"`
		Responsibility Responsibility `json:"responsibility"`
		ResultFields   []string       `json:"result_fields"`
		Instruction    string         `json:"instruction"`
		Recovery       *struct {
			Kind    RecoverableInputKind `json:"kind"`
			Content string               `json:"content"`
		} `json:"recovery,omitempty"`
	}
	envelope := promptEnvelope{
		SchemaVersion:  "sworn.model-prompt/v1",
		InvocationID:   invocation.Request.InvocationID,
		Role:           invocation.Request.Role,
		Operation:      invocation.Request.Operation,
		Workspace:      invocation.Request.Workspace,
		Inputs:         invocation.Request.Inputs,
		Responsibility: descriptor.Responsibility,
		ResultFields:   submissionResultFields(descriptor.Responsibility),
		Instruction:    "Use only the advertised tools. Read each listed input at /sworn/inputs/ followed by that input's path. Finish with exactly one terminal: use sworn_submit with this envelope's exact invocation_id and responsibility when the work result is complete, or sworn_yield with the exact invocation_id when a bounded question or real block prevents completion. Then stop.",
	}
	if invocation.recoverableInput != nil {
		if err := ValidateRecoverableTurnInput(
			*invocation.recoverableInput,
		); err != nil {
			return nil, err
		}
		content := invocation.recoverableInput.Answer
		if invocation.recoverableInput.Kind == RecoverableInputNudge {
			content = recoverableTurnNudge
		}
		envelope.Recovery = &struct {
			Kind    RecoverableInputKind `json:"kind"`
			Content string               `json:"content"`
		}{
			Kind: invocation.recoverableInput.Kind, Content: content,
		}
	}
	body, err := json.Marshal(envelope)
	if err != nil || len(body) > MaxProviderRequestBytes {
		return nil, fail("RESOURCE_LIMIT")
	}
	return body, nil
}

func submissionResultFields(responsibility Responsibility) []string {
	fields := []string{"summary", "detail"}
	switch responsibility {
	case PlannerProposal:
		return append(fields, "plan")
	case ImplementerImplementation:
		return append(fields, "checks")
	case CaptainReview, CaptainPlanReview:
		return append(fields, "decision")
	case WorkVerification, AssemblyVerification:
		return append(fields, "checks", "decision")
	default:
		return fields
	}
}

func clearBytes(body []byte) {
	for index := range body {
		body[index] = 0
	}
}
