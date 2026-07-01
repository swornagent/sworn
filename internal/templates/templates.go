// Package templates embeds the project-scaffolding markdown templates
// (AGENTS.md MCP-pointer, consideration catalog, decision registry) into the
// sworn binary so that sworn init / sworn induction work on cold start in any
// repo — an adopting repo never has these files locally (sworn#28).
package templates

import _ "embed"

//go:embed agents.md
var agentsMD string

//go:embed considerations.md
var considerationsMD string

//go:embed decisions.md
var decisionsMD string

// AgentsMD returns the embedded MCP-pointer AGENTS.md template.
func AgentsMD() string { return agentsMD }

// ConsiderationsMD returns the embedded consideration catalog template.
func ConsiderationsMD() string { return considerationsMD }

// DecisionsMD returns the embedded decision registry template.
func DecisionsMD() string { return decisionsMD }
