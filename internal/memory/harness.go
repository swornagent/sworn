package memory

import (
	"os"
	"path/filepath"
)

// HarnessInfo describes a known AI coding harness and its memory path status.
type HarnessInfo struct {
	ID          HarnessID `json:"id"`
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Exists      bool      `json:"exists"`
	Description string    `json:"description,omitempty"`
}

// HarnessMemoryPath returns the canonical memory file or directory for
// the given harness ID, relative to cwd (the project root).
//
// Returns empty string for harnesses with no native memory path (e.g. Codex).
func HarnessMemoryPath(id HarnessID, cwd string) string {
	switch id {
	case HarnessClaudeCode:
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		encoded := EncodeProjectPath(cwd)
		return filepath.Join(home, ".claude", "projects", encoded, "memory")
	case HarnessGeminiCLI:
		// Gemini CLI has a global file and a per-project file.
		// Return the per-project variant; the global one is discoverable by name.
		return filepath.Join(cwd, "GEMINI.md")
	case HarnessOpenCode:
		return filepath.Join(cwd, "AGENTS.md")
	case HarnessCursor:
		return filepath.Join(cwd, ".cursorrules")
	case HarnessWindsurf:
		return filepath.Join(cwd, ".windsurfrules")
	case HarnessCodex:
		// Codex has no native memory path.
		return ""
	case HarnessCustom:
		// Custom harness paths come from config extra_paths, not a fixed path.
		return ""
	default:
		return ""
	}
}

// pathExists returns true if path exists on disk (file or directory).
func pathExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// ListHarnesses returns the set of harnesses configured in cfg, each annotated
// with its canonical memory path and whether that path exists on disk.
//
// The canonical path is derived from cwd (the project/working directory).
// Extra paths from cfg.ExtraPaths are appended as "custom" entries.
func ListHarnesses(cfg *MemoryConfig, cwd string) []HarnessInfo {
	var result []HarnessInfo

	for _, idStr := range cfg.Harnesses {
		id := HarnessID(idStr)
		path := HarnessMemoryPath(id, cwd)
		result = append(result, HarnessInfo{
			ID:     id,
			Name:   harnessDisplayName(id),
			Path:   path,
			Exists: pathExists(path),
		})
	}

	// Append custom extra paths as additional harness entries.
	for _, extra := range cfg.ExtraPaths {
		result = append(result, HarnessInfo{
			ID:     HarnessCustom,
			Name:   "custom",
			Path:   extra,
			Exists: pathExists(extra),
		})
	}

	return result
}

// harnessDisplayName returns a human-readable name for a harness ID.
func harnessDisplayName(id HarnessID) string {
	switch id {
	case HarnessClaudeCode:
		return "Claude Code"
	case HarnessGeminiCLI:
		return "Gemini CLI"
	case HarnessOpenCode:
		return "OpenCode"
	case HarnessCursor:
		return "Cursor"
	case HarnessWindsurf:
		return "Windsurf"
	case HarnessCodex:
		return "Codex"
	case HarnessCustom:
		return "Custom"
	default:
		return string(id)
	}
}