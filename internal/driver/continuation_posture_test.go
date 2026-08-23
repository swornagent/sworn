package driver

import "testing"

// TestContinuationPostureDeclaredPerDialect pins the per-dialect declaration
// table the degradation counter reads: gemini (the google-native stateless
// per-request surface whose engine continuation is a replay cache, sworn#227)
// declares fresh_by_design; every other dialect declares context_retaining.
func TestContinuationPostureDeclaredPerDialect(t *testing.T) {
	t.Parallel()

	for _, dialect := range []providerDialect{
		providerDialectOpenAIResponses,
		providerDialectOpenAIChat,
		providerDialectOpenRouterChat,
		providerDialectOpaqueChat,
		providerDialectGoogleChat,
		providerDialectXAIChat,
		providerDialectXAIResponses,
		providerDialectGemini,
		providerDialectBedrockConverse,
	} {
		want := ContinuationPostureContextRetaining
		if dialect == providerDialectGemini {
			want = ContinuationPostureFreshByDesign
		}
		if got := dialect.continuationPosture(); got != want {
			t.Errorf("%q continuationPosture() = %q, want %q", dialect, got, want)
		}
	}
}

// TestContinuationPostureUndeclaredFailsClosed pins the default: a driver or
// adapter that declares no posture is read as context_retaining.
func TestContinuationPostureUndeclaredFailsClosed(t *testing.T) {
	t.Parallel()

	// An invocation with no selected adapter carries no declaration.
	if got := (Dispatcher{}).ContinuationPosture(Invocation{}); got != ContinuationPostureContextRetaining {
		t.Fatalf("undeclared posture = %q, want %q", got, ContinuationPostureContextRetaining)
	}

	// An adapter that is not a loopAdapter (for example, a process or native
	// adapter without the declaration capability) also fails closed.
	invocation := Invocation{Selected: SelectedProfile{
		adapter: nil,
	}}
	if got := (Dispatcher{}).ContinuationPosture(invocation); got != ContinuationPostureContextRetaining {
		t.Fatalf("nil adapter posture = %q, want %q", got, ContinuationPostureContextRetaining)
	}
}
