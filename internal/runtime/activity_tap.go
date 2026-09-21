package runtime

import (
	"time"
)

// ActivityTapEvent is the runtime-declared observation the cockpit ring
// consumes. It carries only what was already bounded and redacted for
// journaling: the durable event offset assigned by
// journal.Store.AppendEventWithOffset, the event kind, the exact journaled
// body bytes, and the journaled timestamp. The ring never sees raw worker
// output, only this already-journaled projection.
//
// The tap is a runtime-declared interface over runtime types so
// internal/cockpit can implement it without an import cycle:
// internal/cockpit imports internal/runtime and not the reverse. The
// cmd/sworn assembly point wires a cockpit ring value into the runtime
// Service that drives the run in-process.
type ActivityTapEvent struct {
	RunID     string
	EffectID  string
	Offset    int64
	Kind      string
	Body      []byte
	CreatedAt time.Time
}

// ActivityTap receives durable activity events for the live ring and
// dispatch-end drops. Both methods must never block the dispatch, never
// fail it, and never persist anything: the ring is an ephemeral,
// bounded, in-memory latency shortening for the same projections the
// journal already holds.
type ActivityTap interface {
	ObserveActivity(ActivityTapEvent)
	DropActivityDispatch(effectID string)
}

// SetActivityTap wires the live activity ring for serve-driven runs. A nil
// tap disables the ring; the activity route then serves the same content
// from the journal alone and says nothing false about liveness.
func (s *Service) SetActivityTap(tap ActivityTap) {
	if s == nil {
		return
	}
	s.activityMu.Lock()
	defer s.activityMu.Unlock()
	s.activityTap = tap
}

func (s *Service) getActivityTap() ActivityTap {
	if s == nil {
		return nil
	}
	s.activityMu.RLock()
	defer s.activityMu.RUnlock()
	return s.activityTap
}

// observeActivityTap feeds one durable event to the ring. It never fails
// the dispatch: a nil tap is a no-op, a panicking tap is recovered, and the
// ring itself never blocks (it drops its oldest turn and counts the drop).
func (s *Service) observeActivityTap(event ActivityTapEvent) {
	tap := s.getActivityTap()
	if tap == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	tap.ObserveActivity(event)
}

// dropActivityDispatch forgets one dispatch's ring entries when the
// dispatch ends. It never fails the dispatch for the same reasons as
// observeActivityTap.
func (s *Service) dropActivityDispatch(effectID string) {
	if effectID == "" {
		return
	}
	tap := s.getActivityTap()
	if tap == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	tap.DropActivityDispatch(effectID)
}
