//go:build linux

package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	w8CorpusPass       = "PASS"
	w8CorpusModel      = "w8-explicit-model"
	w8CorpusSentinel   = "w8-transport-text-must-not-escape"
	w8CorpusResultBody = "w8 exact implementation result\n"
)

type w8CorpusRecord struct {
	Target string
	Case   string
	Status string
}

type w8CorpusTarget struct {
	name       string
	family     ProfileFamily
	surface    ProfileSurface
	profile    ProfileConfig
	adapter    *w8CountedAdapter
	transport  *w8ProviderRoundTripper
	credential string
}

type w8CountedAdapter struct {
	delegate    Adapter
	invocations atomic.Int64
}

func (adapter *w8CountedAdapter) Identity() AdapterIdentity {
	return adapter.delegate.Identity()
}

func (adapter *w8CountedAdapter) invoke(
	ctx context.Context,
	invocation Invocation,
) (Observation, error) {
	adapter.invocations.Add(1)
	return adapter.delegate.invoke(ctx, invocation)
}

func (adapter *w8CountedAdapter) profileFamily() ProfileFamily {
	return adapter.delegate.(profileChecker).profileFamily()
}

func (adapter *w8CountedAdapter) profileSurface() ProfileSurface {
	if reporter, ok := adapter.delegate.(profileSurfaceReporter); ok {
		return reporter.profileSurface()
	}
	return ""
}

func (adapter *w8CountedAdapter) checkProfile(
	ctx context.Context,
	kind profileCheckKind,
	profile ProfileConfig,
	model string,
) (ReadinessState, string) {
	return adapter.delegate.(profileChecker).checkProfile(
		ctx,
		kind,
		profile,
		model,
	)
}

type w8ProviderCodec string

const (
	w8OpenAICodec    w8ProviderCodec = "openai"
	w8ResponsesCodec w8ProviderCodec = "responses"
	w8GeminiCodec    w8ProviderCodec = "gemini"
	w8BedrockCodec   w8ProviderCodec = "bedrock"
)

type w8ProviderRoundTripper struct {
	codec          w8ProviderCodec
	scenario       string
	submission     []byte
	requests       int
	explicitModel  string
	transportInput []byte
}

func (transport *w8ProviderRoundTripper) prepare(
	scenario string,
	submission []byte,
) {
	transport.scenario = scenario
	transport.submission = append(transport.submission[:0], submission...)
	transport.requests = 0
	transport.explicitModel = ""
	transport.transportInput = transport.transportInput[:0]
}

func (transport *w8ProviderRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	transport.requests++
	body, err := io.ReadAll(io.LimitReader(
		request.Body,
		int64(MaxProviderRequestBytes)+1,
	))
	if err != nil {
		return nil, err
	}
	transport.transportInput = append(transport.transportInput[:0], body...)
	switch transport.codec {
	case w8OpenAICodec, w8ResponsesCodec:
		var envelope struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(body, &envelope) == nil {
			transport.explicitModel = envelope.Model
		}
	case w8GeminiCodec:
		if strings.Contains(request.URL.Path, "/"+w8CorpusModel+":generateContent") {
			transport.explicitModel = w8CorpusModel
		}
	case w8BedrockCodec:
		if strings.Contains(request.URL.Path, "/model/"+w8CorpusModel+"/converse") {
			transport.explicitModel = w8CorpusModel
		}
	}

	if transport.scenario == "p07-block" {
		<-request.Context().Done()
		return nil, request.Context().Err()
	}
	if transport.scenario == "p07-overflow" {
		return w8HTTPResponse(
			bytes.Repeat([]byte{'x'}, MaxProviderResponseBytes+1),
		), nil
	}
	if transport.scenario == "p06-malformed" {
		return w8HTTPResponse([]byte(`{"malformed":true}`)), nil
	}

	name := "sworn_submit"
	arguments := w8SubmissionArguments(transport.submission)
	if (transport.scenario == "p09-write" ||
		transport.scenario == "p10-read-only") &&
		transport.requests == 1 {
		name = "Write"
		arguments = map[string]any{
			"path":    GuestWorkspacePath + "/result.txt",
			"content": w8CorpusResultBody,
		}
	}
	response, err := transport.toolCallResponse(
		fmt.Sprintf("w8-call-%d", transport.requests),
		name,
		arguments,
		transport.scenario == "p05-opaque",
	)
	if err != nil {
		return nil, err
	}
	return w8HTTPResponse(response), nil
}

func (transport *w8ProviderRoundTripper) toolCallResponse(
	id string,
	name string,
	arguments any,
	includeOpaqueText bool,
) ([]byte, error) {
	var value any
	switch transport.codec {
	case w8ResponsesCodec:
		argumentBody, err := json.Marshal(arguments)
		if err != nil {
			return nil, err
		}
		summary := []any{}
		if includeOpaqueText {
			summary = append(summary, map[string]any{
				"type": "summary_text",
				"text": w8CorpusSentinel,
			})
		}
		value = map[string]any{
			"id":     fmt.Sprintf("w8-response-%d", transport.requests),
			"object": "response",
			"status": "completed",
			"error":  nil,
			"output": []any{
				map[string]any{
					"type":              "reasoning",
					"id":                fmt.Sprintf("w8-reasoning-%d", transport.requests),
					"status":            "completed",
					"summary":           summary,
					"content":           []any{},
					"encrypted_content": fmt.Sprintf("w8-encrypted-%d", transport.requests),
				},
				map[string]any{
					"type":      "function_call",
					"call_id":   id,
					"name":      name,
					"arguments": string(argumentBody),
					"status":    "completed",
				},
			},
		}
	case w8OpenAICodec:
		argumentBody, err := json.Marshal(arguments)
		if err != nil {
			return nil, err
		}
		var content any
		if includeOpaqueText {
			content = w8CorpusSentinel
		}
		value = map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
					"tool_calls": []any{map[string]any{
						"id": id, "type": "function",
						"function": map[string]any{
							"name": name, "arguments": string(argumentBody),
						},
					}},
				},
				"finish_reason": "tool_calls",
			}},
		}
	case w8GeminiCodec:
		parts := make([]any, 0, 2)
		if includeOpaqueText {
			parts = append(parts, map[string]any{"text": w8CorpusSentinel})
		}
		parts = append(parts, map[string]any{"functionCall": map[string]any{
			"id": id, "name": name, "args": arguments,
		}})
		value = map[string]any{
			"candidates": []any{map[string]any{
				"content": map[string]any{
					"role": "model", "parts": parts,
				},
				"finishReason": "STOP",
			}},
		}
	case w8BedrockCodec:
		content := make([]any, 0, 2)
		if includeOpaqueText {
			content = append(content, map[string]any{"text": w8CorpusSentinel})
		}
		content = append(content, map[string]any{"toolUse": map[string]any{
			"toolUseId": id, "name": name, "input": arguments,
		}})
		value = map[string]any{
			"output": map[string]any{"message": map[string]any{
				"role": "assistant", "content": content,
			}},
			"stopReason": "tool_use",
		}
	default:
		return nil, fmt.Errorf("unknown provider codec %q", transport.codec)
	}
	return json.Marshal(value)
}

func w8HTTPResponse(body []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func w8SubmissionArguments(body []byte) map[string]any {
	var submission any
	if json.Unmarshal(bytes.TrimSpace(body), &submission) != nil {
		panic("invalid W8 submission fixture")
	}
	return map[string]any{"submission": submission}
}

func TestW8SharedProductionCorpusHasExactSeventyPassRecords(t *testing.T) {
	nativeBinary := w8BuildNativeFixture(t)
	targets := []*w8CorpusTarget{
		w8NewNativeTarget(t, "codex", ProfileCodex, nativeBinary),
		w8NewNativeTarget(t, "claude", ProfileClaude, nativeBinary),
		w8NewProviderTarget(
			t,
			"openai",
			ProfileOpenAIHTTP,
			ProfileSurfaceOpenAIResponses,
			w8ResponsesCodec,
		),
		w8NewProviderTarget(t, "deepseek", ProfileDeepSeek, "", w8OpenAICodec),
		w8NewProviderTarget(t, "gemini", ProfileGemini, "", w8GeminiCodec),
		w8NewProviderTarget(
			t,
			"bedrock-runtime",
			ProfileBedrock,
			ProfileSurfaceBedrockRuntimeConverse,
			w8BedrockCodec,
		),
		w8NewProviderTarget(
			t,
			"mantle",
			ProfileBedrock,
			ProfileSurfaceBedrockMantleChat,
			w8OpenAICodec,
		),
	}
	cases := []struct {
		id  string
		run func(*testing.T, *w8CorpusTarget)
	}{
		{"P01", w8CorpusP01},
		{"P02", w8CorpusP02},
		{"P03", w8CorpusP03},
		{"P04", w8CorpusP04},
		{"P05", w8CorpusP05},
		{"P06", w8CorpusP06},
		{"P07", w8CorpusP07},
		{"P08", w8CorpusP08},
		{"P09", w8CorpusP09},
		{"P10", w8CorpusP10},
	}

	records := make([]w8CorpusRecord, 0, len(targets)*len(cases))
	for _, target := range targets {
		for _, corpusCase := range cases {
			executed := false
			passed := t.Run(
				target.name+"/"+corpusCase.id,
				func(t *testing.T) {
					executed = true
					corpusCase.run(t, target)
				},
			)
			status := "NOT_RUN"
			if executed && passed {
				status = w8CorpusPass
			} else if executed {
				status = "FAIL"
			}
			records = append(records, w8CorpusRecord{
				Target: target.name,
				Case:   corpusCase.id,
				Status: status,
			})
		}
	}

	targetNames := make([]string, len(targets))
	for index, target := range targets {
		targetNames[index] = target.name
	}
	caseIDs := make([]string, len(cases))
	for index, corpusCase := range cases {
		caseIDs[index] = corpusCase.id
	}
	if err := w8ValidateCorpusRecords(records, targetNames, caseIDs); err != nil {
		t.Fatal(err)
	}

	t.Run("cardinality-gate-rejects-mutations", func(t *testing.T) {
		mutations := map[string]func([]w8CorpusRecord) []w8CorpusRecord{
			"missing": func(input []w8CorpusRecord) []w8CorpusRecord {
				return append([]w8CorpusRecord(nil), input[:len(input)-1]...)
			},
			"extra": func(input []w8CorpusRecord) []w8CorpusRecord {
				output := append([]w8CorpusRecord(nil), input...)
				output[len(output)-1].Target = "unexpected-target"
				return output
			},
			"duplicate": func(input []w8CorpusRecord) []w8CorpusRecord {
				output := append([]w8CorpusRecord(nil), input...)
				output[len(output)-1] = output[0]
				return output
			},
			"non-pass": func(input []w8CorpusRecord) []w8CorpusRecord {
				output := append([]w8CorpusRecord(nil), input...)
				output[0].Status = "FAIL"
				return output
			},
		}
		for name, mutate := range mutations {
			t.Run(name, func(t *testing.T) {
				if err := w8ValidateCorpusRecords(
					mutate(records),
					targetNames,
					caseIDs,
				); err == nil {
					t.Fatal("invalid corpus record set was accepted")
				}
			})
		}
	})
}

func w8CorpusP01(t *testing.T, target *w8CorpusTarget) {
	identity := target.adapter.Identity()
	body, err := EncodeDriverInfo(DriverInfo{
		ContractVersion: DriverContractVersion,
		AdapterID:       identity.ID,
		AdapterVersion:  identity.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := DecodeDriverInfo(body, DriverInfoBinding{
		AdapterID: identity.ID, AdapterVersion: identity.Version,
	})
	var fields map[string]json.RawMessage
	if err != nil || json.Unmarshal(body, &fields) != nil ||
		len(fields) != 3 ||
		info.AdapterID != identity.ID ||
		info.AdapterVersion != identity.Version {
		t.Fatalf("driver info = %#v, fields=%v, error=%v", info, fields, err)
	}
	registry, err := NewSelectionRegistry(
		[]ProfileConfig{target.profile},
		[]Adapter{target.adapter},
	)
	if err != nil {
		t.Fatal(err)
	}
	report := registry.Inspect(
		context.Background(),
		target.profile.Key,
		w8CorpusModel,
	)
	if report.State != ReadinessPass ||
		report.Family != target.family ||
		report.Surface != target.surface ||
		report.AdapterID != identity.ID ||
		report.ConfigurationDigest != identity.ConfigurationDigest {
		t.Fatalf("profile report = %#v", report)
	}
}

func w8CorpusP02(t *testing.T, target *w8CorpusTarget) {
	rows := []struct {
		role           Role
		responsibility Responsibility
		outcome        DecisionOutcome
		access         WorkspaceAccess
	}{
		{RolePlanner, PlannerProposal, "", ReadWrite},
		{RoleImplementer, ImplementerImplementation, "", ReadWrite},
		{RoleCaptain, CaptainReview, DecisionProceed, ReadOnly},
		{RoleVerifier, WorkVerification, DecisionPass, ReadOnly},
	}
	for _, row := range rows {
		invocation, observation, err := target.invoke(
			t,
			"p02-"+string(row.role),
			row.role,
			row.responsibility,
			row.outcome,
			row.access,
			"p02",
		)
		w8RequireCompleted(t, invocation, observation, err, row.responsibility)
		if invocation.Request.Model != w8CorpusModel ||
			invocation.Selected.Model != w8CorpusModel ||
			len(invocation.Request.Inputs) != 1 ||
			invocation.Request.Inputs[0].Digest !=
				invocation.Inputs[0].Input.Digest {
			t.Fatalf("translated invocation = %#v", invocation)
		}
	}
	if _, err := NewRequest(
		"w8-merge-is-engine-owned",
		Role("merge"),
		target.profile.Key,
		"",
		Workspace{Path: GuestWorkspacePath, Access: ReadWrite},
		[]Input{},
		true,
		Limits{TimeoutMillis: 1_000, OutputBytes: 1_024},
	); !IsCode(err, "INVALID_ROLE") {
		t.Fatalf("production Merge dispatch error = %v", err)
	}
}

func w8CorpusP03(t *testing.T, target *w8CorpusTarget) {
	invocation := target.newInvocation(
		t,
		"p03-contract",
		RolePlanner,
		PlannerProposal,
		"",
		ReadWrite,
	)
	body, err := EncodeRequest(invocation.Request)
	if err != nil {
		t.Fatal(err)
	}
	unknown := bytes.Replace(
		body,
		[]byte("}\n"),
		[]byte(",\"unknown\":true}\n"),
		1,
	)
	if _, err := DecodeRequest(
		append(append([]byte(nil), body...), []byte("{}")...),
	); err == nil {
		t.Fatal("trailing JSON accepted")
	}
	if _, err := DecodeRequest(unknown); err == nil {
		t.Fatal("unknown field accepted")
	}
	var missingValue map[string]any
	if json.Unmarshal(body, &missingValue) != nil {
		t.Fatal("valid request did not decode as JSON")
	}
	delete(missingValue, "model")
	missingBody, err := json.Marshal(missingValue)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeRequest(append(missingBody, '\n')); err == nil {
		t.Fatal("missing field accepted")
	}
	relative := invocation.Request
	relative.Workspace.Path = "relative"
	if err := ValidateRequest(relative); err == nil {
		t.Fatal("relative workspace accepted")
	}
	duplicate := invocation.Request
	duplicate.Inputs = append(
		append([]Input(nil), duplicate.Inputs...),
		duplicate.Inputs[0],
	)
	if err := ValidateRequest(duplicate); err == nil {
		t.Fatal("duplicate input accepted")
	}
	stale := invocation.Request
	stale.Operation.Digest = Digest([]byte("substituted operation"))
	if err := ValidateRequest(stale); !IsCode(err, "STALE_OPERATION") {
		t.Fatalf("stale operation error = %v", err)
	}
	mismatched := invocation.Request
	mismatched.Role = RoleCaptain
	if err := ValidateRequest(mismatched); !IsCode(err, "OPERATION_ROLE_MISMATCH") {
		t.Fatalf("role/operation mismatch error = %v", err)
	}
	target.adapter.invocations.Store(0)
	unbound := invocation
	unbound.Selected.Adapter.ConfigurationDigest = Digest([]byte("stale adapter"))
	observation, err := (Dispatcher{}).Invoke(context.Background(), unbound)
	if err == nil || observation.Handoff != nil ||
		target.adapter.invocations.Load() != 0 {
		t.Fatalf(
			"unbound invocation = %#v, calls=%d, error=%v",
			observation,
			target.adapter.invocations.Load(),
			err,
		)
	}
}

func w8CorpusP04(t *testing.T, target *w8CorpusTarget) {
	invocation, observation, err := target.invoke(
		t,
		"p04-status",
		RolePlanner,
		PlannerProposal,
		"",
		ReadWrite,
		"p04",
	)
	w8RequireCompleted(t, invocation, observation, err, PlannerProposal)
	if observation.Usage.TokenStatus != UsageUnavailable ||
		observation.Usage.InputTokens != nil ||
		observation.Usage.OutputTokens != nil {
		t.Fatalf("unreported usage = %#v", observation.Usage)
	}
	identity := target.adapter.Identity()
	for _, status := range []TransportStatus{
		Completed, TransportError, TimedOut, Cancelled, RunnerError,
	} {
		result := Result{
			SchemaVersion:   ResultSchemaVersion,
			InvocationID:    "w8-p04-" + string(status),
			AdapterID:       identity.ID,
			AdapterVersion:  identity.Version,
			ObservedModel:   w8CorpusModel,
			DurationMillis:  1,
			TransportStatus: status,
		}
		body, err := EncodeResult(result)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeResult(body, ResultBinding{
			InvocationID:   result.InvocationID,
			AdapterID:      identity.ID,
			AdapterVersion: identity.Version,
			Model:          w8CorpusModel,
			BindModel:      true,
		})
		if err != nil || decoded.TransportStatus != status ||
			decoded.Usage != nil {
			t.Fatalf("%s result = %#v, error=%v", status, decoded, err)
		}
	}
}

func w8CorpusP05(t *testing.T, target *w8CorpusTarget) {
	invocation, observation, err := target.invoke(
		t,
		"p05-opaque",
		RoleImplementer,
		ImplementerImplementation,
		"",
		ReadWrite,
		"p05-opaque",
	)
	w8RequireCompleted(
		t,
		invocation,
		observation,
		err,
		ImplementerImplementation,
	)
	body, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte(w8CorpusSentinel)) ||
		bytes.Contains(observation.Handoff.SubmissionBytes, []byte(w8CorpusSentinel)) {
		t.Fatalf("transport-only text escaped observation: %s", body)
	}
}

func w8CorpusP06(t *testing.T, target *w8CorpusTarget) {
	_, observation, err := target.invoke(
		t,
		"p06-malformed",
		RolePlanner,
		PlannerProposal,
		"",
		ReadWrite,
		"p06-malformed",
	)
	if err == nil || observation.Handoff != nil {
		t.Fatalf("malformed transport result = %#v, error=%v", observation, err)
	}
	identity := target.adapter.Identity()
	hostile := Result{
		SchemaVersion:   ResultSchemaVersion,
		InvocationID:    "w8-p06-binding",
		AdapterID:       identity.ID,
		AdapterVersion:  identity.Version,
		ObservedModel:   w8CorpusModel,
		DurationMillis:  1,
		TransportStatus: Completed,
	}
	body, err := EncodeResult(hostile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeResult(body, ResultBinding{
		InvocationID:   "different-invocation",
		AdapterID:      identity.ID,
		AdapterVersion: identity.Version,
	}); !IsCode(err, "RESULT_BINDING_MISMATCH") {
		t.Fatalf("result binding mismatch error = %v", err)
	}
}

func w8CorpusP07(t *testing.T, target *w8CorpusTarget) {
	_, overflow, overflowErr := target.invoke(
		t,
		"p07-overflow",
		RolePlanner,
		PlannerProposal,
		"",
		ReadWrite,
		"p07-overflow",
	)
	if overflowErr == nil || overflow.Handoff != nil {
		t.Fatalf("overflow result = %#v, error=%v", overflow, overflowErr)
	}
	_, timedOut, timeoutErr := target.invoke(
		t,
		"p07-block",
		RolePlanner,
		PlannerProposal,
		"",
		ReadWrite,
		"p07-block",
	)
	if !IsCode(timeoutErr, "INVOCATION_TIMEOUT") || timedOut.Handoff != nil {
		t.Fatalf("timeout result = %#v, error=%v", timedOut, timeoutErr)
	}
	if target.transport != nil && target.transport.requests != 1 {
		t.Fatalf("timed-out provider attempts = %d", target.transport.requests)
	}
}

func w8CorpusP08(t *testing.T, target *w8CorpusTarget) {
	target.adapter.invocations.Store(0)
	invocation, observation, err := target.invoke(
		t,
		"p08-single",
		RoleImplementer,
		ImplementerImplementation,
		"",
		ReadWrite,
		"p08-single",
	)
	w8RequireCompleted(
		t,
		invocation,
		observation,
		err,
		ImplementerImplementation,
	)
	if target.adapter.invocations.Load() != 1 {
		t.Fatalf("adapter invocation attempts = %d", target.adapter.invocations.Load())
	}
	if invocation.Request.Model != w8CorpusModel ||
		invocation.Selected.Model != w8CorpusModel {
		t.Fatalf("implicit model selection = %#v", invocation.Selected)
	}
	if target.transport != nil &&
		(target.transport.requests != 1 ||
			target.transport.explicitModel != w8CorpusModel) {
		t.Fatalf(
			"provider requests=%d model=%q",
			target.transport.requests,
			target.transport.explicitModel,
		)
	}
}

func w8CorpusP09(t *testing.T, target *w8CorpusTarget) {
	invocation, observation, err := target.invoke(
		t,
		"p09-write",
		RoleImplementer,
		ImplementerImplementation,
		"",
		ReadWrite,
		"p09-write",
	)
	w8RequireCompleted(
		t,
		invocation,
		observation,
		err,
		ImplementerImplementation,
	)
	result, err := os.ReadFile(filepath.Join(invocation.HostWorkspace, "result.txt"))
	if err != nil || Digest(result) != Digest([]byte(w8CorpusResultBody)) {
		t.Fatalf("implementation result = %q, error=%v", result, err)
	}
}

func w8CorpusP10(t *testing.T, target *w8CorpusTarget) {
	invocation, observation, err := target.invoke(
		t,
		"p10-read-only",
		RoleVerifier,
		WorkVerification,
		DecisionPass,
		ReadOnly,
		"p10-read-only",
	)
	w8RequireCompleted(t, invocation, observation, err, WorkVerification)
	if !invocation.Request.FreshContext ||
		invocation.Request.Workspace.Access != ReadOnly {
		t.Fatalf("verifier invocation = %#v", invocation.Request)
	}
	if _, err := os.Stat(
		filepath.Join(invocation.HostWorkspace, "result.txt"),
	); !os.IsNotExist(err) {
		t.Fatalf("read-only mutation escaped: %v", err)
	}
}

func (target *w8CorpusTarget) invoke(
	t *testing.T,
	idSuffix string,
	role Role,
	responsibility Responsibility,
	outcome DecisionOutcome,
	access WorkspaceAccess,
	scenario string,
) (Invocation, Observation, error) {
	t.Helper()
	invocation := target.newInvocation(
		t,
		idSuffix,
		role,
		responsibility,
		outcome,
		access,
	)
	if target.transport != nil {
		target.transport.prepare(
			scenario,
			invocation.Inputs[0].Bytes,
		)
	}
	ctx := context.Background()
	cancel := func() {}
	if scenario == "p07-block" {
		ctx, cancel = context.WithTimeout(ctx, 500*time.Millisecond)
	}
	defer cancel()
	observation, err := (Dispatcher{}).Invoke(ctx, invocation)
	return invocation, observation, err
}

func (target *w8CorpusTarget) newInvocation(
	t *testing.T,
	idSuffix string,
	role Role,
	responsibility Responsibility,
	outcome DecisionOutcome,
	access WorkspaceAccess,
) Invocation {
	t.Helper()
	invocationID := "w8-" + target.name + "-" + idSuffix
	submissionBody, err := EncodeSubmission(submissionFixture(
		t,
		invocationID,
		responsibility,
		outcome,
	))
	if err != nil {
		t.Fatal(err)
	}
	input := Input{
		Name: "submission", Path: "submission.json",
		Digest: Digest(submissionBody),
	}
	request, err := NewRequest(
		invocationID,
		role,
		target.profile.Key,
		w8CorpusModel,
		Workspace{Path: GuestWorkspacePath, Access: access},
		[]Input{input},
		true,
		Limits{TimeoutMillis: 500, OutputBytes: 65_536},
	)
	if err != nil {
		t.Fatal(err)
	}
	selected := SelectedProfile{
		Profile: target.profile,
		Adapter: target.adapter.Identity(),
		Model:   w8CorpusModel,
		adapter: target.adapter,
	}
	containment := ContainmentReadWrite
	if access == ReadOnly {
		containment = ContainmentReadOnly
	}
	permission, err := NewSubmissionPermission(
		request,
		selected,
		containment,
		responsibility,
	)
	if err != nil {
		t.Fatal(err)
	}
	return Invocation{
		Request:       request,
		HostWorkspace: t.TempDir(),
		Selected:      selected,
		Permission:    permission,
		Inputs: []InputContent{{
			Input: input, Bytes: submissionBody,
		}},
	}
}

func w8RequireCompleted(
	t *testing.T,
	invocation Invocation,
	observation Observation,
	err error,
	responsibility Responsibility,
) {
	t.Helper()
	if err != nil ||
		observation.TransportStatus != Completed ||
		observation.Handoff == nil ||
		observation.Diagnostic.Code != "none" {
		t.Fatalf("observation = %#v, error=%v", observation, err)
	}
	submission, err := DecodeSubmission(observation.Handoff.SubmissionBytes)
	if err != nil ||
		submission.InvocationID != invocation.Request.InvocationID ||
		submission.Responsibility != responsibility ||
		observation.Handoff.SubmissionDigest !=
			Digest(observation.Handoff.SubmissionBytes) {
		t.Fatalf("submission = %#v, error=%v", submission, err)
	}
}

func w8NewProviderTarget(
	t *testing.T,
	name string,
	family ProfileFamily,
	surface ProfileSurface,
	codec w8ProviderCodec,
) *w8CorpusTarget {
	t.Helper()
	transport := &w8ProviderRoundTripper{codec: codec}
	key := "w8-" + name
	id := "sworn.w8." + name
	ref := "w8-credential"
	var adapter Adapter
	var err error
	switch name {
	case "openai":
		adapter, err = NewOpenAIAdapter(
			OpenAIProfileConfig{
				HTTPProfileConfig: HTTPProfileConfig{
					Key: key, ID: id, Version: "1.0.0",
					Endpoint:         "http://localhost/openai/v1/responses",
					CredentialHeader: "Authorization",
					CredentialPrefix: "Bearer ",
					CredentialRefs:   []string{ref},
					ResponseBytes:    MaxProviderResponseBytes,
				},
				API:             OpenAIResponsesAPI,
				ReasoningEffort: "medium",
			},
			w8HeaderCredential,
			nil,
			transport,
		)
	case "deepseek":
		adapter, err = NewDeepSeekAdapter(
			HTTPProfileConfig{
				Key: key, ID: id, Version: "1.0.0",
				Endpoint:         "http://localhost/deepseek/chat/completions",
				CredentialHeader: "Authorization",
				CredentialPrefix: "Bearer ",
				CredentialRefs:   []string{ref},
				ResponseBytes:    MaxProviderResponseBytes,
			},
			w8HeaderCredential,
			nil,
			transport,
		)
	case "gemini":
		adapter, err = NewGeminiAdapter(
			HTTPProfileConfig{
				Key: key, ID: id, Version: "1.0.0",
				Endpoint:         "http://localhost/gemini",
				CredentialHeader: "x-goog-api-key",
				CredentialRefs:   []string{ref},
				ResponseBytes:    MaxProviderResponseBytes,
			},
			w8HeaderCredential,
			nil,
			transport,
		)
	case "bedrock-runtime":
		awsPath := filepath.Join(t.TempDir(), "aws-never-called")
		if err := os.WriteFile(awsPath, nil, 0o700); err != nil {
			t.Fatal(err)
		}
		spec := w8AWSChain(t, awsPath)
		adapter, err = NewBedrockAdapter(
			BedrockProfileConfig{
				Key: key, ID: id, Version: "1.0.0",
				Endpoint:       "http://localhost/bedrock",
				CredentialRefs: []string{ref},
				ResponseBytes:  MaxProviderResponseBytes,
				Chain:          spec,
			},
			w8AWSRuntimeCredential,
			nil,
			transport,
		)
		if err == nil {
			var runAWSCalls atomic.Int64
			bedrock := adapter.(*loopAdapter).transport.(*bedrockTransport)
			bedrock.runAWS = func(
				context.Context,
				AWSChainSpec,
				[][]byte,
				...string,
			) ([]byte, error) {
				runAWSCalls.Add(1)
				return nil, fmt.Errorf(
					"AWS CLI must not run for direct environment credentials",
				)
			}
			t.Cleanup(func() {
				if got := runAWSCalls.Load(); got != 0 {
					t.Errorf("AWS CLI runner calls = %d, want 0", got)
				}
			})
		}
	case "mantle":
		adapter, err = NewBedrockMantleAdapter(
			BedrockMantleProfileConfig{
				Key: key, ID: id, Version: "1.0.0",
				Endpoint:       "http://localhost/mantle/v1/chat/completions",
				CredentialRefs: []string{ref},
				ResponseBytes:  MaxProviderResponseBytes,
				AuthMode:       BedrockMantleAPIKey,
			},
			w8HeaderCredential,
			nil,
			nil,
			transport,
		)
	default:
		t.Fatalf("unknown W8 provider target %q", name)
	}
	if err != nil {
		t.Fatal(err)
	}
	counted := &w8CountedAdapter{delegate: adapter}
	profile := ProfileConfig{
		Key: "w8-profile-" + name, Adapter: key,
		Network: NetworkRequired, CredentialRef: &ref,
	}
	return &w8CorpusTarget{
		name: name, family: family, surface: surface,
		profile: profile, adapter: counted, transport: transport,
		credential: ref,
	}
}

func w8HeaderCredential(context.Context, string) ([]byte, error) {
	return []byte("w8-secret"), nil
}

func w8AWSRuntimeCredential(context.Context, string) ([][]byte, error) {
	return [][]byte{
		[]byte("AWS_ACCESS_KEY_ID=AKIAEXAMPLE1234"),
		[]byte("AWS_SECRET_ACCESS_KEY=secret-example-value"),
		[]byte("AWS_REGION=ap-southeast-2"),
		[]byte("AWS_DEFAULT_REGION=ap-southeast-2"),
	}, nil
}

func w8AWSChain(t *testing.T, pathValue string) AWSChainSpec {
	t.Helper()
	runtimeFiles := systemRuntimeFiles(t)
	required := make([]string, len(runtimeFiles))
	for index := range runtimeFiles {
		required[index] = runtimeFiles[index].Target
	}
	return AWSChainSpec{
		CLI: ExecutableIdentity{
			Path: pathValue, Digest: AWSCLIDigest,
		},
		CLIVersion:       AWSCLIVersion,
		Region:           "ap-southeast-2",
		RegionSource:     AWSSourceEnvironment,
		CredentialSource: AWSSourceEnvironment,
		EnvironmentKeys: []string{
			"AWS_ACCESS_KEY_ID",
			"AWS_DEFAULT_REGION",
			"AWS_REGION",
			"AWS_SECRET_ACCESS_KEY",
		},
		RuntimeFiles:           runtimeFiles,
		RequiredRuntimeTargets: required,
	}
}

func w8NewNativeTarget(
	t *testing.T,
	name string,
	family ProfileFamily,
	binary string,
) *w8CorpusTarget {
	t.Helper()
	digest, err := executableDigest(binary)
	if err != nil {
		t.Fatal(err)
	}
	runtimeFiles := systemRuntimeFiles(t)
	required := make([]string, len(runtimeFiles))
	for index := range runtimeFiles {
		required[index] = runtimeFiles[index].Target
	}
	targetPath := CodexCredentialTarget
	if family == ProfileClaude {
		targetPath = ClaudeCredentialTarget
	}
	ref := "w8-credential"
	config := NativeAdapterConfig{
		Key: "w8-" + name, ID: "sworn.w8." + name, Version: "1.0.0",
		Family: family,
		CLI: ExecutableIdentity{
			Path: binary, Digest: digest,
		},
		CLIVersion:             "w8-fixture-1.0.0",
		VersionOutput:          "w8-fixture-1.0.0",
		RuntimeFiles:           runtimeFiles,
		RequiredRuntimeTargets: required,
		CredentialTarget:       targetPath,
		CredentialRefs:         []string{ref},
		MaxCredentialBytes:     65_536,
	}
	configBody, err := canonicalJSON(config)
	if err != nil {
		t.Fatal(err)
	}
	identity := AdapterIdentity{
		Key: config.Key, ID: config.ID, Version: config.Version,
		ConfigurationDigest: Digest(configBody),
	}
	credentialRoot := t.TempDir()
	credentialPath := filepath.Join(credentialRoot, "credential")
	if err := os.WriteFile(
		credentialPath,
		[]byte(`{"fixture":"w8"}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	// NewNativeAdapter deliberately admits only the pinned live CLI. The
	// deterministic corpus substitutes a static fake CLI while retaining the
	// production nativeAdapter invocation, broker, certificate, and sandbox.
	native := &nativeAdapter{
		identity: identity,
		config:   config,
		resolve: func(context.Context, string) (string, error) {
			return credentialPath, nil
		},
		refs:      map[string]struct{}{ref: {}},
		certified: make(map[string]nativeSurfaceCertificate),
	}
	profile := ProfileConfig{
		Key: "w8-profile-" + name, Adapter: identity.Key,
		Network: NetworkRequired, CredentialRef: &ref,
	}
	native.certified[nativeCertificationKey(profile, w8CorpusModel)] =
		nativeSurfaceCertificateFixture(
			Invocation{Selected: SelectedProfile{
				Profile: profile,
				Adapter: identity,
				Model:   w8CorpusModel,
			}},
			config,
			"w8-native",
			"1.0.0",
			Digest([]byte("w8-native-capture")),
		)
	return &w8CorpusTarget{
		name: name, family: family, profile: profile,
		adapter:    &w8CountedAdapter{delegate: native},
		credential: ref,
	}
}

func w8ValidateCorpusRecords(
	records []w8CorpusRecord,
	targets []string,
	cases []string,
) error {
	expectedCount := len(targets) * len(cases)
	if len(records) != expectedCount || expectedCount != 70 {
		return fmt.Errorf(
			"W8 corpus cardinality = %d, expected %d",
			len(records),
			expectedCount,
		)
	}
	expected := make(map[string]struct{}, expectedCount)
	for _, target := range targets {
		for _, corpusCase := range cases {
			expected[target+"\x00"+corpusCase] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		key := record.Target + "\x00" + record.Case
		if _, present := expected[key]; !present {
			return fmt.Errorf(
				"unexpected W8 corpus record %s/%s",
				record.Target,
				record.Case,
			)
		}
		if record.Status != w8CorpusPass {
			return fmt.Errorf(
				"W8 corpus record %s/%s = %s",
				record.Target,
				record.Case,
				record.Status,
			)
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf(
				"duplicate W8 corpus record %s/%s",
				record.Target,
				record.Case,
			)
		}
		seen[key] = struct{}{}
	}
	for key := range expected {
		if _, present := seen[key]; !present {
			return fmt.Errorf(
				"missing W8 corpus record %q",
				key,
			)
		}
	}
	return nil
}

func w8BuildNativeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	source := filepath.Join(root, "main.go")
	binary := filepath.Join(root, "w8-native-fixture")
	if err := os.WriteFile(source, []byte(w8NativeFixtureSource), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(
		"go",
		"build",
		"-trimpath",
		"-o",
		binary,
		source,
	)
	command.Env = append(
		os.Environ(),
		"CGO_ENABLED=0",
		"GOFLAGS=-buildvcs=false",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build native corpus fixture: %v\n%s", err, output)
	}
	return binary
}

const w8NativeFixtureSource = `
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type promptEnvelope struct {
	InvocationID string
	Model        string
	Access       string
}

func main() {
	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(20)
	}
	var value map[string]any
	if json.Unmarshal(body, &value) != nil {
		os.Exit(21)
	}
	workspace, _ := value["workspace"].(map[string]any)
	prompt := promptEnvelope{
		InvocationID: text(value["invocation_id"]),
		Model: "w8-explicit-model",
		Access: text(workspace["access"]),
	}
	family, brokerURL, token := brokerConfiguration()
	if family == "" || brokerURL == "" || token == "" ||
		!hasModel(os.Args[1:], prompt.Model) {
		os.Exit(22)
	}
	lowerID := strings.ToLower(prompt.InvocationID)
	if strings.Contains(lowerID, "p07-overflow") {
		fmt.Fprintln(os.Stdout, strings.Repeat("x", 1048577))
		block()
	}
	if strings.Contains(lowerID, "p06-malformed") {
		if family == "codex" {
			fmt.Println("{\"type\":\"thread.started\",\"thread_id\":\"w8\",\"extra\":true}")
		} else {
			fmt.Println("{\"type\":\"system\",\"subtype\":\"init\"}")
		}
		block()
	}
	emitInit(family, prompt.Model, prompt.Access)
	initialize := map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities": map[string]any{},
		"clientInfo": map[string]any{
			"name": "w8-native", "version": "1.0.0",
		},
	}
	if status, _ := rpc(brokerURL, token, 1, "initialize", initialize); status != 200 {
		os.Exit(23)
	}
	if status, _ := notify(brokerURL, token, "notifications/initialized", map[string]any{}); status != 202 {
		os.Exit(24)
	}
	if status, _ := rpc(brokerURL, token, 2, "tools/list", map[string]any{}); status != 200 {
		os.Exit(25)
	}
	time.Sleep(150 * time.Millisecond)
	if strings.Contains(lowerID, "p05-opaque") {
		fmt.Println("{\"type\":\"transport\",\"text\":\"w8-transport-text-must-not-escape\"}")
	}
	if strings.Contains(lowerID, "p07-block") {
		block()
	}
	status, response := rpc(brokerURL, token, 3, "tools/call", map[string]any{
		"name": "Read",
		"arguments": map[string]any{"path": "/sworn/inputs/submission.json"},
	})
	if status != 200 {
		os.Exit(26)
	}
	submissionText, failed := toolText(response)
	if failed || submissionText == "" {
		os.Exit(27)
	}
	var submission any
	if json.Unmarshal([]byte(submissionText), &submission) != nil {
		os.Exit(28)
	}
	if strings.Contains(lowerID, "p09-write") ||
		strings.Contains(lowerID, "p10-read-only") {
		status, response = rpc(brokerURL, token, 4, "tools/call", map[string]any{
			"name": "Write",
			"arguments": map[string]any{
				"path": "/workspace/result.txt",
				"content": "w8 exact implementation result\n",
			},
		})
		_, writeFailed := toolText(response)
		expectFailure := strings.Contains(lowerID, "p10-read-only")
		if status != 200 || writeFailed != expectFailure {
			os.Exit(29)
		}
	}
	status, response = rpc(brokerURL, token, 5, "tools/call", map[string]any{
		"name": "sworn_submit",
		"arguments": map[string]any{"submission": submission},
	})
	_, failed = toolText(response)
	if status != 200 || failed {
		os.Exit(30)
	}
	block()
}

func brokerConfiguration() (string, string, string) {
	if body, err := os.ReadFile("/etc/codex/config.toml"); err == nil {
		var brokerURL, token string
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "url = ") {
				brokerURL = quoted(line)
			}
			if strings.HasPrefix(line, "http_headers = ") {
				start := strings.Index(line, "Bearer ")
				if start >= 0 {
					rest := line[start+len("Bearer "):]
					end := strings.Index(rest, "\"")
					if end >= 0 {
						token = rest[:end]
					}
				}
			}
		}
		return "codex", brokerURL, token
	}
	body, err := os.ReadFile("/sworn/config/mcp.json")
	if err != nil {
		return "", "", ""
	}
	var config map[string]any
	if json.Unmarshal(body, &config) != nil {
		return "", "", ""
	}
	servers, _ := config["mcpServers"].(map[string]any)
	server, _ := servers["sworn"].(map[string]any)
	headers, _ := server["headers"].(map[string]any)
	return "claude", text(server["url"]), strings.TrimPrefix(
		text(headers["Authorization"]),
		"Bearer ",
	)
}

func quoted(line string) string {
	start := strings.Index(line, "\"")
	if start < 0 {
		return ""
	}
	rest := line[start+1:]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func hasModel(arguments []string, model string) bool {
	for index := 0; index+1 < len(arguments); index++ {
		if (arguments[index] == "-m" || arguments[index] == "--model") &&
			arguments[index+1] == model {
			return true
		}
	}
	return false
}

func emitInit(family, model, access string) {
	if family == "codex" {
		fmt.Println("{\"type\":\"thread.started\",\"thread_id\":\"w8-native\"}")
		return
	}
	tools := []any{
		"mcp__sworn__Bash",
		"mcp__sworn__Read",
		"mcp__sworn__Glob",
		"mcp__sworn__Grep",
	}
	if access == "read_write" {
		tools = append(tools, "mcp__sworn__Write", "mcp__sworn__Edit")
	}
	tools = append(
		tools,
		"mcp__sworn__sworn_yield",
		"mcp__sworn__sworn_submit",
	)
	event := map[string]any{
		"type": "system", "subtype": "init",
		"model": model, "permissionMode": "dontAsk",
		"slash_commands": []any{}, "skills": []any{}, "plugins": []any{},
		"tools": tools,
		"mcp_servers": []any{map[string]any{
			"name": "sworn", "status": "connected",
		}},
		"capabilities": []any{"interrupt_receipt_v1", "msg_lifecycle_v1"},
		"analytics_disabled": true,
		"product_feedback_disabled": true,
	}
	body, _ := json.Marshal(event)
	fmt.Println(string(body))
}

func rpc(
	url string,
	token string,
	id int,
	method string,
	params any,
) (int, map[string]any) {
	return post(url, token, map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	})
}

func notify(
	url string,
	token string,
	method string,
	params any,
) (int, map[string]any) {
	return post(url, token, map[string]any{
		"jsonrpc": "2.0", "method": method, "params": params,
	})
}

func post(url string, token string, value any) (int, map[string]any) {
	body, _ := json.Marshal(value)
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, nil
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(response.Body)
	var result map[string]any
	_ = json.Unmarshal(responseBody, &result)
	return response.StatusCode, result
}

func toolText(response map[string]any) (string, bool) {
	result, _ := response["result"].(map[string]any)
	failed, _ := result["isError"].(bool)
	content, _ := result["content"].([]any)
	if len(content) != 1 {
		return "", true
	}
	item, _ := content[0].(map[string]any)
	text, _ := item["text"].(string)
	return text, failed
}

func text(value any) string {
	result, _ := value.(string)
	return result
}

func block() {
	for {
		time.Sleep(time.Hour)
	}
}
`
