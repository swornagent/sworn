package baton

import (
	"bytes"
	"fmt"
	"reflect"
	"sync"
)

type Profile string

const (
	Guided     Profile = "guided"
	Autonomous Profile = "autonomous"
)

type EvidenceRequest struct{ Kind, Ref, Digest, Invocation, PlanDigest, ProofDigest, CandidateCommit, ProductTree string }
type EvidenceProvenance struct {
	Kind, Ref, Decision, PlanDigest, Role, Invocation, ProofDigest, CandidateCommit, ProductTree string
	Protected, AuthorizerIsolated, DeliveryWritable, FreshContext, ReadOnly, EngineControlled    bool
}
type Evidence struct {
	Bytes      []byte
	Provenance EvidenceProvenance
}
type EvidenceResolver func(EvidenceRequest) (Evidence, error)
type evidenceAdmission struct {
	profile      Profile
	statusBytes  []byte
	approval     EvidenceProvenance
	verification *EvidenceProvenance
}

func resolveStatusEvidence(status Status, profile Profile, resolver EvidenceResolver) (*evidenceAdmission, error) {
	admission, err := status.require()
	if err != nil {
		return nil, err
	}
	if profile != Guided && profile != Autonomous {
		return nil, recordFail("INVALID_ADMISSION_PROFILE", "trusted admission profile must be guided or autonomous")
	}
	if resolver == nil {
		return nil, recordFail("EVIDENCE_RESOLVER_REQUIRED", "trusted admission requires an evidence resolver")
	}
	view := admission.view
	approval, err := resolveTrustedEvidence(resolver, EvidenceRequest{
		Kind:       "approval",
		Ref:        view.Plan.Approval.Ref,
		Digest:     view.Plan.Approval.Digest,
		PlanDigest: view.Plan.Digest,
	})
	if err != nil {
		return nil, err
	}
	if approval.Kind != "approval" || approval.Ref != view.Plan.Approval.Ref || !approval.Protected ||
		approval.Decision != "approved" || approval.PlanDigest != view.Plan.Digest ||
		!approval.AuthorizerIsolated || approval.DeliveryWritable {
		return nil, recordFail("UNTRUSTED_EVIDENCE_PROVENANCE", "approval provenance does not establish protected approval")
	}
	result := &evidenceAdmission{
		profile:     profile,
		statusBytes: append([]byte(nil), admission.raw...),
		approval:    approval,
	}
	if view.Verification != nil {
		verification, err := resolveTrustedEvidence(resolver, EvidenceRequest{
			Kind:            "verifier_dispatch",
			Ref:             view.Verification.AttestationRef,
			Digest:          view.Verification.AttestationDigest,
			Invocation:      view.Verification.Invocation,
			PlanDigest:      view.Plan.Digest,
			ProofDigest:     view.Proof.Digest,
			CandidateCommit: view.Proof.CandidateCommit,
			ProductTree:     view.Proof.ProductTree,
		})
		if err != nil {
			return nil, err
		}
		if verification.Kind != "verifier_dispatch" || verification.Ref != view.Verification.AttestationRef ||
			!verification.Protected || verification.Role != "verifier" || !verification.FreshContext ||
			!verification.ReadOnly || verification.Invocation != view.Verification.Invocation ||
			verification.PlanDigest != view.Plan.Digest || verification.ProofDigest != view.Proof.Digest ||
			verification.CandidateCommit != view.Proof.CandidateCommit ||
			verification.ProductTree != view.Proof.ProductTree ||
			(profile == Autonomous && !verification.EngineControlled) {
			return nil, recordFail("UNTRUSTED_EVIDENCE_PROVENANCE", "Verifier dispatch provenance is not exact")
		}
		copyProvenance := verification
		result.verification = &copyProvenance
	}
	return result, nil
}
func resolveTrustedEvidence(resolver EvidenceResolver, request EvidenceRequest) (EvidenceProvenance, error) {
	resolved, err := resolver(request)
	if err != nil {
		return EvidenceProvenance{}, recordWrap("UNRESOLVED_EVIDENCE", "cannot resolve "+request.Kind+" evidence "+request.Ref, err)
	}
	if len(resolved.Bytes) > MaxEvidenceBytes {
		return EvidenceProvenance{}, recordFail("RESOURCE_LIMIT", fmt.Sprintf("%s evidence exceeds maximum size %d bytes", request.Kind, MaxEvidenceBytes))
	}
	if resolved.Provenance.Kind != request.Kind || resolved.Provenance.Ref != request.Ref ||
		DigestBytes(resolved.Bytes) != request.Digest {
		return EvidenceProvenance{}, recordFail("EVIDENCE_BINDING_MISMATCH", request.Kind+" evidence does not match its recorded ref and digest")
	}
	return resolved.Provenance, nil
}
func requireEvidenceAdmission(status Status, admission *evidenceAdmission, profile Profile) error {
	statusAdmission, err := status.require()
	if err != nil {
		return err
	}
	if admission == nil || admission.profile != profile || !bytes.Equal(admission.statusBytes, statusAdmission.raw) {
		return recordFail("EVIDENCE_ADMISSION_REQUIRED", "action requires a matching status evidence admission for the selected profile")
	}
	return nil
}

type evidenceCache struct {
	mu       sync.Mutex
	resolver EvidenceResolver
	values   map[EvidenceRequest]Evidence
}

func newEvidenceCache(resolver EvidenceResolver) *evidenceCache {
	return &evidenceCache{resolver: resolver, values: make(map[EvidenceRequest]Evidence)}
}
func (c *evidenceCache) resolve(request EvidenceRequest) (Evidence, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if value, ok := c.values[request]; ok {
		value.Bytes = append([]byte(nil), value.Bytes...)
		return value, nil
	}
	value, err := c.resolver(request)
	if err != nil {
		return Evidence{}, err
	}
	value.Bytes = append([]byte(nil), value.Bytes...)
	c.values[request] = value
	copyValue := value
	copyValue.Bytes = append([]byte(nil), value.Bytes...)
	return copyValue, nil
}

type InertnessRequest struct{ Repository, RecordRoot, Commit string }
type InertnessDecision struct {
	Repository, RecordRoot, Commit, Decision string
	Consumed                                 bool
}
type InertnessResolver func(InertnessRequest) (InertnessDecision, error)

func resolveInertness(resolver InertnessResolver, request InertnessRequest) error {
	if resolver == nil {
		return recordFail("INERTNESS_RESOLVER_REQUIRED", "product exclusion requires a behavioral-inertness resolver")
	}
	decision, err := resolver(request)
	if err != nil {
		return recordWrap("UNRESOLVED_INERTNESS", "cannot resolve record-root inertness", err)
	}
	if !reflect.DeepEqual(decision, InertnessDecision{
		Repository: request.Repository,
		RecordRoot: request.RecordRoot,
		Commit:     request.Commit,
		Decision:   "inert",
		Consumed:   false,
	}) {
		return recordFail("UNTRUSTED_INERTNESS", "behavioral-inertness decision is not exact")
	}
	return nil
}
