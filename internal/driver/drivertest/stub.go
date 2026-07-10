package drivertest

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/swornagent/sworn/internal/driver"
)

// StubDriver is the reference driver.Driver implementation: a maximally
// contract-conformant driver with no transport at all (design D4). It is both
// (a) the fifth conformance subject — proving the suite's clauses assert the
// behavioural contract, not subprocess/HTTP specifics — and (b) the exact
// driver TestLoopSIT registers for the cold-board smoke, so the SIT's
// dispatched driver can never silently diverge from what the conformance
// suite certified.
//
// Behaviour is scripted per role via Handlers (Captain review pin 8: the
// per-role callback shape, so the SIT can serve the captain-family legs —
// design TL;DR, captain review, DoR requirements grading — with distinct
// outputs). A role with no handler falls back to a benign, contract-conformant
// default. The Rule-11 worktree guard always fires before any handler.
type StubDriver struct {
	// DriverName overrides Name(). Empty means "conformance-reference".
	DriverName string

	// DeclaredRoles overrides Roles(). Nil means all three loop roles.
	DeclaredRoles driver.RoleSet

	// Handlers maps a role to its scripted dispatch behaviour. A nil map or
	// missing entry falls back to the per-role default below.
	Handlers map[driver.Role]func(in driver.DispatchInput) (driver.Result, error)

	mu    sync.Mutex
	calls []driver.DispatchInput
}

// NewStub returns a StubDriver with default (contract-conformant) behaviour
// for every role.
func NewStub() *StubDriver { return &StubDriver{} }

// Name identifies the stub for logging, telemetry, and resolution.
func (d *StubDriver) Name() string {
	if d.DriverName != "" {
		return d.DriverName
	}
	return "conformance-reference"
}

// Roles declares the stub's role set — all three loop roles unless narrowed
// via DeclaredRoles.
func (d *StubDriver) Roles() driver.RoleSet {
	if d.DeclaredRoles != nil {
		return d.DeclaredRoles
	}
	return driver.RoleSet{
		driver.RoleImplementer: true,
		driver.RoleVerifier:    true,
		driver.RoleCaptain:     true,
	}
}

// Calls returns a copy of every DispatchInput the stub has served, in order.
// TestLoopSIT uses it to assert the implement/verify/captain legs actually
// fired through the registry.
func (d *StubDriver) Calls() []driver.DispatchInput {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]driver.DispatchInput, len(d.calls))
	copy(out, d.calls)
	return out
}

// RoleCounts returns how many dispatches each role received.
func (d *StubDriver) RoleCounts() map[driver.Role]int {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := map[driver.Role]int{}
	for _, c := range d.calls {
		out[c.Role]++
	}
	return out
}

// Dispatch serves one scripted dispatch. The Rule-11 fail-closed target
// assertion fires before any handler runs — the stub enforces the guard
// exactly like every registered driver (Coach disposition 5: the worktree
// clause is mandatory, no opt-out knob).
func (d *StubDriver) Dispatch(_ context.Context, in driver.DispatchInput) (driver.Result, error) {
	start := time.Now()
	if err := driver.AssertWorktree(in.WorktreeRoot); err != nil {
		return driver.Result{Status: driver.StatusError, ErrKind: driver.ErrKindConfig}, err
	}

	d.mu.Lock()
	d.calls = append(d.calls, in)
	handler := d.Handlers[in.Role]
	d.mu.Unlock()

	if handler != nil {
		res, err := handler(in)
		if res.DurationMS == 0 {
			res.DurationMS = time.Since(start).Milliseconds()
		}
		return res, err
	}

	res := driver.Result{
		Status:     driver.StatusOK,
		ModelID:    in.ModelID,
		CostSource: driver.CostSourceUnknown,
		DurationMS: time.Since(start).Milliseconds(),
	}
	switch in.Role {
	case driver.RoleVerifier:
		res.ResultText = "scripted verification narrative"
		res.StructuredJSON = json.RawMessage(`{"verdict":"PASS","rationale":"scripted reference verdict"}`)
	default:
		res.ResultText = "scripted " + string(in.Role) + " result"
	}
	return res, nil
}
