// Package design produces pre-implementation design artefacts (the design-TL;DR,
// S45) and the design-review stage for the sworn run loop.
//
// Stdlib only — zero runtime dependencies beyond the internal packages.
package design

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/prompt"
)

// GenerateOptions configures the Generate call.
type GenerateOptions struct {
	// Regenerate forces overwrite of an existing design.md. When false and
	// design.md already exists, Generate is a no-op and returns ("", nil).
	Regenerate bool
}

// Generate produces a design TL;DR (design.md) in sliceDir from the given
// spec content. It uses a for a single-shot, tool-less model call — no agent
// loop, no file writes by the model itself. Returns the generated design
// text (or "" if skipped).
//
// Idempotency: if design.md already exists and opts.Regenerate is false,
// Generate returns ("", nil) without calling the model.
func Generate(ctx context.Context, sliceDir, spec string, a agent.Agent, opts GenerateOptions) (string, error) {
	designPath := filepath.Join(sliceDir, "design.md")

	if _, err := os.Stat(designPath); err == nil && !opts.Regenerate {
		// design.md exists and regenerate not requested — skip.
		return "", nil
	}

	systemPrompt := prompt.DesignTLDR()
	userPayload := fmt.Sprintf("Spec:\n\n%s", spec)

	messages := []model.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPayload},
	}

	// Tool-less call: pass nil tools so the model cannot request tool use.
	resp, err := a.Chat(ctx, messages, nil)
	if err != nil {
		return "", fmt.Errorf("design: model call: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("design: empty response from model")
	}

	text := resp.Choices[0].Message.Content

	// Sanity: the response must contain all six § headers.
	if !hasSixSections(text) {
		return "", fmt.Errorf("design: model response missing required sections (need §1–§6 headers)")
	}

	// Write design.md.
	if err := os.WriteFile(designPath, []byte(text), 0o644); err != nil {
		return "", fmt.Errorf("design: write design.md: %w", err)
	}

	return text, nil
}

// hasSixSections checks that text contains all six § section markers. The
// markers are the heading prefixes: "§1", "§2", "§3", "§4", "§5", "§6".
// The check is substring-based — an LLM may produce "## §1 …" or "# §1 …"
// or just "§1 …" — any occurrence of the marker counts.
func hasSixSections(text string) bool {
	for _, marker := range []string{"§1", "§2", "§3", "§4", "§5", "§6"} {
		if !strings.Contains(text, marker) {
			return false
		}
	}
	return true
}
