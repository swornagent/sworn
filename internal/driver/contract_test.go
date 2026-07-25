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

func contractRequest(t *testing.T, role Role, model *string) Request {
	t.Helper()
	request, err := NewRequest(
		"invocation-001",
		role,
		model,
		Workspace{Path: "/workspace/project", Access: ReadOnly},
		[]Input{
			{
				Name:   "plan",
				Path:   ".baton/releases/v1.0.0/plan.md",
				Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
			{
				Name:   "proof",
				Path:   ".baton/releases/v1.0.0/work/W1/proof.md",
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

func TestCanonicalOperationsBindExactRC2Assets(t *testing.T) {
	t.Parallel()
	expected := map[Role]struct {
		id     string
		digest string
	}{
		RolePlanner: {
			"baton-plan",
			"sha256:e5c3ace4177cb10c9b0d3b5e569aa7cbe43bfdb3b7f4a17071a925a5ba3b77d3",
		},
		RoleImplementer: {
			"baton-implement",
			"sha256:2444bead5b1a32188003ce515ac8862bd04d373b740bd89646a86ac5341c2f88",
		},
		RoleCaptain: {
			"baton-design-review",
			"sha256:ead3a7d0e22a794ca5430fdbaca5c29f3ae5d5f6fad7c102d1f2bd878f28e356",
		},
		RoleVerifier: {
			"baton-verify",
			"sha256:a6f0e9b9bf95cb59e5030b7f95f72d8d3545b52ef771c7d20e7be44a20e45bed",
		},
		RoleMerge: {
			"baton-merge",
			"sha256:94b8fb6026c903569cd375cafd11d27868759072dde256265556c710387ae62c",
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
			if operation.ID != want.id || operation.Version != OperationVersion ||
				operation.Digest != want.digest {
				t.Fatalf("operation = %#v", operation)
			}
			if !strings.HasSuffix(operation.Instructions, "\n") ||
				strings.Contains(operation.Instructions, "\r") ||
				Digest([]byte(operation.Instructions)) != want.digest {
				t.Fatal("operation bytes are not exact UTF-8/LF bytes")
			}
		})
	}
}

func TestDriverInfoCodecIsExactStrictAndBound(t *testing.T) {
	t.Parallel()
	info := FakeInfo()
	body, err := EncodeDriverInfo(info)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"contract_version":"baton.driver/v1","driver_id":"baton.fake","driver_version":"1.0.0"}` + "\n"
	if string(body) != want {
		t.Fatalf("info = %q", body)
	}
	decoded, err := DecodeDriverInfo(body, DriverInfoBinding{
		DriverID:      FakeDriverID,
		DriverVersion: FakeDriverVersion,
	})
	if err != nil || decoded != info {
		t.Fatalf("decoded = %#v, error = %v", decoded, err)
	}
	extra := bytes.Replace(body,
		[]byte(`"driver_version":"1.0.0"`),
		[]byte(`"driver_version":"1.0.0","default_model":"forbidden"`),
		1,
	)
	if _, err := DecodeDriverInfo(extra, DriverInfoBinding{}); !IsCode(err, "UNKNOWN_FIELD") {
		t.Fatalf("unknown error = %v", err)
	}
	duplicate := bytes.Replace(body,
		[]byte(`"driver_id":"baton.fake"`),
		[]byte(`"driver_id":"baton.fake","driver_id":"baton.fake"`),
		1,
	)
	if _, err := DecodeDriverInfo(duplicate, DriverInfoBinding{}); !IsCode(err, "DUPLICATE_NAME") {
		t.Fatalf("duplicate error = %v", err)
	}
	if _, err := DecodeDriverInfo(body, DriverInfoBinding{DriverID: "other"}); !IsCode(err, "DRIVER_BINDING_MISMATCH") {
		t.Fatalf("binding error = %v", err)
	}
}

func TestRequestCodecIsStrictBoundedAndStable(t *testing.T) {
	t.Parallel()
	request := contractRequest(t, RoleVerifier, pointer("fake-model-v1"))
	body, err := EncodeRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(body, []byte(`{"schema_version":"baton.driver-request/v1","invocation_id":"invocation-001","role":"verifier"`)) {
		t.Fatalf("unexpected canonical prefix: %.120q", body)
	}
	decoded, err := DecodeRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	reencoded, err := EncodeRequest(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, reencoded) {
		t.Fatal("request codec is not byte stable")
	}

	tests := map[string]struct {
		body []byte
		code string
	}{
		"duplicate": {
			bytes.Replace(body,
				[]byte(`"schema_version":"baton.driver-request/v1"`),
				[]byte(`"schema_version":"baton.driver-request/v1","schema_version":"baton.driver-request/v1"`),
				1),
			"DUPLICATE_NAME",
		},
		"trailing":        {append(append([]byte(nil), body...), []byte(`{}`)...), "TRAILING_JSON"},
		"invalid utf8":    {[]byte{'{', '"', 0xff, '"', '}'}, "INVALID_UTF8"},
		"invalid unicode": {[]byte(`{"schema_version":"baton.driver-request/v1","invocation_id":"\ud800"}`), "INVALID_UNICODE"},
		"unknown field":   {bytes.Replace(body, []byte(`"fresh_context":true`), []byte(`"fallback_model":"x","fresh_context":true`), 1), "UNKNOWN_FIELD"},
		"missing model":   {bytes.Replace(body, []byte(`"model":"fake-model-v1",`), nil, 1), "MISSING_FIELD"},
		"float timeout":   {bytes.Replace(body, []byte(`"timeout_ms":60000`), []byte(`"timeout_ms":1.5`), 1), "INVALID_FIELD"},
		"unsafe integer":  {bytes.Replace(body, []byte(`"timeout_ms":60000`), []byte(`"timeout_ms":9007199254740992`), 1), "INVALID_FIELD"},
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
	if _, err := DecodeRequest(bytes.Repeat([]byte{' '}, MaxRequestBytes+1)); !IsCode(err, "RESOURCE_LIMIT") {
		t.Fatalf("oversized error = %v", err)
	}
}

func TestRequestRejectsReplacementOperationRoleDriftAndBadInputs(t *testing.T) {
	t.Parallel()
	request := contractRequest(t, RoleVerifier, pointer("fake-model-v1"))

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
	relative := request
	relative.Workspace.Path = "relative"
	if err := ValidateRequest(relative); !IsCode(err, "INVALID_WORKSPACE") {
		t.Fatalf("workspace error = %v", err)
	}
	duplicateName := request
	duplicateName.Inputs = append([]Input(nil), request.Inputs...)
	duplicateName.Inputs[1].Name = duplicateName.Inputs[0].Name
	if err := ValidateRequest(duplicateName); !IsCode(err, "DUPLICATE_INPUT") {
		t.Fatalf("duplicate error = %v", err)
	}
	escape := request
	escape.Inputs = append([]Input(nil), request.Inputs...)
	escape.Inputs[0].Path = "../escape"
	if err := ValidateRequest(escape); !IsCode(err, "INVALID_PATH") {
		t.Fatalf("path error = %v", err)
	}
	zeroLimit := request
	zeroLimit.Limits.OutputBytes = 0
	if err := ValidateRequest(zeroLimit); !IsCode(err, "INVALID_LIMIT") {
		t.Fatalf("limit error = %v", err)
	}
}

func TestRC2WorkspaceAndRepositoryPathBoundaries(t *testing.T) {
	t.Parallel()

	validWorkspace := "/" + strings.Repeat("w", 4095)
	if err := validateWorkspace(Workspace{Path: validWorkspace, Access: ReadOnly}); err != nil {
		t.Fatalf("4096-byte workspace error = %v", err)
	}
	for name, workspace := range map[string]string{
		"over byte limit": "/" + strings.Repeat("w", 4096),
		"C0 control":      "/workspace/\x1fchild",
		"C1 control":      "/workspace/\u0085child",
		"invalid UTF-8":   "/workspace/\xffchild",
	} {
		if err := validateWorkspace(Workspace{Path: workspace, Access: ReadOnly}); !IsCode(err, "INVALID_WORKSPACE") {
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
	for name, repositoryPath := range map[string]string{
		"over byte limit":  strings.Repeat("p", 1001),
		"root git segment": ".git/config",
		"root parent":      "..",
		"nested parent":    "nested/../escape",
		"C0 control":       "nested/\x1fchild",
		"C1 control":       "nested/\u0085child",
		"invalid UTF-8":    "nested/\xffchild",
	} {
		if err := validateRepositoryPath(repositoryPath); !IsCode(err, "INVALID_PATH") {
			t.Fatalf("%s repository path error = %v", name, err)
		}
	}
}

func TestResultCodecAllowsEmptyTextAndBindsEveryIdentity(t *testing.T) {
	t.Parallel()
	result := Result{
		SchemaVersion:   ResultSchemaVersion,
		InvocationID:    "invocation-001",
		DriverID:        FakeDriverID,
		DriverVersion:   FakeDriverVersion,
		ObservedModel:   pointer("fake-model-v1"),
		DurationMillis:  0,
		Text:            "",
		TransportStatus: Completed,
		Usage:           &Usage{InputTokens: 0, OutputTokens: 0},
	}
	body, err := EncodeResult(result)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeResult(body, ResultBinding{
		InvocationID:  result.InvocationID,
		DriverID:      result.DriverID,
		DriverVersion: result.DriverVersion,
		Model:         result.ObservedModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Text != "" || decoded.Usage == nil ||
		decoded.Usage.InputTokens != 0 || decoded.Usage.OutputTokens != 0 {
		t.Fatalf("decoded = %#v", decoded)
	}
	withoutUsage := result
	withoutUsage.Usage = nil
	withoutUsage.TransportStatus = TransportError
	body, err = EncodeResult(withoutUsage)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err = DecodeResult(body, ResultBinding{})
	if err != nil || decoded.Usage != nil {
		t.Fatalf("absent usage = %#v, %v", decoded, err)
	}
	extra := bytes.Replace(body, []byte(`"transport_status":"transport_error"`),
		[]byte(`"transport_status":"transport_error","verdict":"pass"`), 1)
	if _, err := DecodeResult(extra, ResultBinding{}); !IsCode(err, "UNKNOWN_FIELD") {
		t.Fatalf("extra result error = %v", err)
	}
	if _, err := DecodeResult(body, ResultBinding{InvocationID: "other"}); !IsCode(err, "RESULT_BINDING_MISMATCH") {
		t.Fatalf("binding error = %v", err)
	}
	nullModel := withoutUsage
	nullModel.ObservedModel = nil
	body, err = EncodeResult(nullModel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeResult(body, ResultBinding{BindModel: true, Model: nil}); err != nil {
		t.Fatalf("deliberate null model was rejected: %v", err)
	}
	if _, err := DecodeResult(body, ResultBinding{
		BindModel: true,
		Model:     pointer("unexpected"),
	}); !IsCode(err, "RESULT_BINDING_MISMATCH") {
		t.Fatalf("null binding error = %v", err)
	}
}

func TestResultRejectsUnsafeNumbersAndOversizedText(t *testing.T) {
	t.Parallel()
	result := Result{
		SchemaVersion:   ResultSchemaVersion,
		InvocationID:    "invocation-001",
		DriverID:        FakeDriverID,
		DriverVersion:   FakeDriverVersion,
		ObservedModel:   pointer("fake-model-v1"),
		DurationMillis:  0,
		Text:            "ok",
		TransportStatus: Completed,
		Usage:           &Usage{InputTokens: 1, OutputTokens: 2},
	}
	body, err := EncodeResult(result)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string][]byte{
		"negative duration": bytes.Replace(body,
			[]byte(`"duration_ms":0`), []byte(`"duration_ms":-1`), 1),
		"float usage": bytes.Replace(body,
			[]byte(`"input_tokens":1`), []byte(`"input_tokens":1.5`), 1),
		"unsafe usage": bytes.Replace(body,
			[]byte(`"input_tokens":1`), []byte(`"input_tokens":9007199254740992`), 1),
	}
	for name, invalid := range tests {
		if _, err := DecodeResult(invalid, ResultBinding{}); err == nil {
			t.Fatalf("%s result was accepted", name)
		}
	}
	result.Text = strings.Repeat("x", MaxResultTextBytes+1)
	if err := ValidateResult(result, ResultBinding{}); !IsCode(err, "INVALID_FIELD") {
		t.Fatalf("text error = %v", err)
	}
}

func TestFakeIsRoleNeutralAndTransportOnly(t *testing.T) {
	t.Parallel()
	roles := []Role{RolePlanner, RoleImplementer, RoleCaptain, RoleVerifier, RoleMerge}
	for _, role := range roles {
		role := role
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()
			model := pointer("fake-model-v1")
			if role == RoleMerge {
				model = nil
			}
			request := contractRequest(t, role, model)
			for _, profile := range []FakeProfile{
				FakeCompleted, FakeTransportError, FakeTimeout, FakeCancelled, FakeRunnerError,
			} {
				result, err := RunFake(request, profile, false)
				if err != nil {
					t.Fatal(err)
				}
				if result.TransportStatus != TransportStatus(profile) ||
					(result.Usage != nil) != (profile == FakeCompleted) {
					t.Fatalf("%s result = %#v", profile, result)
				}
				body, err := EncodeResult(result)
				if err != nil {
					t.Fatal(err)
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(body, &fields); err != nil {
					t.Fatal(err)
				}
				for _, forbidden := range []string{"outcome", "verdict", "proof", "merge", "fresh_context"} {
					if _, exists := fields[forbidden]; exists {
						t.Fatalf("result contains %q", forbidden)
					}
				}
			}
			empty, err := RunFake(request, FakeCompleted, true)
			if err != nil || empty.Text != "" {
				t.Fatalf("empty result = %#v, %v", empty, err)
			}
		})
	}
	info, err := json.Marshal(FakeInfo())
	if err != nil {
		t.Fatal(err)
	}
	if string(info) != `{"contract_version":"baton.driver/v1","driver_id":"baton.fake","driver_version":"1.0.0"}` {
		t.Fatalf("info = %s", info)
	}
}
