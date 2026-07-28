package cockpit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

const (
	testLocalHost   = "127.0.0.1:8090"
	testLocalOrigin = "http://" + testLocalHost
	testPublicHost  = "sworn.example"
	testPublicURL   = "https://" + testPublicHost
	testHTTPToken   = "0123456789abcdefghijklmnopqrstuvwxyzABCD"
)

type httpFakeProjector struct {
	mu         sync.Mutex
	snapshot   Snapshot
	events     EventPage
	eventsErr  error
	eventAfter int64
	eventLimit int
	onEvents   func()
}

func (f *httpFakeProjector) Snapshot(
	context.Context,
	string,
) (Snapshot, error) {
	return f.snapshot, nil
}

func (f *httpFakeProjector) Events(
	_ context.Context,
	_ string,
	after int64,
	limit int,
) (EventPage, error) {
	f.mu.Lock()
	f.eventAfter = after
	f.eventLimit = limit
	onEvents := f.onEvents
	result := f.events
	err := f.eventsErr
	f.mu.Unlock()
	if onEvents != nil {
		onEvents()
	}
	return result, err
}

type httpFakeCommands struct {
	mu           sync.Mutex
	startCalls   int
	controlCalls int
	redeliveries int
}

type httpFakeTelemetry struct {
	status TelemetryHealth
}

func (f httpFakeTelemetry) TelemetryHealth() TelemetryHealth {
	return f.status
}

func (f *httpFakeCommands) Start(
	context.Context,
	StartCommand,
) (runtimepkg.RunStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalls++
	return runtimepkg.RunStatus{RunID: "run-1"}, nil
}

func (f *httpFakeCommands) Control(
	context.Context,
	ControlCommand,
) (runtimepkg.RunStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.controlCalls++
	return runtimepkg.RunStatus{RunID: "run-1"}, nil
}

func (f *httpFakeCommands) Redeliver(
	context.Context,
	RedeliveryCommand,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.redeliveries++
	return nil
}

func httpSnapshot() Snapshot {
	return Snapshot{
		SchemaVersion: SnapshotSchemaVersion,
		Run: RunView{
			ID: "run-1", Release: "release-1", State: "running",
		},
		Graph: Graph{
			Nodes: []Node{{
				ID: "release:release-1", Kind: "release",
				Label: "release-1", State: "running",
			}},
			Edges: []Edge{},
		},
		Handoff: Handoff{
			Nodes: []string{}, Responsibilities: []string{},
		},
		Runtime: RuntimeView{
			Effects:       []EffectView{},
			Attempts:      []AttemptView{},
			Notifications: []NotificationView{},
		},
		Evidence: []Evidence{},
		Actions: []Action{{
			Kind: "pause", ExpectedGeneration: 1,
		}},
		Diagnostics:   []Diagnostic{},
		ThroughOffset: 5,
	}
}

func newHTTPFixture(
	t *testing.T,
	host, origin string,
) (*HTTPHandler, *httpFakeProjector, *httpFakeCommands) {
	t.Helper()
	projector := &httpFakeProjector{
		snapshot: httpSnapshot(),
		events: EventPage{
			SchemaVersion: SnapshotSchemaVersion,
			RunID:         "run-1",
			Events:        []Evidence{},
			ThroughOffset: 5,
			EventOffset:   5,
		},
	}
	commands := &httpFakeCommands{}
	handler, err := NewHTTPHandler(projector, commands, HTTPConfig{
		RunID: "run-1", Host: host, Origin: origin,
		BearerToken: []byte(testHTTPToken),
		MaxSSE:      2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler, projector, commands
}

func httpRequest(
	method, target, remote string,
	body []byte,
) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.RemoteAddr = remote
	return request
}

func serve(handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestHTTPConfigRequiresOneRunScope(t *testing.T) {
	t.Parallel()

	for _, runID := range []string{"", "bad/run", strings.Repeat("a", 129)} {
		handler, err := NewHTTPHandler(
			&httpFakeProjector{},
			&httpFakeCommands{},
			HTTPConfig{
				RunID:  runID,
				Host:   testLocalHost,
				Origin: testLocalOrigin,
			},
		)
		if !IsCode(err, "INVALID_HTTP_CONFIG") || handler != nil {
			t.Errorf("run scope %q = %#v, %v", runID, handler, err)
		}
	}
}

func TestHTTPAuthorityUsesPeerAndConfiguredHost(t *testing.T) {
	t.Parallel()

	local, _, localCommands := newHTTPFixture(
		t,
		testLocalHost,
		testLocalOrigin,
	)
	localMutation := httpRequest(
		http.MethodPost,
		testLocalOrigin+"/api/v1/start",
		"127.0.0.1:41000",
		[]byte(`{"manifest_digest":"admitted"}`),
	)
	localMutation.Header.Set("Content-Type", "application/json")
	localMutation.Header.Set("Forwarded", "for=203.0.113.10;proto=https")
	localMutation.Header.Set("X-Forwarded-For", "203.0.113.10")
	if response := serve(local, localMutation); response.Code != http.StatusOK {
		t.Fatalf("local mutation = %d, body %s", response.Code, response.Body)
	}
	if localCommands.startCalls != 1 {
		t.Fatalf("local start calls = %d, want 1", localCommands.startCalls)
	}

	public, _, publicCommands := newHTTPFixture(
		t,
		testPublicHost,
		testPublicURL,
	)
	proxiedMutation := httpRequest(
		http.MethodPost,
		testPublicURL+"/api/v1/start",
		"127.0.0.1:42000",
		[]byte(`{"manifest_digest":"admitted"}`),
	)
	proxiedMutation.Header.Set("Content-Type", "application/json")
	proxiedMutation.SetBasicAuth("sworn", testHTTPToken)
	if response := serve(public, proxiedMutation); response.Code != http.StatusForbidden {
		t.Fatalf("proxied mutation = %d, want 403", response.Code)
	}
	if publicCommands.startCalls != 0 {
		t.Fatalf("proxied start calls = %d, want 0", publicCommands.startCalls)
	}

	noTLS := httpRequest(
		http.MethodGet,
		testPublicURL+"/api/v1/runs/run-1/snapshot",
		"203.0.113.10:43000",
		nil,
	)
	noTLS.TLS = nil
	noTLS.Header.Set("Authorization", "Bearer "+testHTTPToken)
	if response := serve(public, noTLS); response.Code != http.StatusForbidden {
		t.Fatalf("remote cleartext read = %d, want 403", response.Code)
	}

	noAuth := httpRequest(
		http.MethodGet,
		testPublicURL+"/api/v1/runs/run-1/snapshot",
		"203.0.113.10:43001",
		nil,
	)
	response := serve(public, noAuth)
	if response.Code != http.StatusUnauthorized ||
		len(response.Header().Values("WWW-Authenticate")) != 2 {
		t.Fatalf(
			"remote unauthenticated read = %d, auth=%q",
			response.Code,
			response.Header().Values("WWW-Authenticate"),
		)
	}

	for name, authorize := range map[string]func(*http.Request){
		"bearer": func(request *http.Request) {
			request.Header.Set("Authorization", "Bearer "+testHTTPToken)
		},
		"basic": func(request *http.Request) {
			request.SetBasicAuth("sworn", testHTTPToken)
		},
	} {
		request := httpRequest(
			http.MethodGet,
			testPublicURL+"/api/v1/runs/run-1/snapshot",
			"203.0.113.10:43002",
			nil,
		)
		authorize(request)
		response := serve(public, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s remote read = %d, body %s", name, response.Code, response.Body)
		}
		if strings.Contains(response.Body.String(), `"kind":"pause"`) {
			t.Fatalf("%s remote read exposed local controls: %s", name, response.Body)
		}
	}
}

func TestTelemetryHealthIsLoopbackOnlyAndReadOnly(t *testing.T) {
	t.Parallel()

	health := TelemetryHealth{
		SchemaVersion:   TelemetryHealthSchemaVersion,
		Enabled:         true,
		QueueCapacity:   256,
		QueueDepth:      3,
		Accepted:        9,
		Processed:       5,
		Dropped:         1,
		TraceExports:    4,
		MetricExports:   2,
		Failures:        1,
		LastFailureCode: "metric_export",
	}
	projector := &httpFakeProjector{}
	commands := &httpFakeCommands{}
	local, err := NewHTTPHandler(projector, commands, HTTPConfig{
		RunID: "run-1", Host: testLocalHost, Origin: testLocalOrigin,
		Telemetry: httpFakeTelemetry{status: health},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httpRequest(
		http.MethodGet,
		testLocalOrigin+telemetryHealthPath,
		"127.0.0.1:41100",
		nil,
	)
	response := serve(local, request)
	if response.Code != http.StatusOK {
		t.Fatalf("local health = %d: %s", response.Code, response.Body)
	}
	var got TelemetryHealth
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != health {
		t.Fatalf("health = %#v, want %#v", got, health)
	}
	for _, target := range []string{
		testLocalOrigin + telemetryHealthPath + "?detail=private",
	} {
		request := httpRequest(
			http.MethodGet,
			target,
			"127.0.0.1:41101",
			nil,
		)
		if response := serve(local, request); response.Code !=
			http.StatusMethodNotAllowed {
			t.Fatalf("health query = %d", response.Code)
		}
	}
	post := httpRequest(
		http.MethodPost,
		testLocalOrigin+telemetryHealthPath,
		"127.0.0.1:41102",
		nil,
	)
	if response := serve(local, post); response.Code !=
		http.StatusMethodNotAllowed {
		t.Fatalf("health mutation = %d", response.Code)
	}

	public, _, _ := newHTTPFixture(t, testPublicHost, testPublicURL)
	for _, authorization := range []string{
		"",
		"Bearer " + testHTTPToken,
	} {
		request := httpRequest(
			http.MethodGet,
			testPublicURL+telemetryHealthPath,
			"203.0.113.10:41103",
			nil,
		)
		request.Header.Set("Authorization", authorization)
		if response := serve(public, request); response.Code !=
			http.StatusNotFound {
			t.Fatalf(
				"public health with auth %q = %d",
				authorization,
				response.Code,
			)
		}
	}

	if handler, err := NewHTTPHandler(
		projector,
		commands,
		HTTPConfig{
			RunID: "run-1", Host: testPublicHost, Origin: testPublicURL,
			BearerToken: []byte(testHTTPToken),
			Telemetry:   httpFakeTelemetry{status: health},
		},
	); !IsCode(err, "INVALID_HTTP_CONFIG") || handler != nil {
		t.Fatalf("public telemetry config = %#v, %v", handler, err)
	}
}

func TestHTTPRejectsNonCanonicalAndCrossOriginRequests(t *testing.T) {
	t.Parallel()

	handler, _, _ := newHTTPFixture(t, testLocalHost, testLocalOrigin)
	tests := []struct {
		name   string
		target string
		status int
	}{
		{name: "run page", target: "/runs/run-1", status: http.StatusOK},
		{name: "other run page", target: "/runs/run-2", status: http.StatusNotFound},
		{
			name:   "other run snapshot",
			target: "/api/v1/runs/run-2/snapshot",
			status: http.StatusNotFound,
		},
		{name: "double slash", target: "//runs/run-1", status: http.StatusNotFound},
		{name: "trailing slash", target: "/runs/run-1/", status: http.StatusNotFound},
		{
			name:   "api trailing slash",
			target: "/api/v1/runs/run-1/snapshot/",
			status: http.StatusNotFound,
		},
		{name: "extra segment", target: "/runs/run-1/extra", status: http.StatusNotFound},
		{name: "dot segment", target: "/runs/../run-1", status: http.StatusNotFound},
		{name: "asset query", target: "/assets/app.css?v=1", status: http.StatusMethodNotAllowed},
		{
			name:   "snapshot query",
			target: "/api/v1/runs/run-1/snapshot?extra=1",
			status: http.StatusMethodNotAllowed,
		},
	}
	for _, test := range tests {
		request := httpRequest(
			http.MethodGet,
			testLocalOrigin+test.target,
			"127.0.0.1:44000",
			nil,
		)
		response := serve(handler, request)
		if response.Code != test.status {
			t.Errorf("%s = %d, want %d", test.name, response.Code, test.status)
		}
	}

	encoded := httpRequest(
		http.MethodGet,
		testLocalOrigin+"/runs/run%2D1",
		"127.0.0.1:44001",
		nil,
	)
	if response := serve(handler, encoded); response.Code != http.StatusBadRequest {
		t.Errorf("encoded path = %d, want 400", response.Code)
	}

	wrongOrigin := httpRequest(
		http.MethodGet,
		testLocalOrigin+"/",
		"127.0.0.1:44002",
		nil,
	)
	wrongOrigin.Header.Set("Origin", "https://attacker.example")
	if response := serve(handler, wrongOrigin); response.Code != http.StatusForbidden {
		t.Errorf("cross origin = %d, want 403", response.Code)
	}
	crossSite := httpRequest(
		http.MethodGet,
		testLocalOrigin+"/",
		"127.0.0.1:44003",
		nil,
	)
	crossSite.Header.Set("Sec-Fetch-Site", "cross-site")
	if response := serve(handler, crossSite); response.Code != http.StatusForbidden {
		t.Errorf("cross site = %d, want 403", response.Code)
	}
	wrongHost := httpRequest(
		http.MethodGet,
		"http://attacker.example/",
		"127.0.0.1:44004",
		nil,
	)
	response := serve(handler, wrongHost)
	if response.Code != http.StatusBadRequest {
		t.Errorf("wrong Host = %d, want 400", response.Code)
	}
	assertSecurityHeaders(t, response)
}

func TestHTTPSSEUsesExactOffsetsAndNativeResume(t *testing.T) {
	t.Parallel()

	handler, projector, _ := newHTTPFixture(
		t,
		testLocalHost,
		testLocalOrigin,
	)
	projector.events = EventPage{
		SchemaVersion: SnapshotSchemaVersion,
		RunID:         "run-1",
		Events: []Evidence{{
			Offset: 7, Kind: "effect_completed",
		}},
		ThroughOffset: 7,
		EventOffset:   7,
	}
	ctx, cancel := context.WithCancel(context.Background())
	projector.onEvents = cancel
	request := httpRequest(
		http.MethodGet,
		testLocalOrigin+"/api/v1/runs/run-1/events?after=5&limit=2",
		"127.0.0.1:45000",
		nil,
	).WithContext(ctx)
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Last-Event-ID", "6")
	response := serve(handler, request)
	if response.Code != http.StatusOK {
		t.Fatalf("SSE = %d, body %s", response.Code, response.Body)
	}
	projector.mu.Lock()
	after, limit := projector.eventAfter, projector.eventLimit
	projector.mu.Unlock()
	if after != 6 || limit != 2 {
		t.Fatalf("native resume = after %d limit %d, want 6/2", after, limit)
	}
	body := response.Body.String()
	for _, required := range []string{
		"id: 7\n",
		"event: invalidate\n",
		`"schema_version":"sworn.cockpit/v1"`,
		`"through_offset":7`,
	} {
		if !strings.Contains(body, required) {
			t.Errorf("SSE missing %q in %q", required, body)
		}
	}
	if strings.Contains(body, "effect_completed") {
		t.Errorf("SSE exposed event detail: %q", body)
	}

	invalidResume := httpRequest(
		http.MethodGet,
		testLocalOrigin+"/api/v1/runs/run-1/events?after=7",
		"127.0.0.1:45001",
		nil,
	)
	invalidResume.Header.Set("Last-Event-ID", "6")
	if response := serve(handler, invalidResume); response.Code != http.StatusBadRequest {
		t.Errorf("backward resume = %d, want 400", response.Code)
	}

	head := httpRequest(
		http.MethodHead,
		testLocalOrigin+"/api/v1/runs/run-1/events?after=0",
		"127.0.0.1:45002",
		nil,
	)
	head.Header.Set("Accept", "text/event-stream")
	headResponse := serve(handler, head)
	if headResponse.Code != http.StatusOK || headResponse.Body.Len() != 0 {
		t.Errorf(
			"HEAD events = %d body=%q, want 200 empty",
			headResponse.Code,
			headResponse.Body,
		)
	}
}

func TestHTTPAssetsArePinnedAndUIContractIsStatic(t *testing.T) {
	t.Parallel()

	handler, _, _ := newHTTPFixture(t, testLocalHost, testLocalOrigin)
	hashes := map[string]string{
		"/assets/barlow-condensed-regular.woff2":         "61ab1c8b3c010a33a0da2b59a7a9f594775d0c992d2dc64b50cd6339b4f3d3ac",
		"/assets/barlow-condensed-bold.woff2":            "b8c0e6116eab19c30e2529326bc6a459e7c851a9881acc7215dab22ec8014176",
		"/assets/atkinson-hyperlegible-regular.woff2":    "2df4ba17804bc7a36f123127966075d8427bff2df58d0d76820c1130bb1a4150",
		"/assets/atkinson-hyperlegible-bold.woff2":       "da8fce41a04f8498fbf79076f92d304b12e70c76f71b143c5dcfb6536c93c075",
		"/assets/licenses/OFL-Barlow.txt":                "186d750eb496a4c17a76385f82be6aea2ac1cf2de074a811d63786cf374ea73f",
		"/assets/licenses/OFL-Atkinson-Hyperlegible.txt": "f32d22b3908fcad2c86a74000614ec22e6a7f66ea7e867e616026a27aebdc143",
	}
	for path, want := range hashes {
		request := httpRequest(
			http.MethodGet,
			testLocalOrigin+path,
			"127.0.0.1:46000",
			nil,
		)
		response := serve(handler, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s = %d", path, response.Code)
		}
		sum := sha256.Sum256(response.Body.Bytes())
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("%s hash = %s, want %s", path, got, want)
		}
		assertSecurityHeaders(t, response)
	}

	index := mustEmbeddedAsset(t, "web/index.html")
	css := mustEmbeddedAsset(t, "web/app.css")
	javascript := mustEmbeddedAsset(t, "web/app.js")
	if strings.Count(index, `id="topology"`) != 1 ||
		strings.Count(index, `id="handoff"`) != 1 {
		t.Errorf("UI must have one topology and one handoff ribbon")
	}
	for _, forbidden := range []string{
		"gradient", "box-shadow", "sheet-handle",
	} {
		if strings.Contains(css+"\n"+index, forbidden) {
			t.Errorf("UI contains prohibited artifact %q", forbidden)
		}
	}
	for _, required := range []string{
		"@media (max-width: 72rem)",
		"@media (max-width: 48rem)",
		"max-height: 78svh",
	} {
		if !strings.Contains(css, required) {
			t.Errorf("CSS missing %q", required)
		}
	}
	for _, required := range []string{
		`window.matchMedia("(max-width: 72rem)")`,
		"const SNAPSHOT_POLL_MILLIS = 5_000;",
		`() => void refresh("", false)`,
		"validGraphHandoff(value.graph, value.handoff)",
		"rail.append(trackNode)",
		"button.dataset.hasBaton = String(node.has_baton)",
		"button.dataset.handoff = String(handoffNodes.has(node.id))",
		"path.dataset.edgeId = edge.id",
		"path.dataset.from = edge.from",
		"path.dataset.to = edge.to",
		`"Not reported"`,
		"No admitted delivery graph is recorded yet.",
		"No durable evidence has been recorded for this snapshot.",
		"State unavailable. Sworn could not confirm this item from durable facts.",
	} {
		if !strings.Contains(javascript, required) {
			t.Errorf("JavaScript missing %q", required)
		}
	}
	if strings.Contains(javascript, `edge.kind === "contains"`) {
		t.Errorf("UI drops a DTO edge kind instead of rendering the complete graph")
	}
	if strings.Contains(javascript, "const valid =") ||
		strings.Contains(index, "<style") ||
		strings.Contains(index, "<script>") {
		t.Errorf("UI contains inline or residue script")
	}
}

func TestHTTPJSONAdmissionIsBoundedAndClosed(t *testing.T) {
	t.Parallel()

	handler, _, commands := newHTTPFixture(
		t,
		testLocalHost,
		testLocalOrigin,
	)
	tests := []struct {
		name        string
		contentType string
		body        string
		status      int
	}{
		{
			name: "valid", contentType: "application/json",
			body: `{"manifest_digest":"admitted"}`, status: http.StatusOK,
		},
		{
			name: "unknown field", contentType: "application/json",
			body:   `{"manifest_digest":"admitted","open":true}`,
			status: http.StatusBadRequest,
		},
		{
			name: "trailing value", contentType: "application/json",
			body:   `{"manifest_digest":"admitted"} {}`,
			status: http.StatusBadRequest,
		},
		{
			name: "wrong media", contentType: "text/plain",
			body:   `{"manifest_digest":"admitted"}`,
			status: http.StatusUnsupportedMediaType,
		},
		{
			name: "bounded", contentType: "application/json",
			body:   `{"manifest_digest":"` + strings.Repeat("a", maxRequestBytes) + `"}`,
			status: http.StatusBadRequest,
		},
	}
	for _, test := range tests {
		request := httpRequest(
			http.MethodPost,
			testLocalOrigin+"/api/v1/start",
			"127.0.0.1:47000",
			[]byte(test.body),
		)
		request.Header.Set("Content-Type", test.contentType)
		response := serve(handler, request)
		if response.Code != test.status {
			t.Errorf("%s = %d, want %d", test.name, response.Code, test.status)
		}
	}
	if commands.startCalls != 1 {
		t.Fatalf("admitted start calls = %d, want 1", commands.startCalls)
	}

	headError := httpRequest(
		http.MethodHead,
		testLocalOrigin+"/not-found",
		"127.0.0.1:47001",
		nil,
	)
	response := serve(handler, headError)
	if response.Code != http.StatusNotFound || response.Body.Len() != 0 {
		t.Errorf("HEAD error = %d body=%q", response.Code, response.Body)
	}
}

func mustEmbeddedAsset(t *testing.T, path string) string {
	t.Helper()
	body, err := embeddedWeb.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func assertSecurityHeaders(
	t *testing.T,
	response *httptest.ResponseRecorder,
) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", response.Header().Get("Cache-Control"))
	}
	csp := response.Header().Get("Content-Security-Policy")
	for _, directive := range []string{
		"default-src 'none'",
		"object-src 'none'",
		"worker-src 'none'",
		"frame-ancestors 'none'",
	} {
		if !strings.Contains(csp, directive) {
			t.Errorf("CSP missing %q in %q", directive, csp)
		}
	}
}
