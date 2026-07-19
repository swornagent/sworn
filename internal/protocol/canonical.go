package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	maximumSafeInteger = int64(9_007_199_254_740_991)
	maximumJSONDepth   = 256
)

// CanonicalizeJSON strictly parses I-JSON and returns its RFC 8785 JSON
// Canonicalization Scheme representation. Baton record digests cover exactly
// these bytes.
func CanonicalizeJSON(contents []byte) ([]byte, error) {
	if !utf8.Valid(contents) {
		return nil, errors.New("JSON is not valid UTF-8")
	}
	if err := rejectLoneSurrogates(contents); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	value, err := decodeStrictValue(decoder, 0)
	if err != nil {
		return nil, fmt.Errorf("strict JSON: %w", err)
	}
	if token, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, fmt.Errorf("strict JSON trailing input: %w", err)
		}
		return nil, fmt.Errorf("strict JSON has a second top-level value %v", token)
	}
	var canonical bytes.Buffer
	if err := writeCanonical(&canonical, value); err != nil {
		return nil, err
	}
	return canonical.Bytes(), nil
}

// EncodeCanonical marshals an engine-owned value and returns strict RFC 8785
// bytes. It is intentionally the only path used to persist Baton records.
func EncodeCanonical(value any) ([]byte, error) {
	if err := validateGoStrings(reflect.ValueOf(value), make(map[visit]struct{}), 0); err != nil {
		return nil, err
	}
	contents, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal JSON: %w", err)
	}
	return CanonicalizeJSON(contents)
}

type visit struct {
	typeOf   reflect.Type
	pointer  uintptr
	length   int
	capacity int
}

func validateGoStrings(value reflect.Value, visited map[visit]struct{}, depth int) error {
	if !value.IsValid() {
		return nil
	}
	if depth > maximumJSONDepth {
		return fmt.Errorf("Go value nesting exceeds %d levels", maximumJSONDepth)
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		return validateGoStrings(value.Elem(), visited, depth+1)
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		identity := visit{typeOf: value.Type(), pointer: value.Pointer()}
		if _, exists := visited[identity]; exists {
			return nil
		}
		visited[identity] = struct{}{}
		return validateGoStrings(value.Elem(), visited, depth+1)
	}
	switch value.Kind() {
	case reflect.String:
		if !utf8.ValidString(value.String()) {
			return errors.New("Go value contains a string that is not valid UTF-8")
		}
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			if value.Type().Field(index).PkgPath != "" {
				continue
			}
			if err := validateGoStrings(value.Field(index), visited, depth+1); err != nil {
				return err
			}
		}
	case reflect.Map:
		if value.IsNil() {
			return nil
		}
		iterator := value.MapRange()
		for iterator.Next() {
			if err := validateGoStrings(iterator.Key(), visited, depth+1); err != nil {
				return err
			}
			if err := validateGoStrings(iterator.Value(), visited, depth+1); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if value.IsNil() {
			return nil
		}
		identity := visit{
			typeOf: value.Type(), pointer: value.Pointer(), length: value.Len(), capacity: value.Cap(),
		}
		if _, exists := visited[identity]; exists {
			return nil
		}
		visited[identity] = struct{}{}
		fallthrough
	case reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if err := validateGoStrings(value.Index(index), visited, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// CanonicalDigest returns the Baton SHA-256 identifier for canonical bytes.
func CanonicalDigest(canonical []byte) string {
	return RawDigest(canonical)
}

// RawDigest returns the SHA-256 identifier for exact bytes. Baton artifacts
// use this directly; record callers first produce canonical JSON bytes.
func RawDigest(contents []byte) string {
	digest := sha256.Sum256(contents)
	return "sha256:" + hex.EncodeToString(digest[:])
}

type strictObject map[string]any

func decodeStrictValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > maximumJSONDepth {
		return nil, fmt.Errorf("nesting exceeds %d levels", maximumJSONDepth)
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch value := token.(type) {
	case nil, bool:
		return value, nil
	case string:
		if !utf8.ValidString(value) {
			return nil, errors.New("string is not valid Unicode")
		}
		return value, nil
	case json.Number:
		return strictNumber(value)
	case json.Delim:
		switch value {
		case '{':
			object := make(strictObject)
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, errors.New("object name is not a string")
				}
				if _, exists := object[key]; exists {
					return nil, fmt.Errorf("duplicate object name %q", key)
				}
				member, err := decodeStrictValue(decoder, depth+1)
				if err != nil {
					return nil, err
				}
				object[key] = member
			}
			if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
				return nil, errors.New("object is not terminated")
			}
			return object, nil
		case '[':
			array := make([]any, 0)
			for decoder.More() {
				member, err := decodeStrictValue(decoder, depth+1)
				if err != nil {
					return nil, err
				}
				array = append(array, member)
			}
			if end, err := decoder.Token(); err != nil || end != json.Delim(']') {
				return nil, errors.New("array is not terminated")
			}
			return array, nil
		default:
			return nil, fmt.Errorf("unexpected delimiter %q", value)
		}
	default:
		return nil, fmt.Errorf("unsupported JSON token %T", token)
	}
}

func strictNumber(number json.Number) (any, error) {
	lexical := string(number)
	if !strings.ContainsAny(lexical, ".eE") {
		value, err := strconv.ParseInt(lexical, 10, 64)
		if err != nil || value < -maximumSafeInteger || value > maximumSafeInteger {
			return nil, fmt.Errorf("integer outside interoperable range: %s", lexical)
		}
		return value, nil
	}
	value, err := strconv.ParseFloat(lexical, 64)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return nil, fmt.Errorf("non-finite or unrepresentable number: %s", lexical)
	}
	if math.Trunc(value) == value && math.Abs(value) > float64(maximumSafeInteger) {
		return nil, fmt.Errorf("integer-valued number outside interoperable range: %s", lexical)
	}
	return value, nil
}

func writeCanonical(output *bytes.Buffer, value any) error {
	switch value := value.(type) {
	case nil:
		output.WriteString("null")
	case bool:
		output.WriteString(strconv.FormatBool(value))
	case int64:
		output.WriteString(strconv.FormatInt(value, 10))
	case float64:
		number, err := formatJCSNumber(value)
		if err != nil {
			return err
		}
		output.WriteString(number)
	case string:
		writeCanonicalString(output, value)
	case []any:
		output.WriteByte('[')
		for index, member := range value {
			if index != 0 {
				output.WriteByte(',')
			}
			if err := writeCanonical(output, member); err != nil {
				return err
			}
		}
		output.WriteByte(']')
	case strictObject:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		slices.SortFunc(keys, compareUTF16)
		output.WriteByte('{')
		for index, key := range keys {
			if index != 0 {
				output.WriteByte(',')
			}
			writeCanonicalString(output, key)
			output.WriteByte(':')
			if err := writeCanonical(output, value[key]); err != nil {
				return err
			}
		}
		output.WriteByte('}')
	default:
		return fmt.Errorf("cannot canonicalize JSON value %T", value)
	}
	return nil
}

func compareUTF16(left, right string) int {
	leftUnits := utf16.Encode([]rune(left))
	rightUnits := utf16.Encode([]rune(right))
	return slices.Compare(leftUnits, rightUnits)
}

func writeCanonicalString(output *bytes.Buffer, value string) {
	const hexadecimal = "0123456789abcdef"
	output.WriteByte('"')
	for _, character := range value {
		switch character {
		case '"', '\\':
			output.WriteByte('\\')
			output.WriteRune(character)
		case '\b':
			output.WriteString(`\b`)
		case '\t':
			output.WriteString(`\t`)
		case '\n':
			output.WriteString(`\n`)
		case '\f':
			output.WriteString(`\f`)
		case '\r':
			output.WriteString(`\r`)
		default:
			if character < 0x20 {
				output.WriteString(`\u00`)
				output.WriteByte(hexadecimal[byte(character)>>4])
				output.WriteByte(hexadecimal[byte(character)&0x0f])
			} else {
				output.WriteRune(character)
			}
		}
	}
	output.WriteByte('"')
}

func formatJCSNumber(value float64) (string, error) {
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return "", errors.New("cannot canonicalize a non-finite number")
	}
	if value == 0 {
		return "0", nil
	}
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	rendered := strings.ToLower(strconv.FormatFloat(value, 'g', -1, 64))
	mantissa, exponentText, hasExponent := strings.Cut(rendered, "e")
	exponent := 0
	if hasExponent {
		parsed, err := strconv.Atoi(exponentText)
		if err != nil {
			return "", fmt.Errorf("parse formatted exponent %q: %w", exponentText, err)
		}
		exponent = parsed
	}
	integer, fraction, hasFraction := strings.Cut(mantissa, ".")
	digits := strings.TrimLeft(integer+fraction, "0")
	if digits == "" {
		digits = "0"
	}
	scale := exponent
	if hasFraction {
		scale -= len(fraction)
	}
	for len(digits) > 1 && strings.HasSuffix(digits, "0") {
		digits = strings.TrimSuffix(digits, "0")
		scale++
	}
	decimalPosition := len(digits) + scale
	switch {
	case decimalPosition > -6 && decimalPosition <= 0:
		return sign + "0." + strings.Repeat("0", -decimalPosition) + digits, nil
	case decimalPosition > 0 && decimalPosition <= 21:
		if decimalPosition < len(digits) {
			return sign + digits[:decimalPosition] + "." + digits[decimalPosition:], nil
		}
		return sign + digits + strings.Repeat("0", decimalPosition-len(digits)), nil
	default:
		coefficient := digits[:1]
		if len(digits) > 1 {
			coefficient += "." + digits[1:]
		}
		scientificExponent := decimalPosition - 1
		exponentSign := ""
		if scientificExponent >= 0 {
			exponentSign = "+"
		}
		return sign + coefficient + "e" + exponentSign + strconv.Itoa(scientificExponent), nil
	}
}

func rejectLoneSurrogates(contents []byte) error {
	for index := 0; index < len(contents); index++ {
		if contents[index] != '"' {
			continue
		}
		for index++; index < len(contents); index++ {
			switch contents[index] {
			case '"':
				goto nextString
			case '\\':
				index++
				if index >= len(contents) {
					return errors.New("unterminated JSON escape")
				}
				if contents[index] != 'u' {
					continue
				}
				code, ok := escapedCodeUnit(contents, index+1)
				if !ok {
					continue
				}
				index += 4
				switch {
				case code >= 0xd800 && code <= 0xdbff:
					if index+6 >= len(contents) || contents[index+1] != '\\' || contents[index+2] != 'u' {
						return errors.New("JSON contains a lone high surrogate")
					}
					low, valid := escapedCodeUnit(contents, index+3)
					if !valid || low < 0xdc00 || low > 0xdfff {
						return errors.New("JSON contains a lone high surrogate")
					}
					index += 6
				case code >= 0xdc00 && code <= 0xdfff:
					return errors.New("JSON contains a lone low surrogate")
				}
			}
		}
		return errors.New("unterminated JSON string")
	nextString:
	}
	return nil
}

func escapedCodeUnit(contents []byte, start int) (uint16, bool) {
	if start+4 > len(contents) {
		return 0, false
	}
	value, err := strconv.ParseUint(string(contents[start:start+4]), 16, 16)
	return uint16(value), err == nil
}
