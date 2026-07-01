package templates

import (
	"strings"
	"testing"
)

func TestEmbeddedTemplatesNonEmpty(t *testing.T) {
	for name, got := range map[string]string{
		"agents.md":         AgentsMD(),
		"considerations.md": ConsiderationsMD(),
		"decisions.md":      DecisionsMD(),
	} {
		if strings.TrimSpace(got) == "" {
			t.Errorf("embedded template %s is empty", name)
		}
	}
}

// The AGENTS.md template is the MCP pointer sworn init installs into adopting
// repos — it must advertise the full-protocol resource URI.
func TestAgentsMDAdvertisesBatonRules(t *testing.T) {
	if !strings.Contains(AgentsMD(), "sworn://baton/rules") {
		t.Error("agents.md template does not reference sworn://baton/rules")
	}
}
