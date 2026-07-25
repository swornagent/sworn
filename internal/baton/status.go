package baton

import (
	"encoding/json"
	"fmt"
	"reflect"
)

const (
	StatusSchema  = "https://baton.sawy3r.net/schemas/work-status-v1.json"
	StatusVersion = "baton.work-status/v1"
)

type ApprovalBinding struct {
	Ref    string `json:"ref"`
	Digest string `json:"digest"`
}
type PlanBinding struct {
	Digest   string          `json:"digest"`
	Approval ApprovalBinding `json:"approval"`
}
type MaterializationDependency struct {
	TrackID    string `json:"track_id"`
	FrozenHead string `json:"frozen_head"`
}
type Materialization struct {
	BaseCommit   string                      `json:"base_commit"`
	Dependencies []MaterializationDependency `json:"dependencies"`
}
type Blocker struct {
	Code    string `json:"code"`
	Summary string `json:"summary"`
}
type DesignBinding struct {
	Digest             string `json:"digest"`
	ProducerInvocation string `json:"producer_invocation"`
}
type CaptainBinding struct {
	Outcome      string `json:"outcome"`
	Invocation   string `json:"invocation"`
	PlanDigest   string `json:"plan_digest"`
	DesignDigest string `json:"design_digest"`
}
type ProofComponent struct {
	TrackID string `json:"track_id"`
	Head    string `json:"head"`
}
type ProofBinding struct {
	Digest             string           `json:"digest"`
	ProducerInvocation string           `json:"producer_invocation"`
	Repository         string           `json:"repository"`
	BaseCommit         string           `json:"base_commit"`
	CandidateCommit    string           `json:"candidate_commit"`
	CandidateTree      string           `json:"candidate_tree"`
	ProductTree        string           `json:"product_tree"`
	PlanDigest         string           `json:"plan_digest"`
	ApprovalDigest     string           `json:"approval_digest"`
	DesignDigest       *string          `json:"design_digest,omitempty"`
	CaptainInvocation  *string          `json:"captain_invocation,omitempty"`
	Components         []ProofComponent `json:"components"`
}
type VerificationBinding struct {
	Outcome           string `json:"outcome"`
	Invocation        string `json:"invocation"`
	AttestationRef    string `json:"attestation_ref"`
	AttestationDigest string `json:"attestation_digest"`
	PlanDigest        string `json:"plan_digest"`
	ProofDigest       string `json:"proof_digest"`
	CandidateCommit   string `json:"candidate_commit"`
	ProductTree       string `json:"product_tree"`
}
type MergeBinding struct {
	Scope                         string  `json:"scope"`
	PassedCandidate               string  `json:"passed_candidate"`
	FrozenTrackHead               *string `json:"frozen_track_head,omitempty"`
	ExpectedTarget                string  `json:"expected_target"`
	Outcome                       string  `json:"outcome"`
	ObservedTarget                string  `json:"observed_target"`
	ResultCommit                  string  `json:"result_commit"`
	PlanDigest                    string  `json:"plan_digest"`
	VerificationAttestationDigest string  `json:"verification_attestation_digest"`
}
type StatusView struct {
	Schema          string               `json:"$schema"`
	SchemaVersion   string               `json:"schema_version"`
	Kind            string               `json:"kind"`
	Release         string               `json:"release"`
	WorkID          *string              `json:"work_id,omitempty"`
	TrackID         *string              `json:"track_id,omitempty"`
	OwnerRef        string               `json:"owner_ref"`
	AuthorityRef    string               `json:"authority_ref"`
	TargetRef       string               `json:"target_ref"`
	Plan            PlanBinding          `json:"plan"`
	Stage           string               `json:"stage"`
	Status          string               `json:"status"`
	NextRole        string               `json:"next_role"`
	Outcome         string               `json:"outcome"`
	Materialization *Materialization     `json:"materialization,omitempty"`
	Blocker         *Blocker             `json:"blocker,omitempty"`
	Design          *DesignBinding       `json:"design,omitempty"`
	Captain         *CaptainBinding      `json:"captain,omitempty"`
	Proof           *ProofBinding        `json:"proof,omitempty"`
	Verification    *VerificationBinding `json:"verification,omitempty"`
	Merge           *MergeBinding        `json:"merge,omitempty"`
}
type statusAdmission struct {
	raw  []byte
	view StatusView
}
type Status struct {
	admission *statusAdmission
}
type StatusExpectation struct {
	PlanDigest     string
	ApprovalRef    string
	ApprovalDigest string
	DesignDigest   string
	ProofDigest    string
}

func ParseStatus(raw []byte, expectations ...StatusExpectation) (Status, error) {
	value, err := strictParseJSON(raw, "status.json", MaxStatusBytes)
	if err != nil {
		return Status{}, err
	}
	object, err := asObject(value, "status")
	if err != nil {
		return Status{}, err
	}
	if err := validateStatusShape(object); err != nil {
		return Status{}, err
	}
	normalized, err := json.Marshal(object)
	if err != nil {
		return Status{}, recordWrap("INVALID_JSON", "normalize status", err)
	}
	var view StatusView
	if err := json.Unmarshal(normalized, &view); err != nil {
		return Status{}, recordWrap("INVALID_JSON", "decode status", err)
	}
	if err := validateStatusSemantics(view); err != nil {
		return Status{}, err
	}
	for _, expected := range expectations {
		if expected.PlanDigest != "" && view.Plan.Digest != expected.PlanDigest {
			return Status{}, recordFail("STALE_BINDING", "status does not bind the expected plan")
		}
		if expected.ApprovalRef != "" && view.Plan.Approval.Ref != expected.ApprovalRef {
			return Status{}, recordFail("STALE_BINDING", "status does not bind the expected approval reference")
		}
		if expected.ApprovalDigest != "" && view.Plan.Approval.Digest != expected.ApprovalDigest {
			return Status{}, recordFail("STALE_BINDING", "status does not bind the expected approval digest")
		}
		if expected.DesignDigest != "" && (view.Design == nil || view.Design.Digest != expected.DesignDigest) {
			return Status{}, recordFail("STALE_BINDING", "status does not bind the expected design")
		}
		if expected.ProofDigest != "" && (view.Proof == nil || view.Proof.Digest != expected.ProofDigest) {
			return Status{}, recordFail("STALE_BINDING", "status does not bind the expected proof")
		}
	}
	return Status{admission: &statusAdmission{raw: append([]byte(nil), raw...), view: copyStatusView(view)}}, nil
}
func (s Status) require() (*statusAdmission, error) {
	if s.admission == nil {
		return nil, recordFail("STATUS_ADMISSION_REQUIRED", "operation requires a strictly parsed status")
	}
	if _, err := ParseStatus(s.admission.raw); err != nil {
		return nil, recordFail("STATUS_ADMISSION_REQUIRED", "status admission is no longer exact")
	}
	return s.admission, nil
}
func (s Status) Bytes() []byte {
	if s.admission == nil {
		return nil
	}
	return append([]byte(nil), s.admission.raw...)
}
func (s Status) View() StatusView {
	if s.admission == nil {
		return StatusView{}
	}
	return copyStatusView(s.admission.view)
}
func (s Status) Projection() string {
	if s.admission == nil {
		return ""
	}
	return projection(s.admission.view)
}
func copyStatusView(value StatusView) StatusView {
	raw, _ := json.Marshal(value)
	var result StatusView
	_ = json.Unmarshal(raw, &result)
	return result
}
func encodeStatus(value StatusView, expectations ...StatusExpectation) (Status, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return Status{}, recordWrap("INVALID_JSON", "encode status", err)
	}
	raw = append(raw, '\n')
	return ParseStatus(raw, expectations...)
}
func validateStatusShape(value map[string]any) error {
	required := []string{"$schema", "schema_version", "kind", "release", "owner_ref", "authority_ref", "target_ref", "plan", "stage", "status", "next_role", "outcome"}
	optional := []string{"work_id", "track_id", "materialization", "blocker", "design", "captain", "proof", "verification", "merge"}
	if err := exactKeys(value, required, optional, "status"); err != nil {
		return err
	}
	if err := validateExactNested(value["plan"], []string{"digest", "approval"}, nil, "status.plan"); err != nil {
		return err
	}
	plan, _ := asObject(value["plan"], "status.plan")
	if err := validateExactNested(plan["approval"], []string{"ref", "digest"}, nil, "status.plan.approval"); err != nil {
		return err
	}
	if raw, ok := value["materialization"]; ok {
		if err := validateExactNested(raw, []string{"base_commit", "dependencies"}, nil, "status.materialization"); err != nil {
			return err
		}
		materialization, _ := asObject(raw, "status.materialization")
		dependencies, err := asArray(materialization["dependencies"], "status.materialization.dependencies", false, MaxTracks)
		if err != nil {
			return err
		}
		for index, dependency := range dependencies {
			if err := validateExactNested(dependency, []string{"track_id", "frozen_head"}, nil, fmt.Sprintf("status.materialization.dependencies[%d]", index)); err != nil {
				return err
			}
		}
	}
	if raw, ok := value["blocker"]; ok {
		if err := validateExactNested(raw, []string{"code", "summary"}, nil, "status.blocker"); err != nil {
			return err
		}
	}
	if raw, ok := value["design"]; ok {
		if err := validateExactNested(raw, []string{"digest", "producer_invocation"}, nil, "status.design"); err != nil {
			return err
		}
	}
	if raw, ok := value["captain"]; ok {
		if err := validateExactNested(raw, []string{"outcome", "invocation", "plan_digest", "design_digest"}, nil, "status.captain"); err != nil {
			return err
		}
	}
	if raw, ok := value["proof"]; ok {
		required := []string{"digest", "producer_invocation", "repository", "base_commit", "candidate_commit", "candidate_tree", "product_tree", "plan_digest", "approval_digest", "components"}
		if err := validateExactNested(raw, required, []string{"design_digest", "captain_invocation"}, "status.proof"); err != nil {
			return err
		}
		proof, _ := asObject(raw, "status.proof")
		components, err := asArray(proof["components"], "status.proof.components", false, MaxProofComponents)
		if err != nil {
			return err
		}
		for index, component := range components {
			if err := validateExactNested(component, []string{"track_id", "head"}, nil, fmt.Sprintf("status.proof.components[%d]", index)); err != nil {
				return err
			}
		}
	}
	if raw, ok := value["verification"]; ok {
		required := []string{"outcome", "invocation", "attestation_ref", "attestation_digest", "plan_digest", "proof_digest", "candidate_commit", "product_tree"}
		if err := validateExactNested(raw, required, nil, "status.verification"); err != nil {
			return err
		}
	}
	if raw, ok := value["merge"]; ok {
		required := []string{"scope", "passed_candidate", "expected_target", "outcome", "observed_target", "result_commit", "plan_digest", "verification_attestation_digest"}
		if err := validateExactNested(raw, required, []string{"frozen_track_head"}, "status.merge"); err != nil {
			return err
		}
	}
	return nil
}
func validateExactNested(value any, required, optional []string, label string) error {
	object, err := asObject(value, label)
	if err != nil {
		return err
	}
	return exactKeys(object, required, optional, label)
}
func validateStatusSemantics(value StatusView) error {
	if value.Schema != StatusSchema {
		return recordFail("INVALID_SCHEMA", "status.$schema must be "+StatusSchema)
	}
	if value.SchemaVersion != StatusVersion {
		return recordFail("INVALID_VERSION", "status.schema_version must be "+StatusVersion)
	}
	if value.Kind != "work" && value.Kind != "assembly" {
		return recordFail("INVALID_FIELD", "status.kind is invalid")
	}
	if !identityPattern.MatchString(value.Release) {
		return recordFail("INVALID_FIELD", "status.release has an invalid value")
	}
	for label, ref := range map[string]string{"owner_ref": value.OwnerRef, "authority_ref": value.AuthorityRef, "target_ref": value.TargetRef} {
		if _, err := headRef(ref, "status."+label); err != nil {
			return err
		}
	}
	if !digestPattern.MatchString(value.Plan.Digest) || !digestPattern.MatchString(value.Plan.Approval.Digest) ||
		value.Plan.Approval.Ref == "" || containsControl(value.Plan.Approval.Ref) {
		return recordFail("INVALID_FIELD", "status.plan is invalid")
	}
	if !oneOf(value.Stage, "plan", "design", "implement", "verify", "merge") ||
		!oneOf(value.Status, "ready", "blocked", "complete") ||
		!oneOf(value.NextRole, "planner", "implementer", "captain", "verifier", "merge", "none") ||
		!oneOf(value.Outcome, "none", "proceed", "revise", "escalate", "pass", "fail", "blocked", "merged") {
		return recordFail("INVALID_FIELD", "status projection fields are invalid")
	}
	if value.Status == "blocked" {
		if value.Blocker == nil || !blockerCodePattern.MatchString(value.Blocker.Code) ||
			value.Blocker.Summary == "" || len([]rune(value.Blocker.Summary)) > 1000 {
			return recordFail("INVALID_STATE_BINDING", "blocked status requires a valid blocker")
		}
	} else if value.Blocker != nil {
		return recordFail("INVALID_STATE_BINDING", "only blocked status may contain blocker")
	}
	if err := validateOptionalBindings(value); err != nil {
		return err
	}
	releaseRef := "refs/heads/release-wt/" + value.Release
	if value.Kind == "work" {
		if value.WorkID == nil || value.TrackID == nil || !identityPattern.MatchString(*value.WorkID) || !identityPattern.MatchString(*value.TrackID) {
			return recordFail("INVALID_STATE_BINDING", "work status requires work_id and track_id")
		}
		trackRef := "refs/heads/track/" + value.Release + "/" + *value.TrackID
		if value.OwnerRef != trackRef {
			return recordFail("INVALID_OWNER", "work owner_ref must be "+trackRef)
		}
		if value.AuthorityRef != releaseRef && value.AuthorityRef != trackRef {
			return recordFail("INVALID_OWNER", "work authority_ref must be its release baseline or owning track")
		}
		if value.AuthorityRef == trackRef {
			if value.Materialization == nil {
				return recordFail("INVALID_STATE_BINDING", "materialised work requires materialization")
			}
		} else if value.Stage == "merge" && value.Status == "complete" {
			if value.Materialization == nil {
				return recordFail("INVALID_STATE_BINDING", "completed work requires materialization")
			}
		} else if value.Materialization != nil {
			return recordFail("INVALID_STATE_BINDING", "release baseline work cannot claim materialization")
		}
		if value.Proof != nil {
			if value.Proof.DesignDigest == nil || value.Proof.CaptainInvocation == nil || len(value.Proof.Components) != 0 {
				return recordFail("INVALID_STATE_BINDING", "work proof requires work bindings and no components")
			}
		}
		if value.Merge != nil && (value.Merge.Scope != "track" || value.Merge.FrozenTrackHead == nil) {
			return recordFail("INVALID_STATE_BINDING", "work Merge must bind a frozen track")
		}
	} else {
		if value.WorkID != nil || value.TrackID != nil || value.Materialization != nil || value.Design != nil || value.Captain != nil {
			return recordFail("INVALID_STATE_BINDING", "assembly status contains work-only fields")
		}
		if value.OwnerRef != releaseRef || value.AuthorityRef != releaseRef {
			return recordFail("INVALID_OWNER", "assembly owner and authority refs must be "+releaseRef)
		}
		if value.Proof == nil || len(value.Proof.Components) == 0 ||
			value.Proof.DesignDigest != nil || value.Proof.CaptainInvocation != nil {
			return recordFail("INVALID_STATE_BINDING", "assembly proof requires components and no work bindings")
		}
		if value.Merge != nil && (value.Merge.Scope != "release" || value.Merge.FrozenTrackHead != nil) {
			return recordFail("INVALID_STATE_BINDING", "assembly Merge scope must be release")
		}
		if value.Stage != "verify" && value.Stage != "merge" {
			return recordFail("INVALID_STATE_BINDING", "assembly status begins at verify")
		}
	}
	if value.Captain != nil {
		if value.Design == nil || value.Captain.PlanDigest != value.Plan.Digest ||
			value.Captain.DesignDigest != value.Design.Digest {
			return recordFail("STALE_BINDING", "Captain binds stale plan or design")
		}
		if value.Captain.Invocation == value.Design.ProducerInvocation {
			return recordFail("SELF_REVIEW", "Captain invocation equals design producer")
		}
	}
	if value.Proof != nil {
		if value.Proof.PlanDigest != value.Plan.Digest || value.Proof.ApprovalDigest != value.Plan.Approval.Digest {
			return recordFail("STALE_BINDING", "proof binds stale plan or approval")
		}
		if value.Kind == "work" {
			if value.Design == nil || value.Captain == nil || value.Captain.Outcome != "proceed" {
				return recordFail("MISSING_PROCEED", "implementation requires Captain PROCEED")
			}
			if value.Proof.DesignDigest == nil || *value.Proof.DesignDigest != value.Design.Digest ||
				value.Proof.CaptainInvocation == nil || *value.Proof.CaptainInvocation != value.Captain.Invocation {
				return recordFail("STALE_BINDING", "proof binds stale design or Captain")
			}
			if value.Proof.ProducerInvocation == value.Captain.Invocation {
				return recordFail("SELF_REVIEW", "proof producer equals Captain invocation")
			}
		}
	}
	if value.Verification != nil {
		if value.Proof == nil || value.Verification.PlanDigest != value.Plan.Digest ||
			value.Verification.ProofDigest != value.Proof.Digest ||
			value.Verification.CandidateCommit != value.Proof.CandidateCommit ||
			value.Verification.ProductTree != value.Proof.ProductTree {
			return recordFail("STALE_BINDING", "Verifier binds stale proof or candidate")
		}
		forbidden := []string{value.Proof.ProducerInvocation}
		if value.Design != nil {
			forbidden = append(forbidden, value.Design.ProducerInvocation)
		}
		if value.Captain != nil {
			forbidden = append(forbidden, value.Captain.Invocation)
		}
		for _, invocation := range forbidden {
			if value.Verification.Invocation == invocation {
				return recordFail("SELF_REVIEW", "Verifier invocation is not independent")
			}
		}
	}
	if value.Merge != nil {
		if value.Proof == nil || value.Verification == nil || value.Verification.Outcome != "pass" ||
			value.Merge.PassedCandidate != value.Proof.CandidateCommit ||
			value.Merge.PlanDigest != value.Plan.Digest ||
			value.Merge.VerificationAttestationDigest != value.Verification.AttestationDigest {
			return recordFail("UNVERIFIED_MERGE", "Merge requires exact PASS bindings")
		}
	}
	return validateProjection(value)
}
func validateOptionalBindings(value StatusView) error {
	if value.Materialization != nil {
		if !objectIDPattern.MatchString(value.Materialization.BaseCommit) || len(value.Materialization.Dependencies) > MaxTracks {
			return recordFail("INVALID_FIELD", "status.materialization is invalid")
		}
		seen := make(map[string]struct{})
		for _, dependency := range value.Materialization.Dependencies {
			if !identityPattern.MatchString(dependency.TrackID) || !objectIDPattern.MatchString(dependency.FrozenHead) {
				return recordFail("INVALID_FIELD", "status.materialization dependency is invalid")
			}
			if _, duplicate := seen[dependency.TrackID]; duplicate {
				return recordFail("DUPLICATE_IDENTITY", "status.materialization repeats dependency "+dependency.TrackID)
			}
			seen[dependency.TrackID] = struct{}{}
		}
	}
	if value.Design != nil && (!digestPattern.MatchString(value.Design.Digest) || !invocationPattern.MatchString(value.Design.ProducerInvocation)) {
		return recordFail("INVALID_FIELD", "status.design is invalid")
	}
	if value.Captain != nil && (!oneOf(value.Captain.Outcome, "proceed", "revise", "escalate") ||
		!invocationPattern.MatchString(value.Captain.Invocation) || !digestPattern.MatchString(value.Captain.PlanDigest) ||
		!digestPattern.MatchString(value.Captain.DesignDigest)) {
		return recordFail("INVALID_FIELD", "status.captain is invalid")
	}
	if value.Proof != nil {
		if !digestPattern.MatchString(value.Proof.Digest) || !invocationPattern.MatchString(value.Proof.ProducerInvocation) ||
			value.Proof.Repository == "" || containsControl(value.Proof.Repository) ||
			!objectIDPattern.MatchString(value.Proof.BaseCommit) || !objectIDPattern.MatchString(value.Proof.CandidateCommit) ||
			!objectIDPattern.MatchString(value.Proof.CandidateTree) || !digestPattern.MatchString(value.Proof.ProductTree) ||
			!digestPattern.MatchString(value.Proof.PlanDigest) || !digestPattern.MatchString(value.Proof.ApprovalDigest) ||
			len(value.Proof.Components) > MaxProofComponents {
			return recordFail("INVALID_FIELD", "status.proof is invalid")
		}
		if value.Proof.DesignDigest != nil && !digestPattern.MatchString(*value.Proof.DesignDigest) {
			return recordFail("INVALID_FIELD", "status.proof.design_digest is invalid")
		}
		if value.Proof.CaptainInvocation != nil && !invocationPattern.MatchString(*value.Proof.CaptainInvocation) {
			return recordFail("INVALID_FIELD", "status.proof.captain_invocation is invalid")
		}
		seen := make(map[string]struct{})
		for _, component := range value.Proof.Components {
			if !identityPattern.MatchString(component.TrackID) || !objectIDPattern.MatchString(component.Head) {
				return recordFail("INVALID_FIELD", "status.proof component is invalid")
			}
			if _, duplicate := seen[component.TrackID]; duplicate {
				return recordFail("DUPLICATE_IDENTITY", "status.proof repeats component "+component.TrackID)
			}
			seen[component.TrackID] = struct{}{}
		}
	}
	if value.Verification != nil && (!oneOf(value.Verification.Outcome, "pass", "fail", "blocked") ||
		!invocationPattern.MatchString(value.Verification.Invocation) || value.Verification.AttestationRef == "" ||
		containsControl(value.Verification.AttestationRef) || !digestPattern.MatchString(value.Verification.AttestationDigest) ||
		!digestPattern.MatchString(value.Verification.PlanDigest) || !digestPattern.MatchString(value.Verification.ProofDigest) ||
		!objectIDPattern.MatchString(value.Verification.CandidateCommit) || !digestPattern.MatchString(value.Verification.ProductTree)) {
		return recordFail("INVALID_FIELD", "status.verification is invalid")
	}
	if value.Merge != nil {
		if !oneOf(value.Merge.Scope, "track", "release") || !objectIDPattern.MatchString(value.Merge.PassedCandidate) ||
			(value.Merge.FrozenTrackHead != nil && !objectIDPattern.MatchString(*value.Merge.FrozenTrackHead)) ||
			!objectIDPattern.MatchString(value.Merge.ExpectedTarget) || value.Merge.Outcome != "merged" ||
			!objectIDPattern.MatchString(value.Merge.ObservedTarget) || !objectIDPattern.MatchString(value.Merge.ResultCommit) ||
			!digestPattern.MatchString(value.Merge.PlanDigest) || !digestPattern.MatchString(value.Merge.VerificationAttestationDigest) {
			return recordFail("INVALID_FIELD", "status.merge is invalid")
		}
		if value.Merge.ObservedTarget != value.Merge.ExpectedTarget {
			return recordFail("MOVED_TARGET", "Merge observed target must equal its expected target")
		}
	}
	return nil
}
func validateProjection(value StatusView) error {
	state := projection(value)
	allowed := map[string]bool{
		"verify/ready/verifier":  true,
		"verify/blocked/planner": true,
		"merge/ready/merge":      true,
		"merge/complete/none":    true,
		"plan/blocked/planner":   true,
	}
	if value.Kind == "work" {
		allowed["design/ready/implementer"] = true
		allowed["design/ready/captain"] = true
		allowed["design/blocked/planner"] = true
		allowed["implement/ready/implementer"] = true
	} else {
		allowed["verify/ready/planner"] = true
	}
	if !allowed[state] {
		return recordFail("INVALID_PROJECTION", "invalid durable projection "+state)
	}
	switch state {
	case "design/ready/implementer":
		if value.Proof != nil || value.Verification != nil || value.Merge != nil {
			return recordFail("INVALID_STATE_BINDING", state+" retains later evidence")
		}
		if value.Outcome == "none" {
			if value.Design != nil || value.Captain != nil {
				return recordFail("INVALID_STATE_BINDING", "initial design state contains gates")
			}
		} else if value.Outcome == "revise" {
			if value.Design == nil || value.Captain == nil || value.Captain.Outcome != "revise" {
				return recordFail("INVALID_STATE_BINDING", "revision requires Captain REVISE")
			}
		} else {
			return recordFail("INVALID_STATE_BINDING", state+" outcome must be none or revise")
		}
	case "design/ready/captain":
		if value.Design == nil || value.Captain != nil || value.Proof != nil || value.Verification != nil ||
			value.Merge != nil || value.Outcome != "none" {
			return recordFail("INVALID_STATE_BINDING", state+" has invalid gates")
		}
	case "design/blocked/planner":
		if value.Design == nil || value.Captain == nil || value.Blocker == nil ||
			value.Captain.Outcome != "escalate" || value.Outcome != "escalate" ||
			value.Proof != nil || value.Verification != nil || value.Merge != nil {
			return recordFail("INVALID_STATE_BINDING", state+" requires Captain ESCALATE")
		}
	case "implement/ready/implementer":
		if value.Design == nil || value.Captain == nil || value.Captain.Outcome != "proceed" || value.Merge != nil {
			return recordFail("INVALID_STATE_BINDING", state+" requires PROCEED")
		}
		if value.Outcome == "proceed" {
			if value.Proof != nil || value.Verification != nil {
				return recordFail("INVALID_STATE_BINDING", "first implementation contains stale evidence")
			}
		} else if value.Outcome == "fail" {
			if value.Proof == nil || value.Verification == nil || value.Verification.Outcome != "fail" {
				return recordFail("INVALID_STATE_BINDING", "repair requires Verifier FAIL")
			}
		} else {
			return recordFail("INVALID_STATE_BINDING", state+" outcome must be proceed or fail")
		}
	case "verify/ready/verifier":
		if value.Proof == nil || value.Verification != nil || value.Merge != nil || value.Outcome != "none" {
			return recordFail("INVALID_STATE_BINDING", state+" has invalid evidence")
		}
	case "verify/ready/planner":
		if value.Kind != "assembly" || value.Proof == nil || value.Verification == nil ||
			value.Verification.Outcome != "fail" || value.Outcome != "fail" || value.Merge != nil || value.Blocker != nil {
			return recordFail("INVALID_STATE_BINDING", state+" requires assembly FAIL")
		}
	case "verify/blocked/planner":
		if value.Proof == nil || value.Verification == nil || value.Blocker == nil ||
			value.Verification.Outcome != "blocked" || value.Outcome != "blocked" || value.Merge != nil {
			return recordFail("INVALID_STATE_BINDING", state+" requires Verifier BLOCKED")
		}
	case "merge/ready/merge":
		if value.Proof == nil || value.Verification == nil || value.Verification.Outcome != "pass" ||
			value.Outcome != "pass" || value.Merge != nil {
			return recordFail("INVALID_STATE_BINDING", state+" requires Verifier PASS")
		}
	case "merge/complete/none":
		releaseRef := "refs/heads/release-wt/" + value.Release
		if value.Proof == nil || value.Verification == nil || value.Merge == nil || value.Status != "complete" ||
			value.Outcome != "merged" || (value.Kind == "work" && value.AuthorityRef != releaseRef) {
			return recordFail("INVALID_STATE_BINDING", state+" requires complete MERGED")
		}
	case "plan/blocked/planner":
		if value.Blocker == nil || value.Outcome != "blocked" || value.Design != nil || value.Captain != nil ||
			value.Proof != nil || value.Verification != nil || value.Merge != nil {
			return recordFail("INVALID_STATE_BINDING", state+" requires a plan blocker only")
		}
	}
	return nil
}
func projection(value StatusView) string {
	return value.Stage + "/" + value.Status + "/" + value.NextRole
}
func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}
func statusesEqual(left, right Status) bool {
	if left.admission == nil || right.admission == nil {
		return left.admission == right.admission
	}
	return reflect.DeepEqual(left.admission.view, right.admission.view)
}
