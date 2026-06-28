// Package model abstracts the verification model behind a single interface.
//
// Design principle (see SwornAgent captures): the customer owns the model and
// the data path; SwornAgent owns the protocol. The model is a parameter — any
// implementation (OpenAI-compatible /chat/completions, a hosted endpoint, the
// customer's own cloud tenancy) plugs in here. The fresh-context, artefact-only,
// fail-closed protocol is enforced by the caller (package verify), not here.
package model

import "context"

// Capability describes what a model driver can do. It is a bitmask so that
// a single driver can advertise multiple capabilities.
type Capability uint

const (
	CapVerify Capability = 1 << iota
	CapChat
)

// CapabilityProvider exposes the capabilities of a model driver. Every driver
// must implement this; callers can check whether a driver supports Chat
// (required for the implementer role) or any future capability without a
// string-parsing dispatch.
type CapabilityProvider interface {
	Capabilities() Capability
}

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

// Capabilities returns 0 — the unconfigured driver has no capabilities.
func (Unconfigured) Capabilities() Capability { return 0 }

// ErrNotConfigured signals no verifier model/key was provided.
var ErrNotConfigured = constErr("verifier model not configured (pass --verifier-model and the provider key)")

type constErr string

func (e constErr) Error() string { return string(e) }
