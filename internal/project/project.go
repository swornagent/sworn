// Package project reads the adopting project's declared context and stakes —
// the Baton project-context-v1 record at .sworn/project.json.
//
// The record answers two questions the LLM checks cannot answer for themselves:
// WHAT is this project, and WHAT IS AT RISK if a defect ships. The second is
// load-bearing: at high stakes a `medium` security finding blocks instead of
// advising (baton v0.13.0, baton/llm-checks/security-review.md).
//
// Everything here fails closed. A record that is absent, malformed, or unratified
// resolves to HIGH stakes, never low. A model may draft the record and propose its
// stakes — it can read the auth code, the payment integration, the schema holding
// customer records — but only a human can confirm that real people depend on the
// system. So a proposal may RAISE the bar; it may never LOWER it.
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
)

// RecordPath is the record's location, relative to the repo root.
const RecordPath = ".sworn/project.json"

// SchemaName is the Baton schema the record is graded against.
const SchemaName = "project-context-v1"

// Record is the project-context-v1 record.
type Record struct {
	Schema       string       `json:"$schema,omitempty"`
	Context      string       `json:"context"`
	Stakes       *Stakes      `json:"stakes,omitempty"`
	Ratification Ratification `json:"ratification"`
}

// Stakes is what is at risk if a defect ships.
type Stakes struct {
	Production    bool     `json:"production,omitempty"`
	RealUsers     bool     `json:"real_users,omitempty"`
	SensitiveData []string `json:"sensitive_data,omitempty"`
	Regulated     []string `json:"regulated,omitempty"`
	Notes         string   `json:"notes,omitempty"`
}

// Ratification records the human sign-off. Mirrors journeys-v1 (Rule 10): the
// model drafts, a human ratifies, and an unratified artefact does not clear a
// gate on its own.
type Ratification struct {
	Ratified  bool   `json:"ratified"`
	At        string `json:"at,omitempty"`
	By        string `json:"by,omitempty"`
	DraftedBy string `json:"drafted_by,omitempty"`
}

// Source records where a resolved context came from — so a caller (and
// `sworn doctor`) can tell a declaration from a guess.
type Source string

const (
	// SourceDeclared: a ratified project-context-v1 record.
	SourceDeclared Source = "declared"
	// SourceDrafted: a record exists but no human has ratified it. Its context is
	// used, but its stakes are NOT trusted to lower the bar.
	SourceDrafted Source = "drafted"
	// SourceInferred: no record. The context is detected from the repo's files and
	// the stakes are unknown.
	SourceInferred Source = "inferred"
)

// Resolved is the context the engine hands to a check.
type Resolved struct {
	Context    string
	Stakes     *Stakes
	Source     Source
	HighStakes bool
}

// ErrNoRecord is returned by Load when no record exists.
var ErrNoRecord = errors.New("project: no " + RecordPath)

// Load reads and grades the record at repoRoot/.sworn/project.json.
//
// A malformed record is an error, never a silent fallback to "no stakes": the
// stakes half decides whether a security finding blocks, so a record we cannot
// read must not read as a record that declares nothing.
func Load(repoRoot string) (*Record, error) {
	path := filepath.Join(repoRoot, RecordPath)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoRecord
		}
		return nil, fmt.Errorf("project: read %s: %w", path, err)
	}

	if err := baton.ValidateSchema(SchemaName, raw); err != nil {
		return nil, fmt.Errorf("project: %s does not satisfy %s: %w", RecordPath, SchemaName, err)
	}

	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, fmt.Errorf("project: parse %s: %w", path, err)
	}
	return &rec, nil
}

// Save writes the record to repoRoot/.sworn/project.json, grading it first. A
// record that would not validate is never written.
func Save(repoRoot string, rec *Record) error {
	rec.Schema = "https://baton.sawy3r.net/schemas/project-context-v1.json"

	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("project: marshal: %w", err)
	}
	raw = append(raw, '\n')

	if err := baton.ValidateSchema(SchemaName, raw); err != nil {
		return fmt.Errorf("project: refusing to write a record that does not satisfy %s: %w", SchemaName, err)
	}

	dir := filepath.Join(repoRoot, ".sworn")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("project: mkdir %s: %w", dir, err)
	}
	path := filepath.Join(repoRoot, RecordPath)
	if err := os.WriteFile(path, raw, 0644); err != nil {
		return fmt.Errorf("project: write %s: %w", path, err)
	}
	return nil
}

// Resolve returns the context and stakes an LLM check should be given.
//
// It FAILS CLOSED on stakes. High stakes are assumed unless a RATIFIED record
// says otherwise:
//
//   - no record         -> inferred context, HIGH stakes (nothing is known)
//   - unratified record -> its context, HIGH stakes (a proposal cannot lower the bar)
//   - ratified record   -> its context, its declared stakes
//
// An undeclared system is not a safe one; it is an unexamined one.
func Resolve(repoRoot string) Resolved {
	rec, err := Load(repoRoot)
	if err != nil || rec == nil {
		// No record, or one we could not read. Either way we know nothing about
		// the stakes, so we assume the worst. The context still beats nothing:
		// detection can at least name the languages.
		return Resolved{
			Context:    Detect(repoRoot),
			Source:     SourceInferred,
			HighStakes: true,
		}
	}

	if !rec.Ratification.Ratified {
		return Resolved{
			Context:    rec.Context,
			Stakes:     rec.Stakes,
			Source:     SourceDrafted,
			HighStakes: true, // a proposal may raise the bar, never lower it
		}
	}

	return Resolved{
		Context:    rec.Context,
		Stakes:     rec.Stakes,
		Source:     SourceDeclared,
		HighStakes: rec.Stakes.isHigh(),
	}
}

// isHigh reports whether the declared stakes are high: the system is in
// production, OR real people depend on it, OR it holds sensitive data.
//
// A nil Stakes on a ratified record means the human ratified a record that
// declares nothing about risk — which is a statement that they did not consider
// it, not that there is none. Fail closed.
func (s *Stakes) isHigh() bool {
	if s == nil {
		return true
	}
	return s.Production || s.RealUsers || len(s.SensitiveData) > 0
}

// RenderStakes renders the stakes for the {{project_stakes}} substitution in a
// check's user payload (baton v0.13.0). The security-review prompt reads this and
// grades against it, so it must state the conclusion the engine reached — not just
// the raw fields — and must be explicit when the conclusion is a fail-closed
// assumption rather than a declaration.
func (r Resolved) RenderStakes() string {
	var b strings.Builder
	b.WriteString("STAKES: ")
	if r.HighStakes {
		b.WriteString("HIGH.\n")
	} else {
		b.WriteString("LOW.\n")
	}

	switch r.Source {
	case SourceInferred:
		b.WriteString("The project has declared no context record, so nothing is known about " +
			"what is at risk. Treat the stakes as HIGH: an undeclared system is not a safe one, " +
			"it is an unexamined one. The project description above was detected from the repo's " +
			"files and may be incomplete.\n")
		return b.String()
	case SourceDrafted:
		b.WriteString("The project's context record has NOT been ratified by a human — it is a " +
			"model-drafted proposal. Treat the stakes as HIGH regardless of what it claims below: " +
			"an unconfirmed proposal may raise the bar, never lower it.\n")
	}

	s := r.Stakes
	if s == nil {
		b.WriteString("No stakes are declared on the record.\n")
		return b.String()
	}
	if s.Production {
		b.WriteString("- The code is deployed and live in production.\n")
	}
	if s.RealUsers {
		b.WriteString("- Real people outside the team depend on this system.\n")
	}
	if len(s.SensitiveData) > 0 {
		b.WriteString("- It holds or processes sensitive data: " + strings.Join(s.SensitiveData, ", ") + ".\n")
	}
	if len(s.Regulated) > 0 {
		b.WriteString("- It is subject to: " + strings.Join(s.Regulated, ", ") + ".\n")
	}
	if s.Notes != "" {
		b.WriteString("- " + s.Notes + "\n")
	}
	if !s.Production && !s.RealUsers && len(s.SensitiveData) == 0 {
		b.WriteString("- Not in production, no real users, no sensitive data.\n")
	}
	return b.String()
}
