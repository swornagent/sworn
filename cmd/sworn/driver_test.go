package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
)

func TestDriverReadinessCLIIsClosedDeterministicAndFailClosed(t *testing.T) {
	t.Setenv("SWORN_TEST_OPENAI_KEY", "")
	configPath := driverCLIConfigFixture(t)

	var inspectOut, inspectErr bytes.Buffer
	if code := run([]string{
		"driver", "inspect",
		"--model", "model-one",
		"--config", configPath,
		"--json",
		"--profile", "openai",
	}, &inspectOut, &inspectErr); code != 0 {
		t.Fatalf(
			"inspect = %d, stdout=%q stderr=%q",
			code,
			inspectOut.String(),
			inspectErr.String(),
		)
	}
	var output driverReadinessOutput
	if err := json.Unmarshal(inspectOut.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if output.SchemaVersion != driverReadinessSchemaVersion ||
		output.Command != "inspect" ||
		len(output.Reports) != 1 ||
		output.Reports[0].Profile != "openai" ||
		output.Reports[0].Model != "model-one" ||
		output.Reports[0].Family != driver.ProfileOpenAIHTTP ||
		output.Reports[0].State != driver.ReadinessPass ||
		inspectErr.Len() != 0 {
		t.Fatalf("inspect output = %#v, stderr=%q", output, inspectErr.String())
	}

	var missingOut, missingErr bytes.Buffer
	if code := run([]string{
		"driver", "doctor",
		"--config", configPath,
		"--profile", "openai",
		"--model", "model-not-configured",
		"--json",
	}, &missingOut, &missingErr); code != 1 {
		t.Fatalf("missing model = %d, stderr=%q", code, missingErr.String())
	}
	output = driverReadinessOutput{}
	if err := json.Unmarshal(missingOut.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Reports) != 1 ||
		output.Reports[0].State != driver.ReadinessNotCertified ||
		output.Reports[0].Code != "model_not_configured" ||
		missingErr.Len() != 0 {
		t.Fatalf("missing model output = %#v, stderr=%q", output, missingErr.String())
	}

	var certifyOut, certifyErr bytes.Buffer
	if code := run([]string{
		"driver", "certify",
		"--config", configPath,
		"--profile", "openai",
		"--model", "model-one",
		"--json",
	}, &certifyOut, &certifyErr); code != 1 {
		t.Fatalf("uncertified = %d, stderr=%q", code, certifyErr.String())
	}
	output = driverReadinessOutput{}
	if err := json.Unmarshal(certifyOut.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Reports) != 1 ||
		output.Reports[0].State != driver.ReadinessFail ||
		output.Reports[0].Code != "certification_credential_failed" ||
		certifyErr.Len() != 0 {
		t.Fatalf("certify output = %#v, stderr=%q", output, certifyErr.String())
	}
}

func TestDriverReadinessCLIRejectsMalformedShapeBeforeIO(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"driver", "inspect", "--config", "/blocking", "--all"},
		{
			"driver", "inspect", "--config", "/blocking",
			"--all", "--profile", "openai", "--model", "model", "--json",
		},
		{
			"driver", "inspect", "--config", "/blocking",
			"--profile", "openai", "--json",
		},
		{
			"driver", "unknown", "--config", "/blocking",
			"--all", "--json",
		},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Fatalf("run(%v) = %d", args, code)
		}
		if stdout.Len() != 0 ||
			!bytes.HasPrefix(stderr.Bytes(), []byte("usage: sworn driver ")) ||
			bytes.Contains(stderr.Bytes(), []byte("/blocking")) {
			t.Fatalf(
				"run(%v) stdout=%q stderr=%q",
				args,
				stdout.String(),
				stderr.String(),
			)
		}
	}
}

func driverCLIConfigFixture(t *testing.T) string {
	t.Helper()
	credential := "openai-environment"
	body, err := driver.EncodeDriverConfig(driver.DriverConfig{
		SchemaVersion: driver.DriverConfigSchemaVersion,
		Credentials: []driver.DriverCredentialSource{{
			Key:       credential,
			Kind:      driver.CredentialEnvironment,
			Reference: "SWORN_TEST_OPENAI_KEY",
		}},
		Adapters: []driver.DriverAdapterConfig{{
			OpenAI: &driver.OpenAIProfileConfig{
				HTTPProfileConfig: driver.HTTPProfileConfig{
					Key:              "openai-adapter",
					ID:               "sworn.openai",
					Version:          "1.0.0",
					Endpoint:         "https://example.invalid/v1/responses",
					CredentialHeader: "Authorization",
					CredentialPrefix: "Bearer ",
					CredentialRefs:   []string{credential},
					ResponseBytes:    driver.MaxProviderResponseBytes,
				},
				API:             driver.OpenAIResponsesAPI,
				ReasoningEffort: "medium",
			},
		}},
		Profiles: []driver.DriverProfile{{
			Key:                 "openai",
			Adapter:             "openai-adapter",
			Network:             driver.NetworkRequired,
			CredentialSource:    &credential,
			CertificationModels: []string{"model-one"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pathValue := filepath.Join(t.TempDir(), "drivers.json")
	if err := os.WriteFile(pathValue, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return pathValue
}

// sworn#267: a registry-build failure that is not an unknown profile - here
// --all's all-families production requirement against a single-profile
// config - must not masquerade as "profile not found"; the operator would
// debug the wrong thing.
func TestDriverAllRegistryFailureIsNotReportedAsProfileNotFound(t *testing.T) {
	t.Setenv("SWORN_TEST_OPENAI_KEY", "")
	configPath := driverCLIConfigFixture(t)

	var stdout, stderr bytes.Buffer
	if code := run([]string{
		"driver", "doctor", "--config", configPath, "--all", "--json",
	}, &stdout, &stderr); code != 1 {
		t.Fatalf("doctor --all = %d, want 1; stderr=%q", code, stderr.String())
	}
	message := stderr.String()
	if strings.Contains(message, "Could not find that profile and model") {
		t.Fatalf("registry-build failure misreported as profile not found: %s", message)
	}
	if !strings.Contains(message, "could not be built into a driver registry") ||
		!strings.Contains(message, "MISSING_PROFILE_FAMILY") {
		t.Fatalf("registry-build failure not reported honestly with its code: %s", message)
	}
	if !strings.Contains(message, "missing families:") ||
		!strings.Contains(
			message,
			"--all checks the complete production roster while --profile P --model M checks one lane",
		) {
		t.Fatalf("registry-build failure did not name the missing roster members: %s", message)
	}
}

func driverProbeConfigFixture(t *testing.T, endpoint string) string {
	t.Helper()
	credential := "probe-environment"
	body, err := driver.EncodeDriverConfig(driver.DriverConfig{
		SchemaVersion: driver.DriverConfigSchemaVersion,
		Credentials: []driver.DriverCredentialSource{{
			Key:       credential,
			Kind:      driver.CredentialEnvironment,
			Reference: "SWORN_TEST_PROBE_KEY",
		}},
		Adapters: []driver.DriverAdapterConfig{{
			OpenAI: &driver.OpenAIProfileConfig{
				HTTPProfileConfig: driver.HTTPProfileConfig{
					Key:              "probe-adapter",
					ID:               "sworn.probe",
					Version:          "1.0.0",
					Endpoint:         endpoint,
					CredentialHeader: "Authorization",
					CredentialPrefix: "Bearer ",
					CredentialRefs:   []string{credential},
					ResponseBytes:    driver.MaxProviderResponseBytes,
				},
				API: driver.OpenAIChatCompletionsAPI,
			},
		}},
		Profiles: []driver.DriverProfile{{
			Key:                 "probe",
			Adapter:             "probe-adapter",
			Network:             driver.NetworkRequired,
			CredentialSource:    &credential,
			CertificationModels: []string{"model-one"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pathValue := filepath.Join(t.TempDir(), "drivers.json")
	if err := os.WriteFile(pathValue, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return pathValue
}

// TestDriverProbeCLIRejectsMalformedShapeBeforeIO pins the probe command's
// usage shape (--config, --profile and --model all required, --all never
// admitted) without ever touching the named path.
func TestDriverProbeCLIRejectsMalformedShapeBeforeIO(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"driver", "probe", "--config", "/blocking"},
		{"driver", "probe", "--config", "/blocking", "--profile", "p"},
		{"driver", "probe", "--config", "/blocking", "--model", "m"},
		{"driver", "probe", "--config", "/blocking", "--profile", "p", "--model", "m", "--all"},
		{"driver", "probe", "--profile", "p", "--model", "m"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Fatalf("run(%v) = %d", args, code)
		}
		if stdout.Len() != 0 ||
			!bytes.HasPrefix(stderr.Bytes(), []byte("usage: sworn driver probe ")) ||
			bytes.Contains(stderr.Bytes(), []byte("/blocking")) {
			t.Fatalf("run(%v) stdout=%q stderr=%q", args, stdout.String(), stderr.String())
		}
	}
}

// TestDriverProbeCLIReportsReadyOnLiveResponseAndRefusalOtherwise pins A1:
// the probe sends one minimal live request and reports ready or a closed
// refusal code, in both the JSON and the default human-readable form.
func TestDriverProbeCLIReportsReadyOnLiveResponseAndRefusalOtherwise(t *testing.T) {
	t.Setenv("SWORN_TEST_PROBE_KEY", "probe-secret-canary")
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		body, _ := io.ReadAll(request.Body)
		if bytes.Contains(body, []byte("tools")) {
			t.Errorf("probe request carried a tools field: %s", body)
		}
		if bytes.Contains(body, []byte("probe-secret-canary")) {
			t.Errorf("probe request leaked the credential")
		}
		writer.Header().Set("X-Request-Id", "cli-probe-request-id")
		writer.WriteHeader(status)
		_, _ = writer.Write([]byte(`{"id":"resp-id","choices":[]}`))
	}))
	defer server.Close()
	configPath := driverProbeConfigFixture(t, server.URL+"/v1/chat/completions")

	var readyOut, readyErr bytes.Buffer
	code := run([]string{
		"driver", "probe", "--config", configPath,
		"--profile", "probe", "--model", "model-one", "--json",
	}, &readyOut, &readyErr)
	if code != 0 || readyErr.Len() != 0 {
		t.Fatalf("ready probe = %d, stdout=%q stderr=%q", code, readyOut.String(), readyErr.String())
	}
	var output driverProbeOutput
	if err := json.Unmarshal(readyOut.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if output.SchemaVersion != driverProbeSchemaVersion ||
		!output.Ready || output.Code != "live_probe_passed" ||
		output.RequestID != "cli-probe-request-id" || !output.LiveCall ||
		output.LatencyMillis < 0 {
		t.Fatalf("ready probe output = %#v", output)
	}

	var textOut, textErr bytes.Buffer
	if code := run([]string{
		"driver", "probe", "--config", configPath,
		"--profile", "probe", "--model", "model-one",
	}, &textOut, &textErr); code != 0 || textErr.Len() != 0 {
		t.Fatalf("ready text probe = %d, stdout=%q stderr=%q", code, textOut.String(), textErr.String())
	}
	if !strings.Contains(textOut.String(), "admitting requests") ||
		!strings.Contains(textOut.String(), "Technical code: live_probe_passed") {
		t.Fatalf("ready text output = %q", textOut.String())
	}

	status = http.StatusUnauthorized
	var refusedOut, refusedErr bytes.Buffer
	code = run([]string{
		"driver", "probe", "--config", configPath,
		"--profile", "probe", "--model", "model-one", "--json",
	}, &refusedOut, &refusedErr)
	if code != 1 || refusedErr.Len() != 0 {
		t.Fatalf("refused probe = %d, stdout=%q stderr=%q", code, refusedOut.String(), refusedErr.String())
	}
	output = driverProbeOutput{}
	if err := json.Unmarshal(refusedOut.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if output.Ready || output.Code != "certification_provider_authorization_failed" {
		t.Fatalf("refused probe output = %#v", output)
	}
}

// TestDriverProbeCLIUnknownProfileIsReportedAsNotFound pins the same
// sworn#267 honesty the other driver commands keep: an unrecognized
// profile is reported as "not found", not misreported as a registry
// build failure or a probe transport failure.
func TestDriverProbeCLIUnknownProfileIsReportedAsNotFound(t *testing.T) {
	t.Setenv("SWORN_TEST_PROBE_KEY", "probe-secret-canary")
	configPath := driverProbeConfigFixture(t, "https://example.invalid/v1/chat/completions")
	var stdout, stderr bytes.Buffer
	if code := run([]string{
		"driver", "probe", "--config", configPath,
		"--profile", "unknown-profile", "--model", "model-one", "--json",
	}, &stdout, &stderr); code != 1 {
		t.Fatalf("unknown profile probe = %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Could not find that profile and model") {
		t.Fatalf("unknown profile probe stderr = %q", stderr.String())
	}
}
