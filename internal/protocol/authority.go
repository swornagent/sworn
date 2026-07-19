package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	ControlReceiptSchemaVersion = "control-receipt-v1"
	AuthorityApprovalKind       = "authority_approval"
	MaximumControlReceiptBytes  = 1 << 20
)

// ParsedAuthorityGrant is a schema-valid standalone grant view. Unlike
// PlanGrant, it carries no claim that the object came from an ExactPlan.
type ParsedAuthorityGrant struct {
	action         string
	integration    PlanTarget
	hasIntegration bool
	canonical      []byte
}

func (grant ParsedAuthorityGrant) Action() string { return grant.action }

func (grant ParsedAuthorityGrant) Integration() (PlanTarget, bool) {
	return grant.integration, grant.hasIntegration
}

func (grant ParsedAuthorityGrant) CanonicalJSON() []byte {
	return append([]byte(nil), grant.canonical...)
}

// AuthorityApproval is the strict Baton authority_approval control receipt.
// Parsing this shape proves schema validity only; policy authenticates the
// source and exact plan before it is allowed to construct one.
type AuthorityApproval struct {
	SchemaVersion   string            `json:"schema_version"`
	Kind            string            `json:"kind"`
	ReceiptID       string            `json:"receipt_id"`
	PlanDigest      string            `json:"plan_digest"`
	AuthorityDigest string            `json:"authority_digest"`
	SourceRef       string            `json:"source_ref"`
	SourceDigest    string            `json:"source_digest"`
	Grants          []json.RawMessage `json:"grants"`
	Repository      string            `json:"repository"`
	TargetRef       string            `json:"target_ref"`
	AuthorizerRef   string            `json:"authorizer_ref"`
	ApprovedAt      string            `json:"approved_at"`
}

// ParseAuthorityApproval strictly validates exact receipt bytes. Grant raw
// messages are returned as defensive copies of their canonical subobjects.
func ParseAuthorityApproval(contents []byte) (AuthorityApproval, error) {
	if len(contents) > MaximumControlReceiptBytes {
		return AuthorityApproval{}, errors.New("authority approval exceeds byte ceiling")
	}
	canonical, err := CanonicalizeJSON(contents)
	if err != nil {
		return AuthorityApproval{}, fmt.Errorf("authority approval is not strict I-JSON: %w", err)
	}
	object, err := decodePlanObject(canonical, "authority approval", []string{
		"schema_version", "kind", "receipt_id", "plan_digest", "authority_digest",
		"source_ref", "source_digest", "grants", "repository", "target_ref",
		"authorizer_ref", "approved_at",
	}, nil)
	if err != nil {
		return AuthorityApproval{}, err
	}
	var receipt AuthorityApproval
	if err := json.Unmarshal(canonical, &receipt); err != nil {
		return AuthorityApproval{}, fmt.Errorf("decode authority approval: %w", err)
	}
	if err := validateAuthorityApprovalShape(receipt); err != nil {
		return AuthorityApproval{}, err
	}
	// json.Unmarshal above read from canonical bytes, so each retained grant is
	// itself the exact canonical subobject covered by receipt equality checks.
	receipt.Grants = cloneRawMessages(receipt.Grants)
	_ = object // Exact-key validation is the purpose of this retained decode.
	return receipt, nil
}

// EncodeAuthorityApproval validates an engine-owned receipt and returns the
// exact canonical Baton bytes. It authenticates nothing by itself.
func EncodeAuthorityApproval(receipt AuthorityApproval) (EncodedRecord, error) {
	if err := validateAuthorityApprovalShape(receipt); err != nil {
		return EncodedRecord{}, err
	}
	canonical, err := EncodeCanonical(receipt)
	if err != nil {
		return EncodedRecord{}, fmt.Errorf("canonicalize authority approval: %w", err)
	}
	if len(canonical) > MaximumControlReceiptBytes {
		return EncodedRecord{}, errors.New("authority approval exceeds byte ceiling")
	}
	// Round-trip through the strict parser so engine construction and external
	// parsing cannot drift on unknown fields or raw-message edge cases.
	if _, err := ParseAuthorityApproval(canonical); err != nil {
		return EncodedRecord{}, err
	}
	return EncodedRecord{
		Kind:          ControlReceiptSchemaVersion,
		CanonicalJSON: canonical,
		Digest:        CanonicalDigest(canonical),
	}, nil
}

func validateAuthorityApprovalShape(receipt AuthorityApproval) error {
	if receipt.SchemaVersion != ControlReceiptSchemaVersion || receipt.Kind != AuthorityApprovalKind {
		return errors.New("artifact is not a Baton authority approval")
	}
	if !ValidID(receipt.ReceiptID) {
		return errors.New("authority approval has an invalid receipt id")
	}
	for label, digest := range map[string]string{
		"plan": receipt.PlanDigest, "authority": receipt.AuthorityDigest, "source": receipt.SourceDigest,
	} {
		if !ValidDigest(digest) {
			return fmt.Errorf("authority approval has an invalid %s digest", label)
		}
	}
	if !ValidNonEmpty(receipt.SourceRef) || !ValidNonEmpty(receipt.Repository) ||
		!ValidBranchRef(receipt.TargetRef) || !ValidNonEmpty(receipt.AuthorizerRef) {
		return errors.New("authority approval has an invalid source, target, or authorizer")
	}
	if !ValidDateTime(receipt.ApprovedAt) {
		return errors.New("authority approval has an invalid approval time")
	}
	if len(receipt.Grants) == 0 {
		return errors.New("authority approval requires at least one grant")
	}
	seen := make(map[string]struct{}, len(receipt.Grants))
	for index, raw := range receipt.Grants {
		grant, err := ParseAuthorityGrant(raw)
		if err != nil {
			return fmt.Errorf("authority approval grant %d: %w", index, err)
		}
		key := string(grant.CanonicalJSON())
		if _, exists := seen[key]; exists {
			return errors.New("authority approval contains a duplicate grant")
		}
		seen[key] = struct{}{}
	}
	return nil
}

// ParseAuthorityGrant validates one strict Baton grant and retains its exact
// canonical object for set and ordered-equality comparisons.
func ParseAuthorityGrant(contents []byte) (ParsedAuthorityGrant, error) {
	canonical, err := CanonicalizeJSON(contents)
	if err != nil {
		return ParsedAuthorityGrant{}, fmt.Errorf("grant is not strict I-JSON: %w", err)
	}
	object, err := decodePlanObject(canonical, "authority grant", []string{"action", "target"}, nil)
	if err != nil {
		return ParsedAuthorityGrant{}, err
	}
	action, err := decodePlanString(object["action"], "authority grant action")
	if err != nil {
		return ParsedAuthorityGrant{}, err
	}
	switch action {
	case "inspect", "edit", "execute", "commit":
		target, err := decodePlanString(object["target"], "authority grant target")
		if err != nil || target != "workspace" {
			return ParsedAuthorityGrant{}, fmt.Errorf("authority %s grant must target workspace", action)
		}
		return ParsedAuthorityGrant{action: action, canonical: canonical}, nil
	case "integrate":
		targetObject, err := decodePlanObject(object["target"], "authority integration target", []string{"repository", "ref"}, nil)
		if err != nil {
			return ParsedAuthorityGrant{}, err
		}
		repository, err := decodePlanString(targetObject["repository"], "authority integration repository")
		if err != nil || !ValidNonEmpty(repository) {
			return ParsedAuthorityGrant{}, errors.New("authority integration grant has an invalid repository")
		}
		ref, err := decodePlanString(targetObject["ref"], "authority integration ref")
		if err != nil || !ValidBranchRef(ref) {
			return ParsedAuthorityGrant{}, errors.New("authority integration grant has an invalid target ref")
		}
		return ParsedAuthorityGrant{
			action:         action,
			integration:    PlanTarget{Repository: repository, Ref: ref},
			hasIntegration: true,
			canonical:      canonical,
		}, nil
	default:
		return ParsedAuthorityGrant{}, fmt.Errorf("authority grant has unknown action %q", action)
	}
}

func cloneRawMessages(values []json.RawMessage) []json.RawMessage {
	cloned := make([]json.RawMessage, len(values))
	for index := range values {
		cloned[index] = append(json.RawMessage(nil), values[index]...)
	}
	return cloned
}
