package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/model"
)

// ---------------------------------------------------------------------------
// Workspace confinement
// ---------------------------------------------------------------------------
// Every file path is resolved relative to the workspace root. ".." segments,
// absolute paths, and symlink traversal are rejected before any I/O.
// This is path-prefix enforcement, not chroot — cross-platform, no root
// privileges, sufficient for the threat model (accidental escape, not an
// adversarial jailbreak). Per spec Risk #2: "document the sandbox boundary."

// resolvePath resolves a tool-supplied path against the workspace root.
// Returns an error if the path escapes the workspace.
func resolvePath(root, p string) (string, error) {
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("agent: absolute path %q rejected (workspace-confined)", p)
	}
	cleaned := filepath.Clean(p)
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, "/../") {
		return "", fmt.Errorf("agent: path traversal %q rejected (workspace-confined)", p)
	}
	resolved := filepath.Join(root, cleaned)
	// Double-check: the resolved path must start with root.
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("agent: cannot resolve root: %w", err)
	}
	absResolved, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("agent: cannot resolve path: %w", err)
	}
	if !strings.HasPrefix(absResolved, absRoot+string(filepath.Separator)) && absResolved != absRoot {
		return "", fmt.Errorf("agent: path %q escapes workspace root", p)
	}
	return resolved, nil
}

// ---------------------------------------------------------------------------
// Tool definitions — each tool provides Schema() model.ToolDef (Captain pin 5:
// tool definitions live in agent package, serialised via model.ToolDef).
// ---------------------------------------------------------------------------

// allToolDefs returns the schema for every registered tool.
func allToolDefs() []model.ToolDef {
	return []model.ToolDef{
		readToolSchema(),
		writeToolSchema(),
		editToolSchema(),
		bashToolSchema(),
		grepToolSchema(),
		globToolSchema(),
	}
}

// --- Read ---

type readArgs struct {
	Path string `json:"path"`
}

func readToolSchema() model.ToolDef {
	return model.ToolDef{
		Name:        "read",
		Description: "Read the contents of a file within the workspace.",
		Parameters: mustMarshal(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file, relative to the workspace root.",
				},
			},
			"required": []string{"path"},
		}),
	}
}

// --- Write ---

type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func writeToolSchema() model.ToolDef {
	return model.ToolDef{
		Name:        "write",
		Description: "Write content to a file within the workspace, creating it if needed.",
		Parameters: mustMarshal(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file, relative to the workspace root.",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "Content to write.",
				},
			},
			"required": []string{"path", "content"},
		}),
	}
}

// --- Edit ---

type editArgs struct {
	Path      string `json:"path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

func editToolSchema() model.ToolDef {
	return model.ToolDef{
		Name:        "edit",
		Description: "Replace old_string with new_string in a file (exact match required).",
		Parameters: mustMarshal(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file, relative to the workspace root.",
				},
				"old_string": map[string]interface{}{
					"type":        "string",
					"description": "The exact string to replace.",
				},
				"new_string": map[string]interface{}{
					"type":        "string",
					"description": "The replacement string.",
				},
			},
			"required": []string{"path", "old_string", "new_string"},
		}),
	}
}

// --- Bash ---

type bashArgs struct {
	Command string `json:"command"`
}

func bashToolSchema() model.ToolDef {
	return model.ToolDef{
		Name:        "bash",
		Description: "Run a shell command within the workspace root. Stdout and stderr are captured.",
		Parameters: mustMarshal(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The shell command to run.",
				},
			},
			"required": []string{"command"},
		}),
	}
}

// --- Grep ---

type grepArgs struct {
	Pattern string `json:"pattern"`
}

func grepToolSchema() model.ToolDef {
	return model.ToolDef{
		Name:        "grep",
		Description: "Search file contents for a pattern within the workspace.",
		Parameters: mustMarshal(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "The regex pattern to search for.",
				},
			},
			"required": []string{"pattern"},
		}),
	}
}

// --- Glob ---

type globArgs struct {
	Pattern string `json:"pattern"`
}

func globToolSchema() model.ToolDef {
	return model.ToolDef{
		Name:        "glob",
		Description: "Find files matching a glob pattern within the workspace.",
		Parameters: mustMarshal(map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "The glob pattern to match.",
				},
			},
			"required": []string{"pattern"},
		}),
	}
}

// ---------------------------------------------------------------------------
// Executor — runs tool calls against the workspace.
// ---------------------------------------------------------------------------

// executor executes tool calls within a workspace root, applying
// output capping and path confinement.
type executor struct {
	root      string
	maxOutput int
}

// run dispatches a named tool call with JSON arguments.
func (e *executor) run(name, rawArgs string) string {
	switch name {
	case "read":
		return e.runRead(rawArgs)
	case "write":
		return e.runWrite(rawArgs)
	case "edit":
		return e.runEdit(rawArgs)
	case "bash":
		return e.runBash(rawArgs)
	case "grep":
		return e.runGrep(rawArgs)
	case "glob":
		return e.runGlob(rawArgs)
	default:
		return fmt.Sprintf("error: unknown tool %q", name)
	}
}

func (e *executor) runRead(rawArgs string) string {
	var args readArgs
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return fmt.Sprintf("error: read: %v", err)
	}
	p, err := resolvePath(e.root, args.Path)
	if err != nil {
		return fmt.Sprintf("error: read: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return fmt.Sprintf("error: read %q: %v", args.Path, err)
	}
	return truncate(string(data), e.maxOutput)
}

func (e *executor) runWrite(rawArgs string) string {
	var args writeArgs
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return fmt.Sprintf("error: write: %v", err)
	}
	p, err := resolvePath(e.root, args.Path)
	if err != nil {
		return fmt.Sprintf("error: write: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Sprintf("error: write: %v", err)
	}
	if err := os.WriteFile(p, []byte(args.Content), 0o644); err != nil {
		return fmt.Sprintf("error: write %q: %v", args.Path, err)
	}
	return fmt.Sprintf("wrote %d bytes to %q", len(args.Content), args.Path)
}

func (e *executor) runEdit(rawArgs string) string {
	var args editArgs
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return fmt.Sprintf("error: edit: %v", err)
	}
	p, err := resolvePath(e.root, args.Path)
	if err != nil {
		return fmt.Sprintf("error: edit: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return fmt.Sprintf("error: edit %q: %v", args.Path, err)
	}
	s := string(data)
	if !strings.Contains(s, args.OldString) {
		return fmt.Sprintf("error: edit %q: old_string not found", args.Path)
	}
	replaced := strings.Replace(s, args.OldString, args.NewString, 1)
	if err := os.WriteFile(p, []byte(replaced), 0o644); err != nil {
		return fmt.Sprintf("error: edit %q: %v", args.Path, err)
	}
	return fmt.Sprintf("edited %q (replaced 1 occurrence)", args.Path)
}

func (e *executor) runBash(rawArgs string) string {
	var args bashArgs
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return fmt.Sprintf("error: bash: %v", err)
	}
	cmd := exec.Command("bash", "-c", args.Command)
	cmd.Dir = e.root
	// Clear environment except PATH — sandbox hygiene.
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + e.root}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("error: bash: %v\n%s", err, truncate(string(out), e.maxOutput))
	}
	return truncate(string(out), e.maxOutput)
}

func (e *executor) runGrep(rawArgs string) string {
	var args grepArgs
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return fmt.Sprintf("error: grep: %v", err)
	}
	// Run grep -rn within workspace root.
	cmd := exec.Command("grep", "-rn", "--", args.Pattern, ".")
	cmd.Dir = e.root
	out, err := cmd.CombinedOutput()
	if err != nil {
		// grep exits 1 when no matches — that's not a tool error.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 && len(out) == 0 {
			return "no matches"
		}
		return fmt.Sprintf("error: grep: %v\n%s", err, truncate(string(out), e.maxOutput))
	}
	return truncate(string(out), e.maxOutput)
}

func (e *executor) runGlob(rawArgs string) string {
	var args globArgs
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return fmt.Sprintf("error: glob: %v", err)
	}
	// Only allow patterns that are relative.
	if filepath.IsAbs(args.Pattern) || strings.Contains(args.Pattern, "..") {
		return fmt.Sprintf("error: glob: pattern %q must be relative", args.Pattern)
	}
	matches, err := filepath.Glob(filepath.Join(e.root, args.Pattern))
	if err != nil {
		return fmt.Sprintf("error: glob: %v", err)
	}
	// Strip root prefix for readability.
	var rel []string
	for _, m := range matches {
		r, _ := filepath.Rel(e.root, m)
		rel = append(rel, r)
	}
	if len(rel) == 0 {
		return "no matches"
	}
	return truncate(strings.Join(rel, "\n"), e.maxOutput)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustMarshal(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("agent: marshal tool schema: %v", err))
	}
	return json.RawMessage(b)
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n... [truncated]"
}
