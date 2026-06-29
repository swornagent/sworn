package captain

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/model"
)

// fakeAgent is a minimal agent.Agent that returns a canned ChatResponse.
type fakeAgent struct {
	text string
	err  error
}

func (f fakeAgent) Chat(_ context.Context, _ []model.ChatMessage, _ []model.ToolDef) (*model.ChatResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &model.ChatResponse{
		Choices: []struct {
			Message struct {
				Content   string           `json:"content"`
				ToolCalls []model.ToolCall `json:"tool_calls,omitempty"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		}{
			{
				Message: struct {
					Content   string           `json:"content"`
					ToolCalls []model.ToolCall `json:"tool_calls,omitempty"`
				}{Content: f.text},
				FinishReason: "stop",
			},
		},
	}, nil
}

const cannedEscalateOutput = `Pins:
1. [escalate] §3.files.1 — design references non-existent file
   What I observed: §3 claims "internal/nonexistent/thing.go" but the file does not exist in the repo.
   What to ask the implementer: Confirm the file path or remove the reference.
2. [mechanical] §2.decisions.1 — missing rationale
   What I observed: Decision "Use a map" has no stated rationale.
   What to ask the implementer: State why a map over alternatives.

Pins: 2 total — 1 mechanical, 0 memory-cited, 1 escalate
Critical pins: 1 (would ship broken if unaddressed)
`

const cannedCleanOutput = `Pins:
1. [mechanical] §2.decisions.1 — missing rationale
   What I observed: Decision "Use a map" has no stated rationale.
   What to ask the implementer: State why a map over alternatives.
2. [memory-cited] §2.decisions.2 — aligns with memory
   What I observed: Decision "Encrypt PII" aligns with project PII encryption policy.
   Citation: [[project_pii_encryption]]

Pins: 2 total — 1 mechanical, 1 memory-cited, 0 escalate
Critical pins: none
`

func TestEscalatePinHalts(t *testing.T) {
	dir := t.TempDir()
	spec := "# Test spec\n\n## User outcome\n\nTest.\n"
	design := "## §1 User-visible change\n\nTest change.\n"

	fa := fakeAgent{text: cannedEscalateOutput}
	result, err := Review(context.Background(), dir, spec, design, fa, "/tmp/wt")
	if err != nil {
		t.Fatalf("Review: %v", err)
	}

	if !result.HasEscalatePins {
		t.Fatal("expected HasEscalatePins=true, got false")
	}
	if result.EscalateCount != 1 {
		t.Fatalf("expected 1 escalate pin, got %d", result.EscalateCount)
	}
	if len(result.Pins) < 2 {
		t.Fatalf("expected at least 2 pins, got %d", len(result.Pins))
	}

	// Verify review.md was written.
	reviewPath := filepath.Join(dir, "review.md")
	data, err := os.ReadFile(reviewPath)
	if err != nil {
		t.Fatalf("review.md not written: %v", err)
	}
	if !strings.Contains(string(data), cannedEscalateOutput) {
		t.Fatalf("review.md content mismatch\ngot:\n%s\nwant (contains):\n%s", string(data), cannedEscalateOutput)
	}
}

func TestCleanDesignProceeds(t *testing.T) {
	dir := t.TempDir()
	spec := "# Test spec\n\n## User outcome\n\nTest.\n"
	design := "## §1 User-visible change\n\nTest change.\n"

	fa := fakeAgent{text: cannedCleanOutput}
	result, err := Review(context.Background(), dir, spec, design, fa, "/tmp/wt")
	if err != nil {
		t.Fatalf("Review: %v", err)
	}

	if result.HasEscalatePins {
		t.Fatal("expected HasEscalatePins=false (clean design), got true")
	}
	if result.EscalateCount != 0 {
		t.Fatalf("expected 0 escalate pins, got %d", result.EscalateCount)
	}

	// Verify review.md was written.
	reviewPath := filepath.Join(dir, "review.md")
	if _, err := os.Stat(reviewPath); os.IsNotExist(err) {
		t.Fatal("review.md was not written for clean design")
	}

	// FormatPinsAsFeedback should exclude escalate and include others.
	feedback := result.FormatPinsAsFeedback()
	if !strings.Contains(feedback, "[mechanical]") {
		t.Fatal("feedback should contain mechanical pin")
	}
	if !strings.Contains(feedback, "[memory-cited]") {
		t.Fatal("feedback should contain memory-cited pin")
	}
	if strings.Contains(feedback, "[escalate]") {
		t.Fatal("feedback should NOT contain escalate pin")
	}
}

func TestPinsClassified(t *testing.T) {
	dir := t.TempDir()
	spec := "# Test spec\n\n## User outcome\n\nTest.\n"
	design := "## §1 User-visible change\n\nTest change.\n"

	// A response with all three pin types.
	mixedOutput := `Pins:
1. [mechanical] §2.1 — missing rationale
   What I observed: No rationale for choice.
   What to ask the implementer: Add rationale.
2. [memory-cited] §2.2 — aligns with PII policy
   What I observed: Aligns with project_pii_encryption.
   Citation: [[project_pii_encryption]]
3. [escalate] §3.1 — references non-existent file
   What I observed: Non-existent file reference.
   What to ask the implementer: Fix the reference.

Pins: 3 total — 1 mechanical, 1 memory-cited, 1 escalate
`

	fa := fakeAgent{text: mixedOutput}
	result, err := Review(context.Background(), dir, spec, design, fa, "/tmp/wt")
	if err != nil {
		t.Fatalf("Review: %v", err)
	}

	if !result.HasEscalatePins {
		t.Fatal("expected HasEscalatePins=true")
	}
	if result.EscalateCount != 1 {
		t.Fatalf("expected 1 escalate pin, got %d", result.EscalateCount)
	}

	// Count pin tags.
	var mech, mem, esc int
	for _, p := range result.Pins {
		switch p.Tag {
		case Mechanical:
			mech++
		case MemoryCited:
			mem++
		case Escalate:
			esc++
		}
	}
	if mech < 1 {
		t.Fatalf("expected at least 1 mechanical pin, got %d", mech)
	}
	if mem < 1 {
		t.Fatalf("expected at least 1 memory-cited pin, got %d", mem)
	}
	if esc < 1 {
		t.Fatalf("expected at least 1 escalate pin, got %d", esc)
	}

	// review.md must contain pin classification tags.
	reviewPath := filepath.Join(dir, "review.md")
	data, err := os.ReadFile(reviewPath)
	if err != nil {
		t.Fatalf("review.md not found: %v", err)
	}
	for _, tag := range []string{"[mechanical]", "[memory-cited]", "[escalate]"} {
		if !strings.Contains(string(data), tag) {
			t.Fatalf("review.md missing tag %s", tag)
		}
	}
}

func TestReviewModelError(t *testing.T) {
	dir := t.TempDir()
	spec := "# Test\n\n## User outcome\n\nTest.\n"
	design := "## §1 User-visible change\n\nTest.\n"

	fa := fakeAgent{err: model.ErrNotConfigured}
	_, err := Review(context.Background(), dir, spec, design, fa, "/tmp/wt")
	if err == nil {
		t.Fatal("expected error from model, got nil")
	}
}

func TestFormatPinsAsFeedbackNil(t *testing.T) {
	var r *ReviewResult
	if got := r.FormatPinsAsFeedback(); got != "" {
		t.Fatalf("expected empty string for nil result, got %q", got)
	}

	r = &ReviewResult{}
	if got := r.FormatPinsAsFeedback(); got != "" {
		t.Fatalf("expected empty string for empty result, got %q", got)
	}
}

// TestSummaryLineNotCountedAsEscalate locks the #34 fix: the captain's summary
// line ("Pins: N total — … 0 [escalate]") contains the "[escalate]" substring
// and must NOT be counted as an escalate pin. The real 2026-06-29 review had
// the summary twice (body + suggested-ack) → the old substring scan counted 2
// phantom escalate pins and halted a run with zero real escalate pins.
func TestSummaryLineNotCountedAsEscalate(t *testing.T) {
	dir := t.TempDir()
	spec := "# Test spec\n\n## User outcome\n\nTest.\n"
	design := "## §1 User-visible change\n\nTest change.\n"

	canned := "## Pins\n\n" +
		"1. [mechanical] §4.1 — Out-of-scope tool-use described as tracked but no issue number cited.\n\n" +
		"Pins: 1 total — 1 [mechanical], 0 [memory-cited], 0 [escalate]\n\n" +
		"## Suggested acknowledgement reply\n\n" +
		"Pins: 1 total — 1 [mechanical], 0 [memory-cited], 0 [escalate]\n" +
		"1. **Missing tracker** — file the issue and cite the number.\n"

	fa := fakeAgent{text: canned}
	result, err := Review(context.Background(), dir, spec, design, fa, "/tmp/wt")
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if result.EscalateCount != 0 {
		t.Fatalf("expected 0 escalate pins (summary lines must not count), got %d", result.EscalateCount)
	}
	if result.HasEscalatePins {
		t.Fatal("expected HasEscalatePins=false, got true")
	}
	if len(result.Pins) != 1 || result.Pins[0].Tag != Mechanical {
		t.Fatalf("expected exactly 1 mechanical pin, got %d pins: %+v", len(result.Pins), result.Pins)
	}
}
