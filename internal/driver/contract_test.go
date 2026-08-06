package driver

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func pointer(value string) *string {
	return &value
}

func contractRequest(t *testing.T, role Role) Request {
	t.Helper()
	request, err := NewRequest(
		"invocation-001",
		role,
		"fake-profile",
		"fake-model-v1",
		Workspace{Path: "/workspace/project", Access: ReadOnly},
		[]Input{
			{
				Name:   "plan",
				Path:   "inputs/plan.md",
				Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
			{
				Name:   "checks",
				Path:   "inputs/checks.bin",
				Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			},
		},
		true,
		Limits{TimeoutMillis: 60_000, OutputBytes: 65_536},
	)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestCanonicalOperationsBindSwornOwnedRoleAssetsAndExcludeMerge(t *testing.T) {
	t.Parallel()
	_, identity, err := admittedPackage()
	if err != nil {
		t.Fatal(err)
	}
	if identity != (PackageIdentity{
		Version:        "sworn.role-assets/v1",
		ManifestSHA256: "sha256:3ee5d18eb6bc38bc3694bfe6ad12a6d45dac3586378f4f4fd572b560aaa9755e",
	}) {
		t.Fatalf("package identity = %#v", identity)
	}
	expected := map[Role]struct {
		id     string
		digest string
	}{
		RolePlanner: {
			"baton-plan",
			"sha256:443f8bbce2914f2586de8ae7796b346554097421742071e8494d459673b82760",
		},
		RoleImplementer: {
			"baton-implement",
			"sha256:c274017d47d9dd7bc86ff1188cab1b688f7df73500b3bacdb4244bf496c8c473",
		},
		RoleCaptain: {
			"baton-design-review",
			"sha256:ecfecf92a1858db9a27de6105ccf647f5a15ec85ed76a346072182e22e99a6d5",
		},
		RoleVerifier: {
			"baton-verify",
			"sha256:8ca4dff1ab2c607cd23ea2828daf11dc88a7dbeb3194229f2ff5c3c83f510014",
		},
	}
	for role, want := range expected {
		role := role
		want := want
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()
			operation, err := CanonicalOperation(role)
			if err != nil {
				t.Fatal(err)
			}
			if operation.ID != want.id || operation.Version != "baton.operation/v2" ||
				operation.Digest != want.digest ||
				Digest([]byte(operation.Instructions)) != want.digest {
				t.Fatalf("operation = %#v", operation)
			}
			if !strings.HasSuffix(operation.Instructions, "\n") ||
				strings.Contains(operation.Instructions, "\r") {
				t.Fatal("operation bytes are not exact LF-only bytes")
			}
		})
	}
	if _, err := CanonicalOperation(Role("merge")); !IsCode(err, "INVALID_ROLE") {
		t.Fatalf("Merge operation error = %v", err)
	}
}

func TestDriverInfoCodecIsSwornOwnedExactStrictAndBound(t *testing.T) {
	t.Parallel()
	info := FakeInfo()
	body, err := EncodeDriverInfo(info)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"contract_version":"sworn.driver/v1","adapter_id":"baton.fake","adapter_version":"1.0.0"}` + "\n"
	if string(body) != want {
		t.Fatalf("info = %q", body)
	}
	decoded, err := DecodeDriverInfo(body, DriverInfoBinding{
		AdapterID:      FakeDriverID,
		AdapterVersion: FakeDriverVersion,
	})
	if err != nil || decoded != info {
		t.Fatalf("decoded = %#v, error = %v", decoded, err)
	}
	extra := bytes.Replace(body,
		[]byte(`"adapter_version":"1.0.0"`),
		[]byte(`"adapter_version":"1.0.0","default_model":"forbidden"`),
		1,
	)
	if _, err := DecodeDriverInfo(extra, DriverInfoBinding{}); !IsCode(err, "UNKNOWN_FIELD") {
		t.Fatalf("unknown error = %v", err)
	}
	duplicate := bytes.Replace(body,
		[]byte(`"adapter_id":"baton.fake"`),
		[]byte(`"adapter_id":"baton.fake","adapter_id":"baton.fake"`),
		1,
	)
	if _, err := DecodeDriverInfo(duplicate, DriverInfoBinding{}); !IsCode(err, "DUPLICATE_NAME") {
		t.Fatalf("duplicate error = %v", err)
	}
	if _, err := DecodeDriverInfo(
		body,
		DriverInfoBinding{AdapterID: "other"},
	); !IsCode(err, "DRIVER_BINDING_MISMATCH") {
		t.Fatalf("binding error = %v", err)
	}
}

func TestRequestCodecIsStrictBoundedStableAndExplicit(t *testing.T) {
	t.Parallel()
	request := contractRequest(t, RoleVerifier)
	body, err := EncodeRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(
		body,
		[]byte(`{"schema_version":"sworn.driver-request/v1","invocation_id":"invocation-001","role":"verifier"`),
	) {
		t.Fatalf("unexpected canonical prefix: %.120q", body)
	}
	if !bytes.Contains(body, []byte(`"profile":"fake-profile","model":"fake-model-v1"`)) {
		t.Fatalf("request omits explicit profile/model: %s", body)
	}
	decoded, err := DecodeRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	reencoded, err := EncodeRequest(decoded)
	if err != nil || !bytes.Equal(body, reencoded) {
		t.Fatalf("request codec unstable: %v", err)
	}

	tests := map[string]struct {
		body []byte
		code string
	}{
		"duplicate": {
			bytes.Replace(
				body,
				[]byte(`"schema_version":"sworn.driver-request/v1"`),
				[]byte(`"schema_version":"sworn.driver-request/v1","schema_version":"sworn.driver-request/v1"`),
				1,
			),
			"DUPLICATE_NAME",
		},
		"trailing":        {append(append([]byte(nil), body...), []byte(`{}`)...), "TRAILING_JSON"},
		"invalid utf8":    {[]byte{'{', '"', 0xff, '"', '}'}, "INVALID_UTF8"},
		"invalid unicode": {[]byte(`{"schema_version":"sworn.driver-request/v1","invocation_id":"\ud800"}`), "INVALID_UNICODE"},
		"unknown field": {
			bytes.Replace(
				body,
				[]byte(`"fresh_context":true`),
				[]byte(`"fallback_model":"x","fresh_context":true`),
				1,
			),
			"UNKNOWN_FIELD",
		},
		"missing profile": {
			bytes.Replace(body, []byte(`"profile":"fake-profile",`), nil, 1),
			"MISSING_FIELD",
		},
		"missing model": {
			bytes.Replace(body, []byte(`"model":"fake-model-v1",`), nil, 1),
			"MISSING_FIELD",
		},
		"float timeout": {
			bytes.Replace(body, []byte(`"timeout_ms":60000`), []byte(`"timeout_ms":1.5`), 1),
			"INVALID_FIELD",
		},
		"unsafe integer": {
			bytes.Replace(
				body,
				[]byte(`"timeout_ms":60000`),
				[]byte(`"timeout_ms":9007199254740992`),
				1,
			),
			"INVALID_LIMIT",
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeRequest(test.body); !IsCode(err, test.code) {
				t.Fatalf("error = %v, want %s", err, test.code)
			}
		})
	}
	if _, err := DecodeRequest(
		bytes.Repeat([]byte{' '}, MaxRequestBytes+1),
	); !IsCode(err, "RESOURCE_LIMIT") {
		t.Fatalf("oversized error = %v", err)
	}
}

func TestRequestRejectsMergeDefaultsDriftAndUnsafeInputs(t *testing.T) {
	t.Parallel()
	request := contractRequest(t, RoleVerifier)
	stale := request
	stale.Operation.Instructions = "Replacement.\n"
	stale.Operation.Digest = Digest([]byte(stale.Operation.Instructions))
	if err := ValidateRequest(stale); !IsCode(err, "STALE_OPERATION") {
		t.Fatalf("stale error = %v", err)
	}
	wrongRole := request
	wrongRole.Role = RoleCaptain
	if err := ValidateRequest(wrongRole); !IsCode(err, "OPERATION_ROLE_MISMATCH") {
		t.Fatalf("role error = %v", err)
	}
	for name, mutate := range map[string]func(*Request){
		"missing profile": func(value *Request) { value.Profile = "" },
		"missing model":   func(value *Request) { value.Model = "" },
		"writable verifier": func(value *Request) {
			value.Workspace.Access = ReadWrite
		},
	} {
		value := request
		mutate(&value)
		if err := ValidateRequest(value); err == nil {
			t.Fatalf("%s request was accepted", name)
		}
	}
	merge := request
	merge.Role = Role("merge")
	merge.Operation.ID = "baton-merge"
	if err := ValidateRequest(merge); !IsCode(err, "INVALID_ROLE") {
		t.Fatalf("Merge wire role error = %v", err)
	}
	for _, reserved := range []string{".git/config", ".baton/plan.md", ".sworn/journal"} {
		value := request
		value.Inputs = append([]Input(nil), request.Inputs...)
		value.Inputs[0].Path = reserved
		if err := ValidateRequest(value); !IsCode(err, "INVALID_PATH") {
			t.Fatalf("reserved input %q error = %v", reserved, err)
		}
	}
	duplicate := request
	duplicate.Inputs = append([]Input(nil), request.Inputs...)
	duplicate.Inputs[1].Name = duplicate.Inputs[0].Name
	if err := ValidateRequest(duplicate); !IsCode(err, "DUPLICATE_INPUT") {
		t.Fatalf("duplicate input error = %v", err)
	}
}

func TestWorkspaceAndRepositoryPathBoundaries(t *testing.T) {
	t.Parallel()
	validWorkspace := "/" + strings.Repeat("w", 4095)
	if err := validateWorkspace(
		Workspace{Path: validWorkspace, Access: ReadOnly},
	); err != nil {
		t.Fatalf("4096-byte workspace error = %v", err)
	}
	for name, workspace := range map[string]string{
		"over byte limit": "/" + strings.Repeat("w", 4096),
		"C0 control":      "/workspace/\x1fchild",
		"C1 control":      "/workspace/\u0085child",
		"invalid UTF-8":   "/workspace/\xffchild",
	} {
		if err := validateWorkspace(
			Workspace{Path: workspace, Access: ReadOnly},
		); !IsCode(err, "INVALID_WORKSPACE") {
			t.Fatalf("%s workspace error = %v", name, err)
		}
	}
	for name, repositoryPath := range map[string]string{
		"1000 bytes":         strings.Repeat("p", 1000),
		"git-like name":      "nested/.gitignore",
		"nested git segment": "nested/.git/file",
	} {
		if err := validateRepositoryPath(repositoryPath); err != nil {
			t.Fatalf("%s repository path error = %v", name, err)
		}
	}
}

func TestResultCodecHasNoTranscriptOrDecisionSeamAndBindsUsage(t *testing.T) {
	t.Parallel()
	zero := int64(0)
	result := Result{
		SchemaVersion:   ResultSchemaVersion,
		InvocationID:    "invocation-001",
		AdapterID:       FakeDriverID,
		AdapterVersion:  FakeDriverVersion,
		ObservedModel:   "fake-model-v1",
		DurationMillis:  0,
		TransportStatus: Completed,
		Usage:           &Usage{InputTokens: 0, OutputTokens: 0},
		Cost: &CostObservation{
			MicroUnits: zero,
			Currency:   "USD",
			Source:     CostSourceProviderReported,
		},
	}
	body, err := EncodeResult(result)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeResult(body, ResultBinding{
		InvocationID:   result.InvocationID,
		AdapterID:      result.AdapterID,
		AdapterVersion: result.AdapterVersion,
		Model:          result.ObservedModel,
		BindModel:      true,
	})
	if err != nil || decoded.Usage == nil || decoded.Cost == nil ||
		decoded.Usage.InputTokens != 0 || decoded.Cost.MicroUnits != 0 {
		t.Fatalf("decoded = %#v, error = %v", decoded, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"text", "transcript", "content", "outcome", "verdict", "proof",
		"candidate", "git", "receipt",
	} {
		if _, present := fields[forbidden]; present {
			t.Fatalf("result exposes forbidden %q field", forbidden)
		}
	}
	hostile := bytes.Replace(
		body,
		[]byte(`"transport_status":"completed"`),
		[]byte(`"text":"secret transcript","transport_status":"completed"`),
		1,
	)
	if _, err := DecodeResult(hostile, ResultBinding{}); !IsCode(err, "UNKNOWN_FIELD") {
		t.Fatalf("raw transcript field error = %v", err)
	}
	withoutReports := result
	withoutReports.Usage = nil
	withoutReports.Cost = nil
	body, err = EncodeResult(withoutReports)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err = DecodeResult(body, ResultBinding{})
	if err != nil || decoded.Usage != nil || decoded.Cost != nil {
		t.Fatalf("unavailable reports = %#v, %v", decoded, err)
	}
}

func TestResultRejectsUnsafeNumbersCostAndBindings(t *testing.T) {
	t.Parallel()
	result := Result{
		SchemaVersion:   ResultSchemaVersion,
		InvocationID:    "invocation-001",
		AdapterID:       FakeDriverID,
		AdapterVersion:  FakeDriverVersion,
		ObservedModel:   "fake-model-v1",
		DurationMillis:  0,
		TransportStatus: Completed,
		Usage:           &Usage{InputTokens: 1, OutputTokens: 2},
	}
	body, err := EncodeResult(result)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string][]byte{
		"negative duration": bytes.Replace(
			body,
			[]byte(`"duration_ms":0`),
			[]byte(`"duration_ms":-1`),
			1,
		),
		"float usage": bytes.Replace(
			body,
			[]byte(`"input_tokens":1`),
			[]byte(`"input_tokens":1.5`),
			1,
		),
		"unsafe usage": bytes.Replace(
			body,
			[]byte(`"input_tokens":1`),
			[]byte(`"input_tokens":9007199254740992`),
			1,
		),
	}
	for name, invalid := range tests {
		if _, err := DecodeResult(invalid, ResultBinding{}); err == nil {
			t.Fatalf("%s result was accepted", name)
		}
	}
	if _, err := DecodeResult(
		body,
		ResultBinding{InvocationID: "other"},
	); !IsCode(err, "RESULT_BINDING_MISMATCH") {
		t.Fatalf("identity binding error = %v", err)
	}
	if _, err := DecodeResult(
		body,
		ResultBinding{BindModel: true, Model: "other-model"},
	); !IsCode(err, "RESULT_BINDING_MISMATCH") {
		t.Fatalf("model binding error = %v", err)
	}
	badCost := result
	badCost.Cost = &CostObservation{
		MicroUnits: 1,
		Currency:   "USD",
		Source:     "estimated",
	}
	if err := ValidateResult(badCost, ResultBinding{}); !IsCode(err, "INVALID_COST_OBSERVATION") {
		t.Fatalf("estimated cost error = %v", err)
	}
}

func TestFakeIsRoleNeutralAndTransportOnly(t *testing.T) {
	t.Parallel()
	for _, role := range []Role{RolePlanner, RoleImplementer, RoleCaptain, RoleVerifier} {
		role := role
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()
			request := contractRequest(t, role)
			for _, profile := range []FakeProfile{
				FakeCompleted,
				FakeTransportError,
				FakeTimeout,
				FakeCancelled,
				FakeRunnerError,
			} {
				result, err := RunFake(request, profile)
				if err != nil {
					t.Fatal(err)
				}
				if result.TransportStatus != TransportStatus(profile) ||
					(result.Usage != nil) != (profile == FakeCompleted) {
					t.Fatalf("%s result = %#v", profile, result)
				}
			}
		})
	}
	if _, err := RunFake(
		func() Request {
			value := contractRequest(t, RolePlanner)
			value.Role = Role("merge")
			return value
		}(),
		FakeCompleted,
	); !IsCode(err, "INVALID_ROLE") {
		t.Fatalf("Merge fake error = %v", err)
	}
}
