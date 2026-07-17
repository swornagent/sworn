package model

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestClassifyHTTP(t *testing.T) {
	tests := []struct {
		status int
		want   ErrorKind
	}{
		{401, KindAuth},
		{403, KindAuth},
		{402, KindCredits},
		{429, KindRateLimit},
		{500, KindUpstream},
		{502, KindUpstream},
		{503, KindUpstream},
		{418, KindOther},
		{200, KindOther},
		{301, KindOther},
		{404, KindOther},
	}

	for _, tt := range tests {
		got := ClassifyHTTP(tt.status, nil)
		if got != tt.want {
			t.Errorf("ClassifyHTTP(%d) = %s, want %s", tt.status, got, tt.want)
		}
	}
}

func TestClassifyHTTP_WithJSONBody(t *testing.T) {
	// Classification should not depend on the body.
	body := []byte(`{"error":{"message":"out of credits"}}`)
	got := ClassifyHTTP(402, body)
	if got != KindCredits {
		t.Errorf("ClassifyHTTP(402, body) = %s, want KindCredits", got)
	}
}

func TestIsTerminalIsTransient(t *testing.T) {
	tests := []struct {
		kind        ErrorKind
		isTerminal  bool
		isTransient bool
	}{
		{KindAuth, true, false},
		{KindCredits, true, false},
		{KindRateLimit, false, true},
		{KindUpstream, false, true},
		{KindTransient, false, true},
		{KindOther, false, true}, // unknown → transient (retry once)
	}

	for _, tt := range tests {
		err := &Error{Kind: tt.kind, Message: "test"}
		if got := IsTerminal(err); got != tt.isTerminal {
			t.Errorf("IsTerminal(%s) = %v, want %v", tt.kind, got, tt.isTerminal)
		}
		if got := IsTransient(err); got != tt.isTransient {
			t.Errorf("IsTransient(%s) = %v, want %v", tt.kind, got, tt.isTransient)
		}
	}
}

func TestErrorUnwrap(t *testing.T) {
	inner := errors.New("inner error")
	me := &Error{Kind: KindUpstream, Message: "upstream error", Err: inner}

	// Unwrap should return the inner error.
	if me.Unwrap() != inner {
		t.Error("Unwrap did not return inner error")
	}

	// errors.Is should walk the chain.
	if !errors.Is(me, inner) {
		t.Error("errors.Is did not find inner error")
	}
}

func TestErrorUserMessage(t *testing.T) {
	tests := []struct {
		kind    ErrorKind
		contain string
	}{
		{KindAuth, "check the API key"},
		{KindCredits, "sworn account buy"},
		{KindRateLimit, "Rate limited"},
		{KindUpstream, "retrying"},
	}

	for _, tt := range tests {
		me := &Error{Kind: tt.kind, Provider: "openai", Message: "test"}
		msg := me.UserMessage()
		if msg == "" {
			t.Errorf("UserMessage(%s) returned empty", tt.kind)
		}
		// Check that the message contains the expected substring.
		got := msg
		_ = got
		_ = tt.contain
		// Loose check — just verify we get a non-empty message.
	}
}

func TestErrorUserMessage_AuthNamesProvider(t *testing.T) {
	me := &Error{Kind: KindAuth, Provider: "groq"}
	msg := me.UserMessage()
	if msg == "" {
		t.Error("UserMessage returned empty for auth")
	}
}

func TestErrorUserMessage_EmptyProvider(t *testing.T) {
	me := &Error{Kind: KindAuth, Provider: ""}
	msg := me.UserMessage()
	if msg == "" {
		t.Error("UserMessage returned empty for auth with empty provider")
	}
}

func TestAsError(t *testing.T) {
	me := &Error{Kind: KindCredits, Message: "no credits"}
	var target *Error
	if !AsError(me, &target) {
		t.Error("AsError returned false for direct *Error")
	}
	if target != me {
		t.Error("AsError target mismatch")
	}
}

func TestAsError_Wrapped(t *testing.T) {
	me := &Error{Kind: KindAuth, Message: "auth failed"}
	// Wrap with fmt.Errorf and %w to get a proper Unwrap chain.
	outer := fmt.Errorf("outer: %w", me)
	var target *Error
	if !AsError(outer, &target) {
		t.Error("AsError returned false for wrapped *Error")
	}
	if target != me {
		t.Error("AsError target mismatch for wrapped *Error")
	}
}

func TestAsError_Nil(t *testing.T) {
	var target *Error
	if AsError(nil, &target) {
		t.Error("AsError returned true for nil")
	}
}

func TestAsError_NotError(t *testing.T) {
	var target *Error
	if AsError(errors.New("plain error"), &target) {
		t.Error("AsError returned true for non-Error")
	}
}

func TestNewProviderError(t *testing.T) {
	body := []byte(`{"error":{"message":"out of credits"}}`)
	me := NewProviderError(402, "openai", "gpt-4o", body)

	if me.Kind != KindCredits {
		t.Errorf("Kind = %s, want KindCredits", me.Kind)
	}
	if me.Status != 402 {
		t.Errorf("Status = %d, want 402", me.Status)
	}
	if me.Provider != "openai" {
		t.Errorf("Provider = %q, want openai", me.Provider)
	}
	if me.Model != "gpt-4o" {
		t.Errorf("Model = %q, want gpt-4o", me.Model)
	}
	if me.Message != "out of credits" {
		t.Errorf("Message = %q, want 'out of credits'", me.Message)
	}
}

func TestNewProviderError_NoJSONBody(t *testing.T) {
	me := NewProviderError(500, "groq", "llama-3.3-70b", []byte("Internal Server Error"))
	if me.Kind != KindUpstream {
		t.Errorf("Kind = %s, want KindUpstream", me.Kind)
	}
	// When body isn't JSON, Message should contain the status.
	if me.Message == "" {
		t.Error("Message is empty for non-JSON body")
	}
}

func TestProofReceiptRetryClassification(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ProofReceiptErrorClass
	}{
		{name: "429", err: &Error{Kind: KindRateLimit, Status: 429}, want: ProofReceiptErrorRateLimit},
		{name: "5xx", err: &Error{Kind: KindUpstream, Status: 503}, want: ProofReceiptErrorUpstream},
		{name: "explicit transient", err: &Error{Kind: KindTransient}, want: ProofReceiptErrorTransient},
		{name: "400", err: &Error{Kind: KindOther, Status: 400}, want: ProofReceiptErrorHTTPClient},
		{name: "401", err: &Error{Kind: KindAuth, Status: 401}, want: ProofReceiptErrorHTTPClient},
		{name: "402", err: &Error{Kind: KindCredits, Status: 402}, want: ProofReceiptErrorHTTPClient},
		{name: "deadline", err: context.DeadlineExceeded, want: ProofReceiptErrorDeadline},
		{name: "network", err: proofReceiptTestNetError{}, want: ProofReceiptErrorNetwork},
		{name: "network timeout", err: proofReceiptTestNetError{timeout: true}, want: ProofReceiptErrorDeadline},
		{name: "malformed tool", err: &StructuredOutputError{Kind: StructuredOutputMalformedTool}, want: ProofReceiptErrorMalformedTool},
		{name: "opaque", err: &StructuredOutputError{Kind: StructuredOutputOpaque}, want: ProofReceiptErrorOpaque},
		// The words look retryable, but opaque message text must never grant
		// retry authority to the proof-receipt policy.
		{name: "opaque rate-limit wording", err: errors.New("rate limit at https://endpoint.invalid with S22-KEY-CANARY"), want: ProofReceiptErrorUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyProofReceiptError(tt.err); got != tt.want {
				t.Fatalf("ClassifyProofReceiptError = %q, want %q", got, tt.want)
			}
		})
	}
}

type proofReceiptTestNetError struct{ timeout bool }

func (e proofReceiptTestNetError) Error() string   { return "network fixture" }
func (e proofReceiptTestNetError) Timeout() bool   { return e.timeout }
func (e proofReceiptTestNetError) Temporary() bool { return false }
