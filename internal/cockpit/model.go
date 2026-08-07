package cockpit

import (
	"time"

	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

const SnapshotSchemaVersion = "sworn.cockpit/v2"

type Snapshot struct {
	SchemaVersion     string                            `json:"schema_version"`
	Run               RunView                           `json:"run"`
	Graph             Graph                             `json:"graph"`
	Handoff           Handoff                           `json:"handoff"`
	Runtime           RuntimeView                       `json:"runtime"`
	Evidence          []Evidence                        `json:"evidence"`
	Actions           []Action                          `json:"actions"`
	Diagnostics       []Diagnostic                      `json:"diagnostics"`
	ThroughOffset     int64                             `json:"through_offset"`
	ApprovalOffer     *runtimepkg.ApprovalOffer         `json:"approval_offer,omitempty"`
	CaptainDelegation *runtimepkg.CaptainDelegationView `json:"captain_delegation,omitempty"`
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
	ManifestVersion string       `json:"manifest_version,omitempty"`
	Nodes           []Node       `json:"nodes"`
	Edges           []Edge       `json:"edges"`
	Touchpoints     []Touchpoint `json:"touchpoints,omitempty"`
}

type Node struct {
	ID                 string              `json:"id"`
	Kind               string              `json:"kind"`
	Label              string              `json:"label"`
	Track              string              `json:"track,omitempty"`
	State              string              `json:"state"`
	RuntimeState       string              `json:"runtime_state,omitempty"`
	Stage              string              `json:"stage,omitempty"`
	Outcome            string              `json:"outcome,omitempty"`
	NextResponsibility string              `json:"next_responsibility,omitempty"`
	Attempt            int64               `json:"attempt,omitempty"`
	HasBaton           bool                `json:"has_baton"`
	ContractPath       string              `json:"contract_path,omitempty"`
	ContractDigest     string              `json:"contract_digest,omitempty"`
	BoundEvidence      []BoundEvidenceItem `json:"bound_evidence,omitempty"`
}

// Touchpoint is the cockpit's read-only presentation of one
// baton.TouchpointRelation: a repository path two slices in independent
// tracks both declare, and whether the plan's dependency closure orders
// them. It carries no scheduling authority.
type Touchpoint struct {
	Left    string `json:"left"`
	Right   string `json:"right"`
	Path    string `json:"path"`
	Ordered bool   `json:"ordered"`
	Before  string `json:"before,omitempty"`
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
	Owner                  OwnerView          `json:"owner"`
	Effects                []EffectView       `json:"effects"`
	Attempts               []AttemptView      `json:"attempts"`
	Attentions             []AttentionView    `json:"attentions"`
	AttentionsTruncated    bool               `json:"attentions_truncated"`
	Notifications          []NotificationView `json:"notifications"`
	NotificationsTruncated bool               `json:"notifications_truncated"`
}

type AttentionView struct {
	ID         string `json:"id"`
	LaneID     string `json:"lane_id"`
	State      string `json:"state"`
	Generation int64  `json:"generation"`
	Question   string `json:"question"`
	Answer     string `json:"answer,omitempty"`
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

type NotificationView struct {
	DestinationID     string     `json:"destination_id"`
	SourceEventOffset int64      `json:"source_event_offset"`
	Sequence          int64      `json:"sequence"`
	MessageID         string     `json:"message_id"`
	State             string     `json:"state"`
	Attempts          int64      `json:"attempts"`
	AvailableAt       time.Time  `json:"available_at"`
	ClaimedUntil      *time.Time `json:"claimed_until"`
	DeliveredAt       *time.Time `json:"delivered_at"`
	LastErrorCode     string     `json:"last_error_code,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Evidence struct {
	Offset    int64     `json:"offset"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
}

type Action struct {
	Kind               string                      `json:"kind"`
	ExpectedGeneration int64                       `json:"expected_generation"`
	AttentionID        string                      `json:"attention_id,omitempty"`
	WorkID             string                      `json:"work_id,omitempty"`
	ExpectedEpoch      int64                       `json:"expected_epoch,omitempty"`
	DestinationID      string                      `json:"destination_id,omitempty"`
	MessageID          string                      `json:"message_id,omitempty"`
	Approval           *runtimepkg.ApprovalCommand `json:"approval,omitempty"`
	CaptainDelegation  *CaptainDelegationAction    `json:"captain_delegation,omitempty"`
}

// CaptainDelegationAction carries the complete immutable authority binding
// that a local cockpit must confirm before asking the shared command service
// to mutate Captain authority. Envelope bytes are supplied only at execution
// time and are independently parsed and rebound by runtime.Service.
type CaptainDelegationAction struct {
	Action         string `json:"action"`
	RunID          string `json:"run_id"`
	ManifestDigest string `json:"manifest_digest"`
	ActorClass     string `json:"actor_class"`
	ActorAuthority string `json:"actor_authority"`
	CurrentEpoch   int64  `json:"current_epoch"`
	CurrentDigest  string `json:"current_digest"`
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
