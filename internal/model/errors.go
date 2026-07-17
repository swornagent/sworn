package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
)

// ErrorKind classifies a provider error by what action (if any) the caller
// can take. This taxonomy lets callers distinguish terminal failures from
// transient ones without string-matching opaque provider error bodies.
type ErrorKind int

const (
	KindOther     ErrorKind = iota // unclassified
	KindAuth                       // 401, 403 — credentials rejected
	KindCredits                    // 402 — payment required / credits exhausted
	KindRateLimit                  // 429 — backoff may help
	KindUpstream                   // 5xx — provider-side fault, retry may help
	KindTransient                  // explicit transient error from the framework
)

// String returns a lowercase name for the kind.
func (k ErrorKind) String() string {
	switch k {
	case KindAuth:
		return "auth"
	case KindCredits:
		return "credits"
	case KindRateLimit:
		return "rate_limit"
	case KindUpstream:
		return "upstream"
	case KindTransient:
		return "transient"
	default:
		return "other"
	}
}

// ProofReceiptErrorClass is the deliberately narrow error taxonomy used by
// S22's native proof receipt. It is separate from IsTransient: legacy callers
// deliberately retry unknown failures once, while a receipt may retry only an
// explicit typed environmental condition.
type ProofReceiptErrorClass string

const (
	ProofReceiptErrorUnknown       ProofReceiptErrorClass = "unknown"
	ProofReceiptErrorHTTPClient    ProofReceiptErrorClass = "http_client_error"
	ProofReceiptErrorRateLimit     ProofReceiptErrorClass = "rate_limit"
	ProofReceiptErrorUpstream      ProofReceiptErrorClass = "upstream"
	ProofReceiptErrorTransient     ProofReceiptErrorClass = "transient"
	ProofReceiptErrorNetwork       ProofReceiptErrorClass = "network"
	ProofReceiptErrorDeadline      ProofReceiptErrorClass = "deadline"
	ProofReceiptErrorMalformedTool ProofReceiptErrorClass = "malformed_tool"
	ProofReceiptErrorOpaque        ProofReceiptErrorClass = "opaque"
)

// StructuredOutputFailureKind is a typed local wire outcome. It ensures the
// proof-receipt classifier never has to inspect a provider error string.
type StructuredOutputFailureKind uint8

const (
	StructuredOutputMalformedTool StructuredOutputFailureKind = iota + 1
	StructuredOutputOpaque
)

// StructuredOutputError is safe to return through existing model interfaces:
// its text carries only the stable class, never an endpoint, payload, or key.
type StructuredOutputError struct {
	Kind    StructuredOutputFailureKind
	message string
}

func (e *StructuredOutputError) Error() string {
	if e != nil && e.message != "" {
		return e.message
	}
	if e != nil && e.Kind == StructuredOutputMalformedTool {
		return "model: structured output malformed tool call"
	}
	return "model: structured output opaque response"
}

// ClassifyProofReceiptError classifies only typed transport and local wire
// facts. It never matches error-message text, so opaque provider bodies,
// requests, credentials, endpoints, and arbitrary local errors cannot become
// retry authority.
func ClassifyProofReceiptError(err error) ProofReceiptErrorClass {
	if err == nil {
		return ProofReceiptErrorUnknown
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ProofReceiptErrorDeadline
	}

	var structuredErr *StructuredOutputError
	if errors.As(err, &structuredErr) {
		if structuredErr.Kind == StructuredOutputMalformedTool {
			return ProofReceiptErrorMalformedTool
		}
		return ProofReceiptErrorOpaque
	}

	var providerErr *Error
	if errors.As(err, &providerErr) {
		switch providerErr.Kind {
		case KindRateLimit:
			return ProofReceiptErrorRateLimit
		case KindUpstream:
			return ProofReceiptErrorUpstream
		case KindTransient:
			return ProofReceiptErrorTransient
		case KindAuth, KindCredits:
			return ProofReceiptErrorHTTPClient
		case KindOther:
			if providerErr.Status >= 400 && providerErr.Status < 500 {
				return ProofReceiptErrorHTTPClient
			}
			return ProofReceiptErrorUnknown
		default:
			return ProofReceiptErrorUnknown
		}
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return ProofReceiptErrorOpaque
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return ProofReceiptErrorOpaque
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		if networkErr.Timeout() {
			return ProofReceiptErrorDeadline
		}
		return ProofReceiptErrorNetwork
	}

	return ProofReceiptErrorUnknown
}

// Error wraps a provider error with a classified Kind. It implements both
// error and Unwrap() so existing err != nil callers are unchanged.
type Error struct {
	Kind     ErrorKind
	Status   int    // HTTP status code (0 if not HTTP)
	Provider string // e.g. "openai", "groq"
	Model    string // model ID that was used in the request
	Message  string // human-readable description
	Err      error  // wrapped underlying error (may be nil)
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s", e.Message, e.Err.Error())
	}
	return e.Message
}

// Unwrap returns the wrapped error for errors.Is / errors.As.
func (e *Error) Unwrap() error {
	return e.Err
}

// ClassifyHTTP maps an HTTP status code and optional JSON error body to an
// ErrorKind. It lifts the provider's JSON error.message field when present.
//
// Mapping:
//
//	401, 403 → KindAuth
//	402       → KindCredits
//	429       → KindRateLimit
//	500–599   → KindUpstream
//	all other → KindOther
func ClassifyHTTP(status int, body []byte) ErrorKind {
	switch {
	case status == 401 || status == 403:
		return KindAuth
	case status == 402:
		return KindCredits
	case status == 429:
		return KindRateLimit
	case status >= 500 && status < 600:
		return KindUpstream
	default:
		return KindOther
	}
}

// IsTerminal reports whether retrying will never help for this error kind.
// Auth and Credits failures are terminal — the same credentials will fail
// again. RateLimit, Upstream, and Transient may succeed on retry.
func IsTerminal(err error) bool {
	var me *Error
	if AsError(err, &me) {
		return me.Kind == KindAuth || me.Kind == KindCredits
	}
	return false
}

// IsTransient reports whether the error may succeed on retry.
// For typed errors, this is the converse of IsTerminal.
// Unknown/untyped errors are assumed transient (retry once).
func IsTransient(err error) bool {
	var me *Error
	if AsError(err, &me) {
		return !IsTerminal(err)
	}
	return true // unknown errors are assumed transient (retry once)
}

// UserMessage returns an actionable message suitable for end-user display.
func (e *Error) UserMessage() string {
	switch e.Kind {
	case KindAuth:
		provider := e.Provider
		if provider == "" {
			provider = "the provider"
		}
		return fmt.Sprintf("Provider rejected credentials — check the API key for %s in ~/.sworn/.env", provider)
	case KindCredits:
		return "Out of credits — run `sworn account buy` or top up your provider account"
	case KindRateLimit:
		return "Rate limited by provider — waiting and retrying"
	case KindUpstream:
		return "Provider returned an error — retrying shortly"
	default:
		// Return the raw message but don't leak provider JSON.
		if e.Message != "" {
			return e.Message
		}
		return "An unexpected error occurred"
	}
}

// AsError is a helper that checks if err (or any wrapped error) is a *model.Error.
// It exists so the model package doesn't have to import errors (which re-exports
// the stdlib errors package and would shadow our Error type).
func AsError(err error, target **Error) bool {
	if err == nil {
		return false
	}
	if me, ok := err.(*Error); ok {
		*target = me
		return true
	}
	// Walk the Unwrap chain manually to avoid importing errors.
	type wrapper interface{ Unwrap() error }
	for {
		w, ok := err.(wrapper)
		if !ok {
			return false
		}
		err = w.Unwrap()
		if err == nil {
			return false
		}
		if me, ok := err.(*Error); ok {
			*target = me
			return true
		}
	}
}

// providerErrorMessage attempts to extract a JSON error message from a
// provider response body. Returns empty string if the body isn't valid JSON
// with an error.message field.
func providerErrorMessage(body []byte) string {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Error.Message)
}

// NewProviderError constructs a *model.Error from an HTTP status, provider
// name, model ID, and response body. This is the canonical constructor for
// typed provider errors from the OAI client (and future native drivers).
func NewProviderError(status int, provider, model string, body []byte) *Error {
	kind := ClassifyHTTP(status, body)
	msg := providerErrorMessage(body)
	if msg == "" {
		msg = fmt.Sprintf("HTTP %d from %s", status, provider)
	}
	return &Error{
		Kind:     kind,
		Status:   status,
		Provider: provider,
		Model:    model,
		Message:  msg,
	}
}
