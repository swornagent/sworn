package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	AssurancePolicySchemaVersion = "assurance-policy-v1"
	MaximumAssurancePolicyBytes  = 1 << 20
)

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
	if len(checks) == 0 {
		return assurancePolicyRegistry{}, errors.New("assurance policy requires at least one check")
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
