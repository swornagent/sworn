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

	// The registry resolves e.g. "claude-cli/sonnet" to this driver but passes
	// the full prefixed id through; claude's --model wants the bare model
	// ("sonnet"), so "claude-cli/sonnet" is an invalid model and exits non-zero
	// (finding O, 2026-07-13 dogfood Rung 1). Strip the driver prefix.
	model := in.ModelID
	if i := strings.Index(model, "/"); i >= 0 {
		model = model[i+1:]
	}
	args := []string{"-p", "--output-format", "json", "--model", model}
	if in.Role == RoleVerifier {
		args = append(args, "--no-session-persistence")
	}
	// The prompt is a POSITIONAL operand, so it must follow a "--" end-of-options
	// separator: the verifier role prompt begins with "---" (YAML frontmatter),
	// which `claude` otherwise parses as an unknown option ("error: unknown
	// option '---…'") and exits non-zero (finding M, 2026-07-13 dogfood Rung 1).
	args = append(args, "--", buildPrompt(in))

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
		text := extractJSONObject(env.Result)
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
	if in.Role == RoleVerifier && len(in.StructuredSchema) > 0 {
		prompt += "\n\nRespond with a single JSON object conforming to this schema:\n" + string(in.StructuredSchema)
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

// parseClaudeEnvelope extracts the result envelope from `claude -p
// --output-format json` output. Current CLI versions (2.x) emit a JSON ARRAY of
// stream events — [{"type":"system"…}, {"type":"assistant"…}, {"type":"result",
// "result":…, "total_cost_usd":…}] — and the envelope is the "type":"result"
// element. Older CLIs emitted a single result object; both are supported
// (finding P, 2026-07-13 dogfood Rung 1).
func parseClaudeEnvelope(raw []byte) (*claudeEnvelope, error) {
	s := strings.TrimSpace(string(raw))
	if strings.HasPrefix(s, "[") {
		var events []json.RawMessage
		if err := json.Unmarshal([]byte(s), &events); err != nil {
			return nil, err
		}
		var resultRaw json.RawMessage
		for _, ev := range events {
			var typed struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(ev, &typed); err != nil {
				continue
			}
			if typed.Type == "result" {
				resultRaw = ev // keep the last result event
			}
		}
		if resultRaw == nil {
			return nil, fmt.Errorf("claude-subprocess: no \"type\":\"result\" event in output array")
		}
		var env claudeEnvelope
		if err := json.Unmarshal(resultRaw, &env); err != nil {
			return nil, err
		}
		return &env, nil
	}
	var env claudeEnvelope
	if err := json.Unmarshal([]byte(s), &env); err != nil {
		return nil, err
	}
	return &env, nil
}

// extractJSONObject pulls a JSON object out of a model's result text, which may
// wrap it in a ```json fence and/or trail it with prose (e.g. an insight block)
// — finding Q, 2026-07-13 dogfood Rung 1. It returns the substring from the
// first '{' to the last '}'; callers still validate it parses as an object.
func extractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}

// costSource classifies the envelope's cost data per design_decision D1
// (Coach-ratified, this slice's status.json): a positively identified,
// testable marker — a strictly positive TotalCostUSD — is the ONLY signal
// that earns CostSourceCLI. A nil TotalCostUSD (the field is absent) and an
// explicit reported zero are BOTH classified CostSourceUnknown: an explicit
// zero is not, by itself, a positively identified subscription marker — it
// is equally consistent with a genuinely free/no-cost turn or an envelope
// quirk, and claudeEnvelope carries no field that distinguishes the two.
// This deliberately does NOT implement a TotalCostUSD==0 -> "subscription"
// inference (no such marker exists in the currently observed claude-cli
// output) — ship "unknown" rather than guess (Rule 2 note: see this slice's
// proof.json not_delivered).
func (e *claudeEnvelope) costSource() string {
	if e.TotalCostUSD != nil && *e.TotalCostUSD > 0 {
		return CostSourceCLI
	}
	return CostSourceUnknown
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
