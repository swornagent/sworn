package driver

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

const (
	YieldSchemaVersion   = "sworn.yield/v1"
	MaxYieldMessageBytes = 8_192
)

const swornYieldInputSchema = `{"type":"object","properties":{"yield":{"type":"object","properties":{"schema_version":{"type":"string","enum":["sworn.yield/v1"]},"invocation_id":{"type":"string"},"kind":{"type":"string","enum":["question","blocked","human_choice","human_confirmation"]},"message":{"type":"string"}},"required":["schema_version","invocation_id","kind","message"],"additionalProperties":false}},"required":["yield"],"additionalProperties":false}`

type YieldKind string

const (
	YieldQuestion YieldKind = "question"
	YieldBlocked  YieldKind = "blocked"
	// Human-only yields are durable operator turn boundaries. Recovery
	// automation and Captain advisory must never answer them.
	YieldHumanChoice       YieldKind = "human_choice"
	YieldHumanConfirmation YieldKind = "human_confirmation"
)

// Yield is a non-authoritative worker terminal. It can ask for help or report
// a real block, but it cannot carry a Baton responsibility, decision, receipt,
// or submission permission.
type Yield struct {
	SchemaVersion string    `json:"schema_version"`
	InvocationID  string    `json:"invocation_id"`
	Kind          YieldKind `json:"kind"`
	Message       string    `json:"message"`
}

func EncodeYield(value Yield) ([]byte, error) {
	if err := ValidateYield(value); err != nil {
		return nil, err
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fail("INVALID_JSON")
	}
	return append(body, '\n'), nil
}

func DecodeYield(body []byte) (Yield, error) {
	if len(body) < 2 || len(body) > MaxYieldMessageBytes+1_024 ||
		body[len(body)-1] != '\n' {
		return Yield{}, fail("INVALID_YIELD")
	}
	var value Yield
	if _, err := decodeTyped(
		body,
		MaxYieldMessageBytes+1_024,
		[]string{"schema_version", "invocation_id", "kind", "message"},
		nil,
		&value,
	); err != nil {
		return Yield{}, err
	}
	if err := ValidateYield(value); err != nil {
		return Yield{}, err
	}
	canonical, err := EncodeYield(value)
	if err != nil {
		return Yield{}, err
	}
	if !bytes.Equal(canonical, body) {
		return Yield{}, fail("NONCANONICAL_JSON")
	}
	return value, nil
}

func ValidateYield(value Yield) error {
	if value.SchemaVersion != YieldSchemaVersion {
		return fail("INVALID_VERSION")
	}
	if err := validateIdentity(value.InvocationID); err != nil {
		return err
	}
	if value.Kind != YieldQuestion && value.Kind != YieldBlocked &&
		value.Kind != YieldHumanChoice &&
		value.Kind != YieldHumanConfirmation {
		return fail("INVALID_YIELD_KIND")
	}
	if !utf8.ValidString(value.Message) ||
		len([]byte(value.Message)) > MaxYieldMessageBytes ||
		strings.TrimSpace(value.Message) == "" ||
		strings.ContainsRune(value.Message, '\x00') ||
		strings.ContainsRune(value.Message, '\r') {
		return fail("INVALID_YIELD_MESSAGE")
	}
	return nil
}

func decodeToolYield(value any) (Yield, error) {
	root, err := closedObject(
		value,
		[]string{"schema_version", "invocation_id", "kind", "message"},
		nil,
	)
	if err != nil {
		return Yield{}, err
	}
	body, err := canonicalJSON(root)
	if err != nil {
		return Yield{}, err
	}
	defer clearBytes(body)
	var result Yield
	if json.Unmarshal(body, &result) != nil {
		return Yield{}, fail("INVALID_YIELD")
	}
	if err := ValidateYield(result); err != nil {
		return Yield{}, err
	}
	return result, nil
}
