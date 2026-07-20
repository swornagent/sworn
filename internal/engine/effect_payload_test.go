package engine

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/protocol"
	"github.com/swornagent/sworn/internal/repo"
)

const testRuntimeDigest = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

func TestBuildAttemptIdentityIsCanonicalAndAttemptBound(t *testing.T) {
	t.Parallel()

	first, err := BuildAttemptIdentityFor("effect-build-1", 1, testDispatchDigest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildAttemptIdentityFor("effect-build-1", 2, testDispatchDigest)
	if err != nil {
		t.Fatal(err)
	}
	if first.InvocationID == second.InvocationID || !ValidID(first.InvocationID) {
		t.Fatalf("attempt invocation identities = %q, %q", first.InvocationID, second.InvocationID)
	}
	otherConfiguration, err := BuildAttemptIdentityFor(
		"effect-build-1", 1,
		"sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
	)
	if err != nil || first.InvocationID == otherConfiguration.InvocationID {
		t.Fatalf("configuration-bound invocation identity = %q, %v", otherConfiguration.InvocationID, err)
	}
	encoded, err := EncodeBuildAttemptIdentity(first)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseBuildAttemptIdentity(encoded)
	if err != nil || parsed != first {
		t.Fatalf("parsed identity = %+v, %v", parsed, err)
	}
	if _, err := ParseBuildAttemptIdentity(append(json.RawMessage{' '}, encoded...)); err == nil {
		t.Fatal("noncanonical attempt identity was accepted")
	}
	forgedConfiguration := first
	forgedConfiguration.BuilderDispatchDigest = otherConfiguration.BuilderDispatchDigest
	if _, err := EncodeBuildAttemptIdentity(forgedConfiguration); err == nil {
		t.Fatal("forged attempt configuration was accepted without a new invocation identity")
	}
	first.InvocationID = second.InvocationID
	if _, err := EncodeBuildAttemptIdentity(first); err == nil {
		t.Fatal("forged attempt invocation identity was accepted")
	}
}

func TestCheckAttemptIdentityIsCanonicalAndAttemptBound(t *testing.T) {
	t.Parallel()

	first, err := CheckAttemptIdentityFor("effect-check-1", 1, testRuntimeDigest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CheckAttemptIdentityFor("effect-check-1", 2, testRuntimeDigest)
	if err != nil {
		t.Fatal(err)
	}
	if first.InvocationID == second.InvocationID || !ValidID(first.InvocationID) {
		t.Fatalf("check invocation identities = %q, %q", first.InvocationID, second.InvocationID)
	}
	otherRuntime, err := CheckAttemptIdentityFor(
		"effect-check-1", 1,
		"sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
	)
	if err != nil || first.InvocationID == otherRuntime.InvocationID {
		t.Fatalf("runtime-bound invocation identity = %q, %v", otherRuntime.InvocationID, err)
	}
	encoded, err := EncodeCheckAttemptIdentity(first)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCheckAttemptIdentity(encoded)
	if err != nil || parsed != first {
		t.Fatalf("parsed check identity = %+v, %v", parsed, err)
	}
	if _, err := ParseCheckAttemptIdentity(append(json.RawMessage{' '}, encoded...)); err == nil {
		t.Fatal("noncanonical check attempt identity was accepted")
	}
	forged := first
	forged.RuntimeManifestDigest = otherRuntime.RuntimeManifestDigest
	if _, err := EncodeCheckAttemptIdentity(forged); err == nil {
		t.Fatal("forged check runtime was accepted without a new invocation identity")
	}
	first.InvocationID = second.InvocationID
	if _, err := EncodeCheckAttemptIdentity(first); err == nil {
		t.Fatal("forged check invocation identity was accepted")
	}
}

func TestBuildEffectResultCanonicalRoundTrip(t *testing.T) {
	t.Parallel()

	result := validBuildEffectResult()
	encoded, err := EncodeBuildEffectResult(result)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := protocol.CanonicalizeJSON(encoded)
	if err != nil || !bytes.Equal(encoded, canonical) {
		t.Fatalf("encoded result is not canonical: %s, %v", encoded, err)
	}
	parsed, err := ParseBuildEffectResult(encoded)
	if err != nil || parsed.Builder != result.Builder || parsed.Candidate.Ref != result.Candidate.Ref ||
		len(parsed.Candidate.ChangedPaths) != 1 {
		t.Fatalf("parsed result = %+v, %v", parsed, err)
	}

	for name, invalid := range map[string]json.RawMessage{
		"noncanonical": append(json.RawMessage{' '}, encoded...),
		"duplicate":    json.RawMessage(`{"builder":{},"outcome":"candidate_ready","outcome":"candidate_ready","schema_version":"sworn-build-effect-result-v1","candidate":{}}`),
		"trailing":     append(append(json.RawMessage(nil), encoded...), []byte(` {}`)...),
		"unknown": canonicalValue(t, map[string]any{
			"schema_version": result.SchemaVersion, "outcome": result.Outcome, "builder": result.Builder,
			"candidate": result.Candidate, "surprise": true,
		}),
	} {
		if _, err := ParseBuildEffectResult(invalid); err == nil {
			t.Fatalf("%s build result was accepted: %s", name, invalid)
		}
	}
}

func TestBuildEffectResultRejectsInvalidFacts(t *testing.T) {
	t.Parallel()

	mutations := map[string]func(*BuildEffectResult){
		"schema":        func(value *BuildEffectResult) { value.SchemaVersion = "sworn-build-effect-result-v2" },
		"outcome":       func(value *BuildEffectResult) { value.Outcome = "pass" },
		"run id":        func(value *BuildEffectResult) { value.Builder.RunID = "bad id" },
		"agent":         func(value *BuildEffectResult) { value.Builder.Agent = " \t" },
		"oversized":     func(value *BuildEffectResult) { value.Builder.Agent = strings.Repeat("a", MaximumEffectPayloadBytes) },
		"start time":    func(value *BuildEffectResult) { value.Builder.StartedAt = "2026-02-30T00:00:00Z" },
		"time order":    func(value *BuildEffectResult) { value.Builder.StartedAt = "2026-07-20T00:00:02Z" },
		"repository":    func(value *BuildEffectResult) { value.Candidate.RepositoryID = "bad id" },
		"target":        func(value *BuildEffectResult) { value.Candidate.TargetRef = "main" },
		"object":        func(value *BuildEffectResult) { value.Candidate.Tree = strings.Repeat("z", 40) },
		"mixed objects": func(value *BuildEffectResult) { value.Candidate.Tree = strings.Repeat("d", 64) },
		"retention":     func(value *BuildEffectResult) { value.Candidate.Ref = "refs/sworn/v1/candidates/wrong" },
		"nil paths":     func(value *BuildEffectResult) { value.Candidate.ChangedPaths = nil },
		"unsorted": func(value *BuildEffectResult) {
			value.Candidate.ChangedPaths = []string{"z.txt", "a.txt"}
		},
		"duplicate": func(value *BuildEffectResult) {
			value.Candidate.ChangedPaths = []string{"README.md", "README.md"}
		},
		"invalid path": func(value *BuildEffectResult) { value.Candidate.ChangedPaths = []string{"src/../secret"} },
		"change shape": func(value *BuildEffectResult) { value.Candidate.Commit = value.Candidate.BaseCommit },
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result := validBuildEffectResult()
			mutate(&result)
			if _, err := EncodeBuildEffectResult(result); err == nil {
				t.Fatal("invalid build result encoded")
			}
			if _, err := ParseBuildEffectResult(canonicalValue(t, result)); err == nil {
				t.Fatal("invalid canonical build result parsed")
			}
		})
	}
}

func TestLocalCheckEffectRequestStrictRoundTrip(t *testing.T) {
	t.Parallel()

	request := validLocalCheckEffectRequest()
	encoded, err := EncodeLocalCheckEffectRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseLocalCheckEffectRequest(append(json.RawMessage{' ', '\n'}, encoded...))
	if err != nil || parsed != request {
		t.Fatalf("parsed request = %+v, %v", parsed, err)
	}

	invalidValues := []LocalCheckEffectRequest{request, request, request, request, request, request, request, request, request}
	invalidValues[0].SchemaVersion = "sworn-local-check-effect-request-v2"
	invalidValues[1].DeliveryRunID = "bad id"
	invalidValues[2].DeliveryID = ""
	invalidValues[3].WorkID = "bad/id"
	invalidValues[4].WorkAttempt = 0
	invalidValues[5].BuilderEffectID = "bad id"
	invalidValues[6].CheckID = "bad id"
	invalidValues[7].DefinitionDigest = "sha256:no"
	invalidValues[8].RuntimeManifestDigest = strings.ToUpper(testRuntimeDigest)
	for index, invalid := range invalidValues {
		if _, err := EncodeLocalCheckEffectRequest(invalid); err == nil {
			t.Fatalf("invalid request %d encoded", index)
		}
		if _, err := ParseLocalCheckEffectRequest(canonicalValue(t, invalid)); err == nil {
			t.Fatalf("invalid request %d parsed", index)
		}
	}

	for name, invalid := range map[string]json.RawMessage{
		"duplicate": json.RawMessage(`{"schema_version":"sworn-local-check-effect-request-v1","schema_version":"sworn-local-check-effect-request-v1"}`),
		"trailing":  append(append(json.RawMessage(nil), encoded...), []byte(` {}`)...),
		"unknown": canonicalValue(t, map[string]any{
			"schema_version": request.SchemaVersion, "delivery_run_id": request.DeliveryRunID,
			"delivery_id": request.DeliveryID, "work_id": request.WorkID, "work_attempt": request.WorkAttempt,
			"builder_effect_id": request.BuilderEffectID, "check_id": request.CheckID,
			"definition_digest": request.DefinitionDigest, "runtime_manifest_digest": request.RuntimeManifestDigest,
			"surprise": true,
		}),
	} {
		if _, err := ParseLocalCheckEffectRequest(invalid); err == nil {
			t.Fatalf("%s local check request was accepted", name)
		}
	}
}

func TestLocalCheckEffectResultCanonicalAndClosed(t *testing.T) {
	t.Parallel()

	for _, outcome := range []string{LocalCheckOutcomePass, LocalCheckOutcomeNotAdmitted, LocalCheckOutcomeControlled} {
		result := validLocalCheckEffectResult()
		result.Outcome = outcome
		encoded, err := EncodeLocalCheckEffectResult(result)
		if err != nil {
			t.Fatalf("encode %q: %v", outcome, err)
		}
		parsed, err := ParseLocalCheckEffectResult(encoded)
		if err != nil || parsed != result {
			t.Fatalf("parse %q = %+v, %v", outcome, parsed, err)
		}
		if _, err := ParseLocalCheckEffectResult(append(json.RawMessage{'\n'}, encoded...)); err == nil {
			t.Fatalf("noncanonical %q result accepted", outcome)
		}
	}

	invalidValues := []LocalCheckEffectResult{
		validLocalCheckEffectResult(), validLocalCheckEffectResult(), validLocalCheckEffectResult(),
		validLocalCheckEffectResult(), validLocalCheckEffectResult(),
	}
	invalidValues[0].SchemaVersion = "sworn-local-check-effect-result-v2"
	invalidValues[1].Outcome = "fail"
	invalidValues[2].Receipt.Ref = " \t"
	invalidValues[3].Receipt.MediaType = "application/json"
	invalidValues[4].Receipt.Digest = "sha256:no"
	for index, invalid := range invalidValues {
		if _, err := EncodeLocalCheckEffectResult(invalid); err == nil {
			t.Fatalf("invalid result %d encoded", index)
		}
		if _, err := ParseLocalCheckEffectResult(canonicalValue(t, invalid)); err == nil {
			t.Fatalf("invalid result %d parsed", index)
		}
	}

	valid := validLocalCheckEffectResult()
	for name, invalid := range map[string]json.RawMessage{
		"duplicate": json.RawMessage(`{"outcome":"pass","outcome":"pass","receipt":{},"schema_version":"sworn-local-check-effect-result-v1"}`),
		"trailing":  append(canonicalValue(t, valid), []byte(` null`)...),
		"unknown": canonicalValue(t, map[string]any{
			"schema_version": valid.SchemaVersion, "outcome": valid.Outcome, "receipt": valid.Receipt, "surprise": true,
		}),
	} {
		if _, err := ParseLocalCheckEffectResult(invalid); err == nil {
			t.Fatalf("%s local check result was accepted", name)
		}
	}
}

func TestValidateEffectResultBindsKindRequestAndIdentity(t *testing.T) {
	t.Parallel()

	buildRequest := json.RawMessage(`{"schema_version":"sworn-build-effect-request-v1","delivery_run_id":"delivery-run","delivery_id":"delivery-1","work_id":"work-1","work_attempt":1,"dispatch_digest":"` + testDispatchDigest + `"}`)
	buildResult, err := EncodeBuildEffectResult(validBuildEffectResult())
	if err != nil {
		t.Fatal(err)
	}
	checkRequest, err := EncodeLocalCheckEffectRequest(validLocalCheckEffectRequest())
	if err != nil {
		t.Fatal(err)
	}
	checkResult, err := EncodeLocalCheckEffectResult(validLocalCheckEffectResult())
	if err != nil {
		t.Fatal(err)
	}

	if err := ValidateEffectResult(EffectBuild, "effect-build-1", buildRequest, buildResult); err != nil {
		t.Fatalf("validate build: %v", err)
	}
	if err := ValidateEffectResult(EffectLocalCheck, "effect-check-1", checkRequest, checkResult); err != nil {
		t.Fatalf("validate local check: %v", err)
	}
	for name, test := range map[string]struct {
		kind    EffectKind
		id      string
		request json.RawMessage
		result  json.RawMessage
	}{
		"bad id":           {EffectLocalCheck, "bad id", checkRequest, checkResult},
		"builder identity": {EffectBuild, "another-effect", buildRequest, buildResult},
		"build request":    {EffectBuild, "effect-build-1", checkRequest, buildResult},
		"build result":     {EffectBuild, "effect-build-1", buildRequest, checkResult},
		"check request":    {EffectLocalCheck, "effect-check-1", buildRequest, checkResult},
		"check result":     {EffectLocalCheck, "effect-check-1", checkRequest, buildResult},
		"unknown kind":     {EffectKind("unknown"), "effect-1", checkRequest, checkResult},
	} {
		if err := ValidateEffectResult(test.kind, test.id, test.request, test.result); err == nil {
			t.Fatalf("%s binding accepted", name)
		}
	}
}

func TestBuildEffectRequestRejectsDuplicateFields(t *testing.T) {
	t.Parallel()

	duplicate := json.RawMessage(`{"schema_version":"sworn-build-effect-request-v1","delivery_run_id":"run-1","delivery_run_id":"run-1","delivery_id":"delivery-1","work_id":"work-1","work_attempt":1,"dispatch_digest":"` + testDispatchDigest + `"}`)
	if _, err := ParseBuildEffectRequest(duplicate); err == nil {
		t.Fatal("duplicate build request field was accepted")
	}
}

func validBuildEffectResult() BuildEffectResult {
	commit := strings.Repeat("c", 40)
	return BuildEffectResult{
		SchemaVersion: BuildEffectResultSchemaVersion,
		Outcome:       BuildOutcomeCandidateReady,
		Builder: protocol.BuilderRun{
			RunID: "effect-build-1", Agent: "sworn-builder/1", StartedAt: "2026-07-20T00:00:00Z",
			CompletedAt: "2026-07-20T00:00:01.000000001Z",
		},
		Candidate: repo.Candidate{
			RepositoryID: "repo-1", TargetRef: "refs/heads/main", BaseCommit: strings.Repeat("a", 40),
			BaseTree: strings.Repeat("b", 40), Commit: commit, Tree: strings.Repeat("d", 40),
			Ref: "refs/sworn/v1/candidates/" + commit, ChangedPaths: []string{"README.md"},
		},
	}
}

func validLocalCheckEffectRequest() LocalCheckEffectRequest {
	return LocalCheckEffectRequest{
		SchemaVersion: LocalCheckEffectRequestSchemaVersion, DeliveryRunID: "delivery-run", DeliveryID: "delivery-1",
		WorkID: "work-1", WorkAttempt: 1, BuilderEffectID: "effect-build-1", CheckID: "test-1",
		DefinitionDigest: testDispatchDigest, RuntimeManifestDigest: testRuntimeDigest,
	}
}

func validLocalCheckEffectResult() LocalCheckEffectResult {
	return LocalCheckEffectResult{
		SchemaVersion: LocalCheckEffectResultSchemaVersion,
		Outcome:       LocalCheckOutcomePass,
		Receipt: protocol.Artifact{
			Ref: testRuntimeDigest, MediaType: localCheckReceiptMediaType, Digest: testRuntimeDigest,
		},
	}
}

func canonicalValue(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := protocol.EncodeCanonical(value)
	if err != nil {
		t.Fatal(err)
	}
	return json.RawMessage(encoded)
}
