package model

import (
	"testing"
)

// TestCapabilities_AllDrivers asserts every driver in the model package
// implements CapabilityProvider and returns the correct capability bits.
func TestCapabilities_AllDrivers(t *testing.T) {
	tests := []struct {
		name string
		cp   CapabilityProvider
		want Capability
	}{
		// Chat-capable drivers.
		{name: "OAI", cp: &OAI{}, want: CapVerify | CapChat},
		{name: "OAI-structured-responseformat", cp: &OAI{Structured: StructuredResponseFormat}, want: CapVerify | CapChat | CapStructuredOutput},
		{name: "OAI-structured-toolcall", cp: &OAI{Structured: StructuredToolCall}, want: CapVerify | CapChat | CapStructuredOutput},
		{name: "OpenAIResponses", cp: &OpenAIResponses{}, want: CapVerify | CapChat | CapStructuredOutput},
		{name: "Anthropic", cp: &Anthropic{}, want: CapVerify | CapChat},
		{name: "cliDriver", cp: &cliDriver{}, want: CapVerify | CapChat},
		// Verify-only drivers.
		{name: "AzureOAI", cp: &AzureOAI{}, want: CapVerify},
		{name: "Bedrock", cp: &Bedrock{}, want: CapVerify},
		{name: "Google", cp: &Google{}, want: CapVerify},
		{name: "OCI", cp: &OCI{}, want: CapVerify},
		{name: "Ollama", cp: &Ollama{}, want: CapVerify},
		// Unconfigured returns 0.
		{name: "Unconfigured", cp: Unconfigured{}, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cp.Capabilities()
			if got != tt.want {
				t.Errorf("%s.Capabilities() = %#b, want %#b", tt.name, got, tt.want)
			}
		})
	}
}

// TestCapabilities_ChatBit asserts that each driver's Capabilities() return
// value is consistent with what the spec requires for Chat support.
func TestCapabilities_ChatBit(t *testing.T) {
	// Drivers that must have Chat set.
	chatDrivers := []CapabilityProvider{
		&OAI{},
		&OpenAIResponses{},
		&Anthropic{},
		&cliDriver{},
	}
	for _, cp := range chatDrivers {
		if cp.Capabilities()&CapChat == 0 {
			t.Errorf("%T: expected CapChat bit to be set", cp)
		}
	}

	// Drivers that must NOT have Chat set.
	noChatDrivers := []CapabilityProvider{
		&AzureOAI{},
		&Bedrock{},
		&Google{},
		&OCI{},
		&Ollama{},
	}
	for _, cp := range noChatDrivers {
		if cp.Capabilities()&CapChat != 0 {
			t.Errorf("%T: expected CapChat bit to NOT be set", cp)
		}
	}
}

// TestCapabilities_InterfaceAssertion is a compile-time guard: every driver
// type in this package must satisfy CapabilityProvider.  This test doesn't
// need to run — if it compiles, the assertion holds.
func TestCapabilities_InterfaceAssertion(t *testing.T) {
	// If any driver does not implement CapabilityProvider, the next block
	// will not compile.  That is the test.
	var _ = []CapabilityProvider{
		Unconfigured{},
		&OAI{},
		&OpenAIResponses{},
		&Anthropic{},
		&cliDriver{},
		&AzureOAI{},
		&Bedrock{},
		&Google{},
		&OCI{},
		&Ollama{},
	}

	// Compile-time guard: drivers advertising CapStructuredOutput must satisfy
	// the StructuredOutput interface (ADR-0011 — no decorative capability bits).
	var _ = []StructuredOutput{
		&OAI{},
		&OpenAIResponses{},
	}
}
