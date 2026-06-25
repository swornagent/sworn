package scheduler

import "sync"

// PauseEngine manages cooperative pause signals for release workers.
// A paused release causes its workers to stop at the next router-poll
// boundary (after completing any in-flight dispatch). Resume by calling
// ResumeRelease or re-running "sworn run --parallel".
//
// Thread-safe: all methods may be called concurrently from CLI, TUI, and MCP.
type PauseEngine struct {
	mu     sync.Mutex
	paused map[string]chan struct{}
}

// NewPauseEngine returns an empty PauseEngine.
func NewPauseEngine() *PauseEngine {
	return &PauseEngine{paused: make(map[string]chan struct{})}
}

// PauseRelease signals workers for release to stop at the next poll boundary.
// Idempotent — a second call while already paused is a no-op.
func (e *PauseEngine) PauseRelease(release string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.paused[release]; !ok {
		ch := make(chan struct{})
		close(ch)
		e.paused[release] = ch
	}
}

// ResumeRelease clears the pause signal for release. Workers that already
// returned TrackPaused are not restarted — re-run "sworn run --parallel"
// to resume from committed state.
func (e *PauseEngine) ResumeRelease(release string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.paused, release)
}

// PauseCh returns the pause channel for release. Workers should check this
// at the top of each loop iteration (non-blocking select):
//
//	select {
//	case <-opts.PauseCh: return TrackPaused
//	default:
//	}
//
// Returns nil if the release is not paused (nil channel blocks forever in
// the select default, so the non-blocking check is a no-op).
func (e *PauseEngine) PauseCh(release string) <-chan struct{} {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.paused[release]
}

// DefaultPauseEngine is the process-global PauseEngine used when callers do
// not supply their own. Shared by CLI, TUI, and MCP via the engine layer.
var DefaultPauseEngine = NewPauseEngine()

// PauseRelease signals a pause on the DefaultPauseEngine.
func PauseRelease(release string) { DefaultPauseEngine.PauseRelease(release) }

// ResumeRelease clears the pause signal on the DefaultPauseEngine.
func ResumeRelease(release string) { DefaultPauseEngine.ResumeRelease(release) }
