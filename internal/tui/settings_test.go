package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/swornagent/sworn/internal/config"
)

func TestSettingsPanel_OpensWithCurrentConfig(t *testing.T) {
	cfg := config.Config{
		Version: 1,
		Verifier: config.ModelSetting{
			Model: "anthropic/claude-sonnet-4-6",
		},
		Implementer: config.ModelSetting{
			Model:            "openai/gpt-4o-mini",
			EscalationModels: []string{"openai/gpt-4o", "openai/o3"},
			MaxAttempts:      3,
		},
	}
	env := map[string]string{}

	sv := NewSettingsViewWith(cfg, env, nil, nil)

	// The rendered view should contain the verifier model string.
	view := sv.View()
	if !strings.Contains(view, "anthropic/claude-sonnet-4-6") {
		t.Errorf("view should contain verifier model, got:\n%s", view)
	}
	if !strings.Contains(view, "openai/gpt-4o-mini") {
		t.Errorf("view should contain implementer model, got:\n%s", view)
	}
	if !strings.Contains(view, "Verifier Model") {
		t.Errorf("view should contain 'Verifier Model' label")
	}
}

func TestSettingsPanel_MasksAPIKey(t *testing.T) {
	cfg := config.DefaultConfig()
	env := map[string]string{
		"OPENAI_API_KEY": "sk-secret-12345",
	}

	sv := NewSettingsViewWith(cfg, env, nil, nil)

	view := sv.View()

	// The view should show **** (masked) for the OpenAI key.
	if !strings.Contains(view, "****") {
		t.Errorf("view should show masked key as ****, got:\n%s", view)
	}
	// The actual key should NOT appear.
	if strings.Contains(view, "sk-secret-12345") {
		t.Errorf("view should NOT contain the raw API key, got:\n%s", view)
	}
}

func TestSettingsPanel_SaveWritesConfig(t *testing.T) {
	cfg := config.Config{
		Version: 1,
		Verifier: config.ModelSetting{
			Model: "anthropic/claude-sonnet-4-6",
		},
		Implementer: config.ModelSetting{
			Model:            "openai/gpt-4o-mini",
			EscalationModels: []string{},
			MaxAttempts:      3,
		},
	}
	env := map[string]string{}

	var savedCfg config.Config
	saver := func(cfg config.Config) error {
		savedCfg = cfg
		return nil
	}
	envWriter := func(updates map[string]string) error {
		return nil
	}

	sv := NewSettingsViewWith(cfg, env, saver, envWriter)

	// Edit the Verifier Model field (index 0) to a new value.
	sv.fields[0].value = "openai/gpt-4.1"

	// Call save directly — tests that saver is called with the edited value.
	model, _ := sv.save()
	sv = model.(*SettingsView)

	// After save, the verifier model should be updated.
	if savedCfg.Verifier.Model != "openai/gpt-4.1" {
		t.Errorf("saved config Verifier.Model = %q, want openai/gpt-4.1", savedCfg.Verifier.Model)
	}
}

func TestSettingsPanel_EscapeDiscards(t *testing.T) {
	cfg := config.Config{
		Version: 1,
		Verifier: config.ModelSetting{
			Model: "anthropic/claude-sonnet-4-6",
		},
		Implementer: config.ModelSetting{
			Model:       "openai/gpt-4o-mini",
			MaxAttempts: 3,
		},
	}
	env := map[string]string{}

	saveCalled := false
	saver := func(cfg config.Config) error {
		saveCalled = true
		return nil
	}
	envWriter := func(updates map[string]string) error {
		return nil
	}

	sv := NewSettingsViewWith(cfg, env, saver, envWriter)

	// Enter edit mode and change the value, then cancel with Esc.
	sv.fields[0].editing = true
	sv.fields[0].value = "changed-model"

	// Press Esc to cancel edit — this should restore original value.
	sv.handleEditKey(0, tea.KeyMsg{Type: tea.KeyEscape})

	// Save should NOT have been called (Esc cancels edit, save is only via Ctrl+S).
	if saveCalled {
		t.Error("save should not have been called after Esc cancel edit")
	}
	// The model value should be restored to original.
	if sv.fields[0].value != "anthropic/claude-sonnet-4-6" {
		t.Errorf("model value was not restored after Esc, got %q", sv.fields[0].value)
	}
}

func TestSettingsPanel_InvalidMaxAttempts(t *testing.T) {
	cfg := config.Config{
		Version: 1,
		Verifier: config.ModelSetting{
			Model: "anthropic/claude-sonnet-4-6",
		},
		Implementer: config.ModelSetting{
			Model:       "openai/gpt-4o-mini",
			MaxAttempts: 3,
		},
	}
	env := map[string]string{}

	saveCalled := false
	saver := func(cfg config.Config) error {
		saveCalled = true
		return nil
	}
	envWriter := func(updates map[string]string) error {
		return nil
	}

	sv := NewSettingsViewWith(cfg, env, saver, envWriter)

	// Set max attempts to "abc" (invalid).
	sv.fields[3].value = "abc"

	// Call save directly.
	model, _ := sv.save()
	sv = model.(*SettingsView)

	// Should show a warning.
	if sv.warningMsg == "" {
		t.Error("expected warning for invalid max attempts")
	}
	// Save should NOT have been called.
	if saveCalled {
		t.Error("save should not have been called with invalid max attempts")
	}
	// The field should be restored to the original value.
	if sv.fields[3].value != "3" {
		t.Errorf("max attempts field should be restored to '3', got %q", sv.fields[3].value)
	}
}