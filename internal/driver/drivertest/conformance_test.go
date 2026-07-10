package drivertest

// Self-test: the suite run against its own transport-less reference driver,
// plus unit checks that the well-formed-result clause detects violations
// (a suite whose clauses cannot fail would certify anything).

import (
	"context"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
)

// TestRunAgainstReference proves the suite passes a maximally
// contract-conformant driver with no transport at all — a clause that only a
// subprocess or HTTP driver could satisfy would fail here first (spec R-02).
func TestRunAgainstReference(t *testing.T) {
	e := StubEnrolment(t)
	Run(t, e.NewDriver, e.Options)
}

// TestCheckOKResult_DetectsViolations exercises the clause predicate against
// non-conformant Results so a regression that blinds the suite is caught by
// the suite's own tests.
func TestCheckOKResult_DetectsViolations(t *testing.T) {
	cases := []struct {
		name string
		role driver.Role
		res  driver.Result
		want string // substring of the failure description; "" = conformant
	}{
		{"status-not-ok", driver.RoleImplementer,
			driver.Result{Status: driver.StatusError, ResultText: "x"}, "Status"},
		{"negative-duration", driver.RoleImplementer,
			driver.Result{Status: driver.StatusOK, ResultText: "x", DurationMS: -1}, "DurationMS"},
		{"negative-tokens", driver.RoleImplementer,
			driver.Result{Status: driver.StatusOK, ResultText: "x", InputTokens: -1}, "tokens"},
		{"implementer-empty-text", driver.RoleImplementer,
			driver.Result{Status: driver.StatusOK}, "empty ResultText"},
		{"verifier-no-structured", driver.RoleVerifier,
			driver.Result{Status: driver.StatusOK, ResultText: "prose"}, "no StructuredJSON"},
		{"verifier-not-an-object", driver.RoleVerifier,
			driver.Result{Status: driver.StatusOK, StructuredJSON: []byte(`[1,2]`)}, "JSON object"},
		{"verifier-ok", driver.RoleVerifier,
			driver.Result{Status: driver.StatusOK, StructuredJSON: []byte(`{"verdict":"PASS"}`)}, ""},
		{"implementer-ok", driver.RoleImplementer,
			driver.Result{Status: driver.StatusOK, ResultText: "done"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkOKResult(tc.role, tc.res)
			if tc.want == "" {
				if got != "" {
					t.Fatalf("checkOKResult flagged a conformant result: %s", got)
				}
				return
			}
			if got == "" {
				t.Fatal("checkOKResult passed a non-conformant result")
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("failure description %q does not name the violation (%q)", got, tc.want)
			}
		})
	}
}

// TestStubWorktreeGuard pins the reference driver's own Rule-11 behaviour:
// guard fires before the handler, so a scripted handler is never invoked on a
// bad WorktreeRoot.
func TestStubWorktreeGuard(t *testing.T) {
	invoked := false
	d := &StubDriver{Handlers: map[driver.Role]func(driver.DispatchInput) (driver.Result, error){
		driver.RoleImplementer: func(driver.DispatchInput) (driver.Result, error) {
			invoked = true
			return driver.Result{Status: driver.StatusOK, ResultText: "x"}, nil
		},
	}}
	res, err := d.Dispatch(context.Background(), driver.DispatchInput{
		Role:         driver.RoleImplementer,
		ModelID:      "conformance-reference/scripted",
		WorktreeRoot: "/nonexistent/reference/worktree",
	})
	if err == nil {
		t.Fatal("expected worktree-guard error")
	}
	if res.Status != driver.StatusError || res.ErrKind == "" {
		t.Fatalf("guard result = %+v, want StatusError with named ErrKind", res)
	}
	if invoked {
		t.Fatal("handler ran despite the worktree guard — guard must fire before any work")
	}
}
