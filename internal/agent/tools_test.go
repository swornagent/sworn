package agent

import (
	"strings"
	"testing"
)

func TestWebSearchToolSchema_InAllToolDefs(t *testing.T) {
	tools := allToolDefs()
	found := false
	for _, tool := range tools {
		if tool.Name == "web_search" {
			found = true
			if tool.Description == "" {
				t.Error("web_search tool has empty description")
			}
			break
		}
	}
	if !found {
		t.Error("web_search tool not found in allToolDefs()")
	}
}

func TestWebSearch_Stubbed(t *testing.T) {
	e := &executor{
		root:      t.TempDir(),
		maxOutput: 10000,
	}

	// Empty query returns error before HTTP call.
	result := e.run("web_search", `{"query":""}`)
	if !strings.Contains(result, "error") && !strings.Contains(result, "empty") {
		t.Errorf("expected empty query error, got: %s", result)
	}
}