package driver

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// MaxStreamBytes bounds the raw SSE byte stream for one turn. Deltas
// duplicate the terminal response's content, so the guard sits well above
// MaxProviderResponseBytes without ever binding an honest stream.
const MaxStreamBytes = 16_777_216

// streamRenderer writes a live, human-readable rendering of provider SSE
// events (Responses-API and generateContent dialects) to stderr. It is
// presentation only: nothing here feeds validation, and rendering failures
// are ignored.
type streamRenderer struct {
	mu        sync.Mutex
	out       io.Writer
	color     bool
	inThought bool
	inText    bool
}

var liveStream = newStreamRenderer()

func newStreamRenderer() *streamRenderer {
	renderer := &streamRenderer{out: os.Stderr}
	if info, err := os.Stderr.Stat(); err == nil {
		renderer.color = info.Mode()&os.ModeCharDevice != 0
	}
	return renderer
}

func (renderer *streamRenderer) dim(text string) string {
	if renderer.color {
		return "\x1b[2m" + text + "\x1b[0m"
	}
	return text
}

func (renderer *streamRenderer) bold(text string) string {
	if renderer.color {
		return "\x1b[1m" + text + "\x1b[0m"
	}
	return text
}

func (renderer *streamRenderer) breakFlow() {
	if renderer.inThought || renderer.inText {
		fmt.Fprintln(renderer.out)
		renderer.inThought, renderer.inText = false, false
	}
}

func (renderer *streamRenderer) event(name string, data []byte) {
	renderer.mu.Lock()
	defer renderer.mu.Unlock()
	switch name {
	case "response.created":
		var envelope struct {
			Response struct {
				Model     string `json:"model"`
				Reasoning struct {
					Effort string `json:"effort"`
				} `json:"reasoning"`
			} `json:"response"`
		}
		_ = json.Unmarshal(data, &envelope)
		renderer.breakFlow()
		fmt.Fprintf(renderer.out, "%s\n", renderer.bold(fmt.Sprintf(
			"── %s turn (effort %s) ──",
			envelope.Response.Model,
			envelope.Response.Reasoning.Effort,
		)))
	case "response.reasoning_text.delta", "response.reasoning_summary_text.delta",
		"gemini.reasoning.delta":
		var event struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal(data, &event) == nil && event.Delta != "" {
			if !renderer.inThought {
				renderer.breakFlow()
				fmt.Fprint(renderer.out, renderer.dim("· "))
				renderer.inThought = true
			}
			fmt.Fprint(renderer.out, renderer.dim(event.Delta))
		}
	case "response.output_text.delta", "gemini.text.delta":
		var event struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal(data, &event) == nil && event.Delta != "" {
			if !renderer.inText {
				renderer.breakFlow()
				renderer.inText = true
			}
			fmt.Fprint(renderer.out, event.Delta)
		}
	case "response.output_item.added":
		var event struct {
			Item struct {
				Type string `json:"type"`
				Name string `json:"name"`
			} `json:"item"`
		}
		if json.Unmarshal(data, &event) == nil &&
			event.Item.Type == "function_call" {
			renderer.breakFlow()
			fmt.Fprintf(
				renderer.out,
				"%s ",
				renderer.bold("⚙ "+event.Item.Name),
			)
		}
	case "response.function_call_arguments.delta":
		var event struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal(data, &event) == nil {
			fmt.Fprint(renderer.out, event.Delta)
		}
	case "response.function_call_arguments.done":
		renderer.breakFlow()
		fmt.Fprintln(renderer.out)
	case "gemini.turn":
		var envelope struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(data, &envelope)
		renderer.breakFlow()
		fmt.Fprintf(
			renderer.out,
			"%s\n",
			renderer.bold(fmt.Sprintf("── %s turn ──", envelope.Model)),
		)
	case "gemini.function_call":
		var event struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(data, &event) == nil && event.Name != "" {
			renderer.breakFlow()
			fmt.Fprintf(
				renderer.out,
				"%s ",
				renderer.bold("⚙ "+event.Name),
			)
		}
	case "gemini.function_call.arguments.delta":
		var event struct {
			Delta string `json:"delta"`
		}
		if json.Unmarshal(data, &event) == nil {
			fmt.Fprint(renderer.out, event.Delta)
		}
	case "gemini.function_call.arguments.done":
		renderer.breakFlow()
		fmt.Fprintln(renderer.out)
	case "gemini.completed":
		var envelope struct {
			FinishReason string `json:"finish_reason"`
			Usage        *struct {
				PromptTokens     int64  `json:"prompt_tokens"`
				CandidatesTokens int64  `json:"candidates_tokens"`
				CachedTokens     *int64 `json:"cached_tokens"`
				ThoughtsTokens   *int64 `json:"thoughts_tokens"`
			} `json:"usage"`
		}
		_ = json.Unmarshal(data, &envelope)
		renderer.breakFlow()
		summary := "status=" + envelope.FinishReason
		if usage := envelope.Usage; usage != nil {
			summary += fmt.Sprintf(
				" in=%d out=%d",
				usage.PromptTokens,
				usage.CandidatesTokens,
			)
			if usage.CachedTokens != nil {
				summary += fmt.Sprintf(" cached=%d", *usage.CachedTokens)
			}
			if usage.ThoughtsTokens != nil {
				summary += fmt.Sprintf(" reasoning=%d", *usage.ThoughtsTokens)
			}
		}
		fmt.Fprintf(renderer.out, "%s\n", renderer.dim("── "+summary+" ──"))
	case "response.completed", "response.incomplete", "response.failed":
		var envelope struct {
			Response struct {
				Status string `json:"status"`
				Usage  *struct {
					InputTokens       int64 `json:"input_tokens"`
					OutputTokens      int64 `json:"output_tokens"`
					InputTokenDetails *struct {
						CachedTokens int64 `json:"cached_tokens"`
					} `json:"input_tokens_details"`
					OutputTokenDetails *struct {
						ReasoningTokens int64 `json:"reasoning_tokens"`
					} `json:"output_tokens_details"`
				} `json:"usage"`
			} `json:"response"`
		}
		_ = json.Unmarshal(data, &envelope)
		renderer.breakFlow()
		summary := "status=" + envelope.Response.Status
		if usage := envelope.Response.Usage; usage != nil {
			summary += fmt.Sprintf(
				" in=%d out=%d",
				usage.InputTokens,
				usage.OutputTokens,
			)
			if usage.InputTokenDetails != nil {
				summary += fmt.Sprintf(
					" cached=%d",
					usage.InputTokenDetails.CachedTokens,
				)
			}
			if usage.OutputTokenDetails != nil {
				summary += fmt.Sprintf(
					" reasoning=%d",
					usage.OutputTokenDetails.ReasoningTokens,
				)
			}
		}
		fmt.Fprintf(renderer.out, "%s\n", renderer.dim("── "+summary+" ──"))
	}
}

// driverError surfaces a driver-side rejection into the live stream so an
// operator watching the log sees why a turn died, not just that it stopped.
func (renderer *streamRenderer) driverError(stage string, err error) {
	renderer.mu.Lock()
	defer renderer.mu.Unlock()
	renderer.breakFlow()
	detail := "unspecified"
	if err != nil {
		detail = err.Error()
	}
	fmt.Fprintf(
		renderer.out,
		"%s\n",
		renderer.bold("── driver "+stage+" error: "+detail+" ──"),
	)
}

// readStreamedResponse consumes one SSE stream, renders every event live,
// and returns the terminal event's embedded response object. The stream
// ends at response.completed / response.incomplete / response.failed; there
// is no data: [DONE] sentinel in this dialect.
func readStreamedResponse(body io.Reader, maximumBytes int) ([]byte, error) {
	scanner := bufio.NewScanner(io.LimitReader(body, MaxStreamBytes+1))
	scanner.Buffer(make([]byte, 0, 64*1024), MaxStreamBytes)
	var eventName string
	var data bytes.Buffer
	var terminal []byte
	total := 0
	flush := func() error {
		if eventName == "" && data.Len() == 0 {
			return nil
		}
		payload := append([]byte(nil), data.Bytes()...)
		name := eventName
		eventName = ""
		data.Reset()
		if name == "" || len(payload) == 0 {
			return nil
		}
		liveStream.event(name, payload)
		switch name {
		case "response.completed", "response.incomplete", "response.failed":
			var envelope struct {
				Response json.RawMessage `json:"response"`
			}
			if json.Unmarshal(payload, &envelope) != nil ||
				len(envelope.Response) == 0 {
				return failContinuation("continuation.stream.response_envelope_invalid")
			}
			if len(envelope.Response) > maximumBytes {
				return fail("OUTPUT_OVERFLOW")
			}
			terminal = append([]byte(nil), envelope.Response...)
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		total += len(line) + 1
		if total > MaxStreamBytes {
			return nil, fail("OUTPUT_OVERFLOW")
		}
		switch {
		case line == "":
			if err := flush(); err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fail("PROVIDER_TRANSPORT_FAILED")
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(terminal) == 0 {
		return nil, fail("PROVIDER_TRANSPORT_FAILED")
	}
	return terminal, nil
}
