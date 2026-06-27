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
	Planned            State = "planned"
	DesignReview       State = "design_review"
	InProgress         State = "in_progress"
	Implemented        State = "implemented"
	Verified           State = "verified"
	FailedVerification State = "failed_verification"
	Deferred           State = "deferred"
)

// allowedTransitions is the state-transition lookup. Every entry is explicit;
// an absent entry means the transition is illegal.
var allowedTransitions = map[State][]State{
	Planned:            {DesignReview, InProgress, Deferred},
	DesignReview:       {InProgress, Deferred},
	InProgress:         {Implemented, Deferred},
	Implemented:        {Verified, FailedVerification, Deferred},
	FailedVerification: {InProgress, Deferred},
	Verified:           {},           // terminal
	Deferred:           {InProgress}, // can resume
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

// ValidationRecord holds the human-ratified requirements validation for one
// slice: scenarios (positive AND negative) and the benefit/alignment hypothesis.
// Populated by the planner during /plan-release or /replan-release. The
// human_ratified flag is mandatory — model-only validation is not a pass.
// See docs/baton/rules/08-requirements-fidelity.md "Validated".
type ValidationRecord struct {
	HumanRatified      bool     `json:"human_ratified"`
	RatifiedBy         string   `json:"ratified_by,omitempty"`
	RatifiedAt         string   `json:"ratified_at,omitempty"`
	PositiveScenarios  []string `json:"positive_scenarios,omitempty"`
	NegativeScenarios  []string `json:"negative_scenarios,omitempty"`
	BenefitHypothesis  string   `json:"benefit_hypothesis,omitempty"`
	ReleaseBenefitLink string   `json:"release_benefit_link,omitempty"`
}

// Dispatch records one role's model dispatch and its USD cost for a slice
// run. One entry per role that dispatched: implementer, verifier, captain,
// orchestrator. CostUSD is 0 when the model was unpriced or the role ran
// deterministically — downstream consumers (S56) treat 0 as "no signal",
// never as "free".
type Dispatch struct {
	Role             string  `json:"role"`
	Model            string  `json:"model"`
	CostUSD          float64 `json:"cost_usd"`
	Attempt          int     `json:"attempt"`
	DurationMS       int64   `json:"duration_ms,omitempty"`
	InputTokens      int64   `json:"input_tokens,omitempty"`
	OutputTokens     int64   `json:"output_tokens,omitempty"`
	ModelIDConfirmed string  `json:"model_id_confirmed,omitempty"`
}
// Verification holds the per-slice verification record (verdict, session
// metadata, violations). It mirrors the nested "verification" object in
// status.json.
type Verification struct {
	Result                  string   `json:"result,omitempty"`
	Model                   string   `json:"model,omitempty"`
	Attempt                 int      `json:"attempt,omitempty"`
	VerifierSessionID       *string  `json:"verifier_session_id,omitempty"`
	VerifierVerdictAt       *string  `json:"verifier_verdict_at,omitempty"`
	VerifierWasFreshContext *bool    `json:"verifier_was_fresh_context,omitempty"`
	Violations              []string `json:"violations,omitempty"`
	// Routing is the blocked-routing owner set by the verifier when it returns
	// a BLOCKED verdict. Consumers (board oracle, router, TUI) use it to direct
	// remediation: "needs_planner" | "needs_human" | "needs_implementer".
	// When absent, the oracle infers from the verdict: "blocked" → needs_planner,
	// "failed_verification" → needs_implementer (S57 spec).
	Routing string `json:"routing,omitempty"`
	// Dispatches records the per-role model and USD cost for each dispatch
	// during the slice run. Omitted from JSON when empty (omitempty).
	// Populated by RunSlice (S55); consumed by ledger.Project (v:2 Records)
	// and S56 cost-aware routing.
	Dispatches []Dispatch `json:"dispatches,omitempty"`
} // StakeClass classifies a design decision by its stakes = reversibility x blast-radius.
// Type-1 (high stakes / hard-to-reverse) requires a recorded human decision.
// Type-2 (low stakes / reversible) may proceed with a noted default.
// See docs/baton/rules/09-design-fidelity.md.
type StakeClass string

const (
	Type1 StakeClass = "Type-1"
	Type2 StakeClass = "Type-2"
)

// DesignDecision records one design choice for a slice. Populated by the
// planner during design and consumed by `sworn designfit <release>` for
// fail-closed enforcement (Rule 9).
//
// A Type-1 choice with no HumanDecision is a violation — the model cannot
// commit to an architecturally-significant choice on its own.
type DesignDecision struct {
	Choice                     string     `json:"choice"`
	StakeClass                 StakeClass `json:"stake_class"`
	Options                    []string   `json:"options,omitempty"`
	HumanDecision              string     `json:"human_decision,omitempty"`
	Rationale                  string     `json:"rationale,omitempty"`
	ArchitecturallySignificant bool       `json:"architecturally_significant,omitempty"`
}

// Status is the full status.json payload for a slice.
type Status struct {
	Schema                string       `json:"$schema"`
	SliceID               string       `json:"slice_id"`
	Release               string       `json:"release"`
	Track                 string       `json:"track"`
	State                 State        `json:"state"`
	Owner                 string       `json:"owner,omitempty"`
	LastUpdatedBy         string       `json:"last_updated_by,omitempty"`
	LastUpdatedAt         string       `json:"last_updated_at,omitempty"`
	StartCommit           string       `json:"start_commit,omitempty"`
	SpecPath              string       `json:"spec_path,omitempty"`
	ProofPath             string       `json:"proof_path,omitempty"`
	JournalPath           string       `json:"journal_path,omitempty"`
	PlannedFiles          []string     `json:"planned_files,omitempty"`
	ActualFiles           []string     `json:"actual_files,omitempty"`
	TestCommands          []string     `json:"test_commands,omitempty"`
	ReachabilityArtifacts []string     `json:"reachability_artifacts,omitempty"`
	OpenDeferrals         []string     `json:"open_deferrals,omitempty"`
	Verification          Verification `json:"verification"`
	ReleaseBase           string       `json:"release_base,omitempty"`
	// Horizontal trace: need ids this slice's acceptance checks satisfy.
	// Populated by the planner during spec authoring; consumed by the RTM
	// (internal/rtm) to build the need -> AC link.
	NeedIDs []string `json:"need_ids,omitempty"`
	// Vertical trace (golden thread): the release benefit this slice
	// contributes to, and the optional org objective. The release goal
	// (from intake.md) is the lightweight floor — when present, every slice
	// satisfies the vertical trace via slice -> release goal without an
	// explicit release_benefit. Org objective is opt-in for enterprise depth.
	ReleaseBenefit  string           `json:"release_benefit,omitempty"`
	OrgObjective    string           `json:"org_objective,omitempty"`
	Validation      ValidationRecord `json:"validation,omitempty"`
	DesignDecisions []DesignDecision `json:"design_decisions,omitempty"`
} // Read parses a status.json file at path and returns the Status. It returns
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

// TransitionGate checks a state transition through a gate callback.
// It first validates that the transition from s to next is legal
// (via s.Transition(next)), then invokes gate. The gate function should
// return nil when the gate passes, or an error describing the failure.
// Pass nil to skip the gate.
//
// Used by the Definition of Ready (S06) to gate planned→in_progress
// on RTM + reqverify + reqvalidate passing without creating a dependency
// from this package to the gate packages.
func (s State) TransitionGate(next State, gate func() error) error {
	if err := s.Transition(next); err != nil {
		return err
	}
	if gate != nil {
		return gate()
	}
	return nil
}
