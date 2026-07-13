// Package driver defines the role-dispatch contract every loop-role
// dispatch crosses at the process boundary. A driver declares the set of
// roles it can serve (Roles()); resolution checks that declaration before
// ever calling Dispatch, so an incapable driver is rejected by name at
// resolution time instead of being discovered mid-run by a type-assert or a
// toolless dispatch. See docs/adr/0012-driver-contract.md for the Type-1
// decision record.
//
// This package is leaf-only: no driver implements Driver yet (subprocess and
// in-process drivers land in later slices) and nothing in the engine calls
// Dispatch yet (the orchestrator rewire is a later slice too). It must not
// import internal/model or internal/agent — TestNoWireImports enforces this
// so the contract package can never regain the wire-type coupling it exists
// to remove.
package driver

import (
	"context"
	"encoding/json"
	"time"
)

// Role names one of the loop roles a driver can be dispatched to serve.
type Role string

const (
	RoleImplementer Role = "implementer"
	RoleVerifier    Role = "verifier"
	RoleCaptain     Role = "captain"
)

// RoleSet is the declared set of roles a driver can serve. Capability IS the
// role set: resolution calls Has before ever dispatching, so a driver that
// does not declare a role is rejected by name, not discovered mid-run.
type RoleSet map[Role]bool

// Has reports whether r is a member of the set.
func (s RoleSet) Has(r Role) bool {
	return s[r]
}

// roleOrder is the deterministic naming order for String(), independent of
// map iteration order.
var roleOrder = []Role{RoleImplementer, RoleVerifier, RoleCaptain}

// String names the declared roles in a fixed order (implementer, verifier,
// captain), comma-separated. An empty set renders as "(none)".
func (s RoleSet) String() string {
	var names []string
	for _, r := range roleOrder {
		if s.Has(r) {
			names = append(names, string(r))
		}
	}
	if len(names) == 0 {
		return "(none)"
	}
	out := names[0]
	for _, n := range names[1:] {
		out += "," + n
	}
	return out
}

// DispatchInput is everything a driver needs to serve one role dispatch. All
// fields are primitives or stdlib types deliberately — no model.ChatMessage,
// no agent.Agent — so this package never has to import the wire-type
// packages it exists to keep out of the driver contract; a driver's own
// implementation is where those wire types live (internal, not exported
// through this contract).
type DispatchInput struct {
	// Role is the loop role this dispatch serves. Resolution must have
	// already confirmed the target driver's Roles().Has(Role) before
	// calling Dispatch — Dispatch itself does not re-check it.
	Role Role
	// ModelID identifies the model to dispatch to, in the driver's own
	// namespace (e.g. "provider/model" or a CLI-specific identifier).
	ModelID string
	// SystemPrompt is the role's system/instructions text.
	SystemPrompt string
	// Payload is the role's user-turn content (spec, diff, proof — whatever
	// the caller has already assembled into one string).
	Payload string
	// WorktreeRoot is the git working tree the dispatch is rooted at, if
	// any. A driver that spawns work scoped to a directory (a subprocess
	// CLI, a sandboxed edit loop) should pass it to AssertWorktree before
	// spawning (Rule 11 fail-closed target assertion).
	WorktreeRoot string
	// StructuredSchema is the JSON schema a schema-constrained dispatch must
	// emit against. It is role-agnostic (ADR-0012 amendment, D1): the verifier
	// dispatch passes verifier-verdict-v1; the captain-family gates (design
	// TL;DR, reqverify DoR) pass their own sworn-local emit schemas. It is
	// opaque to the driver: the driver's job is to get the model to emit JSON
	// conforming to it and return that JSON unmodified as
	// Result.StructuredJSON. The driver never validates against it and never
	// self-certifies — the ENGINE validates Result.StructuredJSON fail-closed
	// after Dispatch returns. When nil, the dispatch takes the plain prose path
	// (no structured-output constraint). A driver whose resolved client cannot
	// emit structured output for a non-nil schema fails closed with
	// ErrKindUnsupported so the gate can record a declared Rule 2 deferral (D3)
	// rather than a silent pass or a hard prose-format failure.
	StructuredSchema []byte
	// Timeout bounds the dispatch. Zero means the driver's own default.
	Timeout time.Duration
}

// Status is the terminal outcome of one Dispatch call.
type Status string

const (
	// StatusOK means the dispatch completed and produced a result.
	StatusOK Status = "ok"
	// StatusBlocked means the dispatch itself completed but the work cannot
	// proceed for a reason that re-dispatch cannot clear — a spec defect, an
	// out-of-authority change, a missing dependency (S14 semantics binding).
	// Terminal for the lane: the engine makes no further dispatches for the
	// slice, consumes no retry budget, and routes to /replan-release.
	// Distinct from StatusError, which is the driver or transport failing:
	// retryable incompleteness (budget/env) is StatusError with a
	// non-terminal ErrKind, never StatusBlocked. The engine keys ONLY off
	// this Status — it never infers blockedness from ResultText prose.
	StatusBlocked Status = "blocked"
	// StatusError means the dispatch failed. ErrKind names the failure
	// class.
	StatusError Status = "error"
)

// Result is what a Dispatch call returns. CostUSD/CostSource/InputTokens/
// OutputTokens/ModelID/DurationMS are dispatch-economics fields the engine
// records regardless of Status, so telemetry survives a blocked or errored
// dispatch and not just a successful one.
type Result struct {
	Status Status
	// ErrKind is set when Status == StatusError, naming the failure class
	// (e.g. "auth", "credits", "timeout") for triage/escalation logic.
	ErrKind string
	// BlockedReason is the blocker text, set when Status == StatusBlocked.
	// The engine emits it VERBATIM (status.json violations, exit report) —
	// never summarised, never truncated (S14 R-03). It is diagnostic
	// payload, not the signal: the engine keys only off Status and never
	// infers blockedness from prose. Zero value ("") on every other Status.
	BlockedReason string
	// ResultText is the model's raw text response, always populated when
	// available (even alongside StructuredJSON) so callers that only need
	// prose never have to round-trip through JSON.
	ResultText string
	// StructuredJSON is the model's structured output, when the dispatch
	// requested one (DispatchInput.StructuredSchema set — a verifier verdict
	// or a captain-family gate emission). The engine — never the driver —
	// validates this against the schema that was passed in DispatchInput.
	StructuredJSON json.RawMessage
	CostUSD        float64
	// CostSource names where CostUSD came from — one of the CostSource*
	// constants below — since not every provider reports cost.
	CostSource   string
	InputTokens  int64
	OutputTokens int64
	// ModelID is the model that actually served the dispatch, which may
	// differ from DispatchInput.ModelID (e.g. a provider-side alias
	// resolution) — recorded for dispatch-economics telemetry.
	ModelID    string
	DurationMS int64
}

// CostSource names where Result.CostUSD came from (S08, honest cost
// telemetry — sworn#70). Every producer and every test references these
// constants, never a scattered string literal, so a typo (e.g.
// "pricing_table" vs "pricing-table") is a compile error instead of a
// silently schema-valid drift (slice-status-v1 is additionalProperties:true,
// so a wrong string would otherwise pass validation unnoticed).
const (
	// CostSourceProviderReported names a dispatch whose CostUSD came directly
	// off the wire from the provider's own billing figure. Reserved for a
	// future driver whose wired client genuinely returns billing data — no
	// currently-wired client does (the in-process Anthropic client computes
	// CostUSD from the pricing table itself; it is not provider-reported).
	// No live dispatch path emits this value in S08 (spec.json AC-02,
	// amended 2026-07-10).
	CostSourceProviderReported = "provider"
	// CostSourcePricingTable names an in-process dispatch whose CostUSD was
	// computed from the CONFIRMED response model-id and the true token split
	// via the unified pricing registry (model.PriceForModel /
	// model.ComputeCostFromTokens).
	CostSourcePricingTable = "pricing-table"
	// CostSourceCLI names a subprocess dispatch whose CostUSD was reported
	// directly by the CLI (e.g. claude -p's total_cost_usd, when positive).
	CostSourceCLI = "cli"
	// CostSourceSubscription names a dispatch that is known to be covered by
	// a subscription rather than metered API billing — CostUSD is honestly
	// 0, not a fabricated API-equivalent price. Emitted only on a positively
	// identified, testable marker in the CLI's own output; no such marker
	// exists in the currently observed claude-cli/codex output, so no
	// subprocess driver emits this value in S08 (Coach-ratified fail-closed
	// posture, design_decisions D1/D2 — see this slice's status.json).
	CostSourceSubscription = "subscription"
	// CostSourceUnknown names a dispatch whose true cost source could not be
	// positively identified — CostUSD is recorded as 0 (fail-closed honesty)
	// rather than guessed or defaulted.
	CostSourceUnknown = "unknown"
)

// Driver is the role-dispatch contract every loop-role dispatch crosses.
// Implementations wrap a specific delivery mechanism — a subprocess CLI, an
// in-process API client — behind this one method so the engine dispatches
// uniformly regardless of mechanism.
type Driver interface {
	// Name identifies the driver for logging, telemetry, and resolution
	// (e.g. "claude-subprocess", "codex-subprocess", "oai-inprocess").
	Name() string
	// Roles declares which loop roles this driver can serve. Resolution
	// calls Roles().Has(role) before ever dispatching that role to this
	// driver — capability IS the declared role set.
	Roles() RoleSet
	// Dispatch serves one role dispatch. For a schema-constrained dispatch
	// (DispatchInput.StructuredSchema set — a verifier verdict or a
	// captain-family gate emission), the driver returns the model's emission
	// as Result.StructuredJSON and never validates or self-certifies it — the
	// ENGINE validates it against DispatchInput.StructuredSchema, fail-closed,
	// after Dispatch returns (see docs/adr/0012-driver-contract.md).
	Dispatch(ctx context.Context, in DispatchInput) (Result, error)
}
