package driver

import (
	"bytes"
	"encoding/json"
	"testing"
)

type adapterTestHarness struct {
	info   DriverInfo
	invoke func(arguments []string, input []byte, profile FakeProfile) (int, []byte, []byte)
}

// runAdapterConformance is intentionally test-only and case-ID stable. W5
// adapter tests can call the same harness without changing the W2 contract.
func runAdapterConformance(t *testing.T, harness adapterTestHarness) {
	t.Helper()
	t.Run("A-W2-info-exact", func(t *testing.T) {
		exit, stdout, stderr := harness.invoke([]string{"info"}, nil, FakeCompleted)
		info, err := DecodeDriverInfo(stdout, DriverInfoBinding{
			AdapterID:      harness.info.AdapterID,
			AdapterVersion: harness.info.AdapterVersion,
		})
		if exit != 0 || len(stderr) != 0 || err != nil || info != harness.info {
			t.Fatalf("exit=%d stdout=%q stderr=%q", exit, stdout, stderr)
		}
	})
	t.Run("A-W2-four-role-common-process", func(t *testing.T) {
		for _, role := range []Role{
			RolePlanner, RoleImplementer, RoleCaptain, RoleVerifier,
		} {
			request := contractRequest(t, role)
			request.InvocationID = "conformance-" + string(role)
			request.Model = "explicit-" + string(role)
			body, err := EncodeRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			exit, stdout, stderr := harness.invoke([]string{"run"}, body, FakeCompleted)
			if exit != 0 || len(stderr) != 0 {
				t.Fatalf("%s exit=%d stderr=%q", role, exit, stderr)
			}
			result, err := DecodeResult(stdout, ResultBinding{
				InvocationID:   request.InvocationID,
				AdapterID:      harness.info.AdapterID,
				AdapterVersion: harness.info.AdapterVersion,
				Model:          request.Model,
				BindModel:      true,
			})
			if err != nil {
				t.Fatalf("%s result=%#v error=%v", role, result, err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(stdout, &fields); err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{
				"text", "transcript", "outcome", "verdict", "action", "fallback_model",
			} {
				if _, exists := fields[forbidden]; exists {
					t.Fatalf("%s exposed %s", role, forbidden)
				}
			}
		}
		mergeBody := bytes.Replace(
			func() []byte {
				body, err := EncodeRequest(contractRequest(t, RolePlanner))
				if err != nil {
					t.Fatal(err)
				}
				return body
			}(),
			[]byte(`"role":"planner"`),
			[]byte(`"role":"merge"`),
			1,
		)
		exit, _, _ := harness.invoke([]string{"run"}, mergeBody, FakeCompleted)
		if exit == 0 {
			t.Fatal("Merge wire role was dispatched")
		}
	})
	t.Run("A-W2-five-transport-statuses", func(t *testing.T) {
		request := contractRequest(t, RolePlanner)
		body, err := EncodeRequest(request)
		if err != nil {
			t.Fatal(err)
		}
		for _, profile := range []FakeProfile{
			FakeCompleted, FakeTransportError, FakeTimeout, FakeCancelled, FakeRunnerError,
		} {
			exit, stdout, stderr := harness.invoke([]string{"run"}, body, profile)
			if exit != 0 || len(stderr) != 0 {
				t.Fatalf("%s exit=%d stderr=%q", profile, exit, stderr)
			}
			result, err := DecodeResult(stdout, ResultBinding{
				InvocationID:   request.InvocationID,
				AdapterID:      harness.info.AdapterID,
				AdapterVersion: harness.info.AdapterVersion,
			})
			if err != nil || result.TransportStatus != TransportStatus(profile) {
				t.Fatalf("%s result=%#v error=%v", profile, result, err)
			}
		}
	})
}

func TestProductionFakePassesSharedAdapterConformance(t *testing.T) {
	t.Parallel()
	runAdapterConformance(t, adapterTestHarness{
		info: FakeInfo(),
		invoke: func(arguments []string, input []byte, profile FakeProfile) (int, []byte, []byte) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exit := RunFakeCommand(arguments, bytes.NewReader(input), &stdout, &stderr, profile)
			return exit, stdout.Bytes(), stderr.Bytes()
		},
	})
}
