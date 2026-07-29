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

type loopAdapter struct {
	identity  AdapterIdentity
	family    ProfileFamily
	surface   ProfileSurface
	new       providerConversationFactory
	transport providerTransport
}

func newLoopAdapter(
	key, id, version string,
	family ProfileFamily,
	surface ProfileSurface,
	configuration any,
	factory providerConversationFactory,
	transport providerTransport,
) (*loopAdapter, error) {
	if !providerKeyPattern.MatchString(key) ||
		!driverIdentityPattern.MatchString(id) ||
		!versionPattern.MatchString(version) ||
		!family.valid() || family == ProfileFake ||
		!surface.validFor(family) ||
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
	session, err := newToolSession(invocation)
	if err != nil {
		return Observation{}, err
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil {
			observation.Handoff = nil
			resultErr = joinErrors(resultErr, closeErr)
		}
	}()
	var total Usage
	usageAvailable := false
	seenIDs := make(map[string]struct{})
	for turn := 0; turn < MaxProviderTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return Observation{}, err
		}
		request, err := conversation.request()
		if err != nil || len(request.Body) > MaxProviderRequestBytes {
			return Observation{}, fail("CONTINUATION_INVALID")
		}
		response, err := adapter.transport.roundTrip(
			ctx,
			invocation.Selected.Profile.CredentialRef,
			request,
		)
		clearBytes(request.Body)
		if err != nil {
			return Observation{}, err
		}
		providerTurn, err := conversation.accept(response)
		clearBytes(response)
		if err != nil {
			return Observation{}, err
		}
		if providerTurn.Usage != nil {
			if providerTurn.Usage.InputTokens < 0 || providerTurn.Usage.OutputTokens < 0 ||
				total.InputTokens > math.MaxInt64-providerTurn.Usage.InputTokens ||
				total.OutputTokens > math.MaxInt64-providerTurn.Usage.OutputTokens {
				return Observation{}, fail("INVALID_USAGE")
			}
			total.InputTokens += providerTurn.Usage.InputTokens
			total.OutputTokens += providerTurn.Usage.OutputTokens
			usageAvailable = true
		}
		if len(providerTurn.Calls) == 0 {
			return Observation{}, fail("MISSING_SUBMISSION")
		}
		if len(providerTurn.Calls) > MaxToolCalls {
			return Observation{}, fail("RESOURCE_LIMIT")
		}
		submitCalls := 0
		for _, call := range providerTurn.Calls {
			if call.Name == "sworn_submit" {
				submitCalls++
			}
		}
		if submitCalls > 0 && (submitCalls != 1 || len(providerTurn.Calls) != 1) {
			return Observation{}, fail("SUBMISSION_PROTOCOL_FAILED")
		}
		results := make([]providerToolResult, 0, len(providerTurn.Calls))
		for _, call := range providerTurn.Calls {
			if validateProviderToolCall(call, seenIDs) != nil {
				return Observation{}, fail("CONTINUATION_INVALID")
			}
			results = append(results, session.execute(ctx, call))
		}
		if submitted, submitErr := session.submitted(); submitted || submitErr != nil {
			if submitErr != nil {
				return Observation{}, submitErr
			}
			if closeErr := session.Close(); closeErr != nil {
				return Observation{}, closeErr
			}
			usage, err := NormalizeUsage(nil, nil)
			if usageAvailable {
				usage, err = NormalizeUsage(&total, nil)
			}
			if err != nil {
				return Observation{}, err
			}
			return completedToolObservation(started, usage, session.handoff()), nil
		}
		if err := conversation.appendResults(results); err != nil {
			return Observation{}, err
		}
	}
	return Observation{}, fail("RESOURCE_LIMIT")
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
