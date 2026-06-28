// Package journey defines the critical-customer-journey model and the durable
// journeys artefact. A journey is an ordered, end-to-end path a user type
// takes across the app to achieve an outcome, crossing many slices.
//
// The artefact lives at <project-root>/.sworn/journeys.json and is a
// first-class Baton platform artefact — version-controlled, human-ratified,
// maintained release over release.
package journey

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/swornagent/sworn/internal/baton"
)
// JourneyArtefactPath returns the path to the journeys artefact relative to a
// project root.
const artefactRelPath = ".sworn/journeys.json"

// JourneyArtefactPath returns the absolute path to the journeys artefact for
// the given project root.
func JourneyArtefactPath(projectRoot string) string {
	return filepath.Join(projectRoot, artefactRelPath)
}

// Journey is one critical customer journey: an ordered, end-to-end path a
// user type takes across the app to achieve an outcome.
//
// Fields are provisional — refined via the live journey-validation hand-run
// and /replan-release. The artefact format itself is stable; new fields are
// additive and do not break existing artefacts.
type Journey struct {
	// ID is a short unique identifier (e.g. "J01-onboard-new-user").
	ID string `json:"id"`

	// UserType is the user persona that follows this journey
	// (e.g. "free_user", "pro_user", "admin").
	UserType string `json:"user_type"`

	// Outcome is what the user achieves at the end of this journey.
	Outcome string `json:"outcome"`

	// Steps is the ordered sequence of user actions comprising the journey.
	Steps []JourneyStep `json:"steps,omitempty"`

	// EntrySurface is the UI or API surface where the journey begins.
	EntrySurface string `json:"entry_surface,omitempty"`

	// HasRegression is true when this journey has been codified into an
	// automated regression test scaffold via sworn journeys --regen.
	HasRegression bool `json:"has_regression,omitempty"`

	// RegressionTestPath is the relative path from the project root to the
	// generated regression test file (e.g. "internal/journey/journey_J01_test.go").
	// Empty when no regression scaffold has been generated.
	RegressionTestPath string `json:"regression_test_path,omitempty"`

	// NoMockBoundary declares the boundary that must cross real infrastructure
	// (not a mock) when this journey is walked. e.g. "entitlement/credits",
	// "loop-verifier", "real-board/real-gates". Rule 10 enforcement: a mock at
	// this boundary during journey validation fails the gate.
	NoMockBoundary string `json:"no_mock_boundary,omitempty"`
}
// JourneyStep is one step within a critical customer journey.
type JourneyStep struct {
	// Order is the step's position in the journey (1-indexed).
	Order int `json:"order"`

	// Description of what the user does at this step.
	Description string `json:"description"`

	// Surface is the UI/API component or route the user interacts with.
	Surface string `json:"surface,omitempty"`
}

// Ratification records the human-ratification metadata for a durable artefact.
// It is nested under the "ratification" key in the JSON output.
type Ratification struct {
	// By records who ratified the artefact (email or identifier).
	By string `json:"by"`
	// At is when the artefact was last ratified (ISO 8601).
	At string `json:"at"`
	// IsRatified is true when the artefact has been human-ratified.
	IsRatified bool `json:"is_ratified"`
}

// JourneyArtefact is the durable, version-controlled journeys artefact.
// It is persisted to .sworn/journeys.json and carries ratification metadata.
type JourneyArtefact struct {
	// Schema identifies the canonical JSON Schema for this artefact.
	Schema string `json:"$schema"`

	// Schema version for forward compatibility.
	Version int `json:"version"`

	// CreatedAt is when the artefact was first created (ISO 8601).
	CreatedAt string `json:"created_at"`

	// UpdatedAt is when the artefact was last modified (ISO 8601).
	UpdatedAt string `json:"updated_at"`

	// Ratification carries human-ratification metadata as a nested object.
	Ratification Ratification `json:"ratification"`

	// Journeys is the list of critical customer journeys.
	Journeys []Journey `json:"journeys"`
}
// NewArtefact creates a new JourneyArtefact with the given journeys and no
// ratification. The caller should add journeys and then call Ratify.
func NewArtefact() *JourneyArtefact {
	now := time.Now().UTC().Format(time.RFC3339)
	return &JourneyArtefact{
		Schema:    baton.JourneysSchemaURI,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
		Journeys:  []Journey{},
		Ratification: Ratification{
			By:         "",
			At:         "",
			IsRatified: false,
		},
	}
}
// Ratify marks the artefact as human-ratified. It records who ratified and
// when. It returns an error if the artefact has no journeys (an artefact
// must contain at least one journey to be meaningful).
func (a *JourneyArtefact) Ratify(ratifiedBy string) error {
	if len(a.Journeys) == 0 {
		return fmt.Errorf("cannot ratify a journeys artefact with no journeys")
	}
	if ratifiedBy == "" {
		return fmt.Errorf("ratified_by is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	a.Ratification.By = ratifiedBy
	a.Ratification.At = now
	a.Ratification.IsRatified = true
	a.UpdatedAt = now
	return nil
}
// AddJourney appends a journey to the artefact. It removes ratification
// status — any edit invalidates prior ratification.
func (a *JourneyArtefact) AddJourney(j Journey) {
	a.Journeys = append(a.Journeys, j)
	a.Ratification.IsRatified = false
	a.Ratification.At = ""
	a.Ratification.By = ""
	a.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
}
// LoadArtefact reads and parses the journeys artefact from the given project
// root. It returns a sentinel error (IsNotExist) when the artefact file does
// not exist, and a different error for parse failures.
func LoadArtefact(projectRoot string) (*JourneyArtefact, error) {
	path := JourneyArtefactPath(projectRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s: %w", path, ErrArtefactNotExist)
		}
		return nil, fmt.Errorf("journey: read %s: %w", path, err)
	}

	var a JourneyArtefact
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("journey: parse %s: %w", path, err)
	}
	return &a, nil
}

// SaveArtefact serialises the artefact to .sworn/journeys.json under the given
// project root. It creates the .sworn directory if needed. Before writing, it
// validates the serialised JSON against the embedded journeys-v1 schema.
func SaveArtefact(projectRoot string, a *JourneyArtefact) error {
	path := JourneyArtefactPath(projectRoot)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("journey: mkdir %s: %w", dir, err)
	}

	a.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	// Ensure $schema is set.
	if a.Schema == "" {
		a.Schema = baton.JourneysSchemaURI
	}

	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("journey: marshal: %w", err)
	}

	// Validate against the embedded journeys-v1 schema before writing.
	if err := baton.Validate("journeys-v1", data); err != nil {
		return fmt.Errorf("journey: validation failed — not written: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("journey: write %s: %w", path, err)
	}
	return nil
}
// ErrArtefactNotExist is returned by LoadArtefact when the artefact file does
// not exist. Callers can use errors.Is to detect this case.
var ErrArtefactNotExist = fmt.Errorf("journeys artefact does not exist")

// CheckResult is the result of a journeys check.
type CheckResult int

const (
	// CheckPass means the artefact exists, is ratified, and parses correctly.
	CheckPass CheckResult = iota

	// CheckMissing means no artefact exists — elicitation has not been run.
	CheckMissing

	// CheckUnratified means the artefact exists but is not yet human-ratified.
	CheckUnratified
)

// String returns a human-readable description of the check result.
func (r CheckResult) String() string {
	switch r {
	case CheckPass:
		return "pass"
	case CheckMissing:
		return "missing — elicitation not run"
	case CheckUnratified:
		return "unratified — journeys exist but need human ratification"
	default:
		return "unknown"
	}
}

// Check examines the journeys artefact at the given project root and returns
// the result. When the result is CheckPass, the artefact is returned as a
// convenience so the caller can list journeys without a second load.
func Check(projectRoot string) (CheckResult, *JourneyArtefact, error) {
	a, err := LoadArtefact(projectRoot)
	if err != nil {
		if isArtefactNotExist(err) {
			return CheckMissing, nil, nil
		}
		return CheckMissing, nil, err
	}

	if !a.Ratification.IsRatified {
		return CheckUnratified, a, nil
	}
	return CheckPass, a, nil
}

// isArtefactNotExist reports whether err is an ErrArtefactNotExist sentinel.
func isArtefactNotExist(err error) bool {
	if err == nil {
		return false
	}
	// The wrapped error path: wrapped with %w in LoadArtefact.
	return containsErr(err, ErrArtefactNotExist)
}

func containsErr(err, target error) bool {
	for e := err; e != nil; {
		if e == target {
			return true
		}
		if u, ok := e.(interface{ Unwrap() error }); ok {
			e = u.Unwrap()
		} else {
			return false
		}
	}
	return false
}

// ListJourneys returns a sorted list of journey descriptions. Each entry
// is formatted as "<id>: <user_type> — <outcome>".
func ListJourneys(a *JourneyArtefact) []string {
	if a == nil || len(a.Journeys) == 0 {
		return nil
	}

	// Sort by ID for stable output.
	sorted := make([]Journey, len(a.Journeys))
	copy(sorted, a.Journeys)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})

	out := make([]string, 0, len(sorted))
	for _, j := range sorted {
		out = append(out, fmt.Sprintf("%s: %s — %s", j.ID, j.UserType, j.Outcome))
	}
	return out
}

// DraftTemplate creates an initial JourneyArtefact with candidate journeys
// inferred from a scan of the project's source tree. This is the starting
// point for the human to edit and ratify.
//
// In a future iteration this will be model-driven (
// "the model drafts candidate journeys from the app"). For now it produces
// a well-structured template with guidance.
func DraftTemplate(projectRoot string) (*JourneyArtefact, error) {
	a := NewArtefact()

	// Scan the project to discover high-level structure.
	entries := scanProjectStructure(projectRoot)

	// If we found subcommands (cmd/), create generic journeys from them.
	if cmds, ok := entries["cmd"]; ok && len(cmds) > 0 {
		for _, cmd := range cmds {
			if cmd == "sworn" {
				continue // the tool itself
			}
			a.AddJourney(Journey{
				ID:       fmt.Sprintf("J-%s", cmd),
				UserType: "end_user",
				Outcome:  fmt.Sprintf("Run the %s subcommand", cmd),
				Steps: []JourneyStep{
					{Order: 1, Description: fmt.Sprintf("Invoke sworn %s", cmd), Surface: "CLI"},
					{Order: 2, Description: "Observe output", Surface: "CLI"},
				},
				EntrySurface: "CLI",
			})
		}
	}

	// If we found internal packages, add a generic development journey.
	if internals, ok := entries["internal"]; ok && len(internals) > 0 {
		a.AddJourney(Journey{
			ID:       "J-develop-feature",
			UserType: "developer",
			Outcome:  "Implement a new feature through the Baton slice workflow",
			Steps: []JourneyStep{
				{Order: 1, Description: "Open a planner session to decompose into slices", Surface: "CLI"},
				{Order: 2, Description: "Implement each slice via the implementer role", Surface: "CLI"},
				{Order: 3, Description: "Verify each slice via a fresh-context verifier", Surface: "CLI"},
				{Order: 4, Description: "Merge the track when all slices are verified", Surface: "CLI"},
			},
			EntrySurface: "CLI / sworn",
		})
	}

	// Add a generic onboarding journey every app should have.
	a.AddJourney(Journey{
		ID:       "J-initial-setup",
		UserType: "new_user",
		Outcome:  "Set up and configure the tool for the first time",
		Steps: []JourneyStep{
			{Order: 1, Description: "Install the tool", Surface: "CLI"},
			{Order: 2, Description: "Run init to bootstrap configuration", Surface: "sworn init"},
			{Order: 3, Description: "Verify the setup with a smoke test", Surface: "CLI"},
		},
		EntrySurface: "CLI",
	})

	return a, nil
}

// scanProjectStructure walks the top two levels of the project and categorises
// directories by their purpose.
func scanProjectStructure(projectRoot string) map[string][]string {
	result := map[string][]string{}

	entries, err := os.ReadDir(projectRoot)
	if err != nil {
		return result
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if e.Name() == ".git" || e.Name() == "node_modules" || e.Name() == ".sworn" || e.Name() == "docs" {
			continue
		}
		children, _ := os.ReadDir(filepath.Join(projectRoot, e.Name()))
		names := make([]string, 0, len(children))
		for _, c := range children {
			if c.IsDir() && !stringsHasPrefix(c.Name(), ".") {
				names = append(names, c.Name())
			}
		}
		if len(names) > 0 {
			result[e.Name()] = names
		}
	}

	return result
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
