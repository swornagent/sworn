package mcp

import (
	"context"
	"fmt"
	"github.com/swornagent/sworn/internal/prompt"
)

// RegisterPrompts registers the planner, implementer, and verifier prompts.
func RegisterPrompts(s *Server) {
	s.RegisterPrompt("planner", func(ctx context.Context, name string, args map[string]string) (string, error) {
		p := prompt.Planner()
		if p == "" {
			return "", fmt.Errorf("sworn://prompts/plan: embedded prompt not found — this is a binary build error; please reinstall sworn.")
		}
		return p, nil
	})
	s.RegisterPrompt("implementer", func(ctx context.Context, name string, args map[string]string) (string, error) {
		p := prompt.Implementer()
		if p == "" {
			return "", fmt.Errorf("sworn://prompts/implement: embedded prompt not found — this is a binary build error; please reinstall sworn.")
		}
		return p, nil
	})
	s.RegisterPrompt("verifier", func(ctx context.Context, name string, args map[string]string) (string, error) {
		p := prompt.Verifier()
		if p == "" {
			return "", fmt.Errorf("sworn://prompts/verify: embedded prompt not found — this is a binary build error; please reinstall sworn.")
		}
		return p, nil
	})
}