package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/swornagent/sworn/internal/config"
)

// settingsField holds one editable field in the settings panel.
type settingsField struct {
	label   string // display label
	value   string // current value
	envKey  string // non-empty for API key fields (maps to .env key)
	masked  bool   // show **** when true and not editing
	editing bool   // in edit mode
	cursor  int    // cursor position within value when editing
}

// SettingsView is a Bubble Tea component that provides a TUI settings panel.
// It is embedded in the root Model when the user presses 's' from the board view.
type SettingsView struct {
	fields     []settingsField
	cursor     int    // index of currently selected field
	config     config.Config
	envValues  map[string]string // loaded from ~/.sworn/.env
	saver      func(config.Config) error
	envWriter  func(map[string]string) error
	message    string
	errMessage string
	warningMsg string
}

// NewSettingsView creates a SettingsView populated from the current config and
// ~/.sworn/.env. The config is loaded via config.Load(); env vars via config.LoadEnv().
func NewSettingsView() (*SettingsView, error) {
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	envVals, err := config.LoadEnv()
	if err != nil {
		envVals = map[string]string{}
	}

	return NewSettingsViewWith(cfg, envVals, config.Save, config.WriteEnv), nil
}

// NewSettingsViewWith creates a SettingsView with explicit config, env, saver,
// and envWriter — useful for testing.
func NewSettingsViewWith(cfg config.Config, envVals map[string]string, saver func(config.Config) error, envWriter func(map[string]string) error) *SettingsView {
	escModels := strings.Join(cfg.Implementer.EscalationModels, ", ")

	return &SettingsView{
		config:    cfg,
		envValues: envVals,
		saver:     saver,
		envWriter: envWriter,
		fields: []settingsField{
			{label: "Verifier Model", value: cfg.Verifier.Model},
			{label: "Implementer Model", value: cfg.Implementer.Model},
			{label: "Escalation Models", value: escModels},
			{label: "Max Attempts", value: strconv.Itoa(cfg.Implementer.MaxAttempts)},
			{label: "OpenAI API Key", value: envVals["OPENAI_API_KEY"], envKey: "OPENAI_API_KEY", masked: true},
			{label: "Anthropic API Key", value: envVals["ANTHROPIC_API_KEY"], envKey: "ANTHROPIC_API_KEY", masked: true},
			{label: "Google API Key", value: envVals["GOOGLE_API_KEY"], envKey: "GOOGLE_API_KEY", masked: true},
			{label: "Groq API Key", value: envVals["GROQ_API_KEY"], envKey: "GROQ_API_KEY", masked: true},
			{label: "Mistral API Key", value: envVals["MISTRAL_API_KEY"], envKey: "MISTRAL_API_KEY", masked: true},
			{label: "DeepSeek API Key", value: envVals["DEEPSEEK_API_KEY"], envKey: "DEEPSEEK_API_KEY", masked: true},
			{label: "OpenRouter API Key", value: envVals["OPENROUTER_API_KEY"], envKey: "OPENROUTER_API_KEY", masked: true},
			{label: "Azure OpenAI API Key", value: envVals["AZURE_OPENAI_API_KEY"], envKey: "AZURE_OPENAI_API_KEY", masked: true},
			{label: "Azure OpenAI Endpoint", value: envVals["AZURE_OPENAI_ENDPOINT"], envKey: "AZURE_OPENAI_ENDPOINT"},
			{label: "OCI Compartment ID", value: envVals["OCI_COMPARTMENT_ID"], envKey: "OCI_COMPARTMENT_ID", masked: true},
			{label: "Ollama Host", value: envVals["OLLAMA_HOST"], envKey: "OLLAMA_HOST"},
		},
	}
}
// Init implements tea.Model.
func (sv *SettingsView) Init() tea.Cmd {
	return nil
}

// Update handles keyboard input for the settings panel.
func (sv *SettingsView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return sv, nil
	}

	// If editing a field, handle edit-mode keys.
	for i := range sv.fields {
		if sv.fields[i].editing {
			return sv.handleEditKey(i, keyMsg)
		}
	}

	// Non-edit mode.
	switch keyMsg.String() {
	case "ctrl+s":
		return sv.save()
	case "esc":
		// Discard changes — return to board (handled by root model).
		return sv, nil
	case "tab":
		sv.cursor = (sv.cursor + 1) % len(sv.fields)
		sv.warningMsg = ""
	case "shift+tab":
		sv.cursor = (sv.cursor - 1 + len(sv.fields)) % len(sv.fields)
		sv.warningMsg = ""
	case "up", "k":
		if sv.cursor > 0 {
			sv.cursor--
		}
		sv.warningMsg = ""
	case "down", "j":
		if sv.cursor < len(sv.fields)-1 {
			sv.cursor++
		}
		sv.warningMsg = ""
	case "enter":
		// Enter edit mode for the selected field.
		sv.fields[sv.cursor].editing = true
		sv.fields[sv.cursor].cursor = len(sv.fields[sv.cursor].value)
		sv.warningMsg = ""
	}

	return sv, nil
}

// handleEditKey handles keyboard input when a field is in edit mode.
func (sv *SettingsView) handleEditKey(idx int, msg tea.KeyMsg) (tea.Model, tea.Cmd) {	f := &sv.fields[idx]
	switch msg.String() {
	case "esc":
		// Cancel edit — restore original value.
		if f.envKey != "" {
			f.value = sv.envValues[f.envKey]
		} else {
			sv.restoreConfigField(idx)
		}
		f.editing = false
	case "enter":
		// Confirm edit.
		f.editing = false
	case "backspace":
		if f.cursor > 0 {
			f.value = f.value[:f.cursor-1] + f.value[f.cursor:]
			f.cursor--
		}
	case "delete":
		// Delete forward (some terminals send "delete" for forward-delete).
		if f.cursor < len(f.value) {
			f.value = f.value[:f.cursor] + f.value[f.cursor+1:]
		}
	case "left":
		if f.cursor > 0 {
			f.cursor--
		}
	case "right":
		if f.cursor < len(f.value) {
			f.cursor++
		}
	case "home":
		f.cursor = 0
	case "end":
		f.cursor = len(f.value)
	default:
		// For printable characters, tea.KeyMsg.String() returns the rune.
		if len(msg.String()) == 1 {
			r := msg.String()
			f.value = f.value[:f.cursor] + r + f.value[f.cursor:]
			f.cursor++
		}
	}
	return sv, nil
}

// restoreConfigField restores a config field's value from the stored config.
func (sv *SettingsView) restoreConfigField(idx int) {
	switch idx {
	case 0:
		sv.fields[idx].value = sv.config.Verifier.Model
	case 1:
		sv.fields[idx].value = sv.config.Implementer.Model
	case 2:
		sv.fields[idx].value = strings.Join(sv.config.Implementer.EscalationModels, ", ")
	case 3:
		sv.fields[idx].value = strconv.Itoa(sv.config.Implementer.MaxAttempts)
	}
}

// save validates, writes config + .env, and returns to board.
func (sv *SettingsView) save() (tea.Model, tea.Cmd) {	// Validate model fields are non-empty (warn only).
	if strings.TrimSpace(sv.fields[0].value) == "" {
		sv.warningMsg = "Warning: Verifier Model is empty"
		return sv, nil
	}
	if strings.TrimSpace(sv.fields[1].value) == "" {
		sv.warningMsg = "Warning: Implementer Model is empty"
		return sv, nil
	}

	// Validate max attempts is a positive integer.
	maxAttemptsStr := strings.TrimSpace(sv.fields[3].value)
	maxAttempts, err := strconv.Atoi(maxAttemptsStr)
	if err != nil || maxAttempts < 1 {
		sv.warningMsg = "Warning: Max Attempts must be a positive integer — not saved"
		// Restore existing value.
		sv.fields[3].value = strconv.Itoa(sv.config.Implementer.MaxAttempts)
		return sv, nil
	}

	// Parse escalation models.
	escModels := []string{}
	for _, m := range strings.Split(sv.fields[2].value, ",") {
		m = strings.TrimSpace(m)
		if m != "" {
			escModels = append(escModels, m)
		}
	}

	// Build updated config.
	cfg := sv.config
	cfg.Verifier.Model = strings.TrimSpace(sv.fields[0].value)
	cfg.Implementer.Model = strings.TrimSpace(sv.fields[1].value)
	cfg.Implementer.EscalationModels = escModels
	cfg.Implementer.MaxAttempts = maxAttempts

	if err := sv.saver(cfg); err != nil {
		sv.errMessage = fmt.Sprintf("Save failed: %v", err)
		return sv, nil
	}
	sv.config = cfg

	// Write API keys to .env.
	envUpdates := map[string]string{}
	for _, f := range sv.fields {
		if f.envKey != "" && strings.TrimSpace(f.value) != "" {
			envUpdates[f.envKey] = f.value
		}
	}
	if len(envUpdates) > 0 {
		if err := sv.envWriter(envUpdates); err != nil {			sv.errMessage = fmt.Sprintf("Env save failed: %v", err)
			return sv, nil
		}
		// Update local env cache.
		for k, v := range envUpdates {
			sv.envValues[k] = v
		}
	}

	sv.message = "Saved!"
	return sv, nil
}

// View renders the settings panel.
func (sv *SettingsView) View() string {
	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().
		Foreground(colAccent).
		Bold(true).
		Padding(0, 1)
	sb.WriteString(titleStyle.Render("Settings"))
	sb.WriteString("\n\n")

	fieldLabelStyle := lipgloss.NewStyle().
		Foreground(colText).
		Width(22)
	fieldValueStyle := lipgloss.NewStyle().
		Foreground(colDim)
	fieldEditStyle := lipgloss.NewStyle().
		Foreground(colText).
		Background(colBgSel)
	fieldCursorStyle := lipgloss.NewStyle().
		Foreground(colPrimary).
		Bold(true)
	warningStyle := lipgloss.NewStyle().
		Foreground(colWarn)
	successStyle := lipgloss.NewStyle().
		Foreground(colAccent)
	errorStyle := lipgloss.NewStyle().
		Foreground(colFail)
	dimStyle := lipgloss.NewStyle().
		Foreground(colMuted).
		Italic(true)
	sectionStyle := lipgloss.NewStyle().
		Foreground(colPrimary).
		Bold(true).
		Padding(0, 1)

	// Model config section.
	sb.WriteString(sectionStyle.Render("Model Configuration"))
	sb.WriteString("\n")
	for i := range 4 {
		f := sv.fields[i]
		cursor := "  "
		if sv.cursor == i {
			cursor = fieldCursorStyle.Render("▸ ")
		}

		displayVal := f.value
		if f.editing {
			// Show value with cursor indicator.
			prefix := displayVal[:f.cursor]
			suffix := displayVal[f.cursor:]
			displayVal = prefix + "│" + suffix
		}

		line := fmt.Sprintf("%s%s%s\n",
			cursor,
			fieldLabelStyle.Render(f.label+":"),
			renderFieldValue(f, displayVal, fieldValueStyle, fieldEditStyle),
		)
		sb.WriteString(line)
	}
	sb.WriteString("\n")

	// API key section.
	sb.WriteString(sectionStyle.Render("API Keys"))
	sb.WriteString("\n")
	for i := 4; i < len(sv.fields); i++ {
		f := sv.fields[i]
		cursor := "  "
		if sv.cursor == i {
			cursor = fieldCursorStyle.Render("▸ ")
		}

		displayVal := f.value
		if f.masked && !f.editing && f.value != "" {
			displayVal = "****"
		}
		if f.editing {
			prefix := displayVal[:f.cursor]
			suffix := displayVal[f.cursor:]
			displayVal = prefix + "│" + suffix
		}

		line := fmt.Sprintf("%s%s%s\n",
			cursor,
			fieldLabelStyle.Render(f.label+":"),
			renderFieldValue(f, displayVal, fieldValueStyle, fieldEditStyle),
		)
		sb.WriteString(line)
	}
	sb.WriteString("\n")

	// Messages.
	if sv.warningMsg != "" {
		sb.WriteString(warningStyle.Render(sv.warningMsg))
		sb.WriteString("\n")
	}
	if sv.message != "" {
		sb.WriteString(successStyle.Render(sv.message))
		sb.WriteString("\n")
	}
	if sv.errMessage != "" {
		sb.WriteString(errorStyle.Render("Error: " + sv.errMessage))
		sb.WriteString("\n")
	}

	// Help bar.
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("↑/↓ navigate  Tab/Shift+Tab next/prev  Enter edit  Ctrl+S save  Esc back"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("API keys show as **** when set. Navigate to a field and press Enter to view/edit."))

	return sb.String()
}

// renderFieldValue renders a field value with the appropriate style.
func renderFieldValue(f settingsField, displayVal string, normalStyle, editStyle lipgloss.Style) string {
	if f.editing {
		return editStyle.Render(displayVal)
	}
	if displayVal == "" {
		return normalStyle.Render("(not set)")
	}
	return normalStyle.Render(displayVal)
}