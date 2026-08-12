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
	return profile == FakeCompleted || profile == FakeTransportError ||
		profile == FakeTimeout || profile == FakeCancelled || profile == FakeRunnerError
}

// EffectiveWorkspacePath resolves the guest workspace path through the
// engine-set test-only override when an uncontained dispatch is active, and
// otherwise returns the canonical guest workspace path unchanged. The override
// is present only in the controlled environment the engine builds for the
// gate-linked uncontained dispatch branch; the contained path never carries it.
func EffectiveWorkspacePath(guest string) string {
	if override := os.Getenv(testUncontainedGuestWorkspaceEnv); override != "" {
		return override
	}
	return guest
}

// EffectiveInputPath resolves the guest input projection root through the
// engine-set test-only override when an uncontained dispatch is active, and
// otherwise returns the canonical guest input path unchanged.
func EffectiveInputPath() string {
	if override := os.Getenv(testUncontainedGuestInputsEnv); override != "" {
		return override
	}
	return GuestInputPath
}

// UncontainedDispatchMarker reports whether this process is executing inside a
// gate-linked uncontained dispatch. The marker travels only in the controlled
// environment the engine builds for that branch, so it can never be set in the
// contained path.
func UncontainedDispatchMarker() bool {
	return os.Getenv(testUncontainedDispatchEnv) == "1"
}
func FakeInfo() DriverInfo {
	return DriverInfo{
		ContractVersion: DriverContractVersion,
		AdapterID:       FakeDriverID,
		AdapterVersion:  FakeDriverVersion,
	}
}
func RunFake(request Request, profile FakeProfile) (Result, error) {
	if err := ValidateRequest(request); err != nil {
		return Result{}, err
	}
	if !profile.valid() {
		return Result{}, fail("INVALID_PROFILE")
	}
	result := Result{
		SchemaVersion:   ResultSchemaVersion,
		InvocationID:    request.InvocationID,
		AdapterID:       FakeDriverID,
		AdapterVersion:  FakeDriverVersion,
		ObservedModel:   request.Model,
		DurationMillis:  0,
		TransportStatus: TransportStatus(profile),
	}
	if profile == FakeCompleted {
		result.Usage = &Usage{InputTokens: 0, OutputTokens: 0}
	}
	if err := ValidateResult(result, ResultBinding{
		InvocationID:   request.InvocationID,
		AdapterID:      FakeDriverID,
		AdapterVersion: FakeDriverVersion,
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
		inputsRoot := EffectiveInputPath()
		target := filepath.Join(inputsRoot, filepath.FromSlash(input.Path))
		if !pathBeneath(inputsRoot, target) {
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
		var script fakeScript
		if _, err := decodeTyped(
			body,
			2_097_152,
			[]string{"schema_version", "behavior"},
			[]string{"submission"},
			&script,
		); err != nil {
			return fakeScript{}, false, err
		}
		if script.SchemaVersion != "sworn.fake-script/v1" {
			return fakeScript{}, false, fail("INVALID_VERSION")
		}
		canonical, err := json.Marshal(script)
		if err != nil || !bytes.Equal(append(canonical, '\n'), body) {
			return fakeScript{}, false, fail("NONCANONICAL_JSON")
		}
		switch script.Behavior {
		case "none", "usage_unavailable", "submit", "block", "attempt_workspace_write", "malformed_submission_frame":
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
func executeFakeScript(request Request, script fakeScript) (<-chan error, error) {
	switch script.Behavior {
	case "none":
		return nil, nil
	case "usage_unavailable":
		return nil, nil
	case "submit":
		body, err := base64.StdEncoding.Strict().DecodeString(script.Submission)
		if err != nil || base64.StdEncoding.EncodeToString(body) != script.Submission {
			return nil, fail("INVALID_FAKE_SCRIPT")
		}
		client, file, err := NewEndpointClientFromEnvironment(request.InvocationID)
		if err != nil {
			return nil, err
		}
		descriptor, err := client.Describe()
		if err != nil || descriptor.InvocationID != request.InvocationID {
			_ = file.Close()
			return nil, fail("INVALID_ENDPOINT")
		}
		done := make(chan error, 1)
		go func() {
			defer file.Close()
			_, _, submitErr := client.Submit(body)
			done <- submitErr
		}()
		return done, nil
	case "block":
		for {
			time.Sleep(time.Hour)
		}
	case "attempt_workspace_write":
		if err := os.WriteFile(filepath.Join(
			EffectiveWorkspacePath(request.Workspace.Path),
			".sworn-fake-write-canary",
		), []byte("write\n"), 0o600); err != nil {
			return nil, fail("FAKE_WRITE_REFUSED")
		}
	case "malformed_submission_frame":
		file := os.NewFile(3, "sworn-submission-malformed")
		if file == nil {
			return nil, fail("INVALID_ENDPOINT")
		}
		defer file.Close()
		if _, err := file.Write([]byte{0, 0, 0, 0}); err != nil {
			return nil, fail("FRAME_WRITE_FAILED")
		}
	}
	return nil, nil
}

// RunFakeCommand supports exactly the info and run protocol operations.
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
	var submitDone <-chan error
	if present {
		submitDone, err = executeFakeScript(request, script)
		if err != nil {
			return report(contractCode(err))
		}
	}
	result, err := RunFake(request, profile)
	if err != nil {
		return report(contractCode(err))
	}
	if present && script.Behavior == "usage_unavailable" {
		result.Usage = nil
	}
	resultBody, err := EncodeResult(result)
	if err != nil {
		return report(contractCode(err))
	}
	_, _ = stdout.Write(resultBody)
	if submitDone != nil {
		if err := <-submitDone; err != nil && !IsCode(err, "SUBMISSION_REJECTED") {
			return report(contractCode(err))
		}
	}
	return 0
}
func contractCode(err error) string {
	var contractErr *ContractError
	if errors.As(err, &contractErr) {
		return contractErr.Code
	}
	return "DRIVER_FAILED"
}
