package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Ollama dispatches verification calls to a local Ollama instance via its
// native POST /api/chat endpoint (not the OAI-compat /v1/chat/completions shim).
// It implements Verifier using stdlib net/http + encoding/json (zero new deps).
type Ollama struct {
	// Host is the Ollama server base URL, e.g. http://localhost:11434.
	Host string
	// Model is the Ollama model name, e.g. llama3.2.
	Model string
	// Client is the HTTP client for dispatching requests. nil means
	// http.DefaultClient.
	Client *http.Client
}

// NewOllama constructs an Ollama driver for the given model name and host.
// If host is empty, the $OLLAMA_HOST env var is read; if that is also empty,
// http://localhost:11434 is used. The NewClient path always supplies
// pcfg.OllamaHost (non-empty) — the $OLLAMA_HOST fallback handles direct
// construction (tests, standalone use).
func NewOllama(modelID, host string) *Ollama {
	if host == "" {
		host = ollamaHost()
	}
	return &Ollama{
		Host:  host,
		Model: modelID,
	}
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Stream   bool            `json:"stream"`
	Messages []ollamaMessage `json:"messages"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatResponse struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done            bool   `json:"done"`
	Error           string `json:"error,omitempty"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
}

// Verify sends the system prompt and user payload to Ollama's /api/chat.
// It returns the model's response text, a cost of 0.0 (Ollama is free), or
// an error on non-200 status or an "error" field in the JSON response.
func (o *Ollama) Verify(ctx context.Context, systemPrompt, userPayload string) (string, float64, int64, int64, error) {
	reqBody := ollamaChatRequest{
		Model:  o.Model,
		Stream: false,
		Messages: []ollamaMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPayload},
		},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(reqBody); err != nil {
		return "", 0, 0, 0, fmt.Errorf("ollama: marshal request: %w", err)
	}

	url := strings.TrimRight(o.Host, "/") + "/api/chat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("ollama: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := o.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("ollama: dispatch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, 0, 0, fmt.Errorf("ollama: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, 0, 0, fmt.Errorf("ollama: non-ok status %d: %s", resp.StatusCode, string(body))
	}

	var cr ollamaChatResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return "", 0, 0, 0, fmt.Errorf("ollama: unmarshal response: %w", err)
	}

	if cr.Error != "" {
		return "", 0, 0, 0, fmt.Errorf("ollama: %s", cr.Error)
	}

	return cr.Message.Content, 0, 0, 0, nil
}
