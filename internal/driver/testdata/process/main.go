package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
				request.Workspace.Path,
				filepath.FromSlash(input.Path),
			))
			if err != nil || driver.Digest(body) != input.Digest {
				os.Exit(64)
			}
			var script struct {
				Behavior string `json:"behavior"`
			}
			if err := json.Unmarshal(body, &script); err != nil {
				os.Exit(64)
			}
			behavior = script.Behavior
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
		_, _ = os.Stdout.Write(bytes.Repeat([]byte{'x'}, driver.MaxStdoutBytes+1))
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
	case "block-descendant":
		child := exec.Command(os.Args[0], "descendant")
		if err := child.Start(); err != nil {
			os.Exit(64)
		}
		for {
			time.Sleep(time.Hour)
		}
	case "secret-text":
		if requestErr != nil {
			os.Exit(64)
		}
		result, err := driver.RunFake(request, driver.FakeCompleted, false)
		if err != nil {
			os.Exit(64)
		}
		result.Text = "hostile-secret-sentinel"
		body, err := driver.EncodeResult(result)
		if err != nil {
			os.Exit(64)
		}
		_, _ = os.Stdout.Write(body)
	case "wrong-binding":
		if requestErr != nil {
			os.Exit(64)
		}
		result, err := driver.RunFake(request, driver.FakeCompleted, false)
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
		valid, _ := driver.RunFake(request, driver.FakeCompleted, false)
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

func isIsolated(request driver.Request) bool {
	expectedEnvironment := map[string]string{
		"HOME":                               "/home/sworn",
		"TMPDIR":                             "/tmp",
		"LANG":                               "C.UTF-8",
		"LC_ALL":                             "C.UTF-8",
		"TZ":                                 "UTC",
		"PATH":                               "/usr/bin:/bin",
		"PWD":                                request.Workspace.Path,
		driver.SubmissionProtocolEnvironment: driver.SubmissionProtocolID,
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
	if _, err := os.Stat("/home/brad/projects/sworn"); !os.IsNotExist(err) {
		return false
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
		target := filepath.Join(request.Workspace.Path, filepath.FromSlash(input.Path))
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
	descriptors, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return false
	}
	for _, descriptor := range descriptors {
		number, err := strconv.Atoi(descriptor.Name())
		if err != nil || number <= 3 {
			continue
		}
		target, err := os.Readlink(filepath.Join("/proc/self/fd", descriptor.Name()))
		if err == nil && strings.HasPrefix(target, "/") {
			return false
		}
	}
	return true
}
