// Package captain implements the captain design-review stage (S46) for the
// sworn run loop. It reads the design TL;DR (design.md from S45), the spec,
// and live code surfaces, then emits classified pins — mechanical,
// memory-cited, or escalate — and writes review.md. The run loop gates on
// escalate pins: none → proceed to implement; any → halt.
//
// Stdlib only — zero runtime dependencies beyond the internal packages.
package captain

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/agent"
	"github.com/swornagent/sworn/internal/model"
	"github.com/swornagent/sworn/internal/prompt"
)

// PinTag classifies a captain review pin.
type PinTag string

const (
	Mechanical  PinTag = "mechanical"
	MemoryCited PinTag = "memory-cited"
	Escalate    PinTag = "escalate"
)

// Pin is one captain review finding.
type Pin struct {
	Number      int
	Tag         PinTag
	Summary     string
	Observation string
	Action      string
	Citation    string // memory name, for memory-cited pins
}

// ReviewResult is the structured output of a captain review.
type ReviewResult struct {
	Pins            []Pin
	EscalateCount   int
	HasEscalatePins bool
	RawOutput       string  // full model output for review.md
	CostUSD         float64 // dispatch cost from token usage; 0 if unpriced
}

// Review runs the captain design-review for one slice. It takes the TL;DR
// (design.md content), the spec, and a model agent, prompts the captain to
// review the design, parses the pin list, and writes review.md to sliceDir.
//
// On success, review.md is written to sliceDir. On model error, the function
// returns an error; the caller may proceed without review or halt.
func Review(ctx context.Context, sliceDir, spec, design string, a agent.Agent, worktreeRoot string) (*ReviewResult, error) {
	// S19-captain-split: dispatch under the design-reviewer identity, not the
	// conflated captain.md (vendored verbatim from upstream, still carries the
	// release-orchestrator function the deterministic engine owns).
	systemPrompt := prompt.DesignReviewer()

	// Build the user payload: spec + design + worktree context.
	var userPayload strings.Builder
	userPayload.WriteString("Design review for a slice in the sworn run loop.\n\n")
	userPayload.WriteString("## Spec\n\n")
	userPayload.WriteString(spec)
	userPayload.WriteString("\n\n## Design TL;DR (design.md)\n\n")
	userPayload.WriteString(design)
	userPayload.WriteString("\n\n## Context\n")
	userPayload.WriteString(fmt.Sprintf("Slice directory: %s\n", sliceDir))
	userPayload.WriteString(fmt.Sprintf("Worktree root: %s\n", worktreeRoot))
	userPayload.WriteString("\nReview the design against the spec and project memory. ")
	userPayload.WriteString("Produce the pin list, review.md content, and suggested acknowledgement reply ")
	userPayload.WriteString("as described in the /design-review function.\n")

	messages := []model.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPayload.String()},
	}

	// Tool-less call — the captain reads artefacts, doesn't write code.
	resp, err := a.Chat(ctx, messages, nil)
	if err != nil {
		return nil, fmt.Errorf("captain: model call: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("captain: empty response from model")
	}

	text := resp.Choices[0].Message.Content

	// Compute dispatch cost from token usage (same nominal estimate as
	// agent.computeCost — $2/1M tokens).  An unpriced model or nil usage
	// yields 0, treated downstream as "no cost signal."
	var costUSD float64
	if resp.Usage != nil && resp.Usage.TotalTokens > 0 {
		costUSD = float64(resp.Usage.TotalTokens) * 0.000002
	}

	// Parse pins from the model output.
	result := parsePins(text)
	result.CostUSD = costUSD
	// Write review.md.
	reviewPath := filepath.Join(sliceDir, "review.md")
	reviewContent := buildReviewMD(sliceDir, text, result)
	if err := os.WriteFile(reviewPath, []byte(reviewContent), 0o644); err != nil {
		return nil, fmt.Errorf("captain: write review.md: %w", err)
	}

	return result, nil
}

// parsePins scans the model output for pin lines matching the captain's
// output format: <n>. [<tag>] §<section>.<bullet> — <summary>
//
// It also counts escalate pins and sets HasEscalatePins.
// pinLineRe matches a real captain pin line: "<n>. [<tag>] …" (per captain.md).
// Anchoring on the numbered-pin format is essential — it excludes (a) the
// summary line "Pins: N total — a [mechanical], … c [escalate]" which itself
// contains the "[escalate]" substring, and (b) the "## Suggested acknowledgement
// reply" restatements and any prose mentions of a tag. Counting bare substrings
// (the old behaviour) miscounted the summary line(s) as escalate pins and
// halted runs that had zero real escalate pins (#34).
var pinLineRe = regexp.MustCompile(`^\d+\.\s+\[(escalate|mechanical|memory-cited)\]`)

func parsePins(text string) *ReviewResult {
	result := &ReviewResult{RawOutput: text}
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		m := pinLineRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}

		pin := Pin{}

		// Determine tag from the captured group (not a substring scan).
		switch m[1] {
		case "escalate":
			pin.Tag = Escalate
			result.EscalateCount++
		case "memory-cited":
			pin.Tag = MemoryCited
		case "mechanical":
			pin.Tag = Mechanical
		}

		// Extract summary: text after " — " on this line.
		if dashIdx := strings.Index(trimmed, " — "); dashIdx >= 0 {
			pin.Summary = strings.TrimSpace(trimmed[dashIdx+len(" — "):])
		} else {
			pin.Summary = trimmed
		}

		// Extract observation: look for "What I observed:" in subsequent lines.
		// Extract action: look for "What to ask the implementer:" in subsequent lines.

		result.Pins = append(result.Pins, pin)
	}

	result.HasEscalatePins = result.EscalateCount > 0
	return result
}

// buildReviewMD constructs the review.md content: a header, the full model
// output, and a date stamp.
func buildReviewMD(sliceDir string, rawOutput string, _ *ReviewResult) string {
	var b strings.Builder
	b.WriteString("# Captain review — ")
	b.WriteString(filepath.Base(sliceDir))
	b.WriteByte('\n')
	b.WriteString("Date: ")
	b.WriteString(time.Now().UTC().Format(time.RFC3339))
	b.WriteString("\n\n")
	b.WriteString(rawOutput)
	return b.String()
}

// FormatPinsAsFeedback formats mechanical and memory-cited pins as
// implementer feedback suitable for injection into the implement prompt
// (S44 mechanism). Escalate pins are excluded — they halt the run.
func (r *ReviewResult) FormatPinsAsFeedback() string {
	if r == nil || len(r.Pins) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Captain design-review pins to address during implementation:\n\n")
	for _, p := range r.Pins {
		if p.Tag == Escalate {
			continue
		}
		b.WriteString(fmt.Sprintf("- [%s] %s\n", p.Tag, p.Summary))
	}
	return b.String()
}
