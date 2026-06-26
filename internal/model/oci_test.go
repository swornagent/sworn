package model

import (
	"context"
	"os"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/generativeaiinference"
)

// fakeOCIClient implements generativeAIInferenceClient for tests.
type fakeOCIClient struct {
	chatFn func(ctx context.Context, request generativeaiinference.ChatRequest) (generativeaiinference.ChatResponse, error)
}

func (f *fakeOCIClient) Chat(ctx context.Context, request generativeaiinference.ChatRequest) (generativeaiinference.ChatResponse, error) {
	return f.chatFn(ctx, request)
}

// ociTextResponse builds a ChatResponse with one GenericChatResponse choice
// containing the given text.
func ociTextResponse(text string) generativeaiinference.ChatResponse {
	return generativeaiinference.ChatResponse{
		ChatResult: generativeaiinference.ChatResult{
			ChatResponse: generativeaiinference.GenericChatResponse{
				Choices: []generativeaiinference.ChatChoice{
					{
						Message: generativeaiinference.AssistantMessage{
							Content: []generativeaiinference.ChatContent{
								generativeaiinference.TextContent{
									Text: common.String(text),
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestOCIVerify_ReturnsText(t *testing.T) {
	fake := &fakeOCIClient{
		chatFn: func(ctx context.Context, req generativeaiinference.ChatRequest) (generativeaiinference.ChatResponse, error) {
			return ociTextResponse("PASS - all checks pass"), nil
		},
	}

	o := &OCI{
		Client:        fake,
		ModelID:       "cohere.command-r-plus",
		CompartmentID: "ocid1.compartment.oc1..test",
	}

	text, cost, err := o.Verify(context.Background(), "be strict", "verify this diff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "PASS - all checks pass" {
		t.Fatalf("want %q, got %q", "PASS - all checks pass", text)
	}
	if cost != 0 {
		t.Fatalf("want cost 0, got %f", cost)
	}
}

func TestOCIVerify_MissingCompartment(t *testing.T) {
	// Spec: cfg.OCICompartmentID empty and $OCI_COMPARTMENT_ID absent →
	// Verify returns a non-nil error naming the missing compartment ID.
	o := &OCI{
		ModelID:       "cohere.command-r-plus",
		CompartmentID: "",
	}
	_, _, err := o.Verify(context.Background(), "be strict", "verify this diff")
	if err == nil {
		t.Fatal("Verify with empty compartmentID returned nil error")
	}
}
func TestOCIVerify_MissingTokenCount(t *testing.T) {
	// Build a response with no usage field (nil Usage pointer on GenericChatResponse).
	fake := &fakeOCIClient{
		chatFn: func(ctx context.Context, req generativeaiinference.ChatRequest) (generativeaiinference.ChatResponse, error) {
			return generativeaiinference.ChatResponse{
				ChatResult: generativeaiinference.ChatResult{
					ChatResponse: generativeaiinference.GenericChatResponse{
						Choices: []generativeaiinference.ChatChoice{
							{
								Message: generativeaiinference.AssistantMessage{
									Content: []generativeaiinference.ChatContent{
										generativeaiinference.TextContent{
											Text: common.String("PASS"),
										},
									},
								},
								// Usage is nil — not set.
							},
						},
					},
				},
			}, nil
		},
	}

	o := &OCI{
		Client:        fake,
		ModelID:       "cohere.command-r-plus",
		CompartmentID: "ocid1.compartment.oc1..test",
	}

	_, cost, err := o.Verify(context.Background(), "be strict", "verify this diff")
	if err != nil {
		t.Fatalf("unexpected error on nil usage: %v", err)
	}
	if cost != 0 {
		t.Fatalf("want cost 0 for nil usage, got %f", cost)
	}
}

func TestNewClient_OCIRouted(t *testing.T) {
	// Set OCI_COMPARTMENT_ID so routing succeeds.
	os.Setenv("OCI_COMPARTMENT_ID", "ocid1.compartment.oc1..test")
	defer os.Unsetenv("OCI_COMPARTMENT_ID")

	cfg := ProviderConfigFromEnv()
	v, err := NewClient("oci/cohere.command-r-plus", cfg)
	if err != nil {
		t.Fatalf("NewClient(oci/cohere.command-r-plus) error: %v", err)
	}
	if _, ok := v.(*OCI); !ok {
		t.Fatalf("NewClient returned %T, want *OCI", v)
	}
}

func TestOCIVerify_MissingModelID(t *testing.T) {
	o := &OCI{
		ModelID:       "",
		CompartmentID: "ocid1.compartment.oc1..test",
	}
	_, _, err := o.Verify(context.Background(), "be strict", "verify this diff")
	if err == nil {
		t.Fatal("Verify with empty modelID returned nil error")
	}
}
func TestOCINew_DeferredCredentialLoading(t *testing.T) {
	// When OCI config is absent/partial, NewOCI should still return a
	// non-nil *OCI with no error (per spec: credential loading deferred
	// to first API call). DefaultConfigProvider() is lenient — it
	// composes file + env providers and ignores missing files.
	o, err := NewOCI("cohere.command-r-plus", "ocid1.compartment.oc1..test")
	if err != nil {
		t.Fatalf("NewOCI should not error on missing config (deferred): %v", err)
	}
	if o == nil {
		t.Fatal("NewOCI returned nil *OCI")
	}
	// Verify will fail on EnsureClient if no real OCI config exists,
	// but NewOCI itself succeeds — that's the deferred-loading contract.
	// Client could be nil or non-nil depending on the host's OCI env.
	t.Log("OCI deferred-loading contract: NewOCI succeeded regardless of config state")
}
