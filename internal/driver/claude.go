package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ClaudeDriver dispatches the implementer and verifier roles by spawning the
// claude CLI as a subprocess, rooted at DispatchInput.WorktreeRoot. Each
// Dispatch call is exactly one subprocess invocation — the implementer
// role's entire agentic loop (multi-turn tool use) happens inside the claude
// process; ClaudeDriver does not orchestrate turns.
type ClaudeDriver struct {
	// Binary is the path to the claude CLI, resolved from PATH if it
	// contains no path separator. Empty defaults to "claude".
	Binary string
}

// NewClaudeDriver returns a ClaudeDriver that invokes "claude" from PATH.
func NewClaudeDriver() *ClaudeDriver {
	return &ClaudeDriver{Binary: "claude"}
}

// Name identifies this driver for logging, telemetry, and resolution.
func (d *ClaudeDriver) Name() string { return "claude-subprocess" }

// Roles declares that ClaudeDriver serves the implementer and verifier
// roles. It deliberately does not declare captain: neither this slice's user
// outcome nor its in-scope list describes captain-role dispatch.
func (d *ClaudeDriver) Roles() RoleSet {
	return RoleSet{RoleImplementer: true, RoleVerifier: true}
}

// Dispatch spawns the claude CLI once and normalizes its JSON result
// envelope into a Result. See AC-01..AC-05 in spec.json for the exact
// contract this implements.
func (d *ClaudeDriver) Dispatch(ctx context.Context, in DispatchInput) (Result, error) {
	if err := AssertWorktree(in.WorktreeRoot); err != nil {
		return Result{Status: StatusError, ErrKind: ErrKindConfig}, err
	}

	binary := d.Binary
	if binary == "" {
		binary = "claude"
	}

	args := []string{"-p", "--output-format", "json", "--model", in.ModelID}
	if in.Role == RoleVerifier {
		args = append(args, "--no-session-persistence")
	}
	args = append(args, buildPrompt(in))

	sr := spawn(ctx, binary, args, in.WorktreeRoot, in.Timeout)
	if sr.Err != nil {
		return Result{Status: StatusError, ErrKind: sr.Err.Kind, DurationMS: sr.DurationMS}, sr.Err
	}

	env, err := parseClaudeEnvelope(sr.Stdout)
	if err != nil {
		return Result{Status: StatusError, ErrKind: ErrKindProtocol, DurationMS: sr.DurationMS},
			fmt.Errorf("claude-subprocess: parse output envelope: %w", err)
	}

	result := Result{
		Status:       StatusOK,
		ResultText:   env.Result,
		CostUSD:      env.costUSD(),
		CostSource:   env.costSource(),
		InputTokens:  env.inputTokens(),
		OutputTokens: env.outputTokens(),
		ModelID:      env.modelID(in.ModelID),
		DurationMS:   env.durationMS(sr.DurationMS),
	}

	if in.Role == RoleVerifier {
		text := strings.TrimSpace(env.Result)
		if !isJSONObject(text) {
			return Result{Status: StatusError, ErrKind: ErrKindProtocol, DurationMS: result.DurationMS},
				fmt.Errorf("claude-subprocess: verifier result did not parse as a JSON object")
		}
		result.StructuredJSON = json.RawMessage(text)
	}

	return result, nil
}

// buildPrompt concatenates the system prompt and payload as claude -p's
// single prompt argument, appending the verdict schema as the required
// output contract for verifier dispatches (AC-03).
func buildPrompt(in DispatchInput) string {
	prompt := in.SystemPrompt + "\n\n" + in.Payload
	if in.Role == RoleVerifier && len(in.VerdictSchema) > 0 {
		prompt += "\n\nRespond with a single JSON object conforming to this schema:\n" + string(in.VerdictSchema)
	}
	return prompt
}

// isJSONObject reports whether s parses as a JSON value whose top-level
// type is an object (not an array, string, number, or scalar).
func isJSONObject(s string) bool {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return false
	}
	_, ok := v.(map[string]interface{})
	return ok
}

// claudeEnvelope is claude -p --output-format json's result envelope.
// TotalCostUSD and Usage are pointers so a missing field is distinguishable
// from a reported zero (R-01: defensive parsing — unknown/absent fields
// degrade gracefully rather than erroring).
type claudeEnvelope struct {
	Result       string       `json:"result"`
	TotalCostUSD *float64     `json:"total_cost_usd"`
	Usage        *claudeUsage `json:"usage"`
	DurationMS   int64        `json:"duration_ms"`
	Model        string       `json:"model"`
}

type claudeUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

func parseClaudeEnvelope(raw []byte) (*claudeEnvelope, error) {
	var env claudeEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

// reported is true when the CLI's envelope carried any cost/usage data.
func (e *claudeEnvelope) reported() bool { return e.TotalCostUSD != nil || e.Usage != nil }

func (e *claudeEnvelope) costSource() string {
	if e.reported() {
		return "provider-reported"
	}
	return "unknown"
}

func (e *claudeEnvelope) costUSD() float64 {
	if e.TotalCostUSD != nil {
		return *e.TotalCostUSD
	}
	return 0
}

func (e *claudeEnvelope) inputTokens() int64 {
	if e.Usage != nil {
		return e.Usage.InputTokens
	}
	return 0
}

func (e *claudeEnvelope) outputTokens() int64 {
	if e.Usage != nil {
		return e.Usage.OutputTokens
	}
	return 0
}

// modelID falls back to the model that was requested when the envelope
// omits its own model field, so Result.ModelID is never left empty.
func (e *claudeEnvelope) modelID(fallback string) string {
	if e.Model != "" {
		return e.Model
	}
	return fallback
}

// durationMS prefers the CLI's own reported duration, falling back to the
// wall-clock time this driver measured around the subprocess call.
func (e *claudeEnvelope) durationMS(measured int64) int64 {
	if e.DurationMS > 0 {
		return e.DurationMS
	}
	return measured
}
