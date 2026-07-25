package driver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	FakeDriverID      = "baton.fake"
	FakeDriverVersion = "1.0.0"
)

type FakeProfile string

const (
	FakeCompleted      FakeProfile = "completed"
	FakeTransportError FakeProfile = "transport_error"
	FakeTimeout        FakeProfile = "timeout"
	FakeCancelled      FakeProfile = "cancelled"
	FakeRunnerError    FakeProfile = "runner_error"
)

func (profile FakeProfile) valid() bool {
	switch profile {
	case FakeCompleted, FakeTransportError, FakeTimeout, FakeCancelled, FakeRunnerError:
		return true
	default:
		return false
	}
}

func FakeInfo() DriverInfo {
	return DriverInfo{
		ContractVersion: DriverContractVersion,
		DriverID:        FakeDriverID,
		DriverVersion:   FakeDriverVersion,
	}
}

func RunFake(request Request, profile FakeProfile, emptyText bool) (Result, error) {
	if err := ValidateRequest(request); err != nil {
		return Result{}, err
	}
	if !profile.valid() {
		return Result{}, fail("INVALID_PROFILE")
	}
	messages := map[FakeProfile]string{
		FakeCompleted:      "Fake completed response for " + string(request.Role) + ".",
		FakeTransportError: "Fake transport error.",
		FakeTimeout:        "Fake timeout.",
		FakeCancelled:      "Fake cancellation.",
		FakeRunnerError:    "Fake runner error.",
	}
	text := messages[profile]
	if emptyText && profile == FakeCompleted {
		text = ""
	}
	if int64(len(text)) > request.Limits.OutputBytes {
		text = text[:request.Limits.OutputBytes]
	}
	result := Result{
		SchemaVersion:   ResultSchemaVersion,
		InvocationID:    request.InvocationID,
		DriverID:        FakeDriverID,
		DriverVersion:   FakeDriverVersion,
		ObservedModel:   cloneString(request.Model),
		DurationMillis:  0,
		Text:            text,
		TransportStatus: TransportStatus(profile),
	}
	if profile == FakeCompleted {
		result.Usage = &Usage{InputTokens: 0, OutputTokens: 0}
	}
	if err := ValidateResult(result, ResultBinding{
		InvocationID:  request.InvocationID,
		DriverID:      FakeDriverID,
		DriverVersion: FakeDriverVersion,
	}); err != nil {
		return Result{}, err
	}
	return result, nil
}

type fakeScript struct {
	SchemaVersion string `json:"schema_version"`
	Behavior      string `json:"behavior"`
	Submission    string `json:"submission,omitempty"`
}

func readFakeScript(request Request) (fakeScript, bool, error) {
	for _, input := range request.Inputs {
		if input.Name != "fake-script" {
			continue
		}
		target := filepath.Join(request.Workspace.Path, filepath.FromSlash(input.Path))
		if !pathBeneath(request.Workspace.Path, target) {
			return fakeScript{}, false, fail("INVALID_FAKE_SCRIPT")
		}
		info, err := os.Lstat(target)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fakeScript{}, false, fail("INVALID_FAKE_SCRIPT")
		}
		body, err := os.ReadFile(target)
		if err != nil || Digest(body) != input.Digest {
			return fakeScript{}, false, fail("INVALID_FAKE_SCRIPT")
		}
		value, err := decodeStrict(body, 2_097_152)
		if err != nil {
			return fakeScript{}, false, err
		}
		root, err := closedObject(value,
			[]string{"schema_version", "behavior"}, []string{"submission"})
		if err != nil {
			return fakeScript{}, false, err
		}
		var script fakeScript
		if script.SchemaVersion, err = requiredString(root, "schema_version"); err != nil {
			return fakeScript{}, false, err
		}
		if script.Behavior, err = requiredString(root, "behavior"); err != nil {
			return fakeScript{}, false, err
		}
		if _, present := root["submission"]; present {
			if script.Submission, err = requiredString(root, "submission"); err != nil {
				return fakeScript{}, false, err
			}
		}
		canonical, err := json.Marshal(script)
		if err != nil || !bytes.Equal(append(canonical, '\n'), body) {
			return fakeScript{}, false, fail("NONCANONICAL_JSON")
		}
		switch script.Behavior {
		case "none", "empty_text", "submit", "block", "attempt_workspace_write", "malformed_submission_frame":
		default:
			return fakeScript{}, false, fail("INVALID_FAKE_SCRIPT")
		}
		if script.Behavior == "submit" && script.Submission == "" {
			return fakeScript{}, false, fail("INVALID_FAKE_SCRIPT")
		}
		if script.Behavior != "submit" && script.Submission != "" {
			return fakeScript{}, false, fail("INVALID_FAKE_SCRIPT")
		}
		return script, true, nil
	}
	return fakeScript{}, false, nil
}

func executeFakeScript(request Request, script fakeScript) (bool, error) {
	switch script.Behavior {
	case "none":
		return false, nil
	case "empty_text":
		return true, nil
	case "submit":
		body, err := base64.StdEncoding.Strict().DecodeString(script.Submission)
		if err != nil || base64.StdEncoding.EncodeToString(body) != script.Submission {
			return false, fail("INVALID_FAKE_SCRIPT")
		}
		client, file, err := NewEndpointClientFromEnvironment(request.InvocationID)
		if err != nil {
			return false, err
		}
		defer file.Close()
		descriptor, err := client.Describe()
		if err != nil || descriptor.InvocationID != request.InvocationID {
			return false, fail("INVALID_ENDPOINT")
		}
		if _, _, err := client.Submit(body); err != nil {
			return false, err
		}
	case "block":
		for {
			time.Sleep(time.Hour)
		}
	case "attempt_workspace_write":
		if err := os.WriteFile(filepath.Join(request.Workspace.Path, ".sworn-fake-write-canary"), []byte("write\n"), 0o600); err != nil {
			return false, fail("FAKE_WRITE_REFUSED")
		}
	case "malformed_submission_frame":
		file := os.NewFile(3, "sworn-submission-malformed")
		if file == nil {
			return false, fail("INVALID_ENDPOINT")
		}
		defer file.Close()
		if _, err := file.Write([]byte{0, 0, 0, 0}); err != nil {
			return false, fail("FRAME_WRITE_FAILED")
		}
	}
	return false, nil
}

// RunFakeCommand is the one implementation used by the test-built executable
// and future packaging. It supports exactly `info` and `run`.
func RunFakeCommand(
	arguments []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	profile FakeProfile,
) int {
	report := func(code string) int {
		_, _ = io.WriteString(stderr, code+": driver protocol rejected\n")
		return 64
	}
	if len(arguments) == 1 && arguments[0] == "info" {
		body, err := EncodeDriverInfo(FakeInfo())
		if err != nil {
			return report("DRIVER_FAILED")
		}
		_, _ = stdout.Write(body)
		return 0
	}
	if len(arguments) != 1 || arguments[0] != "run" {
		return report("INVALID_INVOCATION")
	}
	body, err := io.ReadAll(io.LimitReader(stdin, MaxRequestBytes+1))
	if err != nil {
		return report("INVALID_JSON")
	}
	request, err := DecodeRequest(body)
	if err != nil {
		return report(contractCode(err))
	}
	script, present, err := readFakeScript(request)
	if err != nil {
		return report(contractCode(err))
	}
	emptyText := false
	if present {
		emptyText, err = executeFakeScript(request, script)
		if err != nil {
			return report(contractCode(err))
		}
	}
	result, err := RunFake(request, profile, emptyText)
	if err != nil {
		return report(contractCode(err))
	}
	resultBody, err := EncodeResult(result)
	if err != nil {
		return report(contractCode(err))
	}
	_, _ = stdout.Write(resultBody)
	return 0
}

func contractCode(err error) string {
	var contractErr *ContractError
	if errors.As(err, &contractErr) {
		return contractErr.Code
	}
	return "DRIVER_FAILED"
}
