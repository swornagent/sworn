// Package journey defines the critical-customer-journey model — including
// human-walkthrough attestation for fail-closed cutover (Baton Rule 10).
//
// The attestation model lives here because sworn top (S15) reads it as a
// read-only evidence surface, and sworn ship (S13) populates it as a cutover
// gate. Model types are additive and forward-compatible.
package journey

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/swornagent/sworn/internal/baton"
)

// WalkStatus is the human-walkthrough status of a critical customer journey.
type WalkStatus string

const (
	// WalkUnwalked means the journey has not yet been walked by a human.
	WalkUnwalked WalkStatus = "un-walked"
	// WalkPass means the journey was walked and passed against real infra.
	WalkPass WalkStatus = "walked-pass"
	// WalkFail means the journey was walked and a defect or gap was found.
	WalkFail WalkStatus = "walked-fail"
)

// Attestation records one human-walkthrough of a critical customer journey.
// The model CANNOT author this — walked_by is mandatory and human-set (S13
// enforces this at the cutover gate).
type Attestation struct {
	// JourneyID references the journey's ID field (e.g. "J01-onboard-new-user").
	JourneyID string `json:"journey_id"`
	// Status is the walkthrough result: walked-pass, walked-fail, or un-walked.
	Status WalkStatus `json:"status"`
	// WalkedBy records who performed the walkthrough (mandatory for pass/fail).
	WalkedBy string `json:"walked_by"`
	// WalkedAt is when the walkthrough was performed (ISO 8601).
	WalkedAt string `json:"walked_at"`
	// RealInfra asserts the walkthrough ran against real infrastructure.
	RealInfra bool `json:"real_infra"`
	// MocksOff asserts no mock or stub was active during the walkthrough.
	MocksOff bool `json:"mocks_off"`
	// Notes are free-form observations from the human walker.
	Notes string `json:"notes,omitempty"`
}

// Boundary describes the walkthrough boundary context.
type Boundary struct {
	// Name is the boundary context (e.g. "production", "staging").
	Name string `json:"name"`
	// MockBanned is true when no mocks or stubs were active.
	MockBanned bool `json:"mock_banned"`
	// EntitlementBoundary is the entitlement boundary path or identifier.
	EntitlementBoundary string `json:"entitlement_boundary"`
}

// AttestationArtefact is the durable, per-project attestation artefact.
// It lives at .sworn/attestations.json and is populated by sworn ship (S13).
type AttestationArtefact struct {
	// Schema identifies the canonical JSON Schema for this artefact.
	Schema string `json:"$schema"`
	// Version for forward compatibility.
	Version int `json:"version"`
	// Ratification carries human-ratification metadata as a nested object.
	Ratification Ratification `json:"ratification"`
	// Boundary describes the walkthrough boundary context.
	Boundary Boundary `json:"boundary"`
	// Attestations is the list of journey attestations.
	Attestations []Attestation `json:"attestations"`
}

// attestationRelPath is the artefact path relative to the project root.
const attestationRelPath = ".sworn/attestations.json"

// AttestationArtefactPath returns the absolute path to the attestations
// artefact for the given project root.
func AttestationArtefactPath(projectRoot string) string {
	return filepath.Join(projectRoot, attestationRelPath)
}

// LoadAttestations reads the attestations artefact from the project root.
// It returns an empty artefact (no error) when the file does not exist —
// attestations are optional until S13's ship gate is active.
func LoadAttestations(projectRoot string) (*AttestationArtefact, error) {
	path := AttestationArtefactPath(projectRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &AttestationArtefact{
				Schema: baton.AttestationsSchemaURI,
				Version:      1,
				Attestations: []Attestation{},
				Ratification: Ratification{
					By:         "",
					At:         "",
					IsRatified: false,
				},
				Boundary: Boundary{
					Name:                "",
					MockBanned:          false,
					EntitlementBoundary: "",
				},
			}, nil
		}
		return nil, fmt.Errorf("journey: read attestations %s: %w", path, err)
	}

	var a AttestationArtefact
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("journey: parse attestations %s: %w", path, err)
	}
	if a.Version == 0 {
		a.Version = 1
	}
	if a.Schema == "" {
		a.Schema = baton.AttestationsSchemaURI
	}
	if a.Attestations == nil {
		a.Attestations = []Attestation{}
	}
	return &a, nil
}

// SaveAttestations serialises the attestation artefact to .sworn/attestations.json
// under the given project root. It creates the .sworn directory if needed.
// Before writing, it validates the serialised JSON against the embedded
// attestations-v1 schema.
func SaveAttestations(projectRoot string, a *AttestationArtefact) error {
	path := AttestationArtefactPath(projectRoot)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("journey: mkdir %s: %w", dir, err)
	}

	// Ensure $schema is set.
	if a.Schema == "" {
		a.Schema = baton.AttestationsSchemaURI
	}

	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("journey: marshal attestations: %w", err)
	}

	// Validate against the embedded attestations-v1 schema before writing.
	if err := baton.Validate("attestations-v1", data); err != nil {
		return fmt.Errorf("journey: attestation validation failed — not written: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("journey: write attestations %s: %w", path, err)
	}
	return nil
}

// AttestationStatus returns the walkthrough status for a given journey ID.
// It returns WalkUnwalked when no attestation exists for that journey.
func AttestationStatus(artefact *AttestationArtefact, journeyID string) WalkStatus {
	if artefact == nil {
		return WalkUnwalked
	}
	for _, a := range artefact.Attestations {
		if a.JourneyID == journeyID {
			return a.Status
		}
	}
	return WalkUnwalked
}