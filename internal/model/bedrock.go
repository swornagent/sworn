package model

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// Bedrock dispatches verification calls to AWS Bedrock's Converse API using
// the aws-sdk-go-v2 bedrockruntime client. It implements Verifier.
//
// OAI-import segregation: this file imports only the AWS SDK v2 types — never
// internal/model/oai.go or any OAI struct types. The two drivers share the
// model.Error taxonomy via this package but have zero import overlap.
type Bedrock struct {
	Client  *bedrockruntime.Client
	ModelID string
	Region  string
}

// NewBedrock constructs a Bedrock driver. modelID is the Bedrock model ID
// (e.g. "anthropic.claude-sonnet-4-5"). region is the AWS region; when empty,
// resolves from AWS_REGION → AWS_DEFAULT_REGION → "us-east-1". The AWS SDK
// config is loaded from the standard credential chain (env vars →
// ~/.aws/credentials → IAM role).
func NewBedrock(modelID, region string) (*Bedrock, error) {
	if region == "" {
		region = resolveBedrockRegion()
	}
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("model: load AWS config: %w", err)
	}
	client := bedrockruntime.NewFromConfig(cfg)
	return &Bedrock{
		Client:  client,
		ModelID: modelID,
		Region:  region,
	}, nil
}

// resolveBedrockRegion returns the AWS region from env vars with a default of
// "us-east-1". Bedrock is currently US-only for most model families;
// us-east-1 is the most broadly available region.
func resolveBedrockRegion() string {
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r
	}
	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		return r
	}
	return "us-east-1"
}

// Verify sends the system prompt as a system message and userPayload as a
// single user turn to the Bedrock Converse API. It returns the text from the
// first text content block in the assistant response, the compute cost in USD,
// or an error.

// Capabilities returns CapVerify — the Bedrock driver supports single-shot
// verification. Chat is available via the Anthropic SDK path.
func (b *Bedrock) Capabilities() Capability { return CapVerify }

func (b *Bedrock) Verify(ctx context.Context, systemPrompt, userPayload string) (string, float64, int64, int64, error) {
	output, err := b.Client.Converse(ctx, &bedrockruntime.ConverseInput{
		ModelId: aws.String(b.ModelID),
		System: []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: systemPrompt},
		},
		Messages: []types.Message{
			{
				Role: types.ConversationRoleUser,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{Value: userPayload},
				},
			},
		},
	})
	if err != nil {
		// Extract HTTP status code from the error chain via
		// *smithyhttp.ResponseError. If found, route through NewProviderError
		// for the model.Error taxonomy. Otherwise return the error as-is
		// (non-HTTP / transient — IsTransient returns true for unknown types).
		var respErr *smithyhttp.ResponseError
		if errors.As(err, &respErr) {
			return "", 0, 0, 0, NewProviderError(respErr.HTTPStatusCode(), "bedrock", b.ModelID, nil)
		}
		return "", 0, 0, 0, fmt.Errorf("model: bedrock dispatch: %w", err)
	}

	// Extract the first text block from the assistant message.
	msg, ok := output.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return "", 0, 0, 0, fmt.Errorf("model: unexpected Converse output type (expected message)")
	}
	for _, block := range msg.Value.Content {
		if textBlock, ok := block.(*types.ContentBlockMemberText); ok {
			cost := computeBedrockCost(b.ModelID, output.Usage)
			return textBlock.Value, cost, 0, 0, nil
		}
	}
	return "", 0, 0, 0, fmt.Errorf("model: no text content in Bedrock response")
}

// bedrockPricing maps model IDs to USD per 1M tokens.
// Prices sourced from AWS Bedrock pricing page:
//
//	https://aws.amazon.com/bedrock/pricing/ (2026-07-08 snapshot).
//
// Model IDs use the anthropic. prefix (matching parseModelID behaviour which
// strips only the bedrock/ prefix). Nova model IDs use the amazon. prefix.
// Unknown models get zero cost (same posture as OAI, Anthropic, Google).
var bedrockPricing = map[string]struct {
	inputPricePer1M  float64
	outputPricePer1M float64
}{
	"anthropic.claude-opus-4-8": {5.00, 25.00},
	// anthropic.claude-sonnet-5: introductory $2/$10 per MTok through 2026-08-31
	// (ratified, Anthropic models-overview footnote 4). Standard rate $3/$15
	// applies AFTER 2026-08-31 — FLIP this entry to {3.00, 15.00} then.
	// Tracked: sworn#41.
	"anthropic.claude-sonnet-5":   {2.00, 10.00},
	"anthropic.claude-sonnet-4-6": {3.00, 15.00},
	"anthropic.claude-haiku-4-5":  {1.00, 5.00},
	"anthropic.claude-sonnet-4":   {3.00, 15.00},
	"amazon.nova-pro-v1:0":        {0.80, 3.20},
	"amazon.nova-lite-v1:0":       {0.06, 0.24},
}

// computeBedrockCost returns the USD cost for a verify call from token counts.
// Returns 0 for unknown models (the caller still received a verdict).
func computeBedrockCost(model string, usage *types.TokenUsage) float64 {
	if usage == nil {
		return 0
	}
	p, ok := bedrockPricing[model]
	if !ok {
		return 0
	}
	inputCost := float64(aws.ToInt32(usage.InputTokens)) / 1_000_000 * p.inputPricePer1M
	outputCost := float64(aws.ToInt32(usage.OutputTokens)) / 1_000_000 * p.outputPricePer1M
	return inputCost + outputCost
}
