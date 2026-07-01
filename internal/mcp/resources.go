package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/prompt"
)

// RegisterResources registers all static and dynamic resources.
func RegisterResources(s *Server, repoRoot string) {
	// Static resources
	s.RegisterResource("sworn://prompts/plan", func(ctx context.Context, uri string) (string, error) {
		p := prompt.Planner()
		if p == "" {
			return "", fmt.Errorf("sworn://prompts/plan: embedded prompt not found — this is a binary build error; please reinstall sworn.")
		}
		return p, nil
	})
	s.RegisterResource("sworn://prompts/implement", func(ctx context.Context, uri string) (string, error) {
		p := prompt.Implementer()
		if p == "" {
			return "", fmt.Errorf("sworn://prompts/implement: embedded prompt not found — this is a binary build error; please reinstall sworn.")
		}
		return p, nil
	})
	s.RegisterResource("sworn://prompts/verify", func(ctx context.Context, uri string) (string, error) {
		p := prompt.Verifier()
		if p == "" {
			return "", fmt.Errorf("sworn://prompts/verify: embedded prompt not found — this is a binary build error; please reinstall sworn.")
		}
		return p, nil
	})
	s.RegisterResource("sworn://baton/rules", func(ctx context.Context, uri string) (string, error) {
		p, err := prompt.Baton("rules.md")
		if err != nil || p == "" {
			return "", fmt.Errorf("sworn://baton/rules: embedded Baton rules not found — this is a binary build error; please reinstall sworn.")
		}
		return p, nil
	})
	s.RegisterResource("sworn://baton/track-mode", func(ctx context.Context, uri string) (string, error) {
		p := prompt.TrackMode()
		if p == "" {
			return "", fmt.Errorf("sworn://baton/track-mode: embedded prompt not found — this is a binary build error; please reinstall sworn.")
		}
		return p, nil
	})
	s.RegisterResource("sworn://baton/version", func(ctx context.Context, uri string) (string, error) {
		v := prompt.BatonVersion()
		if v == "" {
			return "", fmt.Errorf("sworn://baton/version: embedded prompt not found — this is a binary build error; please reinstall sworn.")
		}
		return v, nil
	})

	// Dynamic release resources (prefix match)
	s.RegisterResource("sworn://release/", func(ctx context.Context, uri string) (string, error) {
		path := strings.TrimPrefix(uri, "sworn://release/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			return "", fmt.Errorf("invalid release resource URI: %s", uri)
		}

		releaseName := parts[0]
		if len(parts) == 2 {
			kind := parts[1]
			switch kind {
			case "board":
				filePath := filepath.Join(repoRoot, "docs", "release", releaseName, "index.md")
				content, err := os.ReadFile(filePath)
				if err != nil {
					return "", fmt.Errorf("failed to read release board: %w", err)
				}
				return string(content), nil
			case "intake":
				filePath := filepath.Join(repoRoot, "docs", "release", releaseName, "intake.md")
				content, err := os.ReadFile(filePath)
				if err != nil {
					return "", fmt.Errorf("failed to read release intake: %w", err)
				}
				return string(content), nil
			default:
				return "", fmt.Errorf("unknown release resource kind %q in URI: %s", kind, uri)
			}
		}

		if len(parts) == 3 {
			sliceID := parts[1]
			kind := parts[2]
			switch kind {
			case "spec":
				filePath := filepath.Join(repoRoot, "docs", "release", releaseName, sliceID, "spec.md")
				content, err := os.ReadFile(filePath)
				if err != nil {
					return "", fmt.Errorf("failed to read slice spec: %w", err)
				}
				return string(content), nil
			case "proof":
				filePath := filepath.Join(repoRoot, "docs", "release", releaseName, sliceID, "proof.md")
				content, err := os.ReadFile(filePath)
				if err != nil {
					if os.IsNotExist(err) {
						return "", nil // returns empty string if proof.md does not yet exist — not an error
					}
					return "", fmt.Errorf("failed to read slice proof: %w", err)
				}
				return string(content), nil
			default:
				return "", fmt.Errorf("unknown slice resource kind %q in URI: %s", kind, uri)
			}
		}

		return "", fmt.Errorf("invalid release resource URI: %s", uri)
	})
}
