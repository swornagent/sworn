package journal

import "time"

type Run struct {
	ID             string
	ManifestDigest string
	Repository     string
	Release        string
	TargetRef      string
	CreatedAt      time.Time
}

type Command struct {
	RunID     string
	ReplayKey string
	Kind      string
	Payload   []byte
	CreatedAt time.Time
}

type EffectState string

const (
	Pending           EffectState = "pending"
	Claimed           EffectState = "claimed"
	Succeeded         EffectState = "succeeded"
	OperationalFailed EffectState = "operational_failed"
	Uncertain         EffectState = "uncertain"
)

type Effect struct {
	RunID          string
	ID             string
	ReplayKey      string
	Kind           string
	State          EffectState
	BeforeDigest   string
	ExpectedDigest string
	CurrentClaim   string
	ResultDigest   string
	Result         []byte
	ErrorCode      string
	UpdatedAt      time.Time
}

type Claim struct {
	RunID      string
	EffectID   string
	Token      string
	AcquiredAt time.Time
	ExpiresAt  time.Time
}

type Attempt struct {
	Number            int64
	Responsibility    string
	TransportStatus   string
	ObservationDigest string
	Usage             []byte
	HandoffDigest     string
}

type Receipt struct {
	Kind string
	Body []byte
}

type Completion struct {
	RunID     string
	EffectID  string
	Token     string
	State     EffectState
	Result    []byte
	ErrorCode string
	Attempt   *Attempt
	Receipts  []Receipt
	EventKind string
	EventBody []byte
	At        time.Time
	// ExpectedEventOffset is an optional journal-wide CAS for transitions that
	// must linearize authority against another completion.
	ExpectedEventOffset *int64
}

type RecoveryDisposition string

const (
	RecoveryAllOld    RecoveryDisposition = "all_old"
	RecoveryAllNew    RecoveryDisposition = "all_new"
	RecoveryAmbiguous RecoveryDisposition = "ambiguous"
)

type Event struct {
	Offset     int64
	RunID      string
	Kind       string
	BodyDigest string
	Body       []byte
	CreatedAt  time.Time
}

type Snapshot struct {
	Run      Run
	Commands []Command
	Effects  []Effect
	Events   []Event
}
