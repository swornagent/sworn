package cockpit

import "time"

const SnapshotSchemaVersion = "sworn.cockpit/v1"

type Snapshot struct {
	SchemaVersion string       `json:"schema_version"`
	Run           RunView      `json:"run"`
	Graph         Graph        `json:"graph"`
	Handoff       Handoff      `json:"handoff"`
	Runtime       RuntimeView  `json:"runtime"`
	Evidence      []Evidence   `json:"evidence"`
	Actions       []Action     `json:"actions"`
	Diagnostics   []Diagnostic `json:"diagnostics"`
	ThroughOffset int64        `json:"through_offset"`
}

type RunView struct {
	ID                string `json:"id"`
	Release           string `json:"release"`
	State             string `json:"state"`
	DesiredState      string `json:"desired_state"`
	ControlGeneration int64  `json:"control_generation"`
	ManifestDigest    string `json:"manifest_digest"`
	PlanDigest        string `json:"plan_digest,omitempty"`
	TargetRef         string `json:"target_ref"`
	TargetHead        string `json:"target_head,omitempty"`
	ReleaseHead       string `json:"release_head,omitempty"`
	Outcome           string `json:"outcome,omitempty"`
}

type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type Node struct {
	ID                 string `json:"id"`
	Kind               string `json:"kind"`
	Label              string `json:"label"`
	Track              string `json:"track,omitempty"`
	State              string `json:"state"`
	Stage              string `json:"stage,omitempty"`
	Outcome            string `json:"outcome,omitempty"`
	NextResponsibility string `json:"next_responsibility,omitempty"`
	Attempt            int64  `json:"attempt,omitempty"`
	HasBaton           bool   `json:"has_baton"`
}

type Edge struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type Handoff struct {
	Ready            bool     `json:"ready"`
	Nodes            []string `json:"nodes"`
	Responsibilities []string `json:"responsibilities"`
}

type RuntimeView struct {
	Owner    OwnerView     `json:"owner"`
	Effects  []EffectView  `json:"effects"`
	Attempts []AttemptView `json:"attempts"`
}

type OwnerView struct {
	Present    bool      `json:"present"`
	Active     bool      `json:"active"`
	Generation int64     `json:"generation"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
}

type EffectView struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	State     string `json:"state"`
	ErrorCode string `json:"error_code,omitempty"`
}

type AttemptView struct {
	EffectID       string    `json:"effect_id"`
	Number         int64     `json:"number"`
	Responsibility string    `json:"responsibility"`
	Transport      string    `json:"transport"`
	InputTokens    *int64    `json:"input_tokens"`
	OutputTokens   *int64    `json:"output_tokens"`
	CostMicroUnits *int64    `json:"cost_micro_units"`
	Currency       *string   `json:"currency"`
	CreatedAt      time.Time `json:"created_at"`
}

type Evidence struct {
	Offset    int64     `json:"offset"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
}

type Action struct {
	Kind               string `json:"kind"`
	ExpectedGeneration int64  `json:"expected_generation"`
	WorkID             string `json:"work_id,omitempty"`
	ExpectedEpoch      int64  `json:"expected_epoch,omitempty"`
}

type Diagnostic struct {
	Code  string `json:"code"`
	Track string `json:"track,omitempty"`
	Work  string `json:"work,omitempty"`
}

type EventPage struct {
	SchemaVersion string     `json:"schema_version"`
	RunID         string     `json:"run_id"`
	Events        []Evidence `json:"events"`
	ThroughOffset int64      `json:"through_offset"`
	EventOffset   int64      `json:"event_offset"`
	HasMore       bool       `json:"has_more"`
}
