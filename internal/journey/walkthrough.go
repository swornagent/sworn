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
	WalkedBy string `json:"walked_by,omitempty"`
	// WalkedAt is when the walkthrough was performed (ISO 8601).
	WalkedAt string `json:"walked_at,omitempty"`
	// RealInfra asserts the walkthrough ran against real infrastructure.
	RealInfra bool `json:"real_infra"`
	// MocksOff asserts no mock or stub was active during the walkthrough.
	MocksOff bool `json:"mocks_off"`
	// Notes are free-form observations from the human walker.
	Notes string `json:"notes,omitempty"`
}

// AttestationArtefact is the durable, per-project attestation artefact.
// It lives at .sworn/attestations.json and is populated by sworn ship (S13).
type AttestationArtefact struct {
	// Version for forward compatibility.
	Version int `json:"version"`
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
				Version:      1,
				Attestations: []Attestation{},
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
	if a.Attestations == nil {
		a.Attestations = []Attestation{}
	}
	return &a, nil
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
