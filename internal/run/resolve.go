package run

import (
	"fmt"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/driver/registry"
)

// ComposeEscalationModels builds the final ordered model list a dispatch resolves
// and dispatches against: implementerModel prepended (if set) to escalationModels.
// Extracted from RunSlice's former inline block (slice.go, pre-S07) so the startup
// sweep (S07 AC-01, cmd/sworn/run.go) composes the IDENTICAL list RunSlice itself
// resolves per-attempt — a list built two different ways is exactly the kind of
// drift that would make "resolved at startup" a false promise.
//
// It NO LONGER falls back to a hardcoded ladder. It used to default to
// ["openai/gpt-4o-mini", "openai/gpt-4o", "openai/o3-mini", "openai/o3"] — four
// stale, hardcoded OpenAI models injected whenever nothing was configured. Two
// things rode on that silently: the escalation ladder, and the CAPTAIN, which took
// models[0] and therefore ran the Rule 9 design-authority role on gpt-4o-mini.
// A configured implementer is now the only rung; an empty list means no escalation,
// which is honest, rather than a guess dressed as a default.
func ComposeEscalationModels(implementerModel string, escalationModels []string) []string {
	models := escalationModels
	if implementerModel != "" {
		models = append([]string{implementerModel}, models...)
	}
	return models
}

// DispatchResolution is the outcome of resolving every role leg a slice
// dispatch touches, in one place.
type DispatchResolution struct {
	Verifier     driver.Driver
	Implementers []driver.Driver // parallel to the input escalationModels
	Captain      driver.Driver
	// CaptainErr is non-nil when the captain leg failed to resolve. Per the
	// S06 Coach ruling (captain-proceed.md pin 1, 2026-07-10) this is NEVER
	// fatal — callers log/record it as a Rule 2 deferral and proceed.
	CaptainErr error
}

// ResolveDispatch resolves the verifier, every entry of escalationModels
// (RoleImplementer), and captainModel (RoleCaptain) through reg.
// Verifier/implementer resolution failure returns err (fatal — S06 AC-02,
// S07 AC-01); captain resolution failure is returned via
// DispatchResolution.CaptainErr and is NEVER fatal (S06 captain-proceed.md
// pin 1, 2026-07-10 — no subprocess driver declares RoleCaptain yet;
// sworn#86 tracks restoring role-universality). errPrefix names the caller
// ("RunSlice" or "sworn run") so the wrapped error reads naturally at either
// call site; the wrapped text shape (%q model, %q role) is unchanged from
// RunSlice's pre-S07 inline wrap, so existing tests asserting on that text
// (TestRunSliceResolutionFailure, TestRunSliceCaptainResolutionFailureDefersAndProceeds)
// require no edits.
func ResolveDispatch(reg *registry.Registry, errPrefix, verifierModel string, escalationModels []string, captainModel string) (DispatchResolution, error) {
	var res DispatchResolution

	verifierDriver, err := reg.Resolve(verifierModel, driver.RoleVerifier)
	if err != nil {
		return res, fmt.Errorf("%s: resolve %q for role %q: %w", errPrefix, verifierModel, driver.RoleVerifier, err)
	}
	res.Verifier = verifierDriver

	implDrivers := make([]driver.Driver, len(escalationModels))
	for i, m := range escalationModels {
		d, rerr := reg.Resolve(m, driver.RoleImplementer)
		if rerr != nil {
			return DispatchResolution{}, fmt.Errorf("%s: resolve %q for role %q: %w", errPrefix, m, driver.RoleImplementer, rerr)
		}
		implDrivers[i] = d
	}
	res.Implementers = implDrivers

	// The captain gets its OWN model. It used to take escalationModels[0] — the
	// first rung of a ladder ordered cheapest-first for RETRY — so the role holding
	// Rule 9 design authority silently ran on the weakest model configured. Nobody
	// decided that; it was an artefact of a list built for another purpose.
	captainDriver, captainResolveErr := reg.Resolve(captainModel, driver.RoleCaptain)
	if captainResolveErr != nil {
		res.CaptainErr = fmt.Errorf("%s: resolve %q for role %q: %w", errPrefix, captainModel, driver.RoleCaptain, captainResolveErr)
	} else {
		res.Captain = captainDriver
	}

	return res, nil
}
