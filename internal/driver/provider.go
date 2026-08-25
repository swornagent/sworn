package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	// MaxProviderTurns is a runaway-loop guard, not a work budget. A careful
	// implementer pass over this repo needs ~30 tool turns, so any value that
	// could bind honest work is too low; recap only from eval evidence.
	MaxProviderTurns = 1_000
	// MaxProviderRequestBytes must never end a conversation the provider's
	// own context window still accepts: a large implementation transcript
	// crosses 1MB of request JSON at roughly 250K tokens, observed live on
	// S2's build. The provider is the authority on its window; this guard
	// only bounds pathological growth. Context compaction is the real fix.
	MaxProviderRequestBytes  = 8_388_608
	MaxProviderResponseBytes = 1_048_576
	// MaxToolCallCorrections bounds per-dispatch grace for a malformed
	// provider tool call, in the MaxSubmissionCorrections house pattern:
	// bounded by turn budget/timeout, not a hard allowance. It is kept
	// strictly below MaxProviderTurns so persistent malformation always
	// falls through to the original continuation.toolcall_decode
	// classification from inside the runaway-loop guard's own iteration
	// bound, never past it into a generic RESOURCE_LIMIT.
	MaxToolCallCorrections = MaxProviderTurns - 1
)

type providerRequest struct {
	Method      string
	URL         string
	ContentType string
	Headers     map[string]string
	Body        []byte
	// Stream marks a request whose response arrives as SSE events; the
	// transport renders them live and returns the terminal event's embedded
	// response object as the body, so validation is unchanged.
	Stream bool
	// StreamFormat names the SSE dialect of a streamed request: "" is the
	// responses-flavour SSE and "gemini" is the generateContent SSE. Every
	// other adapter leaves it empty, so its behavior is untouched.
	StreamFormat string
	// StreamModel is the presentation label for streamed deltas (the model
	// shown in the live turn header). It rides for rendering only; nothing
	// in validation or dispatch semantics reads it.
	StreamModel string
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
	// Cost is a typed provider-reported observation for this turn when an
	// adapter captured one (OpenRouter usage.cost). Nil is honest absence
	// and preserves today's receipts.
	Cost *CostObservation
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
	limits Limits,
) (providerConversation, error)

// optionalOutputLimit reads the optional trailing output-limit argument the
// conversation constructors accept: absent means zero (the field is omitted
// from every request surface), and a present value must stay inside the same
// bound ValidateRequest already enforces on Limits.OutputBytes.
func optionalOutputLimit(values []int64) (int64, error) {
	switch len(values) {
	case 0:
		return 0, nil
	case 1:
		if values[0] < 0 || values[0] > MaxProviderOutputBytes {
			return 0, fail("INVALID_ADAPTER")
		}
		return values[0], nil
	default:
		return 0, fail("INVALID_ADAPTER")
	}
}

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
	providerDialectGoogleChat      providerDialect = "google_chat"
	providerDialectXAIChat         providerDialect = "xai_chat"
	providerDialectXAIResponses    providerDialect = "xai_responses"
	providerDialectGemini          providerDialect = "gemini"
	providerDialectBedrockConverse providerDialect = "bedrock_converse"
)

func (dialect providerDialect) continuationMode() ContinuationMode {
	switch dialect {
	case providerDialectOpenAIChat, providerDialectXAIChat:
		return ContinuationModeTranscriptReplay
	case providerDialectOpenAIResponses,
		providerDialectOpenRouterChat,
		providerDialectOpaqueChat,
		providerDialectGoogleChat,
		providerDialectXAIResponses,
		providerDialectGemini,
		providerDialectBedrockConverse:
		return ContinuationModeOpaqueReplay
	default:
		return ""
	}
}

// ContinuationPosture declares whether fresh rehydration is ordinary
// operation for an adapter. Degradation counting reads this declaration: an
// adapter that rehydrates by design accumulates zero degradation budget from
// transport churn, while a continuation-bearing adapter losing a retained
// session still counts. An adapter that declares nothing is read as
// context_retaining (fail-closed).
type ContinuationPosture string

const (
	ContinuationPostureContextRetaining ContinuationPosture = "context_retaining"
	ContinuationPostureFreshByDesign    ContinuationPosture = "fresh_by_design"
)

// continuationPosture sits beside continuationMode: the mode says how a
// retained conversation replays, the posture says whether losing that replay
// is degradation at all. Gemini is the google-native stateless per-request
// surface whose engine continuation is a replay cache (sworn#227), so fresh
// rehydration is its ordinary operation; every other dialect retains context
// by design and loses real session state on a fresh rehydrate.
func (dialect providerDialect) continuationPosture() ContinuationPosture {
	switch dialect {
	case providerDialectGemini:
		return ContinuationPostureFreshByDesign
	case providerDialectOpenAIResponses,
		providerDialectOpenAIChat,
		providerDialectOpenRouterChat,
		providerDialectOpaqueChat,
		providerDialectGoogleChat,
		providerDialectXAIChat,
		providerDialectXAIResponses,
		providerDialectBedrockConverse:
		return ContinuationPostureContextRetaining
	default:
		return ContinuationPostureContextRetaining
	}
}

type loopAdapter struct {
	identity  AdapterIdentity
	family    ProfileFamily
	surface   ProfileSurface
	dialect   providerDialect
	new       providerConversationFactory
	transport providerTransport
	// pacingCap is the operator-configured provider input-tokens-per-minute
	// quota; zero disables proactive pacing (reactive 429 pacing always
	// applies).
	pacingCap int64
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

// declaredContinuationPosture implements the private opt-in capability the
// Dispatcher reads through ContinuationPosture. It is a per-dialect
// declaration, never a per-dispatch inference.
func (adapter *loopAdapter) declaredContinuationPosture() ContinuationPosture {
	if adapter == nil {
		return ContinuationPostureContextRetaining
	}
	return adapter.dialect.continuationPosture()
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
		invocation.Request.Limits,
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
		invocation.Request.Limits,
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
		liveStream.driverError("resume-binding", nil)
		return Observation{}, nil, failContinuation("continuation.provider.resume_state_invalid")
	}
	prompt, err := modelPrompt(invocation)
	if err != nil {
		liveStream.driverError("resume-prompt", err)
		return Observation{}, nil, failContinuation("continuation.provider.resume_prompt_build_failed")
	}
	err = state.conversation.resume(
		prompt,
		toolDefinitions(invocation.Request.Workspace.Access),
	)
	clearBytes(prompt)
	if err != nil {
		liveStream.driverError("resume-conversation", err)
		return Observation{}, nil, failContinuation("continuation.provider.resume_conversation_failed")
	}
	request, err := state.conversation.request()
	if err != nil || len(request.Body) < 1 ||
		len(request.Body) > MaxProviderRequestBytes {
		clearBytes(request.Body)
		liveStream.driverError("resume-request", err)
		return Observation{}, nil, failContinuation("continuation.provider.resume_request_build_failed")
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
	// The operator-declared TimeoutMillis now bounds the API conversation
	// path's wall clock (A3). ValidateRequest guarantees TimeoutMillis >= 1,
	// so the wrapped deadline can never be zero. The shared HTTP client
	// keeps Timeout:0 deliberately: a client-level timeout would cut
	// individual streamed requests per-request, while this dispatch-level
	// deadline bounds the whole conversation. The caller's context is no
	// longer passed straight through.
	loopCtx, cancel := providerTimeout(ctx, invocation.Request.Limits.TimeoutMillis)
	ctx = loopCtx
	defer cancel()
	turnBudget := invocation.Request.Limits.EffectiveMaxTurnsPerWork()
	tokenBudget := invocation.Request.Limits.EffectiveMaxOutputTokensPerWork()
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
	var totalCost *CostObservation
	usageAvailable := false
	effortRequested := conversation.declaredReasoningEffort()
	var effortReported *string
	seenIDs := make(map[string]struct{})
	proseNudges := 0
	toolCallCorrections := 0
	// Turn economics are engine-counted facts: every accepted provider turn
	// counts (prose nudges are turns with zero calls), and every executed
	// tool call counts by canonical name.
	var turnCount, toolCallCount int64
	toolCallsByName := make(map[string]int64)
	pacer := newInputTokenPacer(adapter.pacingCap)
	pacedBudget := MaxProviderPacedWait
	for turn := 0; turn < MaxProviderTurns; turn++ {
		// Per-work economy budgets (A1) are evaluated at the loop-top safe
		// boundary: the previous turn's tool results are already appended
		// and observed, and the next request is not yet built or sent. A
		// terminal submit/yield that lands on this turn's crossing already
		// returned above, so crossing never kills finished work.
		if turnCount >= turnBudget {
			return economyBudgetFailure(
				started,
				adapter.identity.ID,
				&total,
				usageAvailable,
				effortRequested,
				effortReported,
				turnCount,
				toolCallCount,
				toolCallsByName,
				totalCost,
				"economy_turn_budget",
			), nil, fail("ECONOMY_TURN_BUDGET_EXCEEDED")
		}
		if usageAvailable && total.OutputTokens >= tokenBudget {
			return economyBudgetFailure(
				started,
				adapter.identity.ID,
				&total,
				usageAvailable,
				effortRequested,
				effortReported,
				turnCount,
				toolCallCount,
				toolCallsByName,
				totalCost,
				"economy_output_budget",
			), nil, fail("ECONOMY_OUTPUT_BUDGET_EXCEEDED")
		}
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
			liveStream.driverError("request-build", err)
			return Observation{}, nil, failContinuation("continuation.provider.loop_request_build_failed")
		}
		if wait := pacer.waitBefore(
			pacer.estimate(int64(len(request.Body))/4), time.Now(),
		); wait > 0 {
			if sleepErr := contextSleep(ctx, wait); sleepErr != nil {
				clearBytes(request.Body)
				return Observation{}, nil, sleepErr
			}
		}
		response, err := pacedRoundTrip(
			ctx,
			func() ([]byte, error) {
				return adapter.transport.roundTrip(
					ctx,
					invocation.Selected.Profile.CredentialRef,
					request,
				)
			},
			&pacedBudget,
			func(limited error) { liveStream.driverError("transport-paced", limited) },
			contextSleep,
		)
		clearBytes(request.Body)
		if err != nil {
			liveStream.driverError("transport", err)
			return Observation{}, nil, err
		}
		providerTurn, err := conversation.accept(response)
		clearBytes(response)
		if err != nil {
			if detail, correctable := toolCallDecodeDetail(err); correctable &&
				toolCallCorrections < MaxToolCallCorrections {
				toolCallCorrections++
				turnCount++
				if recoveryErr := reserveRecoveryStep(
					ctx,
					invocation.RecoveryStepHook,
					RecoveryStepMalformedToolCall,
				); recoveryErr != nil {
					return Observation{}, nil, recoveryErr
				}
				if instructionErr := conversation.appendInstruction(
					malformedToolCallInstruction(detail),
				); instructionErr != nil {
					return Observation{}, nil, instructionErr
				}
				continue
			}
			liveStream.driverError("accept", err)
			return Observation{}, nil, err
		}
		turnCount++
		for _, call := range providerTurn.Calls {
			toolCallCount++
			toolCallsByName[call.Name]++
		}
		if providerTurn.Usage != nil {
			if err := addTurnUsage(&total, providerTurn.Usage); err != nil {
				return Observation{}, nil, err
			}
			usageAvailable = true
			pacer.record(providerTurn.Usage.InputTokens, time.Now())
		}
		if err := addTurnCost(&totalCost, providerTurn.Cost); err != nil {
			return Observation{}, nil, err
		}
		if providerTurn.ReasoningEffort != nil {
			value := *providerTurn.ReasoningEffort
			effortReported = &value
		}
		if providerTurn.Truncated {
			usage, err := NormalizeUsage(nil, totalCost, adapter.identity.ID)
			if usageAvailable {
				usage, err = NormalizeUsage(&total, totalCost, adapter.identity.ID)
			}
			if err != nil {
				return Observation{}, nil, err
			}
			applyTurnEconomics(&usage, turnCount, toolCallCount, toolCallsByName)
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
			if callErr := validateProviderToolCall(call, seenIDs); callErr != nil {
				liveStream.driverError("tool-call "+call.Name, callErr)
				return Observation{}, nil, failContinuation("continuation.provider.tool_call_invalid")
			}
			results = append(results, session.execute(ctx, call))
		}
		if terminated, terminalErr := session.terminated(); terminated {
			if terminalErr != nil {
				return Observation{}, nil, terminalErr
			}
			if retain {
				// Project the exact results slice this appendResults
				// crossing receives, ahead of per-dialect formatting, so
				// the observed bytes are the model-facing bytes. A
				// !retain terminal turn appends nothing to any model and
				// therefore emits nothing.
				session.observeToolResultTurn(turnCount, results)
				if err := conversation.appendResults(results); err != nil {
					liveStream.driverError("append-results-terminal", err)
					return Observation{}, nil, err
				}
			}
			if closeErr := session.Close(); closeErr != nil {
				return Observation{}, nil, closeErr
			}
			usage, err := NormalizeUsage(nil, totalCost, adapter.identity.ID)
			if usageAvailable {
				usage, err = NormalizeUsage(&total, totalCost, adapter.identity.ID)
			}
			if err != nil {
				return Observation{}, nil, err
			}
			applyTurnEconomics(&usage, turnCount, toolCallCount, toolCallsByName)
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
		// Project the exact results slice this appendResults crossing
		// receives, ahead of per-dialect formatting, so the observed
		// bytes are the model-facing bytes.
		session.observeToolResultTurn(turnCount, results)
		if err := conversation.appendResults(results); err != nil {
			liveStream.driverError("append-results", err)
			return Observation{}, nil, err
		}
	}
	return Observation{}, nil, fail("RESOURCE_LIMIT")
}

// economyBudgetFailure builds the named economy-budget failure observation
// (A1). It mirrors the provider-truncated receipt exactly: the accumulated
// usage with the engine-counted turn economics and invocation facts, so the
// spent-versus-budget evidence survives the failure-sanitization seam and
// lands durably in the attempt BLOB the runtime park gate reads back.
func economyBudgetFailure(
	started time.Time,
	adapterID string,
	total *Usage,
	usageAvailable bool,
	effortRequested string,
	effortReported *string,
	turns, toolCalls int64,
	toolCallsByName map[string]int64,
	cost *CostObservation,
	diagnostic string,
) Observation {
	usage, err := NormalizeUsage(nil, cost, adapterID)
	if usageAvailable && total != nil {
		usage, err = NormalizeUsage(total, cost, adapterID)
	}
	if err != nil {
		// Accumulated facts outside their bounds cannot yield a canonical
		// receipt; the unavailable receipt still names the crossing loudly
		// rather than defaulting silent.
		usage, _ = NormalizeUsage(nil, nil, adapterID)
	}
	applyTurnEconomics(&usage, turns, toolCalls, toolCallsByName)
	applyInvocationFacts(&usage, effortRequested, effortReported, nil, false)
	return Observation{
		TransportStatus: RunnerError,
		DurationMillis:  time.Since(started).Milliseconds(),
		Usage:           usage,
		Diagnostic:      Diagnostic{Code: diagnostic},
	}
}

const providerProseNudge = "Finish this turn now by calling exactly one advertised Sworn terminal tool. Do not answer in prose."

// toolCallDecodeDetail reports whether err is a decode failure this loop may
// correct: a CONTINUATION_INVALID whose detail carries the
// continuation.toolcall_decode. prefix - the sole label family
// responsesFunctionCall emits (every other decoder, and every other
// continuation failure, fails under its own distinct label and is never
// intercepted here).
func toolCallDecodeDetail(err error) (string, bool) {
	var contractErr *ContractError
	if errors.As(err, &contractErr) &&
		contractErr.Code == "CONTINUATION_INVALID" &&
		strings.HasPrefix(contractErr.Detail, "continuation.toolcall_decode.") {
		return contractErr.Detail, true
	}
	return "", false
}

func malformedToolCallInstruction(detail string) []byte {
	return []byte(fmt.Sprintf(
		"Your last turn's tool call was malformed (defect: %s). Re-emit this entire turn now with every intended tool call well-formed: a valid tool name, valid JSON arguments, and a valid call id. Do not describe the defect in prose.",
		detail,
	))
}

func validateProviderToolCall(call providerToolCall, seen map[string]struct{}) error {
	if validateText(call.ID, MaxCorrelationIDBytes, false) != nil ||
		!providerKeyPattern.MatchString(call.Name) ||
		len(call.Arguments) == 0 || len(call.Arguments) > MaxToolArgumentBytes {
		return failContinuation("continuation.provider.tool_call_fields_invalid")
	}
	if _, exists := seen[call.ID]; exists {
		return failContinuation("continuation.provider.tool_call_duplicate_id")
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
	// Reasoning tokens are summed across turns exactly like cache reads, so
	// the invocation total carries every turn's reported thinking.
	if turn.ReasoningTokens != nil {
		if *turn.ReasoningTokens < 0 {
			return fail("INVALID_USAGE")
		}
		if total.ReasoningTokens == nil {
			reasoning := *turn.ReasoningTokens
			total.ReasoningTokens = &reasoning
		} else if *total.ReasoningTokens > math.MaxInt64-*turn.ReasoningTokens {
			return fail("INVALID_USAGE")
		} else {
			*total.ReasoningTokens += *turn.ReasoningTokens
		}
	}
	return nil
}

// addTurnCost accumulates one provider-reported cost observation into the
// invocation total. Only same-currency provider_reported observations sum;
// mixed currency, mixed source, or overflow fail closed. Nil is honest
// absence and leaves the running total untouched.
func addTurnCost(total **CostObservation, turn *CostObservation) error {
	if turn == nil {
		return nil
	}
	if total == nil ||
		turn.Source != CostSourceProviderReported ||
		turn.MicroUnits < 0 || turn.MicroUnits > MaxSafeInteger ||
		!currencyPattern.MatchString(turn.Currency) {
		return fail("INVALID_USAGE")
	}
	if *total == nil {
		cloned := *turn
		*total = &cloned
		return nil
	}
	current := *total
	if current.Currency != turn.Currency || current.Source != turn.Source {
		return fail("INVALID_USAGE")
	}
	if current.MicroUnits > MaxSafeInteger-turn.MicroUnits {
		return fail("INVALID_USAGE")
	}
	current.MicroUnits += turn.MicroUnits
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
		SchemaVersion     string         `json:"schema_version"`
		InvocationID      string         `json:"invocation_id"`
		Role              Role           `json:"role"`
		Operation         Operation      `json:"operation"`
		Workspace         Workspace      `json:"workspace"`
		Inputs            []Input        `json:"inputs"`
		Responsibility    Responsibility `json:"responsibility"`
		ResultFields      []string       `json:"result_fields"`
		Instruction       string         `json:"instruction"`
		RoleAssetAddendum *Addendum      `json:"role_asset_addendum,omitempty"`
		Recovery          *struct {
			Kind    RecoverableInputKind `json:"kind"`
			Content string               `json:"content"`
		} `json:"recovery,omitempty"`
	}
	envelope := promptEnvelope{
		SchemaVersion:     "sworn.model-prompt/v1",
		InvocationID:      invocation.Request.InvocationID,
		Role:              invocation.Request.Role,
		Operation:         invocation.Request.Operation,
		Workspace:         invocation.Request.Workspace,
		Inputs:            invocation.Request.Inputs,
		Responsibility:    descriptor.Responsibility,
		ResultFields:      submissionResultFields(descriptor.Responsibility),
		Instruction:       "Use only the advertised tools. Read each listed input at /sworn/inputs/ followed by that input's path. Scratch output such as check logs belongs under the workspace tmp/ directory, which never enters the submitted candidate; every other workspace change must stay inside the slice scope, because the candidate is judged on its full diff. Finish with exactly one terminal: use sworn_submit with this envelope's exact invocation_id and responsibility when the work result is complete, or sworn_yield with the exact invocation_id when a bounded question or real block prevents completion. Then stop.",
		RoleAssetAddendum: RoleAssetAddendum(invocation.Request.Role),
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
