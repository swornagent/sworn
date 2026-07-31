package observe

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

const (
	TelemetryStatusSchemaVersion = "sworn.telemetry-status/v1"
	telemetryQueueCapacity       = 256
	telemetryExportInterval      = 5 * time.Second
	telemetryExportTimeout       = 3 * time.Second
	// The closed responsibility, operation, transport, outcome, and
	// usage-known product has 1,008 possible group series.
	telemetryMetricCardinality = 1024
)

type Status struct {
	SchemaVersion   string     `json:"schema_version"`
	Enabled         bool       `json:"enabled"`
	QueueCapacity   int        `json:"queue_capacity"`
	QueueDepth      int        `json:"queue_depth"`
	Accepted        uint64     `json:"accepted"`
	Processed       uint64     `json:"processed"`
	Dropped         uint64     `json:"dropped"`
	TraceExports    uint64     `json:"trace_exports"`
	MetricExports   uint64     `json:"metric_exports"`
	Failures        uint64     `json:"failures"`
	LastSuccessAt   *time.Time `json:"last_success_at"`
	LastFailureAt   *time.Time `json:"last_failure_at"`
	LastFailureCode string     `json:"last_failure_code,omitempty"`
}

type Telemetry struct {
	enabled   bool
	available bool
	queue     chan Record

	sendMu sync.Mutex
	closed bool
	stop   chan struct{}
	done   chan struct{}

	accepted      atomic.Uint64
	processed     atomic.Uint64
	dropped       atomic.Uint64
	traceExports  atomic.Uint64
	metricExports atomic.Uint64
	failures      atomic.Uint64

	statusMu        sync.Mutex
	lastSuccessAt   time.Time
	lastFailureAt   time.Time
	lastFailureCode string
}

type telemetryRuntime struct {
	owner          *Telemetry
	traceExporter  telemetryTraceExporter
	metricExporter telemetryMetricExporter
	metrics        metricAccumulator
	resource       telemetryResource
	interval       time.Duration
}

type telemetryTraceExporter interface {
	ExportSpans(context.Context, []telemetrySpan) error
	Shutdown(context.Context) error
}

type telemetryMetricExporter interface {
	Export(context.Context, *telemetryMetricPayload) error
	Shutdown(context.Context) error
}

func Noop() *Telemetry {
	return &Telemetry{}
}

func newTelemetry(
	traceExporter telemetryTraceExporter,
	metricExporter telemetryMetricExporter,
	version string,
	queueCapacity int,
	interval time.Duration,
) (*Telemetry, error) {
	if traceExporter == nil || metricExporter == nil ||
		!validVersion(version) || queueCapacity < 1 || interval <= 0 {
		return nil, fail("INVALID_TELEMETRY")
	}
	owner := &Telemetry{
		enabled:   true,
		available: true,
		queue:     make(chan Record, queueCapacity),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	runtime := &telemetryRuntime{
		owner:          owner,
		traceExporter:  traceExporter,
		metricExporter: metricExporter,
		metrics:        newMetricAccumulator(),
		resource: telemetryResource{
			serviceName:    "sworn",
			serviceVersion: version,
		},
		interval: interval,
	}
	go runtime.run()
	return owner, nil
}

// TryEnqueue copies one already-sanitized local record without waiting. A full
// queue drops telemetry only; it never blocks or reports an error to delivery.
func (t *Telemetry) TryEnqueue(record Record) bool {
	if t == nil || !t.enabled || !validTelemetryRecord(record) {
		return false
	}
	if !t.available {
		t.dropped.Add(1)
		return false
	}
	copy := cloneRecord(record)
	t.sendMu.Lock()
	defer t.sendMu.Unlock()
	if t.closed {
		return false
	}
	select {
	case t.queue <- copy:
		t.accepted.Add(1)
		return true
	default:
		t.dropped.Add(1)
		return false
	}
}

func (t *Telemetry) Status() Status {
	result := Status{
		SchemaVersion: TelemetryStatusSchemaVersion,
		Enabled:       t != nil && t.enabled,
	}
	if t == nil || !t.enabled {
		return result
	}
	result.QueueCapacity = cap(t.queue)
	result.QueueDepth = len(t.queue)
	result.Accepted = t.accepted.Load()
	result.Processed = t.processed.Load()
	result.Dropped = t.dropped.Load()
	result.TraceExports = t.traceExports.Load()
	result.MetricExports = t.metricExports.Load()
	result.Failures = t.failures.Load()
	t.statusMu.Lock()
	defer t.statusMu.Unlock()
	if !t.lastSuccessAt.IsZero() {
		value := t.lastSuccessAt
		result.LastSuccessAt = &value
	}
	if !t.lastFailureAt.IsZero() {
		value := t.lastFailureAt
		result.LastFailureAt = &value
		result.LastFailureCode = t.lastFailureCode
	}
	return result
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t == nil || !t.enabled || !t.available {
		return nil
	}
	if ctx == nil {
		return fail("INVALID_CONTEXT")
	}
	t.sendMu.Lock()
	if !t.closed {
		t.closed = true
		close(t.stop)
	}
	t.sendMu.Unlock()
	select {
	case <-t.done:
		return nil
	case <-ctx.Done():
		return fail("TELEMETRY_SHUTDOWN_TIMEOUT")
	}
}

func (r *telemetryRuntime) run() {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	defer close(r.owner.done)
	dirtyMetrics := false
	for {
		select {
		case <-r.owner.stop:
			r.dropQueued()
			if dirtyMetrics {
				r.exportMetrics()
			}
			r.shutdown()
			return
		default:
		}
		select {
		case <-r.owner.stop:
			r.dropQueued()
			if dirtyMetrics {
				r.exportMetrics()
			}
			r.shutdown()
			return
		case record := <-r.owner.queue:
			r.emitTrace(record)
			r.recordMetrics(record)
			dirtyMetrics = true
			r.owner.processed.Add(1)
		case <-ticker.C:
			if dirtyMetrics {
				r.exportMetrics()
				dirtyMetrics = false
			}
		}
	}
}

func (r *telemetryRuntime) dropQueued() {
	for {
		select {
		case <-r.owner.queue:
			r.owner.dropped.Add(1)
		default:
			return
		}
	}
}

func (r *telemetryRuntime) emitTrace(record Record) {
	spans, err := telemetrySpans(record, r.resource)
	if err != nil {
		r.owner.failure("trace_build")
		return
	}
	exportCtx, cancel := context.WithTimeout(
		context.Background(),
		telemetryExportTimeout,
	)
	err = r.traceExporter.ExportSpans(exportCtx, spans)
	cancel()
	if err != nil {
		r.owner.failure("trace_export")
		return
	}
	r.owner.traceExports.Add(1)
	r.owner.success()
}

func (r *telemetryRuntime) recordMetrics(record Record) {
	observedAt := record.ObservedAt
	outcome := []telemetryAttribute{
		stringTelemetryAttribute("sworn.outcome", record.Outcome),
	}
	r.metrics.record(
		"sworn.eval.events",
		record.Events,
		observedAt,
		outcome,
	)
	for _, count := range record.Continuation.Counts {
		r.metrics.record(
			"sworn.eval.continuations",
			count.Count,
			observedAt,
			[]telemetryAttribute{
				stringTelemetryAttribute(
					"sworn.continuation.mode",
					count.Mode,
				),
				stringTelemetryAttribute(
					"sworn.continuation.outcome",
					count.Outcome,
				),
			},
		)
	}
	for _, continuationOutcome := range []struct {
		outcome string
		value   int64
	}{
		{
			outcome: continuationOutcomeReuse,
			value:   record.Continuation.Reused,
		},
		{
			outcome: continuationOutcomeFallback,
			value:   record.Continuation.Fallback,
		},
		{
			outcome: continuationOutcomeFallbackExpired,
			value:   record.Continuation.Expired,
		},
	} {
		r.metrics.record(
			"sworn.eval.continuation.outcomes",
			continuationOutcome.value,
			observedAt,
			[]telemetryAttribute{
				stringTelemetryAttribute(
					"sworn.continuation.outcome",
					continuationOutcome.outcome,
				),
			},
		)
	}
	for _, recovery := range []struct {
		category string
		value    int64
	}{
		{category: "uncertain", value: record.Recovery.Uncertain},
		{category: "reconciled", value: record.Recovery.Reconciled},
		{category: "recovered", value: record.Recovery.Recovered},
		{category: "rolled_back", value: record.Recovery.RolledBack},
	} {
		r.metrics.record(
			"sworn.eval.recoveries",
			recovery.value,
			observedAt,
			[]telemetryAttribute{
				stringTelemetryAttribute(
					"sworn.recovery",
					recovery.category,
				),
				stringTelemetryAttribute("sworn.outcome", record.Outcome),
			},
		)
	}
	for _, count := range record.TurnRecovery.Actions {
		r.metrics.record(
			"sworn.eval.turn_recovery.actions",
			count.Count,
			observedAt,
			[]telemetryAttribute{
				stringTelemetryAttribute(
					"sworn.turn_recovery.action",
					count.Action,
				),
			},
		)
	}
	for _, outcome := range []struct {
		name  string
		value int64
	}{
		{name: "recovered", value: record.TurnRecovery.Recovered},
		{
			name:  "human_escalation",
			value: record.TurnRecovery.HumanEscalations,
		},
		{
			name:  "false_acceptance",
			value: record.TurnRecovery.FalseAcceptances,
		},
	} {
		r.metrics.record(
			"sworn.eval.turn_recovery.outcomes",
			outcome.value,
			observedAt,
			[]telemetryAttribute{
				stringTelemetryAttribute(
					"sworn.turn_recovery.outcome",
					outcome.name,
				),
			},
		)
	}
	for _, group := range record.Groups {
		usageKnown := "unavailable"
		if group.Usage.InputTokens != nil {
			usageKnown = "reported"
		}
		labels := []telemetryAttribute{
			stringTelemetryAttribute("sworn.role", group.Role),
			stringTelemetryAttribute(
				"sworn.responsibility",
				group.Responsibility,
			),
			stringTelemetryAttribute("sworn.operation", group.Operation),
			stringTelemetryAttribute("sworn.transport", group.Transport),
			stringTelemetryAttribute("sworn.outcome", group.Outcome),
			stringTelemetryAttribute("sworn.usage_known", usageKnown),
		}
		r.metrics.record(
			"sworn.eval.attempts",
			group.Attempts,
			observedAt,
			labels,
		)
		r.metrics.record(
			"sworn.eval.retries",
			group.Retries,
			observedAt,
			labels,
		)
		r.metrics.record(
			"sworn.eval.duration_ns.numerator",
			*group.DurationNS.Numerator,
			observedAt,
			labels,
		)
		r.metrics.record(
			"sworn.eval.duration_ns.denominator",
			*group.DurationNS.Denominator,
			observedAt,
			labels,
		)
		r.metrics.record(
			"sworn.eval.usage_coverage.numerator",
			*group.Usage.TokenCoverage.Numerator,
			observedAt,
			labels,
		)
		r.metrics.record(
			"sworn.eval.usage_coverage.denominator",
			*group.Usage.TokenCoverage.Denominator,
			observedAt,
			labels,
		)
		if group.Usage.InputTokens != nil {
			r.metrics.record(
				"sworn.eval.input_tokens",
				*group.Usage.InputTokens,
				observedAt,
				labels,
			)
			r.metrics.record(
				"sworn.eval.output_tokens",
				*group.Usage.OutputTokens,
				observedAt,
				labels,
			)
		}
	}
	for _, quality := range record.Quality {
		if quality.Numerator == nil {
			continue
		}
		labels := []telemetryAttribute{
			stringTelemetryAttribute("sworn.quality", quality.Name),
		}
		r.metrics.record(
			"sworn.eval.quality.numerator",
			*quality.Numerator,
			observedAt,
			labels,
		)
		r.metrics.record(
			"sworn.eval.quality.denominator",
			*quality.Denominator,
			observedAt,
			labels,
		)
	}
}

func (r *telemetryRuntime) exportMetrics() {
	exportCtx, cancel := context.WithTimeout(
		context.Background(),
		telemetryExportTimeout,
	)
	defer cancel()
	data := r.metrics.payload(r.resource)
	if err := r.metricExporter.Export(exportCtx, &data); err != nil {
		r.owner.failure("metric_export")
		return
	}
	r.owner.metricExports.Add(1)
	r.owner.success()
}

func (r *telemetryRuntime) shutdown() {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		telemetryExportTimeout,
	)
	defer cancel()
	if err := r.traceExporter.Shutdown(ctx); err != nil {
		r.owner.failure("trace_shutdown")
	}
	if err := r.metricExporter.Shutdown(ctx); err != nil {
		r.owner.failure("metric_shutdown")
	}
}

func (t *Telemetry) success() {
	t.statusMu.Lock()
	t.lastSuccessAt = time.Now().UTC()
	t.statusMu.Unlock()
}

func (t *Telemetry) failure(code string) {
	t.failures.Add(1)
	t.statusMu.Lock()
	t.lastFailureAt = time.Now().UTC()
	t.lastFailureCode = code
	t.statusMu.Unlock()
}

func segmentAttributes(record Record) []telemetryAttribute {
	result := []telemetryAttribute{
		stringTelemetryAttribute("sworn.schema", record.SchemaVersion),
		stringTelemetryAttribute("sworn.measurement", "cumulative"),
		stringTelemetryAttribute("sworn.run.state", record.RunState),
		stringTelemetryAttribute("sworn.outcome", record.Outcome),
		int64TelemetryAttribute("sworn.events", record.Events),
		int64TelemetryAttribute("sworn.attempts", record.Attempts),
		int64TelemetryAttribute("sworn.retries", record.Retries),
		int64TelemetryAttribute("sworn.elapsed_ns", record.ElapsedNS),
		boolTelemetryAttribute(
			"sworn.usage_known",
			record.Usage.InputTokens != nil,
		),
	}
	if record.Usage.InputTokens != nil {
		result = append(
			result,
			int64TelemetryAttribute(
				"sworn.input_tokens",
				*record.Usage.InputTokens,
			),
			int64TelemetryAttribute(
				"sworn.output_tokens",
				*record.Usage.OutputTokens,
			),
		)
	}
	return result
}

func recoveryAttributes(value RecoverySummary) []telemetryAttribute {
	return []telemetryAttribute{
		stringTelemetryAttribute("sworn.measurement", "cumulative"),
		int64TelemetryAttribute("sworn.recovery.uncertain", value.Uncertain),
		int64TelemetryAttribute(
			"sworn.recovery.reconciled",
			value.Reconciled,
		),
		int64TelemetryAttribute("sworn.recovery.recovered", value.Recovered),
		int64TelemetryAttribute(
			"sworn.recovery.rolled_back",
			value.RolledBack,
		),
	}
}

func hasRecovery(value RecoverySummary) bool {
	return value.Uncertain != 0 || value.Reconciled != 0 ||
		value.Recovered != 0 || value.RolledBack != 0
}
