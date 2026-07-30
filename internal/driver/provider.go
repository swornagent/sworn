package driver

import (
	"context"
	"encoding/json"
	"math"
	"time"
)

const (
	MaxProviderTurns         = 32
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
}

type providerConversation interface {
	request() (providerRequest, error)
	accept([]byte) (providerTurn, error)
	appendResults([]providerToolResult) error
	resume([]byte, []providerToolDefinition) error
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
	providerDialectDeepSeekChat    providerDialect = "deepseek_chat"
	providerDialectGemini          providerDialect = "gemini"
	providerDialectBedrockConverse providerDialect = "bedrock_converse"
	providerDialectMantleChat      providerDialect = "mantle_chat"
)

func (dialect providerDialect) continuationMode() ContinuationMode {
	switch dialect {
	case providerDialectOpenAIChat, providerDialectMantleChat:
		return ContinuationModeTranscriptReplay
	case providerDialectOpenAIResponses,
		providerDialectOpenRouterChat,
		providerDialectDeepSeekChat,
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

func (adapter *loopAdapter) resumeContinuation(
	ctx context.Context,
	invocation Invocation,
	prior continuationState,
) (Observation, error) {
	started := time.Now()
	state, ok := prior.(*apiContinuationState)
	if adapter == nil || !ok || state == nil || state.closed ||
		state.conversation == nil ||
		state.dialect != adapter.dialect ||
		state.mode != adapter.dialect.continuationMode() ||
		state.bytes < 1 || state.bytes > MaxProviderRequestBytes ||
		invocation.Selected.Adapter != adapter.identity ||
		invocation.Selected.Profile.Network != NetworkRequired {
		return Observation{}, fail("CONTINUATION_INVALID")
	}
	prompt, err := modelPrompt(invocation)
	if err != nil {
		return Observation{}, fail("CONTINUATION_INVALID")
	}
	err = state.conversation.resume(
		prompt,
		toolDefinitions(invocation.Request.Workspace.Access),
	)
	clearBytes(prompt)
	if err != nil {
		return Observation{}, fail("CONTINUATION_INVALID")
	}
	request, err := state.conversation.request()
	if err != nil || len(request.Body) < 1 ||
		len(request.Body) > MaxProviderRequestBytes {
		clearBytes(request.Body)
		return Observation{}, fail("CONTINUATION_INVALID")
	}
	defer clearBytes(request.Body)
	observation, _, resultErr := adapter.runConversation(
		ctx,
		started,
		invocation,
		state.conversation,
		&request,
		false,
	)
	// CONTINUATION_INVALID is reserved for rejection before provider or tool
	// effects so the dispatcher can safely request one fresh rehydration.
	if IsCode(resultErr, "CONTINUATION_INVALID") {
		resultErr = fail("PROTOCOL_FAILURE")
	}
	return observation, resultErr
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
			if state != nil {
				_ = closeContinuationState(state)
				state = nil
			}
			resultErr = joinErrors(resultErr, closeErr)
		}
	}()
	var total Usage
	usageAvailable := false
	seenIDs := make(map[string]struct{})
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
			if providerTurn.Usage.InputTokens < 0 || providerTurn.Usage.OutputTokens < 0 ||
				total.InputTokens > math.MaxInt64-providerTurn.Usage.InputTokens ||
				total.OutputTokens > math.MaxInt64-providerTurn.Usage.OutputTokens {
				return Observation{}, nil, fail("INVALID_USAGE")
			}
			total.InputTokens += providerTurn.Usage.InputTokens
			total.OutputTokens += providerTurn.Usage.OutputTokens
			usageAvailable = true
		}
		if len(providerTurn.Calls) == 0 {
			return Observation{}, nil, fail("MISSING_SUBMISSION")
		}
		if len(providerTurn.Calls) > MaxToolCalls {
			return Observation{}, nil, fail("RESOURCE_LIMIT")
		}
		submitCalls := 0
		for _, call := range providerTurn.Calls {
			if call.Name == "sworn_submit" {
				submitCalls++
			}
		}
		if submitCalls > 0 && (submitCalls != 1 || len(providerTurn.Calls) != 1) {
			return Observation{}, nil, fail("SUBMISSION_PROTOCOL_FAILED")
		}
		results := make([]providerToolResult, 0, len(providerTurn.Calls))
		for _, call := range providerTurn.Calls {
			if validateProviderToolCall(call, seenIDs) != nil {
				return Observation{}, nil, fail("CONTINUATION_INVALID")
			}
			results = append(results, session.execute(ctx, call))
		}
		if submitted, submitErr := session.submitted(); submitted || submitErr != nil {
			if submitErr != nil {
				return Observation{}, nil, submitErr
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
			observation := completedToolObservation(
				started,
				usage,
				session.handoff(),
			)
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
	case "Bash", "Read", "Write", "Edit", "Glob", "Grep", "sworn_submit":
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
	}
	body, err := json.Marshal(promptEnvelope{
		SchemaVersion:  "sworn.model-prompt/v1",
		InvocationID:   invocation.Request.InvocationID,
		Role:           invocation.Request.Role,
		Operation:      invocation.Request.Operation,
		Workspace:      invocation.Request.Workspace,
		Inputs:         invocation.Request.Inputs,
		Responsibility: descriptor.Responsibility,
		ResultFields:   submissionResultFields(descriptor.Responsibility),
		Instruction:    "Use only the advertised tools. Read each listed input at /sworn/inputs/ followed by that input's path. Copy this envelope's exact invocation_id and responsibility into sworn_submit, call it exactly once, and then stop.",
	})
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
	case CaptainReview:
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
