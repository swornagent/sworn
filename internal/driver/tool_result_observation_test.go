//go:build linux

package driver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recordingToolResultHook collects emitted turns in call order.
type recordingToolResultHook struct {
	mu    sync.Mutex
	turns []ToolResultTurn
}

func (recorder *recordingToolResultHook) hook() ToolResultHook {
	return func(_ context.Context, turn ToolResultTurn) error {
		recorder.mu.Lock()
		defer recorder.mu.Unlock()
		copyTurn := turn
		copyTurn.Results = append([]ToolResultRecord(nil), turn.Results...)
		recorder.turns = append(recorder.turns, copyTurn)
		return nil
	}
}

func (recorder *recordingToolResultHook) snapshot() []ToolResultTurn {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	result := make([]ToolResultTurn, len(recorder.turns))
	copy(result, recorder.turns)
	return result
}

func (recorder *recordingToolResultHook) waitFor(t *testing.T, count int) []ToolResultTurn {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		turns := recorder.snapshot()
		if len(turns) >= count {
			return turns
		}
		if time.Now().After(deadline) {
			t.Fatalf("recorded turns = %d, want >= %d", len(turns), count)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func decodeRecordSpans(t *testing.T, record ToolResultRecord) ([]byte, []byte) {
	t.Helper()
	head, err := base64.StdEncoding.DecodeString(record.Head)
	if err != nil {
		t.Fatalf("head is not standard base64: %v", err)
	}
	tail, err := base64.StdEncoding.DecodeString(record.Tail)
	if err != nil {
		t.Fatalf("tail is not standard base64: %v", err)
	}
	return head, tail
}

func TestToolResultProjectionBoundsGeometryAndRedaction(t *testing.T) {
	// Under the bound: the full bytes ride with an explicit zero omitted.
	short := []byte("hello worker")
	shortRecord := projectToolResult(
		providerToolResult{ID: "c1", Name: "Read", Content: short},
		1,
		nil,
	)
	head, tail := decodeRecordSpans(t, shortRecord)
	if !bytes.Equal(head, short) || len(tail) != 0 {
		t.Fatalf("short spans = %q / %q", head, tail)
	}
	if shortRecord.TotalBytes != int64(len(short)) ||
		shortRecord.OmittedBytes != 0 ||
		shortRecord.RedactedBytes != 0 ||
		shortRecord.Sequence != 1 || shortRecord.ToolCallID != "c1" ||
		shortRecord.Tool != "Read" || shortRecord.Failed {
		t.Fatalf("short record = %#v", shortRecord)
	}

	// Over the bound: exact head/tail with a named omitted byte count.
	long := []byte(strings.Repeat("x", 5_000))
	longRecord := projectToolResult(
		providerToolResult{ID: "c2", Name: "Bash", Content: long, Failed: true},
		2,
		nil,
	)
	head, tail = decodeRecordSpans(t, longRecord)
	if len(head) != MaxToolResultHeadBytes ||
		!bytes.Equal(head, long[:MaxToolResultHeadBytes]) ||
		len(tail) != MaxToolResultTailBytes ||
		!bytes.Equal(tail, long[len(long)-MaxToolResultTailBytes:]) {
		t.Fatalf("long spans = %d/%d bytes", len(head), len(tail))
	}
	if longRecord.TotalBytes != 5_000 ||
		longRecord.OmittedBytes != 5_000-MaxToolResultHeadBytes-MaxToolResultTailBytes {
		t.Fatalf("long counts = %#v", longRecord)
	}

	// Redaction replaces held secrets with the fixed marker and names the
	// replaced byte count; the marker is shorter than the secret, so the
	// span only ever shrinks.
	secret := []byte("capability-secret-0123456789")
	planted := append(append([]byte("pre "), secret...), []byte(" post")...)
	redactedRecord := projectToolResult(
		providerToolResult{ID: "c3", Name: "Bash", Content: planted},
		3,
		[][]byte{secret},
	)
	head, _ = decodeRecordSpans(t, redactedRecord)
	if bytes.Contains(head, secret) {
		t.Fatalf("secret survived redaction: %q", head)
	}
	if !bytes.Contains(head, []byte(toolResultRedactedMarker)) ||
		redactedRecord.RedactedBytes != int64(len(secret)) ||
		redactedRecord.TotalBytes != int64(len(planted)) ||
		redactedRecord.OmittedBytes != 0 {
		t.Fatalf("redacted record = %#v (head %q)", redactedRecord, head)
	}
}

func TestToolResultTurnSplitsIntoNamedPartsUnderBudget(t *testing.T) {
	content := bytes.Repeat([]byte("y"), MaxToolResultHeadBytes+MaxToolResultTailBytes)
	records := make([]ToolResultRecord, 0, 64)
	for index := 0; index < 64; index++ {
		records = append(records, projectToolResult(
			providerToolResult{ID: "call-" + itoa(index+1), Name: "Read", Content: content},
			int64(index+1),
			nil,
		))
	}
	parts := splitToolResultParts(ToolResultTurn{Turn: 9, Results: records})
	if len(parts) < 2 {
		t.Fatalf("parts = %d, want a named split", len(parts))
	}
	var sequence int64
	for index, part := range parts {
		if len(part.Results) == 0 {
			t.Fatalf("part %d is empty", index)
		}
		if part.Part != int64(index+1) || part.Parts != int64(len(parts)) ||
			part.Turn != 9 {
			t.Fatalf("part geometry = %#v", part)
		}
		body, err := json.Marshal(part)
		if err != nil || len(body) > toolResultEventBudget {
			t.Fatalf("part %d marshals %d bytes, budget %d", index, len(body), toolResultEventBudget)
		}
		for _, record := range part.Results {
			sequence++
			if record.Sequence != sequence {
				t.Fatalf("part %d record sequence = %d, want %d (global across parts)",
					index, record.Sequence, sequence)
			}
		}
	}
	if sequence != 64 {
		t.Fatalf("sequences = %d, want 64", sequence)
	}
}

func TestToolResultObservationRedactsBoundSecretsBeforeHook(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	recorder := &recordingToolResultHook{}
	invocation.ToolResultHook = recorder.hook()
	secret := []byte("capability-secret-0123456789")
	body := "command printed " + string(secret) + " to output\n"
	if err := osWriteProviderFixture(
		invocation.HostWorkspace, "planted.txt", body,
	); err != nil {
		t.Fatal(err)
	}
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	session.bindRedactionSecrets([][]byte{secret})

	arguments, err := json.Marshal(map[string]any{
		"path": "/workspace/planted.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	result := session.execute(context.Background(), providerToolCall{
		ID: "read-planted", Name: "Read", Arguments: arguments,
	})
	if result.Failed || !bytes.Contains(result.Content, secret) {
		t.Fatalf("model-facing result = %q failed=%v", result.Content, result.Failed)
	}
	session.observeToolResultTurn(1, []providerToolResult{result})
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	turns := recorder.snapshot()
	if len(turns) != 1 || len(turns[0].Results) != 1 {
		t.Fatalf("turns = %#v", turns)
	}
	record := turns[0].Results[0]
	head, tail := decodeRecordSpans(t, record)
	if bytes.Contains(head, secret) || bytes.Contains(tail, secret) {
		t.Fatalf("secret reached the hook: %q / %q", head, tail)
	}
	if record.RedactedBytes != int64(len(secret)) ||
		record.TotalBytes != int64(len(body)) ||
		record.OmittedBytes != 0 {
		t.Fatalf("record = %#v", record)
	}
	if !bytes.Contains(head, []byte(toolResultRedactedMarker)) {
		t.Fatalf("marker missing from %q", head)
	}
}

// testProviderToolResultServer runs two provider turns: Read calls for the
// given contents then a terminal submit. It pins the model-facing bytes by
// asserting the exact content the second request carries.
func testProviderToolResultServer(
	t *testing.T,
	invocationID string,
	contents []string,
) (*httptest.Server, Adapter) {
	t.Helper()
	submission := submissionFixture(t, invocationID, ImplementerImplementation, "")
	submitArguments := submissionToolArguments(t, submission)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		turn := requests.Add(1)
		body, err := ioReadAllBounded(request.Body, MaxProviderRequestBytes)
		if err != nil {
			t.Error(err)
			return
		}
		if turn == 1 {
			var calls []any
			for index := range contents {
				calls = append(calls, openAIToolCallFixture(
					"read-"+itoa(index+1),
					"Read",
					`{"path":"/workspace/read-`+itoa(index+1)+`.txt"}`,
				))
			}
			writeJSONResponse(t, writer, map[string]any{
				"choices": []any{map[string]any{
					"message": map[string]any{
						"role": "assistant", "content": nil,
						"tool_calls": calls,
					},
					"finish_reason": "tool_calls",
				}},
				"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 3},
			})
			return
		}
		var requestBody struct {
			Messages []struct {
				Role       string `json:"role"`
				ToolCallID string `json:"tool_call_id"`
				Content    string `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal(body, &requestBody) != nil ||
			len(requestBody.Messages) != 2+len(contents) {
			t.Errorf("second request shape = %s", body)
			return
		}
		for index, content := range contents {
			if requestBody.Messages[2+index].Content != content {
				t.Errorf("model-facing content %d = %q, want %q",
					index, requestBody.Messages[2+index].Content, content)
			}
		}
		writeJSONResponse(t, writer, openAIToolCallResponse(
			"submit-final", "sworn_submit", submitArguments, 5, 7,
		))
	}))
	adapter, err := NewOpenAIAdapter(
		OpenAIProfileConfig{
			HTTPProfileConfig: HTTPProfileConfig{
				Key: "openai-tool-result", ID: "sworn.openai.toolresult", Version: "1.0.0",
				Endpoint:         server.URL + "/v1/chat/completions",
				CredentialHeader: "Authorization", CredentialPrefix: "Bearer ",
				CredentialRefs: []string{"credential-ref"},
				ResponseBytes:  MaxProviderResponseBytes,
			},
			API:             OpenAIChatCompletionsAPI,
			ReasoningEffort: "none",
		},
		func(context.Context, string) ([]byte, error) { return []byte("secret"), nil },
		nil,
		nil,
		nil,
	)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	return server, adapter
}

func writeToolResultFixtures(
	t *testing.T,
	workspace string,
	contents []string,
) {
	t.Helper()
	for index, content := range contents {
		if err := osWriteProviderFixture(
			workspace, "read-"+itoa(index+1)+".txt", content,
		); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProviderToolResultEventsRideAppendResultsSeams(t *testing.T) {
	long := strings.Repeat("q", 5_000)
	contents := []string{"alpha", "beta", long}

	// Plain dispatch: retain=false, so the terminal submit turn appends
	// nothing to any model and must emit nothing.
	{
		server, adapter := testProviderToolResultServer(t, "tool-result-plain", contents)
		defer server.Close()
		invocation := productionInvocationFixture(
			t, adapter, ProfileOpenAIHTTP, "tool-result-plain",
			RoleImplementer, ImplementerImplementation, ReadWrite,
		)
		writeToolResultFixtures(t, invocation.HostWorkspace, contents)
		recorder := &recordingToolResultHook{}
		invocation.ToolResultHook = recorder.hook()
		observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
		if err != nil || observation.Handoff == nil {
			t.Fatalf("observation = %#v, error = %v", observation, err)
		}
		turns := recorder.snapshot()
		if len(turns) != 1 {
			t.Fatalf("emitted turns = %d, want 1 (terminal !retain emits nothing)", len(turns))
		}
		if turns[0].Turn != 1 || len(turns[0].Results) != 3 {
			t.Fatalf("turn = %#v", turns[0])
		}
		for index, record := range turns[0].Results {
			if record.Sequence != int64(index+1) {
				t.Fatalf("sequence = %d, want %d", record.Sequence, index+1)
			}
			head, tail := decodeRecordSpans(t, record)
			if index < 2 {
				if len(tail) != 0 ||
					string(head) != contents[index] ||
					record.OmittedBytes != 0 ||
					record.TotalBytes != int64(len(contents[index])) {
					t.Fatalf("record %d = %#v (%q / %q)", index, record, head, tail)
				}
				continue
			}
			if len(head) != MaxToolResultHeadBytes ||
				!bytes.Equal(head, []byte(long[:MaxToolResultHeadBytes])) ||
				len(tail) != MaxToolResultTailBytes ||
				!bytes.Equal(tail, []byte(long[len(long)-MaxToolResultTailBytes:])) ||
				record.OmittedBytes != int64(len(long))-MaxToolResultHeadBytes-MaxToolResultTailBytes ||
				record.TotalBytes != int64(len(long)) {
				t.Fatalf("long record = %#v", record)
			}
		}
	}

	// Retaining continuation: the terminal submit turn does cross into the
	// retained conversation, so its result must emit as well.
	{
		server, adapter := testProviderToolResultServer(t, "tool-result-retain", []string{"gamma"})
		defer server.Close()
		invocation := productionInvocationFixture(
			t, adapter, ProfileOpenAIHTTP, "tool-result-retain",
			RoleImplementer, ImplementerImplementation, ReadWrite,
		)
		writeToolResultFixtures(t, invocation.HostWorkspace, []string{"gamma"})
		recorder := &recordingToolResultHook{}
		invocation.ToolResultHook = recorder.hook()
		observation, _, err := adapter.(*loopAdapter).invokeRecoverableContinuation(
			context.Background(), invocation,
		)
		if err != nil || observation.Handoff == nil {
			t.Fatalf("observation = %#v, error = %v", observation, err)
		}
		turns := recorder.snapshot()
		if len(turns) != 2 {
			t.Fatalf("emitted turns = %d, want 2", len(turns))
		}
		if turns[0].Turn != 1 || turns[1].Turn != 2 {
			t.Fatalf("turn identities = %d, %d", turns[0].Turn, turns[1].Turn)
		}
		head, _ := decodeRecordSpans(t, turns[1].Results[0])
		if string(head) != "accepted" ||
			turns[1].Results[0].Tool != "sworn_submit" {
			t.Fatalf("terminal record = %#v (%q)", turns[1].Results[0], head)
		}
	}
}

func TestNativeBrokerToolResultEventsAcrossTurns(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	recorder := &recordingToolResultHook{}
	invocation.ToolResultHook = recorder.hook()
	if err := osWriteProviderFixture(
		invocation.HostWorkspace, "broker.txt", "broker body",
	); err != nil {
		t.Fatal(err)
	}
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	broker, err := newNativeBroker(session)
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	capability := broker.capability()
	defer clearBytes(capability)

	var mu sync.Mutex
	turn := int64(1)
	broker.bindTurnSource(func() int64 {
		mu.Lock()
		defer mu.Unlock()
		return turn
	})
	openNativeBrokerForTest(t, broker, capability)

	read := func(id int) {
		t.Helper()
		status, body := brokerRequest(t, broker, capability, toolCallRequest(
			id, "Read", map[string]any{"path": "/workspace/broker.txt"},
		))
		if status != http.StatusOK ||
			!bytes.Contains(body, []byte(`"text":"broker body"`)) {
			t.Fatalf("read call = %d %s", status, body)
		}
	}
	read(1)
	read(2)
	broker.flushPending()
	turns := recorder.waitFor(t, 1)
	if turns[0].Turn != 1 || len(turns[0].Results) != 2 {
		t.Fatalf("turn 1 events = %#v", turns)
	}
	for index, record := range turns[0].Results {
		if record.Sequence != int64(index+1) {
			t.Fatalf("sequence = %d", record.Sequence)
		}
		head, _ := decodeRecordSpans(t, record)
		if string(head) != "broker body" || record.OmittedBytes != 0 {
			t.Fatalf("record %d = %#v (%q)", index, record, head)
		}
	}

	mu.Lock()
	turn = 2
	mu.Unlock()
	read(3)
	broker.flushPending()
	turns = recorder.waitFor(t, 2)
	if turns[1].Turn != 2 ||
		len(turns[1].Results) != 1 || turns[1].Results[0].Sequence != 1 {
		t.Fatalf("turn 2 events = %#v", turns)
	}
}

func TestNativeBrokerRefusedResultNeverCrossesOrEmits(t *testing.T) {
	invocation, _, _ := memoryInvocationFixture(t)
	recorder := &recordingToolResultHook{}
	invocation.ToolResultHook = recorder.hook()
	session, err := newToolSession(invocation)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	broker, err := newNativeBroker(session)
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	capability := broker.capability()
	defer clearBytes(capability)
	openNativeBrokerForTest(t, broker, capability)

	status, body := brokerRequest(t, broker, capability, toolCallRequest(
		1, "sworn_submit", map[string]any{"submission": map[string]any{}},
	))
	if status != http.StatusConflict ||
		!bytes.Contains(body, []byte(`"message":"closed"`)) ||
		bytes.Contains(body, []byte(`"result"`)) {
		t.Fatalf("refused call = %d %s", status, body)
	}
	broker.flushPending()
	if turns := recorder.snapshot(); len(turns) != 0 {
		t.Fatalf("refused call emitted %#v", turns)
	}
}

func TestToolResultObserverDropsDrainAndBoundedClose(t *testing.T) {
	// A nil hook disables observation entirely.
	if observer := newToolResultObserver(nil); observer != nil {
		t.Fatal("nil hook must build no observer")
	}

	// Loud drops: one wedged delivery, a full queue, then the cumulative
	// count rides on the next accepted event.
	release := make(chan struct{})
	var deliveredMu sync.Mutex
	var delivered []ToolResultTurn
	observer := newToolResultObserver(func(
		_ context.Context, value ToolResultTurn,
	) error {
		if value.Turn == 0 {
			<-release
			return nil
		}
		deliveredMu.Lock()
		delivered = append(delivered, value)
		deliveredMu.Unlock()
		return nil
	})
	observer.enqueue(ToolResultTurn{Turn: 0})
	// Wait until the pump has popped turn 0 and wedged on the release
	// gate, so the queue geometry below is deterministic: 32 queued
	// turns, then loud drops.
	deadline := time.Now().Add(2 * time.Second)
	for observer.inFlight.Load() != 1 {
		if time.Now().After(deadline) {
			t.Fatal("pump never picked up the wedging turn")
		}
		time.Sleep(5 * time.Millisecond)
	}
	for turn := int64(1); turn <= 34; turn++ {
		observer.enqueue(ToolResultTurn{Turn: turn})
	}
	close(release)
	// Wait for the pump to drain the 32 queued turns before enqueueing
	// the next accepted event; it must carry the loud drop count.
	deadline = time.Now().Add(5 * time.Second)
	for {
		deliveredMu.Lock()
		drained := len(delivered) == 32
		deliveredMu.Unlock()
		if drained {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pump never drained the queued turns")
		}
		time.Sleep(5 * time.Millisecond)
	}
	observer.enqueue(ToolResultTurn{Turn: 35})
	deadline = time.Now().Add(5 * time.Second)
	for {
		deliveredMu.Lock()
		found := false
		var stamped *ToolResultTurn
		for index := range delivered {
			if delivered[index].Turn == 35 {
				copyValue := delivered[index]
				stamped = &copyValue
				found = true
			}
		}
		deliveredMu.Unlock()
		if found {
			if stamped.DroppedEvents != 2 {
				t.Fatalf("turn 35 dropped_events = %d, want 2", stamped.DroppedEvents)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("turn 35 never delivered")
		}
		time.Sleep(5 * time.Millisecond)
	}
	observer.close()
	if observer.dropped != 2 {
		t.Fatalf("dropped = %d, want 2", observer.dropped)
	}

	// An erroring hook degrades observation only: every call is dropped
	// loudly, close still returns.
	erroring := newToolResultObserver(func(
		context.Context, ToolResultTurn,
	) error {
		return fail("TEST_OBSERVER_ERROR")
	})
	erroring.enqueue(ToolResultTurn{Turn: 1})
	erroring.close()
	if erroring.dropped != 1 {
		t.Fatalf("erroring dropped = %d, want 1", erroring.dropped)
	}

	// A permanently blocked hook cannot stall dispatch completion beyond
	// the bounded drain: close returns and counts the wedged delivery.
	blocked := newToolResultObserver(func(
		context.Context, ToolResultTurn,
	) error {
		select {}
	})
	blocked.enqueue(ToolResultTurn{Turn: 1})
	started := time.Now()
	blocked.close()
	elapsed := time.Since(started)
	if elapsed > toolResultCloseDrainTotal+2*time.Second {
		t.Fatalf("close took %v, want bounded drain", elapsed)
	}
	if blocked.dropped != 1 {
		t.Fatalf("blocked dropped = %d, want 1", blocked.dropped)
	}
}

// normalizedObservationBytes makes two dispatches comparable regardless of
// wall-clock duration and per-session seal randomness: the delivery facts —
// transport status, usage, diagnostic, handoff submission bytes and digest,
// yield, and terminal events — stay byte-identical.
func normalizedObservationBytes(t *testing.T, observation Observation) []byte {
	t.Helper()
	copyObservation := observation
	copyObservation.DurationMillis = 0
	if copyObservation.Handoff != nil {
		handoff := *copyObservation.Handoff
		handoff.SealBytes = nil
		handoff.SealDigest = ""
		copyObservation.Handoff = &handoff
	}
	body, err := json.Marshal(copyObservation)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// runToolResultFaultFixture always uses the same invocation ID so the
// sealed submission bytes are byte-comparable across runs.
func runToolResultFaultFixture(
	t *testing.T,
	hook ToolResultHook,
) (Observation, time.Duration) {
	t.Helper()
	server, adapter := testProviderToolResultServer(
		t, "tool-result-fault", []string{"delta"},
	)
	defer server.Close()
	invocation := productionInvocationFixture(
		t, adapter, ProfileOpenAIHTTP, "tool-result-fault",
		RoleImplementer, ImplementerImplementation, ReadWrite,
	)
	writeToolResultFixtures(t, invocation.HostWorkspace, []string{"delta"})
	invocation.ToolResultHook = hook
	started := time.Now()
	observation, err := (Dispatcher{}).Invoke(context.Background(), invocation)
	if err != nil || observation.Handoff == nil {
		t.Fatalf("observation = %#v, error = %v", observation, err)
	}
	return observation, time.Since(started)
}

func TestToolResultHookFailuresAndBlocksNeverAlterDelivery(t *testing.T) {
	baseline, baselineElapsed := runToolResultFaultFixture(
		t, nil,
	)
	want := normalizedObservationBytes(t, baseline)

	// An always-erroring hook degrades observation only.
	erroring, erroringElapsed := runToolResultFaultFixture(
		t,
		func(context.Context, ToolResultTurn) error {
			return fail("TEST_OBSERVER_ERROR")
		},
	)
	if got := normalizedObservationBytes(t, erroring); !bytes.Equal(got, want) {
		t.Fatalf("erroring-hook observation differs:\n%q\n%q", got, want)
	}
	if erroringElapsed > baselineElapsed+10*time.Second {
		t.Fatalf("erroring-hook dispatch took %v vs baseline %v",
			erroringElapsed, baselineElapsed)
	}

	// A permanently blocking hook cannot fail, stall, or alter delivery:
	// the dispatch completes and its observation is byte-identical.
	block := make(chan struct{})
	defer close(block)
	blocked, blockedElapsed := runToolResultFaultFixture(
		t,
		func(context.Context, ToolResultTurn) error {
			<-block
			return nil
		},
	)
	if got := normalizedObservationBytes(t, blocked); !bytes.Equal(got, want) {
		t.Fatalf("blocked-hook observation differs:\n%q\n%q", got, want)
	}
	if blockedElapsed > baselineElapsed+toolResultCloseDrainTotal+3*time.Second {
		t.Fatalf("blocked-hook dispatch took %v vs baseline %v",
			blockedElapsed, baselineElapsed)
	}
}
