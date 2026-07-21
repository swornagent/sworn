package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

const (
	maximumNativeCodexVerifierEvents    = 4096
	maximumNativeCodexVerifierEventLine = 2 << 20
)

// NativeCodexVerifierTurn is the model-owned result recovered from one exact
// native Codex JSONL turn. It deliberately carries no engine-owned envelope or
// verdict authority.
type NativeCodexVerifierTurn struct {
	Assessment []byte
	ThreadID   string
}

// ParseNativeCodexVerifierJSONL accepts one complete fresh native Codex turn
// and returns its exact final assessment bytes and thread identity. Only item
// types required by the fixed verifier profile are admitted. Verifier profile
// schema v1 selects this grammar; the native adapter admits the executable,
// while persisted profile bytes bind that admitted binary historically.
func ParseNativeCodexVerifierJSONL(contents []byte) (NativeCodexVerifierTurn, error) {
	if len(contents) == 0 {
		return NativeCodexVerifierTurn{}, errors.New("Codex verifier JSONL output is empty")
	}
	lines := bytes.Split(contents, []byte{'\n'})
	if len(lines) > maximumNativeCodexVerifierEvents+1 {
		return NativeCodexVerifierTurn{}, errors.New("Codex verifier JSONL exceeds the event ceiling")
	}
	threadStarted, turnStarted, turnCompleted := false, false, false
	var threadID string
	var assessment []byte
	seenEvents := 0
	for index, line := range lines {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(line) == 0 {
			if index == len(lines)-1 {
				continue
			}
			return NativeCodexVerifierTurn{}, fmt.Errorf("Codex verifier JSONL event %d is empty", index+1)
		}
		if len(line) > maximumNativeCodexVerifierEventLine {
			return NativeCodexVerifierTurn{}, fmt.Errorf(
				"Codex verifier JSONL event %d exceeds the byte ceiling", index+1,
			)
		}
		if turnCompleted {
			return NativeCodexVerifierTurn{}, errors.New("Codex verifier emitted an event after turn completion")
		}
		event, err := strictNativeCodexObject(line, fmt.Sprintf("event %d", index+1))
		if err != nil {
			return NativeCodexVerifierTurn{}, err
		}
		eventType, err := nativeCodexString(event, "type")
		if err != nil {
			return NativeCodexVerifierTurn{}, fmt.Errorf("Codex verifier JSONL event %d: %w", index+1, err)
		}
		seenEvents++
		switch eventType {
		case "thread.started":
			if err := exactNativeCodexFields(event, "type", "thread_id"); err != nil {
				return NativeCodexVerifierTurn{}, err
			}
			threadID, err = nativeCodexString(event, "thread_id")
			if err != nil || !ValidID(threadID) || threadStarted || turnStarted || seenEvents != 1 {
				return NativeCodexVerifierTurn{}, errors.New("Codex verifier JSONL has an invalid thread start")
			}
			threadStarted = true
		case "turn.started":
			if err := exactNativeCodexFields(event, "type"); err != nil {
				return NativeCodexVerifierTurn{}, err
			}
			if !threadStarted || turnStarted || seenEvents != 2 {
				return NativeCodexVerifierTurn{}, errors.New("Codex verifier JSONL has an invalid turn start")
			}
			turnStarted = true
		case "item.started", "item.updated", "item.completed":
			if err := exactNativeCodexFields(event, "type", "item"); err != nil {
				return NativeCodexVerifierTurn{}, err
			}
			if !turnStarted {
				return NativeCodexVerifierTurn{}, errors.New("Codex verifier JSONL item is outside the active turn")
			}
			if assessment != nil {
				return NativeCodexVerifierTurn{}, errors.New("Codex verifier emitted an item after its final agent message")
			}
			item, err := nativeCodexObject(event, "item")
			if err != nil {
				return NativeCodexVerifierTurn{}, err
			}
			itemID, idErr := nativeCodexString(item, "id")
			itemType, typeErr := nativeCodexString(item, "type")
			if idErr != nil || typeErr != nil || itemID == "" || len(itemID) > 256 {
				return NativeCodexVerifierTurn{}, errors.New("Codex verifier JSONL contains an invalid item")
			}
			if itemType == "error" {
				return NativeCodexVerifierTurn{}, errors.New("Codex verifier JSONL reported an error item")
			}
			if !allowedNativeCodexVerifierItemType(itemType) {
				return NativeCodexVerifierTurn{}, fmt.Errorf(
					"Codex verifier JSONL contains forbidden item type %q", itemType,
				)
			}
			if itemType == "agent_message" {
				if eventType != "item.completed" {
					return NativeCodexVerifierTurn{}, errors.New("Codex verifier JSONL final agent message is not completed")
				}
				if err := exactNativeCodexFields(item, "id", "type", "text"); err != nil {
					return NativeCodexVerifierTurn{}, err
				}
				text, err := nativeCodexString(item, "text")
				if err != nil || text == "" || len(text) > MaximumVerifierAssessmentBytes {
					return NativeCodexVerifierTurn{}, errors.New(
						"Codex verifier final agent message is empty or exceeds its byte ceiling",
					)
				}
				assessment = []byte(text)
			}
		case "turn.completed":
			if err := exactNativeCodexFields(event, "type", "usage"); err != nil {
				return NativeCodexVerifierTurn{}, err
			}
			if !turnStarted || assessment == nil || turnCompleted {
				return NativeCodexVerifierTurn{}, errors.New("Codex verifier JSONL has an invalid terminal completion")
			}
			usage, err := nativeCodexObject(event, "usage")
			if err != nil || validateNativeCodexVerifierUsage(usage) != nil {
				return NativeCodexVerifierTurn{}, errors.New("Codex verifier JSONL terminal completion has invalid usage")
			}
			turnCompleted = true
		case "turn.failed", "error":
			return NativeCodexVerifierTurn{}, fmt.Errorf(
				"Codex verifier JSONL reported terminal failure %q", eventType,
			)
		default:
			return NativeCodexVerifierTurn{}, fmt.Errorf(
				"Codex verifier JSONL contains unsupported event %q", eventType,
			)
		}
	}
	if seenEvents == 0 || !threadStarted || !turnStarted || !turnCompleted || assessment == nil {
		return NativeCodexVerifierTurn{}, errors.New(
			"Codex verifier JSONL lacks one complete thread, turn, and final agent message",
		)
	}
	return NativeCodexVerifierTurn{Assessment: slices.Clone(assessment), ThreadID: threadID}, nil
}

func strictNativeCodexObject(contents []byte, label string) (map[string]json.RawMessage, error) {
	canonical, err := CanonicalizeJSON(contents)
	if err != nil {
		return nil, fmt.Errorf("decode Codex verifier JSONL %s: %w", label, err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &object); err != nil || object == nil {
		return nil, fmt.Errorf("decode Codex verifier JSONL %s as an object", label)
	}
	return object, nil
}

func nativeCodexObject(object map[string]json.RawMessage, name string) (map[string]json.RawMessage, error) {
	raw, exists := object[name]
	if !exists {
		return nil, fmt.Errorf("Codex verifier JSONL omits %q", name)
	}
	return strictNativeCodexObject(raw, name)
}

func nativeCodexString(object map[string]json.RawMessage, name string) (string, error) {
	raw, exists := object[name]
	if !exists {
		return "", fmt.Errorf("Codex verifier JSONL omits %q", name)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("Codex verifier JSONL field %q is not a string", name)
	}
	return value, nil
}

func exactNativeCodexFields(object map[string]json.RawMessage, names ...string) error {
	if len(object) != len(names) {
		return errors.New("Codex verifier JSONL object does not use its exact field shape")
	}
	for _, name := range names {
		if _, exists := object[name]; !exists {
			return fmt.Errorf("Codex verifier JSONL omits %q", name)
		}
	}
	return nil
}

func allowedNativeCodexVerifierItemType(itemType string) bool {
	switch itemType {
	case "agent_message", "reasoning", "command_execution", "todo_list":
		return true
	default:
		return false
	}
}

func validateNativeCodexVerifierUsage(usage map[string]json.RawMessage) error {
	names := []string{
		"input_tokens",
		"cached_input_tokens",
		"cache_write_input_tokens",
		"output_tokens",
		"reasoning_output_tokens",
	}
	if err := exactNativeCodexFields(usage, names...); err != nil {
		return err
	}
	for _, name := range names {
		var count int64
		if err := json.Unmarshal(usage[name], &count); err != nil || count < 0 {
			return fmt.Errorf("invalid Codex verifier usage field %q", name)
		}
	}
	return nil
}
