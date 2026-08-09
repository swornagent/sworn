package baton

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"unicode/utf8"
)

const (
	MaxJSONDepth     = 64
	MaxPlanBytes     = 1_048_576
	MaxReceiptBytes  = 1_048_576
	MaxDetailBytes   = 1_048_576
	MaxMessageBytes  = 2_097_152
	MaxEvidenceBytes = 1_048_576
	MaxTracks        = 64
	MaxSlices        = 1_024
	MaxListItems     = 256
)

type RecordError struct {
	Code string
	Msg  string
	Err  error
}

func (e *RecordError) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Msg + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Msg
}
func (e *RecordError) Unwrap() error        { return e.Err }
func recordFail(code, message string) error { return &RecordError{Code: code, Msg: message} }
func recordWrap(code, message string, err error) error {
	return &RecordError{Code: code, Msg: message, Err: err}
}
func ErrorCode(err error) string {
	var record *RecordError
	if errors.As(err, &record) {
		return record.Code
	}
	return ""
}

var numberPattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

func strictParseJSON(raw []byte, label string, maxBytes int) (any, error) {
	if len(raw) > maxBytes {
		return nil, recordFail("RESOURCE_LIMIT", fmt.Sprintf("%s exceeds maximum size %d bytes", label, maxBytes))
	}
	if !utf8.Valid(raw) {
		return nil, recordFail("INVALID_UTF8", label+" is not valid UTF-8")
	}
	if err := validateUnicodeEscapes(raw); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeStrictValue(decoder, 0)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, recordFail("TRAILING_JSON", "strict JSON has trailing input")
		}
		return nil, recordWrap("TRAILING_JSON", "strict JSON has trailing input", err)
	}
	return value, nil
}
func decodeStrictValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > MaxJSONDepth {
		return nil, recordFail("RESOURCE_LIMIT", fmt.Sprintf("strict JSON exceeds maximum depth %d", MaxJSONDepth))
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, recordWrap("INVALID_JSON", "cannot decode strict JSON", err)
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			result := make(map[string]any)
			for decoder.More() {
				nameToken, err := decoder.Token()
				if err != nil {
					return nil, recordWrap("INVALID_JSON", "cannot decode object name", err)
				}
				name, ok := nameToken.(string)
				if !ok {
					return nil, recordFail("INVALID_JSON", "object name is not a string")
				}
				if _, duplicate := result[name]; duplicate {
					return nil, recordFail("DUPLICATE_NAME", fmt.Sprintf("duplicate object name %q", name))
				}
				nested, err := decodeStrictValue(decoder, depth+1)
				if err != nil {
					return nil, err
				}
				result[name] = nested
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim('}') {
				return nil, recordWrap("INVALID_JSON", "object has no closing delimiter", err)
			}
			return result, nil
		case '[':
			result := make([]any, 0)
			for decoder.More() {
				nested, err := decodeStrictValue(decoder, depth+1)
				if err != nil {
					return nil, err
				}
				result = append(result, nested)
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim(']') {
				return nil, recordWrap("INVALID_JSON", "array has no closing delimiter", err)
			}
			return result, nil
		default:
			return nil, recordFail("INVALID_JSON", "unexpected closing delimiter")
		}
	case json.Number:
		raw := value.String()
		if !numberPattern.MatchString(raw) {
			return nil, recordFail("INVALID_JSON", "invalid JSON number")
		}
		number, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
			return nil, recordFail("NONFINITE_NUMBER", "non-finite number "+raw)
		}
		if math.Trunc(number) != number || math.Abs(number) > 9_007_199_254_740_991 {
			return nil, recordFail("UNSAFE_INTEGER", "number is not a safe integer: "+raw)
		}
		return json.Number(strconv.FormatInt(int64(number), 10)), nil
	case string, bool, nil:
		return value, nil
	default:
		return nil, recordFail("INVALID_JSON", "unexpected JSON value")
	}
}
func validateUnicodeEscapes(raw []byte) error {
	inString := false
	for index := 0; index < len(raw); index++ {
		character := raw[index]
		if !inString {
			if character == '"' {
				inString = true
			}
			continue
		}
		if character == '"' {
			inString = false
			continue
		}
		if character != '\\' {
			continue
		}
		index++
		if index >= len(raw) {
			return recordFail("INVALID_JSON", "unterminated escape")
		}
		if raw[index] != 'u' {
			continue
		}
		value, ok := hexQuad(raw, index+1)
		if !ok {
			return recordFail("INVALID_JSON", "invalid Unicode escape")
		}
		index += 4
		if value >= 0xdc00 && value <= 0xdfff {
			return recordFail("INVALID_UNICODE", "string contains a lone low surrogate")
		}
		if value < 0xd800 || value > 0xdbff {
			continue
		}
		if index+6 >= len(raw) || raw[index+1] != '\\' || raw[index+2] != 'u' {
			return recordFail("INVALID_UNICODE", "string contains a lone high surrogate")
		}
		low, ok := hexQuad(raw, index+3)
		if !ok || low < 0xdc00 || low > 0xdfff {
			return recordFail("INVALID_UNICODE", "string contains a lone high surrogate")
		}
		index += 6
	}
	return nil
}
func hexQuad(raw []byte, start int) (uint16, bool) {
	if start+4 > len(raw) {
		return 0, false
	}
	var value uint16
	for _, character := range raw[start : start+4] {
		value <<= 4
		switch {
		case character >= '0' && character <= '9':
			value += uint16(character - '0')
		case character >= 'a' && character <= 'f':
			value += uint16(character-'a') + 10
		case character >= 'A' && character <= 'F':
			value += uint16(character-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}
func exactKeys(value map[string]any, required, optional []string, label string) error {
	allowed := make(map[string]struct{}, len(required)+len(optional))
	for _, key := range append(append([]string(nil), required...), optional...) {
		allowed[key] = struct{}{}
	}
	for key := range value {
		if _, ok := allowed[key]; !ok {
			return recordFail("UNKNOWN_FIELD", label+" has unknown field "+key)
		}
	}
	for _, key := range required {
		if _, ok := value[key]; !ok {
			return recordFail("MISSING_FIELD", label+" is missing "+key)
		}
	}
	return nil
}
func asObject(value any, label string) (map[string]any, error) {
	result, ok := value.(map[string]any)
	if !ok {
		return nil, recordFail("INVALID_SHAPE", label+" must be an object")
	}
	return result, nil
}
func asArray(value any, label string, nonempty bool, max int) ([]any, error) {
	result, ok := value.([]any)
	if !ok || (nonempty && len(result) == 0) {
		return nil, recordFail("INVALID_FIELD", label+" must be an array")
	}
	if len(result) > max {
		return nil, recordFail("RESOURCE_LIMIT", fmt.Sprintf("%s exceeds maximum length %d", label, max))
	}
	return result, nil
}
func asString(value any, label string, min, max int) (string, error) {
	result, ok := value.(string)
	if !ok || len([]rune(result)) < min || len([]rune(result)) > max || !utf8.ValidString(result) {
		return "", recordFail("INVALID_FIELD", fmt.Sprintf("%s must be a string of %d-%d characters", label, min, max))
	}
	return result, nil
}
