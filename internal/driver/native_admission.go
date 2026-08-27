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
