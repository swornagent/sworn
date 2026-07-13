// Package design produces pre-implementation design artefacts (the design-TL;DR,
// S45) and the design-review stage for the sworn run loop.
//
// Stdlib only — zero runtime dependencies beyond the internal packages.
package design

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/prompt"
)

// ErrStructuredUnsupported is returned by Generate when the captain model
// cannot emit structured output (no StructuredOutput capability). It is a
// DECLARED Rule 2 deferral signal, NOT a hard failure: the loop caller
// (internal/run/slice.go) surfaces it as a machine-readable design-gate
// deferral naming the missing capability (S02 AC-03), never a silent pass and
// never a hard prose-format failure. The sentinel wraps the driver's
// ErrKindUnsupported error so callers can errors.Is on it.
var ErrStructuredUnsupported = errors.New("design: captain model does not support structured output (capability absent) — Rule 9 design gate deferred")

// designTLDR is the typed structured design object the captain model emits via
// ChatStructured (S02 D1/D4). Each field carries one of the six required
// sections of the design TL;DR. Replaces the prior prose §1–§6 header scrape:
// the schema is the contract, not the prose shape, so a model that returns
// valid structured content passes regardless of prose formatting (the exact
// Grok failure this slice fixes).
type designTLDR struct {
	UserVisibleChange string `json:"user_visible_change"`
	DesignDecisions   string `json:"design_decisions"`
	FilesTouched      string `json:"files_touched"`
	NotDoing          string `json:"not_doing"`
	ReachabilityPlan  string `json:"reachability_plan"`
	OpenQuestions     string `json:"open_questions"`
}

// designEmitSchema is the sworn-local emit schema handed to ChatStructured
// (S02 D2, Coach-confirmed inline-emit-only — the design TL;DR is an artefact,
// not a fail-closed gate, so no canonical validate-schema). It stays inside
// OpenAI's strict-mode keyword subset — no minLength/pattern/format (those
// break a strict response_format target; see internal/model/structured.go
// strict-projection). All six sections are required (schema-enforced presence);
// non-emptiness is enforced in Go (complete) since strict mode forbids
// minLength. The "title" sets the OpenAI json_schema name (^[a-zA-Z0-9_-]+$).
var designEmitSchema = []byte(`{
  "title": "design-tldr",
  "type": "object",
  "additionalProperties": false,
  "required": [
    "user_visible_change",
    "design_decisions",
    "files_touched",
    "not_doing",
    "reachability_plan",
    "open_questions"
  ],
  "properties": {
    "user_visible_change": { "type": "string", "description": "§1 — one sentence: what the user sees or experiences differently after this slice lands (or the observable behaviour that proves it live)." },
    "design_decisions": { "type": "string", "description": "§2 — at most 5 design decisions the spec does not specify, one bullet each (data structure, algorithm, package boundary, interface shape, error posture, naming)." },
    "files_touched": { "type": "string", "description": "§3 — bulleted list of files (paths from repo root) and the purpose of each change." },
    "not_doing": { "type": "string", "description": "§4 — bulleted list of work that might be expected but is out of scope, each citing the spec section that rules it out." },
    "reachability_plan": { "type": "string", "description": "§5 — one sentence naming the integration point and the concrete gesture or test command that proves the change is live." },
    "open_questions": { "type": "string", "description": "§6 — at most 3 questions needed before/early in implementation, one bullet each; \"None.\" if there are none." }
  }
}`)

// GenerateOptions configures the Generate call.
type GenerateOptions struct {
	// Regenerate forces overwrite of an existing design.md. When false and
	// design.md already exists, Generate is a no-op and returns ("", nil).
	Regenerate bool
}

// Generate produces a design TL;DR (design.md) in sliceDir from the given
// spec content via a single schema-constrained Role=captain driver dispatch
// (S02 rewire): prompt assembly stays here (orchestrator-side); the model call
// happens behind the driver, which serves the captain dispatch tool-lessly and
// — because StructuredSchema is set — constrains the model to emit a JSON
// object conforming to designEmitSchema. Generate parses the typed object,
// requires all six sections non-empty (acceptance semantics preserved verbatim
// from the prior hasSixSections scrape, D4), and renders design.md
// deterministically from the fields so /design-review still reads a prose doc.
// Returns the rendered design text (or "" if skipped).
//
// Idempotency: if design.md already exists and opts.Regenerate is false,
// Generate returns ("", nil) without dispatching.
//
// Capability-absent (S02 AC-03): if the model cannot emit structured output,
// Generate returns ErrStructuredUnsupported (errors.Is-matchable) so the loop
// caller records a declared Rule 2 deferral — never a silent pass, never a
// hard prose-format failure.
func Generate(ctx context.Context, sliceDir, spec string, d driver.Driver, modelID, worktreeRoot string, timeout time.Duration, opts GenerateOptions) (string, error) {
	designPath := filepath.Join(sliceDir, "design.md")

	if _, err := os.Stat(designPath); err == nil && !opts.Regenerate {
		// design.md exists and regenerate not requested — skip.
		return "", nil
	}

	res, err := d.Dispatch(ctx, driver.DispatchInput{
		Role:             driver.RoleCaptain,
		ModelID:          modelID,
		SystemPrompt:     prompt.DesignTLDR(),
		Payload:          fmt.Sprintf("Spec:\n\n%s", spec),
		WorktreeRoot:     worktreeRoot,
		StructuredSchema: designEmitSchema,
		Timeout:          timeout,
	})
	if err != nil {
		// Capability-absent diverges from every other dispatch failure: it is a
		// declared Rule 2 deferral, not a hard error (S02 D3, AC-03).
		if res.ErrKind == driver.ErrKindUnsupported {
			return "", fmt.Errorf("%w: %v", ErrStructuredUnsupported, err)
		}
		return "", fmt.Errorf("design: dispatch: %w", err)
	}
	if res.Status != driver.StatusOK || len(res.StructuredJSON) == 0 {
		return "", fmt.Errorf("design: empty structured response from model")
	}

	var td designTLDR
	if err := json.Unmarshal(res.StructuredJSON, &td); err != nil {
		return "", fmt.Errorf("design: parse structured design object: %w", err)
	}

	// Acceptance semantics preserved verbatim (D4): all six sections must be
	// present AND non-empty. Schema strict-mode enforces presence; non-emptiness
	// is enforced here (strict mode forbids minLength), replacing the prior
	// hasSixSections substring scrape.
	if missing := td.missingSections(); len(missing) > 0 {
		return "", fmt.Errorf("design: model response missing required sections (empty: %s)", strings.Join(missing, ", "))
	}

	text := td.render()

	// Write design.md.
	if err := os.WriteFile(designPath, []byte(text), 0o644); err != nil {
		return "", fmt.Errorf("design: write design.md: %w", err)
	}

	return text, nil
}

// section pairs a field's rendered §N heading with its content, in canonical
// order. Single source of truth for both the non-empty check and the render.
func (t designTLDR) sections() []struct {
	heading string
	content string
} {
	return []struct {
		heading string
		content string
	}{
		{"## §1 User-visible change", t.UserVisibleChange},
		{"## §2 Design decisions not in the spec", t.DesignDecisions},
		{"## §3 Files I'll touch by purpose", t.FilesTouched},
		{"## §4 Things I'm NOT doing", t.NotDoing},
		{"## §5 Reachability plan", t.ReachabilityPlan},
		{"## §6 Open questions", t.OpenQuestions},
	}
}

// missingSections returns the headings of any section whose content is empty
// (whitespace-only counts as empty). A passing design has none — the
// schema-enforced six-fields-present check plus this non-empty check together
// reproduce the prior hasSixSections gate semantics (D4).
func (t designTLDR) missingSections() []string {
	var missing []string
	for _, s := range t.sections() {
		if strings.TrimSpace(s.content) == "" {
			missing = append(missing, s.heading)
		}
	}
	return missing
}

// render deterministically builds the human-readable design.md from the typed
// structured fields (D4): the §1–§6 headers are GENERATED, not scraped, so
// /design-review still reads a prose document regardless of how the model
// formatted (or did not format) its own prose.
func (t designTLDR) render() string {
	var b strings.Builder
	for i, s := range t.sections() {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(s.heading)
		b.WriteString("\n\n")
		b.WriteString(strings.TrimSpace(s.content))
		b.WriteString("\n")
	}
	return b.String()
}
