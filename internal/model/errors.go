package model

import (
	"encoding/json"
	"fmt"
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
