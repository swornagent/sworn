package runtime

import (
	"encoding/json"

	"github.com/swornagent/sworn/internal/driver"
)

// EventAssociation carries content-free effect, work, track, slice, and
// responsibility association for runtime-journaled events.
type EventAssociation struct {
	EffectID       string                `json:"effect_id,omitempty"`
	WorkID         string                `json:"work_id,omitempty"`
	Track          string                `json:"track,omitempty"`
	Slice          string                `json:"slice,omitempty"`
	Responsibility driver.Responsibility `json:"responsibility,omitempty"`
}

// MarshalAssociation returns canonical JSON bytes for the event association.
func MarshalAssociation(assoc EventAssociation) []byte {
	body, _ := json.Marshal(assoc)
	return body
}
