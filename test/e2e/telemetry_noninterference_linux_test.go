//go:build linux

package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/observe"
	swornruntime "github.com/swornagent/sworn/internal/runtime"
)

type telemetryParityMode struct {
	name     string
	endpoint string
}

type telemetryParityProcess struct {
	command *exec.Cmd
	stdout  bytes.Buffer
	stderr  bytes.Buffer
	done    chan error
}

type telemetryParityHTTPResult struct {
	status     swornruntime.RunStatus
	statusCode int
	body       []byte
	err        error
}

type telemetryParitySnapshot struct {
	Plan       string    `json:"plan"`
	Candidates []string  `json:"candidates"`
	Assembly   []string  `json:"assembly"`
	Receipts   []string  `json:"receipts"`
	States     [2]string `json:"states"`
	ExitStatus int       `json:"exit_status"`
}

type telemetryParityBlocker struct {
	started   chan struct{}
	release   chan struct{}
	armed     atomic.Bool
	active    atomic.Int64
	requests  atomic.Uint64
	timedOut  atomic.Uint64
	blockedNS atomic.Int64
	startOnce sync.Once
	stopOnce  sync.Once
}

func newTelemetryParityBlocker() *telemetryParityBlocker {
	return &telemetryParityBlocker{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (b *telemetryParityBlocker) serve(
	writer http.ResponseWriter,
	request *http.Request,
) {
	_, _ = io.Copy(io.Discard, io.LimitReader(request.Body, 2*1024*1024))
	_ = request.Body.Close()
	b.requests.Add(1)
	if !b.armed.Load() {
		writer.WriteHeader(http.StatusOK)
		return
	}
	b.active.Add(1)
	defer b.active.Add(-1)
	b.startOnce.Do(func() { close(b.started) })
	started := time.Now()
	select {
	case <-b.release:
		writer.WriteHeader(http.StatusOK)
	case <-request.Context().Done():
		b.timedOut.Add(1)
	}
	b.blockedNS.Add(time.Since(started).Nanoseconds())
}

func (b *telemetryParityBlocker) arm() {
	b.armed.Store(true)
}

func (b *telemetryParityBlocker) unblock() {
	b.stopOnce.Do(func() { close(b.release) })
}

func TestRealBinaryTelemetryCannotAffectDelivery(t *testing.T) {
	t.Parallel()

	var failedRequests atomic.Uint64
	failingOTLP := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		_, _ = io.Copy(io.Discard, io.LimitReader(request.Body, 2*1024*1024))
		_ = request.Body.Close()
		failedRequests.Add(1)
		http.Error(writer, "fixture exporter failure", http.StatusServiceUnavailable)
	}))
	defer failingOTLP.Close()

	blocker := newTelemetryParityBlocker()
	blockedOTLP := httptest.NewServer(http.HandlerFunc(blocker.serve))
	defer func() {
		blocker.unblock()
		blockedOTLP.Close()
	}()

	buildRoot := t.TempDir()
	fakeBinary := filepath.Join(buildRoot, "telemetry-fake")
	buildBinary(t, fakeBinary, "./test/e2e/testdata/fake", "")
	fakeDigest := fileDigest(t, fakeBinary)
	swornBinary := filepath.Join(buildRoot, "sworn")
	buildBinary(t, swornBinary, "./cmd/sworn", "")

	fixtureRoot := t.TempDir()
	repository := filepath.Join(fixtureRoot, "product")
	modes := []telemetryParityMode{
		{name: "disabled"},
		{name: "exporter_failing", endpoint: failingOTLP.URL},
		{name: "exporter_backpressured", endpoint: blockedOTLP.URL},
	}

	results := make(map[string]telemetryParitySnapshot, len(modes))
	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			results[mode.name] = telemetryParityDelivery(
				t,
				mode,
				swornBinary,
				fakeBinary,
				fakeDigest,
				fixtureRoot,
				repository,
				&failedRequests,
				blocker,
			)
		})
	}

	baseline := telemetryParityJSON(t, results["disabled"])
	for _, mode := range modes[1:] {
		got := telemetryParityJSON(t, results[mode.name])
		if !bytes.Equal(got, baseline) {
			t.Fatalf(
				"%s changed delivery\nbaseline:\n%s\nobserved:\n%s",
				mode.name,
				baseline,
				got,
			)
		}
	}
}

func telemetryParityDelivery(
	t *testing.T,
	mode telemetryParityMode,
	swornBinary, fakeBinary, fakeDigest, fixtureRoot, repository string,
	failedRequests *atomic.Uint64,
	blocker *telemetryParityBlocker,
) telemetryParitySnapshot {
	t.Helper()
	const (
		runID   = "e2e-telemetry-parity"
		release = "e2e-telemetry-parity-release"
	)

	if err := os.RemoveAll(repository); err != nil {
		t.Fatal(err)
	}
	telemetryParityRepository(t, repository)

	manifestBody, planBytes, plan := e2eManifest(
		t,
		runID,
		repository,
		release,

		fakeBinary,
		fakeDigest,
		"verifier-model")

	modeRoot := filepath.Join(fixtureRoot, "run-"+mode.name)
	if err := os.MkdirAll(modeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	manifestPath := writeManifest(t, modeRoot, manifestBody)
	journalPath := filepath.Join(modeRoot, "run.sqlite")
	address := telemetryParityAddress(t)
	configPath := telemetryParityOperatorConfig(
		t,
		modeRoot,
		address,
		mode.endpoint,
	)
	process := telemetryParityStartServe(
		t,
		swornBinary,
		runID,
		journalPath,
		manifestPath,
		configPath,
	)
	t.Cleanup(func() {
		if process.command.Process != nil {
			_ = process.command.Process.Signal(syscall.SIGTERM)
		}
	})
	telemetryParityWaitHealth(t, address, func(cockpit.TelemetryHealth) bool {
		return true
	})

	sum := sha256.Sum256(manifestBody)
	start := telemetryParityPost(
		t,
		address,
		"/api/v2/start",
		cockpit.StartCommand{
			ManifestDigest: "sha256:" + hex.EncodeToString(sum[:]),
		},
	)
	if start.State != "awaiting_approval" {
		t.Fatalf("start state = %q", start.State)
	}
	telemetryParityObserveMode(
		t,
		mode.name,
		address,
		failedRequests,
	)

	authorizePlan(t, journalPath, runID, plan)
	installAndPassComponent(t, repository, release, planBytes)
	resumeCommand := cockpit.ControlCommand{
		RunID:              runID,
		CommandID:          "resume-1",
		Kind:               journal.Resume,
		ExpectedGeneration: 0,
	}
	var resume swornruntime.RunStatus
	if mode.name != "exporter_backpressured" {
		resume = telemetryParityPost(
			t,
			address,
			"/api/v2/runs/"+runID+"/commands",
			resumeCommand,
		)
	} else {
		resume = telemetryParityBackpressuredResume(
			t,
			address,
			blocker,
			resumeCommand,
		)
	}
	if resume.State != "complete" || resume.Outcome != "merged" {
		t.Fatalf("resume status = %#v", resume)
	}

	snapshot := telemetryParityBatonSnapshot(t, repository, release)
	snapshot.States = [2]string{start.State, resume.State}
	assertDispatchOrder(t, journalPath, runID)
	if mode.name == "exporter_backpressured" {
		telemetryParityFinishBackpressure(t, address, blocker)
	}
	snapshot.ExitStatus = telemetryParityStopServe(t, process)
	return snapshot
}

func telemetryParityObserveMode(
	t *testing.T,
	mode, address string,
	failedRequests *atomic.Uint64,
) {
	t.Helper()
	switch mode {
	case "disabled":
		health := telemetryParityWaitHealth(t, address, func(
			value cockpit.TelemetryHealth,
		) bool {
			return !value.Enabled
		})
		if health.QueueCapacity != 0 || health.Accepted != 0 ||
			health.Processed != 0 || health.Dropped != 0 ||
			health.Failures != 0 {
			t.Fatalf("disabled telemetry health = %#v", health)
		}
	case "exporter_failing":
		health := telemetryParityWaitHealth(t, address, func(
			value cockpit.TelemetryHealth,
		) bool {
			return value.Enabled && value.Failures > 0 &&
				value.LastFailureCode == "trace_export"
		})
		if failedRequests.Load() == 0 || health.Processed == 0 {
			t.Fatalf(
				"failing exporter was not exercised: requests=%d health=%#v",
				failedRequests.Load(),
				health,
			)
		}
	case "exporter_backpressured":
		health := telemetryParityWaitHealth(t, address, func(
			value cockpit.TelemetryHealth,
		) bool {
			return value.Enabled && value.Processed > 0 &&
				value.TraceExports > 0
		})
		if health.QueueCapacity != 256 || health.Failures != 0 {
			t.Fatalf("pre-backpressure telemetry health = %#v", health)
		}
	default:
		t.Fatalf("unknown telemetry mode %q", mode)
	}
}

func telemetryParityBackpressuredResume(
	t *testing.T,
	address string,
	blocker *telemetryParityBlocker,
	command cockpit.ControlCommand,
) swornruntime.RunStatus {
	t.Helper()
	const path = "/api/v2/runs/e2e-telemetry-parity/commands"
	blocker.arm()
	completed := make(chan telemetryParityHTTPResult, 1)
	go func() {
		completed <- telemetryParityDoPost(address, path, command)
	}()
	select {
	case <-blocker.started:
	case result := <-completed:
		t.Fatalf("delivery completed before OTLP backpressure: %v", result.err)
	case <-time.After(15 * time.Second):
		t.Fatal("delivery produced no backpressured OTLP request")
	}
	select {
	case result := <-completed:
		return telemetryParityRequirePost(t, path, result)
	case <-time.After(30 * time.Second):
		t.Fatal("delivery did not complete under OTLP backpressure")
	}
	return swornruntime.RunStatus{}
}

func telemetryParityFinishBackpressure(
	t *testing.T,
	address string,
	blocker *telemetryParityBlocker,
) {
	t.Helper()
	if blocker.active.Load() == 0 &&
		(blocker.timedOut.Load() == 0 ||
			time.Duration(blocker.blockedNS.Load()) < 2500*time.Millisecond) {
		t.Fatalf(
			"OTLP request was not sustainably backpressured: active=%d timeouts=%d blocked=%s",
			blocker.active.Load(),
			blocker.timedOut.Load(),
			time.Duration(blocker.blockedNS.Load()),
		)
	}
	blocker.unblock()
	telemetryParityWaitHealth(t, address, func(
		value cockpit.TelemetryHealth,
	) bool {
		return blocker.active.Load() == 0 &&
			value.Processed > 0 && value.TraceExports > 0
	})
}

func telemetryParityRepository(t *testing.T, repository string) {
	t.Helper()
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(
		e2eGit,
		"init",
		"--quiet",
		"--initial-branch=main",
		repository,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	if err := os.WriteFile(
		filepath.Join(repository, "base.txt"),
		[]byte("base\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "--", "base.txt")
	runGit(t, repository, "commit", "--quiet", "-m", "base")
}

func telemetryParityAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func telemetryParityOperatorConfig(
	t *testing.T,
	root, address, endpoint string,
) string {
	t.Helper()
	config := struct {
		SchemaVersion string `json:"schema_version"`
		Local         struct {
			Listen string `json:"listen"`
		} `json:"local"`
		OTel *observe.Config `json:"otel,omitempty"`
	}{
		SchemaVersion: "sworn.operator-config/v1",
	}
	config.Local.Listen = address
	if endpoint != "" {
		config.OTel = &observe.Config{
			SchemaVersion: observe.OTelConfigSchemaVersion,
			Endpoint:      endpoint,
			Headers:       map[string]string{},
		}
	}
	body, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "operator.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func telemetryParityStartServe(
	t *testing.T,
	binary, runID, journalPath, manifestPath, configPath string,
) *telemetryParityProcess {
	t.Helper()
	result := &telemetryParityProcess{done: make(chan error, 1)}
	result.command = exec.Command(
		binary,
		"serve",
		"--run",
		runID,
		"--journal",
		journalPath,
		"--manifest",
		manifestPath,
		"--operator-config",
		configPath,
	)
	result.command.Env = cleanEnvironment(map[string]string{
		"OTEL_EXPORTER_OTLP_CERTIFICATE":                "",
		"OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE":         "",
		"OTEL_EXPORTER_OTLP_CLIENT_KEY":                 "",
		"OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE":         "",
		"OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE":  "",
		"OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY":          "",
		"OTEL_EXPORTER_OTLP_METRICS_CERTIFICATE":        "",
		"OTEL_EXPORTER_OTLP_METRICS_CLIENT_CERTIFICATE": "",
		"OTEL_EXPORTER_OTLP_METRICS_CLIENT_KEY":         "",
		"OTEL_GO_X_OBSERVABILITY":                       "",
	})
	result.command.Stdout = &result.stdout
	result.command.Stderr = &result.stderr
	if err := result.command.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		result.done <- result.command.Wait()
	}()
	return result
}

func telemetryParityStopServe(
	t *testing.T,
	process *telemetryParityProcess,
) int {
	t.Helper()
	if err := process.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("stop sworn serve: %v", err)
	}
	var runErr error
	select {
	case runErr = <-process.done:
	case <-time.After(15 * time.Second):
		_ = process.command.Process.Kill()
		t.Fatal("sworn serve did not stop")
	}
	exit := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			t.Fatalf("wait sworn serve: %v", runErr)
		}
		exit = exitErr.ExitCode()
	}
	if exit != 0 ||
		process.stdout.String() != "sworn serve: ready\n" ||
		process.stderr.Len() != 0 {
		t.Fatalf(
			"serve exit=%d stdout=%q stderr=%q",
			exit,
			process.stdout.String(),
			process.stderr.String(),
		)
	}
	return exit
}

func telemetryParityPost(
	t *testing.T,
	address, path string,
	value any,
) swornruntime.RunStatus {
	t.Helper()
	return telemetryParityRequirePost(
		t,
		path,
		telemetryParityDoPost(address, path, value),
	)
}

func telemetryParityDoPost(
	address, path string,
	value any,
) telemetryParityHTTPResult {
	body, err := json.Marshal(value)
	if err != nil {
		return telemetryParityHTTPResult{err: err}
	}
	request, err := http.NewRequest(
		http.MethodPost,
		"http://"+address+path,
		bytes.NewReader(body),
	)
	if err != nil {
		return telemetryParityHTTPResult{err: err}
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
		Timeout:   30 * time.Second,
	}
	response, err := client.Do(request)
	if err != nil {
		return telemetryParityHTTPResult{err: err}
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
	if err != nil {
		return telemetryParityHTTPResult{err: err}
	}
	result := telemetryParityHTTPResult{
		statusCode: response.StatusCode,
		body:       responseBody,
	}
	if response.StatusCode == http.StatusOK {
		result.err = json.Unmarshal(responseBody, &result.status)
	}
	return result
}

func telemetryParityRequirePost(
	t *testing.T,
	path string,
	result telemetryParityHTTPResult,
) swornruntime.RunStatus {
	t.Helper()
	if result.err != nil {
		t.Fatalf("POST %s: %v\n%s", path, result.err, result.body)
	}
	if result.statusCode != http.StatusOK {
		t.Fatalf(
			"POST %s status=%d body=%s",
			path,
			result.statusCode,
			result.body,
		)
	}
	return result.status
}

func telemetryParityWaitHealth(
	t *testing.T,
	address string,
	ready func(cockpit.TelemetryHealth) bool,
) cockpit.TelemetryHealth {
	t.Helper()
	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
		Timeout:   time.Second,
	}
	deadline := time.Now().Add(15 * time.Second)
	var last cockpit.TelemetryHealth
	var lastErr error
	for time.Now().Before(deadline) {
		response, err := client.Get(
			"http://" + address + "/api/v2/operator/telemetry",
		)
		if err == nil {
			body, readErr := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
			closeErr := response.Body.Close()
			if readErr == nil && closeErr == nil &&
				response.StatusCode == http.StatusOK {
				lastErr = json.Unmarshal(body, &last)
				if lastErr == nil && ready(last) {
					return last
				}
			} else if readErr != nil {
				lastErr = readErr
			} else if closeErr != nil {
				lastErr = closeErr
			} else {
				lastErr = fmt.Errorf("telemetry status %d", response.StatusCode)
			}
		} else {
			lastErr = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("telemetry health timeout: last=%#v err=%v", last, lastErr)
	return cockpit.TelemetryHealth{}
}

func telemetryParityBatonSnapshot(
	t *testing.T,
	repository, release string,
) telemetryParitySnapshot {
	t.Helper()
	state := readBatonState(t, repository, release)
	if state.Assembly.Outcome != "merged" ||
		state.Assembly.Candidate == nil ||
		state.Assembly.Pass == nil ||
		state.Assembly.CurrentReceipt == nil ||
		state.Assembly.ResultCommit == "" {
		t.Fatalf("incomplete assembly = %#v", state.Assembly)
	}
	result := telemetryParitySnapshot{
		Plan: telemetryParityReceipt(t, "plan", state.Plan.Approval),
	}
	histories := append([]baton.SliceHistoryState(nil), state.SliceHistories...)
	sort.Slice(histories, func(left, right int) bool {
		return histories[left].Slice < histories[right].Slice
	})
	for _, history := range histories {
		slice, ok := state.Slice(history.Slice)
		if !ok || slice.Candidate == nil || slice.Pass == nil ||
			slice.Candidate.Receipt.Candidate == nil ||
			slice.Candidate.Receipt.ProductTree == nil ||
			slice.Pass.Receipt.Candidate == nil ||
			slice.Pass.Receipt.ProductTree == nil {
			t.Fatalf("incomplete slice %s = %#v", history.Slice, slice)
		}
		if *slice.Candidate.Receipt.Candidate !=
			*slice.Pass.Receipt.Candidate ||
			*slice.Candidate.Receipt.ProductTree !=
				*slice.Pass.Receipt.ProductTree {
			t.Fatalf("slice %s PASS changed candidate", history.Slice)
		}
		candidate := *slice.Candidate.Receipt.Candidate
		result.Candidates = append(result.Candidates, strings.Join([]string{
			history.Slice,
			telemetryParityReceipt(t, history.Slice, *slice.Candidate),
			candidate,
			runGit(t, repository, "rev-parse", candidate+"^{tree}"),
			*slice.Candidate.Receipt.ProductTree,
			telemetryParityReceipt(t, history.Slice, *slice.Pass),
		}, "\x1e"))
		for _, entry := range history.History.Entries {
			result.Receipts = append(
				result.Receipts,
				telemetryParityReceipt(t, history.Slice, entry),
			)
		}
	}

	assemblyCandidate := state.Assembly.Candidate
	assemblyPass := state.Assembly.Pass
	merge := state.Assembly.CurrentReceipt
	if assemblyCandidate.Receipt.Candidate == nil ||
		assemblyCandidate.Receipt.ProductTree == nil ||
		assemblyPass.Receipt.Candidate == nil ||
		assemblyPass.Receipt.ProductTree == nil ||
		merge.Receipt.Candidate == nil ||
		merge.Receipt.ProductTree == nil ||
		merge.Receipt.ResultCommit == nil {
		t.Fatalf("incomplete assembly receipts = %#v", state.Assembly)
	}
	candidate := *assemblyCandidate.Receipt.Candidate
	candidateTree := runGit(
		t,
		repository,
		"rev-parse",
		candidate+"^{tree}",
	)
	target := runGit(t, repository, "rev-parse", "refs/heads/main")
	targetTree := runGit(
		t,
		repository,
		"rev-parse",
		"refs/heads/main^{tree}",
	)
	if *assemblyPass.Receipt.Candidate != candidate ||
		*merge.Receipt.Candidate != candidate ||
		*assemblyPass.Receipt.ProductTree !=
			*assemblyCandidate.Receipt.ProductTree ||
		*merge.Receipt.ProductTree !=
			*assemblyCandidate.Receipt.ProductTree ||
		*merge.Receipt.ResultCommit != state.Assembly.ResultCommit ||
		state.Assembly.ResultCommit != target ||
		candidateTree != targetTree {
		t.Fatalf(
			"assembly/target identity changed: candidate=%s pass=%s merge=%s result=%s target=%s candidate_tree=%s target_tree=%s",
			candidate,
			*assemblyPass.Receipt.Candidate,
			*merge.Receipt.Candidate,
			state.Assembly.ResultCommit,
			target,
			candidateTree,
			targetTree,
		)
	}
	result.Assembly = []string{
		telemetryParityReceipt(t, "assembly", *assemblyCandidate),
		telemetryParityReceipt(t, "assembly", *assemblyPass),
		telemetryParityReceipt(t, "assembly", *merge),
		candidate,
		candidateTree,
		*assemblyCandidate.Receipt.ProductTree,
		state.Assembly.ResultCommit,
		target,
		targetTree,
	}
	for _, entry := range state.Assembly.History {
		result.Receipts = append(
			result.Receipts,
			telemetryParityReceipt(t, "assembly", entry),
		)
	}
	return result
}

func telemetryParityReceipt(
	t *testing.T,
	location string,
	entry baton.ReceiptEntry,
) string {
	t.Helper()
	canonical, err := entry.Receipt.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join([]string{
		location,
		entry.OID,
		entry.Parent,
		entry.Tree,
		entry.Subject,
		hex.EncodeToString(entry.Detail),
		string(canonical),
	}, "\x1f")
}

func telemetryParityJSON(
	t *testing.T,
	value telemetryParitySnapshot,
) []byte {
	t.Helper()
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return body
}
