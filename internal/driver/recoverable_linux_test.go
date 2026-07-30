//go:build linux

package driver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestRecoverableProviderTurnRetainsYieldAndRehydratesWithInput(
	t *testing.T,
) {
	t.Parallel()
	const invocationID = "recoverable-same-invocation"
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		turn := requests.Add(1)
		var body struct {
			Input []map[string]any `json:"input"`
		}
		if json.NewDecoder(request.Body).Decode(&body) != nil ||
			len(body.Input) == 0 {
			t.Error("invalid provider request")
			return
		}
		last := body.Input[len(body.Input)-1]
		content, contentOK := last["content"].(string)
		var prompt struct {
			InvocationID string `json:"invocation_id"`
			Recovery     *struct {
				Kind    RecoverableInputKind `json:"kind"`
				Content string               `json:"content"`
			} `json:"recovery"`
		}
		if last["role"] != "user" || !contentOK ||
			json.Unmarshal([]byte(content), &prompt) != nil {
			t.Errorf("last prompt = %#v", last)
			return
		}
		switch turn {
		case 1:
			if prompt.InvocationID != invocationID || prompt.Recovery != nil {
				t.Errorf("start prompt = %#v", prompt)
			}
			writeRecoverableYield(
				t, writer, invocationID, "Which base should I use?",
			)
		case 2:
			if prompt.InvocationID != invocationID ||
				prompt.Recovery == nil ||
				prompt.Recovery.Kind != RecoverableInputAnswer ||
				prompt.Recovery.Content != "Use the pinned prepared base." {
				t.Errorf("answer prompt = %#v", prompt)
			}
			writeRecoverableYieldCall(
				t,
				writer,
				invocationID,
				"May I proceed with the bounded fix?",
				"recoverable-answer-call",
			)
		case 3:
			if prompt.InvocationID != invocationID ||
				prompt.Recovery == nil ||
				prompt.Recovery.Kind != RecoverableInputNudge ||
				prompt.Recovery.Content != recoverableTurnNudge {
				t.Errorf("nudge prompt = %#v", prompt)
			}
			writeRecoverableSubmissionCall(
				t,
				writer,
				invocationID,
				ImplementerImplementation,
				"recoverable-submit-call",
			)
		case 4:
			if prompt.InvocationID != invocationID ||
				prompt.Recovery == nil ||
				prompt.Recovery.Kind != RecoverableInputAnswer ||
				prompt.Recovery.Content != "Resume from durable evidence." {
				t.Errorf("rehydrate prompt = %#v", prompt)
			}
			writeRecoverableSubmit(t, writer, invocationID)
		default:
			t.Errorf("unexpected request %d", turn)
		}
	}))
	defer server.Close()
	adapter := recoverableProviderAdapter(t, server.URL)
	binding := continuationContractBinding()

	start := productionInvocationFixture(
		t,
		adapter,
		ProfileOpenAIHTTP,
		invocationID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	observation, handle, result, err :=
		(Dispatcher{}).InvokeRecoverableTurn(
			context.Background(), start, binding, nil, nil,
		)
	if err != nil || observation.Yield == nil || handle == nil ||
		result.Status != ContinuationStatusSuspended {
		t.Fatalf(
			"start observation=%#v handle=%v result=%#v error=%v",
			observation, handle != nil, result, err,
		)
	}

	answer := productionInvocationFixture(
		t,
		adapter,
		ProfileOpenAIHTTP,
		invocationID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	answerInput := &RecoverableTurnInput{
		SchemaVersion: RecoverableTurnInputSchemaVersion,
		Kind:          RecoverableInputAnswer,
		Answer:        "Use the pinned prepared base.",
	}
	observation, handle, result, err =
		(Dispatcher{}).InvokeRecoverableTurn(
			context.Background(), answer, binding, handle, answerInput,
		)
	if err != nil || observation.Yield == nil || handle == nil ||
		result.Status != ContinuationStatusSuspended {
		t.Fatalf(
			"answer observation=%#v handle=%v result=%#v error=%v",
			observation, handle != nil, result, err,
		)
	}

	nudge := productionInvocationFixture(
		t,
		adapter,
		ProfileOpenAIHTTP,
		invocationID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	nudgeInput := &RecoverableTurnInput{
		SchemaVersion: RecoverableTurnInputSchemaVersion,
		Kind:          RecoverableInputNudge,
	}
	observation, handle, result, err =
		(Dispatcher{}).InvokeRecoverableTurn(
			context.Background(), nudge, binding, handle, nudgeInput,
		)
	if err != nil || observation.Handoff == nil || handle != nil ||
		result.Status != ContinuationStatusResumed {
		t.Fatalf(
			"nudge observation=%#v handle=%v result=%#v error=%v",
			observation, handle != nil, result, err,
		)
	}

	rehydrate := productionInvocationFixture(
		t,
		adapter,
		ProfileOpenAIHTTP,
		invocationID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	rehydrateInput := &RecoverableTurnInput{
		SchemaVersion: RecoverableTurnInputSchemaVersion,
		Kind:          RecoverableInputAnswer,
		Answer:        "Resume from durable evidence.",
	}
	observation, handle, result, err =
		(Dispatcher{}).InvokeRecoverableTurn(
			context.Background(),
			rehydrate,
			binding,
			nil,
			rehydrateInput,
		)
	if err != nil || observation.Handoff == nil || handle != nil ||
		result.Mode != ContinuationModeFreshRehydrate ||
		result.Status != ContinuationStatusCompleted ||
		requests.Load() != 4 {
		t.Fatalf(
			"rehydrate observation=%#v handle=%v result=%#v requests=%d error=%v",
			observation, handle != nil, result, requests.Load(), err,
		)
	}
}

func TestW3YieldHandleIsAdoptedOnlyBySameDutyRecovery(t *testing.T) {
	t.Parallel()
	const (
		startID      = "w3-yield-start"
		resumeID     = "w3-yield-resume"
		wrongStartID = "w3-yield-wrong-start"
		wrongID      = "w3-yield-wrong-resume"
		dutyStartID  = "w3-yield-duty-start"
		dutyID       = "w3-yield-duty-resume"
	)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		switch requests.Add(1) {
		case 1:
			writeRecoverableYield(
				t, writer, startID, "Which design constraint controls?",
			)
			return
		case 3:
			writeRecoverableYield(
				t, writer, wrongStartID, "Which design constraint controls?",
			)
			return
		case 4:
			writeRecoverableYield(
				t, writer, dutyStartID, "Which design constraint controls?",
			)
			return
		}
		submission := submissionFixture(
			t, resumeID, ImplementerDesign, "",
		)
		writeJSONResponse(t, writer, responsesToolCallResponse(
			"w3-resume-response",
			"w3-resume-function",
			"w3-resume-call",
			"sworn_submit",
			submissionToolArguments(t, submission),
			1,
			1,
		))
	}))
	defer server.Close()
	adapter := recoverableProviderAdapter(t, server.URL)
	binding := continuationContractBinding()
	start := productionInvocationFixture(
		t,
		adapter,
		ProfileOpenAIHTTP,
		startID,
		RoleImplementer,
		ImplementerDesign,
		ReadOnly,
	)
	observation, handle, result, err := (Dispatcher{}).InvokeTurn(
		context.Background(), start, binding, nil,
	)
	if err != nil || observation.Yield == nil || handle == nil ||
		result.Status != ContinuationStatusSuspended {
		t.Fatalf(
			"start observation=%#v handle=%v result=%#v error=%v",
			observation, handle != nil, result, err,
		)
	}
	resume := productionInvocationFixture(
		t,
		adapter,
		ProfileOpenAIHTTP,
		resumeID,
		RoleImplementer,
		ImplementerDesign,
		ReadOnly,
	)
	input := &RecoverableTurnInput{
		SchemaVersion: RecoverableTurnInputSchemaVersion,
		Kind:          RecoverableInputAnswer,
		Answer:        "The approved design constraint is AC-02.",
	}
	observation, handle, result, err =
		(Dispatcher{}).InvokeRecoverableTurn(
			context.Background(), resume, binding, handle, input,
		)
	if err != nil || observation.Handoff == nil || handle != nil ||
		result.Status != ContinuationStatusResumed ||
		requests.Load() != 2 {
		t.Fatalf(
			"resume observation=%#v handle=%v result=%#v requests=%d error=%v",
			observation, handle != nil, result, requests.Load(), err,
		)
	}

	wrongStart := productionInvocationFixture(
		t,
		adapter,
		ProfileOpenAIHTTP,
		wrongStartID,
		RoleImplementer,
		ImplementerDesign,
		ReadOnly,
	)
	observation, handle, result, err = (Dispatcher{}).InvokeTurn(
		context.Background(), wrongStart, binding, nil,
	)
	if err != nil || observation.Yield == nil || handle == nil {
		t.Fatalf(
			"wrong start observation=%#v handle=%v result=%#v error=%v",
			observation, handle != nil, result, err,
		)
	}
	wrong := productionInvocationFixture(
		t,
		adapter,
		ProfileOpenAIHTTP,
		wrongID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	wrong.Request.FreshContext = false
	wrong.Permission, err = NewSubmissionPermission(
		wrong.Request,
		wrong.Selected,
		ContainmentReadWrite,
		ImplementerImplementation,
	)
	if err != nil {
		t.Fatal(err)
	}
	observation, handle, result, err = (Dispatcher{}).InvokeTurn(
		context.Background(), wrong, binding, handle,
	)
	if err != nil || observation.Handoff != nil ||
		observation.Yield != nil ||
		observation.TransportStatus != "" || handle != nil ||
		result.Status != ContinuationStatusMismatch ||
		requests.Load() != 3 {
		t.Fatalf(
			"wrong resume observation=%#v handle=%v result=%#v requests=%d error=%v",
			observation, handle != nil, result, requests.Load(), err,
		)
	}

	dutyStart := productionInvocationFixture(
		t,
		adapter,
		ProfileOpenAIHTTP,
		dutyStartID,
		RoleImplementer,
		ImplementerDesign,
		ReadOnly,
	)
	observation, handle, result, err = (Dispatcher{}).InvokeTurn(
		context.Background(), dutyStart, binding, nil,
	)
	if err != nil || observation.Yield == nil || handle == nil {
		t.Fatalf(
			"duty start observation=%#v handle=%v result=%#v error=%v",
			observation, handle != nil, result, err,
		)
	}
	duty := productionInvocationFixture(
		t,
		adapter,
		ProfileOpenAIHTTP,
		dutyID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	dutyInput := &RecoverableTurnInput{
		SchemaVersion: RecoverableTurnInputSchemaVersion,
		Kind:          RecoverableInputAnswer,
		Answer:        "This answer must not cross responsibility.",
	}
	observation, handle, result, err =
		(Dispatcher{}).InvokeRecoverableTurn(
			context.Background(), duty, binding, handle, dutyInput,
		)
	if err != nil || observation.Handoff != nil ||
		observation.Yield != nil ||
		observation.TransportStatus != "" || handle != nil ||
		result.Status != ContinuationStatusMismatch ||
		requests.Load() != 4 {
		t.Fatalf(
			"duty resume observation=%#v handle=%v result=%#v requests=%d error=%v",
			observation, handle != nil, result, requests.Load(), err,
		)
	}
}

func TestRecoverableDesignTerminalPromotesAtomicallyToW3(t *testing.T) {
	t.Parallel()
	const (
		designID         = "recoverable-promoted-design"
		implementationID = "recoverable-promoted-implementation"
	)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		switch requests.Add(1) {
		case 1:
			writeRecoverableYield(
				t,
				writer,
				designID,
				"Which exact design constraint controls?",
			)
		case 2:
			writeRecoverableSubmissionCall(
				t,
				writer,
				designID,
				ImplementerDesign,
				"promoted-design-terminal",
			)
		case 3:
			writeRecoverableSubmission(
				t,
				writer,
				implementationID,
				ImplementerImplementation,
			)
		default:
			t.Errorf("unexpected provider request %d", requests.Load())
		}
	}))
	defer server.Close()

	adapter := recoverableProviderAdapter(t, server.URL)
	recoveryBinding := continuationContractBinding()
	targetBinding := recoveryBinding
	targetBinding.ToolContractDigest =
		Digest([]byte("promoted-design-to-implementation"))
	design := productionInvocationFixture(
		t,
		adapter,
		ProfileOpenAIHTTP,
		designID,
		RoleImplementer,
		ImplementerDesign,
		ReadOnly,
	)
	observation, source, result, err := (Dispatcher{}).InvokeTurn(
		context.Background(),
		design,
		recoveryBinding,
		nil,
	)
	if err != nil || observation.Yield == nil || source == nil ||
		result.Status != ContinuationStatusSuspended {
		t.Fatalf(
			"design yield = observation %#v, source %p, result %#v, error %v",
			observation,
			source,
			result,
			err,
		)
	}
	sourceAlias := *source
	input := &RecoverableTurnInput{
		SchemaVersion: RecoverableTurnInputSchemaVersion,
		Kind:          RecoverableInputAnswer,
		Answer:        "Use the exact current design constraint.",
		TargetBinding: &targetBinding,
	}
	observation, promoted, result, err :=
		(Dispatcher{}).InvokeRecoverableTurn(
			context.Background(),
			design,
			recoveryBinding,
			source,
			input,
		)
	if err != nil || observation.Handoff == nil ||
		observation.Yield != nil || promoted == nil ||
		result.Mode != ContinuationModeOpaqueReplay ||
		result.Status != ContinuationStatusSuspended ||
		requests.Load() != 2 {
		t.Fatalf(
			"design promotion = observation %#v, promoted %p, result %#v, requests %d, error %v",
			observation,
			promoted,
			result,
			requests.Load(),
			err,
		)
	}
	promotedAlias := *promoted

	replayObservation, replay, replayResult, replayErr :=
		(Dispatcher{}).InvokeRecoverableTurn(
			context.Background(),
			design,
			recoveryBinding,
			&sourceAlias,
			input,
		)
	if replayErr != nil ||
		!reflect.DeepEqual(replayObservation, Observation{}) ||
		replay != nil ||
		replayResult.Mode != ContinuationModeFreshRehydrate ||
		replayResult.Status != ContinuationStatusClosed ||
		requests.Load() != 2 {
		t.Fatalf(
			"source replay = observation %#v, next %p, result %#v, requests %d, error %v",
			replayObservation,
			replay,
			replayResult,
			requests.Load(),
			replayErr,
		)
	}

	implementation := productionInvocationFixture(
		t,
		adapter,
		ProfileOpenAIHTTP,
		implementationID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	setRecoverableInvocationFreshContext(
		t,
		&implementation,
		false,
		ContainmentReadWrite,
		ImplementerImplementation,
	)
	observation, next, result, err := (Dispatcher{}).InvokeTurn(
		context.Background(),
		implementation,
		targetBinding,
		promoted,
	)
	if err != nil || observation.Handoff == nil ||
		observation.Yield != nil || next != nil ||
		result.Mode != ContinuationModeOpaqueReplay ||
		result.Status != ContinuationStatusResumed ||
		requests.Load() != 3 {
		t.Fatalf(
			"implementation resume = observation %#v, next %p, result %#v, requests %d, error %v",
			observation,
			next,
			result,
			requests.Load(),
			err,
		)
	}
	replayObservation, next, replayResult, replayErr =
		(Dispatcher{}).InvokeTurn(
			context.Background(),
			implementation,
			targetBinding,
			&promotedAlias,
		)
	if replayErr != nil ||
		!reflect.DeepEqual(replayObservation, Observation{}) ||
		next != nil ||
		replayResult.Mode != ContinuationModeFreshRehydrate ||
		replayResult.Status != ContinuationStatusClosed ||
		requests.Load() != 3 {
		t.Fatalf(
			"promoted replay = observation %#v, next %p, result %#v, requests %d, error %v",
			replayObservation,
			next,
			replayResult,
			requests.Load(),
			replayErr,
		)
	}
}

func TestRecoverableDesignPromotionPreservesOnlyItsExactContract(t *testing.T) {
	t.Parallel()
	const (
		mismatchID  = "recoverable-promotion-mismatch"
		wrongDutyID = "recoverable-promotion-wrong-duty"
		yieldID     = "recoverable-promotion-yield"
	)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		switch requests.Add(1) {
		case 1, 2, 3:
			writeRecoverableYield(
				t, writer, mismatchID, "Which constraint controls?",
			)
		case 4:
			writeRecoverableYield(
				t, writer, wrongDutyID, "Which implementation base?",
			)
		case 5:
			writeRecoverableYield(
				t, writer, yieldID, "Which constraint controls?",
			)
		case 6:
			writeRecoverableYieldCall(
				t,
				writer,
				yieldID,
				"One bounded question remains.",
				"promoted-design-yield-again",
			)
		default:
			t.Errorf("unexpected provider request %d", requests.Load())
		}
	}))
	defer server.Close()
	adapter := recoverableProviderAdapter(t, server.URL)
	recoveryBinding := continuationContractBinding()
	targetBinding := recoveryBinding
	targetBinding.ToolContractDigest =
		Digest([]byte("exact-design-promotion"))
	input := &RecoverableTurnInput{
		SchemaVersion: RecoverableTurnInputSchemaVersion,
		Kind:          RecoverableInputAnswer,
		Answer:        "Use the exact admitted evidence.",
	}

	for index, mutate := range []func(*ContinuationBinding){
		func(binding *ContinuationBinding) {
			binding.Slice = "different-slice"
		},
		func(binding *ContinuationBinding) {
			binding.PlanAuthorityDigest =
				Digest([]byte("different-plan-authority"))
		},
		func(binding *ContinuationBinding) {
			binding.TargetAuthorityDigest =
				Digest([]byte("different-target-authority"))
		},
	} {
		mismatch := productionInvocationFixture(
			t,
			adapter,
			ProfileOpenAIHTTP,
			mismatchID,
			RoleImplementer,
			ImplementerDesign,
			ReadOnly,
		)
		_, mismatchHandle, _, err := (Dispatcher{}).InvokeTurn(
			context.Background(),
			mismatch,
			recoveryBinding,
			nil,
		)
		if err != nil || mismatchHandle == nil {
			t.Fatalf(
				"mismatch %d setup handle=%p error=%v",
				index,
				mismatchHandle,
				err,
			)
		}
		mismatchAlias := *mismatchHandle
		wrongTarget := targetBinding
		mutate(&wrongTarget)
		input.TargetBinding = &wrongTarget
		observation, next, result, err :=
			(Dispatcher{}).InvokeRecoverableTurn(
				context.Background(),
				mismatch,
				recoveryBinding,
				mismatchHandle,
				input,
			)
		if err != nil ||
			!reflect.DeepEqual(observation, Observation{}) ||
			next != nil ||
			result.Status != ContinuationStatusMismatch ||
			requests.Load() != int64(index+1) {
			t.Fatalf(
				"mismatch %d = observation %#v, next %p, result %#v, requests %d, error %v",
				index,
				observation,
				next,
				result,
				requests.Load(),
				err,
			)
		}
		input.TargetBinding = &targetBinding
		_, _, result, err =
			(Dispatcher{}).InvokeRecoverableTurn(
				context.Background(),
				mismatch,
				recoveryBinding,
				&mismatchAlias,
				input,
			)
		if err != nil ||
			result.Status != ContinuationStatusClosed ||
			requests.Load() != int64(index+1) {
			t.Fatalf(
				"mismatch %d alias result=%#v requests=%d error=%v",
				index,
				result,
				requests.Load(),
				err,
			)
		}
	}

	wrongDuty := productionInvocationFixture(
		t,
		adapter,
		ProfileOpenAIHTTP,
		wrongDutyID,
		RoleImplementer,
		ImplementerImplementation,
		ReadWrite,
	)
	_, wrongDutyHandle, _, err :=
		(Dispatcher{}).InvokeRecoverableTurn(
			context.Background(),
			wrongDuty,
			recoveryBinding,
			nil,
			nil,
		)
	if err != nil || wrongDutyHandle == nil || requests.Load() != 4 {
		t.Fatalf(
			"wrong-duty setup handle=%p requests=%d error=%v",
			wrongDutyHandle,
			requests.Load(),
			err,
		)
	}
	input.TargetBinding = &targetBinding
	observation, next, result, err :=
		(Dispatcher{}).InvokeRecoverableTurn(
			context.Background(),
			wrongDuty,
			recoveryBinding,
			wrongDutyHandle,
			input,
		)
	if err != nil || !reflect.DeepEqual(observation, Observation{}) || next != nil ||
		result.Status != ContinuationStatusMismatch ||
		requests.Load() != 4 {
		t.Fatalf(
			"wrong-duty promotion = observation %#v, next %p, result %#v, requests %d, error %v",
			observation,
			next,
			result,
			requests.Load(),
			err,
		)
	}

	yielding := productionInvocationFixture(
		t,
		adapter,
		ProfileOpenAIHTTP,
		yieldID,
		RoleImplementer,
		ImplementerDesign,
		ReadOnly,
	)
	_, yieldingHandle, _, err := (Dispatcher{}).InvokeTurn(
		context.Background(),
		yielding,
		recoveryBinding,
		nil,
	)
	if err != nil || yieldingHandle == nil || requests.Load() != 5 {
		t.Fatalf(
			"yield setup handle=%p requests=%d error=%v",
			yieldingHandle,
			requests.Load(),
			err,
		)
	}
	input.TargetBinding = &targetBinding
	observation, next, result, err =
		(Dispatcher{}).InvokeRecoverableTurn(
			context.Background(),
			yielding,
			recoveryBinding,
			yieldingHandle,
			input,
		)
	if err != nil || observation.Yield == nil ||
		observation.Handoff != nil || next == nil ||
		result.Status != ContinuationStatusSuspended ||
		requests.Load() != 6 {
		t.Fatalf(
			"yielded promotion = observation %#v, next %p, result %#v, requests %d, error %v",
			observation,
			next,
			result,
			requests.Load(),
			err,
		)
	}
	cell := continuationCellFor(next)
	cell.mu.Lock()
	flow := cell.flow
	cell.mu.Unlock()
	if flow != continuationFlowRecoverable {
		t.Fatalf("yielded promotion flow = %d", flow)
	}
	if err := next.Close(); err != nil {
		t.Fatal(err)
	}
}

func setRecoverableInvocationFreshContext(
	t *testing.T,
	invocation *Invocation,
	fresh bool,
	containment ContainmentProfile,
	responsibility Responsibility,
) {
	t.Helper()
	invocation.Request.FreshContext = fresh
	permission, err := NewSubmissionPermission(
		invocation.Request,
		invocation.Selected,
		containment,
		responsibility,
	)
	if err != nil {
		t.Fatal(err)
	}
	invocation.Permission = permission
}

func recoverableProviderAdapter(t *testing.T, endpoint string) Adapter {
	t.Helper()
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "openai-recoverable", ID: "sworn.openai.recoverable",
				Version: "1.0.0", Endpoint: endpoint + "/v1/responses",
				CredentialHeader: "Authorization",
				CredentialPrefix: "Bearer ",
				CredentialRefs:   []string{"credential-ref"},
				ResponseBytes:    MaxProviderResponseBytes,
			},
			API: OpenAIResponsesAPI, ReasoningEffort: "none",
		},
		func(context.Context, string) ([]byte, error) {
			return []byte("secret"), nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func writeRecoverableYield(
	t *testing.T,
	writer http.ResponseWriter,
	invocationID string,
	message string,
) {
	t.Helper()
	writeRecoverableYieldCall(
		t,
		writer,
		invocationID,
		message,
		invocationID,
	)
}

func writeRecoverableYieldCall(
	t *testing.T,
	writer http.ResponseWriter,
	invocationID string,
	message string,
	correlation string,
) {
	t.Helper()
	arguments, err := json.Marshal(map[string]any{
		"yield": Yield{
			SchemaVersion: YieldSchemaVersion,
			InvocationID:  invocationID,
			Kind:          YieldQuestion,
			Message:       message,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeJSONResponse(t, writer, responsesToolCallResponse(
		"yield-"+correlation,
		"function-"+correlation,
		"call-"+correlation,
		"sworn_yield",
		string(arguments),
		1,
		1,
	))
}

func writeRecoverableSubmit(
	t *testing.T,
	writer http.ResponseWriter,
	invocationID string,
) {
	t.Helper()
	writeRecoverableSubmission(
		t,
		writer,
		invocationID,
		ImplementerImplementation,
	)
}

func writeRecoverableSubmission(
	t *testing.T,
	writer http.ResponseWriter,
	invocationID string,
	responsibility Responsibility,
) {
	t.Helper()
	writeRecoverableSubmissionCall(
		t,
		writer,
		invocationID,
		responsibility,
		invocationID,
	)
}

func writeRecoverableSubmissionCall(
	t *testing.T,
	writer http.ResponseWriter,
	invocationID string,
	responsibility Responsibility,
	correlation string,
) {
	t.Helper()
	submission := submissionFixture(
		t, invocationID, responsibility, "",
	)
	writeJSONResponse(t, writer, responsesToolCallResponse(
		"submit-"+correlation,
		"function-"+correlation,
		"call-"+correlation,
		"sworn_submit",
		submissionToolArguments(t, submission),
		1,
		1,
	))
}
