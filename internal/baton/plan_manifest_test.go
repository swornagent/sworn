package baton

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func manifestSliceEntry(id, contractPath, touchpoint, digest string) map[string]any {
	return map[string]any{
		"id": id, "outcome": "Deliver " + id + ".",
		"contract_path": contractPath, "digest": digest,
		"depends_on": []any{}, "consumes": []any{},
		"touchpoints": []any{touchpoint},
	}
}

func baseManifest() map[string]any {
	placeholder := "sha256:" + strings.Repeat("a", 64)
	return map[string]any{
		"schema_version": ManifestVersion, "release": "nativekernel", "revision": int64(1),
		"previous_plan": nil, "repository": "sworn/native", "target_ref": "refs/heads/main",
		"approval_ref": "sworn://approval/nativekernel/1",
		"tracks": []any{
			map[string]any{
				"id": "T1", "depends_on": []any{},
				"slices": []any{manifestSliceEntry("S1", "contracts/S1.json", "one/file.go", placeholder)},
			},
			map[string]any{
				"id": "T2", "depends_on": []any{},
				"slices": []any{manifestSliceEntry("S2", "contracts/S2.json", "two/file.go", placeholder)},
			},
		},
	}
}

func manifestRaw(t *testing.T, value map[string]any) []byte {
	t.Helper()
	metadata, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(append([]byte(manifestOpen), metadata...), []byte(manifestClose+"body\n")...)
}

func TestParsePlanUnknownFenceFailsClosed(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string][]byte{
		"empty":         []byte(""),
		"garbage":       []byte("not a plan"),
		"unknown fence": []byte("```something-else\n{}\n```\nbody\n"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParsePlan(raw); ErrorCode(err) != "INVALID_PLAN_FENCE" {
				t.Fatalf("code = %q (%v), want INVALID_PLAN_FENCE", ErrorCode(err), err)
			}
		})
	}
}

func TestManifestPlanParsesCompactEntries(t *testing.T) {
	t.Parallel()
	plan, err := ParsePlan(manifestRaw(t, baseManifest()))
	if err != nil {
		t.Fatal(err)
	}
	metadata := plan.Metadata()
	if metadata.SchemaVersion != ManifestVersion {
		t.Fatalf("schema_version = %q, want %q", metadata.SchemaVersion, ManifestVersion)
	}
	track, slice, ok := plan.FindSlice("S1")
	if !ok || track.ID != "T1" {
		t.Fatalf("FindSlice(S1) = %#v %#v %v", track, slice, ok)
	}
	if slice.Outcome != "Deliver S1." || slice.ContractPath != "contracts/S1.json" ||
		!reflect.DeepEqual(slice.Scope.Include, []string{"one/file.go"}) ||
		len(slice.Scope.Exclude) != 0 || len(slice.Acceptance) != 0 ||
		len(slice.Checks) != 0 || len(slice.Constraints) != 0 {
		t.Fatalf("slice = %#v", slice)
	}
	digest, ok := plan.Contract("S1")
	if !ok || digest != "sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("Contract(S1) = %q, %v", digest, ok)
	}
}

func TestManifestPlanValidationFailures(t *testing.T) {
	t.Parallel()
	placeholder := "sha256:" + strings.Repeat("a", 64)
	parse := func(value map[string]any) error {
		_, err := ParsePlan(manifestRaw(t, value))
		return err
	}
	cases := map[string]struct {
		mutate func(map[string]any)
		code   string
	}{
		"missing field": {
			mutate: func(value map[string]any) {
				tracks := value["tracks"].([]any)
				entry := tracks[0].(map[string]any)["slices"].([]any)[0].(map[string]any)
				delete(entry, "digest")
			},
			code: "MISSING_FIELD",
		},
		"unknown field": {
			mutate: func(value map[string]any) {
				tracks := value["tracks"].([]any)
				entry := tracks[0].(map[string]any)["slices"].([]any)[0].(map[string]any)
				entry["foreign"] = true
			},
			code: "UNKNOWN_FIELD",
		},
		"malformed digest": {
			mutate: func(value map[string]any) {
				tracks := value["tracks"].([]any)
				entry := tracks[0].(map[string]any)["slices"].([]any)[0].(map[string]any)
				entry["digest"] = "not-a-digest"
			},
			code: "INVALID_FIELD",
		},
		"escaping contract path": {
			mutate: func(value map[string]any) {
				tracks := value["tracks"].([]any)
				entry := tracks[0].(map[string]any)["slices"].([]any)[0].(map[string]any)
				entry["contract_path"] = "../escape.json"
			},
			code: "INVALID_PATH",
		},
		"reserved contract path": {
			mutate: func(value map[string]any) {
				tracks := value["tracks"].([]any)
				entry := tracks[0].(map[string]any)["slices"].([]any)[0].(map[string]any)
				entry["contract_path"] = RecordRoot + "/S1.json"
			},
			code: "RESERVED_RECORD_ROOT",
		},
		"empty touchpoints": {
			mutate: func(value map[string]any) {
				tracks := value["tracks"].([]any)
				entry := tracks[0].(map[string]any)["slices"].([]any)[0].(map[string]any)
				entry["touchpoints"] = []any{}
			},
			code: "INVALID_FIELD",
		},
		"reserved touchpoint": {
			mutate: func(value map[string]any) {
				tracks := value["tracks"].([]any)
				entry := tracks[0].(map[string]any)["slices"].([]any)[0].(map[string]any)
				entry["touchpoints"] = []any{RecordRoot}
			},
			code: "RESERVED_RECORD_ROOT",
		},
		"multi-line outcome": {
			mutate: func(value map[string]any) {
				tracks := value["tracks"].([]any)
				entry := tracks[0].(map[string]any)["slices"].([]any)[0].(map[string]any)
				entry["outcome"] = "Deliver S1.\nSecond line."
			},
			code: "INVALID_FIELD",
		},
		"duplicate slice id": {
			mutate: func(value map[string]any) {
				tracks := value["tracks"].([]any)
				tracks[1].(map[string]any)["slices"] =
					[]any{manifestSliceEntry("S1", "contracts/other.json", "three/file.go", placeholder)}
			},
			code: "DUPLICATE_IDENTITY",
		},
		"duplicate contract path": {
			mutate: func(value map[string]any) {
				tracks := value["tracks"].([]any)
				tracks[1].(map[string]any)["slices"] =
					[]any{manifestSliceEntry("S2", "contracts/S1.json", "two/file.go", placeholder)}
			},
			code: "DUPLICATE_IDENTITY",
		},
		"inconsistent dependency": {
			mutate: func(value map[string]any) {
				tracks := value["tracks"].([]any)
				entry := tracks[0].(map[string]any)["slices"].([]any)[0].(map[string]any)
				entry["depends_on"] = []any{"UNKNOWN"}
			},
			code: "INVALID_DEPENDENCY",
		},
		"overlapping touchpoint": {
			mutate: func(value map[string]any) {
				tracks := value["tracks"].([]any)
				entry := tracks[1].(map[string]any)["slices"].([]any)[0].(map[string]any)
				entry["touchpoints"] = []any{"one/file.go"}
			},
			code: "PARALLEL_TOUCH_CONFLICT",
		},
		"wrong schema": {
			mutate: func(value map[string]any) {
				value["schema_version"] = "baton.plan/v3"
			},
			code: "INVALID_FIELD",
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			value := baseManifest()
			test.mutate(value)
			if code := ErrorCode(parse(value)); code != test.code {
				t.Fatalf("code = %q, want %q", code, test.code)
			}
		})
	}
}

func TestManifestPlanOrderedTouchpointOverlapSucceedsAndAppearsInMatrix(t *testing.T) {
	t.Parallel()
	value := baseManifest()
	tracks := value["tracks"].([]any)
	tracks[1].(map[string]any)["depends_on"] = []any{"T1"}
	entry := tracks[1].(map[string]any)["slices"].([]any)[0].(map[string]any)
	entry["touchpoints"] = []any{"one/file.go"}
	plan, err := ParsePlan(manifestRaw(t, value))
	if err != nil {
		t.Fatal(err)
	}
	matrix := plan.TouchpointMatrix()
	if len(matrix) != 1 {
		t.Fatalf("matrix = %#v, want one relation", matrix)
	}
	relation := matrix[0]
	if relation.Left != "S1" || relation.Right != "S2" || relation.Path != "one/file.go" ||
		!relation.Ordered || relation.Before != "S1" {
		t.Fatalf("relation = %#v", relation)
	}
}

func legacySliceValue(id string) map[string]any {
	return map[string]any{
		"id": id, "outcome": "Deliver " + id + ".",
		"scope":      map[string]any{"include": []any{"src/one.go"}, "exclude": []any{"src/one_test.go"}},
		"acceptance": []any{map[string]any{"id": "A-" + id, "text": id + " is exact."}},
		"checks":     []any{"go test ./..."}, "constraints": []any{"deterministic"},
		"depends_on": []any{}, "consumes": []any{},
	}
}

func TestParseSliceContractDigestParityWithLegacySlice(t *testing.T) {
	t.Parallel()
	legacyPlanValue := map[string]any{
		"schema_version": PlanVersion, "release": "parity", "revision": int64(1),
		"previous_plan": nil, "repository": "sworn/native", "target_ref": "refs/heads/main",
		"approval_ref": "sworn://approval/parity/1",
		"tracks": []any{
			map[string]any{"id": "T1", "depends_on": []any{}, "slices": []any{legacySliceValue("S1")}},
		},
	}
	metadata, err := json.MarshalIndent(legacyPlanValue, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	legacyPlan, err := ParsePlan(append(append([]byte(planOpen), metadata...), []byte(planClose+"body\n")...))
	if err != nil {
		t.Fatal(err)
	}
	legacyDigest, ok := legacyPlan.Contract("S1")
	if !ok {
		t.Fatal("legacy plan has no contract for S1")
	}
	_, legacySlice, ok := legacyPlan.FindSlice("S1")
	if !ok {
		t.Fatal("legacy plan has no slice S1")
	}

	contractValue := map[string]any{
		"outcome":     "Deliver S1.",
		"scope":       map[string]any{"include": []any{"src/one.go"}, "exclude": []any{"src/one_test.go"}},
		"acceptance":  []any{map[string]any{"id": "A-S1", "text": "S1 is exact."}},
		"checks":      []any{"go test ./..."},
		"constraints": []any{"deterministic"},
		"depends_on":  []any{}, "consumes": []any{},
	}
	contractRaw, err := json.MarshalIndent(contractValue, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	contractSlice, contractDigest, err := ParseSliceContract(contractRaw, "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}
	if contractDigest != legacyDigest {
		t.Fatalf("contract digest = %q, want legacy digest %q", contractDigest, legacyDigest)
	}
	if !reflect.DeepEqual(contractSlice, legacySlice) {
		t.Fatalf("contract slice = %#v, want %#v", contractSlice, legacySlice)
	}
}

func TestParseSliceContractRejectsMalformedAndUnsafeContent(t *testing.T) {
	t.Parallel()
	valid := func() map[string]any {
		return map[string]any{
			"outcome":     "Deliver S1.",
			"scope":       map[string]any{"include": []any{"src/one.go"}, "exclude": []any{}},
			"acceptance":  []any{map[string]any{"id": "A-S1", "text": "S1 is exact."}},
			"checks":      []any{"go test ./..."},
			"constraints": []any{"deterministic"},
			"depends_on":  []any{}, "consumes": []any{},
		}
	}
	cases := map[string]struct {
		mutate func(map[string]any)
		code   string
	}{
		"unknown field": {
			mutate: func(v map[string]any) { v["foreign"] = true },
			code:   "UNKNOWN_FIELD",
		},
		"embedded id rejected": {
			mutate: func(v map[string]any) { v["id"] = "S1" },
			code:   "UNKNOWN_FIELD",
		},
		"duplicate acceptance": {
			mutate: func(v map[string]any) {
				v["acceptance"] = []any{
					map[string]any{"id": "A-S1", "text": "one"},
					map[string]any{"id": "A-S1", "text": "two"},
				}
			},
			code: "DUPLICATE_IDENTITY",
		},
		"escaping scope path": {
			mutate: func(v map[string]any) {
				v["scope"] = map[string]any{"include": []any{"../escape.go"}, "exclude": []any{}}
			},
			code: "INVALID_PATH",
		},
		"reserved scope path": {
			mutate: func(v map[string]any) {
				v["scope"] = map[string]any{"include": []any{RecordRoot}, "exclude": []any{}}
			},
			code: "RESERVED_RECORD_ROOT",
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			value := valid()
			test.mutate(value)
			raw, err := json.MarshalIndent(value, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = ParseSliceContract(raw, "S1", "T1")
			if code := ErrorCode(err); code != test.code {
				t.Fatalf("code = %q (%v), want %q", code, err, test.code)
			}
		})
	}
}

func manifestPlanWithContract(t *testing.T, digest string) Plan {
	t.Helper()
	value := map[string]any{
		"schema_version": ManifestVersion, "release": "resolve", "revision": int64(1),
		"previous_plan": nil, "repository": "sworn/native", "target_ref": "refs/heads/main",
		"approval_ref": "sworn://approval/resolve/1",
		"tracks": []any{
			map[string]any{
				"id": "T1", "depends_on": []any{},
				"slices": []any{manifestSliceEntry("S1", "contracts/S1.json", "one/file.go", digest)},
			},
		},
	}
	plan, err := ParsePlan(manifestRaw(t, value))
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestResolveSliceContractCrossValidatesManifestDeclaration(t *testing.T) {
	t.Parallel()
	realContract := func() map[string]any {
		return map[string]any{
			"outcome":     "Deliver S1.",
			"scope":       map[string]any{"include": []any{"one/file.go"}, "exclude": []any{}},
			"acceptance":  []any{map[string]any{"id": "A-S1", "text": "S1 is exact."}},
			"checks":      []any{"go test ./..."},
			"constraints": []any{"deterministic"},
			"depends_on":  []any{}, "consumes": []any{},
		}
	}
	rawFor := func(t *testing.T, value map[string]any) []byte {
		t.Helper()
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	_, realDigest, err := ParseSliceContract(rawFor(t, realContract()), "S1", "T1")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("matching contract resolves fully", func(t *testing.T) {
		plan := manifestPlanWithContract(t, realDigest)
		slice, err := plan.ResolveSliceContract("S1", rawFor(t, realContract()))
		if err != nil {
			t.Fatal(err)
		}
		if slice.ContractPath != "contracts/S1.json" || slice.Outcome != "Deliver S1." ||
			len(slice.Acceptance) != 1 || len(slice.Checks) != 1 || len(slice.Constraints) != 1 {
			t.Fatalf("resolved slice = %#v", slice)
		}
	})

	t.Run("tampered contract fails on digest", func(t *testing.T) {
		plan := manifestPlanWithContract(t, realDigest)
		tampered := realContract()
		tampered["outcome"] = "Deliver S1 differently."
		if _, err := plan.ResolveSliceContract("S1", rawFor(t, tampered)); ErrorCode(err) != "STALE_BINDING" {
			t.Fatalf("code = %q (%v), want STALE_BINDING", ErrorCode(err), err)
		}
	})

	t.Run("manifest outcome preview mismatch fails despite matching digest", func(t *testing.T) {
		value := map[string]any{
			"schema_version": ManifestVersion, "release": "resolve", "revision": int64(1),
			"previous_plan": nil, "repository": "sworn/native", "target_ref": "refs/heads/main",
			"approval_ref": "sworn://approval/resolve/1",
			"tracks": []any{
				map[string]any{
					"id": "T1", "depends_on": []any{},
					"slices": []any{map[string]any{
						"id": "S1", "outcome": "Deliver S1 (preview).",
						"contract_path": "contracts/S1.json", "digest": realDigest,
						"depends_on": []any{}, "consumes": []any{}, "touchpoints": []any{"one/file.go"},
					}},
				},
			},
		}
		plan, err := ParsePlan(manifestRaw(t, value))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := plan.ResolveSliceContract("S1", rawFor(t, realContract())); ErrorCode(err) != "STALE_BINDING" {
			t.Fatalf("code = %q (%v), want STALE_BINDING", ErrorCode(err), err)
		}
	})

	t.Run("unknown slice fails closed", func(t *testing.T) {
		plan := manifestPlanWithContract(t, realDigest)
		if _, err := plan.ResolveSliceContract("MISSING", rawFor(t, realContract())); ErrorCode(err) != "SLICE_NOT_FOUND" {
			t.Fatalf("code = %q (%v), want SLICE_NOT_FOUND", ErrorCode(err), err)
		}
	})
}
