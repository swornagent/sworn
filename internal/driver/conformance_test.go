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
			DriverID:      harness.info.DriverID,
			DriverVersion: harness.info.DriverVersion,
		})
		if exit != 0 || len(stderr) != 0 || err != nil || info != harness.info {
			t.Fatalf("exit=%d stdout=%q stderr=%q", exit, stdout, stderr)
		}
	})
	t.Run("A-W2-five-role-common-process", func(t *testing.T) {
		for _, role := range []Role{
			RolePlanner, RoleImplementer, RoleCaptain, RoleVerifier, RoleMerge,
		} {
			model := pointer("explicit-" + string(role))
			if role == RoleMerge {
				model = nil
			}
			request := contractRequest(t, role, model)
			request.InvocationID = "conformance-" + string(role)
			body, err := EncodeRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			exit, stdout, stderr := harness.invoke([]string{"run"}, body, FakeCompleted)
			if exit != 0 || len(stderr) != 0 {
				t.Fatalf("%s exit=%d stderr=%q", role, exit, stderr)
			}
			result, err := DecodeResult(stdout, ResultBinding{
				InvocationID:  request.InvocationID,
				DriverID:      harness.info.DriverID,
				DriverVersion: harness.info.DriverVersion,
				Model:         model,
				BindModel:     true,
			})
			if err != nil || (role == RoleMerge && result.ObservedModel != nil) {
				t.Fatalf("%s result=%#v error=%v", role, result, err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(stdout, &fields); err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"outcome", "verdict", "action", "fallback_model"} {
				if _, exists := fields[forbidden]; exists {
					t.Fatalf("%s exposed %s", role, forbidden)
				}
			}
		}
	})
	t.Run("A-W2-five-transport-statuses", func(t *testing.T) {
		request := contractRequest(t, RolePlanner, pointer("explicit-model"))
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
				InvocationID:  request.InvocationID,
				DriverID:      harness.info.DriverID,
				DriverVersion: harness.info.DriverVersion,
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
