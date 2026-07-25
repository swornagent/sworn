package driver

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"unicode/utf8"
)

const (
	SubmissionSchemaVersion = "sworn.submission/v1"
	SealSchemaVersion       = "sworn.submission-seal/v1"
	MaxFrameBytes           = 4_194_304
	MaxSubmissionBytes      = 2_097_152
	MaxArtifactBytes        = 1_048_576
	MaxArtifactTotalBytes   = 1_048_576
	MaxDecisionEvidence     = 262_144
)

type ArtifactKind string

const (
	ArtifactPlan           ArtifactKind = "plan"
	ArtifactDesign         ArtifactKind = "design"
	ArtifactWorkProof      ArtifactKind = "work_proof"
	ArtifactWorkStatus     ArtifactKind = "work_status"
	ArtifactAssemblyStatus ArtifactKind = "assembly_status"
)

type Artifact struct {
	Kind      ArtifactKind `json:"kind"`
	ByteCount int64        `json:"byte_count"`
	Digest    string       `json:"digest"`
	Bytes     string       `json:"bytes"`
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
	Outcome        DecisionOutcome `json:"outcome"`
	EvidenceBytes  int64           `json:"evidence_bytes"`
	EvidenceDigest string          `json:"evidence_digest"`
	Evidence       string          `json:"evidence"`
}

type Submission struct {
	SchemaVersion string     `json:"schema_version"`
	InvocationID  string     `json:"invocation_id"`
	Artifacts     []Artifact `json:"artifacts"`
	Decision      *Decision  `json:"decision"`
}

func NewArtifact(kind ArtifactKind, body []byte) (Artifact, error) {
	if !kind.valid() || len(body) > artifactMaximum(kind) || !utf8.Valid(body) {
		return Artifact{}, fail("INVALID_ARTIFACT")
	}
	return Artifact{
		Kind:      kind,
		ByteCount: int64(len(body)),
		Digest:    Digest(body),
		Bytes:     base64.StdEncoding.EncodeToString(body),
	}, nil
}

func NewDecision(outcome DecisionOutcome, evidence []byte) (*Decision, error) {
	if !outcome.valid() || len(evidence) < 1 || len(evidence) > MaxDecisionEvidence ||
		!utf8.Valid(evidence) {
		return nil, fail("INVALID_DECISION")
	}
	return &Decision{
		Outcome:        outcome,
		EvidenceBytes:  int64(len(evidence)),
		EvidenceDigest: Digest(evidence),
		Evidence:       base64.StdEncoding.EncodeToString(evidence),
	}, nil
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
	value, err := decodeStrict(body, MaxSubmissionBytes)
	if err != nil {
		return Submission{}, err
	}
	root, err := closedObject(value,
		[]string{"schema_version", "invocation_id", "artifacts", "decision"}, nil)
	if err != nil {
		return Submission{}, err
	}
	var submission Submission
	if submission.SchemaVersion, err = requiredString(root, "schema_version"); err != nil {
		return Submission{}, err
	}
	if submission.InvocationID, err = requiredString(root, "invocation_id"); err != nil {
		return Submission{}, err
	}
	artifactValues, ok := root["artifacts"].([]any)
	if !ok {
		return Submission{}, fail("INVALID_ARTIFACT")
	}
	submission.Artifacts = make([]Artifact, 0, len(artifactValues))
	for _, value := range artifactValues {
		object, err := closedObject(value,
			[]string{"kind", "byte_count", "digest", "bytes"}, nil)
		if err != nil {
			return Submission{}, err
		}
		var artifact Artifact
		kind, err := requiredString(object, "kind")
		if err != nil {
			return Submission{}, err
		}
		artifact.Kind = ArtifactKind(kind)
		if artifact.ByteCount, err = requiredInteger(object, "byte_count"); err != nil {
			return Submission{}, err
		}
		if artifact.Digest, err = requiredString(object, "digest"); err != nil {
			return Submission{}, err
		}
		if artifact.Bytes, err = requiredStringAllowEmpty(object, "bytes"); err != nil {
			return Submission{}, err
		}
		submission.Artifacts = append(submission.Artifacts, artifact)
	}
	if root["decision"] != nil {
		object, err := closedObject(root["decision"],
			[]string{"outcome", "evidence_bytes", "evidence_digest", "evidence"}, nil)
		if err != nil {
			return Submission{}, err
		}
		decision := &Decision{}
		outcome, err := requiredString(object, "outcome")
		if err != nil {
			return Submission{}, err
		}
		decision.Outcome = DecisionOutcome(outcome)
		if decision.EvidenceBytes, err = requiredInteger(object, "evidence_bytes"); err != nil {
			return Submission{}, err
		}
		if decision.EvidenceDigest, err = requiredString(object, "evidence_digest"); err != nil {
			return Submission{}, err
		}
		if decision.Evidence, err = requiredString(object, "evidence"); err != nil {
			return Submission{}, err
		}
		submission.Decision = decision
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
		return fail("INVALID_VERSION")
	}
	if err := validateIdentity(submission.InvocationID); err != nil {
		return err
	}
	if submission.Artifacts == nil || len(submission.Artifacts) > 2 {
		return fail("INVALID_ARTIFACT")
	}
	var total int64
	for _, artifact := range submission.Artifacts {
		if !artifact.Kind.valid() || artifact.ByteCount < 0 ||
			artifact.ByteCount > int64(artifactMaximum(artifact.Kind)) ||
			!digestPattern.MatchString(artifact.Digest) {
			return fail("INVALID_ARTIFACT")
		}
		decoded, err := base64.StdEncoding.Strict().DecodeString(artifact.Bytes)
		if err != nil || int64(len(decoded)) != artifact.ByteCount ||
			base64.StdEncoding.EncodeToString(decoded) != artifact.Bytes ||
			Digest(decoded) != artifact.Digest || !utf8.Valid(decoded) {
			return fail("INVALID_ARTIFACT")
		}
		total += artifact.ByteCount
	}
	if total > MaxArtifactTotalBytes {
		return fail("RESOURCE_LIMIT")
	}
	if submission.Decision != nil {
		decision := submission.Decision
		if !decision.Outcome.valid() || decision.EvidenceBytes < 1 ||
			decision.EvidenceBytes > MaxDecisionEvidence ||
			!digestPattern.MatchString(decision.EvidenceDigest) {
			return fail("INVALID_DECISION")
		}
		evidence, err := base64.StdEncoding.Strict().DecodeString(decision.Evidence)
		if err != nil || int64(len(evidence)) != decision.EvidenceBytes ||
			base64.StdEncoding.EncodeToString(evidence) != decision.Evidence ||
			Digest(evidence) != decision.EvidenceDigest || !utf8.Valid(evidence) {
			return fail("INVALID_DECISION")
		}
	}
	return nil
}

func (kind ArtifactKind) valid() bool {
	switch kind {
	case ArtifactPlan, ArtifactDesign, ArtifactWorkProof, ArtifactWorkStatus, ArtifactAssemblyStatus:
		return true
	default:
		return false
	}
}

func artifactMaximum(kind ArtifactKind) int {
	if kind == ArtifactPlan {
		return MaxArtifactBytes
	}
	return 262_144
}

func (outcome DecisionOutcome) valid() bool {
	switch outcome {
	case DecisionProceed, DecisionRevise, DecisionEscalate,
		DecisionPass, DecisionFail, DecisionBlocked:
		return true
	default:
		return false
	}
}

type Responsibility string

const (
	PlannerProposal           Responsibility = "planner_proposal"
	ImplementerDesign         Responsibility = "implementer_design"
	ImplementerImplementation Responsibility = "implementer_implementation"
	CaptainReview             Responsibility = "captain_review"
	WorkVerification          Responsibility = "work_verification"
	AssemblyVerification      Responsibility = "assembly_verification"
)

type ContainmentProfile string

const (
	ContainmentReadOnly  ContainmentProfile = "linux_bwrap_read_only/v1"
	ContainmentReadWrite ContainmentProfile = "linux_bwrap_read_write/v1"
)

type PermissionDescriptor struct {
	SchemaVersion    string             `json:"schema_version"`
	InvocationID     string             `json:"invocation_id"`
	RequestDigest    string             `json:"request_digest"`
	Role             Role               `json:"role"`
	OperationID      string             `json:"operation_id"`
	ProviderKey      string             `json:"provider_key"`
	DriverID         string             `json:"driver_id"`
	DriverVersion    string             `json:"driver_version"`
	ExecutableDigest string             `json:"executable_digest"`
	Network          NetworkPolicy      `json:"network"`
	Model            string             `json:"model"`
	WorkspaceAccess  WorkspaceAccess    `json:"workspace_access"`
	FreshContext     bool               `json:"fresh_context"`
	InputsDigest     string             `json:"inputs_digest"`
	Containment      ContainmentProfile `json:"containment"`
	Responsibility   Responsibility     `json:"responsibility"`
}

type SubmissionPermission struct {
	descriptor PermissionDescriptor
}

func NewSubmissionPermission(
	request Request,
	selected SelectedProvider,
	containment ContainmentProfile,
	responsibility Responsibility,
) (SubmissionPermission, error) {
	if err := ValidateRequest(request); err != nil {
		return SubmissionPermission{}, err
	}
	if request.Role == RoleMerge || request.Model == nil ||
		*request.Model != selected.Model {
		return SubmissionPermission{}, fail("PERMISSION_BINDING_MISMATCH")
	}
	if !providerKeyPattern.MatchString(selected.Provider.Key) ||
		!driverIdentityPattern.MatchString(selected.Provider.DriverID) ||
		!versionPattern.MatchString(selected.Provider.DriverVersion) ||
		!digestPattern.MatchString(selected.Provider.Executable.Digest) ||
		(selected.Provider.Network != NetworkNone &&
			selected.Provider.Network != NetworkRequired) {
		return SubmissionPermission{}, fail("PERMISSION_BINDING_MISMATCH")
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
		SchemaVersion:    "sworn.submission-permission/v1",
		InvocationID:     request.InvocationID,
		RequestDigest:    Digest(requestBody),
		Role:             request.Role,
		OperationID:      request.Operation.ID,
		ProviderKey:      selected.Provider.Key,
		DriverID:         selected.Provider.DriverID,
		DriverVersion:    selected.Provider.DriverVersion,
		ExecutableDigest: selected.Provider.Executable.Digest,
		Network:          selected.Provider.Network,
		Model:            selected.Model,
		WorkspaceAccess:  request.Workspace.Access,
		FreshContext:     request.FreshContext,
		InputsDigest:     Digest(inputBody),
		Containment:      containment,
		Responsibility:   responsibility,
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
	if descriptor.SchemaVersion != "sworn.submission-permission/v1" ||
		!digestPattern.MatchString(descriptor.RequestDigest) ||
		!digestPattern.MatchString(descriptor.InputsDigest) ||
		validateIdentity(descriptor.InvocationID) != nil ||
		!providerKeyPattern.MatchString(descriptor.ProviderKey) ||
		!driverIdentityPattern.MatchString(descriptor.DriverID) ||
		!versionPattern.MatchString(descriptor.DriverVersion) ||
		!digestPattern.MatchString(descriptor.ExecutableDigest) ||
		(descriptor.Network != NetworkNone && descriptor.Network != NetworkRequired) ||
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
	case CaptainReview:
		if descriptor.Role != RoleCaptain {
			return fail("INVALID_PERMISSION")
		}
	case WorkVerification, AssemblyVerification:
		if descriptor.Role != RoleVerifier {
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
	if submission.InvocationID != permission.descriptor.InvocationID {
		return fail("SUBMISSION_BINDING_MISMATCH")
	}
	kinds := make([]ArtifactKind, len(submission.Artifacts))
	for index, artifact := range submission.Artifacts {
		kinds[index] = artifact.Kind
	}
	require := func(expected []ArtifactKind, decisions ...DecisionOutcome) error {
		if len(kinds) != len(expected) {
			return fail("SUBMISSION_SHAPE_MISMATCH")
		}
		for index := range expected {
			if kinds[index] != expected[index] {
				return fail("SUBMISSION_SHAPE_MISMATCH")
			}
		}
		if len(decisions) == 0 {
			if submission.Decision != nil {
				return fail("SUBMISSION_SHAPE_MISMATCH")
			}
			return nil
		}
		if submission.Decision == nil {
			return fail("SUBMISSION_SHAPE_MISMATCH")
		}
		for _, allowed := range decisions {
			if submission.Decision.Outcome == allowed {
				return nil
			}
		}
		return fail("SUBMISSION_SHAPE_MISMATCH")
	}
	switch permission.descriptor.Responsibility {
	case PlannerProposal:
		return require([]ArtifactKind{ArtifactPlan})
	case ImplementerDesign:
		return require([]ArtifactKind{ArtifactDesign, ArtifactWorkStatus})
	case ImplementerImplementation:
		return require([]ArtifactKind{ArtifactWorkProof, ArtifactWorkStatus})
	case CaptainReview:
		return require([]ArtifactKind{ArtifactWorkStatus},
			DecisionProceed, DecisionRevise, DecisionEscalate)
	case WorkVerification:
		return require([]ArtifactKind{ArtifactWorkStatus},
			DecisionPass, DecisionFail, DecisionBlocked)
	case AssemblyVerification:
		return require([]ArtifactKind{ArtifactAssemblyStatus},
			DecisionPass, DecisionFail, DecisionBlocked)
	default:
		return fail("INVALID_PERMISSION")
	}
}

type Seal struct {
	SchemaVersion    string `json:"schema_version"`
	InvocationID     string `json:"invocation_id"`
	SubmissionDigest string `json:"submission_digest"`
	Accepted         bool   `json:"accepted"`
	Code             string `json:"code"`
}

type SubmissionServer struct {
	mu         sync.Mutex
	permission SubmissionPermission
	sealed     bool
	body       []byte
	seal       Seal
	sealBytes  []byte
}

func NewSubmissionServer(permission SubmissionPermission) (*SubmissionServer, error) {
	if _, err := permission.Describe(); err != nil {
		return nil, err
	}
	return &SubmissionServer{permission: permission}, nil
}

func (server *SubmissionServer) Describe() (PermissionDescriptor, error) {
	return server.permission.Describe()
}

func (server *SubmissionServer) Submit(body []byte) (Seal, []byte, error) {
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
		return Seal{}, nil, fail("SUBMISSION_CONFLICT")
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

func (server *SubmissionServer) Accepted() ([]byte, Seal, []byte, bool) {
	server.mu.Lock()
	defer server.mu.Unlock()
	if !server.sealed || !server.seal.Accepted {
		return nil, Seal{}, nil, false
	}
	return append([]byte(nil), server.body...),
		server.seal,
		append([]byte(nil), server.sealBytes...),
		true
}

type SubmissionClient interface {
	Describe() (PermissionDescriptor, error)
	Submit([]byte) (Seal, []byte, error)
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
	if _, err := writer.Write(frame); err != nil {
		return fail("FRAME_WRITE_FAILED")
	}
	return nil
}
