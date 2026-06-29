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
	// CapStructuredOutput advertises that a driver implements StructuredOutput:
	// it can be handed a JSON Schema and forced to emit a single conforming JSON
	// object (ADR-0011 authoring path). Additive — a driver that does not set
	// this bit is unchanged.
	CapStructuredOutput
)

// CapabilityProvider exposes the capabilities of a model driver. Every driver
// must implement this; callers can check whether a driver supports Chat
// (required for the implementer role) or any future capability without a
// string-parsing dispatch.
type CapabilityProvider interface {
	Capabilities() Capability
}

// StructuredOutput dispatches a chat completion constrained to emit a single
// JSON object conforming to the supplied JSON Schema (ADR-0011 authoring path;
// ADR-0009 invariant: "the machine parses JSON only — never prose"). It is an
// ADDITIVE interface — a driver opts in by implementing it and advertising
// CapStructuredOutput; Verify and Chat signatures are untouched.
//
// Contract:
//   - schema is the LENIENT canonical JSON Schema (opaque bytes, no name).
//     Drivers using OpenAI strict response_format project it to the strict
//     profile at call time (D1); drivers using the tool-call fallback pass it
//     through unchanged as a single forced tool's parameters.
//   - The emitted JSON object is returned normalised into the first choice's
//     Content (Choices[0].Message.Content) regardless of which path produced it.
//   - ChatStructured is fail-closed at the WIRE level only: it guarantees the
//     content is non-empty and parses as a JSON object, erroring otherwise.
//     SEMANTIC validation against the canonical schema BY NAME
//     (baton.ValidateSchema) is the caller's responsibility — the schema layer
//     stays decoupled from this wire layer because the name never crosses here.
type StructuredOutput interface {
	ChatStructured(ctx context.Context, messages []ChatMessage, schema []byte) (*ChatResponse, error)
}

// Verifier dispatches one fresh-context verification and returns the model's raw
// verdict text plus the dispatch cost in USD (0 if the provider does not report
// cost). Token counts (input/output) and the confirmed model ID from the response
// are returned for dispatch-record enrichment (S24).
type Verifier interface {
	Verify(ctx context.Context, systemPrompt, userPayload string) (text string, costUSD float64, inputTokens int64, outputTokens int64, err error)
}

// ModelPricing holds the USD cost per 1M tokens for a model.
type ModelPricing struct {
	InputPricePer1M  float64
	OutputPricePer1M float64
}

// PriceForModel returns the pricing for a model ID across all known provider
// pricing maps. Returns (0, 0, false) for unknown models.
func PriceForModel(modelID string) (ModelPricing, bool) {
	// Check OAI pricing map.
	if p, ok := modelPricing[modelID]; ok {
		return ModelPricing{InputPricePer1M: p.promptCostPer1M, OutputPricePer1M: p.completionCostPer1M}, true
	}
	// Check Anthropic pricing map.
	if p, ok := anthropicPricing[modelID]; ok {
		return ModelPricing{InputPricePer1M: p.inputPricePer1M, OutputPricePer1M: p.outputPricePer1M}, true
	}
	// Check Google pricing map.
	if p, ok := googlePricing[modelID]; ok {
		return ModelPricing{InputPricePer1M: p.inputPricePer1M, OutputPricePer1M: p.outputPricePer1M}, true
	}
	// Check Bedrock pricing map.
	if p, ok := bedrockPricing[modelID]; ok {
		return ModelPricing{InputPricePer1M: p.inputPricePer1M, OutputPricePer1M: p.outputPricePer1M}, true
	}
	return ModelPricing{}, false
}
// ComputeCostFromTokens returns the USD cost for a model given token counts.
// Returns 0 for unknown models.
func ComputeCostFromTokens(modelID string, inputTokens, outputTokens int64) float64 {
	p, ok := PriceForModel(modelID)
	if !ok {
		return 0
	}
	inputCost := float64(inputTokens) / 1_000_000 * p.InputPricePer1M
	outputCost := float64(outputTokens) / 1_000_000 * p.OutputPricePer1M
	return inputCost + outputCost
}

// Unconfigured is the default until a provider client is wired (next slice:
// OpenAI-compatible client). It fails closed so an unconfigured gate BLOCKS
// rather than silently passing.
type Unconfigured struct{}

func (Unconfigured) Verify(context.Context, string, string) (string, float64, int64, int64, error) {
	return "", 0, 0, 0, ErrNotConfigured
}

// Capabilities returns 0 — the unconfigured driver has no capabilities.
func (Unconfigured) Capabilities() Capability { return 0 }

// ErrNotConfigured signals no verifier model/key was provided.
var ErrNotConfigured = constErr("verifier model not configured (pass --verifier-model and the provider key)")

type constErr string

func (e constErr) Error() string { return string(e) }