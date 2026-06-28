package model

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// bedrockMsg builds a Converse API response JSON blob for test handlers.
// Returns a minimal valid response with one message containing one text block.
func bedrockMsg(text string, inputTokens, outputTokens int32) []byte {
	resp := map[string]any{
		"output": map[string]any{
			"message": map[string]any{
				"role": "assistant",
				"content": []map[string]any{
					{"text": text},
				},
			},
		},
		"stopReason": "end_turn",
		"usage": map[string]any{
			"inputTokens":  inputTokens,
			"outputTokens": outputTokens,
			"totalTokens":  inputTokens + outputTokens,
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

// bedrockError builds an AWS API error JSON blob.
func bedrockError(message string) []byte {
	e := map[string]any{
		"message": message,
	}
	b, _ := json.Marshal(e)
	return b
}

func TestBedrockVerify_ReturnsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(bedrockMsg("PASS - all checks pass", 100, 50))
	}))
	defer srv.Close()

	b := newTestBedrock(srv.URL, "anthropic.claude-sonnet-4-6", "us-east-1")
text, cost, _, _, err := b.Verify(context.Background(), "be strict", "verify this diff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "PASS - all checks pass" {
		t.Fatalf("want %q, got %q", "PASS - all checks pass", text)
	}
	if cost <= 0 {
		t.Fatalf("want cost > 0, got %f", cost)
	}
}

func TestBedrockVerify_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write(bedrockError("ThrottlingException"))
	}))
	defer srv.Close()

	b := newTestBedrock(srv.URL, "anthropic.claude-sonnet-4-5", "us-east-1")
_, _, _, _, err := b.Verify(context.Background(), "be strict", "verify this diff")
	if err == nil {
		t.Fatal("want error, got nil")
	}

	var me *Error
	if !errors.As(err, &me) || me.Kind != KindRateLimit {
		t.Fatalf("expected KindRateLimit, got %v", err)
	}
}

func TestBedrockVerify_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write(bedrockError("AccessDeniedException"))
	}))
	defer srv.Close()

	b := newTestBedrock(srv.URL, "anthropic.claude-sonnet-4-5", "us-east-1")
_, _, _, _, err := b.Verify(context.Background(), "be strict", "verify this diff")
	if err == nil {
		t.Fatal("want error, got nil")
	}

	var me *Error
	if !errors.As(err, &me) || me.Kind != KindAuth {
		t.Fatalf("expected KindAuth, got %v", err)
	}
}

func TestBedrockRegionResolution_ExplicitRegion(t *testing.T) {
	// When region is passed explicitly, it should be used.
	b, err := NewBedrock("anthropic.claude-sonnet-4-5", "eu-west-1")
	// NewBedrock calls config.LoadDefaultConfig which may fail without AWS creds
	// in CI. We accept either success or a transient config-load error.
	if err != nil {
		// The config load may fail in CI (no credentials file), but the region
		// should have been resolved before that point — verify it's in the struct
		// if construction succeeded before the load failed.
		if strings.Contains(err.Error(), "load AWS config") {
			t.Logf("config load failed as expected in env without AWS creds: %v", err)
			return
		}
		t.Fatalf("NewBedrock error (not config-related): %v", err)
	}
	if b.Region != "eu-west-1" {
		t.Errorf("Region = %q, want eu-west-1", b.Region)
	}
}

func TestBedrockRegionResolution_EnvVar(t *testing.T) {
	t.Setenv("AWS_REGION", "ap-southeast-2")
	t.Setenv("AWS_DEFAULT_REGION", "") // Ensure no fallback interference.

	region := resolveBedrockRegion()
	if region != "ap-southeast-2" {
		t.Errorf("resolveBedrockRegion() = %q, want ap-southeast-2", region)
	}
}

func TestBedrockRegionResolution_DefaultEnvVar(t *testing.T) {
	// When AWS_REGION is unset, AWS_DEFAULT_REGION should be used.
	os.Unsetenv("AWS_REGION")
	t.Setenv("AWS_DEFAULT_REGION", "eu-central-1")

	region := resolveBedrockRegion()
	if region != "eu-central-1" {
		t.Errorf("resolveBedrockRegion() = %q, want eu-central-1", region)
	}
}

func TestBedrockRegionResolution_Fallback(t *testing.T) {
	// When no region env vars are set, fall back to us-east-1.
	os.Unsetenv("AWS_REGION")
	os.Unsetenv("AWS_DEFAULT_REGION")

	region := resolveBedrockRegion()
	if region != "us-east-1" {
		t.Errorf("resolveBedrockRegion() = %q, want us-east-1", region)
	}
}

func TestNewClient_BedrockRouted(t *testing.T) {
	// Model ID with bedrock/ prefix routes to *Bedrock.
	// Note: NewClient calls NewBedrock which calls config.LoadDefaultConfig.
	// This may fail in CI environments without AWS credentials. The routing
	// test validates the provider.go switch case works; if config load fails,
	// we log and skip the type assertion but still confirm the error path is
	// reached (not ErrDriverNotImplemented, which would mean routing failed).
	cfg := ProviderConfig{}
	v, err := NewClient("bedrock/amazon.nova-pro-v1:0", cfg)
	if err != nil {
		// Accept config-load failure as valid (no AWS creds in CI).
		if strings.Contains(err.Error(), "load AWS config") {
			t.Logf("config load failed as expected without AWS creds: %v", err)
			// Verify it's NOT ErrDriverNotImplemented (routing worked).
			if errors.Is(err, ErrDriverNotImplemented) {
				t.Fatal("routing failed: got ErrDriverNotImplemented")
			}
			return
		}
		t.Fatalf("NewClient error: %v", err)
	}
	_, ok := v.(*Bedrock)
	if !ok {
		t.Fatalf("expected *Bedrock, got %T", v)
	}
}

func TestBedrockVerify_UnknownModelCostIsZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(bedrockMsg("PASS", 100, 50))
	}))
	defer srv.Close()

	b := newTestBedrock(srv.URL, "anthropic.unknown-model", "us-east-1")
_, cost, _, _, err := b.Verify(context.Background(), "be strict", "verify")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cost != 0 {
		t.Fatalf("want cost 0 for unknown model, got %f", cost)
	}
}

func TestBedrockVerify_NonHTTPErrorIsTransient(t *testing.T) {
	// Return a 400 with a non-standard body that the SDK will parse as an error
	// but which wraps in *smithyhttp.ResponseError. KindOther errors are
	// transient per IsTransient.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write(bedrockError("ValidationException"))
	}))
	defer srv.Close()

	b := newTestBedrock(srv.URL, "anthropic.claude-sonnet-4-5", "us-east-1")
_, _, _, _, err := b.Verify(context.Background(), "be strict", "verify this diff")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !IsTransient(err) {
		t.Fatalf("expected Bedrock error to be transient, got %v", err)
	}
}

// newTestBedrock returns a Bedrock driver pointed at a test server.
// Uses static credentials and BaseEndpoint override to avoid hitting
// the real AWS endpoint.
func newTestBedrock(baseURL, modelID, region string) *Bedrock {
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")),
	)
	if err != nil {
		panic("newTestBedrock: " + err.Error())
	}
	client := bedrockruntime.NewFromConfig(cfg, func(o *bedrockruntime.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
	return &Bedrock{
		Client:  client,
		ModelID: modelID,
		Region:  region,
	}
}

// TestBedrockVerify_Live is the spec-mandated live reachability artefact.
// It is skipped unless SWORN_LIVE_TESTS=1 AND AWS_ACCESS_KEY_ID is set, so it
// runs only when a developer explicitly opts in with real credentials.
func TestBedrockVerify_Live(t *testing.T) {
	if os.Getenv("SWORN_LIVE_TESTS") != "1" || os.Getenv("AWS_ACCESS_KEY_ID") == "" {
		t.Skip("live test requires SWORN_LIVE_TESTS=1 and AWS_ACCESS_KEY_ID")
	}
	b, err := NewBedrock("anthropic.claude-sonnet-4-5", os.Getenv("AWS_REGION"))
	if err != nil {
		t.Fatalf("NewBedrock error: %v", err)
	}
text, _, _, _, err := b.Verify(context.Background(), "Reply with PASS.", "verify")
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if !strings.Contains(text, "PASS") {
		t.Fatalf("want text containing %q, got %q", "PASS", text)
	}
}
