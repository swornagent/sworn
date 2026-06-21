package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/swornagent/sworn/internal/state"
	"gopkg.in/yaml.v3"
)

// BlockedView is a Bubble Tea component for resolving blocked/failed slices.
type BlockedView struct {
	repoRoot     string
	releaseName  string
	sliceID      string
	track        string
	worktreePath string
	violations   []string
	proofContent string

	// UI state
	viewingProof bool
	deferring    bool
	deferInput   textinput.Model
	message      string
	errMessage   string
}

// ExtractViolations parses proof.md content and extracts bullet points under
// "## Violations" or "## Not delivered" sections.
func ExtractViolations(proofContent string) []string {
	var violations []string
	lines := strings.Split(proofContent, "\n")
	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			heading := strings.ToLower(strings.TrimPrefix(trimmed, "## "))
			if strings.Contains(heading, "violations") || strings.Contains(heading, "not delivered") {
				inSection = true
			} else {
				inSection = false
			}
			continue
		}
		if inSection {
			if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
				violation := strings.TrimSpace(trimmed[2:])
				if violation != "" {
					violations = append(violations, violation)
				}
			}
		}
	}
	return violations
}

// AppendDeferralToIntake appends a Rule 2 deferral to intake.md.
func AppendDeferralToIntake(intakePath, sliceID, reason string) error {
	data, err := os.ReadFile(intakePath)
	if err != nil {
		if os.IsNotExist(err) {
			header := fmt.Sprintf("# Release Intake\n\n## Adjacent / out of scope (Rule 2 deferrals)\n\n")
			data = []byte(header)
		} else {
			return err
		}
	}

	content := string(data)
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	deferralLine := fmt.Sprintf("- **%s**: %s. **Why**: Deferred by user from TUI. **Tracking**: %s. **Acknowledged**: %s.\n", sliceID, reason, sliceID, timestamp)

	sectionHeading := "## Adjacent / out of scope (Rule 2 deferrals)"
	idx := strings.Index(content, sectionHeading)
	if idx == -1 {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n" + sectionHeading + "\n\n" + deferralLine
	} else {
		rest := content[idx+len(sectionHeading):]
		nextHeadingIdx := strings.Index(rest, "\n## ")
		if nextHeadingIdx == -1 {
			if !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			content += deferralLine
		} else {
			insertPos := idx + len(sectionHeading) + nextHeadingIdx
			content = content[:insertPos] + "\n" + deferralLine + content[insertPos:]
		}
	}

	return os.WriteFile(intakePath, []byte(content), 0644)
}

// LoadBlockedView loads the blocked view for a slice.
func LoadBlockedView(repoRoot, releaseName, sliceID string) (*BlockedView, error) {
	statusPath := filepath.Join(repoRoot, "docs", "release", releaseName, sliceID, "status.json")
	st, err := state.Read(statusPath)
	if err != nil {
		return nil, err
	}

	worktreePath := ""
	indexPath := filepath.Join(repoRoot, "docs", "release", releaseName, "index.md")
	indexData, err := os.ReadFile(indexPath)
	if err == nil {
		type TrackInfoWithWT struct {
			ID           string `yaml:"id"`
			WorktreePath string `yaml:"worktree_path"`
		}
		type indexFM struct {
			Tracks []TrackInfoWithWT `yaml:"tracks"`
		}
		var fm indexFM
		rest := strings.TrimPrefix(string(indexData), "---")
		parts := strings.SplitN(rest, "---", 2)
		if len(parts) >= 1 {
			_ = yaml.Unmarshal([]byte(parts[0]), &fm)
		}
		for _, t := range fm.Tracks {
			if t.ID == st.Track {
				worktreePath = t.WorktreePath
				break
			}
		}
	}
	if worktreePath == "" {
		worktreePath = repoRoot
	}

	proofPath := filepath.Join(repoRoot, "docs", "release", releaseName, sliceID, "proof.md")
	proofData, _ := os.ReadFile(proofPath)
	proofContent := string(proofData)

	violations := ExtractViolations(proofContent)

	return &BlockedView{
		repoRoot:     repoRoot,
		releaseName:  releaseName,
		sliceID:      sliceID,
		track:        st.Track,
		worktreePath: worktreePath,
		violations:   violations,
		proofContent: proofContent,
	}, nil
}

// Update handles keyboard input for BlockedView.
func (b *BlockedView) Update(msg tea.Msg) (*BlockedView, tea.Cmd) {
	if b.viewingProof {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc", "q":
				b.viewingProof = false
			}
		}
		return b, nil
	}

	if b.deferring {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				b.deferring = false
			case "enter":
				if strings.TrimSpace(b.deferInput.Value()) != "" {
					if err := b.deferSlice(b.deferInput.Value()); err != nil {
						b.errMessage = err.Error()
					} else {
						b.message = "Slice deferred successfully!"
						b.deferring = false
					}
				}
			default:
				b.deferInput, _ = b.deferInput.Update(msg)
			}
		}
		return b, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "1":
			b.message = "Auto-fix + rerun is not implemented yet. Run 'sworn run --slice " + b.sliceID + " --release " + b.releaseName + "' in your terminal."
		case "2":
			specPath := filepath.Join(b.repoRoot, "docs", "release", b.releaseName, b.sliceID, "spec.md")
			specData, _ := os.ReadFile(specPath)

			diffCmd := exec.Command("git", "-C", b.worktreePath, "diff")
			diffData, _ := diffCmd.Output()

			violationsStr := strings.Join(b.violations, "\n")
			ctxPath, err := WriteContextFile(b.worktreePath, string(specData), violationsStr, string(diffData))
			if err != nil {
				b.errMessage = "Failed to write context file: " + err.Error()
				return b, nil
			}

			errLaunch := LaunchClaudeCode(b.worktreePath)
			if errLaunch != nil {
				b.message = fmt.Sprintf("Claude Code not found — context written to %s", ctxPath)
			} else {
				b.message = fmt.Sprintf("Context written to %s and Claude Code launched!", ctxPath)
			}
		case "3":
			specPath := filepath.Join(b.repoRoot, "docs", "release", b.releaseName, b.sliceID, "spec.md")
			specData, _ := os.ReadFile(specPath)

			diffCmd := exec.Command("git", "-C", b.worktreePath, "diff")
			diffData, _ := diffCmd.Output()

			violationsStr := strings.Join(b.violations, "\n")
			ctxPath, err := WriteContextFile(b.worktreePath, string(specData), violationsStr, string(diffData))
			if err != nil {
				b.errMessage = "Failed to write context file: " + err.Error()
				return b, nil
			}

			errLaunch := LaunchCodex(b.worktreePath)
			if errLaunch != nil {
				b.message = fmt.Sprintf("Codex not found — context written to %s", ctxPath)
			} else {
				b.message = fmt.Sprintf("Context written to %s and Codex launched!", ctxPath)
			}
		case "4":
			b.viewingProof = true
		case "5":
			b.deferring = true
			ti := textinput.New()
			ti.Placeholder = "One-line reason for deferring..."
			ti.Focus()
			b.deferInput = ti
			b.message = ""
			b.errMessage = ""
		}
	}

	return b, nil
}

func (b *BlockedView) deferSlice(reason string) error {
	statusPath := filepath.Join(b.repoRoot, "docs", "release", b.releaseName, b.sliceID, "status.json")
	st, err := state.Read(statusPath)
	if err != nil {
		return err
	}
	st.State = state.Deferred
	st.LastUpdatedBy = "implementer"
	st.LastUpdatedAt = time.Now().Format(time.RFC3339)
	if err := state.Write(statusPath, st); err != nil {
		return err
	}

	intakePath := filepath.Join(b.repoRoot, "docs", "release", b.releaseName, "intake.md")
	if err := AppendDeferralToIntake(intakePath, b.sliceID, reason); err != nil {
		return err
	}

	return nil
}

// View renders the BlockedView.
func (b *BlockedView) View() string {
	if b.viewingProof {
		var sb strings.Builder
		sb.WriteString(lipgloss.NewStyle().Foreground(colAccent).Bold(true).Render("Proof Bundle: " + b.sliceID))
		sb.WriteString("\n\n")
		if b.proofContent == "" {
			sb.WriteString(lipgloss.NewStyle().Foreground(colMuted).Italic(true).Render("No proof.md found or empty."))
		} else {
			sb.WriteString(b.proofContent)
		}
		sb.WriteString("\n\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(colDim).Render("Press Esc to return to options menu."))
		return sb.String()
	}

	if b.deferring {
		var sb strings.Builder
		sb.WriteString(lipgloss.NewStyle().Foreground(colWarn).Bold(true).Render("Defer Slice: " + b.sliceID))
		sb.WriteString("\n\n")
		sb.WriteString("Enter a one-line reason for deferring this slice:\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(colAccent).Render("> " + b.deferInput.View()))
		sb.WriteString("\n\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(colDim).Render("Press Enter to confirm, Esc to cancel."))
		return sb.String()
	}
	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Foreground(colFail).Bold(true).Render("Blocked Slice Resolution: " + b.sliceID))
	sb.WriteString("\n\n")

	sb.WriteString(fmt.Sprintf("Release:       %s\n", b.releaseName))
	sb.WriteString(fmt.Sprintf("Track:         %s\n", b.track))
	sb.WriteString(fmt.Sprintf("Worktree Path: %s\n", b.worktreePath))
	sb.WriteString("\n")

	sb.WriteString(lipgloss.NewStyle().Foreground(colWarn).Bold(true).Render("Violations Summary:"))
	sb.WriteString("\n")
	if len(b.violations) == 0 {
		sb.WriteString("  No violations extracted from proof.md.\n")
	} else {
		for _, v := range b.violations {
			sb.WriteString(fmt.Sprintf("  • %s\n", v))
		}
	}
	sb.WriteString("\n")

	sb.WriteString(lipgloss.NewStyle().Foreground(colAccent).Bold(true).Render("Resolution Options:"))
	sb.WriteString("\n")
	sb.WriteString("  [1] Auto-fix + rerun\n")
	sb.WriteString("  [2] Open in Claude Code\n")
	sb.WriteString("  [3] Open in Codex\n")
	sb.WriteString("  [4] View full proof bundle\n")
	sb.WriteString("  [5] Defer slice\n")
	sb.WriteString("\n")

	if b.message != "" {
		sb.WriteString(lipgloss.NewStyle().Foreground(colAccent).Bold(true).Render(b.message))
		sb.WriteString("\n\n")
	}
	if b.errMessage != "" {
		sb.WriteString(lipgloss.NewStyle().Foreground(colFail).Bold(true).Render("Error: " + b.errMessage))
		sb.WriteString("\n\n")
	}

	sb.WriteString(lipgloss.NewStyle().Foreground(colDim).Render("Press Esc to return to board view."))
	return sb.String()
}
