package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"

	"github.com/swornagent/sworn/internal/repo"
)

const VerifierReviewCheckSchemaVersion = "sworn-verifier-review-check-v1"

// VerifierReviewInput is one deterministic, content-addressed file supplied to
// the verifier alongside the canonical plan, submission, and dispatch.
type VerifierReviewInput struct {
	Name     string
	Digest   string
	Contents []byte
}

// ExactVerifierReview is an immutable, fully resolved review-input closure.
// Its files are engine-derived; callers receive defensive copies only.
type ExactVerifierReview struct {
	inputs []VerifierReviewInput
}

func (review ExactVerifierReview) Inputs() []VerifierReviewInput {
	inputs := make([]VerifierReviewInput, len(review.inputs))
	for index, input := range review.inputs {
		inputs[index] = input
		inputs[index].Contents = slices.Clone(input.Contents)
	}
	return inputs
}

type verifierReviewJSONArtifact struct {
	Pointer  Artifact        `json:"pointer"`
	Contents json.RawMessage `json:"contents"`
}

type verifierReviewCapture struct {
	Artifact CapturedArtifact `json:"artifact"`
	Encoding string           `json:"encoding"`
	Contents string           `json:"contents"`
}

type verifierReviewCheck struct {
	SchemaVersion string                     `json:"schema_version"`
	CheckID       string                     `json:"check_id"`
	Definition    verifierReviewJSONArtifact `json:"definition"`
	Receipt       verifierReviewJSONArtifact `json:"receipt"`
	Environment   verifierReviewJSONArtifact `json:"environment"`
	Stdout        verifierReviewCapture      `json:"stdout"`
	Stderr        verifierReviewCapture      `json:"stderr"`
}

// ResolveExactVerifierReview closes an already parsed plan and submission over
// every content-addressed policy, definition, authority, check, environment,
// and captured-output artifact on which the review relies. It returns only a
// defensive, deterministic input manifest; successful resolution is the
// capability.
//
// The verifier still receives the canonical plan and submission as its review
// documents. Resolving the transitive closure here prevents a compact model
// prompt from silently weakening the evidence that those documents name.
func ResolveExactVerifierReview(
	ctx context.Context,
	artifacts ArtifactReader,
	plan ExactPlan,
	submission ExactSubmission,
) (ExactVerifierReview, error) {
	if artifacts == nil {
		return ExactVerifierReview{}, errors.New("exact verifier review requires an artifact reader")
	}
	planRecord, submissionRecord := plan.Record(), submission.Record()
	if planRecord.Kind != DeliveryPlanSchemaVersion || !ValidDigest(planRecord.Digest) ||
		submissionRecord.Kind != SubmissionSchemaVersion || !submission.valid() {
		return ExactVerifierReview{}, errors.New("exact verifier review requires exact plan and submission capabilities")
	}
	view := submission.View()
	contract, exists := plan.Work(view.WorkID)
	if !exists || view.PlanDigest != planRecord.Digest || view.DeliveryID != plan.DeliveryID() ||
		view.ContractDigest != contract.Digest() {
		return ExactVerifierReview{}, errors.New("verifier submission does not match its exact plan and work contract")
	}
	wantSubmissionID, err := SubmissionID(view.DeliveryID, view.WorkID, view.Attempt)
	if err != nil || view.SubmissionID != wantSubmissionID {
		return ExactVerifierReview{}, errors.New("verifier submission does not match its exact work attempt")
	}
	target, policy, work := plan.Target(), plan.Policy(), contract.View()
	if view.Base.Repository != target.Repository || view.Base.Ref != target.Ref ||
		view.Candidate.Repository != target.Repository || view.Assurance.PolicyRef != policy.Ref ||
		view.Assurance.PolicyDigest != policy.Digest || view.Assurance.Profile != work.Assurance.Profile ||
		!slices.Equal(view.Assurance.Packs, work.Assurance.Packs) {
		return ExactVerifierReview{}, errors.New("verifier submission does not match its exact target and assurance policy")
	}

	resolver := artifactResolver{reader: artifacts, cached: make(map[string]resolvedArtifact)}
	checks, err := resolveExactLocalChecks(ctx, &resolver, plan, view.WorkID)
	if err != nil {
		return ExactVerifierReview{}, fmt.Errorf("resolve verifier policy closure: %w", err)
	}
	policyBytes, err := resolver.resolve(ctx, jsonCAS(policy.Digest), MaximumAssurancePolicyBytes)
	if err != nil {
		return ExactVerifierReview{}, fmt.Errorf("resolve verifier policy input: %w", err)
	}
	authorityBytes, err := resolver.resolve(ctx, view.AuthorityReceipt, MaximumControlReceiptBytes)
	if err != nil {
		return ExactVerifierReview{}, fmt.Errorf("resolve verifier authority receipt: %w", err)
	}
	if view.AuthorityReceipt.MediaType != "application/json" {
		return ExactVerifierReview{}, errors.New("verifier authority receipt must be application/json")
	}
	if err := ValidateAuthorityApprovalForBuilder(authorityBytes, plan, view.Builder); err != nil {
		return ExactVerifierReview{}, fmt.Errorf("validate verifier authority receipt: %w", err)
	}

	requirements := checks.Requirements()
	if len(view.Checks) != len(requirements) || len(view.Evidence) != len(requirements) {
		return ExactVerifierReview{}, errors.New("verifier submission does not exactly cover its policy checks and evidence")
	}
	snapshotDigest, err := SnapshotDigest()
	if err != nil {
		return ExactVerifierReview{}, fmt.Errorf("measure embedded protocol snapshot: %w", err)
	}
	inputs := []VerifierReviewInput{
		verifierReviewInput("review-policy", policyBytes),
		verifierReviewInput("review-authority", authorityBytes),
	}
	evidenceByID := make(map[string]Evidence, len(view.Evidence))
	for _, evidence := range view.Evidence {
		evidenceByID[evidence.ID] = evidence
	}
	candidate := repo.Candidate{
		RepositoryID: view.Candidate.Repository,
		Commit:       view.Candidate.Commit,
		Tree:         view.Candidate.Tree,
	}
	for index, requirement := range requirements {
		declared := view.Checks[index]
		if declared.ID != requirement.CheckID {
			return ExactVerifierReview{}, fmt.Errorf("verifier check %d does not match exact policy order", index)
		}
		receiptBytes, err := resolver.resolve(ctx, declared.Receipt, MaximumLocalCheckReceiptBytes)
		if err != nil {
			return ExactVerifierReview{}, fmt.Errorf("resolve verifier check %q receipt: %w", declared.ID, err)
		}
		receipt, err := ParseLocalCheckReceipt(receiptBytes)
		if err != nil {
			return ExactVerifierReview{}, fmt.Errorf("parse verifier check %q receipt: %w", declared.ID, err)
		}
		projectedCheck, projectedEvidence, err := projectMeasuredReceipt(
			MeasuredCheck{RunID: declared.RunID, Receipt: declared.Receipt},
			receipt,
			requirement,
			candidate,
		)
		if err != nil {
			return ExactVerifierReview{}, err
		}
		if !reflect.DeepEqual(declared, projectedCheck) {
			return ExactVerifierReview{}, fmt.Errorf("verifier check %q is not the exact measured projection", declared.ID)
		}
		declaredEvidence, exists := evidenceByID[projectedEvidence.ID]
		if !exists || !reflect.DeepEqual(declaredEvidence, projectedEvidence) {
			return ExactVerifierReview{}, fmt.Errorf("verifier evidence %q is not the exact measured projection", projectedEvidence.ID)
		}
		delete(evidenceByID, projectedEvidence.ID)

		captures := make(map[string][]byte, 2)
		for label, capture := range map[string]CapturedArtifact{"stdout": receipt.Stdout, "stderr": receipt.Stderr} {
			contents, err := resolver.resolve(ctx, capture.Pointer(), uint64(capture.Size))
			if err != nil {
				return ExactVerifierReview{}, fmt.Errorf("resolve verifier check %q %s: %w", declared.ID, label, err)
			}
			if int64(len(contents)) != capture.Size {
				return ExactVerifierReview{}, fmt.Errorf("verifier check %q %s size does not match its receipt", declared.ID, label)
			}
			captures[label] = contents
		}
		environmentBytes, err := resolver.resolve(ctx, Artifact{
			Ref: receipt.Environment.Ref, MediaType: LocalEnvironmentMediaType, Digest: receipt.Environment.Ref,
		}, MaximumLocalEnvironmentBytes)
		if err != nil {
			return ExactVerifierReview{}, fmt.Errorf("resolve verifier check %q environment: %w", declared.ID, err)
		}
		environment, err := ParseLocalEnvironment(environmentBytes)
		if err != nil {
			return ExactVerifierReview{}, fmt.Errorf("parse verifier check %q environment: %w", declared.ID, err)
		}
		if environment.ProtocolSnapshotDigest != "sha256:"+snapshotDigest {
			return ExactVerifierReview{}, fmt.Errorf("verifier check %q environment does not bind the protocol snapshot", declared.ID)
		}
		definitionBytes, err := resolver.resolve(ctx, requirement.Definition, MaximumLocalCheckDefinitionBytes)
		if err != nil {
			return ExactVerifierReview{}, fmt.Errorf("resolve verifier check %q definition input: %w", declared.ID, err)
		}
		bundleBytes, err := EncodeCanonical(verifierReviewCheck{
			SchemaVersion: VerifierReviewCheckSchemaVersion,
			CheckID:       declared.ID,
			Definition: verifierReviewJSONArtifact{
				Pointer: requirement.Definition, Contents: json.RawMessage(definitionBytes),
			},
			Receipt: verifierReviewJSONArtifact{
				Pointer: declared.Receipt, Contents: json.RawMessage(receiptBytes),
			},
			Environment: verifierReviewJSONArtifact{
				Pointer: Artifact{
					Ref: receipt.Environment.Ref, MediaType: LocalEnvironmentMediaType, Digest: receipt.Environment.Ref,
				},
				Contents: json.RawMessage(environmentBytes),
			},
			Stdout: verifierReviewCapture{
				Artifact: receipt.Stdout, Encoding: "base64",
				Contents: base64.StdEncoding.EncodeToString(captures["stdout"]),
			},
			Stderr: verifierReviewCapture{
				Artifact: receipt.Stderr, Encoding: "base64",
				Contents: base64.StdEncoding.EncodeToString(captures["stderr"]),
			},
		})
		if err != nil {
			return ExactVerifierReview{}, fmt.Errorf("encode verifier check %q review input: %w", declared.ID, err)
		}
		inputs = append(inputs, verifierReviewInput(fmt.Sprintf("review-check-%02d", index+1), bundleBytes))
	}
	if len(evidenceByID) != 0 {
		return ExactVerifierReview{}, errors.New("verifier submission contains evidence outside its exact policy closure")
	}
	sort.Slice(inputs, func(left, right int) bool { return inputs[left].Name < inputs[right].Name })
	return ExactVerifierReview{inputs: inputs}, nil
}

func verifierReviewInput(name string, contents []byte) VerifierReviewInput {
	return VerifierReviewInput{Name: name, Digest: RawDigest(contents), Contents: slices.Clone(contents)}
}
