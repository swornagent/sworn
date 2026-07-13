package inprocess

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/model"
)

// verdictNudge is the final user turn appended after the investigation
// transcript, prompting the model to emit the verdict object the
// ChatStructured call constrains it to. The schema itself travels on the
// wire as the structured-output constraint (response_format / forced tool),
// not as prose.
const verdictNudge = "Investigation complete. Now emit your verdict as a single JSON object conforming to the required schema. Output the JSON object only."

// dispatchVerifier serves a Role=verifier dispatch (AC-02): first run the
// multi-turn tool loop for investigation — the in-process verifier can
// re-run tests and read live repo state (sworn#55) — then obtain the verdict
// via exactly ONE ChatStructured call over the accumulated transcript
// against DispatchInput.StructuredSchema. The driver returns the emitted JSON
// unmodified in Result.StructuredJSON and never validates or self-certifies
// it; the ENGINE validates it against verifier-verdict-v1, fail-closed.
func (d *InProcess) dispatchVerifier(ctx context.Context, in driver.DispatchInput, client model.Verifier, meter *chatMeter, start time.Time) (driver.Result, error) {
	// Fail-closed by construction (design D7): a client that can chat but
	// cannot emit a structured verdict is rejected before the investigation
	// loop spends any tokens — same ErrKind bucket as a structured-emission
	// failure.
	so, ok := client.(model.StructuredOutput)
	if !ok {
		return driver.Result{Status: driver.StatusError, ErrKind: driver.ErrKindProtocol},
			fmt.Errorf("inprocess: client for %q does not support structured output", in.ModelID)
	}

	// Investigation loop. Errors here classify exactly as the implementer
	// path's do (AC-04: max-turns → transient; classified provider errors
	// keep their kind; the *model.Error stays in the returned chain so
	// model.IsTerminal keeps firing — Coach ack pin 1).
	text, transcript, err := agent.Run(ctx, meter, in.SystemPrompt, in.Payload, in.WorktreeRoot, agent.Config{MaxTurns: d.maxTurns})
	if err != nil {
		res := d.economics(driver.Result{Status: driver.StatusError, ErrKind: classifyErr(err)}, in, meter, start)
		return res, err
	}

	// One structured verdict call over the accumulated transcript.
	messages := append(toModelMessages(transcript), model.ChatMessage{Role: "user", Content: verdictNudge})
	vresp, err := so.ChatStructured(ctx, messages, in.StructuredSchema)
	if vresp != nil {
		meter.observe(vresp)
	}
	if err != nil {
		res := d.economics(driver.Result{Status: driver.StatusError, ErrKind: classifyVerdictErr(err)}, in, meter, start)
		return res, fmt.Errorf("inprocess: verdict emission: %w", err)
	}
	if len(vresp.Choices) == 0 {
		// ChatStructured guards this on the happy path; checked explicitly
		// so the driver can never panic on an unchecked index (AC-04).
		res := d.economics(driver.Result{Status: driver.StatusError, ErrKind: driver.ErrKindProtocol}, in, meter, start)
		return res, fmt.Errorf("inprocess: verdict emission: empty choices")
	}

	verdict := vresp.Choices[0].Message.Content
	res := d.economics(driver.Result{
		Status:         driver.StatusOK,
		ResultText:     text,
		StructuredJSON: json.RawMessage(verdict),
	}, in, meter, start)
	return res, nil
}

// classifyVerdictErr maps a failure of the structured verdict call (AC-04,
// as narrowed by Coach ack pin 6): a CLASSIFIED provider error — auth,
// credits, rate-limit, upstream, transient — keeps its real ErrKind, because
// a transport auth failure is not a protocol failure and folding it into
// "protocol" would hide the fail-fast signal pin 1 protects. Everything that
// would otherwise classify as "other" (empty choices, content failing the
// non-empty/valid-JSON-object guard, an unsupported structured mode) is a
// structured-emission failure → driver.ErrKindProtocol.
func classifyVerdictErr(err error) string {
	var me *model.Error
	if model.AsError(err, &me) && me.Kind != model.KindOther {
		return errKindFromModel(me.Kind)
	}
	return driver.ErrKindProtocol
}

// toModelMessages converts the agent package's transcript type into the wire
// messages ChatStructured consumes (design D4): role/content/tool-call-id
// passthrough, with agent.ToolCall wrapped back into the OpenAI tool_calls
// shape.
func toModelMessages(msgs []agent.Message) []model.ChatMessage {
	out := make([]model.ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		cm := model.ChatMessage{Role: m.Role, Content: m.Content}
		if m.ToolCallID != "" {
			id := m.ToolCallID
			cm.ToolCallID = &id
		}
		for _, tc := range m.ToolCalls {
			cm.ToolCalls = append(cm.ToolCalls, model.ToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: model.FunctionCall{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
		out = append(out, cm)
	}
	return out
}
