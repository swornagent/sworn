package baton

import (
	"fmt"
	"os"
	"path/filepath"
)

// FileMapping pairs a Baton source path (relative to the source directory root)
// with the SwornAgent embed destination path (relative to the repository root).
type FileMapping struct {
	Source string // relative to Baton source dir (e.g. "baton/reachability-gate.md")
	Dest   string // relative to repo root (e.g. "internal/adopt/baton/rules/01-reachability-gate.md")
}

// batonFileMappings is the explicit, hand-maintained map of every Baton source
// file → SwornAgent embed destination. This is safer than a recursive glob —
// a new file type upstream won't silently land in the embed without an explicit
// mapping decision (Design Decision §2.2).
var batonFileMappings = []FileMapping{
	// Rules (numbered in SwornAgent; flat in Baton)
	{Source: "baton/reachability-gate.md", Dest: "internal/adopt/baton/rules/01-reachability-gate.md"},
	{Source: "baton/no-silent-deferrals.md", Dest: "internal/adopt/baton/rules/02-no-silent-deferrals.md"},
	{Source: "baton/capture-discipline.md", Dest: "internal/adopt/baton/rules/03-capture-discipline.md"},
	{Source: "baton/commit-messages-as-capture.md", Dest: "internal/adopt/baton/rules/04-commit-messages-as-capture.md"},
	{Source: "baton/session-discipline.md", Dest: "internal/adopt/baton/rules/05-session-discipline.md"},
	{Source: "baton/proof-bundle.md", Dest: "internal/adopt/baton/rules/06-proof-bundle.md"},
	{Source: "baton/adversarial-verification.md", Dest: "internal/adopt/baton/rules/07-adversarial-verification.md"},
	{Source: "baton/requirements-fidelity.md", Dest: "internal/adopt/baton/rules/08-requirements-fidelity.md"},
	{Source: "baton/design-fidelity.md", Dest: "internal/adopt/baton/rules/09-design-fidelity.md"},
	{Source: "baton/customer-journey-validation.md", Dest: "internal/adopt/baton/rules/10-customer-journey-validation.md"},
	{Source: "baton/process-global-mutation.md", Dest: "internal/adopt/baton/rules/11-process-global-mutation.md"},
	{Source: "baton/guard-fidelity.md", Dest: "internal/adopt/baton/rules/12-guard-fidelity.md"},

	// Capability policy companion doc (v0.11.0 — schema explainer, not a numbered rule)
	{Source: "baton/capability-policy.md", Dest: "internal/adopt/baton/capability-policy.md"},

	// Adopt README
	{Source: "baton/README.md", Dest: "internal/adopt/baton/README.md"},

	// Role prompts
	{Source: "baton/role-prompts/implementer.md", Dest: "internal/prompt/implementer.md"},
	{Source: "baton/role-prompts/planner.md", Dest: "internal/prompt/planner.md"},
	{Source: "baton/role-prompts/verifier.md", Dest: "internal/prompt/verifier.md"},
	{Source: "baton/role-prompts/captain.md", Dest: "internal/prompt/captain.md"},

	// Architecture rules (v0.5.0)
	{Source: "baton/architecture.json", Dest: "internal/adopt/baton/architecture.json"},

	// Baton protocol documents (embedded under internal/prompt/baton/)
	{Source: "baton/track-mode.md", Dest: "internal/prompt/baton/track-mode.md"},
	{Source: "baton/session-discipline.md", Dest: "internal/prompt/baton/session-discipline.md"},
	{Source: "baton/brainstorm-patterns.md", Dest: "internal/prompt/baton/brainstorm-patterns.md"},
	{Source: "baton/README.md", Dest: "internal/prompt/baton/README.md"},

	// Combined rules (concatenated by Vendor, not a single source file).
	// This entry is a sentinel: the Vendor reads each individual rule source,
	// transforms it, concatenates them, and writes the result.
	{Source: "baton/rules.md", Dest: "internal/prompt/baton/rules.md"},
}

// ValidateSource checks that every source file in the mapping exists under
// sourceDir. Returns nil if all are present, or an error naming the first
// missing file.
func ValidateSource(sourceDir string) error {
	for _, m := range batonFileMappings {
		p := filepath.Join(sourceDir, m.Source)
		// rules.md is a concatenation target, not a source file — skip it.
		if m.Source == "baton/rules.md" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("baton: source file missing: %s (expected at %s)", m.Source, p)
			}
			return fmt.Errorf("baton: cannot stat source file %s: %w", m.Source, err)
		}
	}
	return nil
}

// AllMappings returns the full file mapping. The caller is expected to skip
// the "rules.md" sentinel entry when reading individual source files (it is a
// concatenation target).
func AllMappings() []FileMapping {
	return batonFileMappings
}

// RuleSources returns the source paths for the ten individual rules,
// in order, so they can be concatenated into rules.md.
func RuleSources() []string {
	return []string{
		"baton/reachability-gate.md",
		"baton/no-silent-deferrals.md",
		"baton/capture-discipline.md",
		"baton/commit-messages-as-capture.md",
		"baton/session-discipline.md",
		"baton/proof-bundle.md",
		"baton/adversarial-verification.md",
		"baton/requirements-fidelity.md",
		"baton/design-fidelity.md",
		"baton/customer-journey-validation.md",
		"baton/process-global-mutation.md",
		"baton/guard-fidelity.md",
	}
}
