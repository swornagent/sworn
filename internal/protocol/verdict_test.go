package protocol

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseDeliveryVerdictMatchesBatonSnapshot(t *testing.T) {
	t.Parallel()

	contents := testSnapshotBytes(t, "examples/pass-verdict.json")
	verdict, err := ParseDeliveryVerdict(contents)
	if err != nil {
		t.Fatal(err)
	}
	const wantDigest = "sha256:4f1e638be19a8fa258aed350a10006a9eca169bf98952d4bbed8e4e3edf5dc0d"
	canonical, err := CanonicalizeJSON(contents)
	if err != nil {
		t.Fatal(err)
	}
	if CanonicalDigest(canonical) != wantDigest || verdict.Outcome != "PASS" ||
		verdict.Review.RunID != "verifier-run-1" {
		t.Fatalf("verdict = %#v, digest = %s", verdict, CanonicalDigest(canonical))
	}
	if verdict.AssuranceResults == nil || verdict.Findings == nil {
		t.Fatal("required empty verdict arrays lost their array shape")
	}
	reencoded, err := EncodeCanonical(verdict)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonical, reencoded) {
		t.Fatal("parsed verdict no longer represents the published record")
	}
}

func TestParseDeliveryVerdictRejectsShapeAndLocalSemanticDrift(t *testing.T) {
	t.Parallel()

	contents := testSnapshotBytes(t, "examples/pass-verdict.json")
	mutations := map[string]func(map[string]any){
		"unknown root": func(root map[string]any) {
			root["profile"] = "trusted"
		},
		"missing fresh context": func(root map[string]any) {
			delete(testNestedObject(t, root, "review"), "fresh_context")
		},
		"false fresh context": func(root map[string]any) {
			testNestedObject(t, root, "review")["fresh_context"] = false
		},
		"unknown review field": func(root map[string]any) {
			testNestedObject(t, root, "review")["model"] = "gpt"
		},
		"missing dispatch digest": func(root map[string]any) {
			review := testNestedObject(t, root, "review")
			delete(testNestedObject(t, review, "dispatch_receipt"), "digest")
		},
		"unknown acceptance result field": func(root map[string]any) {
			result := testArrayObject(t, testObjectArray(t, root, "acceptance_results"), 0)
			result["confidence"] = 1
		},
		"null assurance results": func(root map[string]any) {
			root["assurance_results"] = nil
		},
		"null findings": func(root map[string]any) {
			root["findings"] = nil
		},
		"reversed review timestamps": func(root map[string]any) {
			review := testNestedObject(t, root, "review")
			review["started_at"] = "2026-07-19T00:08:00Z"
		},
		"PASS result without evidence": func(root map[string]any) {
			result := testArrayObject(t, testObjectArray(t, root, "acceptance_results"), 0)
			result["evidence_ids"] = []any{}
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := testJSONObject(t, contents)
			mutate(root)
			if _, err := ParseDeliveryVerdict(testJSONBytes(t, root)); err == nil {
				t.Fatal("invalid delivery verdict was accepted")
			}
		})
	}
}

func TestParseVerifierAssessmentRejectsEnvelopeAndShapeSmuggling(t *testing.T) {
	t.Parallel()

	_, _, assessment := testVerdictChain(t)
	contents := testJSONBytes(t, assessment)
	parsed, err := ParseVerifierAssessment(contents)
	if err != nil {
		t.Fatal(err)
	}
	parsedView := parsed.View()
	if parsedView.AssuranceResults == nil || parsedView.Findings == nil {
		t.Fatal("required empty assessment arrays lost their array shape")
	}
	parsedRecord := parsed.Record()
	canonical, err := CanonicalizeJSON(contents)
	if err != nil {
		t.Fatal(err)
	}
	if parsedRecord.Kind != VerifierAssessmentSchemaVersion ||
		parsedRecord.Digest != CanonicalDigest(canonical) ||
		!bytes.Equal(parsedRecord.CanonicalJSON, canonical) {
		t.Fatal("exact assessment did not retain its canonical record")
	}
	parsedRecord.CanonicalJSON[0] = 'x'
	parsedView.AcceptanceResults[0].EvidenceIDs[0] = "mutated"
	if parsed.Record().CanonicalJSON[0] != '{' ||
		parsed.View().AcceptanceResults[0].EvidenceIDs[0] != "health-smoke" {
		t.Fatal("caller mutation escaped the exact assessment capability")
	}

	mutations := map[string]func(map[string]any){
		"verdict id": func(root map[string]any) {
			root["verdict_id"] = "model-owned-verdict"
		},
		"submission digest": func(root map[string]any) {
			root["submission_digest"] = strings.Repeat("0", 71)
		},
		"review envelope": func(root map[string]any) {
			root["review"] = map[string]any{"agent": "model"}
		},
		"timestamp": func(root map[string]any) {
			root["completed_at"] = "2026-07-19T00:07:00Z"
		},
		"missing schema": func(root map[string]any) {
			delete(root, "schema_version")
		},
		"missing assurance results": func(root map[string]any) {
			delete(root, "assurance_results")
		},
		"null assurance results": func(root map[string]any) {
			root["assurance_results"] = nil
		},
		"missing findings": func(root map[string]any) {
			delete(root, "findings")
		},
		"null findings": func(root map[string]any) {
			root["findings"] = nil
		},
		"empty acceptance results": func(root map[string]any) {
			root["acceptance_results"] = []any{}
		},
		"null acceptance results": func(root map[string]any) {
			root["acceptance_results"] = nil
		},
		"unknown result field": func(root map[string]any) {
			result := testArrayObject(t, testObjectArray(t, root, "acceptance_results"), 0)
			result["confidence"] = 1
		},
		"missing result evidence": func(root map[string]any) {
			result := testArrayObject(t, testObjectArray(t, root, "acceptance_results"), 0)
			delete(result, "evidence_ids")
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := testJSONObject(t, contents)
			mutate(root)
			if _, err := ParseVerifierAssessment(testJSONBytes(t, root)); err == nil {
				t.Fatal("assessment shape smuggling was accepted")
			}
		})
	}
	if _, err := ParseVerifierAssessment(append([]byte("```json\n"), append(contents, []byte("\n```")...)...)); err == nil {
		t.Fatal("markdown-wrapped assessment was accepted")
	}
	if _, err := ParseVerifierAssessment(append([]byte("assessment: "), contents...)); err == nil {
		t.Fatal("prose-wrapped assessment was accepted")
	}
}

func TestVerifierAssessmentOutcomeMatrix(t *testing.T) {
	t.Parallel()

	_, _, base := testVerdictChain(t)
	failWithEmptyEvidence := testAssessmentWithFindings(
		base, "FAIL", testFinding("failure-1", "evidence", "blocking"),
	)
	failWithEmptyEvidence.AcceptanceResults[0].Outcome = "fail"
	failWithEmptyEvidence.AcceptanceResults[0].EvidenceIDs = []string{}
	passAssuranceWithoutEvidence := cloneVerifierAssessment(base)
	passAssuranceWithoutEvidence.AssuranceResults = []AssuranceResult{{
		Pack: "security@1", Outcome: "pass", EvidenceIDs: []string{}, Summary: "No evidence.",
	}}
	valid := map[string]VerifierAssessment{
		"PASS": base,
		"PASS with non-blocking finding": testAssessmentWithFindings(
			base, "PASS", testFinding("note-1", "contract", "non_blocking"),
		),
		"FAIL with evidence blocker": testAssessmentWithFindings(
			base, "FAIL", testFinding("failure-1", "evidence", "blocking"),
		),
		"FAIL with empty result evidence": failWithEmptyEvidence,
		"SPEC_BLOCK with upstream and delivery blockers": testAssessmentWithFindings(
			base, "SPEC_BLOCK",
			testFinding("spec-1", "authority", "blocking"),
			testFinding("implementation-1", "implementation", "blocking"),
		),
		"SPEC_BLOCK with contract blocker": testAssessmentWithFindings(
			base, "SPEC_BLOCK", testFinding("contract-1", "contract", "blocking"),
		),
		"INCONCLUSIVE with environment and delivery blockers": testAssessmentWithFindings(
			base, "INCONCLUSIVE",
			testFinding("environment-1", "environment", "blocking"),
			testFinding("implementation-1", "implementation", "blocking"),
		),
		"INCONCLUSIVE with evidence blocker": testAssessmentWithFindings(
			base, "INCONCLUSIVE", testFinding("evidence-1", "evidence", "blocking"),
		),
	}
	for name, assessment := range valid {
		assessment := assessment
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			parsed, err := ParseVerifierAssessment(testJSONBytes(t, assessment))
			if err != nil {
				t.Fatal(err)
			}
			parsedView := parsed.View()
			for _, result := range parsedView.AcceptanceResults {
				if result.EvidenceIDs == nil {
					t.Fatal("valid empty result evidence lost its array shape")
				}
			}
			for _, finding := range parsedView.Findings {
				if finding.AcceptanceIDs == nil || finding.EvidenceIDs == nil {
					t.Fatal("valid empty finding references lost their array shape")
				}
			}
		})
	}

	passFailure := cloneVerifierAssessment(base)
	passFailure.AcceptanceResults[0].Outcome = "fail"
	passNoEvidence := cloneVerifierAssessment(base)
	passNoEvidence.AcceptanceResults[0].EvidenceIDs = []string{}
	invalid := map[string]VerifierAssessment{
		"PASS with failed result":         passFailure,
		"PASS without evidence":           passNoEvidence,
		"PASS assurance without evidence": passAssuranceWithoutEvidence,
		"PASS with blocking finding": testAssessmentWithFindings(
			base, "PASS", testFinding("failure-1", "implementation", "blocking"),
		),
		"FAIL without blocker": testAssessmentWithFindings(base, "FAIL"),
		"FAIL with environment blocker": testAssessmentWithFindings(
			base, "FAIL",
			testFinding("failure-1", "implementation", "blocking"),
			testFinding("environment-1", "environment", "blocking"),
		),
		"SPEC_BLOCK without upstream blocker": testAssessmentWithFindings(
			base, "SPEC_BLOCK", testFinding("failure-1", "implementation", "blocking"),
		),
		"INCONCLUSIVE without verifier blocker": testAssessmentWithFindings(
			base, "INCONCLUSIVE", testFinding("failure-1", "implementation", "blocking"),
		),
		"INCONCLUSIVE with upstream blocker": testAssessmentWithFindings(
			base, "INCONCLUSIVE",
			testFinding("environment-1", "environment", "blocking"),
			testFinding("authority-1", "authority", "blocking"),
		),
	}
	for name, assessment := range invalid {
		assessment := assessment
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseVerifierAssessment(testJSONBytes(t, assessment)); err == nil {
				t.Fatal("incoherent outcome was accepted")
			}
		})
	}
}

func TestVerifierAssessmentRejectsDuplicateSemanticIdentities(t *testing.T) {
	t.Parallel()

	_, _, base := testVerdictChain(t)
	duplicateAcceptance := cloneVerifierAssessment(base)
	duplicateAcceptance.AcceptanceResults = append(
		duplicateAcceptance.AcceptanceResults, duplicateAcceptance.AcceptanceResults[0],
	)
	duplicateEvidence := cloneVerifierAssessment(base)
	duplicateEvidence.AcceptanceResults[0].EvidenceIDs = []string{"health-smoke", "health-smoke"}
	duplicatePacks := cloneVerifierAssessment(base)
	packResult := AssuranceResult{
		Pack: "security@1", Outcome: "pass", EvidenceIDs: []string{"health-smoke"}, Summary: "Pass.",
	}
	duplicatePacks.AssuranceResults = []AssuranceResult{packResult, packResult}
	duplicateFindings := cloneVerifierAssessment(base)
	finding := testFinding("note-1", "evidence", "non_blocking")
	duplicateFindings.Findings = []Finding{finding, finding}
	duplicateFindingRefs := cloneVerifierAssessment(base)
	finding = testFinding("note-1", "evidence", "non_blocking")
	finding.EvidenceIDs = []string{"health-smoke", "health-smoke"}
	duplicateFindingRefs.Findings = []Finding{finding}

	for name, assessment := range map[string]VerifierAssessment{
		"acceptance result": duplicateAcceptance,
		"result evidence":   duplicateEvidence,
		"assurance result":  duplicatePacks,
		"finding":           duplicateFindings,
		"finding evidence":  duplicateFindingRefs,
	} {
		if _, err := ParseVerifierAssessment(testJSONBytes(t, assessment)); err == nil {
			t.Errorf("duplicate %s identity was accepted", name)
		}
	}
}

func TestBuildDeliveryVerdictStampsExactPublishedEnvelope(t *testing.T) {
	t.Parallel()

	input, published, assessment := testVerdictChain(t)
	stamp := testVerdictStamp(published)
	exactAssessment := testExactAssessment(t, assessment)
	if _, err := BuildDeliveryVerdict(input, stamp, ExactVerifierAssessment{}); err == nil {
		t.Fatal("verdict without an exact assessment capability was accepted")
	}
	badDispatchDigest := input
	badDispatchDigest.DispatchReceipt.Digest = "sha256:" + strings.Repeat("0", 64)
	if _, err := BuildDeliveryVerdict(badDispatchDigest, stamp, exactAssessment); err == nil {
		t.Fatal("dispatch pointer with the wrong raw digest was accepted")
	}
	record, err := BuildDeliveryVerdict(input, stamp, exactAssessment)
	if err != nil {
		t.Fatal(err)
	}
	const wantDigest = "sha256:4f1e638be19a8fa258aed350a10006a9eca169bf98952d4bbed8e4e3edf5dc0d"
	if record.Kind != DeliveryVerdictSchemaVersion || record.Digest != wantDigest {
		t.Fatalf("record = %#v, want published digest %q", record, wantDigest)
	}
	second, err := BuildDeliveryVerdict(input, stamp, exactAssessment)
	if err != nil {
		t.Fatal(err)
	}
	if record.Digest != second.Digest || !bytes.Equal(record.CanonicalJSON, second.CanonicalJSON) {
		t.Fatal("identical verdict inputs did not produce deterministic bytes")
	}
	built, err := ParseDeliveryVerdict(record.CanonicalJSON)
	if err != nil {
		t.Fatal(err)
	}
	if built.Review.DispatchReceipt != input.DispatchReceipt {
		t.Fatal("engine-stamped verdict changed the exact dispatch pointer")
	}
	if err := ValidateDeliveryVerdictBindings(input, built); err != nil {
		t.Fatal(err)
	}

	assessment.AcceptanceResults[0].EvidenceIDs[0] = "mutated"
	record.CanonicalJSON[0] = 'x'
	third, err := BuildDeliveryVerdict(input, stamp, exactAssessment)
	if err != nil || third.Digest != wantDigest || third.CanonicalJSON[0] != '{' {
		t.Fatalf("caller mutation changed deterministic construction: %#v, %v", third, err)
	}
}

func TestValidateDeliveryVerdictBindingsRejectsCrossRecordDrift(t *testing.T) {
	t.Parallel()

	input, published, assessment := testVerdictChain(t)
	record, err := BuildDeliveryVerdict(input, testVerdictStamp(published), testExactAssessment(t, assessment))
	if err != nil {
		t.Fatal(err)
	}
	valid, err := ParseDeliveryVerdict(record.CanonicalJSON)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*DeliveryVerdict){
		"submission id": func(verdict *DeliveryVerdict) {
			verdict.SubmissionID = "different-submission"
		},
		"submission digest": func(verdict *DeliveryVerdict) {
			verdict.SubmissionDigest = "sha256:" + strings.Repeat("0", 64)
		},
		"delivery id": func(verdict *DeliveryVerdict) {
			verdict.DeliveryID = "different-delivery"
		},
		"work id": func(verdict *DeliveryVerdict) {
			verdict.WorkID = "different-work"
		},
		"dispatch locator": func(verdict *DeliveryVerdict) {
			verdict.Review.DispatchReceipt.Ref = "other-dispatch.json"
		},
		"dispatch digest": func(verdict *DeliveryVerdict) {
			verdict.Review.DispatchReceipt.Digest = "sha256:" + strings.Repeat("0", 64)
		},
		"dispatch run": func(verdict *DeliveryVerdict) {
			verdict.Review.RunID = "another-verifier-run"
		},
		"review precedes dispatch": func(verdict *DeliveryVerdict) {
			verdict.Review.StartedAt = "2026-07-19T00:05:54Z"
		},
		"acceptance set": func(verdict *DeliveryVerdict) {
			verdict.AcceptanceResults[0].AcceptanceID = "AC2"
		},
		"unknown result evidence": func(verdict *DeliveryVerdict) {
			verdict.AcceptanceResults[0].EvidenceIDs[0] = "unknown-evidence"
		},
		"unknown finding acceptance": func(verdict *DeliveryVerdict) {
			finding := testFinding("note-1", "evidence", "non_blocking")
			finding.AcceptanceIDs = []string{"AC2"}
			verdict.Findings = []Finding{finding}
		},
		"unknown finding evidence": func(verdict *DeliveryVerdict) {
			finding := testFinding("note-1", "evidence", "non_blocking")
			finding.EvidenceIDs = []string{"unknown-evidence"}
			verdict.Findings = []Finding{finding}
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			verdict := cloneDeliveryVerdict(valid)
			mutate(&verdict)
			if err := ValidateDeliveryVerdictBindings(input, verdict); err == nil {
				t.Fatal("cross-record drift was accepted")
			}
		})
	}

	huge := cloneDeliveryVerdict(valid)
	huge.Summary = strings.Repeat("x", MaximumDeliveryVerdictBytes+1)
	if err := ValidateDeliveryVerdictBindings(input, huge); err == nil {
		t.Fatal("programmatic verdict above the byte ceiling was accepted")
	}
}

func TestBuildDeliveryVerdictRejectsMutatedInputs(t *testing.T) {
	t.Parallel()

	input, published, assessment := testVerdictChain(t)
	stamp := testVerdictStamp(published)
	exactAssessment := testExactAssessment(t, assessment)

	planRoot := testJSONObject(t, testSnapshotBytes(t, "examples/standard-plan.json"))
	planRoot["outcome"] = "A different exact plan outcome."
	otherPlan, err := ParseDeliveryPlan(testJSONBytes(t, planRoot))
	if err != nil {
		t.Fatal(err)
	}
	planMismatch := input
	planMismatch.Plan = otherPlan
	if _, err := BuildDeliveryVerdict(planMismatch, stamp, exactAssessment); err == nil {
		t.Fatal("submission bound to another plan was accepted")
	}

	for name, mutate := range map[string]func(map[string]any){
		"contract digest": func(root map[string]any) {
			root["contract_digest"] = "sha256:" + strings.Repeat("0", 64)
		},
		"policy ref": func(root map[string]any) {
			testNestedObject(t, root, "assurance")["policy_ref"] = "another-policy.json"
		},
		"non-passing check": func(root map[string]any) {
			check := testArrayObject(t, testObjectArray(t, root, "checks"), 0)
			check["outcome"] = "fail"
		},
		"unknown acceptance evidence": func(root map[string]any) {
			evidence := testArrayObject(t, testObjectArray(t, root, "evidence"), 0)
			evidence["acceptance_ids"] = []any{"AC2"}
		},
		"weak acceptance evidence": func(root map[string]any) {
			evidence := testArrayObject(t, testObjectArray(t, root, "evidence"), 0)
			evidence["boundary"] = "component"
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := testJSONObject(t, testSnapshotBytes(t, "examples/standard-submission.json"))
			mutate(root)
			submission, err := ParseSubmission(testJSONBytes(t, root))
			if err != nil {
				t.Fatal(err)
			}
			mutated := testBindingForSubmission(t, input.Plan, submission, "verifier-run-mutated")
			if _, err := BuildDeliveryVerdict(mutated, stamp, exactAssessment); err == nil {
				t.Fatal("mutated submission was accepted")
			}
		})
	}

	for name, mutate := range map[string]func(map[string]any){
		"submission digest": func(root map[string]any) {
			root["submission_digest"] = "sha256:" + strings.Repeat("0", 64)
		},
		"candidate": func(root map[string]any) {
			testNestedObject(t, root, "candidate")["tree"] = strings.Repeat("d", 40)
		},
		"early dispatch": func(root map[string]any) {
			root["created_at"] = "2026-07-19T00:04:59Z"
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := testJSONObject(t, input.Dispatch)
			mutate(root)
			dispatch := testJSONBytes(t, root)
			mutated := input
			mutated.Dispatch = dispatch
			mutated.DispatchReceipt.Digest = RawDigest(dispatch)
			if _, err := BuildDeliveryVerdict(mutated, stamp, exactAssessment); err == nil {
				t.Fatal("mutated dispatch was accepted")
			}
		})
	}
}

func TestBuildDeliveryVerdictBindsSelectedAssurancePacks(t *testing.T) {
	t.Parallel()

	planRoot := testJSONObject(t, testSnapshotBytes(t, "examples/standard-plan.json"))
	work := testArrayObject(t, testObjectArray(t, planRoot, "work"), 0)
	assurance := testNestedObject(t, work, "assurance")
	assurance["profile"] = "assured"
	assurance["packs"] = []any{"security@1"}
	plan, err := ParseDeliveryPlan(testJSONBytes(t, planRoot))
	if err != nil {
		t.Fatal(err)
	}
	contract, exists := plan.Work("health-endpoint")
	if !exists {
		t.Fatal("assured plan lost its work contract")
	}

	submissionRoot := testJSONObject(t, testSnapshotBytes(t, "examples/standard-submission.json"))
	submissionRoot["plan_digest"] = plan.Record().Digest
	submissionRoot["contract_digest"] = contract.Digest()
	submissionAssurance := testNestedObject(t, submissionRoot, "assurance")
	submissionAssurance["profile"] = "assured"
	submissionAssurance["packs"] = []any{"security@1"}
	evidence := testArrayObject(t, testObjectArray(t, submissionRoot, "evidence"), 0)
	evidence["pack_ids"] = []any{"security@1"}
	submission, err := ParseSubmission(testJSONBytes(t, submissionRoot))
	if err != nil {
		t.Fatal(err)
	}
	input := testBindingForSubmission(t, plan, submission, "verifier-run-assured")
	_, published, assessment := testVerdictChain(t)
	stamp := testVerdictStamp(published)
	stamp.VerdictID = "verdict-assured"

	if _, err := BuildDeliveryVerdict(input, stamp, testExactAssessment(t, assessment)); err == nil {
		t.Fatal("verdict missing the selected assurance pack result was accepted")
	}
	assessment.AssuranceResults = []AssuranceResult{{
		Pack:        "security@1",
		Outcome:     "pass",
		EvidenceIDs: []string{"health-smoke"},
		Summary:     "The selected security pack passed with bound evidence.",
	}}
	record, err := BuildDeliveryVerdict(input, stamp, testExactAssessment(t, assessment))
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := ParseDeliveryVerdict(record.CanonicalJSON)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateDeliveryVerdictBindings(input, verdict); err != nil {
		t.Fatal(err)
	}

	unbound := cloneVerifierAssessment(assessment)
	unbound.AssuranceResults[0].EvidenceIDs = []string{"unknown-evidence"}
	if _, err := BuildDeliveryVerdict(input, stamp, testExactAssessment(t, unbound)); err == nil {
		t.Fatal("assurance result with unbound evidence was accepted")
	}
	extra := cloneVerifierAssessment(assessment)
	extra.AssuranceResults = append(extra.AssuranceResults, AssuranceResult{
		Pack: "privacy@1", Outcome: "pass", EvidenceIDs: []string{"health-smoke"}, Summary: "Unexpected pack.",
	})
	if _, err := BuildDeliveryVerdict(input, stamp, testExactAssessment(t, extra)); err == nil {
		t.Fatal("verdict with an unselected assurance pack was accepted")
	}
}

func TestVerifierRunMayReuseCheckNamespaceButNotBuilderIdentity(t *testing.T) {
	t.Parallel()

	input, published, assessment := testVerdictChain(t)
	checkRunID := input.Submission.View().Checks[0].RunID
	input = testBindingForSubmission(t, input.Plan, input.Submission, checkRunID)
	stamp := testVerdictStamp(published)
	stamp.VerdictID = "verdict-check-namespace"
	if _, err := BuildDeliveryVerdict(input, stamp, testExactAssessment(t, assessment)); err != nil {
		t.Fatalf("Baton-compatible check/verifier namespace reuse was rejected: %v", err)
	}
}

func TestVerdictParsersRejectHostileJSON(t *testing.T) {
	t.Parallel()

	_, _, assessment := testVerdictChain(t)
	assessmentBytes := testJSONBytes(t, assessment)
	verdictBytes := testSnapshotBytes(t, "examples/pass-verdict.json")
	parsers := []struct {
		name    string
		valid   []byte
		maximum int
		parse   func([]byte) error
	}{
		{"assessment", assessmentBytes, MaximumVerifierAssessmentBytes, func(value []byte) error { _, err := ParseVerifierAssessment(value); return err }},
		{"verdict", verdictBytes, MaximumDeliveryVerdictBytes, func(value []byte) error { _, err := ParseDeliveryVerdict(value); return err }},
	}
	for _, parser := range parsers {
		parser := parser
		t.Run(parser.name, func(t *testing.T) {
			t.Parallel()
			duplicate := append([]byte(`{"schema_version":"duplicate",`), parser.valid[1:]...)
			for name, value := range map[string][]byte{
				"duplicate key":  duplicate,
				"trailing value": append(append([]byte(nil), parser.valid...), []byte(` {}`)...),
				"lone surrogate": []byte(`{"value":"\ud800"}`),
				"unsafe integer": []byte(`{"value":9007199254740992}`),
				"non-finite":     []byte(`{"value":1e9999}`),
				"over ceiling":   bytes.Repeat([]byte{' '}, parser.maximum+1),
			} {
				if err := parser.parse(value); err == nil {
					t.Errorf("%s input was accepted", name)
				}
			}
		})
	}
}

func testVerdictChain(t *testing.T) (VerdictBindingInput, DeliveryVerdict, VerifierAssessment) {
	t.Helper()
	plan, err := ParseDeliveryPlan(testSnapshotBytes(t, "examples/standard-plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	submission, err := ParseSubmission(testSnapshotBytes(t, "examples/standard-submission.json"))
	if err != nil {
		t.Fatal(err)
	}
	dispatch := testSnapshotBytes(t, "examples/artifacts/dispatch/verifier-run-1.json")
	verdict, err := ParseDeliveryVerdict(testSnapshotBytes(t, "examples/pass-verdict.json"))
	if err != nil {
		t.Fatal(err)
	}
	return VerdictBindingInput{
		Plan: plan, Submission: submission, DispatchReceipt: verdict.Review.DispatchReceipt, Dispatch: dispatch,
	}, verdict, testAssessmentFromVerdict(verdict)
}

func testAssessmentFromVerdict(verdict DeliveryVerdict) VerifierAssessment {
	return cloneVerifierAssessment(VerifierAssessment{
		SchemaVersion:     VerifierAssessmentSchemaVersion,
		Outcome:           verdict.Outcome,
		Summary:           verdict.Summary,
		AcceptanceResults: verdict.AcceptanceResults,
		AssuranceResults:  verdict.AssuranceResults,
		Findings:          verdict.Findings,
	})
}

func testExactAssessment(t *testing.T, assessment VerifierAssessment) ExactVerifierAssessment {
	t.Helper()
	exact, err := ParseVerifierAssessment(testJSONBytes(t, assessment))
	if err != nil {
		t.Fatal(err)
	}
	return exact
}

func testVerdictStamp(verdict DeliveryVerdict) VerdictStamp {
	return VerdictStamp{
		VerdictID: verdict.VerdictID, Agent: verdict.Review.Agent,
		StartedAt: verdict.Review.StartedAt, CompletedAt: verdict.Review.CompletedAt,
	}
}

func testFinding(id, kind, severity string) Finding {
	return Finding{
		ID: id, Kind: kind, Principle: "B3", Severity: severity, Summary: "Finding summary.",
		AcceptanceIDs: []string{}, EvidenceIDs: []string{},
	}
}

func testAssessmentWithFindings(
	base VerifierAssessment,
	outcome string,
	findings ...Finding,
) VerifierAssessment {
	assessment := cloneVerifierAssessment(base)
	assessment.Outcome = outcome
	assessment.Findings = append([]Finding{}, findings...)
	return assessment
}

func testBindingForSubmission(
	t *testing.T,
	plan ExactPlan,
	submission ExactSubmission,
	dispatchID string,
) VerdictBindingInput {
	t.Helper()
	dispatch, err := BuildVerifierDispatch(VerifierDispatchInput{
		Submission: submission,
		DispatchID: dispatchID,
		Workspace:  "fresh-read-only-materialization",
		CreatedAt:  "2026-07-19T00:05:55Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	return VerdictBindingInput{
		Plan:       plan,
		Submission: submission,
		DispatchReceipt: Artifact{
			Ref: "test-dispatch.json", MediaType: "application/json", Digest: RawDigest(dispatch.CanonicalJSON),
		},
		Dispatch: dispatch.CanonicalJSON,
	}
}
