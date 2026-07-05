package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CodexDriver dispatches the implementer and verifier roles by spawning the
// codex CLI (`codex exec`) as a subprocess, rooted at
// DispatchInput.WorktreeRoot. Each Dispatch call is exactly one subprocess
// invocation — like ClaudeDriver, the implementer role's entire agentic
// loop happens inside the codex process; CodexDriver does not orchestrate
// turns.
//
// Envelope assumption (R-01, spec.json): this driver assumes codex exec's
// --json non-interactive mode emits one JSON object per stdout line
// (JSONL), matching the documented sample stream confirmed during this
// slice's design review (2026-07-06): a "thread.started" event opens the
// stream, "item.completed" events carry agent turns (this driver reads the
// last one whose item.type == "agent_message" as the final result text),
// and a terminal "turn.completed" event carries a "usage" object
// (input_tokens/cached_input_tokens/output_tokens/reasoning_output_tokens)
// — no "model" or "duration_ms" field at any level. This is not verified
// against a live codex binary; S10's conformance suite exercises this same
// fake, not a real one, so a real-CLI drift surfaces at SIT/cutover, not
// here (see design.md's "Risks / open items").
type CodexDriver struct {
	// Binary is the path to the codex CLI, resolved from PATH if it
	// contains no path separator. Empty defaults to "codex".
	Binary string
}

// NewCodexDriver returns a CodexDriver that invokes "codex" from PATH.
func NewCodexDriver() *CodexDriver {
	return &CodexDriver{Binary: "codex"}
}

// Name identifies this driver for logging, telemetry, and resolution.
func (d *CodexDriver) Name() string { return "codex-subprocess" }

// Roles declares that CodexDriver serves the implementer and verifier
// roles, mirroring ClaudeDriver — neither this slice's user outcome nor its
// in-scope list describes captain-role dispatch.
func (d *CodexDriver) Roles() RoleSet {
	return RoleSet{RoleImplementer: true, RoleVerifier: true}
}

// Dispatch spawns the codex CLI once and normalizes its JSONL event stream
// into a Result. See AC-01..AC-05 in spec.json for the exact contract this
// implements.
func (d *CodexDriver) Dispatch(ctx context.Context, in DispatchInput) (Result, error) {
	if err := AssertWorktree(in.WorktreeRoot); err != nil {
		return Result{Status: StatusError, ErrKind: ErrKindConfig}, err
	}

	binary := d.Binary
	if binary == "" {
		binary = "codex"
	}

	// -C roots the child at WorktreeRoot in addition to cmd.Dir (set by
	// spawnClassified) — AC-01 calls out both explicitly. --ephemeral is
	// the codex-side equivalent of claude's --no-session-persistence
	// (design.md decision 1, resolved at design review): it avoids
	// persisting session rollout files to disk for the fresh-context
	// verifier role (Rule 7).
	args := []string{"exec", "--json", "-C", in.WorktreeRoot}
	if in.Role == RoleVerifier {
		args = append(args, "--ephemeral")
	}
	args = append(args, buildPrompt(in))

	// Non-zero exit maps to ErrKindAuth, matching the claude driver — the
	// binding cross-driver contract (design.md decision 6).
	sr := spawnClassified(ctx, binary, args, in.WorktreeRoot, in.Timeout, ErrKindAuth)
	if sr.Err != nil {
		return Result{Status: StatusError, ErrKind: sr.Err.Kind, DurationMS: sr.DurationMS}, sr.Err
	}

	env, err := parseCodexEnvelope(sr.Stdout)
	if err != nil {
		return Result{Status: StatusError, ErrKind: ErrKindProtocol, DurationMS: sr.DurationMS},
			fmt.Errorf("codex-subprocess: parse output stream: %w", err)
	}

	result := Result{
		Status:       StatusOK,
		ResultText:   env.ResultText,
		CostUSD:      0,
		CostSource:   env.costSource(),
		InputTokens:  env.inputTokens(),
		OutputTokens: env.outputTokens(),
		ModelID:      env.modelID(in.ModelID),
		DurationMS:   env.durationMS(sr.DurationMS),
	}

	if in.Role == RoleVerifier {
		text := strings.TrimSpace(result.ResultText)
		if !isJSONObject(text) {
			return Result{Status: StatusError, ErrKind: ErrKindProtocol, DurationMS: result.DurationMS},
				fmt.Errorf("codex-subprocess: verifier result did not parse as a JSON object")
		}
		result.StructuredJSON = json.RawMessage(text)
	}

	return result, nil
}

// codexEvent is one line of codex exec --json's JSONL event stream.
type codexEvent struct {
	Type  string      `json:"type"`
	Item  *codexItem  `json:"item"`
	Usage *codexUsage `json:"usage"`
}

// codexItem is an "item.completed" event's payload. Only item.type ==
// "agent_message" is treated as the driver's ResultText — other item types
// (e.g. tool calls, reasoning) are not part of this contract.
type codexItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// codexUsage is a "turn.completed" event's usage object, per the documented
// sample confirmed at design review: no model or duration fields anywhere
// in the stream, only these four token counts. Result has no slot for
// CachedInputTokens/ReasoningOutputTokens (AC-04 only requires InputTokens/
// OutputTokens), but they're decoded here for fidelity to the documented
// shape rather than silently dropped by the JSON decoder.
type codexUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
}

// codexEnvelope is the accumulated result of scanning a codex JSONL event
// stream: the last agent_message text seen, and the final turn's usage
// (nil if the stream never emitted a turn.completed event).
type codexEnvelope struct {
	ResultText string
	Usage      *codexUsage
}

// reported is true when the stream carried a turn.completed usage object.
func (e *codexEnvelope) reported() bool { return e.Usage != nil }

func (e *codexEnvelope) costSource() string {
	if e.reported() {
		return "provider-reported"
	}
	return "unknown"
}

func (e *codexEnvelope) inputTokens() int64 {
	if e.Usage != nil {
		return e.Usage.InputTokens
	}
	return 0
}

func (e *codexEnvelope) outputTokens() int64 {
	if e.Usage != nil {
		return e.Usage.OutputTokens
	}
	return 0
}

// modelID always falls back to the requested model — codex's JSONL stream
// never reports a model field (R-01 assumption, confirmed at design
// review). This is the normal path for codex, not a rare edge case.
func (e *codexEnvelope) modelID(fallback string) string { return fallback }

// durationMS always falls back to the measured wall-clock time — codex's
// JSONL stream never reports a duration field (R-01 assumption, confirmed
// at design review). This is the normal path for codex, not a rare edge
// case.
func (e *codexEnvelope) durationMS(measured int64) int64 { return measured }

// parseCodexEnvelope scans raw as newline-delimited JSON events, returning
// the last agent_message text and the final usage object seen. A line that
// fails to parse as JSON is a hard failure (ErrKind=protocol at the call
// site) — mirrors claude.go's outer-envelope-protocol-error principle,
// applied per line instead of to a single envelope. A stream with no
// recognisable event lines at all is also a hard failure; a well-formed
// stream that never emits an agent_message leaves ResultText empty rather
// than erroring (nothing to fabricate).
func parseCodexEnvelope(raw []byte) (*codexEnvelope, error) {
	env := &codexEnvelope{}
	sawAny := false
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var evt codexEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			return nil, fmt.Errorf("parse event line %q: %w", line, err)
		}
		sawAny = true
		switch evt.Type {
		case "item.completed":
			if evt.Item != nil && evt.Item.Type == "agent_message" {
				env.ResultText = evt.Item.Text
			}
		case "turn.completed":
			if evt.Usage != nil {
				env.Usage = evt.Usage
			}
		}
	}
	if !sawAny {
		return nil, fmt.Errorf("no JSON event lines found in codex output")
	}
	return env, nil
}
