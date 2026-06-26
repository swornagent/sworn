package journey

import (
	"fmt"
	"sort"
)

// ShipGateResult holds the outcome of a ship gate check.
type ShipGateResult struct {
	// Pass is true when all touched journeys have complete, passing attestations.
	Pass bool `json:"pass"`

	// TouchedJourneys is the set of journeys the release touches.
	TouchedJourneys []string `json:"touched_journeys"`

	// AttestationsFound is the map of journey ID -> attestation for those
	// journeys that have attestations.
	AttestationsFound map[string]*Attestation `json:"-"`

	// BlockedReasons records per-journey reasons when the gate blocks.
	// Key is journey ID, value is the reason.
	BlockedReasons map[string]string `json:"blocked_reasons,omitempty"`
}

// UnwalkedJourneys returns the list of touched journeys that have no
// attestation at all.
func (r *ShipGateResult) UnwalkedJourneys() []string {
	var result []string
	for _, jid := range r.TouchedJourneys {
		if _, ok := r.AttestationsFound[jid]; !ok {
			result = append(result, jid)
		}
	}
	sort.Strings(result)
	return result
}

// IncompleteJourneys returns the list of journey IDs for which the
// attestation exists but is incomplete or failed, with the reason.
func (r *ShipGateResult) IncompleteJourneys() []string {
	var result []string
	for _, jid := range r.TouchedJourneys {
		if reason, ok := r.BlockedReasons[jid]; ok {
			if _, found := r.AttestationsFound[jid]; found {
				result = append(result, jid+": "+reason)
			}
		}
	}
	sort.Strings(result)
	return result
}

// CheckShipGate determines whether a release is ready to ship by verifying
// that every touched journey carries a complete, passing human-walkthrough
// attestation.
//
// projectRoot is the project root (to find .sworn/journeys.json and
// .sworn/attestations.json).
// releaseDir is the absolute path to docs/release/<release-name>/.
//
// Returns a ShipGateResult (where Pass indicates all gates clear) and an
// error for I/O or parse failures.
func CheckShipGate(projectRoot, releaseDir string) (*ShipGateResult, error) {
	// 1. Validate the journeys artefact exists and is ratified.
	checkResult, _, err := Check(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("ship gate: journeys check: %w", err)
	}
	switch checkResult {
	case CheckMissing:
		return nil, fmt.Errorf("ship gate: no journeys artefact at %s — run 'sworn journeys' first (S11)",
			JourneyArtefactPath(projectRoot))
	case CheckUnratified:
		return nil, fmt.Errorf("ship gate: journeys artefact at %s is NOT human-ratified — ratify before shipping",
			JourneyArtefactPath(projectRoot))
	}

	// 2. Determine the release's validation scope via impact analysis.
	impact, err := AnalyzeImpact(projectRoot, releaseDir)
	if err != nil {
		return nil, fmt.Errorf("ship gate: impact analysis: %w", err)
	}

	// 3. Load attestations for this project.
	attestations, err := LoadAttestations(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("ship gate: load attestations: %w", err)
	}

	// 4. Build a lookup of attestations by journey ID.
	attMap := make(map[string]*Attestation, len(attestations.Attestations))
	for i := range attestations.Attestations {
		att := &attestations.Attestations[i]
		attMap[att.JourneyID] = att
	}

	// 5. Check each touched journey.
	touched := impact.JourneysTouched

	result := &ShipGateResult{
		TouchedJourneys:   touched,
		AttestationsFound: make(map[string]*Attestation),
		BlockedReasons:    make(map[string]string),
	}

	// If no journeys are touched, the ship gate passes (nothing to walk).
	if len(touched) == 0 {
		result.Pass = true
		return result, nil
	}

	for _, jid := range touched {
		att, ok := attMap[jid]
		if !ok {
			result.BlockedReasons[jid] = "no attestation"
			continue
		}
		result.AttestationsFound[jid] = att

		reason := attestationComplete(att)
		if reason != "" {
			result.BlockedReasons[jid] = reason
		}
	}

	result.Pass = len(result.BlockedReasons) == 0
	return result, nil
}

// attestationComplete checks whether an attestation has all required fields.
// Returns an empty string if complete, or a human-readable reason if not.
func attestationComplete(att *Attestation) string {
	if att == nil {
		return "no attestation"
	}
	if att.Status != WalkPass {
		switch att.Status {
		case WalkFail:
			return "walkthrough recorded as failed"
		case WalkUnwalked:
			return "attestation exists but status is un-walked"
		default:
			return fmt.Sprintf("unexpected attestation status: %s", att.Status)
		}
	}
	if att.WalkedBy == "" {
		return "walked_by is empty — model cannot author attestation; human must set"
	}
	if !att.RealInfra {
		return "real_infra assertion is missing — walkthrough must assert real infrastructure"
	}
	if !att.MocksOff {
		return "mocks_off assertion is missing — walkthrough must assert mocks are off"
	}
	return ""
}
