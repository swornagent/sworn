package model

import (
	"context"
	"fmt"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/generativeaiinference"
)

// generativeAIInferenceClient is a local interface matching the Chat method on
// generativeaiinference.GenerativeAiInferenceClient. The OCI SDK client is a
// concrete struct — unlike Bedrock's client (which exposes BaseEndpoint for
// httptest), the OCI SDK does not expose a BaseEndpoint override, so we
// extract a local interface for test substitution.
type generativeAIInferenceClient interface {
	Chat(ctx context.Context, request generativeaiinference.ChatRequest) (generativeaiinference.ChatResponse, error)
}

// OCI dispatches verification calls to Oracle Cloud Infrastructure
// Generative AI using the official oci-go-sdk/v65 generativeaiinference
// client. It implements Verifier.
//
// OAI-import segregation: this file imports only the OCI SDK types — never
// internal/model/oai.go or any OAI struct types. The two drivers share the
// model.Error taxonomy via this package but have zero import overlap.
type OCI struct {
	Client        generativeAIInferenceClient
	ModelID       string
	CompartmentID string
}

// NewOCI constructs an OCI driver. modelID is the OCI model ID (e.g.
// "cohere.command-r-plus"). compartmentID must be non-empty — it is the OCID
// of the compartment that hosts the Generative AI service.
//
// Credential loading (OCI config file / OCI_* env vars) is deferred to the
// first Verify call — per spec acceptance check: NewOCI returns a non-nil
// *OCI with no error even when ~/.oci/config is absent; the error surfaces
// at Verify time.
func NewOCI(modelID, compartmentID string) (*OCI, error) {
	// Validation of modelID and compartmentID is deferred to Verify
	// (spec: NewOCI returns non-nil *OCI with no error — credential loading
	// deferred to first API call).
	o := &OCI{
		ModelID:       modelID,
		CompartmentID: compartmentID,
	}	// Best-effort client creation; defer error to Verify call.
	configProvider := common.DefaultConfigProvider()
	client, err := generativeaiinference.NewGenerativeAiInferenceClientWithConfigurationProvider(configProvider)
	if err != nil {
		// Credential loading deferred — surface at Verify time.
		return o, nil
	}
	o.Client = client
	return o, nil
}

// EnsureClient returns a ready generativeAIInferenceClient, creating one if
// this OCI was constructed with deferred credential loading.
func (o *OCI) EnsureClient() (generativeAIInferenceClient, error) {
	if o.Client != nil {
		return o.Client, nil
	}
	configProvider := common.DefaultConfigProvider()
	client, err := generativeaiinference.NewGenerativeAiInferenceClientWithConfigurationProvider(configProvider)
	if err != nil {
		return nil, fmt.Errorf("model: create OCI client: %w", err)
	}
	o.Client = client
	return o.Client, nil
}

// Verify sends the system prompt as a system message and userPayload as a
// single user turn to the OCI Generative AI Chat endpoint. It returns the
// text from the first text content block in the first choice, the compute
// cost in USD (always 0.0 — OCI does not always return token counts), or
// an error.
func (o *OCI) Verify(ctx context.Context, systemPrompt, userPayload string) (string, float64, error) {
	if o.ModelID == "" {
		return "", 0, fmt.Errorf("model: missing OCI model ID")
	}
	if o.CompartmentID == "" {
		return "", 0, fmt.Errorf("model: missing OCI compartment ID (set OCI_COMPARTMENT_ID)")
	}
	client, err := o.EnsureClient()
	if err != nil {
		return "", 0, err
	}

	req := generativeaiinference.ChatRequest{
		ChatDetails: generativeaiinference.ChatDetails{
			CompartmentId: common.String(o.CompartmentID),
			ServingMode: generativeaiinference.OnDemandServingMode{
				ModelId: common.String(o.ModelID),
			},
			ChatRequest: generativeaiinference.GenericChatRequest{
				Messages: []generativeaiinference.Message{
					generativeaiinference.SystemMessage{
						Content: []generativeaiinference.ChatContent{
							generativeaiinference.TextContent{
								Text: common.String(systemPrompt),
							},
						},
					},
					generativeaiinference.UserMessage{
						Content: []generativeaiinference.ChatContent{
							generativeaiinference.TextContent{
								Text: common.String(userPayload),
							},
						},
					},
				},
			},
		},
	}

	resp, err := client.Chat(ctx, req)
	if err != nil {
		// Route OCI HTTP errors through NewProviderError for the typed
		// model.Error taxonomy, matching all other native drivers.
		if se, ok := common.IsServiceError(err); ok {
			return "", 0, NewProviderError(se.GetHTTPStatusCode(), "oci", o.ModelID, nil)
		}
		return "", 0, fmt.Errorf("model: oci dispatch: %w", err)
	}

	// Extract the first choice's first text content block from the
	// polymorphic ChatResponse. OCI uses the GENERIC apiFormat for
	// standard chat.
	cr := resp.ChatResult.ChatResponse
	generic, ok := cr.(generativeaiinference.GenericChatResponse)
	if !ok {
		return "", 0, fmt.Errorf("model: unexpected OCI chat response type (expected GENERIC)")
	}
	if len(generic.Choices) == 0 {
		return "", 0, fmt.Errorf("model: empty choices in OCI response")
	}

	choice := generic.Choices[0]
	content := choice.Message.GetContent()
	if len(content) == 0 {
		return "", 0, fmt.Errorf("model: no content in OCI response message")
	}

	textContent, ok := content[0].(generativeaiinference.TextContent)
	if !ok {
		return "", 0, fmt.Errorf("model: unexpected OCI content type (expected TEXT)")
	}

	// OCI cost: token counts are optional (Usage is a pointer, nil when
	// the model doesn't return counts). Per spec, return 0.0 when absent
	// rather than an error — same posture as Azure.
	return *textContent.Text, 0, nil
}