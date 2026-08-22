package driver

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// geminiStreamFormat names the generateContent SSE dialect on providerRequest.
// The responses-flavour reader stays the default (empty format), so every
// other adapter's behavior is untouched.
const geminiStreamFormat = "gemini"

// readGeminiStream consumes one streamGenerateContent SSE stream, renders
// every delta live through the shared stream renderer, and returns one
// reconstructed terminal GenerateContentResponse object. The reconstruction
// carries every chunk field through — unknowns included — so accept() stays
// the single closed-validation authority: anything the stream carried is
// classified exactly as the same bytes on the non-streaming path. Chunks are
// merged to EOF with usageMetadata taken last-seen (never summed, and never
// used as a terminal marker); the terminal condition is a candidate
// finishReason exactly as accept() requires it, and a stream that ends
// without one fails PROVIDER_TRANSPORT_FAILED under the responses reader's
// missing-terminal doctrine.
func readGeminiStream(body io.Reader, maximumBytes int, model string) ([]byte, error) {
	scanner := bufio.NewScanner(io.LimitReader(body, MaxStreamBytes+1))
	scanner.Buffer(make([]byte, 0, 64*1024), MaxStreamBytes)
	var data bytes.Buffer
	total := 0
	var merged map[string]any
	headerEmitted := false
	flush := func() error {
		if data.Len() == 0 {
			return nil
		}
		payload := append([]byte(nil), data.Bytes()...)
		data.Reset()
		// Syntax-level admission only (UTF-8, JSON, duplicates, escapes): the
		// raw-stream bound is the guard here, closed validation stays in
		// accept(), and the reconstructed terminal object is bounded at
		// maximumBytes at the end — the same doctrine as the responses
		// reader.
		value, err := decodeStrict(payload, MaxStreamBytes)
		if err != nil {
			return err
		}
		chunk, ok := value.(map[string]any)
		if !ok {
			return failContinuation("continuation.stream.gemini_chunk_invalid")
		}
		if !headerEmitted {
			turnEvent, _ := json.Marshal(struct {
				Model string `json:"model"`
			}{model})
			liveStream.event("gemini.turn", turnEvent)
			headerEmitted = true
		}
		renderGeminiChunk(chunk)
		merged = mergeGeminiChunk(merged, chunk)
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
	if merged == nil || !geminiFinishReasonPresent(merged) {
		return nil, fail("PROVIDER_TRANSPORT_FAILED")
	}
	liveStream.event("gemini.completed", geminiSummaryEvent(merged))
	terminal, err := json.Marshal(merged)
	if err != nil {
		return nil, fail("RESOURCE_LIMIT")
	}
	if len(terminal) > maximumBytes {
		return nil, fail("OUTPUT_OVERFLOW")
	}
	return terminal, nil
}

// renderGeminiChunk renders one SSE chunk's parts live. It is presentation
// only: malformed shapes are skipped and rendering failures are ignored,
// exactly like the responses renderer. Thought text rides the reasoning
// channel and visible text the plain channel, so gemini deliberation reads
// like deepseek and qwen deliberation already does.
func renderGeminiChunk(chunk map[string]any) {
	candidate := geminiChunkCandidate(chunk)
	if candidate == nil {
		return
	}
	content, ok := candidate["content"].(map[string]any)
	if !ok {
		return
	}
	parts, ok := content["parts"].([]any)
	if !ok {
		return
	}
	for _, rawPart := range parts {
		part, ok := rawPart.(map[string]any)
		if !ok {
			continue
		}
		if text, ok := part["text"].(string); ok {
			thought, _ := part["thought"].(bool)
			if thought {
				liveStream.event("gemini.reasoning.delta", geminiDeltaEvent(text))
			} else {
				liveStream.event("gemini.text.delta", geminiDeltaEvent(text))
			}
			continue
		}
		call, ok := part["functionCall"].(map[string]any)
		if !ok {
			continue
		}
		name, _ := call["name"].(string)
		callEvent, _ := json.Marshal(struct {
			Name string `json:"name"`
		}{name})
		liveStream.event("gemini.function_call", callEvent)
		if args, present := call["args"]; present {
			if rendered, err := json.Marshal(args); err == nil {
				liveStream.event(
					"gemini.function_call.arguments.delta",
					geminiDeltaEvent(string(rendered)),
				)
			}
		}
		liveStream.event("gemini.function_call.arguments.done", nil)
	}
}

func geminiDeltaEvent(text string) []byte {
	event, _ := json.Marshal(struct {
		Delta string `json:"delta"`
	}{text})
	return event
}

// geminiChunkCandidate returns the first candidate of a chunk, or nil when
// the chunk carries no candidates (e.g. a usage-only chunk).
func geminiChunkCandidate(chunk map[string]any) map[string]any {
	candidates, ok := chunk["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		return nil
	}
	candidate, _ := candidates[0].(map[string]any)
	return candidate
}

// geminiFinishReasonPresent reports whether the reconstructed object carries
// a candidate finishReason — the only terminal condition accept() requires.
func geminiFinishReasonPresent(merged map[string]any) bool {
	candidate := geminiChunkCandidate(merged)
	if candidate == nil {
		return false
	}
	_, present := candidate["finishReason"]
	return present
}

// geminiSummaryEvent renders the live completion line from the reconstructed
// usage (last-seen) and finish reason.
func geminiSummaryEvent(merged map[string]any) []byte {
	type geminiSummaryUsage struct {
		PromptTokens     int64  `json:"prompt_tokens"`
		CandidatesTokens int64  `json:"candidates_tokens"`
		CachedTokens     *int64 `json:"cached_tokens,omitempty"`
		ThoughtsTokens   *int64 `json:"thoughts_tokens,omitempty"`
	}
	summary := struct {
		FinishReason string              `json:"finish_reason"`
		Usage        *geminiSummaryUsage `json:"usage,omitempty"`
	}{}
	if candidate := geminiChunkCandidate(merged); candidate != nil {
		summary.FinishReason, _ = candidate["finishReason"].(string)
	}
	if raw, present := merged["usageMetadata"]; present {
		if metadata, ok := raw.(map[string]any); ok {
			usage := &geminiSummaryUsage{}
			if value, ok := safeJSONInt(metadata["promptTokenCount"]); ok {
				usage.PromptTokens = value
			}
			if value, ok := safeJSONInt(metadata["candidatesTokenCount"]); ok {
				usage.CandidatesTokens = value
			}
			if value, ok := safeJSONInt(metadata["cachedContentTokenCount"]); ok {
				usage.CachedTokens = &value
			}
			if value, ok := safeJSONInt(metadata["thoughtsTokenCount"]); ok {
				usage.ThoughtsTokens = &value
			}
			summary.Usage = usage
		}
	}
	event, _ := json.Marshal(summary)
	return event
}

// mergeGeminiChunk folds one chunk into the reconstruction. usageMetadata and
// every other root field are taken last-seen (the provider's final accounting
// is authoritative, never summed); candidates merge structurally.
func mergeGeminiChunk(merged map[string]any, chunk map[string]any) map[string]any {
	if merged == nil {
		merged = make(map[string]any, len(chunk))
	}
	for key, value := range chunk {
		if key == "candidates" {
			merged[key] = mergeGeminiCandidates(merged[key], value)
			continue
		}
		merged[key] = value
	}
	return merged
}

func mergeGeminiCandidates(existing, incoming any) any {
	oldArray, oldOK := existing.([]any)
	newArray, newOK := incoming.([]any)
	if !oldOK || !newOK {
		// Carried through untouched: accept() classifies a non-array
		// candidates field exactly as on the non-streaming path.
		return incoming
	}
	merged := append([]any(nil), oldArray...)
	if len(newArray) == 0 {
		return merged
	}
	if len(merged) == 0 {
		merged = append(merged, newArray[0])
	} else {
		oldCandidate, oldMap := merged[0].(map[string]any)
		newCandidate, newMap := newArray[0].(map[string]any)
		if oldMap && newMap {
			merged[0] = mergeGeminiCandidate(oldCandidate, newCandidate)
		} else {
			merged[0] = newArray[0]
		}
	}
	// Extra candidates ride along untouched so accept()'s exactly-one
	// candidate rule classifies them as it classifies unstreamed bytes.
	return append(merged, newArray[1:]...)
}

func mergeGeminiCandidate(existing, incoming map[string]any) map[string]any {
	for key, value := range incoming {
		if key == "content" {
			existing[key] = mergeGeminiContent(existing[key], value)
			continue
		}
		existing[key] = value
	}
	return existing
}

func mergeGeminiContent(existing, incoming any) any {
	oldObject, oldOK := existing.(map[string]any)
	newObject, newOK := incoming.(map[string]any)
	if !oldOK || !newOK {
		return incoming
	}
	for key, value := range newObject {
		if key == "parts" {
			oldObject[key] = mergeGeminiParts(oldObject[key], value)
			continue
		}
		oldObject[key] = value
	}
	return oldObject
}

func mergeGeminiParts(existing, incoming any) any {
	oldArray, oldOK := existing.([]any)
	newArray, newOK := incoming.([]any)
	if !oldOK || !newOK {
		return incoming
	}
	merged := append([]any(nil), oldArray...)
	for _, rawPart := range newArray {
		merged = appendGeminiPart(merged, rawPart)
	}
	return merged
}

// appendGeminiPart folds one arriving part into the reconstructed part list.
// Only adjacent same-kind text parts merge (thought text never merges with
// visible text, and functionCall parts never merge), so chunk granularity can
// never trip accept's part-count bound on a long stream. Every other shape is
// carried through untouched and left for accept() to classify.
func appendGeminiPart(parts []any, raw any) []any {
	part, ok := raw.(map[string]any)
	if !ok {
		return append(parts, raw)
	}
	textValue, hasText := part["text"]
	_, hasCall := part["functionCall"]
	if hasText == hasCall {
		return append(parts, part)
	}
	if hasCall {
		return append(parts, part)
	}
	text, ok := textValue.(string)
	if !ok {
		return append(parts, part)
	}
	thought, _ := part["thought"].(bool)
	if len(parts) > 0 {
		if last, ok := parts[len(parts)-1].(map[string]any); ok {
			lastTextValue, lastHasText := last["text"]
			_, lastHasCall := last["functionCall"]
			lastThought, _ := last["thought"].(bool)
			if lastHasText && !lastHasCall && lastThought == thought {
				if lastText, ok := lastTextValue.(string); ok {
					last["text"] = lastText + text
					for key, value := range part {
						if key != "text" {
							last[key] = value
						}
					}
					return parts
				}
			}
		}
	}
	return append(parts, part)
}
