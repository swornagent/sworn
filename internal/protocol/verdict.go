package protocol

import (
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"
)

const (
	VerifierAssessmentSchemaVersion = "sworn-verifier-assessment-v1"
	DeliveryVerdictSchemaVersion    = "delivery-verdict-v1"
	MaximumVerifierAssessmentBytes  = 1 << 20
	MaximumDeliveryVerdictBytes     = 1 << 20
)

const (
	maximumVerifierAssessmentSummaryCodePoints = 4096
	maximumVerifierResultSummaryCodePoints     = 512
	maximumVerifierAssessmentCollectionItems   = 64
	maximumVerifierAssessmentReferenceItems    = 16
)

type AcceptanceResult struct {
	AcceptanceID string   `json:"acceptance_id"`
	Outcome      string   `json:"outcome"`
	EvidenceIDs  []string `json:"evidence_ids"`
	Summary      string   `json:"summary"`
}

type AssuranceResult struct {
	Pack        string   `json:"pack"`
	Outcome     string   `json:"outcome"`
	EvidenceIDs []string `json:"evidence_ids"`
	Summary     string   `json:"summary"`
}

type Finding struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	Principle     string   `json:"principle"`
	Severity      string   `json:"severity"`
	Summary       string   `json:"summary"`
	AcceptanceIDs []string `json:"acceptance_ids"`
	EvidenceIDs   []string `json:"evidence_ids"`
}

// VerifierAssessment is the complete model-owned output. It deliberately has
// no verdict, submission, dispatch, review, agent, or timestamp authority.
type VerifierAssessment struct {
	SchemaVersion     string             `json:"schema_version"`
	Outcome           string             `json:"outcome"`
	Summary           string             `json:"summary"`
	AcceptanceResults []AcceptanceResult `json:"acceptance_results"`
	AssuranceResults  []AssuranceResult  `json:"assurance_results"`
	Findings          []Finding          `json:"findings"`
}

// ExactVerifierAssessment is an immutable capability obtained only by parsing
// one strict model output. The engine may inspect a defensive view, but verdict
// construction cannot accept a programmatically manufactured assessment.
type ExactVerifierAssessment struct {
	record EncodedRecord
	view   VerifierAssessment
}

type VerdictReview struct {
	RunID           string   `json:"run_id"`
	Agent           string   `json:"agent"`
	FreshContext    bool     `json:"fresh_context"`
	DispatchReceipt Artifact `json:"dispatch_receipt"`
	StartedAt       string   `json:"started_at"`
	CompletedAt     string   `json:"completed_at"`
}

type DeliveryVerdict struct {
	SchemaVersion     string             `json:"schema_version"`
	VerdictID         string             `json:"verdict_id"`
	SubmissionID      string             `json:"submission_id"`
	SubmissionDigest  string             `json:"submission_digest"`
	DeliveryID        string             `json:"delivery_id"`
	WorkID            string             `json:"work_id"`
	Review            VerdictReview      `json:"review"`
	Outcome           string             `json:"outcome"`
	Summary           string             `json:"summary"`
	AcceptanceResults []AcceptanceResult `json:"acceptance_results"`
	AssuranceResults  []AssuranceResult  `json:"assurance_results"`
	Findings          []Finding          `json:"findings"`
}

// VerdictBindingInput supplies already parsed immutable plan/submission truth
// plus one engine-bound dispatch artifact pointer and its exact raw bytes. This
// pure value proves the pointer's digest, but Store must still prove that its
// locator resolves to those bytes and that all inputs are current and durable.
type VerdictBindingInput struct {
	Plan            ExactPlan
	Submission      ExactSubmission
	DispatchReceipt Artifact
	Dispatch        []byte
}

// VerdictStamp contains only durable engine-owned values. BuildDeliveryVerdict
// never reads a clock or accepts envelope fields from the model assessment.
type VerdictStamp struct {
	VerdictID   string
	Agent       string
	StartedAt   string
	CompletedAt string
}

// ParseVerifierAssessment accepts exactly one strict assessment object. It
// never scans prose or strips markdown wrappers.
func ParseVerifierAssessment(contents []byte) (ExactVerifierAssessment, error) {
	var assessment VerifierAssessment
	if err := decodeExactJSONShape(
		contents, MaximumVerifierAssessmentBytes, "verifier assessment", &assessment,
	); err != nil {
		return ExactVerifierAssessment{}, err
	}
	if err := validateVerifierAssessment(assessment); err != nil {
		return ExactVerifierAssessment{}, err
	}
	canonical, err := EncodeCanonical(assessment)
	if err != nil {
		return ExactVerifierAssessment{}, fmt.Errorf("canonicalize verifier assessment: %w", err)
	}
	return ExactVerifierAssessment{
		record: EncodedRecord{
			Kind: VerifierAssessmentSchemaVersion, CanonicalJSON: canonical, Digest: CanonicalDigest(canonical),
		},
		view: cloneVerifierAssessment(assessment),
	}, nil
}

func (assessment ExactVerifierAssessment) Record() EncodedRecord {
	record := assessment.record
	record.CanonicalJSON = slices.Clone(record.CanonicalJSON)
	return record
}

func (assessment ExactVerifierAssessment) View() VerifierAssessment {
	return cloneVerifierAssessment(assessment.view)
}

func (assessment ExactVerifierAssessment) valid() bool {
	return assessment.record.Kind == VerifierAssessmentSchemaVersion &&
		ValidDigest(assessment.record.Digest) && len(assessment.record.CanonicalJSON) != 0
}

// ParseDeliveryVerdict proves strict Baton shape and intra-record semantics. It
// does not prove artifact resolution, current authority, durable identity, or
// admission against the current Store.
func ParseDeliveryVerdict(contents []byte) (DeliveryVerdict, error) {
	var verdict DeliveryVerdict
	if err := decodeExactJSONShape(
		contents, MaximumDeliveryVerdictBytes, "delivery verdict", &verdict,
	); err != nil {
		return DeliveryVerdict{}, err
	}
	if err := validateDeliveryVerdictShape(verdict); err != nil {
		return DeliveryVerdict{}, err
	}
	return cloneDeliveryVerdict(verdict), nil
}

// BuildDeliveryVerdict stamps the immutable Baton envelope around a model
// assessment, validates the complete pure plan/submission/dispatch binding, and
// returns canonical record bytes. Store must still resolve current durable
// truth, enforce write-once identities, and apply the PASS authority gate.
func BuildDeliveryVerdict(
	input VerdictBindingInput,
	stamp VerdictStamp,
	assessment ExactVerifierAssessment,
) (EncodedRecord, error) {
	resolved, err := resolveVerdictBinding(input)
	if err != nil {
		return EncodedRecord{}, err
	}
	if !assessment.valid() {
		return EncodedRecord{}, errors.New("delivery verdict requires an exact verifier assessment capability")
	}
	assessmentView := assessment.View()
	verdict := DeliveryVerdict{
		SchemaVersion:    DeliveryVerdictSchemaVersion,
		VerdictID:        stamp.VerdictID,
		SubmissionID:     resolved.submission.SubmissionID,
		SubmissionDigest: input.Submission.Record().Digest,
		DeliveryID:       resolved.submission.DeliveryID,
		WorkID:           resolved.submission.WorkID,
		Review: VerdictReview{
			RunID:           resolved.dispatch.DispatchID,
			Agent:           stamp.Agent,
			FreshContext:    true,
			DispatchReceipt: input.DispatchReceipt,
			StartedAt:       stamp.StartedAt,
			CompletedAt:     stamp.CompletedAt,
		},
		Outcome:           assessmentView.Outcome,
		Summary:           assessmentView.Summary,
		AcceptanceResults: assessmentView.AcceptanceResults,
		AssuranceResults:  assessmentView.AssuranceResults,
		Findings:          assessmentView.Findings,
	}
	if err := validateDeliveryVerdictShape(verdict); err != nil {
		return EncodedRecord{}, err
	}
	if err := validateResolvedVerdictBindings(resolved, input, verdict); err != nil {
		return EncodedRecord{}, err
	}
	canonical, err := EncodeCanonical(verdict)
	if err != nil {
		return EncodedRecord{}, fmt.Errorf("canonicalize delivery verdict: %w", err)
	}
	if len(canonical) > MaximumDeliveryVerdictBytes {
		return EncodedRecord{}, errors.New("delivery verdict exceeds byte ceiling")
	}
	if _, err := ParseDeliveryVerdict(canonical); err != nil {
		return EncodedRecord{}, err
	}
	return EncodedRecord{
		Kind: DeliveryVerdictSchemaVersion, CanonicalJSON: canonical, Digest: CanonicalDigest(canonical),
	}, nil
}

// ValidateDeliveryVerdictBindings checks the pure cross-record closure used by
// BuildDeliveryVerdict. It is intentionally not a durable admission API.
func ValidateDeliveryVerdictBindings(input VerdictBindingInput, verdict DeliveryVerdict) error {
	if err := validateDeliveryVerdictShape(verdict); err != nil {
		return err
	}
	canonical, err := EncodeCanonical(verdict)
	if err != nil {
		return fmt.Errorf("canonicalize delivery verdict: %w", err)
	}
	if len(canonical) > MaximumDeliveryVerdictBytes {
		return errors.New("delivery verdict exceeds byte ceiling")
	}
	resolved, err := resolveVerdictBinding(input)
	if err != nil {
		return err
	}
	return validateResolvedVerdictBindings(resolved, input, verdict)
}

func validateVerifierAssessment(assessment VerifierAssessment) error {
	if assessment.SchemaVersion != VerifierAssessmentSchemaVersion {
		return fmt.Errorf("unknown verifier assessment schema %q", assessment.SchemaVersion)
	}
	if !ValidNonEmpty(assessment.Summary) ||
		utf8.RuneCountInString(assessment.Summary) > maximumVerifierAssessmentSummaryCodePoints ||
		len(assessment.AcceptanceResults) == 0 ||
		len(assessment.AcceptanceResults) > maximumVerifierAssessmentCollectionItems ||
		assessment.AssuranceResults == nil ||
		len(assessment.AssuranceResults) > maximumVerifierAssessmentCollectionItems ||
		assessment.Findings == nil ||
		len(assessment.Findings) > maximumVerifierAssessmentCollectionItems {
		return errors.New("verifier assessment requires a summary and complete result arrays")
	}
	acceptanceIDs := make(map[string]struct{}, len(assessment.AcceptanceResults))
	for index, result := range assessment.AcceptanceResults {
		if err := validateAcceptanceResult(result); err != nil {
			return fmt.Errorf("acceptance result %d: %w", index, err)
		}
		if _, exists := acceptanceIDs[result.AcceptanceID]; exists {
			return fmt.Errorf("duplicate acceptance result %q", result.AcceptanceID)
		}
		acceptanceIDs[result.AcceptanceID] = struct{}{}
	}
	packIDs := make(map[string]struct{}, len(assessment.AssuranceResults))
	for index, result := range assessment.AssuranceResults {
		if err := validateAssuranceResult(result); err != nil {
			return fmt.Errorf("assurance result %d: %w", index, err)
		}
		if _, exists := packIDs[result.Pack]; exists {
			return fmt.Errorf("duplicate assurance result %q", result.Pack)
		}
		packIDs[result.Pack] = struct{}{}
	}
	findingIDs := make(map[string]struct{}, len(assessment.Findings))
	for index, finding := range assessment.Findings {
		if err := validateFinding(finding); err != nil {
			return fmt.Errorf("finding %d: %w", index, err)
		}
		if _, exists := findingIDs[finding.ID]; exists {
			return fmt.Errorf("duplicate finding id %q", finding.ID)
		}
		findingIDs[finding.ID] = struct{}{}
	}
	return validateAssessmentOutcome(assessment)
}

func validateAcceptanceResult(result AcceptanceResult) error {
	if !ValidID(result.AcceptanceID) || !validResultOutcome(result.Outcome) ||
		result.EvidenceIDs == nil || len(result.EvidenceIDs) > maximumVerifierAssessmentReferenceItems ||
		duplicateStrings(result.EvidenceIDs) || !ValidNonEmpty(result.Summary) ||
		utf8.RuneCountInString(result.Summary) > maximumVerifierResultSummaryCodePoints {
		return errors.New("acceptance result has invalid identity, outcome, evidence, or summary")
	}
	for _, evidenceID := range result.EvidenceIDs {
		if !ValidID(evidenceID) {
			return errors.New("acceptance result has an invalid evidence id")
		}
	}
	return nil
}

func validateAssuranceResult(result AssuranceResult) error {
	if !packIDPattern.MatchString(result.Pack) || !validResultOutcome(result.Outcome) ||
		result.EvidenceIDs == nil || len(result.EvidenceIDs) > maximumVerifierAssessmentReferenceItems ||
		duplicateStrings(result.EvidenceIDs) || !ValidNonEmpty(result.Summary) ||
		utf8.RuneCountInString(result.Summary) > maximumVerifierResultSummaryCodePoints {
		return errors.New("assurance result has invalid pack, outcome, evidence, or summary")
	}
	for _, evidenceID := range result.EvidenceIDs {
		if !ValidID(evidenceID) {
			return errors.New("assurance result has an invalid evidence id")
		}
	}
	return nil
}

func validateFinding(finding Finding) error {
	if !ValidID(finding.ID) || !validFindingKind(finding.Kind) || !validPrinciple(finding.Principle) ||
		(finding.Severity != "blocking" && finding.Severity != "non_blocking") ||
		!ValidNonEmpty(finding.Summary) || finding.AcceptanceIDs == nil || finding.EvidenceIDs == nil ||
		utf8.RuneCountInString(finding.Summary) > maximumVerifierResultSummaryCodePoints ||
		len(finding.AcceptanceIDs) > maximumVerifierAssessmentReferenceItems ||
		len(finding.EvidenceIDs) > maximumVerifierAssessmentReferenceItems ||
		duplicateStrings(finding.AcceptanceIDs) || duplicateStrings(finding.EvidenceIDs) {
		return errors.New("finding has invalid identity, taxonomy, summary, or references")
	}
	for _, id := range append(append([]string(nil), finding.AcceptanceIDs...), finding.EvidenceIDs...) {
		if !ValidID(id) {
			return errors.New("finding has an invalid reference id")
		}
	}
	return nil
}

func validateAssessmentOutcome(assessment VerifierAssessment) error {
	hasBlocking := func(kinds ...string) bool {
		for _, finding := range assessment.Findings {
			if finding.Severity == "blocking" && slices.Contains(kinds, finding.Kind) {
				return true
			}
		}
		return false
	}
	switch assessment.Outcome {
	case "PASS":
		for _, result := range assessment.AcceptanceResults {
			if result.Outcome != "pass" || len(result.EvidenceIDs) == 0 {
				return errors.New("PASS requires every acceptance result to pass with evidence")
			}
		}
		for _, result := range assessment.AssuranceResults {
			if result.Outcome != "pass" || len(result.EvidenceIDs) == 0 {
				return errors.New("PASS requires every assurance result to pass with evidence")
			}
		}
		if hasBlocking("authority", "contract", "implementation", "evidence", "environment", "composition") {
			return errors.New("PASS cannot contain a blocking finding")
		}
	case "FAIL":
		if !hasBlocking("implementation", "evidence", "composition") ||
			hasBlocking("authority", "contract", "environment") {
			return errors.New("FAIL requires a delivery blocker and forbids upstream or environment blockers")
		}
	case "SPEC_BLOCK":
		if !hasBlocking("authority", "contract") {
			return errors.New("SPEC_BLOCK requires a blocking authority or contract finding")
		}
	case "INCONCLUSIVE":
		if !hasBlocking("evidence", "environment") || hasBlocking("authority", "contract") {
			return errors.New("INCONCLUSIVE requires a verifier blocker and forbids upstream blockers")
		}
	default:
		return fmt.Errorf("invalid verifier outcome %q", assessment.Outcome)
	}
	return nil
}

func validateDeliveryVerdictShape(verdict DeliveryVerdict) error {
	if verdict.SchemaVersion != DeliveryVerdictSchemaVersion {
		return fmt.Errorf("unknown delivery verdict schema %q", verdict.SchemaVersion)
	}
	for label, value := range map[string]string{
		"verdict": verdict.VerdictID, "submission": verdict.SubmissionID,
		"delivery": verdict.DeliveryID, "work": verdict.WorkID, "review run": verdict.Review.RunID,
	} {
		if !ValidID(value) {
			return fmt.Errorf("delivery verdict has an invalid %s id", label)
		}
	}
	if !ValidDigest(verdict.SubmissionDigest) || !ValidNonEmpty(verdict.Review.Agent) ||
		!verdict.Review.FreshContext {
		return errors.New("delivery verdict has an invalid submission or review identity")
	}
	if err := validateArtifact(verdict.Review.DispatchReceipt, "verifier dispatch receipt"); err != nil {
		return err
	}
	if verdict.Review.DispatchReceipt.MediaType != "application/json" {
		return errors.New("verifier dispatch receipt must use application/json")
	}
	startedAt, err := parseRecordTime(verdict.Review.StartedAt, "review start")
	if err != nil {
		return err
	}
	completedAt, err := parseRecordTime(verdict.Review.CompletedAt, "review completion")
	if err != nil || completedAt.Before(startedAt) {
		return errors.New("delivery verdict has invalid review timestamps")
	}
	return validateVerifierAssessment(VerifierAssessment{
		SchemaVersion:     VerifierAssessmentSchemaVersion,
		Outcome:           verdict.Outcome,
		Summary:           verdict.Summary,
		AcceptanceResults: verdict.AcceptanceResults,
		AssuranceResults:  verdict.AssuranceResults,
		Findings:          verdict.Findings,
	})
}

type resolvedVerdictBinding struct {
	submission Submission
	dispatch   VerifierDispatch
}

func resolveVerdictBinding(input VerdictBindingInput) (resolvedVerdictBinding, error) {
	if input.Plan.Record().Digest == "" || !input.Submission.valid() {
		return resolvedVerdictBinding{}, errors.New("verdict binding requires exact plan and submission capabilities")
	}
	if len(input.Dispatch) == 0 || len(input.Dispatch) > MaximumControlReceiptBytes {
		return resolvedVerdictBinding{}, errors.New("verifier dispatch is empty or exceeds its byte ceiling")
	}
	if err := validateArtifact(input.DispatchReceipt, "verifier dispatch receipt"); err != nil {
		return resolvedVerdictBinding{}, err
	}
	if input.DispatchReceipt.MediaType != "application/json" ||
		RawDigest(input.Dispatch) != input.DispatchReceipt.Digest {
		return resolvedVerdictBinding{}, errors.New("verifier dispatch receipt does not bind the exact raw artifact")
	}
	dispatch, err := ParseVerifierDispatch(input.Dispatch)
	if err != nil {
		return resolvedVerdictBinding{}, fmt.Errorf("parse bound verifier dispatch: %w", err)
	}
	return resolvedVerdictBinding{submission: input.Submission.View(), dispatch: dispatch}, nil
}

func validateResolvedVerdictBindings(
	resolved resolvedVerdictBinding,
	input VerdictBindingInput,
	verdict DeliveryVerdict,
) error {
	submission := resolved.submission
	submissionRecord := input.Submission.Record()
	planRecord := input.Plan.Record()
	contract, exists := input.Plan.Work(submission.WorkID)
	if !exists {
		return errors.New("submission work is absent from the exact plan")
	}
	work := contract.View()
	target := input.Plan.Target()
	if submission.PlanDigest != planRecord.Digest || submission.ContractDigest != contract.Digest() ||
		submission.DeliveryID != input.Plan.DeliveryID() || work.ID != submission.WorkID ||
		submission.Base.Repository != target.Repository || submission.Base.Ref != target.Ref ||
		submission.Candidate.Repository != target.Repository {
		return errors.New("submission does not bind the exact plan, contract, work, and target")
	}
	policy := input.Plan.Policy()
	if submission.Assurance.Profile != work.Assurance.Profile ||
		!sameStringSet(submission.Assurance.Packs, work.Assurance.Packs) ||
		submission.Assurance.PolicyRef != policy.Ref || submission.Assurance.PolicyDigest != policy.Digest {
		return errors.New("submission does not bind the exact assurance selection")
	}
	acceptance := make(map[string]PlanAcceptance, len(work.Acceptance))
	for _, criterion := range work.Acceptance {
		acceptance[criterion.ID] = criterion
	}
	evidence := make(map[string]Evidence, len(submission.Evidence))
	for _, item := range submission.Evidence {
		evidence[item.ID] = item
		for _, acceptanceID := range item.AcceptanceIDs {
			criterion, exists := acceptance[acceptanceID]
			if !exists {
				return fmt.Errorf("submission evidence %q names unknown acceptance %q", item.ID, acceptanceID)
			}
			if evidenceBoundaryRank(item.Boundary) < evidenceBoundaryRank(criterion.EvidenceLevel) {
				return fmt.Errorf("submission evidence %q is too weak for acceptance %q", item.ID, acceptanceID)
			}
		}
	}
	dispatch := resolved.dispatch
	if verdict.Review.DispatchReceipt != input.DispatchReceipt ||
		verdict.Review.DispatchReceipt.MediaType != "application/json" ||
		RawDigest(input.Dispatch) != verdict.Review.DispatchReceipt.Digest {
		return errors.New("verdict dispatch pointer does not bind the exact raw artifact")
	}
	if dispatch.DispatchID != verdict.Review.RunID || dispatch.SubmissionDigest != submissionRecord.Digest ||
		dispatch.Candidate != submission.Candidate {
		return errors.New("verifier dispatch does not bind the verdict submission and candidate")
	}
	if verdict.SubmissionID != submission.SubmissionID || verdict.SubmissionDigest != submissionRecord.Digest ||
		verdict.DeliveryID != submission.DeliveryID || verdict.WorkID != submission.WorkID {
		return errors.New("delivery verdict does not bind the exact submission")
	}
	if verdict.Review.RunID == submission.Builder.RunID {
		return errors.New("verifier run reuses the builder run identity")
	}
	submissionCreated, _ := parseRecordTime(submission.CreatedAt, "submission creation")
	dispatchCreated, _ := parseRecordTime(dispatch.CreatedAt, "dispatch creation")
	reviewStarted, _ := parseRecordTime(verdict.Review.StartedAt, "review start")
	if dispatchCreated.Before(submissionCreated) || reviewStarted.Before(dispatchCreated) {
		return errors.New("dispatch and review timestamps are outside the submission window")
	}
	resultAcceptance := make([]string, len(verdict.AcceptanceResults))
	for index, result := range verdict.AcceptanceResults {
		resultAcceptance[index] = result.AcceptanceID
		for _, evidenceID := range result.EvidenceIDs {
			item, exists := evidence[evidenceID]
			if !exists || !slices.Contains(item.AcceptanceIDs, result.AcceptanceID) {
				return fmt.Errorf("acceptance result %q has unbound evidence %q", result.AcceptanceID, evidenceID)
			}
		}
	}
	contractAcceptance := make([]string, 0, len(acceptance))
	for acceptanceID := range acceptance {
		contractAcceptance = append(contractAcceptance, acceptanceID)
	}
	if !sameStringSet(resultAcceptance, contractAcceptance) {
		return errors.New("verdict acceptance results do not exactly cover the contract")
	}
	resultPacks := make([]string, len(verdict.AssuranceResults))
	for index, result := range verdict.AssuranceResults {
		resultPacks[index] = result.Pack
		for _, evidenceID := range result.EvidenceIDs {
			item, exists := evidence[evidenceID]
			if !exists || !slices.Contains(item.PackIDs, result.Pack) {
				return fmt.Errorf("assurance result %q has unbound evidence %q", result.Pack, evidenceID)
			}
		}
	}
	if !sameStringSet(resultPacks, work.Assurance.Packs) {
		return errors.New("verdict assurance results do not exactly cover selected packs")
	}
	for _, finding := range verdict.Findings {
		for _, acceptanceID := range finding.AcceptanceIDs {
			if _, exists := acceptance[acceptanceID]; !exists {
				return fmt.Errorf("finding %q names unknown acceptance %q", finding.ID, acceptanceID)
			}
		}
		for _, evidenceID := range finding.EvidenceIDs {
			if _, exists := evidence[evidenceID]; !exists {
				return fmt.Errorf("finding %q names unknown evidence %q", finding.ID, evidenceID)
			}
		}
	}
	if verdict.Outcome == "PASS" {
		for _, check := range submission.Checks {
			if check.Outcome != "pass" {
				return fmt.Errorf("PASS cannot bind non-passing check %q", check.ID)
			}
		}
	}
	return nil
}

func cloneVerifierAssessment(assessment VerifierAssessment) VerifierAssessment {
	assessment.AcceptanceResults = slices.Clone(assessment.AcceptanceResults)
	for index := range assessment.AcceptanceResults {
		assessment.AcceptanceResults[index].EvidenceIDs = slices.Clone(
			assessment.AcceptanceResults[index].EvidenceIDs,
		)
	}
	assessment.AssuranceResults = slices.Clone(assessment.AssuranceResults)
	for index := range assessment.AssuranceResults {
		assessment.AssuranceResults[index].EvidenceIDs = slices.Clone(
			assessment.AssuranceResults[index].EvidenceIDs,
		)
	}
	assessment.Findings = slices.Clone(assessment.Findings)
	for index := range assessment.Findings {
		assessment.Findings[index].AcceptanceIDs = slices.Clone(assessment.Findings[index].AcceptanceIDs)
		assessment.Findings[index].EvidenceIDs = slices.Clone(assessment.Findings[index].EvidenceIDs)
	}
	return assessment
}

func cloneDeliveryVerdict(verdict DeliveryVerdict) DeliveryVerdict {
	assessment := cloneVerifierAssessment(VerifierAssessment{
		SchemaVersion: VerifierAssessmentSchemaVersion, Outcome: verdict.Outcome, Summary: verdict.Summary,
		AcceptanceResults: verdict.AcceptanceResults, AssuranceResults: verdict.AssuranceResults,
		Findings: verdict.Findings,
	})
	verdict.AcceptanceResults = assessment.AcceptanceResults
	verdict.AssuranceResults = assessment.AssuranceResults
	verdict.Findings = assessment.Findings
	return verdict
}

func validResultOutcome(value string) bool {
	return value == "pass" || value == "fail" || value == "inconclusive"
}

func validFindingKind(value string) bool {
	return value == "authority" || value == "contract" || value == "implementation" ||
		value == "evidence" || value == "environment" || value == "composition"
}

func validPrinciple(value string) bool {
	return value == "B1" || value == "B2" || value == "B3" || value == "B4" || value == "B5"
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	values := make(map[string]struct{}, len(left))
	for _, value := range left {
		values[value] = struct{}{}
	}
	if len(values) != len(left) {
		return false
	}
	for _, value := range right {
		if _, exists := values[value]; !exists {
			return false
		}
	}
	rightValues := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightValues[value] = struct{}{}
	}
	return len(rightValues) == len(right)
}

func evidenceBoundaryRank(value string) int {
	switch value {
	case "component":
		return 1
	case "assembled":
		return 2
	case "live":
		return 3
	default:
		return 0
	}
}
