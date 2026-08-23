package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/observe"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"

	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

// otlpSink is a fixture OTLP collector capturing protobuf request bodies per
// signal path.
type otlpSink struct {
	server *httptest.Server
	mu     sync.Mutex
	bodies map[string][][]byte
}

func newOTLPSink(t *testing.T) *otlpSink {
	t.Helper()
	sink := &otlpSink{bodies: make(map[string][][]byte)}
	sink.server = httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		defer request.Body.Close()
		body, err := io.ReadAll(io.LimitReader(
			request.Body,
			1<<20+1,
		))
		if err != nil {
			t.Errorf("read OTLP request: %v", err)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		sink.mu.Lock()
		sink.bodies[request.URL.Path] = append(
			sink.bodies[request.URL.Path],
			body,
		)
		sink.mu.Unlock()
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(sink.server.Close)
	return sink
}

func (s *otlpSink) URL() string {
	if s == nil || s.server == nil {
		return ""
	}
	return s.server.URL
}

func (s *otlpSink) count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies[path])
}

func (s *otlpSink) snapshot(path string) [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([][]byte, len(s.bodies[path]))
	copy(result, s.bodies[path])
	return result
}

func (s *otlpSink) waitFor(t *testing.T, path string, minimum int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s.count(path) >= minimum {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s requests = %d, want >= %d", path, s.count(path), minimum)
}

// runTelemetryFixture builds one self-contained project: a git repository
// with an installed approved plan, a scripted manifest whose planner attempt
// takes no action, and the journal path inside the repository (so the F5
// journal-root operator config rule resolves).
func runTelemetryFixture(
	t *testing.T,
	runID string,
) (manifestPath, journalPath, root string) {
	t.Helper()
	root, _ = projectRepositoryFixture(t)
	installProjectPlan(t, root, "delivery")
	manifestDir := filepath.Join(root, ".sworn", "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projectWriteManifest(
		t,
		manifestDir,
		"delivery.json",
		root,
		"delivery",
		runID,
	)
	manifestPath = filepath.Join(manifestDir, "delivery.json")
	journalPath = filepath.Join(root, ".sworn", "run.sqlite")
	return manifestPath, journalPath, root
}

// writeRunOperatorConfig writes a private 0600 operator config at the
// project's canonical location with the given channels.
func writeRunOperatorConfig(
	t *testing.T,
	root string,
	otelEndpoint string,
	share *observe.ShareConfig,
) string {
	t.Helper()
	config := operatorConfig{
		SchemaVersion: operatorConfigSchemaVersion,
		Local:         operatorLocalConfig{Listen: defaultOperatorListen},
	}
	if otelEndpoint != "" {
		config.OTel = &observe.Config{
			SchemaVersion: observe.OTelConfigSchemaVersion,
			Endpoint:      otelEndpoint,
			Headers:       map[string]string{},
		}
	}
	config.Share = share
	body, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".sworn", "operator.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func runCLI(
	t *testing.T,
	args ...string,
) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func evalRecordCount(t *testing.T, journalPath, runID string) int {
	t.Helper()
	ctx := context.Background()
	store, err := journal.OpenReadOnly(ctx, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	records, err := store.EvalRecords(
		ctx,
		runID,
		observe.EvalObserver,
		0,
		100,
	)
	if err != nil {
		t.Fatal(err)
	}
	return len(records)
}

func runStatusFacts(t *testing.T, journalPath, runID string) runtimepkg.RunStatus {
	t.Helper()
	reader, err := runtimepkg.OpenStatusService(
		context.Background(),
		journalPath,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	status, err := reader.Status(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

// A1: a plain sworn run with a configured private channel exports spans and
// metrics with no serve process running, and the run process persisted its
// own eval records.
func TestRunExportsTelemetryWithoutServe(t *testing.T) {
	sink := newOTLPSink(t)
	manifestPath, journalPath, root := runTelemetryFixture(t, "run-otel")
	writeRunOperatorConfig(t, root, sink.URL(), nil)

	code, stdout, stderr := runCLI(t,
		"run", "--manifest", manifestPath, "--journal", journalPath)
	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Sworn run run-otel") {
		t.Fatalf("run stdout missing status: %q", stdout)
	}
	sink.waitFor(t, "/v1/traces", 1)
	sink.waitFor(t, "/v1/metrics", 1)
	if evalRecordCount(t, journalPath, "run-otel") == 0 {
		t.Fatal("run process persisted no eval.core records")
	}

	for _, body := range sink.snapshot("/v1/traces") {
		var request collectortracepb.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode trace request: %v", err)
		}
		found := false
		for _, resourceSpans := range request.ResourceSpans {
			for _, scope := range resourceSpans.ScopeSpans {
				for _, span := range scope.Spans {
					if span.Name == "sworn.process.segment" {
						found = true
					}
				}
			}
		}
		if !found {
			t.Fatal("private channel carried no segment span")
		}
	}
}

// A1 companion: the drive-hosting control verbs export the delivery they
// host through Service.Wait exactly as run does.
func TestControlVerbsExportTheDeliveryTheyHost(t *testing.T) {
	t.Run("resume", func(t *testing.T) {
		sink := newOTLPSink(t)
		manifestPath, journalPath, root := runTelemetryFixture(t, "run-resume-otel")
		writeRunOperatorConfig(t, root, sink.URL(), nil)
		if code, _, stderr := runCLI(t,
			"run", "--manifest", manifestPath, "--journal", journalPath); code != 0 {
			t.Fatalf("run exit = %d, stderr = %q", code, stderr)
		}
		sink.waitFor(t, "/v1/traces", 1)
		before := sink.count("/v1/traces")
		code, stdout, stderr := runCLI(t,
			"resume", "--run", "run-resume-otel", "--journal", journalPath,
			"--command", "resume-1", "--generation", "0")
		if code != 0 {
			t.Fatalf("resume exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
		sink.waitFor(t, "/v1/traces", before+1)
	})

	t.Run("takeover", func(t *testing.T) {
		sink := newOTLPSink(t)
		manifestPath, journalPath, root := runTelemetryFixture(t, "run-takeover-otel")
		writeRunOperatorConfig(t, root, sink.URL(), nil)
		if code, _, stderr := runCLI(t,
			"run", "--manifest", manifestPath, "--journal", journalPath); code != 0 {
			t.Fatalf("run exit = %d, stderr = %q", code, stderr)
		}
		sink.waitFor(t, "/v1/traces", 1)
		before := sink.count("/v1/traces")
		store, err := journal.Open(context.Background(), journalPath)
		if err != nil {
			t.Fatal(err)
		}
		expired := time.Now().UTC().Add(-10 * time.Second)
		if _, err := store.AcquireOwner(
			context.Background(),
			"run-takeover-otel",
			expired,
			2*time.Second,
			false,
		); err != nil {
			_ = store.Close()
			t.Fatal(err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runCLI(t,
			"takeover", "--run", "run-takeover-otel", "--journal", journalPath,
			"--command", "takeover-1", "--generation", "0")
		if code != 0 {
			t.Fatalf("takeover exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
		}
		sink.waitFor(t, "/v1/traces", before+1)
	})

}

// A2/A3: both channels configure independently and the share channel carries
// only allowlisted names and attributes with the schema stamp, while the
// private channel stays unfiltered.
func TestRunExportsBothChannelsWithShareFiltered(t *testing.T) {
	privateSink := newOTLPSink(t)
	shareSink := newOTLPSink(t)
	manifestPath, journalPath, root := runTelemetryFixture(t, "run-both-channels")
	writeRunOperatorConfig(t, root, privateSink.URL(), &observe.ShareConfig{
		SchemaVersion: observe.ShareConfigSchemaVersion,
		Enabled:       true,
		Endpoint:      shareSink.URL(),
		Headers:       map[string]string{},
	})

	code, _, stderr := runCLI(t,
		"run", "--manifest", manifestPath, "--journal", journalPath)
	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %q", code, stderr)
	}
	privateSink.waitFor(t, "/v1/traces", 1)
	privateSink.waitFor(t, "/v1/metrics", 1)
	shareSink.waitFor(t, "/v1/traces", 1)
	shareSink.waitFor(t, "/v1/metrics", 1)

	for _, body := range shareSink.snapshot("/v1/traces") {
		var request collectortracepb.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode share trace request: %v", err)
		}
		for _, resourceSpans := range request.ResourceSpans {
			for _, scope := range resourceSpans.ScopeSpans {
				for _, span := range scope.Spans {
					if span.Name != "sworn.process.segment" &&
						span.Name != "sworn.recovery" {
						t.Fatalf("non-allowlisted share span: %q", span.Name)
					}
					keys := make([]string, 0, len(span.Attributes))
					schemaStamped := false
					for _, attribute := range span.Attributes {
						keys = append(keys, attribute.Key)
						if attribute.Key == observe.ShareSchemaAttribute &&
							attribute.Value.GetStringValue() ==
								observe.ShareSchemaVersion {
							schemaStamped = true
						}
					}
					if !schemaStamped {
						t.Fatalf("share span lacks schema stamp: %v", keys)
					}
					for _, key := range keys {
						if key == observe.ShareSchemaAttribute {
							continue
						}
						if key == "sworn.schema" ||
							key == "sworn.run.state" ||
							key == "sworn.outcome" ||
							key == "sworn.events" ||
							key == "sworn.attempts" ||
							key == "sworn.retries" ||
							key == "sworn.elapsed_ns" ||
							key == "sworn.usage_known" ||
							key == "sworn.measurement" ||
							strings.HasPrefix(key, "sworn.recovery.") {
							continue
						}
						t.Fatalf("non-allowlisted share attribute: %q", key)
					}
				}
			}
		}
	}
	for _, body := range shareSink.snapshot("/v1/metrics") {
		var request collectormetricspb.ExportMetricsServiceRequest
		if err := proto.Unmarshal(body, &request); err != nil {
			t.Fatalf("decode share metric request: %v", err)
		}
		for _, resourceMetrics := range request.ResourceMetrics {
			for _, scope := range resourceMetrics.ScopeMetrics {
				for _, metric := range scope.Metrics {
					if !strings.HasPrefix(metric.Name, "sworn.eval.") {
						t.Fatalf("non-allowlisted share metric: %q", metric.Name)
					}
				}
			}
		}
	}
	if privateSink.count("/v1/traces") == 0 {
		t.Fatal("private channel received no trace exports")
	}
}

// A4: telemetry is incapable of affecting delivery. Four identical runs -
// baseline without any collector, one with a 503-ing collector, one with an
// unreachable loopback endpoint, and one with a blocking hostile collector -
// must leave byte-identical status output, identical run facts, and the same
// exit code. The telemetry pipeline's own eval.core rows exist only in the
// configured modes, and the hostile collector costs only a bounded wait.
func TestTelemetryCannotAffectDelivery(t *testing.T) {
	blockingRelease := make(chan struct{})
	blocking := httptest.NewServer(http.HandlerFunc(func(
		http.ResponseWriter,
		*http.Request,
	) {
		<-blockingRelease
	}))
	defer func() {
		close(blockingRelease)
		blocking.Close()
	}()
	unavailable := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer unavailable.Close()

	type mode struct {
		name     string
		endpoint string
	}
	modes := []mode{
		{name: "baseline", endpoint: ""},
		{name: "unavailable", endpoint: unavailable.URL},
		{name: "unreachable", endpoint: "http://127.0.0.1:1"},
		{name: "blocking", endpoint: blocking.URL},
	}

	type outcome struct {
		mode    string
		stdout  string
		code    int
		state   string
		final   string
		evals   int
		elapsed time.Duration
	}
	var outcomes []outcome
	for _, mode := range modes {
		manifestPath, journalPath, root := runTelemetryFixture(t, "run-parity")
		if mode.endpoint != "" {
			writeRunOperatorConfig(t, root, mode.endpoint, nil)
		}
		started := time.Now()
		code, stdout, stderr := runCLI(t,
			"run", "--manifest", manifestPath, "--journal", journalPath)
		outcomes = append(outcomes, outcome{
			mode:    mode.name,
			stdout:  stdout,
			code:    code,
			state:   runStatusFacts(t, journalPath, "run-parity").State,
			final:   runStatusFacts(t, journalPath, "run-parity").Outcome,
			evals:   evalRecordCount(t, journalPath, "run-parity"),
			elapsed: time.Since(started),
		})
		if stderr != "" {
			t.Fatalf("%s stderr = %q", mode.name, stderr)
		}
	}

	baseline := outcomes[0]
	for _, outcome := range outcomes[1:] {
		if outcome.stdout != baseline.stdout {
			t.Fatalf(
				"%s stdout differs from baseline:\nbaseline:\n%s\n%s:\n%s",
				outcome.mode,
				baseline.stdout,
				outcome.mode,
				outcome.stdout,
			)
		}
		if outcome.code != baseline.code ||
			outcome.state != baseline.state ||
			outcome.final != baseline.final {
			t.Fatalf("%s run facts differ: %#v vs %#v",
				outcome.mode, outcome, baseline)
		}
		if outcome.evals == 0 {
			t.Fatalf("%s persisted no eval.core rows", outcome.mode)
		}
	}
	if baseline.evals != 0 {
		t.Fatalf("baseline persisted %d eval.core rows", baseline.evals)
	}
	blockingOutcome := outcomes[3]
	if blockingOutcome.elapsed > 30*time.Second {
		t.Fatalf("hostile collector was not bounded: %s", blockingOutcome.elapsed)
	}
}

// A5/F3: record-level no-double-emit through the shared eval.core observer
// cursor. A second host (the identical evaluator+cursor machinery serve
// uses) on the same journal after the run completed emits zero additional
// trace batches for records the run already exported.
func TestRunSideHostDoesNotDoubleEmitRecords(t *testing.T) {
	sink := newOTLPSink(t)
	manifestPath, journalPath, root := runTelemetryFixture(t, "run-no-double")
	operatorConfig := writeRunOperatorConfig(t, root, sink.URL(), nil)
	if code, _, stderr := runCLI(t,
		"run", "--manifest", manifestPath, "--journal", journalPath); code != 0 {
		t.Fatalf("run exit = %d, stderr = %q", code, stderr)
	}
	sink.waitFor(t, "/v1/traces", 1)
	beforeTraces := sink.count("/v1/traces")
	beforeMetrics := sink.count("/v1/metrics")

	ctx := context.Background()
	service, factory, err := openRuntimeService(ctx, journalPath, "")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if factory != nil {
		defer factory.Close()
	}
	host := newRunTelemetryHost(
		ctx,
		journalPath,
		"run-no-double",
		service,
		operatorConfig,
	)
	if host == nil {
		t.Fatal("second host was not built")
	}
	time.Sleep(1500 * time.Millisecond)
	host.finish()

	if got := sink.count("/v1/traces"); got != beforeTraces {
		t.Fatalf("second host emitted %d extra trace batches (want 0)", got-beforeTraces)
	}
	if got := sink.count("/v1/metrics"); got != beforeMetrics {
		t.Fatalf("second host emitted %d extra metric batches (want 0)", got-beforeMetrics)
	}
}

// C3: sworn tui hosts delivery in-process through StartDetached, so a run
// started from the TUI exports through the resident host with teardown bound
// to the backend's close path.
func TestTUIStartedRunExportsThroughResidentHost(t *testing.T) {
	sink := newOTLPSink(t)
	manifestPath, _, root := runTelemetryFixture(t, "run-tui-otel")
	writeRunOperatorConfig(t, root, sink.URL(), nil)

	ctx := context.Background()
	project, err := discoverProject(ctx, root, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	var release projectRelease
	for _, candidate := range project.releases {
		if candidate.name == "delivery" {
			release = candidate
		}
	}
	if release.name == "" {
		t.Fatal("fixture release not discovered")
	}
	selection, err := newTUISelection(project, release, "")
	if err != nil {
		t.Fatal(err)
	}
	backend := newProjectTUIBackend(root, "", "", "")
	if err := backend.startTUIRun(
		ctx,
		project,
		release,
		selection,
		manifestPath,
		project.paths.journal,
		existingRegularFile(project.paths.config),
	); err != nil {
		t.Fatalf("startTUIRun: %v", err)
	}
	sink.waitFor(t, "/v1/traces", 1)
	sink.waitFor(t, "/v1/metrics", 1)
	if err := backend.Close(); err != nil {
		t.Fatalf("backend.Close: %v", err)
	}
	if evalRecordCount(t, project.paths.journal, "run-tui-otel") == 0 {
		t.Fatal("TUI-hosted run persisted no eval.core records")
	}
}

// The F5 resolution rule: the run-side host derives the operator config from
// the journal's own git root, so a journal outside any admitted git project
// yields silent no-telemetry even when a sink would be reachable.
func TestRunSideHostSilentlySkipsJournalOutsideGitProject(t *testing.T) {
	sink := newOTLPSink(t)
	root, _ := projectRepositoryFixture(t)
	installProjectPlan(t, root, "delivery")
	manifestDir := filepath.Join(root, ".sworn", "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projectWriteManifest(t, manifestDir, "delivery.json", root, "delivery", "run-outside-git")
	manifestPath := filepath.Join(manifestDir, "delivery.json")

	// The journal lives in a plain temp directory outside any repository.
	journalPath := filepath.Join(t.TempDir(), "run.sqlite")
	code, stdout, stderr := runCLI(t,
		"run", "--manifest", manifestPath, "--journal", journalPath)
	if code != 0 {
		t.Fatalf("run exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	time.Sleep(1500 * time.Millisecond)
	if sink.count("/v1/traces") != 0 || sink.count("/v1/metrics") != 0 {
		t.Fatal("journal outside a git project exported telemetry")
	}
	if evalRecordCount(t, journalPath, "run-outside-git") != 0 {
		t.Fatal("journal outside a git project gained eval.core rows")
	}
}

// C6 adjudication: serve keeps exporting as a view through both configured
// channels, built by the same helper the run-side host uses, and the run it
// hosts exports into each.
func TestServeExportsBothChannelsInRunMode(t *testing.T) {
	privateSink := newOTLPSink(t)
	shareSink := newOTLPSink(t)
	body := operatorManifestBody(t, "run-serve-share", "serve hosts share too")
	manifest, err := cockpit.AdmitManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(manifestPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listenAddress := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	config := operatorConfig{
		SchemaVersion: operatorConfigSchemaVersion,
		Local:         operatorLocalConfig{Listen: listenAddress},
		OTel: &observe.Config{
			SchemaVersion: observe.OTelConfigSchemaVersion,
			Endpoint:      privateSink.URL(),
			Headers:       map[string]string{},
		},
		Share: &observe.ShareConfig{
			SchemaVersion: observe.ShareConfigSchemaVersion,
			Enabled:       true,
			Endpoint:      shareSink.URL(),
			Headers:       map[string]string{},
		},
	}
	configBody, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "operator.json")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(t.TempDir(), "run.sqlite")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout synchronizedBuffer
	result := make(chan error, 1)
	go func() {
		result <- serveOperator(ctx, serveOptions{
			runID:          "run-serve-share",
			journalPath:    journalPath,
			manifestPath:   manifestPath,
			operatorConfig: configPath,
		}, &stdout)
	}()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(stdout.String(), "sworn serve: ready\n") &&
		time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(stdout.String(), "sworn serve: ready\n") {
		cancel()
		t.Fatalf("serve did not become ready; output %q", stdout.String())
	}

	requestBody, err := json.Marshal(cockpit.StartCommand{
		ManifestDigest: manifest.Digest(),
	})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	request, err := http.NewRequest(
		http.MethodPost,
		"http://"+listenAddress+"/api/v2/start",
		bytes.NewReader(requestBody),
	)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
		Timeout:   5 * time.Second,
	}
	response, err := client.Do(request)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	_ = response.Body.Close()

	privateSink.waitFor(t, "/v1/traces", 1)
	privateSink.waitFor(t, "/v1/metrics", 1)
	shareSink.waitFor(t, "/v1/traces", 1)
	shareSink.waitFor(t, "/v1/metrics", 1)

	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("serve shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop after cancellation")
	}
}

// C6 unit pin: the shared channel-construction helper yields both configured
// channels, keeps the private channel's error return, and degrades a share
// construction failure to a disabled channel instead of an error.
func TestBuildTelemetryChannelsSharedHelper(t *testing.T) {
	t.Parallel()

	settings := operatorSettings{
		otel: &observe.Config{
			SchemaVersion: observe.OTelConfigSchemaVersion,
			Endpoint:      "http://127.0.0.1:1",
		},
		share: &observe.ShareConfig{
			SchemaVersion: observe.ShareConfigSchemaVersion,
			Enabled:       true,
			Endpoint:      "http://127.0.0.1:1",
		},
	}
	private, share, err := buildTelemetryChannels(
		context.Background(),
		settings,
	)
	if err != nil || !private.Status().Enabled ||
		!share.Status().Enabled {
		t.Fatalf("channels = %v %v, %v",
			private.Status(), share.Status(), err)
	}

	// Disabled share block leaves the share channel off.
	settings.share = &observe.ShareConfig{
		SchemaVersion: observe.ShareConfigSchemaVersion,
		Enabled:       false,
	}
	_, share, err = buildTelemetryChannels(context.Background(), settings)
	if err != nil || share.Status().Enabled {
		t.Fatalf("disabled share = %v, %v", share.Status(), err)
	}

	// A private-channel construction failure is surfaced to the caller.
	settings.otel = &observe.Config{
		SchemaVersion: observe.OTelConfigSchemaVersion,
		Endpoint:      "http://127.0.0.1:1",
		Headers: map[string]string{
			"Authorization": strings.Repeat("x", 2000),
		},
	}
	settings.share = &observe.ShareConfig{
		SchemaVersion: observe.ShareConfigSchemaVersion,
		Enabled:       true,
		Endpoint:      "http://127.0.0.1:1",
	}
	if _, _, err := buildTelemetryChannels(
		context.Background(),
		settings,
	); err == nil {
		t.Fatal("private-channel construction failure was swallowed")
	}
}
