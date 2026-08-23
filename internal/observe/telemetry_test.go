package observe

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/journal"
)

type capturedSpan struct {
	name         string
	attributes   map[string]any
	resource     map[string]any
	traceID      [16]byte
	spanID       [8]byte
	parentSpanID [8]byte
	startedAt    time.Time
	endedAt      time.Time
	parent       bool
	events       int
	links        int
}

type captureTraceExporter struct {
	mu        sync.Mutex
	spans     []capturedSpan
	callSizes []int
	exports   int
	exportErr error
	// failOnCall makes the given ExportSpans call (1-based) fail once; the
	// failing call captures nothing, so later calls keep their numbering.
	failOnCall int
	started    chan struct{}
	release    chan struct{}
	startOnce  sync.Once
}

func (e *captureTraceExporter) ExportSpans(
	ctx context.Context,
	spans []telemetrySpan,
) error {
	if e.started != nil {
		e.startOnce.Do(func() { close(e.started) })
	}
	if e.release != nil {
		select {
		case <-e.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.exports++
	if e.failOnCall != 0 && e.exports == e.failOnCall {
		return errors.New("capture export failed")
	}
	e.callSizes = append(e.callSizes, len(spans))
	for _, span := range spans {
		e.spans = append(e.spans, capturedSpan{
			name:         span.name,
			attributes:   attributeMap(span.attributes),
			resource:     resourceMap(span.resource),
			traceID:      span.traceID,
			spanID:       span.spanID,
			parentSpanID: span.parentSpanID,
			startedAt:    span.startedAt,
			endedAt:      span.endedAt,
			parent:       span.hasParent,
		})
	}
	return e.exportErr
}

func (e *captureTraceExporter) Shutdown(context.Context) error {
	return nil
}

func (e *captureTraceExporter) snapshot() ([]capturedSpan, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]capturedSpan(nil), e.spans...), e.exports
}

type capturedMetric struct {
	name       string
	attributes map[string]any
	value      int64
	resource   map[string]any
}

type captureMetricExporter struct {
	mu        sync.Mutex
	metrics   []capturedMetric
	exports   int
	exportErr error
}

func (e *captureMetricExporter) Export(
	_ context.Context,
	value *telemetryMetricPayload,
) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.exports++
	resourceAttributes := resourceMap(value.resource)
	for _, metric := range value.metrics {
		for _, point := range metric.points {
			e.metrics = append(e.metrics, capturedMetric{
				name:       metric.name,
				attributes: attributeMap(point.attributes),
				value:      point.value,
				resource:   resourceAttributes,
			})
		}
	}
	return e.exportErr
}

func (*captureMetricExporter) Shutdown(context.Context) error {
	return nil
}

func (e *captureMetricExporter) snapshot() ([]capturedMetric, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]capturedMetric(nil), e.metrics...), e.exports
}

func attributeMap(values []telemetryAttribute) map[string]any {
	result := make(map[string]any, len(values))
	for _, value := range values {
		switch value.kind {
		case telemetryStringAttribute:
			result[value.key] = value.stringValue
		case telemetryInt64Attribute:
			result[value.key] = value.int64Value
		case telemetryBoolAttribute:
			result[value.key] = value.boolValue
		}
	}
	return result
}

func resourceMap(value telemetryResource) map[string]any {
	return map[string]any{
		"service.name":    value.serviceName,
		"service.version": value.serviceVersion,
	}
}

func testTelemetryRecord(sentinel string) Record {
	started := time.Date(2026, 7, 29, 1, 2, 3, 0, time.UTC)
	observed := started.Add(30 * time.Second)
	usage := UsageSummary{
		InputTokens:  int64Pointer(120),
		OutputTokens: int64Pointer(30),
		Costs: []CostTotal{{
			Currency:   "AUD",
			MicroUnits: 70,
			Source:     driver.CostSourceProviderReported,
		}},
		TokenCoverage:      knownRatio(1, 2),
		CostCoverage:       knownRatio(1, 2),
		CacheReadTokens:    int64Pointer(80),
		CacheWriteTokens:   int64Pointer(20),
		CacheCoverage:      knownRatio(1, 2),
		EffortRequested:    textPointer("high"),
		EffortReported:     textPointer("high"),
		FinishReason:       textPointer("stop"),
		Truncated:          boolPointer(false),
		UnreportedSurfaces: []string{"sworn.missing"},
	}
	return Record{
		SchemaVersion: EvalSchemaVersion,
		ID:            "eval-" + sentinel,
		SwornVersion:  "0.3.0-dev",
		RunID:         "run-" + sentinel,
		Release:       "release-" + sentinel,
		RunState:      "complete",
		Outcome:       "pass",
		ThroughOffset: 17,
		StartedAt:     started,
		ObservedAt:    observed,
		ElapsedNS:     observed.Sub(started).Nanoseconds(),
		Events:        8,
		Attempts:      2,
		Retries:       1,
		Recovery: RecoverySummary{
			Uncertain:  1,
			Reconciled: 1,
		},
		Continuation: ContinuationSummary{
			Reused:   1,
			Fallback: 2,
			Expired:  1,
			Counts: []ContinuationCount{
				{
					Mode:    "fresh_rehydrate",
					Outcome: "fallback",
					Count:   1,
				},
				{
					Mode:    "fresh_rehydrate",
					Outcome: "fallback_expired",
					Count:   1,
				},
				{
					Mode:    "provider_cursor",
					Outcome: "reuse",
					Count:   1,
				},
			},
		},
		TurnRecovery: TurnRecoverySummary{
			Recovered:        1,
			HumanEscalations: 1,
			Actions: []TurnRecoveryCount{
				{Action: turnRecoveryAskCaptain, Count: 1},
				{Action: turnRecoveryPauseForHuman, Count: 1},
				{Action: turnRecoveryResumeWorker, Count: 1},
			},
		},
		DurationNS: knownRatio(30, 2),
		Usage:      usage,
		Groups: []AttemptGroup{{
			Role:                  "implementer",
			Responsibility:        "implementer_implementation",
			Operation:             "driver.dispatch",
			Transport:             "completed",
			Outcome:               "succeeded",
			Attempts:              2,
			Retries:               1,
			DurationNS:            knownRatio(30, 2),
			ObservationDurationNS: knownRatio(30, 2),
			Profile:               "sworn.test",
			Model:                 "model-a",
			Usage:                 usage,
			TurnEconomics: TurnEconomics{
				Turns:            int64Pointer(3),
				ToolCalls:        int64Pointer(2),
				ToolCallsPerTurn: knownRatio(2, 3),
				ToolCallMix: []ToolCallCount{
					{Name: "Bash", Count: 1},
					{Name: "Read", Count: 1},
				},
			},
		}},
		Quality: []Quality{
			{Name: "delivery", Numerator: int64Pointer(1),
				Denominator: int64Pointer(1)},
			{Name: "integration", Numerator: int64Pointer(1),
				Denominator: int64Pointer(1)},
			{Name: "requirements"},
			{Name: "verification", Numerator: int64Pointer(1),
				Denominator: int64Pointer(1)},
		},
	}
}

func testTelemetryResource() telemetryResource {
	return telemetryResource{
		serviceName:    "sworn",
		serviceVersion: "0.3.0-dev",
	}
}

func testDispatchAttempt(
	t *testing.T,
	effectKind string,
	effectState string,
	responsibility string,
	attempt int64,
	transport string,
	startedAt time.Time,
	finishedAt time.Time,
	receipt driver.UsageReceipt,
) dispatchAttempt {
	t.Helper()
	body, err := driver.EncodeUsageReceipt(receipt)
	if err != nil {
		t.Fatalf("encode usage receipt: %v", err)
	}
	return dispatchAttempt{
		effectKind:     effectKind,
		effectState:    boundedEffectState(journal.EffectState(effectState)),
		responsibility: boundedResponsibility(responsibility),
		attempt:        attempt,
		transport:      boundedTransport(transport),
		startedAt:      startedAt,
		finishedAt:     finishedAt,
		usageBytes:     body,
	}
}

func testReportedReceipt() driver.UsageReceipt {
	return driver.UsageReceipt{
		SchemaVersion:    driver.UsageSchemaV2,
		Surface:          "sworn.gemini",
		TokenStatus:      driver.UsageReported,
		InputTokens:      int64Pointer(120),
		OutputTokens:     int64Pointer(30),
		CostStatus:       driver.UsageReported,
		CostMicroUnits:   int64Pointer(70),
		Currency:         textPointer("USD"),
		Source:           textPointer(driver.CostSourceProviderReported),
		CacheStatus:      driver.UsageReported,
		CacheReadTokens:  int64Pointer(8),
		CacheWriteTokens: int64Pointer(2),
		ReasoningTokens:  int64Pointer(1779),
		DurationMillis:   int64Pointer(1000),
		Profile:          textPointer("sworn.test"),
		Model:            textPointer("gemini-2.5-pro"),
	}
}

// testTelemetryDispatchRecord is the S2 carrier fixture: three dispatch
// attempts in journal order (a full reported v2 receipt, a loud unavailable
// v2 receipt, and a legacy silent blob), plus a bounded snapshot join that
// resolves two of them to their effect identity.
func testTelemetryDispatchRecord(t *testing.T) Record {
	t.Helper()
	record := testTelemetryRecord("dispatch")
	started := time.Date(2026, 7, 29, 1, 2, 4, 0, time.UTC)
	record.dispatchAttempts = []dispatchAttempt{
		testDispatchAttempt(
			t,
			dispatchEffectKind,
			"succeeded",
			"implementer_implementation",
			1,
			"completed",
			started,
			started.Add(time.Second),
			testReportedReceipt(),
		),
		testDispatchAttempt(
			t,
			dispatchEffectKind,
			"operational_failed",
			"work_verification",
			2,
			"timeout",
			started.Add(time.Second),
			started.Add(2*time.Second),
			driver.UsageReceipt{
				SchemaVersion:     driver.UsageSchemaV2,
				Surface:           "sworn.gemini",
				TokenStatus:       driver.UsageUnavailable,
				CostStatus:        driver.UsageUnavailable,
				CacheStatus:       driver.UsageUnavailable,
				UnavailableReason: driver.UsageReasonWireLacked,
			},
		),
		testDispatchAttempt(
			t,
			dispatchEffectKind,
			"succeeded",
			"captain_review",
			1,
			"completed",
			started.Add(2*time.Second),
			started.Add(3*time.Second),
			driver.UsageReceipt{
				TokenStatus: driver.UsageUnavailable,
				CostStatus:  driver.UsageUnavailable,
			},
		),
	}
	record.spanJoin = spanJoin{
		attempts: []cockpit.AttemptView{
			{
				EffectID:       "attempt/work-a/e2/t3",
				Number:         1,
				Responsibility: "implementer_implementation",
				Transport:      "completed",
				CreatedAt:      started.Add(time.Second),
			},
			{
				EffectID:       "attempt/work-b/e1/t1",
				Number:         2,
				Responsibility: "work_verification",
				Transport:      "timeout",
				CreatedAt:      started.Add(2 * time.Second),
			},
			{
				EffectID:       "attempt/work-c/e4/t2/human-park",
				Number:         1,
				Responsibility: "captain_review",
				Transport:      "completed",
				CreatedAt:      started.Add(3 * time.Second),
			},
		},
		evidence: []cockpit.Evidence{
			{
				EffectID:  "attempt/work-a/e2/t3",
				WorkID:    "work-a",
				Slice:     "S2-genai-spans",
				CreatedAt: started.Add(time.Second),
			},
			{
				EffectID:  "attempt/work-b/e1/t1",
				WorkID:    "work-b",
				Track:     "T1-telemetry",
				Slice:     "S1-usage-truth",
				CreatedAt: started.Add(2 * time.Second),
			},
		},
		sliceTracks: map[string]string{
			"slice:S2-genai-spans": "T1-telemetry",
		},
	}
	return record
}

func dispatchSpanByName(
	t *testing.T,
	spans []telemetrySpan,
) map[string]telemetrySpan {
	t.Helper()
	result := make(map[string]telemetrySpan)
	for _, span := range spans {
		if span.name != dispatchSpanName {
			continue
		}
		key, ok := span.attributesByName("sworn.responsibility")
		if !ok {
			t.Fatalf("dispatch span without responsibility: %#v", span)
		}
		result[key] = span
	}
	return result
}

func (s telemetrySpan) attributesByName(key string) (string, bool) {
	for _, attribute := range s.attributes {
		if attribute.key == key &&
			attribute.kind == telemetryStringAttribute {
			return attribute.stringValue, true
		}
	}
	return "", false
}

// TestTelemetryDispatchSpansCarryPinnedGenAIVocabulary pins A1, A3, and A4:
// one sworn.dispatch span per dispatch attempt with the three verbatim
// GenAI keys when usage was reported, the in-band status trio when it was
// not, cache/reasoning components only when reported, the bounded sworn.*
// identity facts, and no sworn.verdict anywhere.
func TestTelemetryDispatchSpansCarryPinnedGenAIVocabulary(t *testing.T) {
	t.Parallel()

	record := testTelemetryDispatchRecord(t)
	spans := telemetrySpans(record, testTelemetryResource(), nil)
	if len(spans) != 5 {
		t.Fatalf("spans = %d, want 5 (segment+recovery+3 dispatch)", len(spans))
	}
	segment := spans[0]
	byResponsibility := dispatchSpanByName(t, spans)
	if len(byResponsibility) != 3 {
		t.Fatalf("dispatch spans = %#v", spans)
	}
	reported := byResponsibility["implementer_implementation"]
	loud := byResponsibility["work_verification"]
	legacy := byResponsibility["captain_review"]

	assertExactKeys(t, attributeMap(reported.attributes), stringSet(
		"sworn.run",
		"sworn.release",
		"sworn.role",
		"sworn.responsibility",
		"sworn.attempt",
		"sworn.epoch",
		"sworn.try",
		"sworn.track",
		"sworn.slice",
		"sworn.transport_status",
		"sworn.outcome",
		"sworn.usage_status",
		"gen_ai.request.model",
		"gen_ai.usage.input_tokens",
		"gen_ai.usage.output_tokens",
		"sworn.usage.cache_read_tokens",
		"sworn.usage.cache_write_tokens",
		"sworn.usage.reasoning_tokens",
	))
	reportedAttributes := attributeMap(reported.attributes)
	for key, want := range map[string]any{
		"sworn.run":                      "run-dispatch",
		"sworn.release":                  "release-dispatch",
		"sworn.role":                     "implementer",
		"sworn.responsibility":           "implementer_implementation",
		"sworn.attempt":                  int64(1),
		"sworn.epoch":                    int64(2),
		"sworn.try":                      int64(3),
		"sworn.track":                    "T1-telemetry",
		"sworn.slice":                    "S2-genai-spans",
		"sworn.transport_status":         "completed",
		"sworn.outcome":                  "succeeded",
		"sworn.usage_status":             "reported",
		"gen_ai.request.model":           "gemini-2.5-pro",
		"gen_ai.usage.input_tokens":      int64(120),
		"gen_ai.usage.output_tokens":     int64(30),
		"sworn.usage.cache_read_tokens":  int64(8),
		"sworn.usage.cache_write_tokens": int64(2),
		"sworn.usage.reasoning_tokens":   int64(1779),
	} {
		if got := reportedAttributes[key]; got != want {
			t.Fatalf("%s = %#v, want %#v", key, got, want)
		}
	}

	assertExactKeys(t, attributeMap(loud.attributes), stringSet(
		"sworn.run",
		"sworn.release",
		"sworn.role",
		"sworn.responsibility",
		"sworn.attempt",
		"sworn.epoch",
		"sworn.try",
		"sworn.track",
		"sworn.slice",
		"sworn.transport_status",
		"sworn.outcome",
		"sworn.usage_status",
		"sworn.surface",
		"sworn.usage_unavailable_reason",
	))
	loudAttributes := attributeMap(loud.attributes)
	for key, want := range map[string]any{
		"sworn.role":                     "verifier",
		"sworn.responsibility":           "work_verification",
		"sworn.attempt":                  int64(2),
		"sworn.epoch":                    int64(1),
		"sworn.try":                      int64(1),
		"sworn.track":                    "T1-telemetry",
		"sworn.slice":                    "S1-usage-truth",
		"sworn.transport_status":         "timeout",
		"sworn.outcome":                  "operational_failed",
		"sworn.usage_status":             "unavailable",
		"sworn.surface":                  "sworn.gemini",
		"sworn.usage_unavailable_reason": "wire-lacked-usage",
	} {
		if got := loudAttributes[key]; got != want {
			t.Fatalf("%s = %#v, want %#v", key, got, want)
		}
	}

	// Legacy silent blob: epoch/try resolve from the effect id (trailing
	// derived path tolerated), evidence miss omits slice/track, and the
	// honest "unknown" surface/reason ride in-band.
	assertExactKeys(t, attributeMap(legacy.attributes), stringSet(
		"sworn.run",
		"sworn.release",
		"sworn.role",
		"sworn.responsibility",
		"sworn.attempt",
		"sworn.epoch",
		"sworn.try",
		"sworn.transport_status",
		"sworn.outcome",
		"sworn.usage_status",
		"sworn.surface",
		"sworn.usage_unavailable_reason",
	))
	legacyAttributes := attributeMap(legacy.attributes)
	for key, want := range map[string]any{
		"sworn.role":                     "captain",
		"sworn.responsibility":           "captain_review",
		"sworn.attempt":                  int64(1),
		"sworn.epoch":                    int64(4),
		"sworn.try":                      int64(2),
		"sworn.transport_status":         "completed",
		"sworn.outcome":                  "succeeded",
		"sworn.usage_status":             "unavailable",
		"sworn.surface":                  "unknown",
		"sworn.usage_unavailable_reason": "unknown",
	} {
		if got := legacyAttributes[key]; got != want {
			t.Fatalf("%s = %#v, want %#v", key, got, want)
		}
	}

	for _, span := range []telemetrySpan{reported, loud, legacy} {
		if _, present := attributeMap(span.attributes)["sworn.verdict"]; present {
			t.Fatalf("sworn.verdict emitted on %#v", span)
		}
		if !span.hasParent || span.parentSpanID != segment.spanID ||
			span.traceID != segment.traceID {
			t.Fatalf("dispatch span identity = %#v", span)
		}
	}

	// A1 wall-clock duration rides as span timing from the receipt stamp;
	// legacy receipts fall back to the event-pair timing.
	started := time.Date(2026, 7, 29, 1, 2, 4, 0, time.UTC)
	if !reported.startedAt.Equal(started) ||
		!reported.endedAt.Equal(started.Add(time.Second)) {
		t.Fatalf("reported timing = %v/%v", reported.startedAt, reported.endedAt)
	}
	if !loud.startedAt.Equal(started.Add(time.Second)) ||
		!loud.endedAt.Equal(started.Add(2*time.Second)) {
		t.Fatalf("loud timing = %v/%v", loud.startedAt, loud.endedAt)
	}
	if !legacy.startedAt.Equal(started.Add(2*time.Second)) ||
		!legacy.endedAt.Equal(started.Add(3*time.Second)) {
		t.Fatalf("legacy timing = %v/%v", legacy.startedAt, legacy.endedAt)
	}
}

// TestTelemetryDispatchSpanIdentitiesAreDeterministic pins A2: all spans of
// one run share one trace identity, re-evaluation yields identical span
// identities, distinct attempts get distinct span ids, and distinct runs get
// distinct traces.
func TestTelemetryDispatchSpanIdentitiesAreDeterministic(t *testing.T) {
	t.Parallel()

	record := testTelemetryDispatchRecord(t)
	resource := testTelemetryResource()
	first := telemetrySpans(record, resource, nil)
	second := telemetrySpans(record, resource, nil)
	if len(first) != len(second) {
		t.Fatalf("span counts differ: %d vs %d", len(first), len(second))
	}
	for index := range first {
		if first[index].traceID != second[index].traceID ||
			first[index].spanID != second[index].spanID {
			t.Fatalf("identity drifted on re-evaluation: %#v vs %#v",
				first[index], second[index])
		}
	}
	traceID := first[0].traceID
	seen := make(map[[8]byte]bool)
	for _, span := range first {
		if span.traceID != traceID {
			t.Fatalf("span %q has a different trace", span.name)
		}
		if seen[span.spanID] {
			t.Fatalf("duplicate span id in one record")
		}
		seen[span.spanID] = true
	}

	otherRun := record
	otherRun.RunID = "run-other"
	otherRun.ID = "eval-other"
	otherSpans := telemetrySpans(otherRun, resource, nil)
	if otherSpans[0].traceID == traceID {
		t.Fatalf("distinct runs share one trace identity")
	}
	for _, span := range otherSpans {
		if span.traceID != otherSpans[0].traceID {
			t.Fatalf("span %q has a different trace", span.name)
		}
	}
}

// TestTelemetryExportsEachAttemptSpanOnceAcrossAdvances pins the A2
// no-duplicate clause: re-evaluation over a cumulative record exports each
// attempt span exactly once.
func TestTelemetryExportsEachAttemptSpanOnceAcrossAdvances(t *testing.T) {
	t.Parallel()

	record := testTelemetryDispatchRecord(t)
	traceExporter := &captureTraceExporter{started: make(chan struct{})}
	metricExporter := &captureMetricExporter{}
	telemetry, err := newTelemetry(
		traceExporter,
		metricExporter,
		"0.3.0-dev",
		4,
		time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !telemetry.TryEnqueue(record) {
		t.Fatal("record was not accepted")
	}
	waitForProcessed(t, telemetry, 1)
	if !telemetry.TryEnqueue(record) {
		t.Fatal("re-evaluation record was not accepted")
	}
	waitForProcessed(t, telemetry, 2)
	shutdownTelemetry(t, telemetry)

	spans, exports := traceExporter.snapshot()
	if exports != 2 {
		t.Fatalf("exports = %d", exports)
	}
	dispatch := 0
	segment := 0
	seenIDs := make(map[[8]byte]bool)
	for _, span := range spans {
		switch span.name {
		case dispatchSpanName:
			dispatch++
			if seenIDs[span.spanID] {
				t.Fatalf("duplicate attempt span id exported")
			}
			seenIDs[span.spanID] = true
		case "sworn.process.segment":
			segment++
		}
	}
	if dispatch != 3 || segment != 2 {
		t.Fatalf("dispatch/segment spans = %d/%d", dispatch, segment)
	}
	status := telemetry.Status()
	if status.TraceExports != 2 || status.Failures != 0 {
		t.Fatalf("status = %#v", status)
	}
}

// TestTelemetryChunksLongDispatchHistoriesWithinExporterBounds pins C3: a
// long cumulative attempt history exports in bounded chunks (one oversized
// batch would be rejected wholesale by the exporter's 1 MiB cap), and the
// export-once filter keeps a re-evaluation from re-sending old attempts.
func TestTelemetryChunksLongDispatchHistoriesWithinExporterBounds(t *testing.T) {
	t.Parallel()

	record := testTelemetryDispatchRecord(t)
	base := record.dispatchAttempts[0]
	history := make([]dispatchAttempt, 0, 600)
	for index := 0; index < 600; index++ {
		entry := base
		entry.attempt = int64(index + 1)
		shift := time.Duration(index+1) * time.Second
		entry.startedAt = base.startedAt.Add(shift)
		entry.finishedAt = base.finishedAt.Add(shift)
		history = append(history, entry)
	}
	record.dispatchAttempts = history

	traceExporter := &captureTraceExporter{started: make(chan struct{})}
	metricExporter := &captureMetricExporter{}
	telemetry, err := newTelemetry(
		traceExporter,
		metricExporter,
		"0.3.0-dev",
		4,
		time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !telemetry.TryEnqueue(record) {
		t.Fatal("record was not accepted")
	}
	waitForProcessed(t, telemetry, 1)
	if !telemetry.TryEnqueue(record) {
		t.Fatal("re-evaluation record was not accepted")
	}
	waitForProcessed(t, telemetry, 2)
	shutdownTelemetry(t, telemetry)

	_, exports := traceExporter.snapshot()
	mu := &traceExporter.mu
	mu.Lock()
	callSizes := append([]int(nil), traceExporter.callSizes...)
	mu.Unlock()
	// 602 spans (segment+recovery+600 dispatch) export as 256+256+90 on
	// the first advance; the second advance carries only segment+recovery.
	wantSizes := []int{256, 256, 90, 2}
	if len(callSizes) != len(wantSizes) {
		t.Fatalf("call sizes = %v, want %v", callSizes, wantSizes)
	}
	for index, want := range wantSizes {
		if callSizes[index] != want {
			t.Fatalf("call sizes = %v, want %v", callSizes, wantSizes)
		}
	}
	if exports != 4 {
		t.Fatalf("exports = %d", exports)
	}
	status := telemetry.Status()
	if status.Failures != 0 || status.TraceExports != 2 {
		t.Fatalf("status = %#v", status)
	}
}

// TestTelemetryMarksExportedAttemptsOnlyAfterSuccessfulChunk pins the C3
// mark-on-success rule: a failed chunk leaves its attempt identities
// unmarked so the next advance retries exactly them.
func TestTelemetryMarksExportedAttemptsOnlyAfterSuccessfulChunk(t *testing.T) {
	t.Parallel()

	record := testTelemetryDispatchRecord(t)
	base := record.dispatchAttempts[0]
	history := make([]dispatchAttempt, 0, 300)
	for index := 0; index < 300; index++ {
		entry := base
		entry.attempt = int64(index + 1)
		shift := time.Duration(index+1) * time.Second
		entry.startedAt = base.startedAt.Add(shift)
		entry.finishedAt = base.finishedAt.Add(shift)
		history = append(history, entry)
	}
	record.dispatchAttempts = history

	traceExporter := &captureTraceExporter{
		started:    make(chan struct{}),
		failOnCall: 2,
	}
	metricExporter := &captureMetricExporter{}
	telemetry, err := newTelemetry(
		traceExporter,
		metricExporter,
		"0.3.0-dev",
		4,
		time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !telemetry.TryEnqueue(record) {
		t.Fatal("record was not accepted")
	}
	waitForProcessed(t, telemetry, 1)
	if !telemetry.TryEnqueue(record) {
		t.Fatal("re-evaluation record was not accepted")
	}
	waitForProcessed(t, telemetry, 2)
	shutdownTelemetry(t, telemetry)

	spans, exports := traceExporter.snapshot()
	if exports != 3 {
		t.Fatalf("exports = %d", exports)
	}
	seenIDs := make(map[[8]byte]bool)
	dispatch := 0
	for _, span := range spans {
		if span.name != dispatchSpanName {
			continue
		}
		dispatch++
		if seenIDs[span.spanID] {
			t.Fatalf("duplicate attempt span id exported")
		}
		seenIDs[span.spanID] = true
	}
	if dispatch != 300 {
		t.Fatalf("dispatch spans = %d, want 300", dispatch)
	}
	status := telemetry.Status()
	if status.Failures != 1 ||
		status.LastFailureCode != "trace_export" ||
		status.TraceExports != 1 {
		t.Fatalf("status = %#v", status)
	}
}

// TestTelemetryDispatchSpanOmitsWindowMissedAndAmbiguousIdentity pins the
// A3 loud-absence rule: an attempt evicted from the snapshot attempts
// window, an ambiguous attempts join, and ambiguous evidence all omit the
// affected attributes instead of guessing.
func TestTelemetryDispatchSpanOmitsWindowMissedAndAmbiguousIdentity(t *testing.T) {
	t.Parallel()

	record := testTelemetryDispatchRecord(t)
	started := time.Date(2026, 7, 29, 1, 2, 4, 0, time.UTC)
	windowMissed := testDispatchAttempt(
		t,
		dispatchEffectKind,
		"succeeded",
		"planner_proposal",
		1,
		"completed",
		started,
		started.Add(time.Second),
		testReportedReceipt(),
	)
	ambiguousAttempt := testDispatchAttempt(
		t,
		dispatchEffectKind,
		"claimed",
		"assembly_verification",
		1,
		"completed",
		started,
		started.Add(time.Second),
		driver.UsageReceipt{
			TokenStatus: driver.UsageUnavailable,
			CostStatus:  driver.UsageUnavailable,
		},
	)
	record.dispatchAttempts = append(
		record.dispatchAttempts,
		windowMissed,
		ambiguousAttempt,
	)
	// Two snapshot rows match the ambiguous attempt's tuple.
	record.spanJoin.attempts = append(
		record.spanJoin.attempts,
		cockpit.AttemptView{
			EffectID:       "attempt/work-x/e1/t1",
			Number:         1,
			Responsibility: "assembly_verification",
			Transport:      "completed",
			CreatedAt:      started.Add(time.Second),
		},
		cockpit.AttemptView{
			EffectID:       "attempt/work-y/e2/t2",
			Number:         1,
			Responsibility: "assembly_verification",
			Transport:      "completed",
			CreatedAt:      started.Add(time.Second),
		},
	)
	// Ambiguous evidence: two entries for one effect id.
	record.spanJoin.evidence = append(
		record.spanJoin.evidence,
		cockpit.Evidence{
			EffectID: "attempt/work-a/e2/t3",
			WorkID:   "work-a",
			Slice:    "S-other",
		},
	)

	spans := telemetrySpans(record, testTelemetryResource(), nil)
	byResponsibility := dispatchSpanByName(t, spans)
	missed := byResponsibility["planner_proposal"]
	assertExactKeys(t, attributeMap(missed.attributes), stringSet(
		"sworn.run",
		"sworn.release",
		"sworn.role",
		"sworn.responsibility",
		"sworn.attempt",
		"sworn.transport_status",
		"sworn.outcome",
		"sworn.usage_status",
		"gen_ai.request.model",
		"gen_ai.usage.input_tokens",
		"gen_ai.usage.output_tokens",
		"sworn.usage.cache_read_tokens",
		"sworn.usage.cache_write_tokens",
		"sworn.usage.reasoning_tokens",
	))
	ambiguous := byResponsibility["assembly_verification"]
	assertExactKeys(t, attributeMap(ambiguous.attributes), stringSet(
		"sworn.run",
		"sworn.release",
		"sworn.role",
		"sworn.responsibility",
		"sworn.attempt",
		"sworn.transport_status",
		"sworn.outcome",
		"sworn.usage_status",
		"sworn.surface",
		"sworn.usage_unavailable_reason",
	))
	// The evidence ambiguity leaves the first attempt's slice/track off,
	// while its epoch/try (from its own effect id) still ride.
	implementer := byResponsibility["implementer_implementation"]
	implementerAttributes := attributeMap(implementer.attributes)
	if _, present := implementerAttributes["sworn.slice"]; present {
		t.Fatalf("ambiguous evidence emitted slice: %#v", implementerAttributes)
	}
	if _, present := implementerAttributes["sworn.track"]; present {
		t.Fatalf("ambiguous evidence emitted track: %#v", implementerAttributes)
	}
	if implementerAttributes["sworn.epoch"] != int64(2) ||
		implementerAttributes["sworn.try"] != int64(3) {
		t.Fatalf("epoch/try = %#v", implementerAttributes)
	}
}

// TestTelemetryDispatchSpanOmitsUnstampedFacts pins the legacy-receipt
// rule: a reported receipt without the S1 model stamp omits
// gen_ai.request.model (never a fabricated id), and without a duration
// stamp the span falls back to the fact's event-pair timing.
func TestTelemetryDispatchSpanOmitsUnstampedFacts(t *testing.T) {
	t.Parallel()

	record := testTelemetryDispatchRecord(t)
	started := time.Date(2026, 7, 29, 1, 2, 4, 0, time.UTC)
	legacyReported := testDispatchAttempt(
		t,
		dispatchEffectKind,
		"succeeded",
		"planner_proposal",
		1,
		"completed",
		started,
		started.Add(2*time.Second),
		driver.UsageReceipt{
			TokenStatus:  driver.UsageReported,
			InputTokens:  int64Pointer(7),
			OutputTokens: int64Pointer(5),
			CostStatus:   driver.UsageUnavailable,
		},
	)
	record.dispatchAttempts = append(record.dispatchAttempts, legacyReported)

	spans := telemetrySpans(record, testTelemetryResource(), nil)
	span := dispatchSpanByName(t, spans)["planner_proposal"]
	attributes := attributeMap(span.attributes)
	assertExactKeys(t, attributes, stringSet(
		"sworn.run",
		"sworn.release",
		"sworn.role",
		"sworn.responsibility",
		"sworn.attempt",
		"sworn.transport_status",
		"sworn.outcome",
		"sworn.usage_status",
		"gen_ai.usage.input_tokens",
		"gen_ai.usage.output_tokens",
	))
	if attributes["gen_ai.usage.input_tokens"] != int64(7) ||
		attributes["gen_ai.usage.output_tokens"] != int64(5) ||
		attributes["sworn.usage_status"] != "reported" {
		t.Fatalf("attributes = %#v", attributes)
	}
	if !span.startedAt.Equal(started) ||
		!span.endedAt.Equal(started.Add(2*time.Second)) {
		t.Fatalf("fallback timing = %v/%v", span.startedAt, span.endedAt)
	}
}

// TestTelemetryDispatchSpanSkipsUnusableTiming pins the skip-at-construction
// rule: an entry that cannot form a valid span (a pathological duration
// stamp that would overflow, a zero finishedAt) is skipped, never turned
// into a batch-killing malformed span.
func TestTelemetryDispatchSpanSkipsUnusableTiming(t *testing.T) {
	t.Parallel()

	record := testTelemetryDispatchRecord(t)
	started := time.Date(2026, 7, 29, 1, 2, 4, 0, time.UTC)
	huge := testReportedReceipt()
	huge.DurationMillis = int64Pointer(maxDispatchDurationMillis + 1)
	record.dispatchAttempts = append(
		record.dispatchAttempts,
		testDispatchAttempt(
			t,
			dispatchEffectKind,
			"succeeded",
			"planner_proposal",
			1,
			"completed",
			started,
			started.Add(time.Second),
			huge,
		),
		testDispatchAttempt(
			t,
			dispatchEffectKind,
			"succeeded",
			"assembly_verification",
			1,
			"completed",
			time.Time{},
			time.Time{},
			testReportedReceipt(),
		),
	)

	spans := telemetrySpans(record, testTelemetryResource(), nil)
	byResponsibility := dispatchSpanByName(t, spans)
	if _, present := byResponsibility["planner_proposal"]; present {
		t.Fatalf("pathological duration stamp minted a span: %#v", spans)
	}
	if _, present := byResponsibility["assembly_verification"]; present {
		t.Fatalf("zero timing minted a span: %#v", spans)
	}
	if len(byResponsibility) != 3 {
		t.Fatalf("dispatch spans = %#v", spans)
	}
}

// TestTelemetryOnlyDispatchEffectsBecomeDispatchSpans pins C4: a
// non-dispatch attempt row never mints a sworn.dispatch span.
func TestTelemetryOnlyDispatchEffectsBecomeDispatchSpans(t *testing.T) {
	t.Parallel()

	record := testTelemetryDispatchRecord(t)
	nonDispatch := record.dispatchAttempts[0]
	nonDispatch.effectKind = "git.seal"
	nonDispatch.attempt = 77
	record.dispatchAttempts = append(record.dispatchAttempts, nonDispatch)

	spans := telemetrySpans(record, testTelemetryResource(), nil)
	dispatch := 0
	for _, span := range spans {
		if span.name == dispatchSpanName {
			dispatch++
		}
	}
	if dispatch != 3 {
		t.Fatalf("dispatch spans = %d, want 3", dispatch)
	}
}

// TestTelemetryDispatchSpansCarryNoContentOrSessionValues pins the A3
// content prohibition with live sentinels: content-shaped and
// session-shaped values planted in every carrier input that must not be
// projected still appear in no dispatch span attribute.
func TestTelemetryDispatchSpansCarryNoContentOrSessionValues(t *testing.T) {
	t.Parallel()

	const contentSentinel = "PROMPT_RESPONSE_REASONING_CONTENT_SENTINEL_9f31"
	const sessionSentinel = "SESSION_IDENTIFIER_SENTINEL_7c42"

	record := testTelemetryDispatchRecord(t)
	// Content-shaped values ride on the first attempt's receipt text facts
	// and must never reach a span attribute.
	reported := testReportedReceipt()
	reported.EffortReported = textPointer(contentSentinel)
	reported.FinishReason = textPointer(contentSentinel)
	body, err := driver.EncodeUsageReceipt(reported)
	if err != nil {
		t.Fatal(err)
	}
	record.dispatchAttempts[0].usageBytes = body
	// Session-shaped values ride on join inputs that are consumed but
	// never emitted: evidence WorkID and an unmatched evidence EffectID.
	record.spanJoin.evidence[0].WorkID = sessionSentinel
	record.spanJoin.evidence = append(
		record.spanJoin.evidence,
		cockpit.Evidence{
			EffectID: sessionSentinel,
			WorkID:   "work-session",
			Slice:    "S-session",
		},
	)

	spans := telemetrySpans(record, testTelemetryResource(), nil)
	dispatch := dispatchSpanByName(t, spans)
	if len(dispatch) != 3 {
		t.Fatalf("dispatch spans = %#v", spans)
	}
	for _, span := range dispatch {
		attributes := attributeMap(span.attributes)
		assertNoSentinel(t, contentSentinel, attributes)
		assertNoSentinel(t, sessionSentinel, attributes)
	}
}

// TestSpanDedupeEvictsOldestRuns pins the bounded FIFO dedupe state.
func TestSpanDedupeEvictsOldestRuns(t *testing.T) {
	t.Parallel()

	dedupe := newSpanDedupe()
	for index := 0; index < telemetryDedupeMaxRuns+4; index++ {
		var id [8]byte
		id[0] = byte(index + 1)
		dedupe.mark(fmt.Sprintf("run-%02d", index), id)
	}
	if len(dedupe.order) != telemetryDedupeMaxRuns {
		t.Fatalf("order = %v", dedupe.order)
	}
	for _, evicted := range []string{"run-00", "run-01", "run-02", "run-03"} {
		if _, present := dedupe.runs[evicted]; present {
			t.Fatalf("run %q was not evicted", evicted)
		}
	}
	if _, present := dedupe.runs["run-19"]; !present {
		t.Fatalf("most recent run was evicted: %v", dedupe.order)
	}
}

func TestNoopTelemetryIsDisabledAndInert(t *testing.T) {
	t.Parallel()

	telemetry := Noop()
	if telemetry.TryEnqueue(testTelemetryRecord("private")) {
		t.Fatal("disabled telemetry accepted a record")
	}
	if status := telemetry.Status(); status.Enabled ||
		status.SchemaVersion != TelemetryStatusSchemaVersion ||
		status.QueueCapacity != 0 {
		t.Fatalf("status = %#v", status)
	}
	if err := telemetry.Shutdown(nil); err != nil {
		t.Fatalf("shutdown = %v", err)
	}
}

func TestTelemetryExportsOnlyThePositiveAllowlist(t *testing.T) {
	t.Parallel()

	const sentinel = "PRIVATE_PATH_CREDENTIAL_MODEL_RUN_ID"
	traceExporter := &captureTraceExporter{started: make(chan struct{})}
	metricExporter := &captureMetricExporter{}
	telemetry, err := newTelemetry(
		traceExporter,
		metricExporter,
		"0.3.0-dev",
		4,
		time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !telemetry.TryEnqueue(testTelemetryRecord(sentinel)) {
		t.Fatal("record was not accepted")
	}
	select {
	case <-traceExporter.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start export")
	}
	shutdownTelemetry(t, telemetry)

	spans, traceExports := traceExporter.snapshot()
	if traceExports != 1 || len(spans) != 2 {
		t.Fatalf("spans = %#v, exports = %d", spans, traceExports)
	}
	sort.Slice(spans, func(left, right int) bool {
		return spans[left].name < spans[right].name
	})
	if spans[0].name != "sworn.process.segment" ||
		spans[1].name != "sworn.recovery" {
		t.Fatalf("span names = %q, %q", spans[0].name, spans[1].name)
	}
	allowedSpanAttributes := map[string]map[string]bool{
		"sworn.process.segment": stringSet(
			"sworn.schema",
			"sworn.measurement",
			"sworn.run.state",
			"sworn.outcome",
			"sworn.events",
			"sworn.attempts",
			"sworn.retries",
			"sworn.elapsed_ns",
			"sworn.usage_known",
			"sworn.input_tokens",
			"sworn.output_tokens",
			"sworn.cache_known",
			"sworn.cache_read_tokens",
			"sworn.cache_write_tokens",
			"sworn.effort_requested",
			"sworn.effort_reported",
			"sworn.finish_reason",
			"sworn.truncated",
		),
		"sworn.recovery": stringSet(
			"sworn.measurement",
			"sworn.recovery.uncertain",
			"sworn.recovery.reconciled",
			"sworn.recovery.recovered",
			"sworn.recovery.rolled_back",
		),
	}
	for _, span := range spans {
		assertExactKeys(t, span.attributes, allowedSpanAttributes[span.name])
		assertResource(t, span.resource)
		assertNoSentinel(t, sentinel, span.attributes)
		assertNoSentinel(t, sentinel, span.resource)
		if span.events != 0 || span.links != 0 {
			t.Fatalf("%s events/links = %d/%d", span.name, span.events, span.links)
		}
		if span.name == "sworn.process.segment" {
			if span.parent || !span.startedAt.Before(span.endedAt) {
				t.Fatalf("segment chronology = %#v", span)
			}
		} else if !span.parent || !span.startedAt.Equal(span.endedAt) {
			t.Fatalf("recovery chronology = %#v", span)
		}
	}

	metrics, metricExports := metricExporter.snapshot()
	if metricExports != 1 || len(metrics) == 0 {
		t.Fatalf("metrics = %#v, exports = %d", metrics, metricExports)
	}
	groupLabels := stringSet(
		"sworn.role",
		"sworn.responsibility",
		"sworn.operation",
		"sworn.transport",
		"sworn.outcome",
		"sworn.profile",
		"sworn.model",
		"sworn.usage_known",
		"sworn.cache_known",
		"sworn.effort_reported",
		"sworn.finish_reason",
		"sworn.truncated",
	)
	allowedMetricAttributes := map[string]map[string]bool{
		"sworn.eval.events":                              stringSet("sworn.outcome"),
		"sworn.eval.continuations":                       stringSet("sworn.continuation.mode", "sworn.continuation.outcome"),
		"sworn.eval.continuation.outcomes":               stringSet("sworn.continuation.outcome"),
		"sworn.eval.turn_recovery.actions":               stringSet("sworn.turn_recovery.action"),
		"sworn.eval.turn_recovery.outcomes":              stringSet("sworn.turn_recovery.outcome"),
		"sworn.eval.attempts":                            groupLabels,
		"sworn.eval.retries":                             groupLabels,
		"sworn.eval.recoveries":                          stringSet("sworn.recovery", "sworn.outcome"),
		"sworn.eval.duration_ns.numerator":               groupLabels,
		"sworn.eval.duration_ns.denominator":             groupLabels,
		"sworn.eval.observation_duration_ns.numerator":   groupLabels,
		"sworn.eval.observation_duration_ns.denominator": groupLabels,
		"sworn.eval.input_tokens":                        groupLabels,
		"sworn.eval.output_tokens":                       groupLabels,
		"sworn.eval.usage_coverage.numerator":            groupLabels,
		"sworn.eval.usage_coverage.denominator":          groupLabels,
		"sworn.eval.cache_read_tokens":                   groupLabels,
		"sworn.eval.cache_write_tokens":                  groupLabels,
		"sworn.eval.cache_coverage.numerator":            groupLabels,
		"sworn.eval.cache_coverage.denominator":          groupLabels,
		"sworn.eval.turns":                               groupLabels,
		"sworn.eval.tool_calls":                          groupLabels,
		"sworn.eval.tool_calls_per_turn.numerator":       groupLabels,
		"sworn.eval.tool_calls_per_turn.denominator":     groupLabels,
		"sworn.eval.tool_calls.by_name": stringSet(
			"sworn.role",
			"sworn.responsibility",
			"sworn.operation",
			"sworn.transport",
			"sworn.outcome",
			"sworn.profile",
			"sworn.model",
			"sworn.usage_known",
			"sworn.cache_known",
			"sworn.effort_reported",
			"sworn.finish_reason",
			"sworn.truncated",
			"sworn.tool.name",
		),
		"sworn.eval.quality.numerator":   stringSet("sworn.quality"),
		"sworn.eval.quality.denominator": stringSet("sworn.quality"),
	}
	seenNames := make(map[string]bool)
	for _, point := range metrics {
		allowed, found := allowedMetricAttributes[point.name]
		if !found {
			t.Fatalf("unknown metric %q", point.name)
		}
		seenNames[point.name] = true
		assertExactKeys(t, point.attributes, allowed)
		assertResource(t, point.resource)
		assertNoSentinel(t, sentinel, point.attributes)
		assertNoSentinel(t, sentinel, point.resource)
	}
	for name := range allowedMetricAttributes {
		if !seenNames[name] {
			t.Errorf("metric %q was not emitted", name)
		}
	}
	status := telemetry.Status()
	if status.Accepted != 1 || status.Processed != 1 ||
		status.Dropped != 0 || status.TraceExports != 1 ||
		status.MetricExports != 1 || status.Failures != 0 ||
		status.LastSuccessAt == nil || status.LastFailureAt != nil {
		t.Fatalf("status = %#v", status)
	}
}

// A7: when a group reports reasoning, the sworn.eval.reasoning_tokens metric
// is emitted with the group labels and the sworn.reasoning_tokens attribute
// rides the process segment span; a record that reports none emits neither,
// keeping the pinned allowlists byte-valid.
func TestTelemetrySurfacesReasoningTokensWhenReported(t *testing.T) {
	t.Parallel()

	record := testTelemetryRecord("reasoning")
	record.Usage.ReasoningTokens = int64Pointer(1779)
	record.Groups[0].Usage.ReasoningTokens = int64Pointer(1779)

	traceExporter := &captureTraceExporter{started: make(chan struct{})}
	metricExporter := &captureMetricExporter{}
	telemetry, err := newTelemetry(
		traceExporter,
		metricExporter,
		"0.3.0-dev",
		4,
		time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !telemetry.TryEnqueue(record) {
		t.Fatal("record was not accepted")
	}
	select {
	case <-traceExporter.started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start export")
	}
	shutdownTelemetry(t, telemetry)

	spans, _ := traceExporter.snapshot()
	found := false
	for _, span := range spans {
		if span.name != "sworn.process.segment" {
			continue
		}
		if value, present := span.attributes["sworn.reasoning_tokens"]; !present ||
			value != int64(1779) {
			t.Fatalf("segment reasoning attribute = %#v", span.attributes)
		}
		found = true
	}
	if !found {
		t.Fatal("segment span missing reasoning attribute")
	}
	metrics, _ := metricExporter.snapshot()
	found = false
	for _, point := range metrics {
		if point.name == "sworn.eval.reasoning_tokens" {
			if point.value != 1779 {
				t.Fatalf("reasoning metric value = %d", point.value)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("sworn.eval.reasoning_tokens metric was not emitted")
	}
}

func TestTelemetryBackpressureAndExporterFailureAreNonBlocking(
	t *testing.T,
) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	traceExporter := &captureTraceExporter{
		started: started,
		release: release,
	}
	metricExporter := &captureMetricExporter{
		exportErr: errors.New("metric unavailable"),
	}
	telemetry, err := newTelemetry(
		traceExporter,
		metricExporter,
		"0.3.0-dev",
		1,
		time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !telemetry.TryEnqueue(testTelemetryRecord("one")) {
		t.Fatal("first record was not accepted")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start export")
	}
	if !telemetry.TryEnqueue(testTelemetryRecord("two")) {
		t.Fatal("queue did not accept its bounded record")
	}
	began := time.Now()
	if telemetry.TryEnqueue(testTelemetryRecord("three")) {
		t.Fatal("full queue accepted a record")
	}
	if time.Since(began) > 250*time.Millisecond {
		t.Fatal("full queue blocked delivery")
	}
	close(release)
	shutdownTelemetry(t, telemetry)
	status := telemetry.Status()
	if status.Accepted != 2 || status.Processed != 1 ||
		status.Dropped != 2 || status.Failures != 1 ||
		status.LastFailureCode != "metric_export" {
		t.Fatalf("status = %#v", status)
	}
}

func TestTelemetryRejectsMalformedRecordsWithoutPanicking(t *testing.T) {
	t.Parallel()

	traceExporter := &captureTraceExporter{}
	metricExporter := &captureMetricExporter{}
	telemetry, err := newTelemetry(
		traceExporter,
		metricExporter,
		"0.3.0-dev",
		2,
		time.Hour,
	)
	if err != nil {
		t.Fatal(err)
	}
	tests := []Record{
		func() Record {
			value := testTelemetryRecord("bad-state")
			value.RunState = "/private/path"
			return value
		}(),
		func() Record {
			value := testTelemetryRecord("bad-ratio")
			value.Groups[0].DurationNS.Numerator = nil
			return value
		}(),
		func() Record {
			value := testTelemetryRecord("bad-label")
			value.Groups[0].Transport = "provider-error-secret"
			return value
		}(),
		func() Record {
			value := testTelemetryRecord("bad-continuation")
			value.Continuation.Counts[0].Mode =
				"provider-cursor-PRIVATE_SESSION"
			return value
		}(),
	}
	for _, record := range tests {
		if telemetry.TryEnqueue(record) {
			t.Fatalf("malformed record accepted: %#v", record)
		}
	}
	shutdownTelemetry(t, telemetry)
	if status := telemetry.Status(); status.Accepted != 0 ||
		status.Processed != 0 || status.Failures != 0 {
		t.Fatalf("status = %#v", status)
	}
}

func shutdownTelemetry(t *testing.T, telemetry *Telemetry) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := telemetry.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown = %v", err)
	}
}

func stringSet(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func appendKey(base map[string]bool, extra ...string) map[string]bool {
	result := make(map[string]bool, len(base)+len(extra))
	for value := range base {
		result[value] = true
	}
	for _, value := range extra {
		result[value] = true
	}
	return result
}

func assertExactKeys(
	t *testing.T,
	values map[string]any,
	allowed map[string]bool,
) {
	t.Helper()
	if len(values) != len(allowed) {
		t.Fatalf("attribute keys = %#v, want %#v", values, allowed)
	}
	for key := range values {
		if !allowed[key] {
			t.Fatalf("attribute %q is not allowlisted", key)
		}
	}
}

func assertResource(t *testing.T, values map[string]any) {
	t.Helper()
	assertExactKeys(
		t,
		values,
		stringSet("service.name", "service.version"),
	)
	if values["service.name"] != "sworn" ||
		values["service.version"] != "0.3.0-dev" {
		t.Fatalf("resource = %#v", values)
	}
}

func assertNoSentinel(
	t *testing.T,
	sentinel string,
	values map[string]any,
) {
	t.Helper()
	for key, value := range values {
		if strings.Contains(key, sentinel) ||
			strings.Contains(fmt.Sprint(value), sentinel) {
			t.Fatalf("sentinel escaped in %q=%v", key, value)
		}
	}
}
