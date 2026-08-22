package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/observe"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

// runTelemetryHost is the silent-fail-open run-side export host. It rides in
// the process that hosts delivery (run's service.Start; answer/resume/
// takeover's service.Wait; the TUI resident host) and pumps the shared
// eval.core observer cursor into the configured private and share channels.
// Every construction step degrades silently to "no telemetry": a missing or
// invalid operator config, an unresolvable git root, a busy journal, or a
// failed exporter never fails the run, changes its exit code, or touches a
// delivery journal write (A4).
type runTelemetryHost struct {
	runID string

	evaluator        *observe.Evaluator
	store            *journal.Store
	privateTelemetry *observe.Telemetry
	shareTelemetry   *observe.Telemetry

	cancel context.CancelFunc
	done   chan struct{}

	finishMu sync.Mutex
	finished bool
}

// newRunTelemetryHost builds and starts one export host. It returns nil when
// neither channel is configured or any construction step fails, so callers
// keep today's exact behavior and journal footprint in those cases.
func newRunTelemetryHost(
	ctx context.Context,
	journalPath, runID string,
	runtimeService *runtimepkg.Service,
	operatorConfigPath string,
) *runTelemetryHost {
	if ctx == nil || runID == "" || runtimeService == nil ||
		journalPath == "" {
		return nil
	}
	settings, err := loadOperatorSettings(operatorConfigPath)
	if err != nil {
		return nil
	}
	if settings.otel == nil &&
		(settings.share == nil || !settings.share.Enabled) {
		return nil
	}
	private, share, err := buildTelemetryChannels(ctx, settings)
	if err != nil {
		private = observe.Noop()
	}
	if !private.Status().Enabled && !share.Status().Enabled {
		return nil
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		return nil
	}
	stateReader, err := cockpit.NewGitStateReader(gitExecutable)
	if err != nil {
		return nil
	}
	store, err := journal.Open(ctx, journalPath)
	if err != nil {
		return nil
	}
	projector, err := cockpit.NewProjector(
		store,
		runtimeService,
		stateReader,
	)
	if err != nil {
		_ = store.Close()
		return nil
	}
	evaluator, err := observe.NewEvaluator(store, projector, swornVersion)
	if err != nil {
		_ = store.Close()
		return nil
	}
	hostCtx, cancel := context.WithCancel(context.Background())
	host := &runTelemetryHost{
		runID:            runID,
		evaluator:        evaluator,
		store:            store,
		privateTelemetry: private,
		shareTelemetry:   share,
		cancel:           cancel,
		done:             make(chan struct{}),
	}
	go host.pump(hostCtx)
	return host
}

// buildTelemetryChannels constructs the private and share channels from one
// operator settings value. The private channel keeps its strict posture: a
// construction failure is returned to the caller (serve fails fast on it; the
// run host ignores it). The share channel is fail-open by construction: any
// share failure leaves a disabled channel and never becomes a run or serve
// failure.
func buildTelemetryChannels(
	ctx context.Context,
	settings operatorSettings,
) (*observe.Telemetry, *observe.Telemetry, error) {
	private := observe.Noop()
	if settings.otel != nil {
		telemetry, err := observe.NewOTLP(
			ctx,
			*settings.otel,
			swornVersion,
		)
		if err != nil {
			return private, observe.Noop(), err
		}
		private = telemetry
	}
	share := observe.Noop()
	if settings.share != nil && settings.share.Enabled {
		telemetry, err := observe.NewShareOTLP(
			ctx,
			*settings.share,
			swornVersion,
		)
		if err == nil {
			share = telemetry
		}
	}
	return private, share, nil
}

// resolveJournalOperatorConfig derives the operator config for a run-side
// host from the journal's own git root (the F5 rule): open the nearest
// existing ancestor of the journal path as a git repository, resolve the
// project paths, and admit the discovered operator config only when it is a
// valid private file. A journal outside any admitted git project yields
// silent no-telemetry.
func resolveJournalOperatorConfig(journalPath string) string {
	if journalPath == "" {
		return ""
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		return ""
	}
	start := filepath.Dir(journalPath)
	for {
		if info, statErr := os.Lstat(start); statErr == nil &&
			info.IsDir() {
			break
		}
		parent := filepath.Dir(start)
		if parent == start {
			return ""
		}
		start = parent
	}
	repo, err := gitx.Open(filepath.Clean(start), gitExecutable)
	if err != nil {
		return ""
	}
	paths, err := resolveProjectPaths(repo.Root(), "", "", "")
	if err != nil {
		return ""
	}
	path, diagnostic := discoverOperatorConfig(paths)
	if diagnostic != "" || path == "" {
		return ""
	}
	return path
}

func (h *runTelemetryHost) pump(ctx context.Context) {
	defer close(h.done)
	ticker := time.NewTicker(operatorPollInterval)
	defer ticker.Stop()
	h.advance(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.advance(ctx)
		}
	}
}

// advance moves the shared eval.core observer cursor forward once and feeds
// the same record to both channels. Errors and non-advances are silent.
func (h *runTelemetryHost) advance(ctx context.Context) {
	record, advanced, err := h.evaluator.Advance(ctx, h.runID)
	if err != nil || !advanced {
		return
	}
	h.privateTelemetry.TryEnqueue(record)
	h.shareTelemetry.TryEnqueue(record)
}

// finish stops the pump, catches terminal facts with one final advance,
// drains both queues with a bounded wait, shuts both channels down within
// the operator shutdown bound, and releases the host's journal handle.
// Telemetry teardown can only ever cost time, never delivery.
func (h *runTelemetryHost) finish() {
	if h == nil {
		return
	}
	h.finishMu.Lock()
	defer h.finishMu.Unlock()
	if h.finished {
		return
	}
	h.finished = true
	h.cancel()
	select {
	case <-h.done:
	case <-time.After(operatorShutdownTimeout):
	}
	h.advance(context.Background())
	drainTelemetryChannel(h.privateTelemetry)
	drainTelemetryChannel(h.shareTelemetry)
	shutdownTelemetry(h.privateTelemetry.Shutdown, operatorShutdownTimeout)
	shutdownTelemetry(h.shareTelemetry.Shutdown, operatorShutdownTimeout)
	_ = h.store.Close()
}

// drainTelemetryChannel waits, bounded by operatorShutdownTimeout, until the
// telemetry runtime has consumed every accepted record. The runtime drops
// its queue on shutdown, so this poll must precede Shutdown or the terminal
// record would be silently discarded.
func drainTelemetryChannel(telemetry *observe.Telemetry) {
	if telemetry == nil {
		return
	}
	deadline := time.Now().Add(operatorShutdownTimeout)
	for {
		status := telemetry.Status()
		if !status.Enabled || status.Processed >= status.Accepted {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
