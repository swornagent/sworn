//go:build linux

package driver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestRecoverableProviderTurnRetainsYieldAndRehydratesWithInput(
	t *testing.T,
) {
	t.Parallel()
	const (
		startID     = "recoverable-start"
		answerID    = "recoverable-answer"
		nudgeID     = "recoverable-nudge"
		rehydrateID = "recoverable-rehydrate"
	)
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
			if prompt.InvocationID != startID || prompt.Recovery != nil {
				t.Errorf("start prompt = %#v", prompt)
			}
			writeRecoverableYield(
				t, writer, startID, "Which base should I use?",
			)
		case 2:
			if prompt.InvocationID != answerID ||
				prompt.Recovery == nil ||
				prompt.Recovery.Kind != RecoverableInputAnswer ||
				prompt.Recovery.Content != "Use the pinned prepared base." {
				t.Errorf("answer prompt = %#v", prompt)
			}
			writeRecoverableYield(
				t, writer, answerID, "May I proceed with the bounded fix?",
			)
		case 3:
			if prompt.InvocationID != nudgeID ||
				prompt.Recovery == nil ||
				prompt.Recovery.Kind != RecoverableInputNudge ||
				prompt.Recovery.Content != recoverableTurnNudge {
				t.Errorf("nudge prompt = %#v", prompt)
			}
			writeRecoverableSubmit(t, writer, nudgeID)
		case 4:
			if prompt.InvocationID != rehydrateID ||
				prompt.Recovery == nil ||
				prompt.Recovery.Kind != RecoverableInputAnswer ||
				prompt.Recovery.Content != "Resume from durable evidence." {
				t.Errorf("rehydrate prompt = %#v", prompt)
			}
			writeRecoverableSubmit(t, writer, rehydrateID)
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
		startID,
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
		answerID,
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
		nudgeID,
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
		rehydrateID,
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
		"yield-"+invocationID,
		"function-"+invocationID,
		"call-"+invocationID,
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
	submission := submissionFixture(
		t, invocationID, ImplementerImplementation, "",
	)
	writeJSONResponse(t, writer, responsesToolCallResponse(
		"submit-"+invocationID,
		"function-"+invocationID,
		"call-"+invocationID,
		"sworn_submit",
		submissionToolArguments(t, submission),
		1,
		1,
	))
}
