package driver

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
)

// MinSubmissionSummaryFloorBytes and MinSubmissionDetailFloorBytes are the
// content floor A3 lands as product: below either bound, a submission from
// one of the floored responsibilities is refused as SUBMISSION_BELOW_FLOOR
// rather than sealed. It is explicitly the second line, not the first - A2's
// self-declaration reading is what actually stops a probe, since the
// native-lane-honesty evidence is that the next probe simply padded past a
// floor alone.
const (
	MinSubmissionSummaryFloorBytes = 120
	MinSubmissionDetailFloorBytes  = 200
)

// maxSubmissionScopeLintDetailBytes bounds the engine-derived package-path
// list submit.plan_scope_lint may carry as Detail.Paths. It mirrors
// sandboxStartCause's bound-at-the-raise-site discipline, but truncates
// (rather than drops) since this Detail is constructed once here, never
// re-parsed from an untrusted source the way a dispatcher funnel would.
const maxSubmissionScopeLintDetailBytes = 512

// submissionScopeLintTruncationMarker replaces the tail of a Paths list once
// the encoded list would exceed maxSubmissionScopeLintDetailBytes.
const submissionScopeLintTruncationMarker = "...(truncated)"

// submissionRefusalDetail is the canonical-JSON envelope every submit-path
// refusal (A1) carries as Detail: which check refused, which engine-known
// field or bound it names, and (for submit.plan_scope_lint only) which
// engine-derived package paths were missing. Field is always an engine-known
// identifier - a wire schema field name or a containing-object name - never
// worker-submitted text; Bound is always a short engine constant identifier,
// never a value. No submitted byte ever reaches this envelope.
type submissionRefusalDetail struct {
	Check string   `json:"check"`
	Field string   `json:"field,omitempty"`
	Bound string   `json:"bound,omitempty"`
	Paths []string `json:"paths,omitempty"`
}

// submissionRefusalDetailBytes encodes one submission-refusal envelope as
// compact canonical JSON. A marshal failure (unreachable for this closed
// shape) yields "" rather than a partial envelope.
func submissionRefusalDetailBytes(check, field, bound string, paths []string) string {
	envelope := submissionRefusalDetail{
		Check: check,
		Field: field,
		Bound: bound,
		Paths: truncateSubmissionScopeLintPaths(paths),
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return ""
	}
	return string(body)
}

// truncateSubmissionScopeLintPaths bounds an engine-derived package-path
// list to maxSubmissionScopeLintDetailBytes once JSON-encoded, dropping
// trailing paths and appending a truncation marker rather than dropping the
// whole list.
func truncateSubmissionScopeLintPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	if encoded, err := json.Marshal(paths); err == nil &&
		len(encoded) <= maxSubmissionScopeLintDetailBytes {
		return paths
	}
	for count := len(paths) - 1; count >= 0; count-- {
		candidate := append(append([]string{}, paths[:count]...), submissionScopeLintTruncationMarker)
		if encoded, err := json.Marshal(candidate); err == nil &&
			len(encoded) <= maxSubmissionScopeLintDetailBytes {
			return candidate
		}
	}
	return []string{submissionScopeLintTruncationMarker}
}

// withSubmissionRefusalDetail rewraps err's own code with a submission-refusal
// Detail envelope naming check (and, when non-empty, field/bound), leaving
// err unchanged if it does not carry a *ContractError.
func withSubmissionRefusalDetail(err error, check, field, bound string) error {
	var contractErr *ContractError
	if !errors.As(err, &contractErr) {
		return err
	}
	return &ContractError{
		Code:   contractErr.Code,
		Detail: submissionRefusalDetailBytes(check, field, bound, nil),
	}
}

// submitArgumentsDecodeDetail wraps toolSession.submit's outermost
// decodeStrict bound: every code but RESOURCE_LIMIT gets Check only, since
// none of MISSING_JSON/INVALID_UTF8/INVALID_UNICODE/INVALID_JSON/
// TRAILING_JSON/DUPLICATE_NAME names a field or bound in any engine-known
// sense at this raw-bytes stage; RESOURCE_LIMIT additionally names the
// max_tool_argument_bytes bound it tripped.
func submitArgumentsDecodeDetail(err error) error {
	var contractErr *ContractError
	if !errors.As(err, &contractErr) {
		return err
	}
	bound := ""
	if contractErr.Code == "RESOURCE_LIMIT" {
		bound = "max_tool_argument_bytes"
	}
	return withSubmissionRefusalDetail(err, "submit.arguments_decode", "", bound)
}

// submitRootObjectDetail wraps the {"submission": ...} wrapper's closedObject
// call: whichever of INVALID_FIELD/MISSING_FIELD/UNKNOWN_FIELD fired, the
// only field in play is "submission" itself, so no dynamic inspection is
// needed to name it.
func submitRootObjectDetail(err error) error {
	return withSubmissionRefusalDetail(err, "submit.root_object", "submission", "")
}

// submitExactBytesPathError builds one submit.exact_bytes_path refusal
// naming which submission member (plan, checks, or one contracts entry) the
// path-materialization step refused, never the guest path text itself.
func submitExactBytesPathError(code, field string) error {
	return &ContractError{
		Code:   code,
		Detail: submissionRefusalDetailBytes("submit.exact_bytes_path", field, "", nil),
	}
}

// submitEncodeDetail wraps EncodeSubmission's own returned error at the
// tools.go call site (fixes finding 3): decodeToolSubmission has already
// validated this exact, unmutated submission moments earlier, so the only
// errors EncodeSubmission's own two RESOURCE_LIMIT sites (json.Marshal
// failure, len(body)+1 > MaxSubmissionBytes) can newly raise here name the
// max_submission_bytes bound. Per Captain correction C2, the bound is
// attached only when the wrapped code is actually RESOURCE_LIMIT, so no
// other error reaching this call site is misnamed with a bound it did not
// trip.
func submitEncodeDetail(err error) error {
	var contractErr *ContractError
	if !errors.As(err, &contractErr) {
		return err
	}
	bound := ""
	if contractErr.Code == "RESOURCE_LIMIT" {
		bound = "max_submission_bytes"
	}
	return withSubmissionRefusalDetail(err, "submit.encode", "", bound)
}

// submitPlanScopeLintError builds the submit.plan_scope_lint refusal (fixes
// finding 4): Detail carries only Check plus the engine-derived package
// import paths LintSlice's own graph walk found under-scoped
// (baton.RecordError.Paths, bounded by maxSubmissionScopeLintDetailBytes) -
// never the worker's own submitted slice.ID or the joined Msg text, both of
// which the prior design carried and the Captain's finding removed.
func submitPlanScopeLintError(code string, lintErr error) error {
	var recordErr *baton.RecordError
	var paths []string
	if errors.As(lintErr, &recordErr) {
		paths = recordErr.Paths
	}
	return &ContractError{
		Code:   code,
		Detail: submissionRefusalDetailBytes("submit.plan_scope_lint", "", "", paths),
	}
}

// submitYieldFirstRequiredError builds the static submit.yield_first_required
// refusal: the branch names no field or bound, only the check that fired.
func submitYieldFirstRequiredError() error {
	return &ContractError{
		Code:   "YIELD_FIRST_REQUIRED",
		Detail: submissionRefusalDetailBytes("submit.yield_first_required", "", "", nil),
	}
}

// submitCheckEvidenceIncompleteError builds the
// submit.check_evidence_incomplete refusal (S5-A3, finding 2): Bound names
// the matching rule that refused it (check_command_covers, the engine
// constant CheckCommandCovers implements) and Paths carries exactly the one
// declared check with no recorded pass covering it, so a verifier can re-run
// it and resubmit inside the same turn instead of losing the dispatch to a
// post-turn CHECK_EVIDENCE_INCOMPLETE it can never see or act on.
func submitCheckEvidenceIncompleteError(check string) error {
	return &ContractError{
		Code: "CHECK_EVIDENCE_INCOMPLETE",
		Detail: submissionRefusalDetailBytes(
			"submit.check_evidence_incomplete", "checks", "check_command_covers", []string{check},
		),
	}
}

// submitCheckEvidenceEncodeError builds the submit.check_evidence_encode
// refusal for the unreachable-in-practice case where the driver's own
// bounded accumulator still fails to encode as a sworn.check-results/v1
// manifest, so that failure never becomes an opaque internal error the
// worker cannot act on.
func submitCheckEvidenceEncodeError(err error) error {
	return &ContractError{
		Code:   "CHECK_EVIDENCE_UNENCODABLE",
		Detail: submissionRefusalDetailBytes("submit.check_evidence_encode", "checks", "", nil),
	}
}

// decodeSubmissionObject wraps one decodeToolSubmission closedObject call
// (submit.decode): on MISSING_FIELD it re-derives Field by scanning value's
// own decoded map for the first absent key in required's own order - an
// engine-known field name from the wire schema, safe to name. On
// UNKNOWN_FIELD or INVALID_FIELD, Field names only containingObject, never
// the worker-chosen key that triggered UNKNOWN_FIELD.
func decodeSubmissionObject(
	value any,
	required, optional []string,
	containingObject string,
) (map[string]any, error) {
	object, err := closedObject(value, required, optional)
	if err == nil {
		return object, nil
	}
	var contractErr *ContractError
	if !errors.As(err, &contractErr) {
		return nil, err
	}
	field := containingObject
	if contractErr.Code == "MISSING_FIELD" {
		if asMap, ok := value.(map[string]any); ok {
			for _, key := range required {
				if _, present := asMap[key]; !present {
					field = key
					break
				}
			}
		}
	}
	return nil, &ContractError{
		Code:   contractErr.Code,
		Detail: submissionRefusalDetailBytes("submit.decode", field, "", nil),
	}
}

// submitDecodeError builds a submit.decode refusal for the two sites inside
// decodeToolSubmission that do not route through decodeSubmissionObject: the
// contracts member's own type assertion, and the (practically unreachable)
// canonicalJSON/json.Unmarshal round trip over an already-validated root.
func submitDecodeError(code, field string) error {
	return &ContractError{
		Code:   code,
		Detail: submissionRefusalDetailBytes("submit.decode", field, "", nil),
	}
}

// submissionValidateError builds a fresh submit.validate refusal: code paired
// with the field ValidateSubmission's own call site already knows it is
// checking, without needing to inspect an existing error.
func submissionValidateError(code, field string) error {
	return &ContractError{
		Code:   code,
		Detail: submissionRefusalDetailBytes("submit.validate", field, "", nil),
	}
}

// submissionValidateWrap renames a shared helper's own returned error
// (validateIdentity, validateExactBytes, validatePlanBytes,
// validateSubmissionDetail) with the submit.validate Check and the field its
// ValidateSubmission call site is checking, preserving whatever code the
// helper itself raised.
func submissionValidateWrap(err error, field string) error {
	return withSubmissionRefusalDetail(err, "submit.validate", field, "")
}

// submissionShapeMismatch builds one SUBMISSION_SHAPE_MISMATCH refusal
// naming the first violated member (plan, checks, decision, or contracts)
// per Captain correction C3's ordered per-member reading.
func submissionShapeMismatch(field string) error {
	return submissionValidateError("SUBMISSION_SHAPE_MISMATCH", field)
}

// submissionPermissionMismatch builds one SUBMISSION_BINDING_MISMATCH
// refusal naming whichever of invocation_id or responsibility first
// disagreed with the permission descriptor.
func submissionPermissionMismatch(field string) error {
	return &ContractError{
		Code:   "SUBMISSION_BINDING_MISMATCH",
		Detail: submissionRefusalDetailBytes("submit.permission_validate", field, "", nil),
	}
}

// submissionProbeError builds the SUBMISSION_DECLARED_PROBE refusal A2
// raises: Field names which submission member self-declared (summary or
// detail), Bound names which trigger class matched - never the field's own
// text.
func submissionProbeError(field, bound string) error {
	return &ContractError{
		Code:   "SUBMISSION_DECLARED_PROBE",
		Detail: submissionRefusalDetailBytes("submit.probe_declaration", field, bound, nil),
	}
}

// submissionFloorError builds the SUBMISSION_BELOW_FLOOR refusal A3 raises.
func submissionFloorError(field, bound string) error {
	return &ContractError{
		Code:   "SUBMISSION_BELOW_FLOOR",
		Detail: submissionRefusalDetailBytes("submit.content_floor", field, bound, nil),
	}
}

// asciiLower lowercases only ASCII letters, leaving every other byte (and
// any multi-byte UTF-8 sequence) untouched - a deliberately narrower
// normalization than strings.ToLower's Unicode case folding, since the
// closed probe-declaration vocabulary below is itself pure ASCII and a
// Unicode-aware fold could fold unrelated text into an accidental match.
func asciiLower(value string) string {
	body := []byte(value)
	for index, char := range body {
		if char >= 'A' && char <= 'Z' {
			body[index] = char + ('a' - 'A')
		}
	}
	return string(body)
}

// normalizeSubmissionField renders one submission field into the form
// submissionDeclaresProbe reads: trimmed and ASCII-lowercased, so surrounding
// whitespace and letter case can never defeat or force a match.
func normalizeSubmissionField(field string) string {
	return strings.TrimSpace(asciiLower(field))
}

// submissionProbeKnownExact is the one observed real-world probe payload
// that is a single bare word: matched only by whole-field equality, never as
// a prefix, since honest work routinely opens with "Test coverage ..." and
// anchoring "test" as a prefix would re-open the over-match finding A2 fixed.
const submissionProbeKnownExact = "test"

// submissionProbeKnownPrefixes are the two observed sentence-length probe
// declarations, matched as the field's whole normalized content or its
// leading content (Captain correction C1): a probe that pads past A3's
// floor by appending text after either sentence still self-declares.
var submissionProbeKnownPrefixes = []string{
	"probe: minimal submission to isolate field validation",
	"isolation test summary padded to reasonable length to avoid floor issues",
}

// submissionProbeSelfLabelPrefixes catches a field whose own leading content
// labels itself a probe, without requiring the exact wording above.
var submissionProbeSelfLabelPrefixes = []string{
	"probe:",
	"probe -",
}

// submissionProbeSurfacePrefixes catches a field whose own leading content
// declares itself a test of the submit surface, the contract's own named
// phrase for exactly this behaviour.
var submissionProbeSurfacePrefixes = []string{
	"test of the submit surface",
	"a test of the submit surface",
	"this is a test of the submit surface",
	"this is a probe",
}

// submissionDeclaresProbe reports whether field's own primary content - not
// a clause buried inside a much larger document - declares it a probe or a
// test of the submit surface (A2), and if so which trigger class matched.
// Matching is structural (whole-field or leading-content), never a
// substring scan: a floor-satisfying field that quotes one of these strings
// mid-document (for instance, as documentation of this very behaviour) does
// not self-trigger, because the quoted string is not the field's own
// leading or whole content.
func submissionDeclaresProbe(field string) (bool, string) {
	normalized := normalizeSubmissionField(field)
	if normalized == "" {
		return false, ""
	}
	if normalized == submissionProbeKnownExact {
		return true, "known_probe_declaration"
	}
	for _, prefix := range submissionProbeKnownPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return true, "known_probe_declaration"
		}
	}
	for _, prefix := range submissionProbeSelfLabelPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return true, "self_label_prefix"
		}
	}
	for _, prefix := range submissionProbeSurfacePrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return true, "submit_surface_declaration"
		}
	}
	return false, ""
}

// submissionFloorResponsibility reports whether responsibility is one of the
// five A3 floors (planner_proposal, implementer_design,
// implementer_implementation, captain_review, work_verification).
// captain_plan_review and assembly_verification are both exempt, per the
// contract's own list.
func submissionFloorResponsibility(responsibility Responsibility) bool {
	switch responsibility {
	case PlannerProposal, ImplementerDesign, ImplementerImplementation,
		CaptainReview, WorkVerification:
		return true
	default:
		return false
	}
}

// submissionFloorCheck refuses a floored responsibility's summary or detail
// under its respective byte floor (A3), naming the field and the bound it
// fell under. Every other responsibility is exempt.
func submissionFloorCheck(submission Submission) error {
	if !submissionFloorResponsibility(submission.Responsibility) {
		return nil
	}
	if len([]byte(submission.Summary)) < MinSubmissionSummaryFloorBytes {
		return submissionFloorError("summary", "min_submission_summary_floor_bytes")
	}
	if len([]byte(submission.Detail)) < MinSubmissionDetailFloorBytes {
		return submissionFloorError("detail", "min_submission_detail_floor_bytes")
	}
	return nil
}
