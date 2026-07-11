// Package drivertest exports the behavioural conformance suite every
// driver.Driver implementation is held to (S10-conformance-sit, N-07). The
// clauses are driver-agnostic contract clauses from ADR-0012 and the driver
// package's doc comments — never per-driver behaviour (spec R-02: a clause
// that cannot run against every driver is a contract-doc defect to surface,
// not a special case to hide).
//
// Callers construct the driver under test for real — a fake CLI binary on
// PATH, an httptest server behind the proxy seam — never a test double of
// driver.Driver itself (the reference StubDriver is the one deliberate
// exception: it is the transport-less fifth subject). Entry point:
//
//	drivertest.Run(t, newDriver, drivertest.Options{ModelID: "..."})
package drivertest

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/driver/registry"
)

// conformanceVerdictSchema is the opaque verdict schema handed to verifier
// dispatches. It stays inside the OpenAI strict-mode keyword subset (no
// minLength/pattern/format) so strict-projecting clients accept it.
var conformanceVerdictSchema = []byte(`{
  "title": "conformance-verdict",
  "type": "object",
  "additionalProperties": false,
  "required": ["verdict", "rationale"],
  "properties": {
    "verdict": {"type": "string"},
    "rationale": {"type": "string"}
  }
}`)

// Options configures one conformance run. ModelID is required — it is the
// model identifier, in the driver's own namespace, that the caller's fake
// wiring serves. The failing fields drive the error-path clause through the
// driver's own fake-failure mode (a CLI binary that exits non-zero, an
// httptest route that returns 500, a scripted error handler).
type Options struct {
	// ModelID the driver under test can serve (required), e.g.
	// "claude-cli/fake", "deepseek/conformance".
	ModelID string

	// NewFailing constructs the same driver wired to its fake-failure mode.
	// Nil falls back to the happy factory (the failure is then driven by
	// FailingModelID alone — the httptest pattern).
	NewFailing func() driver.Driver

	// FailingModelID is the model ID dispatched on the error-path clause.
	// Empty falls back to ModelID.
	FailingModelID string

	// WorkCount, when non-nil, reports how many times the driver's transport
	// has been invoked (fake-binary spawns, HTTP requests). The Rule-11
	// worktree-guard clauses snapshot it to prove the guard fires BEFORE any
	// work (AC-01).
	WorkCount func() int
}

// dispatchTimeout bounds every conformance dispatch. The fakes are instant;
// this only guards against a hung fake wiring.
const dispatchTimeout = 60 * time.Second

// canonicalRoles is the deterministic role iteration order for the
// per-role well-formed-result clause.
var canonicalRoles = []driver.Role{driver.RoleImplementer, driver.RoleVerifier, driver.RoleCaptain}

// Run executes the conformance suite against the driver newDriver constructs.
// It runs as one subtest named after the driver, with one nested subtest per
// contract clause (AC-04: a failure names driver AND clause,
// e.g. TestDriverConformance/codex-subprocess/worktree-guard-missing-path).
//
// newDriver must return a FRESH driver per call (design D1) so no clause's
// dispatch leaks state into the next.
func Run(t *testing.T, newDriver func() driver.Driver, opts Options) {
	t.Helper()
	if opts.ModelID == "" {
		t.Fatal("drivertest.Run: Options.ModelID is required")
	}
	name := newDriver().Name()

	t.Run(name, func(t *testing.T) {
		// Clause: a successful dispatch returns a well-formed Result, per
		// declared role (AC-01: Status set; tokens/duration non-negative;
		// ResultText or StructuredJSON present per role). The verifier arm is
		// either-or by contract: StructuredJSON that parses as a JSON object,
		// OR fail closed (Status=error with a named ErrKind) — never malformed
		// JSON passed off as a verdict.
		roles := newDriver().Roles()
		for _, role := range canonicalRoles {
			if !roles.Has(role) {
				continue
			}
			role := role
			t.Run("ok-"+string(role), func(t *testing.T) {
				d := newDriver()
				res, err := dispatch(t, d, role, opts.ModelID, gitWorktree(t))
				if role == driver.RoleVerifier {
					if err != nil || res.Status != driver.StatusOK {
						assertFailClosed(t, res, err)
						return
					}
					if msg := checkOKResult(role, res); msg != "" {
						t.Error(msg)
					}
					return
				}
				if err != nil {
					t.Fatalf("Dispatch(%s) error: %v", role, err)
				}
				if msg := checkOKResult(role, res); msg != "" {
					t.Error(msg)
				}
			})
		}

		// Clause: every error path returns Status=error with a non-empty
		// ErrKind and never panics (AC-01), driven via the driver's own
		// fake-failure mode.
		t.Run("error-errkind", func(t *testing.T) {
			factory := opts.NewFailing
			if factory == nil {
				factory = newDriver
			}
			modelID := opts.FailingModelID
			if modelID == "" {
				modelID = opts.ModelID
			}
			d := factory()
			res, err := dispatch(t, d, driver.RoleImplementer, modelID, gitWorktree(t))
			if err == nil {
				t.Fatal("failing dispatch returned nil error")
			}
			assertFailClosed(t, res, err)
		})

		// Clause: requesting a role outside the declared RoleSet fails at the
		// RESOLUTION boundary — Registry.Resolve's role-arm error (AC-01 as
		// amended 2026-07-11; the S01 Type-1 fail-fast-at-resolution contract,
		// ADR-0012). Drivers' Dispatch is deliberately NOT hardened.
		t.Run("undeclared-role-resolution", func(t *testing.T) {
			d := newDriver()
			role := undeclaredRole(d)
			reg := registry.New()
			reg.Register(registry.Entry{Driver: d, Prefixes: []string{modelPrefix(t, opts.ModelID)}})
			_, err := reg.Resolve(opts.ModelID, role)
			if err == nil {
				t.Fatalf("Registry.Resolve(%q, %q) returned nil error for an undeclared role", opts.ModelID, role)
			}
			if !strings.Contains(err.Error(), "cannot serve role") {
				t.Errorf("Resolve role-arm error = %q, want it to name the role rejection (\"cannot serve role\")", err)
			}
		})

		// Clause: an undeclared-role Dispatch — which resolution would never
		// permit — must still never panic (AC-01's no-panic floor; the result
		// is otherwise unspecified because ADR-0012 places role enforcement at
		// resolution, not in Dispatch).
		t.Run("undeclared-role-dispatch-no-panic", func(t *testing.T) {
			d := newDriver()
			role := undeclaredRole(d)
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Dispatch with undeclared role %q panicked: %v", role, r)
				}
			}()
			_, _ = dispatch(t, d, role, opts.ModelID, gitWorktree(t))
		})

		// Clause: the Rule-11 guard fires on a missing WorktreeRoot BEFORE any
		// work (AC-01). Mandatory for every driver — no opt-out knob (Coach
		// disposition 5); a future transport-less driver failing here is the
		// R-02 surface-to-human path.
		t.Run("worktree-guard-missing-path", func(t *testing.T) {
			assertWorktreeGuard(t, newDriver, opts, "/nonexistent/conformance/worktree")
		})

		// Clause: same guard on a directory that exists but is not a git
		// working tree.
		t.Run("worktree-guard-not-a-worktree", func(t *testing.T) {
			assertWorktreeGuard(t, newDriver, opts, t.TempDir())
		})
	})
}

// dispatch performs one bounded Dispatch call.
func dispatch(t *testing.T, d driver.Driver, role driver.Role, modelID, worktree string) (driver.Result, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), dispatchTimeout)
	defer cancel()
	in := driver.DispatchInput{
		Role:         role,
		ModelID:      modelID,
		SystemPrompt: "conformance system prompt",
		Payload:      "conformance payload",
		WorktreeRoot: worktree,
		Timeout:      dispatchTimeout,
	}
	if role == driver.RoleVerifier {
		in.StructuredSchema = conformanceVerdictSchema
	}
	return d.Dispatch(ctx, in)
}

// checkOKResult validates the well-formed-Result clause for a successful
// dispatch. It returns "" when conformant, else a failure description —
// split out as a pure function so the suite's own self-test can assert the
// clause detects violations (conformance_test.go).
func checkOKResult(role driver.Role, res driver.Result) string {
	if res.Status != driver.StatusOK {
		return fmt.Sprintf("Status = %q, want %q", res.Status, driver.StatusOK)
	}
	if res.DurationMS < 0 {
		return fmt.Sprintf("DurationMS = %d, want non-negative", res.DurationMS)
	}
	if res.InputTokens < 0 || res.OutputTokens < 0 {
		return fmt.Sprintf("tokens negative: input=%d output=%d", res.InputTokens, res.OutputTokens)
	}
	if role == driver.RoleVerifier {
		if len(res.StructuredJSON) == 0 {
			return "verifier dispatch returned no StructuredJSON"
		}
		var probe map[string]any
		if err := json.Unmarshal(res.StructuredJSON, &probe); err != nil {
			return fmt.Sprintf("verifier StructuredJSON does not parse as a JSON object: %v", err)
		}
		return ""
	}
	if strings.TrimSpace(res.ResultText) == "" {
		return fmt.Sprintf("%s dispatch returned empty ResultText", role)
	}
	return ""
}

// assertFailClosed asserts the error-path contract: Status=error and a
// non-empty ErrKind, alongside a non-nil error.
func assertFailClosed(t *testing.T, res driver.Result, err error) {
	t.Helper()
	if err == nil {
		t.Error("fail-closed path returned nil error")
	}
	if res.Status != driver.StatusError {
		t.Errorf("Status = %q, want %q", res.Status, driver.StatusError)
	}
	if res.ErrKind == "" {
		t.Error("ErrKind is empty — every error path must name its failure class")
	}
}

// assertWorktreeGuard dispatches with a bad WorktreeRoot and asserts the
// fail-closed guard fired with no transport work performed.
func assertWorktreeGuard(t *testing.T, newDriver func() driver.Driver, opts Options, badRoot string) {
	t.Helper()
	before := 0
	if opts.WorkCount != nil {
		before = opts.WorkCount()
	}
	d := newDriver()
	res, err := dispatch(t, d, driver.RoleImplementer, opts.ModelID, badRoot)
	assertFailClosed(t, res, err)
	if opts.WorkCount != nil {
		if after := opts.WorkCount(); after != before {
			t.Errorf("transport invoked %d time(s) despite the worktree guard — the guard must fire before any work", after-before)
		}
	}
}

// undeclaredRole picks a role outside d's declared RoleSet: the first
// canonical role not declared, else a synthetic role name (a driver declaring
// all three canonical roles still cannot serve a role that does not exist).
func undeclaredRole(d driver.Driver) driver.Role {
	roles := d.Roles()
	for _, r := range canonicalRoles {
		if !roles.Has(r) {
			return r
		}
	}
	return driver.Role("conformance-undeclared")
}

// modelPrefix extracts the provider prefix from a provider/model ID.
func modelPrefix(t *testing.T, modelID string) string {
	t.Helper()
	idx := strings.IndexByte(modelID, '/')
	if idx <= 0 {
		t.Fatalf("Options.ModelID %q is not provider/model shaped", modelID)
	}
	return modelID[:idx]
}

// gitWorktree returns a fresh directory initialised as a real git working
// tree — driver.AssertWorktree shells out to git, so the happy-path dispatch
// needs a genuine repo, not a bare temp dir.
func gitWorktree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v\n%s", dir, err, out)
	}
	return dir
}
