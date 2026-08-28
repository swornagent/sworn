package driver

import (
	"context"
	"io"
	"net/http"
	"time"
)

// providerBalanceProbeBound is this probe's own bound, distinct from any
// invocation timeout, mirroring nativeAdmissionProbeBound's shape.
const providerBalanceProbeBound = 5 * time.Second

// maxProviderBalanceProbeResponseBytes bounds the balance/quota endpoint's
// response: it is a small status object, never a model response.
const maxProviderBalanceProbeResponseBytes = 65_536

const ProviderBalanceProbeEventVersion = "sworn.provider-balance-probe/v1"

// ProviderBalanceProbeEvent is the canonical journal body for one dispatch
// admission's provider balance/quota probe (A3(b)).
type ProviderBalanceProbeEvent struct {
	SchemaVersion string `json:"schema_version"`
	RunID         string `json:"run_id"`
	Outcome       string `json:"outcome"`
}

// ProbeProviderBalance runs a bounded, side-effect-free provider
// balance/quota check for HTTP-family adapters at dispatch admission
// (A3(b)): the class where the provider exposes a cheap balance or quota
// surface (the deepseek negative-balance class, probed by hand as raw
// curls during unattended-operability). It is a complete no-op unless the
// resolved adapter is the HTTP loop adapter and its transport carries a
// configured BalanceProbe - today, that is never, since no in-tree adapter
// configuration sets one.
//
// When applicable, it performs exactly one bounded GET against the
// configured balance endpoint - never the model endpoint - and strictly
// decodes the response as a JSON object, never a substring or regex scan.
// Only a positive, structurally-decoded true value for ExhaustedField
// refuses, via PROVIDER_LIMITED with HardLimit set so S4's classifyKind
// places it at KindHardExhaustion, never paced. Any transport, status, or
// decode failure - unreachable endpoint, non-2xx status, malformed body,
// missing or wrong-typed field - degrades to honestly-unevaluable, never a
// refusal: a probe that cannot see must say so, not detonate an otherwise
// admissible dispatch.
func ProbeProviderBalance(
	ctx context.Context,
	selected SelectedProfile,
	runID string,
) ([]byte, error) {
	loop, ok := selected.adapter.(*loopAdapter)
	if !ok || loop == nil {
		return nil, nil
	}
	transport, ok := loop.transport.(*httpTransport)
	if !ok || transport == nil || transport.config.BalanceProbe == nil {
		return nil, nil
	}
	probe := *transport.config.BalanceProbe
	outcome := nativeAdmissionProbeUnevaluable
	var refusal error
	exhausted, evaluated := providerBalanceExhausted(
		ctx, transport, selected.Profile.CredentialRef, probe,
	)
	switch {
	case evaluated && exhausted:
		outcome = nativeAdmissionProbeRefused
		refusal = &ContractError{Code: "PROVIDER_LIMITED", HardLimit: true}
	case evaluated:
		outcome = nativeAdmissionProbePassed
	}
	body, err := canonicalJSON(ProviderBalanceProbeEvent{
		SchemaVersion: ProviderBalanceProbeEventVersion,
		RunID:         runID,
		Outcome:       outcome,
	})
	if err != nil {
		return nil, &ContractError{Code: "PROVIDER_LIMITED", HardLimit: true}
	}
	return body, refusal
}

// providerBalanceExhausted performs the one bounded GET and strict decode.
// evaluated is false for every infra, status, or decode failure - the
// honesty floor - so the caller can never turn "I could not see" into a
// refusal.
func providerBalanceExhausted(
	ctx context.Context,
	transport *httpTransport,
	ref *string,
	probe BalanceProbeConfig,
) (exhausted bool, evaluated bool) {
	if validateEndpoint(probe.Endpoint) != nil {
		return false, false
	}
	probeCtx, cancel := context.WithTimeout(ctx, providerBalanceProbeBound)
	defer cancel()
	request, err := http.NewRequestWithContext(
		probeCtx, http.MethodGet, probe.Endpoint, nil,
	)
	if err != nil {
		return false, false
	}
	if probe.CredentialHeader != "" && ref != nil && transport.resolve != nil {
		secret, resolveErr := transport.resolve(probeCtx, *ref)
		if resolveErr != nil || len(secret) == 0 || len(secret) > 65_536 {
			clearBytes(secret)
			return false, false
		}
		defer clearBytes(secret)
		request.Header.Set(probe.CredentialHeader, probe.CredentialPrefix+string(secret))
	}
	response, err := transport.client.Do(request)
	if err != nil {
		return false, false
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return false, false
	}
	reader := io.LimitReader(
		response.Body, int64(maxProviderBalanceProbeResponseBytes)+1,
	)
	responseBody, readErr := io.ReadAll(reader)
	if readErr != nil || len(responseBody) > maxProviderBalanceProbeResponseBytes {
		return false, false
	}
	decoded, decodeErr := decodeStrict(
		responseBody, maxProviderBalanceProbeResponseBytes,
	)
	if decodeErr != nil {
		return false, false
	}
	root, ok := decoded.(map[string]any)
	if !ok {
		return false, false
	}
	value, present := root[probe.ExhaustedField]
	if !present {
		return false, false
	}
	flag, ok := value.(bool)
	if !ok {
		return false, false
	}
	return flag, true
}
