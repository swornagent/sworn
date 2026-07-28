package driver

import (
	"bytes"
	"encoding/base64"
	"unicode/utf8"
)

const (
	MaxContinuationSteps        = 32
	MaxCorrelationIDBytes       = 256
	MaxOpaqueFieldBytes         = 262_144
	MaxOpaqueStepBytes          = 524_288
	MaxOpaqueInvocationBytes    = 1_048_576
	MaxDecodedOpaqueBinaryBytes = 196_608
)

type opaqueKind uint8

const (
	opaqueText opaqueKind = iota + 1
	opaqueBase64
)

type opaqueField struct {
	kind opaqueKind
	body []byte
}

// continuationLedger owns provider-required replay bytes for exactly one
// invocation. It has no encoding or observation method by design.
type continuationLedger struct {
	steps  int
	total  int
	ids    map[string]struct{}
	fields [][]byte
	closed bool
}

func newContinuationLedger() *continuationLedger {
	return &continuationLedger{ids: make(map[string]struct{})}
}

func (ledger *continuationLedger) retain(fields ...opaqueField) ([][]byte, error) {
	if ledger == nil || ledger.closed || len(fields) == 0 ||
		ledger.steps >= MaxContinuationSteps {
		return nil, fail("CONTINUATION_INVALID")
	}
	stepBytes := 0
	retained := make([][]byte, len(fields))
	for index, field := range fields {
		if len(field.body) > MaxOpaqueFieldBytes {
			clearRetained(retained)
			return nil, fail("CONTINUATION_INVALID")
		}
		switch field.kind {
		case opaqueText:
			if !validOpaqueText(field.body) {
				clearRetained(retained)
				return nil, fail("CONTINUATION_INVALID")
			}
		case opaqueBase64:
			decoded := make([]byte, base64.StdEncoding.DecodedLen(len(field.body)))
			count, err := base64.StdEncoding.Strict().Decode(decoded, field.body)
			if err != nil || count > MaxDecodedOpaqueBinaryBytes ||
				!bytes.Equal(
					[]byte(base64.StdEncoding.EncodeToString(decoded[:count])),
					field.body,
				) {
				clearBytes(decoded)
				clearRetained(retained)
				return nil, fail("CONTINUATION_INVALID")
			}
			clearBytes(decoded)
		default:
			clearRetained(retained)
			return nil, fail("CONTINUATION_INVALID")
		}
		stepBytes += len(field.body)
		if stepBytes > MaxOpaqueStepBytes ||
			ledger.total > MaxOpaqueInvocationBytes-stepBytes {
			clearRetained(retained)
			return nil, fail("CONTINUATION_INVALID")
		}
		retained[index] = append([]byte(nil), field.body...)
	}
	ledger.steps++
	ledger.total += stepBytes
	ledger.fields = append(ledger.fields, retained...)
	return retained, nil
}

func (ledger *continuationLedger) correlate(id string) error {
	if ledger == nil || ledger.closed ||
		validateText(id, MaxCorrelationIDBytes, false) != nil {
		return fail("CONTINUATION_INVALID")
	}
	if _, duplicate := ledger.ids[id]; duplicate {
		return fail("CONTINUATION_INVALID")
	}
	ledger.ids[id] = struct{}{}
	return nil
}

func (ledger *continuationLedger) Close() {
	if ledger == nil || ledger.closed {
		return
	}
	ledger.closed = true
	clearRetained(ledger.fields)
	ledger.fields = nil
	clear(ledger.ids)
	ledger.total = 0
	ledger.steps = 0
}

func validOpaqueText(body []byte) bool {
	if !utf8.Valid(body) {
		return false
	}
	for _, character := range string(body) {
		if character == 0 || (character < 0x20 &&
			character != '\n' && character != '\r' && character != '\t') ||
			(character >= 0x7f && character <= 0x9f) {
			return false
		}
	}
	return true
}

func clearRetained(fields [][]byte) {
	for _, field := range fields {
		clearBytes(field)
	}
}
