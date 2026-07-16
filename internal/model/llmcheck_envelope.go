package model

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
)

// The canonical generic report is an immutable Baton protocol input. OpenAI's
// strict-output subset cannot express its conditional allOf branches, so the
// two explicitly profiled OpenAI structured-output routes use this small
// outbound envelope instead. The canonical bytes still cross the model
// boundary untouched and remain the sole semantic authority in gate.
const (
	canonicalLLMCheckReportID     = "https://baton.sawy3r.net/schemas/llm-check-report-v1.json"
	canonicalLLMCheckReportDigest = "ed38b77823af1b329c1dc7d8427b08849f15690d5afa9625e196505bdfa5b65b"
	specAmbiguityReportID         = "https://baton.sawy3r.net/schemas/spec-ambiguity-report-v1.json"
	openAILLMCheckEnvelopeName    = "llm-check-report-v1-openai-envelope"
)

// Structured provider identity is intentionally separate from wire mode.
// OAI-compatible URLs and Go concrete types are not authority: only this
// construction-time profile may select the OpenAI report envelope.
type structuredProviderProfile uint8

const (
	structuredProfileNone structuredProviderProfile = iota
	structuredProfileOpenAIResponses
	structuredProfileOpenAICompletions
)

// structuredWireMode records the actual structured-output wire shape. The
// existing StructuredMode remains the OAI driver's public emission selector;
// this companion is carried through direct and proxy construction so profile
// selection cannot be inferred from a type assertion or endpoint URL.
type structuredWireMode uint8

const (
	structuredWireNone structuredWireMode = iota
	structuredWireChatResponseFormat
	structuredWireToolCall
	structuredWireResponsesTextFormat
)

type structuredProviderRoute struct {
	profile structuredProviderProfile
	wire    structuredWireMode
	oaiMode StructuredMode
}

func (r structuredProviderRoute) String() string {
	return r.profile.String() + "-" + r.wire.String()
}

func (p structuredProviderProfile) String() string {
	switch p {
	case structuredProfileOpenAIResponses:
		return "openai-responses"
	case structuredProfileOpenAICompletions:
		return "openai-completions"
	default:
		return "unprofiled"
	}
}

func (m structuredWireMode) String() string {
	switch m {
	case structuredWireChatResponseFormat:
		return "response-format"
	case structuredWireToolCall:
		return "tool-call"
	case structuredWireResponsesTextFormat:
		return "responses-text-format"
	default:
		return "none"
	}
}

// structuredRouteForProvider is the single default-deny prefix mapping used by
// direct NewClient and proxyClient construction. It deliberately recognises
// the deprecated openai-responses alias as the same Responses profile for its
// one-release compatibility window.
func structuredRouteForProvider(provider string) structuredProviderRoute {
	switch provider {
	case "openai", "openai-responses":
		return structuredProviderRoute{
			profile: structuredProfileOpenAIResponses,
			wire:    structuredWireResponsesTextFormat,
		}
	case "openai-completions":
		return structuredProviderRoute{
			profile: structuredProfileOpenAICompletions,
			wire:    structuredWireChatResponseFormat,
			oaiMode: StructuredResponseFormat,
		}
	case "xai":
		return structuredProviderRoute{
			wire:    structuredWireChatResponseFormat,
			oaiMode: StructuredResponseFormat,
		}
	case "deepseek":
		return structuredProviderRoute{
			wire:    structuredWireToolCall,
			oaiMode: StructuredToolCall,
		}
	default:
		return structuredProviderRoute{}
	}
}

// These errors are intentionally stable and source-free. A pre-dispatch
// rejection must not expose the supplied schema, user payload, or credentials.
var (
	errOpenAIEnvelopeDigestMismatch    = constErr("model: OpenAI structured envelope rejected llm-check report digest")
	errOpenAIEnvelopeUnsupportedFamily = constErr("model: OpenAI structured envelope rejected unsupported llm-check report family")
	errOpenAIEnvelopeSpecAmbiguity     = constErr("model: OpenAI structured envelope rejected dedicated spec-ambiguity report")
)

type openAIStructuredEnvelope struct {
	Name   string
	Schema []byte
}

// compileOpenAILLMCheckEnvelope is a closed-world compiler. It has exactly
// one successful source identity: the canonical report's exact $id and exact
// byte digest, through one explicitly profiled OpenAI response-format wire.
// Other providers and modes return no selection so their existing schema path
// remains unchanged.
func compileOpenAILLMCheckEnvelope(profile structuredProviderProfile, wire structuredWireMode, source []byte) (openAIStructuredEnvelope, bool, error) {
	if !isOpenAIEnvelopeRoute(profile, wire) {
		return openAIStructuredEnvelope{}, false, nil
	}

	var identity struct {
		ID string `json:"$id"`
	}
	if err := json.Unmarshal(source, &identity); err != nil {
		// Preserve the existing strict-projection parse error for malformed
		// unrelated schemas. It remains a local, pre-HTTP failure.
		return openAIStructuredEnvelope{}, false, nil
	}

	switch identity.ID {
	case canonicalLLMCheckReportID:
		digest := sha256.Sum256(source)
		if fmtDigest(digest) != canonicalLLMCheckReportDigest {
			return openAIStructuredEnvelope{}, false, errOpenAIEnvelopeDigestMismatch
		}
		return openAIStructuredEnvelope{
			Name:   openAILLMCheckEnvelopeName,
			Schema: append([]byte(nil), openAILLMCheckEnvelopeSchema...),
		}, true, nil
	case specAmbiguityReportID:
		// C-02 owns its map-shaped contract. It is never flattened into the
		// generic report envelope.
		return openAIStructuredEnvelope{}, false, errOpenAIEnvelopeSpecAmbiguity
	default:
		if isLLMCheckReportFamilyID(identity.ID) {
			return openAIStructuredEnvelope{}, false, errOpenAIEnvelopeUnsupportedFamily
		}
		return openAIStructuredEnvelope{}, false, nil
	}
}

// strictSchemaForWire returns the outbound strict schema for a native
// response-format path. The closed-world envelope is selected first; every
// other source retains the existing strictProjection behavior unchanged.
func strictSchemaForWire(profile structuredProviderProfile, wire structuredWireMode, source []byte) (name string, schema []byte, err error) {
	envelope, selected, err := compileOpenAILLMCheckEnvelope(profile, wire, source)
	if err != nil {
		return "", nil, err
	}
	if selected {
		return envelope.Name, envelope.Schema, nil
	}
	strict, err := strictProjection(source)
	if err != nil {
		return "", nil, err
	}
	return schemaName(source), strict, nil
}

func isOpenAIEnvelopeRoute(profile structuredProviderProfile, wire structuredWireMode) bool {
	return (profile == structuredProfileOpenAIResponses && wire == structuredWireResponsesTextFormat) ||
		(profile == structuredProfileOpenAICompletions && wire == structuredWireChatResponseFormat)
}

// isLLMCheckReportFamilyID is intentionally anchored to Baton schema IDs. A
// title, substring, endpoint, map shape, or lookalike domain cannot enter the
// family. The canonical v1 is handled before this predicate; v2+ is rejected.
func isLLMCheckReportFamilyID(id string) bool {
	const prefix = "https://baton.sawy3r.net/schemas/llm-check-report-v"
	const suffix = ".json"
	if !strings.HasPrefix(id, prefix) || !strings.HasSuffix(id, suffix) {
		return false
	}
	version := strings.TrimSuffix(strings.TrimPrefix(id, prefix), suffix)
	if version == "" {
		return false
	}
	for _, digit := range version {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func fmtDigest(sum [sha256.Size]byte) string {
	const hex = "0123456789abcdef"
	buf := make([]byte, len(sum)*2)
	for i, b := range sum {
		buf[i*2] = hex[b>>4]
		buf[i*2+1] = hex[b&0x0f]
	}
	return string(buf)
}

// This fixed schema intentionally contains only the generic fields OpenAI
// strict output must emit. It is an outbound wire envelope, not a canonical
// semantic schema: optional evidence and every allOf conditional remain solely
// in the unchanged Baton validation performed after model output.
var openAILLMCheckEnvelopeSchema = []byte(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["check", "verdict", "findings"],
  "properties": {
    "check": {
      "type": "string",
      "enum": [
        "spec-ambiguity",
        "design-review",
        "ac-satisfaction",
        "security-review",
        "semantic-coverage",
        "maintainability-review"
      ]
    },
    "verdict": {"type": "string", "enum": ["PASS", "FAIL"]},
    "findings": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["id", "severity", "blocking", "title", "detail"],
        "properties": {
          "id": {"type": "string", "minLength": 1},
          "severity": {"type": "string", "enum": ["critical", "high", "medium", "low", "info"]},
          "blocking": {"type": "boolean"},
          "title": {"type": "string", "minLength": 1},
          "detail": {"type": "string", "minLength": 1}
        }
      }
    }
  }
}`)
