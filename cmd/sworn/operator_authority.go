package main

import (
	"context"
	"errors"
	"time"

	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

var (
	errOperatorAuthorityMismatch    = errors.New("operator authority mismatch")
	errOperatorAuthorityUnavailable = errors.New(
		"operator authority unavailable",
	)
)

type operatorBindingReader interface {
	RunBinding(context.Context, string) (journal.Run, error)
}

type operatorRunAuthority struct {
	journal        operatorBindingReader
	runID          string
	manifestDigest string
	allowAbsent    bool
}

type operatorProjector struct {
	authority *operatorRunAuthority
	delegate  cockpit.SnapshotAPI
}

type operatorCommands struct {
	authority *operatorRunAuthority
	delegate  cockpit.CommandAPI
}

func (a *operatorRunAuthority) matches(
	ctx context.Context,
) (bool, error) {
	if a == nil || a.journal == nil || ctx == nil ||
		a.runID == "" || a.manifestDigest == "" {
		return false, errOperatorAuthorityUnavailable
	}
	binding, err := a.journal.RunBinding(ctx, a.runID)
	if journal.IsCode(err, "RUN_NOT_FOUND") {
		if a.allowAbsent {
			return false, nil
		}
		return false, errOperatorAuthorityMismatch
	}
	if err != nil {
		return false, errOperatorAuthorityUnavailable
	}
	if binding.ManifestDigest != a.manifestDigest {
		return false, errOperatorAuthorityMismatch
	}
	return true, nil
}

func (a *operatorRunAuthority) require(ctx context.Context) error {
	matched, err := a.matches(ctx)
	if err != nil {
		return err
	}
	if !matched {
		return errOperatorAuthorityUnavailable
	}
	return nil
}

func (p *operatorProjector) Snapshot(
	ctx context.Context,
	runID string,
) (cockpit.Snapshot, error) {
	if p == nil || p.authority == nil || p.delegate == nil ||
		runID != p.authority.runID {
		return cockpit.Snapshot{}, errOperatorAuthorityUnavailable
	}
	if err := p.authority.require(ctx); err != nil {
		return cockpit.Snapshot{}, err
	}
	return p.delegate.Snapshot(ctx, runID)
}

func (p *operatorProjector) Events(
	ctx context.Context,
	runID string,
	after int64,
	limit int,
) (cockpit.EventPage, error) {
	if p == nil || p.authority == nil || p.delegate == nil ||
		runID != p.authority.runID {
		return cockpit.EventPage{}, errOperatorAuthorityUnavailable
	}
	if err := p.authority.require(ctx); err != nil {
		return cockpit.EventPage{}, err
	}
	return p.delegate.Events(ctx, runID, after, limit)
}

func (c *operatorCommands) Start(
	ctx context.Context,
	command cockpit.StartCommand,
) (runtimepkg.RunStatus, error) {
	if c == nil || c.authority == nil || c.delegate == nil ||
		command.ManifestDigest != c.authority.manifestDigest {
		return runtimepkg.RunStatus{}, errOperatorAuthorityUnavailable
	}
	matched, err := c.authority.matches(ctx)
	if err != nil {
		return runtimepkg.RunStatus{}, err
	}
	if !matched && !c.authority.allowAbsent {
		return runtimepkg.RunStatus{}, errOperatorAuthorityUnavailable
	}
	status, err := c.delegate.Start(ctx, command)
	if err != nil {
		return runtimepkg.RunStatus{}, err
	}
	if err := c.authority.require(ctx); err != nil {
		return runtimepkg.RunStatus{}, err
	}
	return status, nil
}

func (c *operatorCommands) Control(
	ctx context.Context,
	command cockpit.ControlCommand,
) (runtimepkg.RunStatus, error) {
	if c == nil || c.authority == nil || c.delegate == nil ||
		command.RunID != c.authority.runID {
		return runtimepkg.RunStatus{}, errOperatorAuthorityUnavailable
	}
	if err := c.authority.require(ctx); err != nil {
		return runtimepkg.RunStatus{}, err
	}
	return c.delegate.Control(ctx, command)
}

func (c *operatorCommands) Redeliver(
	ctx context.Context,
	command cockpit.RedeliveryCommand,
) error {
	if c == nil || c.authority == nil || c.delegate == nil ||
		command.RunID != c.authority.runID {
		return errOperatorAuthorityUnavailable
	}
	if err := c.authority.require(ctx); err != nil {
		return err
	}
	return c.delegate.Redeliver(ctx, command)
}

func waitForRunAuthority(
	ctx context.Context,
	authority *operatorRunAuthority,
	interval time.Duration,
) bool {
	if ctx == nil || authority == nil || interval <= 0 {
		return false
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		matched, err := authority.matches(ctx)
		if matched {
			return true
		}
		if errors.Is(err, errOperatorAuthorityMismatch) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}
