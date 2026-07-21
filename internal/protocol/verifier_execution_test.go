package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestVerifierProfileCanonicalRoundTripAndDigestClosure(t *testing.T) {
	t.Parallel()
	profile := validVerifierProfile(t)
	record, err := EncodeVerifierProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	if record.Kind != VerifierProfileSchemaVersion || record.Digest != CanonicalDigest(record.CanonicalJSON) ||
		len(record.CanonicalJSON) == 0 || len(record.CanonicalJSON) > MaximumVerifierProfileBytes {
		t.Fatalf("profile record = %#v", record)
	}
	parsed, err := ParseVerifierProfile(record.CanonicalJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(parsed.Argv, profile.Argv) || parsed.OutputSchemaDigest != profile.OutputSchemaDigest ||
		parsed.MaterializeBytes != profile.MaterializeBytes {
		t.Fatalf("parsed profile = %#v", parsed)
	}

	changed := profile
	changed.Model = "gpt-changed"
	changed.Argv = CanonicalCodexVerifierArgv(changed.Model)
	changedRecord, err := EncodeVerifierProfile(changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedRecord.Digest == record.Digest {
		t.Fatal("model change did not change verifier profile digest")
	}

	object := testJSONObject(t, record.CanonicalJSON)
	object["unexpected"] = true
	if _, err := ParseVerifierProfile(testJSONBytes(t, object)); err == nil || !strings.Contains(err.Error(), "exact field shape") {
		t.Fatalf("extra-field parse error = %v", err)
	}
	delete(object, "unexpected")
	delete(object, "prompt_digest")
	if _, err := ParseVerifierProfile(testJSONBytes(t, object)); err == nil || !strings.Contains(err.Error(), "exact field shape") {
		t.Fatalf("missing-field parse error = %v", err)
	}
	if _, err := ParseVerifierProfile(bytes.Repeat([]byte{' '}, MaximumVerifierProfileBytes+1)); err == nil {
		t.Fatal("over-ceiling profile was accepted")
	}
}

func TestVerifierProfileRejectsUnsafeOrUnboundFacts(t *testing.T) {
	t.Parallel()
	base := validVerifierProfile(t)
	tests := map[string]func(*VerifierProfile){
		"unknown schema":        func(value *VerifierProfile) { value.SchemaVersion += "-changed" },
		"different agent":       func(value *VerifierProfile) { value.Agent = "other" },
		"relative binary":       func(value *VerifierProfile) { value.BinaryPath = "codex" },
		"invalid binary digest": func(value *VerifierProfile) { value.BinaryDigest = "sha256:no" },
		"zero binary size":      func(value *VerifierProfile) { value.BinarySize = 0 },
		"input collision":       func(value *VerifierProfile) { value.ExecutableInput = "dispatch" },
		"missing provider":      func(value *VerifierProfile) { value.Provider = "" },
		"unsafe model token": func(value *VerifierProfile) {
			value.Model = "--yolo"
			value.Argv = CanonicalCodexVerifierArgv(value.Model)
		},
		"workspace credential overlap": func(value *VerifierProfile) {
			value.WorkspaceRoot = value.CredentialHome + "/work"
		},
		"wrong output schema":      func(value *VerifierProfile) { value.OutputSchemaDigest = testProtocolDigest("f") },
		"no timeout":               func(value *VerifierProfile) { value.TimeoutNanoseconds = 0 },
		"no materialization bytes": func(value *VerifierProfile) { value.MaterializeBytes = 0 },
		"no nested sandbox":        func(value *VerifierProfile) { value.NestedSandbox = false },
		"no credential":            func(value *VerifierProfile) { value.CredentialAccess = false },
		"tool network":             func(value *VerifierProfile) { value.ModelToolNetwork = true },
		"tool credential":          func(value *VerifierProfile) { value.ModelToolCredentialAccess = true },
		"writable workspace":       func(value *VerifierProfile) { value.WorkspaceAccess = "writable_export" },
		"inherited environment":    func(value *VerifierProfile) { value.EnvironmentNames = []string{"TOKEN"} },
		"nil environment":          func(value *VerifierProfile) { value.EnvironmentNames = nil },
		"yolo":                     func(value *VerifierProfile) { value.Argv = append(value.Argv, "--yolo") },
		"long yolo": func(value *VerifierProfile) {
			value.Argv = append(value.Argv, "--dangerously-bypass-approvals-and-sandbox")
		},
		"approval override": func(value *VerifierProfile) {
			value.Argv = append(value.Argv, "-a", "on-request")
		},
		"resume":       func(value *VerifierProfile) { value.Argv = append(value.Argv, "resume") },
		"prompt drift": func(value *VerifierProfile) { value.Argv[len(value.Argv)-1] = "different prompt" },
		"alternate bound prompt": func(value *VerifierProfile) {
			prompt := "A different but digest-bound verifier instruction."
			value.Argv[len(value.Argv)-1] = prompt
			value.PromptDigest = RawDigest([]byte(prompt))
		},
		"missing ephemeral": func(value *VerifierProfile) {
			index := slices.Index(value.Argv, "--ephemeral")
			value.Argv = slices.Delete(slices.Clone(value.Argv), index, index+1)
		},
		"missing memory disable": func(value *VerifierProfile) {
			index := slices.Index(value.Argv, `features.memories=false`)
			value.Argv = slices.Delete(slices.Clone(value.Argv), index-1, index+1)
		},
		"duplicate configuration key": func(value *VerifierProfile) {
			insertVerifierArgumentBeforePrompt(value, "-c", `history.persistence="save-all"`)
		},
		"unique command configuration": func(value *VerifierProfile) {
			insertVerifierArgumentBeforePrompt(value, "-c", `notify=["/bin/true"]`)
		},
		"unique provider configuration": func(value *VerifierProfile) {
			insertVerifierArgumentBeforePrompt(value, "-c", `openai_base_url="https://example.invalid"`)
		},
		"long configuration alias": func(value *VerifierProfile) {
			insertVerifierArgumentBeforePrompt(value, "--config", `features.hooks=true`)
		},
		"feature enable alias": func(value *VerifierProfile) {
			insertVerifierArgumentBeforePrompt(value, "--enable=hooks")
		},
		"equals sandbox override": func(value *VerifierProfile) {
			insertVerifierArgumentBeforePrompt(value, "--sandbox=danger-full-access")
		},
		"equals model override": func(value *VerifierProfile) {
			insertVerifierArgumentBeforePrompt(value, "--model=gpt-changed")
		},
		"equals output override": func(value *VerifierProfile) {
			insertVerifierArgumentBeforePrompt(value, "--output-last-message=/tmp/leak")
		},
		"configuration profile": func(value *VerifierProfile) {
			insertVerifierArgumentBeforePrompt(value, "--profile", "unsafe")
		},
		"image input": func(value *VerifierProfile) {
			insertVerifierArgumentBeforePrompt(value, "--image=/home/sworn/.codex/auth.json")
		},
		"untrusted hook bypass": func(value *VerifierProfile) {
			insertVerifierArgumentBeforePrompt(value, "--dangerously-bypass-hook-trust")
		},
		"extra exec subcommand": func(value *VerifierProfile) {
			insertVerifierArgumentBeforePrompt(value, "review")
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := cloneVerifierProfile(base)
			mutate(&value)
			if _, err := EncodeVerifierProfile(value); err == nil {
				t.Fatalf("unsafe verifier profile accepted: %#v", value)
			}
		})
	}
}

func insertVerifierArgumentBeforePrompt(profile *VerifierProfile, arguments ...string) {
	profile.Argv = slices.Insert(
		slices.Clone(profile.Argv), len(profile.Argv)-1, arguments...,
	)
}

func TestVerifierExecutionReceiptCanonicalRoundTrip(t *testing.T) {
	t.Parallel()
	receipt := validVerifierExecutionReceipt(t)
	record, err := EncodeVerifierExecutionReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if record.Kind != VerifierExecutionReceiptSchemaVersion ||
		record.Digest != CanonicalDigest(record.CanonicalJSON) ||
		len(record.CanonicalJSON) == 0 || len(record.CanonicalJSON) > MaximumVerifierExecutionReceiptBytes {
		t.Fatalf("receipt record = %#v", record)
	}
	parsed, err := ParseVerifierExecutionReceipt(record.CanonicalJSON)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.DispatchDigest != receipt.DispatchDigest || parsed.AssessmentDigest != receipt.AssessmentDigest ||
		parsed.Stdout != receipt.Stdout || parsed.ThreadID != receipt.ThreadID ||
		!slices.Equal(parsed.Inputs, receipt.Inputs) {
		t.Fatalf("parsed receipt = %#v", parsed)
	}

	object := testJSONObject(t, record.CanonicalJSON)
	object["verdict_id"] = "model-must-not-own-this"
	if _, err := ParseVerifierExecutionReceipt(testJSONBytes(t, object)); err == nil ||
		!strings.Contains(err.Error(), "exact field shape") {
		t.Fatalf("authority-field parse error = %v", err)
	}
	if _, err := ParseVerifierExecutionReceipt(append(record.CanonicalJSON, []byte("\n{}")...)); err == nil {
		t.Fatal("second top-level receipt value was accepted")
	}
}

func TestVerifierExecutionReceiptRejectsUnboundOrNonordinaryFacts(t *testing.T) {
	t.Parallel()
	base := validVerifierExecutionReceipt(t)
	tests := map[string]func(*VerifierExecutionReceipt){
		"effect dispatch mismatch": func(value *VerifierExecutionReceipt) { value.EffectID = "effect-other" },
		"invalid attempt":          func(value *VerifierExecutionReceipt) { value.EffectAttempt = 0 },
		"invalid candidate":        func(value *VerifierExecutionReceipt) { value.Candidate.Tree = "bad" },
		"invalid assessment":       func(value *VerifierExecutionReceipt) { value.AssessmentDigest = "sha256:no" },
		"tool network":             func(value *VerifierExecutionReceipt) { value.ModelToolNetwork = true },
		"tool credential":          func(value *VerifierExecutionReceipt) { value.ModelToolCredentialAccess = true },
		"writable workspace":       func(value *VerifierExecutionReceipt) { value.WorkspaceAccess = "writable_export" },
		"unsorted inputs": func(value *VerifierExecutionReceipt) {
			value.Inputs[0], value.Inputs[1] = value.Inputs[1], value.Inputs[0]
		},
		"missing plan": func(value *VerifierExecutionReceipt) {
			value.Inputs = slices.Delete(value.Inputs, 3, 4)
		},
		"wrong schema input": func(value *VerifierExecutionReceipt) {
			value.Inputs[0].Digest = testProtocolDigest("f")
		},
		"wrong executable input": func(value *VerifierExecutionReceipt) {
			value.Inputs[1].Digest = testProtocolDigest("f")
		},
		"duplicate input": func(value *VerifierExecutionReceipt) {
			value.Inputs = append(value.Inputs, value.Inputs[len(value.Inputs)-1])
		},
		"bad stdout media":   func(value *VerifierExecutionReceipt) { value.Stdout.MediaType = "text/plain" },
		"bad stderr pointer": func(value *VerifierExecutionReceipt) { value.Stderr.Digest = "sha256:no" },
		"missing unit":       func(value *VerifierExecutionReceipt) { value.Unit = "" },
		"bad thread":         func(value *VerifierExecutionReceipt) { value.ThreadID = "thread with spaces" },
		"time reversal": func(value *VerifierExecutionReceipt) {
			value.StartedAt, value.CompletedAt = value.CompletedAt, value.StartedAt
		},
		"target not started": func(value *VerifierExecutionReceipt) { value.TargetStarted = false },
		"not quiescent":      func(value *VerifierExecutionReceipt) { value.ServiceQuiescent = false },
		"nonzero exit":       func(value *VerifierExecutionReceipt) { value.ExitCode = 1 },
		"cancelled":          func(value *VerifierExecutionReceipt) { value.Cancelled = true },
		"timed out":          func(value *VerifierExecutionReceipt) { value.TimedOut = true },
		"truncated":          func(value *VerifierExecutionReceipt) { value.OutputTruncated = true },
		"export present":     func(value *VerifierExecutionReceipt) { value.ExportPresent = true },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := base
			value.Inputs = slices.Clone(base.Inputs)
			mutate(&value)
			if _, err := EncodeVerifierExecutionReceipt(value); err == nil {
				t.Fatalf("invalid execution receipt accepted: %#v", value)
			}
		})
	}
}

func TestVerifierAssessmentOutputSchemaIsStrictBoundedAndAssessmentOnly(t *testing.T) {
	t.Parallel()
	contents, err := VerifierAssessmentOutputSchema()
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := VerifierAssessmentOutputSchema()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contents, repeated) || !bytes.Equal(contents, testJSONBytes(t, testJSONObject(t, contents))) {
		t.Fatal("assessment schema is not stable canonical JSON")
	}
	digest, err := VerifierAssessmentOutputSchemaDigest()
	if err != nil {
		t.Fatal(err)
	}
	if digest != RawDigest(contents) || !ValidDigest(digest) {
		t.Fatalf("assessment schema digest = %q", digest)
	}
	for _, forbidden := range [][]byte{
		[]byte(`"verdict_id"`), []byte(`"submission_id"`), []byte(`"dispatch_id"`),
		[]byte(`"agent"`), []byte(`"fresh_context"`), []byte(`"started_at"`),
	} {
		if bytes.Contains(contents, forbidden) {
			t.Fatalf("assessment schema grants forbidden field %s", forbidden)
		}
	}

	assessment := VerifierAssessment{
		SchemaVersion: VerifierAssessmentSchemaVersion,
		Outcome:       "PASS",
		Summary:       `The "exact" candidate satisfies the \\ contract.`,
		AcceptanceResults: []AcceptanceResult{{
			AcceptanceID: "AC1", Outcome: "pass", EvidenceIDs: []string{"evidence-1"}, Summary: `Observed "exactly".`,
		}},
		AssuranceResults: []AssuranceResult{},
		Findings:         []Finding{},
	}
	assessmentBytes := testJSONBytes(t, assessment)
	if _, err := ParseVerifierAssessment(assessmentBytes); err != nil {
		t.Fatalf("protocol parser rejected schema fixture: %v", err)
	}
	schema := testJSONObject(t, contents)
	if err := validateVerifierSchemaFixture(schema, testJSONObject(t, assessmentBytes)); err != nil {
		t.Fatalf("output schema rejected protocol assessment: %v", err)
	}
	withEnvelope := testJSONObject(t, assessmentBytes)
	withEnvelope["verdict_id"] = "verdict-1"
	if err := validateVerifierSchemaFixture(schema, withEnvelope); err == nil {
		t.Fatal("output schema accepted a model-owned verdict id")
	}
	tooLong := testJSONObject(t, assessmentBytes)
	tooLong["summary"] = strings.Repeat("x", maximumVerifierAssessmentSummaryCodePoints+1)
	if err := validateVerifierSchemaFixture(schema, tooLong); err == nil {
		t.Fatal("output schema accepted an oversized root summary")
	}
}

func TestVerifierAssessmentOutputSchemaMaximumFitsOneJSONLEvent(t *testing.T) {
	t.Parallel()
	schemaBytes, err := VerifierAssessmentOutputSchema()
	if err != nil {
		t.Fatal(err)
	}
	schema := testJSONObject(t, schemaBytes)
	references := make([]any, maximumVerifierAssessmentReferenceItems)
	for index := range references {
		references[index] = strings.Repeat("e", 125) + fmt.Sprintf("%03d", index)
	}
	nestedSummary := strings.Repeat(`"\`, maximumVerifierResultSummaryCodePoints/2)
	acceptance := make([]any, maximumVerifierAssessmentCollectionItems)
	assurance := make([]any, maximumVerifierAssessmentCollectionItems)
	findings := make([]any, maximumVerifierAssessmentCollectionItems)
	for index := 0; index < maximumVerifierAssessmentCollectionItems; index++ {
		acceptance[index] = map[string]any{
			"acceptance_id": "A" + strings.Repeat("a", 124) + fmt.Sprintf("%03d", index),
			"outcome":       "inconclusive", "evidence_ids": references, "summary": nestedSummary,
		}
		assurance[index] = map[string]any{
			"pack":    strings.Repeat("p", 64) + "@" + strings.Repeat("v", 29) + fmt.Sprintf("%03d", index),
			"outcome": "inconclusive", "evidence_ids": references, "summary": nestedSummary,
		}
		findings[index] = map[string]any{
			"id":   "F" + strings.Repeat("f", 124) + fmt.Sprintf("%03d", index),
			"kind": "environment", "principle": "B4", "severity": "blocking", "summary": nestedSummary,
			"acceptance_ids": references, "evidence_ids": references,
		}
	}
	maximal := map[string]any{
		"schema_version": VerifierAssessmentSchemaVersion, "outcome": "INCONCLUSIVE",
		"summary": strings.Repeat(`"\`, maximumVerifierAssessmentSummaryCodePoints/2), "acceptance_results": acceptance,
		"assurance_results": assurance, "findings": findings,
	}
	if err := validateVerifierSchemaFixture(schema, maximal); err != nil {
		t.Fatalf("schema rejected its bounded maximum fixture: %v", err)
	}
	assessment, err := json.Marshal(maximal)
	if err != nil {
		t.Fatal(err)
	}
	event, err := json.Marshal(map[string]any{
		"type": "item.completed",
		"item": map[string]any{"id": "item-1", "type": "agent_message", "text": string(assessment)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(event) > MaximumVerifierAssessmentBytes {
		t.Fatalf("maximum schema-valid JSONL event is %d bytes, ceiling %d", len(event), MaximumVerifierAssessmentBytes)
	}
}

func validVerifierProfile(t testing.TB) VerifierProfile {
	t.Helper()
	outputSchemaDigest, err := VerifierAssessmentOutputSchemaDigest()
	if err != nil {
		t.Fatal(err)
	}
	return VerifierProfile{
		SchemaVersion: VerifierProfileSchemaVersion,
		Agent:         "codex-cli 0.145.0-alpha.18", BinaryPath: "/opt/sworn/codex",
		BinaryVersion: "codex-cli 0.145.0-alpha.18", BinaryDigest: testProtocolDigest("a"), BinarySize: 1024,
		ExecutableInput: "codex", Provider: "openai", Authentication: "codex-cli-chatgpt-file-v1",
		CredentialHome: "/home/sworn/.codex", PermissionProfile: "sworn_verifier", Model: "gpt-test",
		ToolSchemaDigest: testProtocolDigest("b"),
		Argv:             CanonicalCodexVerifierArgv("gpt-test"),
		EnvironmentNames: []string{}, PromptDigest: RawDigest([]byte(NativeCodexVerifierPrompt)), OutputSchemaDigest: outputSchemaDigest,
		TimeoutNanoseconds: 60_000_000_000, Network: "host", WorkspaceAccess: "read_only",
		NestedSandbox: true, CredentialAccess: true, ModelToolNetwork: false, ModelToolCredentialAccess: false,
		ExecutorConfigurationDigest: testProtocolDigest("d"), RepositoryID: "repository-1",
		WorkspaceRoot: "/srv/sworn/verifier-workspaces", MaterializeBytes: 1 << 20, MaterializeEntries: 1024,
	}
}

func validVerifierExecutionReceipt(t testing.TB) VerifierExecutionReceipt {
	t.Helper()
	assessmentSchemaDigest, err := VerifierAssessmentOutputSchemaDigest()
	if err != nil {
		t.Fatal(err)
	}
	return VerifierExecutionReceipt{
		SchemaVersion: VerifierExecutionReceiptSchemaVersion,
		EffectID:      "verifier-effect-1", EffectAttempt: 1, InvocationID: "verifier-attempt-1",
		DeliveryRunID: "delivery-run-1", DeliveryID: "delivery-1", WorkID: "work-1", WorkAttempt: 1,
		PlanDigest: testProtocolDigest("a"), SubmissionID: "submission-1", SubmissionDigest: testProtocolDigest("b"),
		Candidate: CandidatePoint{
			Repository: "repository-1", Commit: strings.Repeat("c", 40), Tree: strings.Repeat("d", 40),
		},
		DispatchID: "verifier-effect-1", DispatchDigest: testProtocolDigest("e"),
		VerifierProfileDigest: testProtocolDigest("f"), Agent: "codex-cli 0.145.0-alpha.18", VerificationEpoch: 1,
		ExecutorConfigurationDigest: testProtocolDigest("1"), ExecutableInput: "codex",
		ExecutableDigest: testProtocolDigest("2"), WorkspaceDigest: testProtocolDigest("3"), WorkspaceAccess: "read_only",
		Inputs: []VerifierExecutionInput{
			{Name: "assessment-schema", Digest: assessmentSchemaDigest, Size: 4096},
			{Name: "codex", Digest: testProtocolDigest("2"), Size: 1024},
			{Name: "dispatch", Digest: testProtocolDigest("e"), Size: 512},
			{Name: "plan", Digest: testProtocolDigest("a"), Size: 1024},
			{Name: "submission", Digest: testProtocolDigest("b"), Size: 2048},
		},
		Network: "host", NestedSandbox: true, CredentialAccess: true,
		ModelToolNetwork: false, ModelToolCredentialAccess: false, AssessmentDigest: testProtocolDigest("4"),
		Stdout: CapturedArtifact{
			Ref: testProtocolDigest("5"), MediaType: "application/octet-stream", Digest: testProtocolDigest("5"), Size: 4096,
		},
		Stderr: CapturedArtifact{
			Ref: testProtocolDigest("6"), MediaType: "application/octet-stream", Digest: testProtocolDigest("6"), Size: 0,
		},
		Unit: "sworn-verifier-attempt-1.service", ThreadID: "thread-1",
		StartedAt: "2026-07-22T00:00:00Z", CompletedAt: "2026-07-22T00:00:01Z",
		TargetStarted: true, ServiceQuiescent: true, ExitCode: 0,
		Cancelled: false, TimedOut: false, OutputTruncated: false, ExportPresent: false,
	}
}

// validateVerifierSchemaFixture is intentionally small: it implements only the
// JSON Schema keywords emitted above so tests prove the generated contract and
// its protocol fixture remain aligned without adding a runtime schema engine.
func validateVerifierSchemaFixture(schema map[string]any, value any) error {
	typeName, _ := schema["type"].(string)
	switch typeName {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("want object, got %T", value)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			return fmt.Errorf("object schema lacks properties")
		}
		if additional, _ := schema["additionalProperties"].(bool); !additional {
			for name := range object {
				if _, exists := properties[name]; !exists {
					return fmt.Errorf("unexpected property %q", name)
				}
			}
		}
		for _, required := range schemaStringArray(schema["required"]) {
			if _, exists := object[required]; !exists {
				return fmt.Errorf("missing property %q", required)
			}
		}
		for name, child := range properties {
			childSchema, ok := child.(map[string]any)
			if !ok {
				return fmt.Errorf("property %q schema is %T", name, child)
			}
			if childValue, exists := object[name]; exists {
				if err := validateVerifierSchemaFixture(childSchema, childValue); err != nil {
					return fmt.Errorf("property %q: %w", name, err)
				}
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("want array, got %T", value)
		}
		if minimum, exists := schemaInteger(schema["minItems"]); exists && len(array) < minimum {
			return fmt.Errorf("array has %d items, minimum %d", len(array), minimum)
		}
		if maximum, exists := schemaInteger(schema["maxItems"]); exists && len(array) > maximum {
			return fmt.Errorf("array has %d items, maximum %d", len(array), maximum)
		}
		itemSchema, ok := schema["items"].(map[string]any)
		if !ok {
			return fmt.Errorf("array schema lacks items")
		}
		for index, item := range array {
			if err := validateVerifierSchemaFixture(itemSchema, item); err != nil {
				return fmt.Errorf("item %d: %w", index, err)
			}
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("want string, got %T", value)
		}
		if minimum, exists := schemaInteger(schema["minLength"]); exists && len([]rune(text)) < minimum {
			return fmt.Errorf("string is shorter than %d", minimum)
		}
		if maximum, exists := schemaInteger(schema["maxLength"]); exists && len([]rune(text)) > maximum {
			return fmt.Errorf("string is longer than %d", maximum)
		}
		if pattern, exists := schema["pattern"].(string); exists {
			matched, err := regexp.MatchString(pattern, text)
			if err != nil || !matched {
				return fmt.Errorf("string does not match %q", pattern)
			}
		}
		if choices := schemaStringArray(schema["enum"]); len(choices) != 0 && !slices.Contains(choices, text) {
			return fmt.Errorf("string %q is outside enum", text)
		}
	default:
		return fmt.Errorf("unsupported schema type %q", typeName)
	}
	return nil
}

func schemaStringArray(value any) []string {
	array, _ := value.([]any)
	result := make([]string, 0, len(array))
	for _, item := range array {
		text, ok := item.(string)
		if !ok {
			return nil
		}
		result = append(result, text)
	}
	return result
}

func schemaInteger(value any) (int, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := strconv.Atoi(number.String())
		return parsed, err == nil
	case float64:
		return int(number), number == float64(int(number))
	default:
		return 0, false
	}
}
