package driver

import (
	"context"
	"time"
)

// nativeAdmissionProbeBound is the probe's own bound, distinct from any
// invocation timeout. Tests force expiry honestly by passing a context that
// is already near its deadline, since context.WithTimeout takes the tighter
// of the two deadlines - no test-only override is needed.
const nativeAdmissionProbeBound = 5 * time.Second

const NativeAdmissionProbeEventVersion = "sworn.native-admission-probe/v1"

const (
	nativeAdmissionProbeRefused     = "refused"
	nativeAdmissionProbePassed      = "passed"
	nativeAdmissionProbeUnevaluable = "unevaluable"
)

// NativeAdmissionProbeEvent is the canonical journal body for one dispatch
// admission's pin-liveness probe.
type NativeAdmissionProbeEvent struct {
	SchemaVersion string `json:"schema_version"`
	RunID         string `json:"run_id"`
	Outcome       string `json:"outcome"`
	Reason        string `json:"reason,omitempty"`
}

// ProbeNativeAdmission runs a bounded, side-effect-free liveness check of the
// pinned CLI at dispatch admission. It applies only when the resolved
// adapter is native; every other adapter kind (fake driver, cloud/HTTP
// adapters) returns (nil, nil) - a complete no-op, so no other dispatch path
// gains a new journal event or a new refusal.
//
// A locally-provable dead pin - the pinned binary cannot execute and report
// its version (absent, not executable, exits nonzero, or overflows its
// bounded output) - refuses with NATIVE_PIN_DEAD before anything is spent.
// The probe's own bound expiring is honestly-unevaluable, never a refusal:
// server-side pin death cannot be established without transacting, and stays
// out of scope by the contract's own exclusion. The returned event body is
// non-nil whenever the probe ran (applicable), so the caller journals it
// exactly once per dispatch admission regardless of outcome.
func ProbeNativeAdmission(
	ctx context.Context,
	selected SelectedProfile,
	runID string,
) ([]byte, error) {
	native, ok := selected.adapter.(*nativeAdapter)
	if !ok || native == nil {
		return nil, nil
	}
	outcome, reason := nativeAdmissionLiveness(ctx, native.config)
	body, err := canonicalJSON(NativeAdmissionProbeEvent{
		SchemaVersion: NativeAdmissionProbeEventVersion,
		RunID:         runID,
		Outcome:       outcome,
		Reason:        reason,
	})
	if err != nil {
		return nil, fail("NATIVE_PIN_DEAD")
	}
	if outcome == nativeAdmissionProbeRefused {
		return body, fail("NATIVE_PIN_DEAD")
	}
	return body, nil
}

const NativeCredentialLivenessProbeEventVersion = "sworn.native-credential-liveness-probe/v1"

// NativeCredentialLivenessProbeEvent is the canonical journal body for one
// dispatch admission's credential-liveness probe (A3(a)).
type NativeCredentialLivenessProbeEvent struct {
	SchemaVersion string `json:"schema_version"`
	RunID         string `json:"run_id"`
	Outcome       string `json:"outcome"`
}

// ProbeNativeCredentialLiveness runs a bounded, side-effect-free credential
// liveness check for CLI-native adapters at dispatch admission (A3(a)): the
// class where a statically-valid credential is dead in practice (the
// stale-OAuth precedent). It is a complete no-op for every non-native
// adapter and for a native profile whose CredentialRef is unbound or
// unrecognized - both defer to the existing CREDENTIAL_NOT_CERTIFIED path,
// so no other dispatch path gains a new journal event or refusal.
//
// When applicable, it resolves the credential and reuses
// nativeCredentialLivenessCheck under this probe's own bound. The honesty
// floor is load-bearing: a bound expiry, a resolve failure, or a read that
// cannot positively evaluate the credential is honestly-unevaluable, never a
// refusal - only a positive read of expiry refuses, with zero dispatch burn.
func ProbeNativeCredentialLiveness(
	ctx context.Context,
	selected SelectedProfile,
	runID string,
) ([]byte, error) {
	native, ok := selected.adapter.(*nativeAdapter)
	if !ok || native == nil || selected.Profile.CredentialRef == nil {
		return nil, nil
	}
	ref := *selected.Profile.CredentialRef
	if _, admitted := native.refs[ref]; !admitted {
		return nil, nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, nativeAdmissionProbeBound)
	defer cancel()
	pathValue, resolveErr := native.resolve(probeCtx, ref)
	outcome := nativeAdmissionProbeUnevaluable
	var refusal error
	if probeCtx.Err() == nil && resolveErr == nil {
		stale, evaluated := nativeCredentialLivenessCheck(
			native.config.Family, pathValue, native.config.MaxCredentialBytes,
		)
		switch {
		case evaluated && stale:
			outcome = nativeAdmissionProbeRefused
			refusal = fail("CREDENTIAL_STALE")
		case evaluated:
			outcome = nativeAdmissionProbePassed
		}
	}
	body, err := canonicalJSON(NativeCredentialLivenessProbeEvent{
		SchemaVersion: NativeCredentialLivenessProbeEventVersion,
		RunID:         runID,
		Outcome:       outcome,
	})
	if err != nil {
		return nil, fail("CREDENTIAL_STALE")
	}
	return body, refusal
}

// nativeAdmissionLiveness reuses nativeVersion's bounded, sandboxed
// liveness-check shape (open the pinned closure, run --version under a
// bounded stdout/stderr capture) rather than duplicating it. A probe-bound
// context wraps ctx so this is never slower than nativeAdmissionProbeBound;
// hitting that bound is distinguished from a genuine local failure by
// probeCtx.Err(), since nativeVersion itself collapses every local failure
// mode (absent, unopenable, nonzero exit, overflow) to one error.
func nativeAdmissionLiveness(
	ctx context.Context,
	config NativeAdapterConfig,
) (outcome string, reason string) {
	probeCtx, cancel := context.WithTimeout(ctx, nativeAdmissionProbeBound)
	defer cancel()
	body, err := nativeVersion(probeCtx, config)
	clearBytes(body)
	if probeCtx.Err() != nil {
		return nativeAdmissionProbeUnevaluable, "bound_expired"
	}
	if err != nil {
		return nativeAdmissionProbeRefused, "not_live"
	}
	return nativeAdmissionProbePassed, ""
}
