package gate

import (
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/project"
)

// TestUserPromptHeaderNamesTheRealProject pins the v0.12.0 defect: the header must
// carry the project's context, and must not carry the old hardcoded string.
func TestUserPromptHeaderNamesTheRealProject(t *testing.T) {
	proj := project.Resolved{
		Context: "a Next.js and TypeScript monorepo",
		Source:  project.SourceDeclared,
	}
	payload := buildUserPayload(proj, "SPEC", "DIFF")

	if !strings.Contains(payload, "a Next.js and TypeScript monorepo") {
		t.Error("user payload does not tell the model what project it is reading")
	}
	if strings.Contains(payload, "SwornAgent project") || strings.Contains(payload, "a Go CLI") {
		t.Error("user payload still carries the hardcoded SwornAgent/Go-CLI header — " +
			"every check in every repo would grade against Go priors")
	}
}

// TestUserPayloadCarriesStakes pins the v0.13.0 contract: security-review grades
// against the stakes block, so it must actually reach the model.
func TestUserPayloadCarriesStakes(t *testing.T) {
	high := project.Resolved{
		Context:    "a Go backend on Postgres",
		Source:     project.SourceDeclared,
		HighStakes: true,
		Stakes: &project.Stakes{
			Production:    true,
			RealUsers:     true,
			SensitiveData: []string{"pii", "financial"},
		},
	}
	payload := buildUserPayload(high, "SPEC", "DIFF")
	for _, want := range []string{"STAKES: HIGH", "live in production", "Real people", "pii, financial"} {
		if !strings.Contains(payload, want) {
			t.Errorf("high-stakes payload missing %q — security-review grades on this", want)
		}
	}

	low := project.Resolved{
		Context:    "a Go CLI",
		Source:     project.SourceDeclared,
		HighStakes: false,
		Stakes:     &project.Stakes{},
	}
	if !strings.Contains(buildUserPayload(low, "SPEC", "DIFF"), "STAKES: LOW") {
		t.Error("low-stakes payload does not say so")
	}
}

// TestUserPayloadNeverClaimsLowStakesOnAGuess is the fail-closed guard. An
// undeclared project must never have the model told the stakes are low.
func TestUserPayloadNeverClaimsLowStakesOnAGuess(t *testing.T) {
	inferred := project.Resolved{
		Context:    "a Go project",
		Source:     project.SourceInferred,
		HighStakes: true,
	}
	payload := buildUserPayload(inferred, "SPEC", "DIFF")
	if strings.Contains(payload, "STAKES: LOW") {
		t.Fatal("an INFERRED project was told the model its stakes are LOW — " +
			"a guess must never lower the security bar")
	}
	if !strings.Contains(payload, "STAKES: HIGH") {
		t.Error("an inferred project must fail closed to HIGH stakes")
	}
	if !strings.Contains(payload, "declared no context record") {
		t.Error("the model must be told the context was inferred, not declared")
	}
}
