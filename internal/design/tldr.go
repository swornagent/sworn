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
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/prompt"
)

// GenerateOptions configures the Generate call.
type GenerateOptions struct {
	// Regenerate forces overwrite of an existing design.md. When false and
	// design.md already exists, Generate is a no-op and returns ("", nil).
	Regenerate bool
}

// Generate produces a design TL;DR (design.md) in sliceDir from the given
// spec content via a single Role=captain driver dispatch (S06 rewire):
// prompt assembly stays here (orchestrator-side); the model call happens
// behind the driver, which serves captain dispatches tool-lessly — no agent
// loop, no file writes by the model itself. Returns the generated design
// text (or "" if skipped).
//
// Idempotency: if design.md already exists and opts.Regenerate is false,
// Generate returns ("", nil) without dispatching.
func Generate(ctx context.Context, sliceDir, spec string, d driver.Driver, modelID, worktreeRoot string, timeout time.Duration, opts GenerateOptions) (string, error) {
	designPath := filepath.Join(sliceDir, "design.md")

	if _, err := os.Stat(designPath); err == nil && !opts.Regenerate {
		// design.md exists and regenerate not requested — skip.
		return "", nil
	}

	res, err := d.Dispatch(ctx, driver.DispatchInput{
		Role:         driver.RoleCaptain,
		ModelID:      modelID,
		SystemPrompt: prompt.DesignTLDR(),
		Payload:      fmt.Sprintf("Spec:\n\n%s", spec),
		WorktreeRoot: worktreeRoot,
		Timeout:      timeout,
	})
	if err != nil {
		return "", fmt.Errorf("design: dispatch: %w", err)
	}
	if res.Status != driver.StatusOK || strings.TrimSpace(res.ResultText) == "" {
		return "", fmt.Errorf("design: empty response from model")
	}

	text := res.ResultText

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
