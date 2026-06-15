// Package model abstracts the verification model behind a single interface.
//
// Design principle (see SwornAgent captures): the customer owns the model and
// the data path; SwornAgent owns the protocol. The model is a parameter — any
// implementation (OpenAI-compatible /chat/completions, a hosted endpoint, the
// customer's own cloud tenancy) plugs in here. The fresh-context, artefact-only,
// fail-closed protocol is enforced by the caller (package verify), not here.
package model

import "context"

// Verifier dispatches one fresh-context verification and returns the model's raw
// verdict text plus the dispatch cost in USD (0 if the provider does not report
// cost).
type Verifier interface {
	Verify(ctx context.Context, systemPrompt, userPayload string) (text string, costUSD float64, err error)
}

// Unconfigured is the default until a provider client is wired (next slice:
// OpenAI-compatible client). It fails closed so an unconfigured gate BLOCKS
// rather than silently passing.
type Unconfigured struct{}

func (Unconfigured) Verify(context.Context, string, string) (string, float64, error) {
	return "", 0, ErrNotConfigured
}

// ErrNotConfigured signals no verifier model/key was provided.
var ErrNotConfigured = constErr("verifier model not configured (pass --verifier-model and the provider key)")

type constErr string

func (e constErr) Error() string { return string(e) }
