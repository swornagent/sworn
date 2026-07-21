package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

const (
	AssurancePolicySchemaVersion = "assurance-policy-v1"
	MaximumAssurancePolicyBytes  = 1 << 20
	// MaximumExactLocalChecks bounds the initial serial check selection.
	MaximumExactLocalChecks = 64
)

// LocalCheckRequirement is the minimal immutable policy fact needed to create
// one local-check request. Definition is normalized to Sworn's CAS pointer;
// the registry's locator remains covered by the exact policy bytes.
type LocalCheckRequirement struct {
	CheckID    string
	Definition Artifact
	definition LocalCheckDefinition
}

// ExactLocalChecks is an opaque selection capability derived from one exact
// plan, its selected canonical policy, and every resolved baseline definition.
// It proves the narrow initial Standard capability before an external check is
// dispatched.
type ExactLocalChecks struct {
	contract     ExactWorkContract
	requirements []LocalCheckRequirement
}

// Requirements returns the policy-ordered check IDs and CAS definition
// pointers. Callers cannot mutate the exact selection through the returned
// slice.
func (selection ExactLocalChecks) Requirements() []LocalCheckRequirement {
	return slices.Clone(selection.requirements)
}

func (selection ExactLocalChecks) ContractDigest() string { return selection.contract.Digest() }

// ResolveExactLocalChecks derives the complete initial local-check selection
// from immutable plan and artifact truth. It accepts no caller-projected check
// IDs, definitions, acceptance requirements, or assurance facts.
func ResolveExactLocalChecks(
	ctx context.Context,
	artifacts ArtifactReader,
	plan ExactPlan,
	workID string,
) (ExactLocalChecks, error) {
	if artifacts == nil {
		return ExactLocalChecks{}, errors.New("exact local checks require an artifact reader")
	}
	resolver := artifactResolver{reader: artifacts, cached: make(map[string]resolvedArtifact)}
	return resolveExactLocalChecks(ctx, &resolver, plan, workID)
}

func resolveExactLocalChecks(
	ctx context.Context,
	resolver *artifactResolver,
	plan ExactPlan,
	workID string,
) (ExactLocalChecks, error) {
	contract, exists := plan.Work(workID)
	if !exists {
		return ExactLocalChecks{}, fmt.Errorf("work %q is absent from the exact plan", workID)
	}
	work := contract.View()
	if err := validateInitialContract(len(plan.data.work), work); err != nil {
		return ExactLocalChecks{}, err
	}
	policyBytes, err := resolver.resolve(ctx, jsonCAS(plan.Policy().Digest), MaximumAssurancePolicyBytes)
	if err != nil {
		return ExactLocalChecks{}, fmt.Errorf("resolve exact assurance policy: %w", err)
	}
	policy, err := parseAssurancePolicyRegistry(policyBytes)
	if err != nil {
		return ExactLocalChecks{}, err
	}
	requirements := make([]LocalCheckRequirement, 0, len(policy.checks))
	coverage := make([]Evidence, 0, len(policy.checks))
	for _, baseline := range policy.checks {
		pointer := jsonCAS(baseline.definition.Digest)
		definitionBytes, err := resolver.resolve(ctx, pointer, MaximumLocalCheckDefinitionBytes)
		if err != nil {
			return ExactLocalChecks{}, fmt.Errorf("resolve check %q definition: %w", baseline.id, err)
		}
		definition, err := ParseLocalCheckDefinition(definitionBytes)
		if err != nil {
			return ExactLocalChecks{}, fmt.Errorf("parse check %q definition: %w", baseline.id, err)
		}
		requirements = append(requirements, LocalCheckRequirement{
			CheckID: baseline.id, Definition: pointer, definition: definition,
		})
		coverage = append(coverage, Evidence{
			ID: definition.Evidence.ID, AcceptanceIDs: definition.Evidence.AcceptanceIDs,
			Boundary: definition.Evidence.Boundary,
		})
	}
	if err := validateAcceptanceCoverage(work.Acceptance, coverage); err != nil {
		return ExactLocalChecks{}, err
	}
	return ExactLocalChecks{contract: contract, requirements: requirements}, nil
}

func jsonCAS(digest string) Artifact {
	return Artifact{Ref: digest, MediaType: "application/json", Digest: digest}
}

type assurancePolicyRegistry struct {
	checks []assurancePolicyEntry
}

type assurancePolicyEntry struct {
	id         string
	definition Artifact
}

// parseAssurancePolicyRegistry validates the complete Baton assurance-policy-v1
// registry. The registry is policy input rather than a Baton record, but its
// exact canonical digest is selected by the delivery plan.
func parseAssurancePolicyRegistry(contents []byte) (assurancePolicyRegistry, error) {
	if len(contents) == 0 || len(contents) > MaximumAssurancePolicyBytes {
		return assurancePolicyRegistry{}, errors.New("assurance policy is empty or exceeds its byte ceiling")
	}
	canonical, err := CanonicalizeJSON(contents)
	if err != nil {
		return assurancePolicyRegistry{}, fmt.Errorf("assurance policy is not strict I-JSON: %w", err)
	}
	// PlanPolicy.Digest is an RFC 8785 digest, while artifact storage is keyed by
	// exact bytes. Sworn therefore retains the registry in canonical form at that
	// digest instead of confusing a pretty-printed source digest with plan truth.
	if !bytes.Equal(contents, canonical) {
		return assurancePolicyRegistry{}, errors.New("assurance policy must be stored as canonical JSON at its plan digest")
	}
	root, err := decodePlanObject(canonical, "assurance policy", []string{
		"schema_version", "policy_id", "checks", "packs",
	}, nil)
	if err != nil {
		return assurancePolicyRegistry{}, err
	}
	schemaVersion, err := decodePlanString(root["schema_version"], "assurance policy schema_version")
	if err != nil || schemaVersion != AssurancePolicySchemaVersion {
		return assurancePolicyRegistry{}, errors.New("assurance policy has an unknown schema version")
	}
	policyID, err := decodePlanString(root["policy_id"], "assurance policy policy_id")
	if err != nil || !ValidID(policyID) {
		return assurancePolicyRegistry{}, errors.New("assurance policy has an invalid policy_id")
	}
	checks, err := parseAssurancePolicyEntries(root["checks"], "assurance policy check", false)
	if err != nil {
		return assurancePolicyRegistry{}, err
	}
	if len(checks) == 0 || len(checks) > MaximumExactLocalChecks {
		return assurancePolicyRegistry{}, fmt.Errorf("assurance policy requires 1-%d local checks", MaximumExactLocalChecks)
	}
	if _, err := parseAssurancePolicyEntries(root["packs"], "assurance policy pack", true); err != nil {
		return assurancePolicyRegistry{}, err
	}
	return assurancePolicyRegistry{checks: checks}, nil
}

func parseAssurancePolicyEntries(
	raw json.RawMessage,
	label string,
	pack bool,
) ([]assurancePolicyEntry, error) {
	items, err := decodePlanArray(raw, label+"s")
	if err != nil {
		return nil, err
	}
	entries := make([]assurancePolicyEntry, 0, len(items))
	seenIDs := make(map[string]struct{}, len(items))
	seenItems := make(map[string]struct{}, len(items))
	for index, item := range items {
		itemLabel := fmt.Sprintf("%s %d", label, index)
		object, err := decodePlanObject(item, itemLabel, []string{"id", "definition"}, nil)
		if err != nil {
			return nil, err
		}
		id, err := decodePlanString(object["id"], itemLabel+" id")
		validID := ValidID(id)
		if pack {
			validID = packIDPattern.MatchString(id)
		}
		if err != nil || !validID {
			return nil, fmt.Errorf("%s has an invalid id", itemLabel)
		}
		if _, exists := seenIDs[id]; exists {
			return nil, fmt.Errorf("%s contains duplicate id %q", label+"s", id)
		}
		seenIDs[id] = struct{}{}
		canonicalItem, err := CanonicalizeJSON(item)
		if err != nil {
			return nil, fmt.Errorf("canonicalize %s: %w", itemLabel, err)
		}
		if _, exists := seenItems[string(canonicalItem)]; exists {
			return nil, fmt.Errorf("%s contains duplicate entries", label+"s")
		}
		seenItems[string(canonicalItem)] = struct{}{}
		definition, err := parseAssuranceDefinition(object["definition"], itemLabel+" definition")
		if err != nil {
			return nil, err
		}
		entries = append(entries, assurancePolicyEntry{id: id, definition: definition})
	}
	return entries, nil
}

func parseAssuranceDefinition(raw json.RawMessage, label string) (Artifact, error) {
	object, err := decodePlanObject(raw, label, []string{"ref", "media_type", "digest"}, nil)
	if err != nil {
		return Artifact{}, err
	}
	ref, err := decodePlanString(object["ref"], label+" ref")
	if err != nil || !nonEmpty(ref) {
		return Artifact{}, fmt.Errorf("%s has an invalid ref", label)
	}
	mediaType, err := decodePlanString(object["media_type"], label+" media_type")
	if err != nil || mediaType != "application/json" {
		return Artifact{}, fmt.Errorf("%s must use application/json", label)
	}
	digest, err := decodePlanString(object["digest"], label+" digest")
	if err != nil || !ValidDigest(digest) {
		return Artifact{}, fmt.Errorf("%s has an invalid digest", label)
	}
	return Artifact{Ref: ref, MediaType: mediaType, Digest: digest}, nil
}
