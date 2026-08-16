package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/swornagent/sworn/internal/driver"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "descendant" {
		for {
			time.Sleep(time.Hour)
		}
	}
	behavior := filepath.Base(os.Args[0])
	var scriptedSubmission string
	var scriptedReserved []string
	requestBody, _ := io.ReadAll(io.LimitReader(os.Stdin, driver.MaxRequestBytes+1))
	request, requestErr := driver.DecodeRequest(requestBody)
	if behavior == "driver" {
		if requestErr != nil {
			os.Exit(64)
		}
		for _, input := range request.Inputs {
			if input.Name != "fake-script" {
				continue
			}
			body, err := os.ReadFile(filepath.Join(
				driver.GuestInputPath,
				filepath.FromSlash(input.Path),
			))
			if err != nil || driver.Digest(body) != input.Digest {
				os.Exit(64)
			}
			var script struct {
				Behavior   string   `json:"behavior"`
				Submission string   `json:"submission"`
				Reserved   []string `json:"reserved"`
			}
			if err := json.Unmarshal(body, &script); err != nil {
				os.Exit(64)
			}
			behavior = script.Behavior
			scriptedSubmission = script.Submission
			scriptedReserved = append([]string(nil), script.Reserved...)
		}
	}
	cleanRequestBody := requestBody
	if requestErr == nil {
		cleanRequest := request
		cleanRequest.Inputs = []driver.Input{}
		if body, err := driver.EncodeRequest(cleanRequest); err == nil {
			cleanRequestBody = body
		}
	}
	switch behavior {
	case "crash":
		_, _ = io.WriteString(os.Stderr, "fixture process exited before producing a result\n")
		os.Exit(17)
	case "missing-result":
		return
	case "malformed-json":
		_, _ = io.WriteString(os.Stdout, "{\"broken\":\n")
	case "nonzero-stdout":
		_, _ = io.WriteString(os.Stdout, "{}\n")
		os.Exit(17)
	case "oversized-stdout":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte{'x'}, driver.MaxResultEnvelopeBytes+1))
	case "oversized-stderr":
		_, _ = os.Stderr.Write(bytes.Repeat([]byte{'x'}, driver.MaxStderrBytes+1))
	case "multiple-results":
		var output bytes.Buffer
		if driver.RunFakeCommand(
			[]string{"run"},
			bytes.NewReader(cleanRequestBody),
			&output,
			io.Discard,
			driver.FakeCompleted,
		) != 0 {
			os.Exit(64)
		}
		_, _ = os.Stdout.Write(output.Bytes())
		_, _ = os.Stdout.Write(output.Bytes())
	case "stderr-noise":
		_, _ = os.Stderr.Write(bytes.Repeat([]byte{'n'}, 4_096))
		os.Exit(driver.RunFakeCommand(
			[]string{"run"},
			bytes.NewReader(cleanRequestBody),
			os.Stdout,
			io.Discard,
			driver.FakeCompleted,
		))
	case "environment-canary":
		if value := os.Getenv("SWORN_PARENT_SECRET"); value != "" {
			_, _ = io.WriteString(os.Stderr, value)
			os.Exit(17)
		}
		os.Exit(driver.RunFakeCommand(
			[]string{"run"},
			bytes.NewReader(cleanRequestBody),
			os.Stdout,
			io.Discard,
			driver.FakeCompleted,
		))
	case "isolation-canaries":
		if requestErr != nil || !isIsolated(request) {
			_, _ = io.WriteString(os.Stderr, "ISOLATION_CANARY_FAILED\n")
			os.Exit(17)
		}
		os.Exit(driver.RunFakeCommand(
			[]string{"run"},
			bytes.NewReader(cleanRequestBody),
			os.Stdout,
			io.Discard,
			driver.FakeCompleted,
		))
	case "reserved-canary":
		if requestErr != nil || !reservedNamesMasked(request, scriptedReserved) {
			_, _ = io.WriteString(os.Stderr, "RESERVED_MASK_CANARY_FAILED\n")
			os.Exit(17)
		}
		// Hand the original body on: the protocol fake sees the script and
		// performs the scripted submission after the mask proof, because
		// every model responsibility must submit to complete.
		os.Exit(driver.RunFakeCommand(
			[]string{"run"},
			bytes.NewReader(requestBody),
			os.Stdout,
			io.Discard,
			driver.FakeCompleted,
		))
	case "block-descendant":
		child := exec.Command(os.Args[0], "descendant")
		if err := child.Start(); err != nil {
			os.Exit(64)
		}
		for {
			time.Sleep(time.Hour)
		}
	case "submit-descendant":
		body, err := base64.StdEncoding.Strict().DecodeString(scriptedSubmission)
		if err != nil {
			os.Exit(64)
		}
		client, file, err := driver.NewEndpointClientFromEnvironment(request.InvocationID)
		if err != nil {
			os.Exit(64)
		}
		defer file.Close()
		if _, err := client.Describe(); err != nil {
			os.Exit(64)
		}
		child := exec.Command(os.Args[0], "descendant")
		if err := child.Start(); err != nil {
			os.Exit(64)
		}
		go func() {
			_, _, _ = client.Submit(body)
		}()
		result, err := driver.RunFake(request, driver.FakeCompleted)
		if err != nil {
			os.Exit(64)
		}
		resultBody, err := driver.EncodeResult(result)
		if err != nil {
			os.Exit(64)
		}
		_, _ = os.Stdout.Write(resultBody)
		for {
			time.Sleep(time.Hour)
		}
	case "submit-no-result":
		body, err := base64.StdEncoding.Strict().DecodeString(scriptedSubmission)
		if err != nil {
			os.Exit(64)
		}
		client, file, err := driver.NewEndpointClientFromEnvironment(request.InvocationID)
		if err != nil {
			os.Exit(64)
		}
		defer file.Close()
		if _, err := client.Describe(); err != nil {
			os.Exit(64)
		}
		_, _, _ = client.Submit(body)
	case "submit-exit-17":
		body, err := base64.StdEncoding.Strict().DecodeString(scriptedSubmission)
		if err != nil {
			os.Exit(64)
		}
		signal.Ignore(syscall.SIGTERM)
		client, file, err := driver.NewEndpointClientFromEnvironment(request.InvocationID)
		if err != nil {
			os.Exit(64)
		}
		defer file.Close()
		if _, err := client.Describe(); err != nil {
			os.Exit(64)
		}
		submitted := make(chan error, 1)
		go func() {
			_, _, err := client.Submit(body)
			submitted <- err
		}()
		result, err := driver.RunFake(request, driver.FakeCompleted)
		if err != nil {
			os.Exit(64)
		}
		resultBody, err := driver.EncodeResult(result)
		if err != nil {
			os.Exit(64)
		}
		_, _ = os.Stdout.Write(resultBody)
		if <-submitted != nil {
			os.Exit(64)
		}
		os.Exit(17)
	case "malformed-control-block":
		file := os.NewFile(3, "sworn-submission-malformed-control")
		if file == nil {
			os.Exit(64)
		}
		controlBody, _ := json.Marshal(struct {
			SchemaVersion string `json:"schema_version"`
			Type          string `json:"type"`
			InvocationID  string `json:"invocation_id"`
		}{
			"sworn.submission-control/v1",
			"invalid",
			request.InvocationID,
		})
		if err := driver.WriteFrame(file, append(controlBody, '\n')); err != nil {
			os.Exit(64)
		}
		for {
			time.Sleep(time.Hour)
		}
	case "secret-text":
		if requestErr != nil {
			os.Exit(64)
		}
		result, err := driver.RunFake(request, driver.FakeCompleted)
		if err != nil {
			os.Exit(64)
		}
		body, err := driver.EncodeResult(result)
		if err != nil {
			os.Exit(64)
		}
		body = bytes.Replace(
			body,
			[]byte(`,"transport_status"`),
			[]byte(`,"text":"hostile-secret-sentinel","transport_status"`),
			1,
		)
		_, _ = os.Stdout.Write(body)
	case "wrong-binding":
		if requestErr != nil {
			os.Exit(64)
		}
		result, err := driver.RunFake(request, driver.FakeCompleted)
		if err != nil {
			os.Exit(64)
		}
		result.InvocationID = "wrong-invocation"
		body, err := driver.EncodeResult(result)
		if err == nil {
			_, _ = os.Stdout.Write(body)
			return
		}
		// EncodeResult deliberately validates the result, so emit a controlled
		// syntactically valid mutation for the parent binding check.
		valid, _ := driver.RunFake(request, driver.FakeCompleted)
		validBody, _ := driver.EncodeResult(valid)
		validBody = bytes.Replace(validBody,
			[]byte(request.InvocationID),
			[]byte("wrong-invocation"),
			1)
		_, _ = os.Stdout.Write(validBody)
	default:
		os.Exit(64)
	}
}

// reservedNamesMasked verifies, from inside the guest, that every workspace
// name the configured containment mask protects is an empty read-only surface:
// the masked directory is empty (the host tree is replaced by a private
// tmpfs) and a write into it is refused. This is the A3 proof that a worker
// cannot write to a configured records or journals root.
func reservedNamesMasked(request driver.Request, reserved []string) bool {
	if request.Workspace.Path != driver.GuestWorkspacePath {
		return false
	}
	for _, name := range reserved {
		if name == "" || name == "." || name == ".." || strings.ContainsRune(name, '/') {
			return false
		}
		target := filepath.Join(request.Workspace.Path, name)
		entries, err := os.ReadDir(target)
		if err != nil || len(entries) != 0 {
			return false
		}
		probe := filepath.Join(target, "canary")
		if err := os.WriteFile(probe, []byte("forged\n"), 0o600); err == nil {
			return false
		}
		if _, err := os.Stat(probe); err == nil {
			return false
		}
	}
	return true
}

func isIsolated(request driver.Request) bool {
	if request.Workspace.Path != driver.GuestWorkspacePath {
		return false
	}
	expectedEnvironment := map[string]string{
		"HOME":                               "/home/sworn",
		"TMPDIR":                             "/tmp",
		"LANG":                               "C.UTF-8",
		"LC_ALL":                             "C.UTF-8",
		"TZ":                                 "UTC",
		"PATH":                               "/usr/bin:/bin",
		"PWD":                                request.Workspace.Path,
		driver.SubmissionProtocolEnvironment: driver.SubmissionControlVersion,
		driver.SubmissionFDEnvironment:       "3",
		"BATON_FAKE_PROFILE":                 string(driver.FakeCompleted),
	}
	if len(os.Environ()) != len(expectedEnvironment) {
		return false
	}
	for key, want := range expectedEnvironment {
		if os.Getenv(key) != want {
			return false
		}
	}
	if _, err := os.Stat("/proc/self/mountinfo"); !os.IsNotExist(err) {
		return false
	}
	if body, err := os.ReadFile(filepath.Join(request.Workspace.Path, ".git")); err == nil &&
		bytes.Contains(body, []byte("git-canary")) {
		return false
	}
	// The fake driver verifies the default mask (records .baton and journals
	// .sworn) worked inside the guest for the uncontained test harness. The
	// configured-root mask that follows a relocated records/journals root is
	// proven by the driver's containment-mask unit test against
	// bubblewrapArguments; the fake driver runs only the default harness.
	for _, reserved := range []string{".baton", ".sworn"} {
		entries, err := os.ReadDir(filepath.Join(request.Workspace.Path, reserved))
		if err == nil && len(entries) != 0 {
			return false
		}
	}
	if file, err := os.OpenFile(
		filepath.Join(request.Workspace.Path, "workspace-canary"),
		os.O_WRONLY,
		0,
	); err == nil {
		_ = file.Close()
		return false
	}
	for _, input := range request.Inputs {
		target := filepath.Join(driver.GuestInputPath, filepath.FromSlash(input.Path))
		body, err := os.ReadFile(target)
		if err != nil || driver.Digest(body) != input.Digest {
			return false
		}
		file, err := os.OpenFile(target, os.O_WRONLY, 0)
		if err == nil {
			_ = file.Close()
			return false
		}
	}
	return true
}
