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

	"github.com/swornagent/sworn/internal/baton"
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
	// Quadrant is the slice's effort_complexity quadrant (chore/grind/puzzle/epic)
	// at dispatch time (#36 / T16). Capturing it per dispatch is what turns the
	// ledger from a global model leaderboard into the routing function
	// f(effort_complexity, cost/velocity) → model: "which model fits this kind of
	// work, on this distribution." Empty when the slice carries no rating yet.
	Quadrant string `json:"quadrant,omitempty"`
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

// EffortComplexity is the two-axis per-slice rating (ADR-0011 §3.7 / #36). The
// planner sets it during decomposition (canonical in spec-v1); the implementer
// confirms or revises it against code reality (mirrored here in status.json).
// Complexity (Cynefin low/high) drives model choice + verification rigor; effort
// (relative T-shirt low/high) drives timeout/retry budget. The quadrant is
// derivable from the two axes (see Quadrant) and stored as a consistency checksum.
type EffortComplexity struct {
	Effort                 string `json:"effort"`     // "low" | "high"
	Complexity             string `json:"complexity"` // "low" | "high"
	Quadrant               string `json:"quadrant"`   // "chore" | "grind" | "puzzle" | "epic"
	Rationale              string `json:"rationale,omitempty"`
	ConfirmedByImplementer bool   `json:"confirmed_by_implementer,omitempty"`
}

// Quadrant derives the routing quadrant from the two axes — the single source of
// truth for the effort×complexity → quadrant mapping (ADR-0011 §3.7):
//
//	             low complexity   high complexity
//	high effort    grind            epic
//	low effort     chore            puzzle
//
// It returns "" if either axis is not "low" or "high".
func Quadrant(effort, complexity string) string {
	switch {
	case effort == "low" && complexity == "low":
		return "chore"
	case effort == "high" && complexity == "low":
		return "grind"
	case effort == "low" && complexity == "high":
		return "puzzle"
	case effort == "high" && complexity == "high":
		return "epic"
	default:
		return ""
	}
}

// Validate checks the axes are valid enums and the stored Quadrant matches the
// derived one. The quadrant checksum catches an inconsistent planner/model rating
// (e.g. effort=low complexity=high mislabelled "grind" instead of "puzzle").
func (ec EffortComplexity) Validate() error {
	want := Quadrant(ec.Effort, ec.Complexity)
	if want == "" {
		return fmt.Errorf("effort_complexity: invalid axes effort=%q complexity=%q (each must be low|high)", ec.Effort, ec.Complexity)
	}
	if ec.Quadrant != want {
		return fmt.Errorf("effort_complexity: quadrant %q inconsistent with effort=%q complexity=%q (want %q)", ec.Quadrant, ec.Effort, ec.Complexity, want)
	}
	return nil
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
	// EffortComplexity is the two-axis routing rating (ADR-0011 §3.7 / #36),
	// mirrored from spec-v1 and confirmed by the implementer. Drives model choice,
	// verification rigor, and timeout/retry budget; the planned→confirmed delta is
	// eval/calibration data. Nil until the planner sets it.
	EffortComplexity *EffortComplexity `json:"effort_complexity,omitempty"`
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
	// Fail closed on an inconsistent effort_complexity rating (#36). JSON Schema
	// enforces the per-axis enums; the quadrant↔axes consistency is a cross-field
	// rule schema can't express, so it's enforced here on every status load.
	if s.EffortComplexity != nil {
		if err := s.EffortComplexity.Validate(); err != nil {
			return nil, fmt.Errorf("state: %s: %w", path, err)
		}
	}
	return &s, nil
}

// Write serialises s as indented JSON to path. File mode is 0644.
// Before writing it sets the canonical $schema field on s and validates
// the marshalled data against the embedded slice-status-v1 schema.
func Write(path string, s *Status) error {
	// Set the canonical $schema field.
	s.Schema = baton.SchemaURI

	// Default verification.result to "pending" when unset. A freshly-created
	// status (e.g. single-slice `sworn run`, which sets no verdict) has no
	// verification result yet, but slice-status-v1 requires verification.result
	// to be present; "pending" is the canonical not-yet-verified value. Without
	// this, every initial status write (and ~28 run tests) failed validation.
	// (2026-06-28 reconcile.)
	if s.Verification.Result == "" {
		s.Verification.Result = "pending"
	}

	// Fail closed on an inconsistent effort_complexity rating (#36) before it
	// reaches disk. Write still runs through the legacy baton.Validate (top-level
	// fields only — the ValidateSchema rewire is step 1b), so this Go-level check
	// is what enforces the rating's axis enums and quadrant↔axes consistency on
	// write until then.
	if s.EffortComplexity != nil {
		if err := s.EffortComplexity.Validate(); err != nil {
			return fmt.Errorf("state: %w", err)
		}
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("state: marshal: %w", err)
	}

	// Validate against the embedded schema before writing to disk.
	if err := baton.Validate("slice-status-v1", data); err != nil {
		return fmt.Errorf("state: validation failed: %w", err)
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
