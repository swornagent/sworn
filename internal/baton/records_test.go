package baton

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type receiptGoldenOutcome struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Code  string `json:"code"`
	Value any    `json:"value"`
}

type receiptGoldenCorpus struct {
	Plan struct {
		BytesBase64 string `json:"bytes_base64"`
		Digest      string `json:"digest"`
		Metadata    struct {
			SchemaVersion string            `json:"schema_version"`
			Release       string            `json:"release"`
			Revision      int64             `json:"revision"`
			PreviousPlan  *string           `json:"previous_plan"`
			Repository    string            `json:"repository"`
			TargetRef     string            `json:"target_ref"`
			ApprovalRef   string            `json:"approval_ref"`
			Tracks        []Track           `json:"tracks"`
			Contracts     map[string]string `json:"contracts"`
		} `json:"metadata"`
		Markdown string `json:"markdown"`
	} `json:"plan"`
	StrictJSON  []receiptGoldenOutcome `json:"strict_json"`
	InvalidPlan []receiptGoldenOutcome `json:"invalid_plans"`
	Receipt     struct {
		Canonical      string          `json:"canonical"`
		Parsed         json.RawMessage `json:"parsed"`
		RenderedBase64 string          `json:"rendered_base64"`
		ParsedCommit   struct {
			Subject      string          `json:"subject"`
			DetailBase64 string          `json:"detail_base64"`
			Receipt      json.RawMessage `json:"receipt"`
		} `json:"parsed_commit"`
		Noncanonical receiptGoldenOutcome `json:"noncanonical"`
	} `json:"receipt"`
}

func loadReceiptGolden(t *testing.T) receiptGoldenCorpus {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "tools", "batongolden", "testdata", "corpus", "receipts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus receiptGoldenCorpus
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&corpus); err != nil {
		t.Fatal(err)
	}
	return corpus
}

func TestPlanAdmissionMatchesExactRC8Golden(t *testing.T) {
	t.Parallel()
	golden := loadReceiptGolden(t)
	raw, err := base64.StdEncoding.DecodeString(golden.Plan.BytesBase64)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := ParsePlan(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plan.Bytes(), raw) || plan.Digest() != golden.Plan.Digest ||
		plan.Markdown() != golden.Plan.Markdown {
		t.Fatalf("plan admission differs: digest=%q markdown=%q", plan.Digest(), plan.Markdown())
	}
	metadata := plan.Metadata()
	if metadata.SchemaVersion != golden.Plan.Metadata.SchemaVersion ||
		metadata.Release != golden.Plan.Metadata.Release ||
		metadata.Revision != golden.Plan.Metadata.Revision ||
		metadata.Repository != golden.Plan.Metadata.Repository ||
		metadata.TargetRef != golden.Plan.Metadata.TargetRef ||
		metadata.ApprovalRef != golden.Plan.Metadata.ApprovalRef ||
		!reflect.DeepEqual(metadata.PreviousPlan, golden.Plan.Metadata.PreviousPlan) ||
		!jsonValueEqual(metadata.Tracks, golden.Plan.Metadata.Tracks) ||
		!reflect.DeepEqual(metadata.Contracts, golden.Plan.Metadata.Contracts) {
		t.Fatalf("metadata = %#v, want %#v", metadata, golden.Plan.Metadata)
	}
	track, slice, ok := plan.FindSlice("S1")
	if !ok || track.ID != "T1" || slice.ID != "S1" {
		t.Fatalf("FindSlice = %#v %#v %v", track, slice, ok)
	}
	if contract, ok := plan.Contract("S1"); !ok || contract != golden.Plan.Metadata.Contracts["S1"] {
		t.Fatalf("Contract(S1) = %q, %v", contract, ok)
	}

	// Returned bytes and metadata are copies, not mutation handles.
	plan.Bytes()[0] = 'x'
	copied := plan.Metadata()
	copied.Tracks[0].Slices[0].Scope.Include[0] = "mutated"
	copied.Contracts["S1"] = "mutated"
	if !bytes.Equal(plan.Bytes(), raw) ||
		plan.Metadata().Tracks[0].Slices[0].Scope.Include[0] != "one.txt" ||
		plan.Metadata().Contracts["S1"] != golden.Plan.Metadata.Contracts["S1"] {
		t.Fatal("immutable plan admission was mutated through a returned value")
	}
}

func TestStrictJSONMatchesExactRC8Golden(t *testing.T) {
	t.Parallel()
	golden := loadReceiptGolden(t)
	inputs := map[string][]byte{
		"canonical":      []byte(`{"a":1,"b":[true,null]}`),
		"duplicate":      []byte(`{"a":1,"a":2}`),
		"fraction":       []byte(`1.5`),
		"unsafe":         []byte(`9007199254740992`),
		"lone_surrogate": []byte(`"\ud800"`),
	}
	for _, expected := range golden.StrictJSON {
		expected := expected
		t.Run(expected.Name, func(t *testing.T) {
			got, err := strictParseJSON(inputs[expected.Name], "golden", 1_024)
			if expected.OK {
				if err != nil {
					t.Fatal(err)
				}
				gotJSON, err := canonicalJSON(got)
				if err != nil {
					t.Fatal(err)
				}
				wantJSON, err := canonicalJSON(expected.Value)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(gotJSON, wantJSON) {
					t.Fatalf("value = %s, want %s", gotJSON, wantJSON)
				}
				return
			}
			if ErrorCode(err) != expected.Code {
				t.Fatalf("code = %q (%v), want %q", ErrorCode(err), err, expected.Code)
			}
		})
	}
}

func TestReceiptCanonicalAndCommitRoundTripMatchGolden(t *testing.T) {
	t.Parallel()
	golden := loadReceiptGolden(t)
	receipt, err := ParseReceipt([]byte(golden.Receipt.Canonical))
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := receipt.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(canonical) != golden.Receipt.Canonical {
		t.Fatalf("canonical = %s, want %s", canonical, golden.Receipt.Canonical)
	}
	rendered, err := base64.StdEncoding.DecodeString(golden.Receipt.RenderedBase64)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseReceiptCommitMessage(rendered)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := base64.StdEncoding.DecodeString(golden.Receipt.ParsedCommit.DetailBase64)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Subject != golden.Receipt.ParsedCommit.Subject || !bytes.Equal(parsed.Detail, detail) {
		t.Fatalf("parsed commit = %q %q", parsed.Subject, parsed.Detail)
	}
	projected, err := parsed.Receipt.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !jsonBytesEqual(projected, golden.Receipt.ParsedCommit.Receipt) {
		t.Fatalf("receipt = %s, want %s", projected, golden.Receipt.ParsedCommit.Receipt)
	}
	if _, err := ParseReceipt([]byte(strings.Replace(golden.Receipt.Canonical, `"version":1`, `"version": 1`, 1))); ErrorCode(err) != "NON_CANONICAL_RECEIPT" {
		t.Fatalf("noncanonical code = %q (%v)", ErrorCode(err), err)
	}
}

func TestReceiptHistoryRequiresMetadataOnlySingleParent(t *testing.T) {
	t.Parallel()
	golden := loadReceiptGolden(t)
	message, err := base64.StdEncoding.DecodeString(golden.Receipt.RenderedBase64)
	if err != nil {
		t.Fatal(err)
	}
	oid, parent, tree := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	entry, err := ParseReceiptHistoryEntry(HistoryEnvelope{
		OID: oid, Parents: []string{parent}, Tree: tree, ParentTree: tree, Message: message,
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.OID != oid || entry.Parent != parent || entry.Tree != tree {
		t.Fatalf("entry = %#v", entry)
	}
	for name, mutate := range map[string]func(*HistoryEnvelope){
		"two parents":  func(value *HistoryEnvelope) { value.Parents = append(value.Parents, strings.Repeat("d", 40)) },
		"tree changed": func(value *HistoryEnvelope) { value.ParentTree = strings.Repeat("d", 40) },
		"bad oid":      func(value *HistoryEnvelope) { value.OID = "short" },
	} {
		t.Run(name, func(t *testing.T) {
			value := HistoryEnvelope{
				OID: oid, Parents: []string{parent}, Tree: tree, ParentTree: tree, Message: message,
			}
			mutate(&value)
			if _, err := ParseReceiptHistoryEntry(value); err == nil {
				t.Fatal("invalid history envelope was admitted")
			}
		})
	}
}

func TestPlanRejectsDependencyCyclesParallelOverlapAndForeignFields(t *testing.T) {
	t.Parallel()
	baseSlice := func(id, include string) map[string]any {
		return map[string]any{
			"id": id, "outcome": "Deliver " + id + ".",
			"scope":      map[string]any{"include": []any{include}, "exclude": []any{}},
			"acceptance": []any{map[string]any{"id": "A-" + id, "text": id + " is exact."}},
			"checks":     []any{"check " + id}, "constraints": []any{"deterministic"},
			"depends_on": []any{}, "consumes": []any{},
		}
	}
	base := func() map[string]any {
		return map[string]any{
			"schema_version": PlanVersion, "release": "adversarial", "revision": int64(1),
			"previous_plan": nil, "repository": "golden/sworn", "target_ref": "refs/heads/main",
			"approval_ref": "golden://approval/adversarial/1",
			"tracks": []any{
				map[string]any{"id": "T1", "depends_on": []any{}, "slices": []any{baseSlice("S1", "one")}},
				map[string]any{"id": "T2", "depends_on": []any{}, "slices": []any{baseSlice("S2", "two")}},
			},
		}
	}
	parse := func(value map[string]any) error {
		metadata, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		_, err = ParsePlan(append(append([]byte(planOpen), metadata...), []byte(planClose+"body\n")...))
		return err
	}
	cases := map[string]struct {
		mutate func(map[string]any)
		code   string
	}{
		"track cycle": {
			mutate: func(value map[string]any) {
				tracks := value["tracks"].([]any)
				tracks[0].(map[string]any)["depends_on"] = []any{"T2"}
				tracks[1].(map[string]any)["depends_on"] = []any{"T1"}
			},
			code: "DEPENDENCY_CYCLE",
		},
		"parallel overlap": {
			mutate: func(value map[string]any) {
				tracks := value["tracks"].([]any)
				tracks[1].(map[string]any)["slices"].([]any)[0].(map[string]any)["scope"] =
					map[string]any{"include": []any{"one/sub"}, "exclude": []any{}}
			},
			code: "PARALLEL_TOUCH_CONFLICT",
		},
		"unknown field": {
			mutate: func(value map[string]any) { value["foreign"] = true },
			code:   "UNKNOWN_FIELD",
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			value := base()
			test.mutate(value)
			if code := ErrorCode(parse(value)); code != test.code {
				t.Fatalf("code = %q, want %q", code, test.code)
			}
		})
	}
}

func jsonBytesEqual(left, right []byte) bool {
	var l, r any
	return json.Unmarshal(left, &l) == nil && json.Unmarshal(right, &r) == nil && reflect.DeepEqual(l, r)
}

func jsonValueEqual(left, right any) bool {
	l, leftErr := json.Marshal(left)
	r, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && jsonBytesEqual(l, r)
}
