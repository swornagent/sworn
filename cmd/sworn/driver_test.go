package main

import (
	"bytes"
	"encoding/json"
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
}
