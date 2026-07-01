// Package state reads and writes Baton slice status.json files and enforces
// the slice state machine. It is deliberately not goroutine-safe: the caller
// (the run-loop) owns serialisation, consistent with the per-slice
// single-writer guarantee.
//
// Stdlib only — zero runtime dependencies.
package state

import (
	"bytes"
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
	Result                  string      `json:"result,omitempty"`
	Model                   string      `json:"model,omitempty"`
	Attempt                 int         `json:"attempt,omitempty"`
	VerifierSessionID       *string     `json:"verifier_session_id,omitempty"`
	VerifierVerdictAt       *string     `json:"verifier_verdict_at,omitempty"`
	VerifierWasFreshContext *bool       `json:"verifier_was_fresh_context,omitempty"`
	Violations              []Violation `json:"violations,omitempty"`
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
}

// Deferral models one slice-status-v1 open_deferrals object (Rule 2 — No Silent
// Deferrals). The canonical (strict additive) shape is why + tracking +
// acknowledgement + acknowledged_by, with acknowledged_at optional:
// acknowledgement is the plain-text evidence the decision-maker was told;
// acknowledged_by is the structured attribution (who); acknowledged_at is when.
// All four (and acknowledged_at) are named fields here, so a typed consumer
// reads them directly (AC-10). The object is additionalProperties:true, so any
// other coach-produced key (id, description, …) lands in Extra and round-trips
// without loss (AC-03). Custom (Un)MarshalJSON is what makes the carrier
// loss-free where the prior []string carrier failed to unmarshal the object form
// at all (AC-01).
type Deferral struct {
	Item            string `json:"item,omitempty"`
	Why             string `json:"why,omitempty"`
	Tracking        string `json:"tracking,omitempty"`
	Acknowledgement string `json:"acknowledgement,omitempty"`
	AcknowledgedBy  string `json:"acknowledged_by,omitempty"`
	AcknowledgedAt  string `json:"acknowledged_at,omitempty"`
	// Extra preserves unknown keys (additionalProperties:true) so no real-data
	// field is dropped on read or write-back. json:"-" keeps the default codec
	// from touching it; the custom (Un)MarshalJSON below own the merge/split.
	Extra map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON decodes the four named keys into struct fields and routes every
// other key into Extra, so unknown coach keys survive the round trip. It also
// tolerates the legacy string-form deferral that sworn wrote before the D6
// object migration: a bare string is preserved whole in the description-bearing
// Item field (so the Rule-10 matcher and display paths behave exactly as the old
// []string carrier did), then upgraded to the object form on write-back. This is
// a one-way read upgrade — write never flattens an object back to a string.
func (d *Deferral) UnmarshalJSON(data []byte) error {
	*d = Deferral{}
	if t := bytes.TrimSpace(data); len(t) > 0 && t[0] == '"' {
		var s string
		if err := json.Unmarshal(t, &s); err != nil {
			return err
		}
		d.Item = s
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	for k, v := range m {
		switch k {
		case "item":
			if err := json.Unmarshal(v, &d.Item); err != nil {
				return err
			}
		case "why":
			if err := json.Unmarshal(v, &d.Why); err != nil {
				return err
			}
		case "tracking":
			if err := json.Unmarshal(v, &d.Tracking); err != nil {
				return err
			}
		case "acknowledgement":
			if err := json.Unmarshal(v, &d.Acknowledgement); err != nil {
				return err
			}
		case "acknowledged_by":
			if err := json.Unmarshal(v, &d.AcknowledgedBy); err != nil {
				return err
			}
		case "acknowledged_at":
			if err := json.Unmarshal(v, &d.AcknowledgedAt); err != nil {
				return err
			}
		default:
			if d.Extra == nil {
				d.Extra = make(map[string]json.RawMessage)
			}
			d.Extra[k] = v
		}
	}
	return nil
}

// MarshalJSON merges the named fields back over Extra and emits via a map, so
// encoding/json sorts the keys — output is byte-stable across writes regardless
// of input key order (AC-02 / the drift-gate safety property).
func (d Deferral) MarshalJSON() ([]byte, error) {
	m := make(map[string]json.RawMessage, len(d.Extra)+4)
	for k, v := range d.Extra {
		m[k] = v
	}
	if err := putString(m, "item", d.Item); err != nil {
		return nil, err
	}
	if err := putString(m, "why", d.Why); err != nil {
		return nil, err
	}
	if err := putString(m, "tracking", d.Tracking); err != nil {
		return nil, err
	}
	if err := putString(m, "acknowledgement", d.Acknowledgement); err != nil {
		return nil, err
	}
	if err := putString(m, "acknowledged_by", d.AcknowledgedBy); err != nil {
		return nil, err
	}
	if err := putString(m, "acknowledged_at", d.AcknowledgedAt); err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

// String projects a Deferral to a single display line for the []string-consuming
// display paths (proof not-delivered list, etc.). Prefers the description-bearing
// fields; falls back to the overflow "description" key real coach data carries.
func (d Deferral) String() string {
	switch {
	case d.Item != "" && d.Why != "":
		return d.Item + ": " + d.Why
	case d.Why != "":
		return d.Why
	case d.Item != "":
		return d.Item
	}
	if raw, ok := d.Extra["description"]; ok {
		var s string
		if json.Unmarshal(raw, &s) == nil && s != "" {
			return s
		}
	}
	return ""
}

// Violation models one slice-status-v1 verification.violations object. Same
// preserve-unknowns contract as Deferral: named gate/description/evidence/
// proposed_amendment, everything else into Extra (AC-03).
type Violation struct {
	Gate              string                     `json:"gate,omitempty"`
	Description       string                     `json:"description,omitempty"`
	Evidence          string                     `json:"evidence,omitempty"`
	ProposedAmendment string                     `json:"proposed_amendment,omitempty"`
	Extra             map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON: named keys into fields, the rest into Extra. Like Deferral, it
// tolerates the legacy string-form violation (preserved whole in Description, the
// field ViolationStrings() projects back), upgrading it to the object form on
// write-back — a one-way read upgrade, never a write-side flatten.
func (v *Violation) UnmarshalJSON(data []byte) error {
	*v = Violation{}
	if t := bytes.TrimSpace(data); len(t) > 0 && t[0] == '"' {
		var s string
		if err := json.Unmarshal(t, &s); err != nil {
			return err
		}
		v.Description = s
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	for k, raw := range m {
		switch k {
		case "gate":
			if err := json.Unmarshal(raw, &v.Gate); err != nil {
				return err
			}
		case "description":
			if err := json.Unmarshal(raw, &v.Description); err != nil {
				return err
			}
		case "evidence":
			if err := json.Unmarshal(raw, &v.Evidence); err != nil {
				return err
			}
		case "proposed_amendment":
			if err := json.Unmarshal(raw, &v.ProposedAmendment); err != nil {
				return err
			}
		default:
			if v.Extra == nil {
				v.Extra = make(map[string]json.RawMessage)
			}
			v.Extra[k] = raw
		}
	}
	return nil
}

// MarshalJSON: deterministic map-based emit (sorted keys), preserving Extra.
func (v Violation) MarshalJSON() ([]byte, error) {
	m := make(map[string]json.RawMessage, len(v.Extra)+4)
	for k, raw := range v.Extra {
		m[k] = raw
	}
	if err := putString(m, "gate", v.Gate); err != nil {
		return nil, err
	}
	if err := putString(m, "description", v.Description); err != nil {
		return nil, err
	}
	if err := putString(m, "evidence", v.Evidence); err != nil {
		return nil, err
	}
	if err := putString(m, "proposed_amendment", v.ProposedAmendment); err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

// String projects a Violation to a single display line. Matches the historical
// "<gate>: <description>" / "<description>" shape the oracle and verdict bridge
// produced, so ViolationStrings() reproduces the same display the []string
// carrier used (AC-04 / D4).
func (v Violation) String() string {
	switch {
	case v.Gate != "" && v.Description != "":
		return v.Gate + ": " + v.Description
	case v.Description != "":
		return v.Description
	case v.Gate != "":
		return v.Gate
	}
	return ""
}

// ViolationStrings projects the typed violations to the []string view the
// display-only consumers (oracle blocked-reason, ledger record, router) read,
// so the migration diff stays bounded to the contract surface (AC-04 / D2).
func (vf Verification) ViolationStrings() []string {
	out := make([]string, 0, len(vf.Violations))
	for _, v := range vf.Violations {
		out = append(out, v.String())
	}
	return out
}

// putString marshals a non-empty string value into the map under key. Empty
// strings are skipped (omitempty parity) — schema-required fields are non-empty
// in real data, so this never drops a present, meaningful value.
func putString(m map[string]json.RawMessage, key, val string) error {
	if val == "" {
		return nil
	}
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	m[key] = b
	return nil
}

// StakeClass classifies a design decision by its stakes = reversibility x blast-radius.
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
	OpenDeferrals         []Deferral   `json:"open_deferrals,omitempty"`
	Verification          Verification `json:"verification"`
	ReleaseBase           string       `json:"release_base,omitempty"`
	// Horizontal trace: need ids this slice's acceptance checks satisfy.
	// Populated by the planner during spec authoring; consumed by the RTM
	// (internal/rtm) to build the need -> AC link. The slice-status-v1 schema
	// names this covers_needs (the Go field/tag lagged as need_ids, so
	// planner-written covers_needs was silently dropped on read — N-03 / AC-06).
	CoversNeeds []string `json:"covers_needs,omitempty"`
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

// DeferralStrings projects the typed open deferrals to the []string view the
// display-only consumers (proof not-delivered list, first-pass boundary input)
// read, keeping the migration diff bounded to the contract surface (AC-04 / D2).
func (s *Status) DeferralStrings() []string {
	out := make([]string, 0, len(s.OpenDeferrals))
	for _, d := range s.OpenDeferrals {
		out = append(out, d.String())
	}
	return out
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
