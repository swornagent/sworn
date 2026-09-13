package driver

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
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
	Check        string   `json:"check"`
	Field        string   `json:"field,omitempty"`
	Bound        string   `json:"bound,omitempty"`
	Paths        []string `json:"paths,omitempty"`
	SandboxCheck string   `json:"sandbox_check,omitempty"`
	SandboxCause string   `json:"sandbox_cause,omitempty"`
	// Expected names the wire shape an INVALID_FIELD submit.decode refusal
	// wanted for Field ("string", "integer", "object"): a fixed engine
	// vocabulary, never the worker's own value (#306).
	Expected string `json:"expected,omitempty"`
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
// post-turn CHECK_EVIDENCE_INCOMPLETE it can never see or act on. S4-A1 additively
// extends it with the sandbox_start.* check name and bounded cause when the
// declared check's last recorded attempt failed with PROCESS_START_FAILED.
func submitCheckEvidenceIncompleteError(check, sandboxCheck, sandboxCause string) error {
	envelope := submissionRefusalDetail{
		Check:        "submit.check_evidence_incomplete",
		Field:        "checks",
		Bound:        "check_command_covers",
		Paths:        []string{check},
		SandboxCheck: sandboxCheck,
		SandboxCause: sandboxCause,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return &ContractError{
			Code: "CHECK_EVIDENCE_INCOMPLETE",
		}
	}
	return &ContractError{
		Code:   "CHECK_EVIDENCE_INCOMPLETE",
		Detail: string(body),
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
// UNKNOWN_FIELD, Field names only containingObject, never the worker-chosen
// key that triggered it. On INVALID_FIELD (closedObject raises it only when
// value itself is not a JSON object) Field names containingObject and
// Expected says "object", so a worker that sent the member as a string or
// an array learns which member and which shape in one step (#306).
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
	if contractErr.Code == "INVALID_FIELD" {
		return nil, submitDecodeTypeError(field, "object")
	}
	return nil, &ContractError{
		Code:   contractErr.Code,
		Detail: submissionRefusalDetailBytes("submit.decode", field, "", nil),
	}
}

// submitDecodeTypeError builds the INVALID_FIELD submit.decode refusal for
// an engine-known key carrying the wrong JSON type (#306): field is the
// schema key (or "<member>.<key>" for a nested blob or decision member) and
// expected the shape the wire schema wants for it. Both are fixed engine
// vocabulary; the offending value itself never reaches the envelope. A
// marshal failure (unreachable for this closed shape) yields "" rather than
// a partial envelope, matching submissionRefusalDetailBytes.
func submitDecodeTypeError(field, expected string) error {
	envelope := submissionRefusalDetail{
		Check:    "submit.decode",
		Field:    field,
		Expected: expected,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return &ContractError{Code: "INVALID_FIELD"}
	}
	return &ContractError{Code: "INVALID_FIELD", Detail: string(body)}
}

// submissionValueHasShape reports whether a decoded JSON value has the wire
// shape named by expected. It goes by reflect kind rather than concrete
// type so that scripted fixtures handing decodeToolSubmission Go-native
// values (a Responsibility constant, an int byte_count) are judged the same
// way as the live path's decodeStrict output (string, json.Number).
func submissionValueHasShape(value any, expected string) bool {
	switch expected {
	case "string":
		// json.Number is a named string type under reflect, but on the wire
		// it was a number: it is never a string here.
		if _, number := value.(json.Number); number {
			return false
		}
		return reflect.ValueOf(value).Kind() == reflect.String
	case "integer":
		if _, ok := value.(json.Number); ok {
			return true
		}
		switch reflect.ValueOf(value).Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			return true
		}
		return false
	case "object":
		_, ok := value.(map[string]any)
		return ok
	default:
		return true
	}
}

// submissionStringKeys are the wire schema's five top-level string members;
// submissionBlobKeys the three members of an exact-bytes blob after
// materializeExactBytesPaths has replaced any path with bytes. Both feed
// requireSubmissionMemberTypes, which is the only place that names them.
var (
	submissionStringKeys = []string{
		"schema_version", "invocation_id", "responsibility", "summary", "detail",
	}
	submissionBlobKeys = map[string]string{
		"byte_count": "integer",
		"digest":     "string",
		"bytes":      "string",
	}
)

// requireSubmissionMemberTypes refuses the first engine-known key in object
// whose JSON type disagrees with the wire schema, naming the key and the
// expected shape (#306). Before this check a wrong-typed known key - the
// observed detail sent as {path: ...} - fell through closedObject (which
// only checks presence) to json.Unmarshal and refused INVALID_SUBMISSION
// with no field at all. prefix is "" for the root submission and
// "<member>." for a nested blob or decision member. Keys absent from
// object are not this check's business (closedObject already enforced
// presence for required ones).
func requireSubmissionMemberTypes(object map[string]any, prefix string, expected map[string]string, order []string) error {
	for _, key := range order {
		value, present := object[key]
		if !present || value == nil {
			continue
		}
		if !submissionValueHasShape(value, expected[key]) {
			return submitDecodeTypeError(prefix+key, expected[key])
		}
	}
	return nil
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

// submissionProbeKnownExacts are the two observed real-world probe payloads
// that are a single bare word: matched only by whole-field equality, never
// as a prefix, since honest work routinely opens with "Test coverage ..."
// or "Probe ordering ..." and anchoring either as a prefix would re-open the
// over-match finding A2 fixed. "probe" is the 2026-09-12 Captain receipt
// (run 2026-09-11-phased-evidence-r7, S2 design t1) whose whole detail body
// was that one word, accepted as an authority decision (#300 escalation).
var submissionProbeKnownExacts = []string{"test", "probe"}

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
	for _, exact := range submissionProbeKnownExacts {
		if normalized == exact {
			return true, "known_probe_declaration"
		}
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

// submissionPointerPrefixes are the leading phrases an observed "see the
// file I wrote" detail body opens with (#307): run 2026-09-11-phased-evidence
// -r3 S1 design attempts 4 and 7 submitted "See detail file: ..." and "See
// attached detail: ..." as the whole design, and both were accepted and then
// judged "contains no design" by the Captain. Matched on normalized leading
// content only, like submissionDeclaresProbe.
var submissionPointerPrefixes = []string{
	"see attached",
	"see the attached",
	"see detail file",
	"see the detail file",
	"see file",
	"see the file",
	"attached:",
	"attached file",
	"full detail in",
	"full design in",
}

// submissionPointerMaxBytes bounds the unattached-pointer refusal: a body
// past this size is a document in its own right even if it opens with "see
// attached" (the observed real designs were 20 KB and more; the observed
// pointers were under 2 KB), so only a short body refuses.
const submissionPointerMaxBytes = 4096

// submissionIsUnattachedPointer reports whether detail is a short body whose
// own leading content points the reader at a file the submission does not
// carry (#307). The submit surface has no path form for detail - it is
// inline text only - so such a body can never be resolved by the engine and
// is refused with a typed correction instead of being sealed as the work.
func submissionIsUnattachedPointer(detail string) (bool, string) {
	if len(detail) > submissionPointerMaxBytes {
		return false, ""
	}
	normalized := normalizeSubmissionField(detail)
	if normalized == "" {
		return false, ""
	}
	for _, prefix := range submissionPointerPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return true, "unattached_file_pointer"
		}
	}
	return false, ""
}

// submissionPointerError builds the SUBMISSION_UNATTACHED_POINTER refusal
// submissionIsUnattachedPointer raises: Field names the member (detail),
// Bound the trigger class - never the body's own text.
func submissionPointerError(field, bound string) error {
	return &ContractError{
		Code:   "SUBMISSION_UNATTACHED_POINTER",
		Detail: submissionRefusalDetailBytes("submit.detail_pointer", field, bound, nil),
	}
}

// detailRequiredResponsibility reports whether responsibility is one of the
// five responsibilities (planner_proposal, implementer_design,
// implementer_implementation, captain_review, work_verification) whose
// Detail must be non-empty. captain_plan_review and assembly_verification
// are both exempt, per the contract's own list.
func detailRequiredResponsibility(responsibility Responsibility) bool {
	switch responsibility {
	case PlannerProposal, ImplementerDesign, ImplementerImplementation,
		CaptainReview, WorkVerification:
		return true
	default:
		return false
	}
}
