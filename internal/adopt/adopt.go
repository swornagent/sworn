// Package adopt materialises the Baton protocol into a target repo:
// writing docs/baton/ (rules + VERSION) and splicing the seven-rule
// fragment into agent config files. Both operations are idempotent.
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
// section in agent config files. It must match the heading used by the
// splice logic exactly.
const BatonSectionHeading = "## Engineering Process — Baton"

// agentFiles lists the recognized agent-config files that sworn splices
// the Baton rules section into. AGENTS.md is always created if absent;
// the others are only spliced if they already exist in the repo.
var agentFiles = []string{
	"AGENTS.md",
	"CLAUDE.md",
}

// batonAGENTSFragment is the seven-rule fragment spliced into agent config
// files. It is hardcoded as a Go constant — the rules are stable text from
// the open Baton protocol and change only at explicit protocol version bumps.
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
can shortcut straight to verified.

Full rule docs: ` + "`docs/baton/`" + `.`

// SpliceAction describes what SpliceAgents did (or would do) for one file.
type SpliceAction int

const (
	SpliceCreated    SpliceAction = iota // file created with fragment
	SpliceAppended                       // section appended to existing file
	SpliceUpdated                        // existing section replaced (force=true)
	SpliceNoOp                           // section already current; no-op
	SpliceCustomized                     // section exists and differs; skipped without --force
	SpliceAbsent                         // optional file absent; skipped
)

// SpliceResult describes the outcome (or planned outcome) for a single agent
// config file.
type SpliceResult struct {
	File   string
	Action SpliceAction
}

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
		{"baton/rules/08-requirements-fidelity.md", filepath.Join(rulesDir, "08-requirements-fidelity.md"), 0644},
		{"baton/rules/09-design-fidelity.md", filepath.Join(rulesDir, "09-design-fidelity.md"), 0644},
		{"baton/rules/10-customer-journey-validation.md", filepath.Join(rulesDir, "10-customer-journey-validation.md"), 0644},
	}
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

// BatonDocsExist reports whether the docs/baton/ directory already exists
// under repoRoot. Used by callers to determine whether Materialise would
// create new files or is a no-op update.
func BatonDocsExist(repoRoot string) bool {
	_, err := os.Stat(filepath.Join(repoRoot, "docs", "baton"))
	return err == nil
}

// PlanSplice scans all candidate agent config files and returns what
// SpliceAgents would do for each, without writing anything. Use this to
// present a plan to the user before applying changes.
func PlanSplice(repoRoot string, force bool) ([]SpliceResult, error) {
	return runSplice(repoRoot, force, true)
}

// SpliceAgents ensures the Baton seven-rule fragment is present in all
// recognized agent config files found in repoRoot. AGENTS.md is always
// created if absent; other files (CLAUDE.md, etc.) are only spliced if
// they already exist.
//
// If force is false and an existing Baton section has been customized
// (differs from the embedded fragment), the file is left unchanged and
// the result carries SpliceCustomized. Pass force=true to overwrite.
//
// Returns one SpliceResult per candidate file.
func SpliceAgents(repoRoot string, force bool) ([]SpliceResult, error) {
	return runSplice(repoRoot, force, false)
}

func runSplice(repoRoot string, force, dryRun bool) ([]SpliceResult, error) {
	var results []SpliceResult
	for i, name := range agentFiles {
		mustCreate := i == 0 // only AGENTS.md is created if absent
		path := filepath.Join(repoRoot, name)
		action, err := spliceOne(path, force, mustCreate, dryRun)
		if err != nil {
			return results, err
		}
		results = append(results, SpliceResult{File: name, Action: action})
	}
	return results, nil
}

func spliceOne(path string, force, mustCreate, dryRun bool) (SpliceAction, error) {
	existing, readErr := os.ReadFile(path)
	if os.IsNotExist(readErr) {
		if !mustCreate {
			return SpliceAbsent, nil
		}
		if dryRun {
			return SpliceCreated, nil
		}
		return SpliceCreated, os.WriteFile(path, []byte(batonAGENTSFragment+"\n"), 0644)
	}
	if readErr != nil {
		return SpliceNoOp, fmt.Errorf("adopt: read %s: %w", path, readErr)
	}

	content := string(existing)
	headingIdx := strings.Index(content, BatonSectionHeading)

	if headingIdx < 0 {
		if dryRun {
			return SpliceAppended, nil
		}
		sep := "\n"
		if !strings.HasSuffix(content, "\n\n") {
			if strings.HasSuffix(content, "\n") {
				sep = "\n"
			} else {
				sep = "\n\n"
			}
		}
		newContent := content + sep + batonAGENTSFragment + "\n"
		return SpliceAppended, os.WriteFile(path, []byte(newContent), 0644)
	}

	// Heading present — locate section bounds.
	sectionStart := headingIdx
	bodyStart := headingIdx + len(BatonSectionHeading)
	if nl := strings.IndexByte(content[bodyStart:], '\n'); nl >= 0 {
		bodyStart += nl + 1
	}
	remaining := content[bodyStart:]
	nextHeading := strings.Index(remaining, "\n## ")
	var sectionEnd int
	if nextHeading >= 0 {
		sectionEnd = bodyStart + nextHeading + 1
	} else {
		sectionEnd = len(content)
	}

	oldSection := content[sectionStart:sectionEnd]
	bodyContent := strings.TrimPrefix(batonAGENTSFragment, BatonSectionHeading+"\n\n")
	newSection := BatonSectionHeading + "\n\n" + bodyContent

	if strings.TrimRight(oldSection, "\n") == strings.TrimRight(newSection, "\n") {
		return SpliceNoOp, nil
	}

	// Section differs from embedded fragment — it has been customized.
	if !force {
		return SpliceCustomized, nil
	}

	if dryRun {
		return SpliceUpdated, nil
	}
	newContent := content[:sectionStart] + newSection + content[sectionEnd:]
	return SpliceUpdated, os.WriteFile(path, []byte(newContent), 0644)
}

// BatonDocsFS returns the embedded docs/baton/ filesystem for use by callers
// that need direct access to the vendored rule files.
func BatonDocsFS() embed.FS {
	return batonFS
}
