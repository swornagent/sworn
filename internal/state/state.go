// Package state reads and writes Baton slice status.json files and enforces
// the slice state machine. It is deliberately not goroutine-safe: the caller
// (the run-loop) owns serialisation, consistent with the per-slice
// single-writer guarantee.
//
// Stdlib only — zero runtime dependencies.
package state

import (
	"encoding/json"
	"fmt"
	"os"
)

// State is a Baton slice state. The canonical state machine is:
//
//	planned → in_progress → implemented → verified | failed_verification
//
// design_review is a pre-implementation gate injected between planned and
// in_progress by the implementer/Captain/Coach handshake.
type State string

const (
	Planned           State = "planned"
	DesignReview      State = "design_review"
	InProgress        State = "in_progress"
	Implemented       State = "implemented"
	Verified          State = "verified"
	FailedVerification State = "failed_verification"
)

// allowedTransitions is the state-transition lookup. Every entry is explicit;
// an absent entry means the transition is illegal.
var allowedTransitions = map[State][]State{
	Planned:            {DesignReview, InProgress},
	DesignReview:       {InProgress},
	InProgress:         {Implemented},
	Implemented:        {Verified, FailedVerification},
	FailedVerification: {InProgress},
	Verified:           {}, // terminal
}

// Transition returns nil if moving from s to next is legal. It fails closed:
// unknown states and unlisted transitions both return errors.
func (s State) Transition(next State) error {
	allowed, ok := allowedTransitions[s]
	if !ok {
		return fmt.Errorf("state: unknown current state %q", s)
	}
	for _, a := range allowed {
		if a == next {
			return nil
		}
	}
	return fmt.Errorf("state: illegal transition %s → %s", s, next)
}

// Verification holds the per-slice verification record (verdict, session
// metadata, violations). It mirrors the nested "verification" object in
// status.json.
type Verification struct {
	Result              string   `json:"result,omitempty"`
	VerifierSessionID   *string  `json:"verifier_session_id,omitempty"`
	VerifierVerdictAt   *string  `json:"verifier_verdict_at,omitempty"`
	VerifierWasFreshContext *bool `json:"verifier_was_fresh_context,omitempty"`
	Violations          []string `json:"violations,omitempty"`
}

// Status is the full status.json payload for a slice.
type Status struct {
	Schema              string       `json:"$schema"`
	SliceID             string       `json:"slice_id"`
	Release             string       `json:"release"`
	Track               string       `json:"track"`
	State               State        `json:"state"`
	Owner               string       `json:"owner,omitempty"`
	LastUpdatedBy       string       `json:"last_updated_by,omitempty"`
	LastUpdatedAt       string       `json:"last_updated_at,omitempty"`
	StartCommit         string       `json:"start_commit,omitempty"`
	SpecPath            string       `json:"spec_path,omitempty"`
	ProofPath           string       `json:"proof_path,omitempty"`
	JournalPath         string       `json:"journal_path,omitempty"`
	PlannedFiles        []string     `json:"planned_files,omitempty"`
	ActualFiles         []string     `json:"actual_files,omitempty"`
	TestCommands        []string     `json:"test_commands,omitempty"`
	ReachabilityArtifacts []string   `json:"reachability_artifacts,omitempty"`
	OpenDeferrals       []string     `json:"open_deferrals,omitempty"`
	Verification        Verification `json:"verification"`
	ReleaseBase         string       `json:"release_base,omitempty"`
}

// Read parses a status.json file at path and returns the Status. It returns
// an error if the file cannot be read or is not valid JSON.
func Read(path string) (*Status, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("state: read %s: %w", path, err)
	}
	var s Status
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("state: parse %s: %w", path, err)
	}
	return &s, nil
}

// Write serialises s as indented JSON to path. File mode is 0644.
func Write(path string, s *Status) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("state: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("state: write %s: %w", path, err)
	}
	return nil
}