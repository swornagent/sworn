package run

import (
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/driver"
)

// TestComposeEscalationModels covers the three composition arms in
// isolation: implementer prepended, no implementer, and the
// DefaultEscalationModels fallback when the result would otherwise be
// empty. RunSlice and cmd/sworn's startup sweep both call this helper, so a
// regression here would silently desync "resolvable at startup" from
// "resolvable per-attempt" (S07 D1).
func TestComposeEscalationModels(t *testing.T) {
	t.Run("implementer prepended to escalation list", func(t *testing.T) {
		got := ComposeEscalationModels("fake/impl", []string{"fake/e1", "fake/e2"})
		want := []string{"fake/impl", "fake/e1", "fake/e2"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("no implementer — escalation list used as-is", func(t *testing.T) {
		got := ComposeEscalationModels("", []string{"fake/e1"})
		if len(got) != 1 || got[0] != "fake/e1" {
			t.Errorf("got %v, want [fake/e1]", got)
		}
	})

	t.Run("empty result falls back to DefaultEscalationModels", func(t *testing.T) {
		got := ComposeEscalationModels("", nil)
		if len(got) != len(DefaultEscalationModels) {
			t.Fatalf("got %v, want DefaultEscalationModels %v", got, DefaultEscalationModels)
		}
		for i := range DefaultEscalationModels {
			if got[i] != DefaultEscalationModels[i] {
				t.Errorf("got[%d] = %q, want %q", i, got[i], DefaultEscalationModels[i])
			}
		}
	})
}

// TestResolveDispatch_AllLegsResolve is the happy-path unit test: every
// role leg resolves through the registry, no error, and Captain is set
// (CaptainErr nil).
func TestResolveDispatch_AllLegsResolve(t *testing.T) {
	fd := &fakeDriver{}
	reg := testRegistry(fd)

	res, err := ResolveDispatch(reg, "test", "fake/verifier", []string{"fake/impl1", "fake/impl2"})
	if err != nil {
		t.Fatalf("ResolveDispatch: %v", err)
	}
	if res.Verifier == nil {
		t.Error("expected Verifier to be resolved")
	}
	if len(res.Implementers) != 2 {
		t.Fatalf("expected 2 implementer drivers, got %d", len(res.Implementers))
	}
	if res.Captain == nil {
		t.Error("expected Captain to be resolved")
	}
	if res.CaptainErr != nil {
		t.Errorf("expected no CaptainErr, got %v", res.CaptainErr)
	}
}

// TestResolveDispatch_VerifierFailureIsFatal proves an unresolvable verifier
// model returns a fatal error naming the model and role, wrapped with the
// caller-supplied errPrefix — the identical wrap RunSlice's pre-S07 inline
// block produced, so downstream error-text assertions in capabilities_test.go
// require no edits.
func TestResolveDispatch_VerifierFailureIsFatal(t *testing.T) {
	fd := &fakeDriver{}
	reg := testRegistry(fd)

	_, err := ResolveDispatch(reg, "test-prefix", "nope/model-x", []string{"fake/impl"})
	if err == nil {
		t.Fatal("expected a fatal error for an unresolvable verifier model")
	}
	if !strings.HasPrefix(err.Error(), "test-prefix: resolve") {
		t.Errorf("expected error to be wrapped with the caller's errPrefix, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"nope/model-x"`) {
		t.Errorf("expected error to name the model ID, got: %v", err)
	}
	if !strings.Contains(err.Error(), `role "verifier"`) {
		t.Errorf("expected error to name the role, got: %v", err)
	}
}

// TestResolveDispatch_ImplementerFailureIsFatal mirrors the verifier case
// for an escalation-list entry naming an unregistered prefix.
func TestResolveDispatch_ImplementerFailureIsFatal(t *testing.T) {
	fd := &fakeDriver{}
	reg := testRegistry(fd)

	_, err := ResolveDispatch(reg, "test-prefix", "fake/verifier", []string{"fake/impl", "nope/model-x"})
	if err == nil {
		t.Fatal("expected a fatal error for an unresolvable escalation-list entry")
	}
	if !strings.Contains(err.Error(), `"nope/model-x"`) {
		t.Errorf("expected error to name the model ID, got: %v", err)
	}
	if !strings.Contains(err.Error(), `role "implementer"`) {
		t.Errorf("expected error to name the role, got: %v", err)
	}
}

// TestResolveDispatch_CaptainFailureIsNonFatal proves the captain leg's
// resolution failure is returned via CaptainErr — never as the function's
// err return — matching the S06/S07 Coach-ratified fail-open captain policy.
func TestResolveDispatch_CaptainFailureIsNonFatal(t *testing.T) {
	fd := &fakeDriver{
		name:  "no-captain-driver",
		roles: driver.RoleSet{driver.RoleImplementer: true, driver.RoleVerifier: true},
	}
	reg := testRegistry(fd)

	res, err := ResolveDispatch(reg, "test", "fake/verifier", []string{"fake/impl"})
	if err != nil {
		t.Fatalf("captain resolution failure must not be fatal, got err: %v", err)
	}
	if res.CaptainErr == nil {
		t.Fatal("expected CaptainErr to be set")
	}
	if !strings.Contains(res.CaptainErr.Error(), `role "captain"`) {
		t.Errorf("expected CaptainErr to name the role, got: %v", res.CaptainErr)
	}
	if res.Captain != nil {
		t.Error("expected Captain to be nil when resolution failed")
	}
	// Verifier/implementer legs must still have resolved.
	if res.Verifier == nil {
		t.Error("expected Verifier to still resolve")
	}
	if len(res.Implementers) != 1 || res.Implementers[0] == nil {
		t.Error("expected the implementer leg to still resolve")
	}
}
