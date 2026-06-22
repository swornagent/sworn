package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// WriteContextFile writes a context file .sworn-context.md to the worktree path.
func WriteContextFile(worktreePath, spec, violations, diff string) (string, error) {
	content := fmt.Sprintf("# Sworn Context\n\n## Spec\n%s\n\n## Violations\n%s\n\n## Diff\n%s\n", spec, violations, diff)
	path := filepath.Join(worktreePath, ".sworn-context.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return path, nil
}

// LaunchClaudeCode launches VS Code with Claude extension or configured SWORN_CLAUDE_CODE_CMD.
func LaunchClaudeCode(worktreePath string) error {
	cmdName := os.Getenv("SWORN_CLAUDE_CODE_CMD")
	if cmdName == "" {
		cmdName = "code"
	}
	if _, err := exec.LookPath(cmdName); err != nil {
		return fmt.Errorf("command %q not found in PATH", cmdName)
	}
	cmd := exec.Command(cmdName, worktreePath)
	return cmd.Start()
}

// LaunchCodex launches Codex or configured SWORN_CODEX_CMD.
func LaunchCodex(worktreePath string) error {
	cmdName := os.Getenv("SWORN_CODEX_CMD")
	if cmdName == "" {
		cmdName = "codex"
	}
	if _, err := exec.LookPath(cmdName); err != nil {
		return fmt.Errorf("command %q not found in PATH", cmdName)
	}
	cmd := exec.Command(cmdName, worktreePath)
	return cmd.Start()
}
