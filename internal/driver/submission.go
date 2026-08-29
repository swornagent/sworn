package driver

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/swornagent/sworn/internal/baton"
)

const (
	SubmissionSchemaVersion = "sworn.submission/v1"
	PermissionSchemaVersion = "sworn.submission-permission/v1"
	SealSchemaVersion       = "sworn.submission-seal/v1"
	MaxFrameBytes           = 4_194_304
	MaxSubmissionBytes      = 2_097_152
	// Summary and detail are bounded only by the tool-argument transport
	// guard (MaxToolArgumentBytes); expression caps below it are removed and
	// may return only from quantitative eval evidence.
	MaxSubmissionSummaryBytes = 262_144
	MaxSubmissionDetailBytes  = 262_144
	MaxPlanBytes              = 1_048_576
	MaxCheckBytes             = 1_048_576
)

// ExactBytes is a length- and digest-bound payload. It is used only for the
// exact plan or exact check-result bytes required by a responsibility.
type ExactBytes struct {
	ByteCount int64  `json:"byte_count"`
	Digest    string `json:"digest"`
	Bytes     string `json:"bytes"`
}

func NewPlanBytes(body []byte) (*ExactBytes, error) {
	if err := validatePlanBytes(body); err != nil {
		return nil, err
	}
	return newExactBytes(body, MaxPlanBytes)
}

func NewCheckBytes(body []byte) (*ExactBytes, error) {
	return newExactBytes(body, MaxCheckBytes)
}

func newExactBytes(body []byte, maximum int) (*ExactBytes, error) {
	if len(body) == 0 || len(body) > maximum {
		return nil, fail("INVALID_EXACT_BYTES")
	}
	return &ExactBytes{
		ByteCount: int64(len(body)),
		Digest:    Digest(body),
		Bytes:     base64.StdEncoding.EncodeToString(body),
	}, nil
}

type DecisionOutcome string

const (
	DecisionProceed  DecisionOutcome = "proceed"
	DecisionRevise   DecisionOutcome = "revise"
	DecisionEscalate DecisionOutcome = "escalate"
	DecisionPass     DecisionOutcome = "pass"
	DecisionFail     DecisionOutcome = "fail"
	DecisionBlocked  DecisionOutcome = "blocked"
)

type Decision struct {
	Outcome DecisionOutcome `json:"outcome"`
}

func NewDecision(outcome DecisionOutcome) (*Decision, error) {
	if !outcome.valid() {
		return nil, fail("INVALID_DECISION")
	}
	return &Decision{Outcome: outcome}, nil
}

type Responsibility string

const (
	PlannerProposal           Responsibility = "planner_proposal"
	ImplementerDesign         Responsibility = "implementer_design"
	ImplementerImplementation Responsibility = "implementer_implementation"
	CaptainReview             Responsibility = "captain_review"
	CaptainPlanReview         Responsibility = "captain_plan_review"
	WorkVerification          Responsibility = "work_verification"
	AssemblyVerification      Responsibility = "assembly_verification"
)

type Submission struct {
	SchemaVersion  string         `json:"schema_version"`
	InvocationID   string         `json:"invocation_id"`
	Responsibility Responsibility `json:"responsibility"`
	Summary        string         `json:"summary"`
	Detail         string         `json:"detail"`
	Plan           *ExactBytes    `json:"plan"`
	Checks         *ExactBytes    `json:"checks"`
	Decision       *Decision      `json:"decision"`
	// Contracts carries new slice-contract files a planner_proposal mints,
	// keyed by their manifest contract_path, so they ride the proposal as
	// durably as plan bytes. Every other responsibility must leave it nil.
	Contracts map[string]*ExactBytes `json:"contracts,omitempty"`
}

func EncodeSubmission(submission Submission) ([]byte, error) {
	if err := ValidateSubmission(submission); err != nil {
		return nil, err
	}
	body, err := json.Marshal(submission)
	if err != nil || len(body)+1 > MaxSubmissionBytes {
		return nil, fail("RESOURCE_LIMIT")
	}
	return append(body, '\n'), nil
}

func DecodeSubmission(body []byte) (Submission, error) {
	if len(body) < 2 || len(body) > MaxSubmissionBytes || body[len(body)-1] != '\n' {
		return Submission{}, fail("INVALID_SUBMISSION")
	}
	var submission Submission
	root, err := decodeTyped(
		body,
		MaxSubmissionBytes,
		[]string{
			"schema_version",
			"invocation_id",
			"responsibility",
			"summary",
			"detail",
			"plan",
			"checks",
			"decision",
		},
		[]string{"contracts"},
		&submission,
	)
	if err != nil {
		return Submission{}, err
	}
	for _, name := range []string{"plan", "checks"} {
		if root[name] == nil {
			continue
		}
		if _, err := closedObject(
			root[name],
			[]string{"byte_count", "digest", "bytes"},
			nil,
		); err != nil {
			return Submission{}, err
		}
	}
	if root["contracts"] != nil {
		contracts, ok := root["contracts"].(map[string]any)
		if !ok {
			return Submission{}, fail("INVALID_FIELD")
		}
		for _, member := range contracts {
			if _, err := closedObject(
				member,
				[]string{"byte_count", "digest", "bytes"},
				nil,
			); err != nil {
				return Submission{}, err
			}
		}
	}
	if root["decision"] != nil {
		if _, err := closedObject(root["decision"], []string{"outcome"}, nil); err != nil {
			return Submission{}, err
		}
	}
	if err := ValidateSubmission(submission); err != nil {
		return Submission{}, err
	}
	canonical, err := EncodeSubmission(submission)
	if err != nil {
		return Submission{}, err
	}
	if !bytes.Equal(canonical, body) {
		return Submission{}, fail("NONCANONICAL_JSON")
	}
	return submission, nil
}

func ValidateSubmission(submission Submission) error {
	if submission.SchemaVersion != SubmissionSchemaVersion {
		return submissionValidateError("INVALID_VERSION", "schema_version")
	}
	if err := validateIdentity(submission.InvocationID); err != nil {
		return submissionValidateWrap(err, "invocation_id")
	}
	if !submission.Responsibility.valid() {
		return submissionValidateError("INVALID_RESPONSIBILITY", "responsibility")
	}
	if !utf8.ValidString(submission.Summary) ||
		len([]byte(submission.Summary)) > MaxSubmissionSummaryBytes ||
		strings.TrimSpace(submission.Summary) == "" {
		return submissionValidateError("INVALID_SUMMARY", "summary")
	}
	if err := validateSubmissionDetail(submission.Detail); err != nil {
		return submissionValidateWrap(err, "detail")
	}
	if submission.Plan != nil {
		if err := validateExactBytes(*submission.Plan, MaxPlanBytes, true); err != nil {
			return submissionValidateWrap(err, "plan")
		}
		planBody, _ := base64.StdEncoding.Strict().DecodeString(submission.Plan.Bytes)
		if err := validatePlanBytes(planBody); err != nil {
			return submissionValidateWrap(err, "plan")
		}
	}
	if submission.Checks != nil {
		if err := validateExactBytes(*submission.Checks, MaxCheckBytes, false); err != nil {
			return submissionValidateWrap(err, "checks")
		}
	}
	if submission.Decision != nil && !submission.Decision.Outcome.valid() {
		return submissionValidateError("INVALID_DECISION", "decision")
	}
	for path, contract := range submission.Contracts {
		if validateText(path, MaxToolPathBytes, false) != nil || contract == nil {
			return submissionValidateError("INVALID_EXACT_BYTES", "contracts entry")
		}
		if err := validateExactBytes(*contract, MaxPlanBytes, true); err != nil {
			return submissionValidateWrap(err, "contracts entry")
		}
	}

	switch submission.Responsibility {
	case PlannerProposal:
		switch {
		case submission.Plan == nil:
			return submissionShapeMismatch("plan")
		case submission.Checks != nil:
			return submissionShapeMismatch("checks")
		case submission.Decision != nil:
			return submissionShapeMismatch("decision")
		}
	case ImplementerDesign:
		switch {
		case submission.Plan != nil:
			return submissionShapeMismatch("plan")
		case submission.Checks != nil:
			return submissionShapeMismatch("checks")
		case submission.Decision != nil:
			return submissionShapeMismatch("decision")
		case submission.Contracts != nil:
			return submissionShapeMismatch("contracts")
		}
	case ImplementerImplementation:
		switch {
		case submission.Plan != nil:
			return submissionShapeMismatch("plan")
		case submission.Checks == nil:
			return submissionShapeMismatch("checks")
		case submission.Decision != nil:
			return submissionShapeMismatch("decision")
		case submission.Contracts != nil:
			return submissionShapeMismatch("contracts")
		}
	case CaptainReview, CaptainPlanReview:
		switch {
		case submission.Plan != nil:
			return submissionShapeMismatch("plan")
		case submission.Checks != nil:
			return submissionShapeMismatch("checks")
		case submission.Decision == nil:
			return submissionShapeMismatch("decision")
		case !submission.Decision.Outcome.captain():
			return submissionShapeMismatch("decision")
		case submission.Contracts != nil:
			return submissionShapeMismatch("contracts")
		}
	case WorkVerification, AssemblyVerification:
		switch {
		case submission.Plan != nil:
			return submissionShapeMismatch("plan")
		case submission.Checks == nil:
			return submissionShapeMismatch("checks")
		case submission.Decision == nil:
			return submissionShapeMismatch("decision")
		case !submission.Decision.Outcome.verifier():
			return submissionShapeMismatch("decision")
		case submission.Contracts != nil:
			return submissionShapeMismatch("contracts")
		}
	default:
		return submissionValidateError("INVALID_RESPONSIBILITY", "responsibility")
	}
	return nil
}

func validateExactBytes(value ExactBytes, maximum int, requireUTF8 bool) error {
	if value.ByteCount < 1 || value.ByteCount > int64(maximum) ||
		!digestPattern.MatchString(value.Digest) {
		return fail("INVALID_EXACT_BYTES")
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(value.Bytes)
	if err != nil || int64(len(decoded)) != value.ByteCount ||
		base64.StdEncoding.EncodeToString(decoded) != value.Bytes ||
		Digest(decoded) != value.Digest ||
		(requireUTF8 && !utf8.Valid(decoded)) {
		return fail("INVALID_EXACT_BYTES")
	}
	return nil
}

func validateSubmissionDetail(detail string) error {
	if len([]byte(detail)) > MaxSubmissionDetailBytes || !utf8.ValidString(detail) ||
		strings.ContainsRune(detail, '\x00') || strings.ContainsRune(detail, '\r') ||
		strings.Contains(detail, "Baton-Detail-Begin") ||
		strings.Contains(detail, "Baton-Detail-End") {
		return fail("INVALID_DETAIL")
	}
	return nil
}

// validatePlanBytes delegates plan admission to the one canonical parser
// shared with plan recording: any bytes a Planner submission carries must be
// exactly what baton.ParsePlan would later admit, whether that is the legacy
// baton.plan/v2 fence or a sworn.release-manifest/v1 manifest. This drops the
// prior driver-local fence/JSON precheck, which duplicated and could drift
// from that authority. The wire-facing code stays INVALID_PLAN_BYTES for
// every rejection, matching this package's existing certification mapping
// and tests.
func validatePlanBytes(body []byte) error {
	if _, err := baton.ParsePlan(body); err != nil {
		return fail("INVALID_PLAN_BYTES")
	}
	return nil
}

func (responsibility Responsibility) valid() bool {
	switch responsibility {
	case PlannerProposal,
		ImplementerDesign,
		ImplementerImplementation,
		CaptainReview,
		CaptainPlanReview,
		WorkVerification,
		AssemblyVerification:
		return true
	default:
		return false
	}
}

func (outcome DecisionOutcome) valid() bool {
	return outcome.captain() || outcome.verifier()
}

func (outcome DecisionOutcome) captain() bool {
	return outcome == DecisionProceed ||
		outcome == DecisionRevise ||
		outcome == DecisionEscalate
}

func (outcome DecisionOutcome) verifier() bool {
	return outcome == DecisionPass ||
		outcome == DecisionFail ||
		outcome == DecisionBlocked
}

type ContainmentProfile string

const (
	ContainmentReadOnly  ContainmentProfile = "linux_bwrap_read_only/v1"
	ContainmentReadWrite ContainmentProfile = "linux_bwrap_read_write/v1"
)

type PermissionDescriptor struct {
	SchemaVersion       string             `json:"schema_version"`
	Package             PackageIdentity    `json:"package"`
	InvocationID        string             `json:"invocation_id"`
	RequestDigest       string             `json:"request_digest"`
	Role                Role               `json:"role"`
	OperationID         string             `json:"operation_id"`
	ProfileKey          string             `json:"profile_key"`
	AdapterID           string             `json:"adapter_id"`
	AdapterVersion      string             `json:"adapter_version"`
	AdapterConfigDigest string             `json:"adapter_config_digest"`
	Network             NetworkPolicy      `json:"network"`
	Model               string             `json:"model"`
	WorkspaceAccess     WorkspaceAccess    `json:"workspace_access"`
	FreshContext        bool               `json:"fresh_context"`
	InputsDigest        string             `json:"inputs_digest"`
	Containment         ContainmentProfile `json:"containment"`
	Responsibility      Responsibility     `json:"responsibility"`
}

type SubmissionPermission struct {
	descriptor PermissionDescriptor
}

func NewSubmissionPermission(
	request Request,
	selected SelectedProfile,
	containment ContainmentProfile,
	responsibility Responsibility,
) (SubmissionPermission, error) {
	if err := ValidateRequest(request); err != nil {
		return SubmissionPermission{}, err
	}
	if err := validateSelectedProfile(selected); err != nil {
		return SubmissionPermission{}, err
	}
	if request.Profile != selected.Profile.Key || request.Model != selected.Model {
		return SubmissionPermission{}, fail("PERMISSION_BINDING_MISMATCH")
	}
	_, packageIdentity, err := admittedPackage()
	if err != nil {
		return SubmissionPermission{}, fail("INVALID_PACKAGE")
	}
	requestBody, err := EncodeRequest(request)
	if err != nil {
		return SubmissionPermission{}, err
	}
	inputBody, err := canonicalJSON(request.Inputs)
	if err != nil {
		return SubmissionPermission{}, err
	}
	expectedContainment := ContainmentReadWrite
	if request.Workspace.Access == ReadOnly {
		expectedContainment = ContainmentReadOnly
	}
	if containment != expectedContainment {
		return SubmissionPermission{}, fail("PERMISSION_BINDING_MISMATCH")
	}
	permission := SubmissionPermission{descriptor: PermissionDescriptor{
		SchemaVersion:       PermissionSchemaVersion,
		Package:             packageIdentity,
		InvocationID:        request.InvocationID,
		RequestDigest:       Digest(requestBody),
		Role:                request.Role,
		OperationID:         request.Operation.ID,
		ProfileKey:          selected.Profile.Key,
		AdapterID:           selected.Adapter.ID,
		AdapterVersion:      selected.Adapter.Version,
		AdapterConfigDigest: selected.Adapter.ConfigurationDigest,
		Network:             selected.Profile.Network,
		Model:               selected.Model,
		WorkspaceAccess:     request.Workspace.Access,
		FreshContext:        request.FreshContext,
		InputsDigest:        Digest(inputBody),
		Containment:         containment,
		Responsibility:      responsibility,
	}}
	if err := validateResponsibility(permission.descriptor); err != nil {
		return SubmissionPermission{}, err
	}
	return permission, nil
}

func (permission SubmissionPermission) Describe() (PermissionDescriptor, error) {
	if err := validateResponsibility(permission.descriptor); err != nil {
		return PermissionDescriptor{}, err
	}
	return permission.descriptor, nil
}

func validateResponsibility(descriptor PermissionDescriptor) error {
	_, packageIdentity, packageErr := admittedPackage()
	if descriptor.SchemaVersion != PermissionSchemaVersion ||
		packageErr != nil || descriptor.Package != packageIdentity ||
		!digestPattern.MatchString(descriptor.RequestDigest) ||
		!digestPattern.MatchString(descriptor.InputsDigest) ||
		validateIdentity(descriptor.InvocationID) != nil ||
		!descriptor.Role.valid() ||
		!providerKeyPattern.MatchString(descriptor.ProfileKey) ||
		!driverIdentityPattern.MatchString(descriptor.AdapterID) ||
		!versionPattern.MatchString(descriptor.AdapterVersion) ||
		!digestPattern.MatchString(descriptor.AdapterConfigDigest) ||
		validateNetworkPolicy(descriptor.AdapterID, descriptor.Network) != nil ||
		validateText(descriptor.Model, 500, false) != nil ||
		operationForRole[descriptor.Role] != descriptor.OperationID {
		return fail("INVALID_PERMISSION")
	}
	expectedContainment := ContainmentReadWrite
	if descriptor.WorkspaceAccess == ReadOnly {
		expectedContainment = ContainmentReadOnly
	} else if descriptor.WorkspaceAccess != ReadWrite {
		return fail("INVALID_PERMISSION")
	}
	if descriptor.Containment != expectedContainment {
		return fail("INVALID_PERMISSION")
	}
	switch descriptor.Responsibility {
	case PlannerProposal:
		if descriptor.Role != RolePlanner {
			return fail("INVALID_PERMISSION")
		}
	case ImplementerDesign, ImplementerImplementation:
		if descriptor.Role != RoleImplementer {
			return fail("INVALID_PERMISSION")
		}
	case CaptainReview, CaptainPlanReview:
		if descriptor.Role != RoleCaptain {
			return fail("INVALID_PERMISSION")
		}
	case WorkVerification:
		if descriptor.Role != RoleVerifier ||
			descriptor.WorkspaceAccess != ReadOnly {
			return fail("INVALID_PERMISSION")
		}
	case AssemblyVerification:
		if descriptor.Role != RoleVerifier ||
			descriptor.WorkspaceAccess != ReadOnly ||
			!descriptor.FreshContext {
			return fail("INVALID_PERMISSION")
		}
	default:
		return fail("INVALID_PERMISSION")
	}
	return nil
}

func (permission SubmissionPermission) validate(submission Submission) error {
	if err := ValidateSubmission(submission); err != nil {
		return err
	}
	switch {
	case submission.InvocationID != permission.descriptor.InvocationID:
		return submissionPermissionMismatch("invocation_id")
	case submission.Responsibility != permission.descriptor.Responsibility:
		return submissionPermissionMismatch("responsibility")
	}
	return nil
}

type Seal struct {
	SchemaVersion    string `json:"schema_version"`
	InvocationID     string `json:"invocation_id"`
	SubmissionDigest string `json:"submission_digest"`
	Accepted         bool   `json:"accepted"`
	Code             string `json:"code"`
}

type submissionServer struct {
	mu         sync.Mutex
	permission SubmissionPermission
	sealed     bool
	body       []byte
	seal       Seal
	sealBytes  []byte
}

func newSubmissionServer(permission SubmissionPermission) (*submissionServer, error) {
	if _, err := permission.Describe(); err != nil {
		return nil, err
	}
	return &submissionServer{permission: permission}, nil
}

func (server *submissionServer) Submit(body []byte) (Seal, []byte, error) {
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.sealed {
		if bytes.Equal(body, server.body) {
			if !server.seal.Accepted {
				return server.seal,
					append([]byte(nil), server.sealBytes...),
					fail("SUBMISSION_REJECTED")
			}
			return server.seal, append([]byte(nil), server.sealBytes...), nil
		}
		return server.seal,
			append([]byte(nil), server.sealBytes...),
			fail("SUBMISSION_CONFLICT")
	}
	server.sealed = true
	server.body = append([]byte(nil), body...)
	submission, err := DecodeSubmission(body)
	code := "accepted"
	accepted := err == nil
	if accepted {
		err = server.permission.validate(submission)
		accepted = err == nil
	}
	if err != nil {
		var contractErr *ContractError
		if errors.As(err, &contractErr) {
			code = contractErr.Code
		} else {
			code = "SUBMISSION_REJECTED"
		}
	}
	server.seal = Seal{
		SchemaVersion:    SealSchemaVersion,
		InvocationID:     server.permission.descriptor.InvocationID,
		SubmissionDigest: Digest(body),
		Accepted:         accepted,
		Code:             code,
	}
	sealBytes, marshalErr := json.Marshal(server.seal)
	if marshalErr != nil {
		return Seal{}, nil, fail("INVALID_JSON")
	}
	server.sealBytes = append(sealBytes, '\n')
	if !accepted {
		return server.seal, append([]byte(nil), server.sealBytes...), fail("SUBMISSION_REJECTED")
	}
	return server.seal, append([]byte(nil), server.sealBytes...), nil
}

func DecodeSeal(body []byte) (Seal, error) {
	if len(body) < 2 || len(body) > 4_096 || body[len(body)-1] != '\n' {
		return Seal{}, fail("INVALID_SEAL")
	}
	var seal Seal
	if _, err := decodeTyped(
		body,
		4_096,
		[]string{
			"schema_version",
			"invocation_id",
			"submission_digest",
			"accepted",
			"code",
		},
		nil,
		&seal,
	); err != nil {
		return Seal{}, err
	}
	if seal.SchemaVersion != SealSchemaVersion ||
		validateIdentity(seal.InvocationID) != nil ||
		!digestPattern.MatchString(seal.SubmissionDigest) ||
		validateText(seal.Code, 128, false) != nil ||
		(seal.Accepted != (seal.Code == "accepted")) {
		return Seal{}, fail("INVALID_SEAL")
	}
	canonical, err := json.Marshal(seal)
	if err != nil {
		return Seal{}, fail("INVALID_JSON")
	}
	if !bytes.Equal(append(canonical, '\n'), body) {
		return Seal{}, fail("NONCANONICAL_JSON")
	}
	return seal, nil
}

func EncodeFrame(payload []byte) ([]byte, error) {
	if len(payload) == 0 || len(payload) > MaxFrameBytes ||
		payload[len(payload)-1] != '\n' || !utf8.Valid(payload) {
		return nil, fail("INVALID_FRAME")
	}
	if _, err := decodeStrict(payload, MaxFrameBytes); err != nil {
		return nil, err
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, payload[:len(payload)-1]); err != nil {
		return nil, fail("INVALID_JSON")
	}
	if !bytes.Equal(append(compact.Bytes(), '\n'), payload) {
		return nil, fail("NONCANONICAL_JSON")
	}
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
	copy(frame[4:], payload)
	return frame, nil
}

func ReadFrame(reader io.Reader) ([]byte, error) {
	var header [4]byte
	if count, err := io.ReadFull(reader, header[:]); err != nil {
		if count == 0 && err == io.EOF {
			return nil, fail("ENDPOINT_CLOSED")
		}
		return nil, fail("PARTIAL_FRAME")
	}
	length := binary.BigEndian.Uint32(header[:])
	if length == 0 || length > MaxFrameBytes {
		return nil, fail("INVALID_FRAME")
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, fail("PARTIAL_FRAME")
	}
	if _, err := EncodeFrame(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func WriteFrame(writer io.Writer, payload []byte) error {
	frame, err := EncodeFrame(payload)
	if err != nil {
		return err
	}
	if count, err := writer.Write(frame); err != nil || count != len(frame) {
		return fail("FRAME_WRITE_FAILED")
	}
	return nil
}
