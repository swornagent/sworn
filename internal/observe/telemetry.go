package observe

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
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
	enabled bool
	queue   chan Record

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
	traceProvider  *sdktrace.TracerProvider
	spanProcessor  *recordSpanProcessor
	traceExporter  sdktrace.SpanExporter
	meterProvider  *sdkmetric.MeterProvider
	metricReader   *sdkmetric.ManualReader
	metricExporter sdkmetric.Exporter
	metrics        metricSet
	resource       *resource.Resource
	interval       time.Duration
}

type recordSpanProcessor struct {
	mu       sync.Mutex
	shutdown bool
	spans    []sdktrace.ReadOnlySpan
}

func (p *recordSpanProcessor) OnStart(context.Context, sdktrace.ReadWriteSpan) {}

func (p *recordSpanProcessor) OnEnd(span sdktrace.ReadOnlySpan) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.shutdown {
		p.spans = append(p.spans, span)
	}
}

func (p *recordSpanProcessor) ForceFlush(context.Context) error { return nil }

func (p *recordSpanProcessor) Shutdown(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.shutdown = true
	return nil
}

func (p *recordSpanProcessor) drain() []sdktrace.ReadOnlySpan {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := append([]sdktrace.ReadOnlySpan(nil), p.spans...)
	p.spans = p.spans[:0]
	return result
}

type metricSet struct {
	events              metric.Int64Gauge
	attempts            metric.Int64Gauge
	retries             metric.Int64Gauge
	recoveries          metric.Int64Gauge
	durationNumerator   metric.Int64Gauge
	durationDenominator metric.Int64Gauge
	inputTokens         metric.Int64Gauge
	outputTokens        metric.Int64Gauge
	usageNumerator      metric.Int64Gauge
	usageDenominator    metric.Int64Gauge
	qualityNumerator    metric.Int64Gauge
	qualityDenominator  metric.Int64Gauge
}

func Noop() *Telemetry {
	return &Telemetry{}
}

func newTelemetry(
	traceExporter sdktrace.SpanExporter,
	metricExporter sdkmetric.Exporter,
	version string,
	queueCapacity int,
	interval time.Duration,
) (*Telemetry, error) {
	if traceExporter == nil || metricExporter == nil ||
		!validVersion(version) || queueCapacity < 1 || interval <= 0 {
		return nil, fail("INVALID_TELEMETRY")
	}
	owner := &Telemetry{
		enabled: true,
		queue:   make(chan Record, queueCapacity),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	service := resource.NewSchemaless(
		attribute.String("service.name", "sworn"),
		attribute.String("service.version", version),
	)
	spanProcessor := &recordSpanProcessor{}
	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(service),
		sdktrace.WithSpanProcessor(spanProcessor),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithRawSpanLimits(sdktrace.SpanLimits{
			AttributeValueLengthLimit:   128,
			AttributeCountLimit:         16,
			EventCountLimit:             0,
			LinkCountLimit:              0,
			AttributePerEventCountLimit: 0,
			AttributePerLinkCountLimit:  0,
		}),
	)
	reader := sdkmetric.NewManualReader(
		sdkmetric.WithTemporalitySelector(metricExporter.Temporality),
		sdkmetric.WithAggregationSelector(metricExporter.Aggregation),
	)
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(service),
		sdkmetric.WithReader(reader),
		sdkmetric.WithCardinalityLimit(telemetryMetricCardinality),
		sdkmetric.WithExemplarFilter(exemplar.AlwaysOffFilter),
	)
	metrics, err := newMetricSet(meterProvider.Meter("sworn.observe"))
	if err != nil {
		_ = traceProvider.Shutdown(context.Background())
		_ = traceExporter.Shutdown(context.Background())
		_ = meterProvider.Shutdown(context.Background())
		_ = metricExporter.Shutdown(context.Background())
		return nil, err
	}
	runtime := &telemetryRuntime{
		owner:          owner,
		traceProvider:  traceProvider,
		spanProcessor:  spanProcessor,
		traceExporter:  traceExporter,
		meterProvider:  meterProvider,
		metricReader:   reader,
		metricExporter: metricExporter,
		metrics:        metrics,
		resource:       service,
		interval:       interval,
	}
	go runtime.run()
	return owner, nil
}

func newMetricSet(meter metric.Meter) (metricSet, error) {
	names := []string{
		"sworn.eval.events",
		"sworn.eval.attempts",
		"sworn.eval.retries",
		"sworn.eval.recoveries",
		"sworn.eval.duration_ns.numerator",
		"sworn.eval.duration_ns.denominator",
		"sworn.eval.input_tokens",
		"sworn.eval.output_tokens",
		"sworn.eval.usage_coverage.numerator",
		"sworn.eval.usage_coverage.denominator",
		"sworn.eval.quality.numerator",
		"sworn.eval.quality.denominator",
	}
	instruments := make([]metric.Int64Gauge, 0, len(names))
	for _, name := range names {
		instrument, err := meter.Int64Gauge(name)
		if err != nil {
			return metricSet{}, fail("OTEL_INSTRUMENT_FAILED")
		}
		instruments = append(instruments, instrument)
	}
	return metricSet{
		events:              instruments[0],
		attempts:            instruments[1],
		retries:             instruments[2],
		recoveries:          instruments[3],
		durationNumerator:   instruments[4],
		durationDenominator: instruments[5],
		inputTokens:         instruments[6],
		outputTokens:        instruments[7],
		usageNumerator:      instruments[8],
		usageDenominator:    instruments[9],
		qualityNumerator:    instruments[10],
		qualityDenominator:  instruments[11],
	}, nil
}

// TryEnqueue copies one already-sanitized local record without waiting. A full
// queue drops telemetry only; it never blocks or reports an error to delivery.
func (t *Telemetry) TryEnqueue(record Record) bool {
	if t == nil || !t.enabled || !validTelemetryRecord(record) {
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
	if t == nil || !t.enabled {
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
	tracer := r.traceProvider.Tracer("sworn.observe")
	ctx, segment := tracer.Start(
		context.Background(),
		"sworn.process.segment",
		trace.WithNewRoot(),
		trace.WithTimestamp(record.StartedAt),
		trace.WithAttributes(segmentAttributes(record)...),
	)
	if hasRecovery(record.Recovery) {
		_, span := tracer.Start(
			ctx,
			"sworn.recovery",
			trace.WithTimestamp(record.ObservedAt),
			trace.WithAttributes(recoveryAttributes(record.Recovery)...),
		)
		span.End(trace.WithTimestamp(record.ObservedAt))
	}
	segment.End(trace.WithTimestamp(record.ObservedAt))
	spans := r.spanProcessor.drain()
	exportCtx, cancel := context.WithTimeout(
		context.Background(),
		telemetryExportTimeout,
	)
	err := r.traceExporter.ExportSpans(
		exportCtx,
		withTelemetryResource(spans, r.resource),
	)
	cancel()
	if err != nil {
		r.owner.failure("trace_export")
		return
	}
	r.owner.traceExports.Add(1)
	r.owner.success()
}

func (r *telemetryRuntime) recordMetrics(record Record) {
	ctx := context.Background()
	outcome := metric.WithAttributes(
		attribute.String("sworn.outcome", record.Outcome),
	)
	r.metrics.events.Record(ctx, record.Events, outcome)
	for category, value := range map[string]int64{
		"uncertain":   record.Recovery.Uncertain,
		"reconciled":  record.Recovery.Reconciled,
		"recovered":   record.Recovery.Recovered,
		"rolled_back": record.Recovery.RolledBack,
	} {
		r.metrics.recoveries.Record(
			ctx,
			value,
			metric.WithAttributes(
				attribute.String("sworn.recovery", category),
				attribute.String("sworn.outcome", record.Outcome),
			),
		)
	}
	for _, group := range record.Groups {
		usageKnown := "unavailable"
		if group.Usage.InputTokens != nil {
			usageKnown = "reported"
		}
		labels := metric.WithAttributes(
			attribute.String("sworn.role", group.Role),
			attribute.String("sworn.responsibility", group.Responsibility),
			attribute.String("sworn.operation", group.Operation),
			attribute.String("sworn.transport", group.Transport),
			attribute.String("sworn.outcome", group.Outcome),
			attribute.String("sworn.usage_known", usageKnown),
		)
		r.metrics.attempts.Record(ctx, group.Attempts, labels)
		r.metrics.retries.Record(ctx, group.Retries, labels)
		r.metrics.durationNumerator.Record(
			ctx,
			*group.DurationNS.Numerator,
			labels,
		)
		r.metrics.durationDenominator.Record(
			ctx,
			*group.DurationNS.Denominator,
			labels,
		)
		r.metrics.usageNumerator.Record(
			ctx,
			*group.Usage.TokenCoverage.Numerator,
			labels,
		)
		r.metrics.usageDenominator.Record(
			ctx,
			*group.Usage.TokenCoverage.Denominator,
			labels,
		)
		if group.Usage.InputTokens != nil {
			r.metrics.inputTokens.Record(ctx, *group.Usage.InputTokens, labels)
			r.metrics.outputTokens.Record(ctx, *group.Usage.OutputTokens, labels)
		}
	}
	for _, quality := range record.Quality {
		if quality.Numerator == nil {
			continue
		}
		label := metric.WithAttributes(
			attribute.String("sworn.quality", quality.Name),
		)
		r.metrics.qualityNumerator.Record(ctx, *quality.Numerator, label)
		r.metrics.qualityDenominator.Record(ctx, *quality.Denominator, label)
	}
}

func (r *telemetryRuntime) exportMetrics() {
	exportCtx, cancel := context.WithTimeout(
		context.Background(),
		telemetryExportTimeout,
	)
	defer cancel()
	var data metricdata.ResourceMetrics
	if err := r.metricReader.Collect(exportCtx, &data); err != nil {
		r.owner.failure("metric_collect")
		return
	}
	// OTel Go merges resource.Environment into SDK providers by design.
	// Replace the final export resource so ambient values cannot escape.
	data.Resource = r.resource
	if err := r.metricExporter.Export(exportCtx, &data); err != nil {
		r.owner.failure("metric_export")
		return
	}
	r.owner.metricExports.Add(1)
	r.owner.success()
}

type telemetryResourceSpan struct {
	sdktrace.ReadOnlySpan
	value *resource.Resource
}

func (s telemetryResourceSpan) Resource() *resource.Resource {
	return s.value
}

func withTelemetryResource(
	spans []sdktrace.ReadOnlySpan,
	value *resource.Resource,
) []sdktrace.ReadOnlySpan {
	// See exportMetrics: the wrapper preserves the SDK span while replacing
	// the environment-merged resource at the final exporter boundary.
	result := make([]sdktrace.ReadOnlySpan, len(spans))
	for index, span := range spans {
		result[index] = telemetryResourceSpan{
			ReadOnlySpan: span,
			value:        value,
		}
	}
	return result
}

func (r *telemetryRuntime) shutdown() {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		telemetryExportTimeout,
	)
	defer cancel()
	if err := r.traceProvider.Shutdown(ctx); err != nil {
		r.owner.failure("trace_shutdown")
	}
	if err := r.traceExporter.Shutdown(ctx); err != nil {
		r.owner.failure("trace_shutdown")
	}
	if err := r.meterProvider.Shutdown(ctx); err != nil {
		r.owner.failure("metric_shutdown")
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

func segmentAttributes(record Record) []attribute.KeyValue {
	result := []attribute.KeyValue{
		attribute.String("sworn.schema", record.SchemaVersion),
		attribute.String("sworn.measurement", "cumulative"),
		attribute.String("sworn.run.state", record.RunState),
		attribute.String("sworn.outcome", record.Outcome),
		attribute.Int64("sworn.events", record.Events),
		attribute.Int64("sworn.attempts", record.Attempts),
		attribute.Int64("sworn.retries", record.Retries),
		attribute.Int64("sworn.elapsed_ns", record.ElapsedNS),
		attribute.Bool("sworn.usage_known", record.Usage.InputTokens != nil),
	}
	if record.Usage.InputTokens != nil {
		result = append(
			result,
			attribute.Int64("sworn.input_tokens", *record.Usage.InputTokens),
			attribute.Int64("sworn.output_tokens", *record.Usage.OutputTokens),
		)
	}
	return result
}

func recoveryAttributes(value RecoverySummary) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("sworn.measurement", "cumulative"),
		attribute.Int64("sworn.recovery.uncertain", value.Uncertain),
		attribute.Int64("sworn.recovery.reconciled", value.Reconciled),
		attribute.Int64("sworn.recovery.recovered", value.Recovered),
		attribute.Int64("sworn.recovery.rolled_back", value.RolledBack),
	}
}

func hasRecovery(value RecoverySummary) bool {
	return value.Uncertain != 0 || value.Reconciled != 0 ||
		value.Recovered != 0 || value.RolledBack != 0
}
