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

func TestCanonicalOperationsBindExactRC5PackageAndExcludeMerge(t *testing.T) {
	t.Parallel()
	_, identity, err := admittedPackage()
	if err != nil {
		t.Fatal(err)
	}
	if identity != (PackageIdentity{
		Version:              "1.0.0-rc.5",
		TagName:              "v1.0.0-rc.5",
		TagObject:            "306ed09c3152e8a7413e6b9d09d63d00ee12ff4a",
		Commit:               "afad775121d7d37244f4d3798b7b4c6a9fbfe9b2",
		Tree:                 "81d089c28639eb3aaeea8f6ced2eb2fad0f596a3",
		ArchiveSHA256:        "sha256:8fea81036dc678e9a0aa4c2d1fb0c8ed016c23b9e7d77c183f3f168467002dd5",
		SupportPackageSHA256: "sha256:cd7db90c183ca4dc443673c370d64a3287d4e31323ea3f7972c5fec83d193bbf",
		ManifestSHA256:       "sha256:4501d7c16c01298565411e7ef263db0da8f14294494fcb18dce0b73ce845b175",
	}) {
		t.Fatalf("package identity = %#v", identity)
	}
	expected := map[Role]struct {
		id     string
		digest string
	}{
		RolePlanner: {
			"baton-plan",
			"sha256:91197ccfdda4475b09f70d50e6dd1fe248f7135625172618051b81dc98016088",
		},
		RoleImplementer: {
			"baton-implement",
			"sha256:f8558c042acc653c3093fc2efb9bac7599a4e479139174e93df9c33e64743a6d",
		},
		RoleCaptain: {
			"baton-design-review",
			"sha256:c12847c8b91f71e96a5996cb064588ff1612df37878065577d3f00ee1073a541",
		},
		RoleVerifier: {
			"baton-verify",
			"sha256:080034f552086a7e73fc27fb9f155320ac7638749481b477d16af4afdc59afaf",
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
		"stale verifier": func(value *Request) { value.FreshContext = false },
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
