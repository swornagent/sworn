package telemetry

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- IsEnabled tests ---------------------------------------------------

func TestIsEnabled_EnvVar(t *testing.T) {
	t.Setenv("SWORN_NO_TELEMETRY", "1")
	if IsEnabled() {
		t.Error("IsEnabled() = true; want false when SWORN_NO_TELEMETRY=1")
	}
}

func TestIsEnabled_Sentinel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgDir := filepath.Join(dir, ".config", "sworn")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatal(err)
	}
	// Create both — .no-telemetry wins.
	if err := os.WriteFile(filepath.Join(cfgDir, ".telemetry-enabled"), []byte{}, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, ".no-telemetry"), []byte{}, 0600); err != nil {
		t.Fatal(err)
	}
	if IsEnabled() {
		t.Error("IsEnabled() = true; want false when .no-telemetry exists")
	}
}

func TestIsEnabled_Neither(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	// No sentinel files exist — IsEnabled() should return false
	// (telemetry disabled: consent not yet given; init not run).
	if IsEnabled() {
		t.Error("IsEnabled() = true; want false when no sentinel files exist (init not run)")
	}
}

func TestIsEnabled_OptedIn_NoOverrides(t *testing.T) {	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgDir := filepath.Join(dir, ".config", "sworn")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, ".telemetry-enabled"), []byte{}, 0600); err != nil {
		t.Fatal(err)
	}
	if !IsEnabled() {
		t.Error("IsEnabled() = false; want true when .telemetry-enabled exists and no override")
	}
}

// --- InstallID tests ---------------------------------------------------

func TestInstallIDIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	first := InstallID()
	if first == "" {
		t.Fatal("InstallID() returned empty string on first call")
	}

	// Reset state to test the read path.
	resetInstallIDForTest()
	second := InstallID()

	if second != first {
		t.Errorf("InstallID() second call = %q; want %q (same UUID)", second, first)
	}

	// Verify the file was written.
	p := filepath.Join(dir, ".config", "sworn", "install-id")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(data))
	if got != first {
		t.Errorf("file contents = %q; want %q", got, first)
	}
}

func TestInstallIDWriteFailure(t *testing.T) {
	dir := t.TempDir()
	// Make the parent read-only.
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(dir, "nonexistent"))

	resetInstallIDForTest()
	got := InstallID()

	if got != "" {
		t.Errorf("InstallID() = %q; want empty string on write failure", got)
	}
}

// --- Fire tests --------------------------------------------------------

// fireTestTransport rewrites the telemetry API URL to the test server.
type fireTestTransport struct {
	targetURL string
}

func (ft *fireTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.String() == "https://api.sworn.sh/v1/events" {
		u, err := url.Parse(ft.targetURL)
		if err != nil {
			return nil, err
		}
		req.URL = u
	}
	return http.DefaultTransport.RoundTrip(req)
}

func TestFireSchema(t *testing.T) {
	// First, verify the event struct serialises correctly.
	evt := event{
		V:            1,
		InstallID:    "test-uuid",
		Cmd:          "run",
		Sub:          "parallel",
		DurationMS:   1234,
		ExitCode:     0,
		SwornVersion: "0.1.0",
		GoVersion:    "go1.26",
		OS:           "linux",
		Arch:         "amd64",
	}
	body, err := json.Marshal(evt)
	if err != nil {
		t.Fatal(err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}

	expectedFields := []string{"v", "install_id", "cmd", "sub", "duration_ms",
		"exit_code", "sworn_version", "go_version", "os", "arch"}
	for _, f := range expectedFields {
		if _, ok := decoded[f]; !ok {
			t.Errorf("event missing field %q", f)
		}
	}
	if len(decoded) != len(expectedFields) {
		t.Errorf("event has %d fields; want %d (extra: %v)", len(decoded), len(expectedFields), decoded)
	}

	// Now test the full Fire() HTTP flow.
	var requestBody []byte
	requestMu := sync.Mutex{}
	requestReceived := make(chan struct{}, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMu.Lock()
		body, _ := ioReadAll(r.Body)
		requestBody = body
		requestMu.Unlock()
		requestReceived <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Set up custom HTTP client with rewriting transport.
	testClient := &http.Client{Transport: &fireTestTransport{targetURL: ts.URL}}
	prevClient := HTTPClient()
	SetHTTPClient(testClient)
	defer SetHTTPClient(prevClient)

	// Set up clean install-id.
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	resetInstallIDForTest()
	InstallID()

	Fire("run", "parallel", "0.1.0", 1234, 0)

	select {
	case <-requestReceived:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Fire() to send request")
	}

	requestMu.Lock()
	sentBody := append([]byte{}, requestBody...)
	requestMu.Unlock()
	var actual map[string]interface{}
	if err := json.Unmarshal(sentBody, &actual); err != nil {
		t.Fatal(err)
	}
	for _, f := range expectedFields {
		if _, ok := actual[f]; !ok {
			t.Errorf("Fire() event missing field %q in HTTP body", f)
		}
	}
	if actual["cmd"].(string) != "run" {
		t.Errorf("Fire() cmd = %v; want run", actual["cmd"])
	}
	if actual["sub"].(string) != "parallel" {
		t.Errorf("Fire() sub = %v; want parallel", actual["sub"])
	}
	if actual["sworn_version"].(string) != "0.1.0" {
		t.Errorf("Fire() sworn_version = %v; want 0.1.0", actual["sworn_version"])
	}
	if actual["os"].(string) == "" {
		t.Error("Fire() os is empty")
	}
	if actual["arch"].(string) == "" {
		t.Error("Fire() arch is empty")
	}
}

var ioReadAll = func(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	return buf.Bytes(), err
}

func TestFireNonBlocking(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	prevClient := HTTPClient()
	testClient := &http.Client{Transport: &fireTestTransport{targetURL: ts.URL}}
	SetHTTPClient(testClient)
	defer SetHTTPClient(prevClient)

	start := time.Now()
	Fire("run", "", "0.1.0", 100, 0)
	elapsed := time.Since(start)
	if elapsed > 10*time.Millisecond {
		t.Errorf("Fire() took %v; want < 10ms (should be non-blocking, per AC8)", elapsed)
	}
}

func TestFireSilentOnError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	prevClient := HTTPClient()
	testClient := &http.Client{Transport: &fireTestTransport{targetURL: ts.URL}}
	SetHTTPClient(testClient)
	defer SetHTTPClient(prevClient)

	Fire("run", "", "0.1.0", 100, 0)

	time.Sleep(100 * time.Millisecond)
	// If we get here without panic, test passes.
}

func TestFireTelemetryMetaCommandExcluded(t *testing.T) {
	var hit bool
	hitMu := sync.Mutex{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {		hitMu.Lock()
		hit = true
		hitMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	prevClient := HTTPClient()
	testClient := &http.Client{Transport: &fireTestTransport{targetURL: ts.URL}}
	SetHTTPClient(testClient)
	defer SetHTTPClient(prevClient)
	Fire("telemetry", "on", "0.1.0", 100, 0)
	time.Sleep(100 * time.Millisecond)

	hitMu.Lock()
	wasHit := hit
	hitMu.Unlock()
	if wasHit {
		t.Error("Fire() sent telemetry event for cmd=telemetry; should be excluded")
	}
}

func TestFireSkipsEmptyCmd(t *testing.T) {
	var hit bool
	hitMu := sync.Mutex{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitMu.Lock()
		hit = true
		hitMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	prevClient := HTTPClient()
	testClient := &http.Client{Transport: &fireTestTransport{targetURL: ts.URL}}
	SetHTTPClient(testClient)
	defer SetHTTPClient(prevClient)

	// Fire with empty cmd (no-args / TUI launch) — should be excluded.
	Fire("", "", "0.1.0", 5000, 0)
	time.Sleep(100 * time.Millisecond)

	hitMu.Lock()
	wasHit := hit
	hitMu.Unlock()
	if wasHit {
		t.Error("Fire() sent telemetry event for cmd=\"\" (TUI launch); should be excluded")
	}
}

func TestFireStillFiresRealCmd(t *testing.T) {
	var hit bool
	hitMu := sync.Mutex{}
	requestReceived := make(chan struct{}, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitMu.Lock()
		hit = true
		hitMu.Unlock()
		requestReceived <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	prevClient := HTTPClient()
	testClient := &http.Client{Transport: &fireTestTransport{targetURL: ts.URL}}
	SetHTTPClient(testClient)
	defer SetHTTPClient(prevClient)

	// Fire with a real command — must still fire (guard against over-broad exclusion).
	Fire("verify", "", "0.1.0", 100, 0)

	select {
	case <-requestReceived:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Fire(\"verify\", ...) to send request — over-broad exclusion?")
	}

	hitMu.Lock()
	wasHit := hit
	hitMu.Unlock()
	if !wasHit {
		t.Error("Fire(\"verify\", ...) did not send request; real commands must still fire")
	}
}

// --- ShowDisclosure tests ----------------------------------------------
func TestShowDisclosure_FirstRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	var buf bytes.Buffer
	ShowDisclosure(&buf)

	output := buf.String()
	if output == "" {
		t.Error("ShowDisclosure() printed nothing on first run; expected disclosure text")
	}
	if !strings.Contains(output, "anonymous usage telemetry") {
		t.Errorf("ShowDisclosure() output missing expected text: %q", output)
	}

	disclosedPath := filepath.Join(dir, ".config", "sworn", ".telemetry-disclosed")
	if !fileExists(disclosedPath) {
		t.Error(".telemetry-disclosed sentinel not created after ShowDisclosure()")
	}
}

func TestShowDisclosure_SubsequentRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	cfgDir := filepath.Join(dir, ".config", "sworn")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, ".telemetry-disclosed"), []byte{}, 0600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	ShowDisclosure(&buf)

	if buf.Len() > 0 {
		t.Errorf("ShowDisclosure() printed on subsequent run: %q; want nothing", buf.String())
	}
}

func TestShowDisclosure_NeutralPrecondition(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgDir := filepath.Join(dir, ".config", "sworn")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatal(err)
	}
	// Opted in but no .telemetry-disclosed — disclosure should NOT print.
	if err := os.WriteFile(filepath.Join(cfgDir, ".telemetry-enabled"), []byte{}, 0600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	ShowDisclosure(&buf)

	if buf.Len() > 0 {
		t.Errorf("ShowDisclosure() printed when opted in: %q; want nothing (neutral precondition)", buf.String())
	}
}

func TestShowDisclosure_OptedOutPrecondition(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cfgDir := filepath.Join(dir, ".config", "sworn")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, ".no-telemetry"), []byte{}, 0600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	ShowDisclosure(&buf)

	if buf.Len() > 0 {
		t.Errorf("ShowDisclosure() printed when opted out: %q; want nothing", buf.String())
	}
}

// --- ShowConsent tests -------------------------------------------------

func TestShowConsent_Yes(t *testing.T) {
	var stdin bytes.Buffer
	var stdout bytes.Buffer
	stdin.WriteString("Y\n")

	optedIn, err := ShowConsent(&stdin, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	if !optedIn {
		t.Error("ShowConsent(Y) = false; want true")
	}
	if !strings.Contains(stdout.String(), "Enable telemetry") {
		t.Error("ShowConsent() did not print prompt")
	}
}

func TestShowConsent_No(t *testing.T) {
	var stdin bytes.Buffer
	var stdout bytes.Buffer
	stdin.WriteString("n\n")

	optedIn, err := ShowConsent(&stdin, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	if optedIn {
		t.Error("ShowConsent(n) = true; want false")
	}
}

func TestShowConsent_Enter(t *testing.T) {
	var stdin bytes.Buffer
	var stdout bytes.Buffer
	stdin.WriteString("\n")

	optedIn, err := ShowConsent(&stdin, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	if !optedIn {
		t.Error("ShowConsent(<Enter>) = false; want true (defaults to yes)")
	}
}

func TestShowConsent_NoLong(t *testing.T) {
	var stdin bytes.Buffer
	var stdout bytes.Buffer
	stdin.WriteString("no\n")

	optedIn, err := ShowConsent(&stdin, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	if optedIn {
		t.Error("ShowConsent(no) = true; want false")
	}
}

// --- trimGoVersion tests -----------------------------------------------

func TestTrimGoVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"go1.26.0", "go1.26"},
		{"go1.21", "go1.21"},
		{"go1.21.13", "go1.21"},
		{"go1.26.0-fips", "go1.26"},
		{"gccgo1.24", "gccgo1.24"},
		{"", ""},
		{"devel", "devel"},
	}
	for _, tc := range tests {
		got := trimGoVersion(tc.input)
		if got != tc.want {
			t.Errorf("trimGoVersion(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}