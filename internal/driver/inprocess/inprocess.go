// Package inprocess implements the driver.Driver contract over the existing
// in-process wire clients: internal/agent.Run (the multi-turn tool loop) and
// the OAI/OpenAIResponses clients resolved by model.NewClient. It wraps —
// it does not rewrite — those mechanisms behind one Dispatch seam so the
// agent loop and provider wire format become an implementation detail
// invisible to the orchestrator.
//
// Placement note (S04 divergence, recorded in the slice journal): the spec's
// touchpoints named internal/driver/inprocess.go, but ADR-0012 pins
// "internal/driver itself imports neither internal/model nor internal/agent"
// and TestNoWireImports (internal/driver/imports_test.go) enforces that over
// every file in that directory. Wire types are this driver's internal
// implementation details (ADR-0012 §Decision), so the driver lives in this
// subpackage — still under internal/driver/, still covered by the spec's
// `go test ./internal/driver/...` command — and imports the contract
// package for Result/DispatchInput/ErrKind* vocabulary.
//
// No logging of message history, file contents, or API keys — per AGENTS.md
// Security; the transcript may contain sensitive workspace data.
package inprocess

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/model"
)

// ErrKind values with no shared constant in the contract package. The
// contract documents "credits" as a sample ErrKind (driver.go Result docs);
// rate_limit/upstream/other reuse the model taxonomy's names verbatim so the
// engine sees one vocabulary across the in-process surface. The shared
// values (config/transient/auth/protocol) come from the driver package
// constants — never string literals (Coach ack pin 2).
const (
	errKindCredits   = "credits"
	errKindRateLimit = "rate_limit"
	errKindUpstream  = "upstream"
	errKindOther     = "other"
)

// defaultTimeout bounds a dispatch when DispatchInput.Timeout is zero,
// mirroring the subprocess drivers' 300s default.
const defaultTimeout = 300 * time.Second

// nominalCostPerToken is the flat ~$2/1M-token placeholder estimate,
// identical in spirit to internal/agent's computeCost. S08 (honest cost
// telemetry) replaces it with real per-model pricing; until then the value
// is clearly tagged CostSource="estimated" so it is never mistaken for a
// provider-reported number (Coach ack pin 5).
const nominalCostPerToken = 0.000002

// InProcess is the in-process OpenAI-compatible driver. One struct carries
// two registered identities (design D1, Coach-decided Type-1): NewOAIChat
// ("oai-inprocess", the chat/completions family) and NewOAIResponses
// ("oai-responses-inprocess", /v1/responses). Both behave identically —
// Dispatch always re-resolves the concrete client from DispatchInput.ModelID
// via model.NewClient — and differ only in the Name they report; which
// provider prefixes route to which identity is S05's (registry) decision.
type InProcess struct {
	name string
	pcfg model.ProviderConfig

	// maxTurns overrides the agent loop's turn cap when > 0 (tests use a
	// small cap; production keeps agent.Run's default of 25).
	maxTurns int

	// newClient resolves a model ID to a concrete client. Defaults to
	// model.NewClient; tests inject a factory pointing at an httptest
	// server so no dispatch ever leaves the process.
	newClient func(modelID string, pcfg model.ProviderConfig) (model.Verifier, error)
}

// NewOAIChat returns the chat/completions-family identity ("oai-inprocess").
func NewOAIChat(pcfg model.ProviderConfig) *InProcess {
	return &InProcess{name: "oai-inprocess", pcfg: pcfg, newClient: model.NewClient}
}

// NewOAIResponses returns the /v1/responses identity
// ("oai-responses-inprocess").
func NewOAIResponses(pcfg model.ProviderConfig) *InProcess {
	return &InProcess{name: "oai-responses-inprocess", pcfg: pcfg, newClient: model.NewClient}
}

// Name identifies this driver instance for logging, telemetry, and
// resolution.
func (d *InProcess) Name() string { return d.name }

// Roles declares implementer and verifier. Captain dispatch is deliberately
// not declared (design D2): no acceptance check in this slice describes it.
func (d *InProcess) Roles() driver.RoleSet {
	return driver.RoleSet{driver.RoleImplementer: true, driver.RoleVerifier: true}
}

// Dispatch serves one role dispatch. For Role=verifier it runs the tool loop
// for investigation and then obtains the verdict via exactly one
// ChatStructured call (see inprocess_verify.go); every other declared role
// runs the plain tool loop. The returned error preserves the underlying
// *model.Error in its chain so the engine's model.IsTerminal terminal-halt
// keeps firing on the pre-S06 path (Coach ack pin 1, option (a)).
func (d *InProcess) Dispatch(ctx context.Context, in driver.DispatchInput) (driver.Result, error) {
	start := time.Now()

	// Fail-closed guards (design D7). An empty WorktreeRoot is a caller-input
	// problem, kept distinct from AssertWorktree's filesystem/git checks.
	if in.WorktreeRoot == "" {
		return driver.Result{Status: driver.StatusError, ErrKind: driver.ErrKindConfig},
			fmt.Errorf("inprocess: DispatchInput.WorktreeRoot is empty")
	}
	if err := driver.AssertWorktree(in.WorktreeRoot); err != nil {
		return driver.Result{Status: driver.StatusError, ErrKind: driver.ErrKindConfig}, err
	}

	client, err := d.newClient(in.ModelID, d.pcfg)
	if err != nil {
		return driver.Result{Status: driver.StatusError, ErrKind: driver.ErrKindConfig},
			fmt.Errorf("inprocess: resolve model %q: %w", in.ModelID, err)
	}
	ag, ok := client.(agent.Agent)
	if !ok {
		// A model ID that resolves to a client this wrapper cannot drive —
		// fails closed instead of a nil-interface panic (design D7).
		return driver.Result{Status: driver.StatusError, ErrKind: driver.ErrKindConfig},
			fmt.Errorf("inprocess: client for %q does not support multi-turn chat", in.ModelID)
	}

	timeout := in.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	meter := &chatMeter{inner: ag}

	if in.Role == driver.RoleVerifier {
		return d.dispatchVerifier(ctx, in, client, meter, start)
	}
	return d.dispatchLoop(ctx, in, meter, start)
}

// dispatchLoop runs the plain multi-turn tool loop (the implementer path,
// AC-01) and maps its outcome to a driver.Result.
func (d *InProcess) dispatchLoop(ctx context.Context, in driver.DispatchInput, meter *chatMeter, start time.Time) (driver.Result, error) {
	text, _, _, err := agent.Run(ctx, meter, in.SystemPrompt, in.Payload, in.WorktreeRoot, agent.Config{MaxTurns: d.maxTurns})
	if err != nil {
		res := d.economics(driver.Result{Status: driver.StatusError, ErrKind: classifyErr(err)}, in, meter, start)
		return res, err
	}

	res := d.economics(driver.Result{Status: driver.StatusOK, ResultText: text}, in, meter, start)
	return res, nil
}

// economics fills the dispatch-economics fields the engine records
// regardless of Status (driver.Result contract): token counts, the nominal
// cost estimate, the confirmed model ID, and wall-clock duration.
func (d *InProcess) economics(res driver.Result, in driver.DispatchInput, meter *chatMeter, start time.Time) driver.Result {
	res.InputTokens = meter.inputTokens
	res.OutputTokens = meter.outputTokens
	res.CostUSD = float64(meter.inputTokens+meter.outputTokens) * nominalCostPerToken
	res.CostSource = "estimated"
	res.ModelID = meter.modelID(in.ModelID)
	res.DurationMS = time.Since(start).Milliseconds()
	return res
}

// classifyErr maps a loop/verdict error to the contract's ErrKind
// vocabulary (design D5, Coach ack pins 1+2):
//
//   - agent.ErrMaxTurns wins first regardless of any wrapped *model.Error —
//     a max-turns exit is retryable (AC-04) → driver.ErrKindTransient.
//   - a classified *model.Error maps kind-for-kind, with KindAuth →
//     driver.ErrKindAuth as an explicit mapping (the cross-driver fail-fast
//     contract), never an incidental String() collision.
//   - anything else is "other".
func classifyErr(err error) string {
	if errors.Is(err, agent.ErrMaxTurns) {
		return driver.ErrKindTransient
	}
	var me *model.Error
	if model.AsError(err, &me) {
		return errKindFromModel(me.Kind)
	}
	return errKindOther
}

// errKindFromModel maps the model package's error taxonomy onto the driver
// contract's ErrKind strings. Shared values reuse the driver package
// constants (Coach ack pin 2).
func errKindFromModel(kind model.ErrorKind) string {
	switch kind {
	case model.KindAuth:
		return driver.ErrKindAuth
	case model.KindCredits:
		return errKindCredits
	case model.KindRateLimit:
		return errKindRateLimit
	case model.KindUpstream:
		return errKindUpstream
	case model.KindTransient:
		return driver.ErrKindTransient
	default:
		return errKindOther
	}
}

// chatMeter wraps the concrete client's Chat to accumulate token usage and
// capture the provider-confirmed model ID across turns (design D3) — pure
// observation of return values the driver already receives, zero change to
// internal/agent. It is what gets passed into agent.Run, not the raw client.
type chatMeter struct {
	inner        agent.Agent
	inputTokens  int64
	outputTokens int64
	lastModel    string
}

// Chat delegates to the wrapped client and accumulates usage.
func (m *chatMeter) Chat(ctx context.Context, messages []model.ChatMessage, tools []model.ToolDef) (*model.ChatResponse, error) {
	resp, err := m.inner.Chat(ctx, messages, tools)
	if resp != nil {
		m.observe(resp)
	}
	return resp, err
}

// observe records one response's usage and confirmed model ID. OAI-derived
// drivers populate PromptTokens/CompletionTokens; native shapes populate the
// InputTokens/OutputTokens aliases — prefer the former, fall back to the
// latter.
func (m *chatMeter) observe(resp *model.ChatResponse) {
	if resp.Usage != nil {
		in := resp.Usage.PromptTokens
		if in == 0 {
			in = resp.Usage.InputTokens
		}
		out := resp.Usage.CompletionTokens
		if out == 0 {
			out = resp.Usage.OutputTokens
		}
		m.inputTokens += int64(in)
		m.outputTokens += int64(out)
	}
	if resp.Model != "" {
		m.lastModel = resp.Model
	}
}

// modelID returns the provider-confirmed model ID from the last response,
// falling back to the requested ID so Result.ModelID is never empty.
func (m *chatMeter) modelID(fallback string) string {
	if m.lastModel != "" {
		return m.lastModel
	}
	return fallback
}
