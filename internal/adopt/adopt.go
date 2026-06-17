// Package adopt materialises the Baton protocol into a target repo:
// writing docs/baton/ (rules + VERSION) and splicing the seven-rule
// fragment into AGENTS.md. Both operations are idempotent.
package adopt

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed baton/README.md baton/VERSION baton/rules/*
var batonFS embed.FS

// BatonSectionHeading is the marker heading that identifies the Baton
// section in AGENTS.md. It must match the heading used by the splice logic
// exactly.
const BatonSectionHeading = "## Engineering Process — Baton"

// batonAGENTSFragment is the seven-rule fragment spliced into AGENTS.md.
// It is hardcoded as a Go constant (Coach Pin 6) — the rules are stable
// text from the open Baton protocol and change only at protocol version
// bumps, which are explicit re-vendor operations. This is the same content
// as the project's own AGENTS.md ## Engineering Process — Baton section.
const batonAGENTSFragment = `## Engineering Process — Baton

This project follows the **Baton** rule-set (see ` + "`docs/baton/`" + ` for
full rule docs and provenance). Seven rules, listed in priority order:

### 1. Reachability Gate (CRITICAL)

For any feature with a user-facing affordance (UI control, route, form field,
API endpoint), the first failing test must render through the integration point
that owns the affordance — NOT the leaf component in isolation.

- If the integration point can't render the feature yet, THAT failure is the
  correct TDD red. Build the integration glue first; the leaf falls out.
- Leaf-level unit tests are fine in addition. They cannot be the sole proof of
  life.
- A component imported only by its own test file is a red flag. Investigate
  before claiming task done.

Before marking any phase complete, produce a **reachability artefact**:
screenshot, end-to-end test run, or explicit "open browser, do X, observe Y"
smoke step. A green typecheck plus green unit suite is not a reachability
artefact.

### 2. No Silent Deferrals

"Deferred" as an inline code comment is not a decision unless all three are
present: **why** (concrete reason), **tracking** (linked issue, plan task, or
punch-list item), **acknowledgement** (decision-maker told in plain text).
Without all three, the inline comment is rationalisation, not decision.

### 3. Capture Discipline

Conversation context is the most ephemeral persistence layer. Subagent findings
and session decisions must land in durable storage before session ends.

**Durability hierarchy (most to least permanent):** git history → code →
` + "`docs/`" + ` → GitHub Issues → project memory → conversation context.

Bias every capture decision toward higher-permanence layers. Conversation
context is a working surface, not a storage surface.

### 4. Commit Messages as Capture Layer

Commits that land a documented decision MUST restate the decision in the
message body, not just "see plan X." Plans get edited and moved; ` + "`git log`" + `
is permanent. Use 3–5 line bodies for any commit landing a decision.

### 5. Session Discipline

Implementation sessions of non-trivial scope are anchored to GitHub Issues.
Session start: confirm the issue. During session: capture decisions at natural
breakpoints. Session end: record decisions, completed work, deferred items,
next steps.

### 6. Proof Bundle (CRITICAL)

Before marking any task, phase, or session complete, produce a **proof bundle**
at the appropriate path, generated from **live repo state** — not recalled from
context. Required sections: Scope, Files changed, Test results, Reachability
artefact, Delivered, Not delivered, Divergence from plan.

Claiming completion without a proof bundle is a silent deferral of verification
(Rule 2).

### 7. Adversarial Verification (CRITICAL)

No slice may transition to verified state without a PASS verdict from a
**fresh-context session** loaded only with the slice artefacts and live repo
state. The session that implemented the work is never allowed to certify it.

Verifier return contract: exactly one of PASS, FAIL (with numbered violations),
or BLOCKED (with reason). Fail closed — absence of evidence is FAIL, not
optimistic PASS. The verifier does not propose redesigns, does not edit
production code, and does not consult the implementer for clarification.

State machine: planned → in_progress → implemented → [fresh verifier] →
verified | failed_verification. The implemented checkpoint exists so no agent
can shortcut directly to verified.

Full rule docs: ` + "`docs/baton/`" + `.`

// Materialise writes the Baton protocol docs (rules + README + VERSION) into
// the target repo at <repoRoot>/docs/baton/. Existing files are overwritten
// with the embedded copies. The VERSION file records the vendored protocol
// version string.
func Materialise(repoRoot string) error {
	batonDir := filepath.Join(repoRoot, "docs", "baton")
	rulesDir := filepath.Join(batonDir, "rules")

	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		return fmt.Errorf("adopt: mkdir %s: %w", rulesDir, err)
	}

	// Files to materialise (embedded path -> filesystem path).
	files := []struct {
		src  string
		dst  string
		perm os.FileMode
	}{
		{"baton/README.md", filepath.Join(batonDir, "README.md"), 0644},
		{"baton/VERSION", filepath.Join(batonDir, "VERSION"), 0644},
		{"baton/rules/01-reachability-gate.md", filepath.Join(rulesDir, "01-reachability-gate.md"), 0644},
		{"baton/rules/02-no-silent-deferrals.md", filepath.Join(rulesDir, "02-no-silent-deferrals.md"), 0644},
		{"baton/rules/03-capture-discipline.md", filepath.Join(rulesDir, "03-capture-discipline.md"), 0644},
		{"baton/rules/04-commit-messages-as-capture.md", filepath.Join(rulesDir, "04-commit-messages-as-capture.md"), 0644},
		{"baton/rules/05-session-discipline.md", filepath.Join(rulesDir, "05-session-discipline.md"), 0644},
		{"baton/rules/06-proof-bundle.md", filepath.Join(rulesDir, "06-proof-bundle.md"), 0644},
		{"baton/rules/07-adversarial-verification.md", filepath.Join(rulesDir, "07-adversarial-verification.md"), 0644},
		{"baton/rules/08-requirements-fidelity.md", filepath.Join(rulesDir, "08-requirements-fidelity.md"), 0644}}

	for _, f := range files {
		data, err := batonFS.ReadFile(f.src)
		if err != nil {
			return fmt.Errorf("adopt: read embedded %s: %w", f.src, err)
		}
		if err := os.WriteFile(f.dst, data, f.perm); err != nil {
			return fmt.Errorf("adopt: write %s: %w", f.dst, err)
		}
	}
	return nil
}

// SpliceAgents ensures the Baton seven-rule fragment is present in the target
// repo's AGENTS.md. Behaviour:
//
//   - If AGENTS.md does not exist, it is created with the fragment.
//   - If the heading "## Engineering Process — Baton" is absent, the fragment
//     is appended.
//   - If the heading is present, the existing section body (from the heading to
//     the next same-level or higher heading, or EOF) is replaced with the
//     embedded fragment. Content before and after the section is preserved.
//   - If the existing section body is byte-identical to the embedded fragment,
//     the file is not modified (is a true no-op).
//
// On success, returns true if the file was created or modified, false if it
// was a no-op.
func SpliceAgents(repoRoot string) (modified bool, err error) {
	path := filepath.Join(repoRoot, "AGENTS.md")

	existing, readErr := os.ReadFile(path)
	if os.IsNotExist(readErr) {
		// Create AGENTS.md with the fragment.
		return true, os.WriteFile(path, []byte(batonAGENTSFragment+"\n"), 0644)
	}
	if readErr != nil {
		return false, fmt.Errorf("adopt: read %s: %w", path, readErr)
	}

	content := string(existing)
	headingIdx := strings.Index(content, BatonSectionHeading)

	if headingIdx < 0 {
		// Heading not present — append the fragment.
		// Ensure a blank line separator before appending.
		sep := "\n"
		if !strings.HasSuffix(content, "\n\n") {
			if strings.HasSuffix(content, "\n") {
				sep = "\n"
			} else {
				sep = "\n\n"
			}
		}
		newContent := content + sep + batonAGENTSFragment + "\n"
		if newContent == content {
			return false, nil
		}
		return true, os.WriteFile(path, []byte(newContent), 0644)
	}

	// Heading is present — find the end of the section to replace.
	// A section ends at the next heading of same or higher level (##) or EOF.
	sectionStart := headingIdx
	bodyStart := headingIdx + len(BatonSectionHeading)
	// Skip the rest of the heading line (description text after the heading).
	if nl := strings.IndexByte(content[bodyStart:], '\n'); nl >= 0 {
		bodyStart += nl + 1
	}

	// Find the next ## heading at the same or higher level.
	remaining := content[bodyStart:]
	nextHeading := strings.Index(remaining, "\n## ")
	var sectionEnd int
	if nextHeading >= 0 {
		sectionEnd = bodyStart + nextHeading + 1 // include the \n before the heading
	} else {
		sectionEnd = len(content)
	}

	oldSection := content[sectionStart:sectionEnd]
	// Reconstruct the section from the embedded fragment. The fragment begins
	// with the heading line + "\n\n" + body. Reconstruct exactly.
	bodyContent := strings.TrimPrefix(batonAGENTSFragment, BatonSectionHeading+"\n\n")
	newSection := BatonSectionHeading + "\n\n" + bodyContent

	// Normalise trailing newlines for comparison — the file may have gained
	// an extra trailing newline from a previous write.
	if strings.TrimRight(oldSection, "\n") == strings.TrimRight(newSection, "\n") {
		return false, nil // byte-identical (modulo trailing newline), no-op
	}
	// Replace the section body.
	newContent := content[:sectionStart] + newSection + content[sectionEnd:]
	return true, os.WriteFile(path, []byte(newContent), 0644)
}

// BatonDocsFS returns the embedded docs/baton/ filesystem for use by callers
// that need direct access to the vendored rule files.
func BatonDocsFS() embed.FS {
	return batonFS
}
