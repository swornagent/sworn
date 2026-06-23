package style

import (
	"os"
	"testing"
)

// saveRestore saves the enabled var and returns a function to restore it.
// Tests that need to override the colour gate must use:
//
//	t.Cleanup(saveRestore())
//	enabled = <test value>
//
// Without the cleanup, subsequent tests in the same package would see the
// stale override.
func saveRestore() func() {
	old := enabled
	return func() { enabled = old }
}

func TestEnabled_NoColor(t *testing.T) {
	t.Cleanup(saveRestore())

	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")
	enabled = detect()
	if Enabled() {
		t.Error("Expected Enabled()=false when NO_COLOR is set")
	}
}

func TestEnabled_ForceColor(t *testing.T) {
	t.Cleanup(saveRestore())

	os.Setenv("NO_COLOR", "")
	os.Setenv("SWORN_FORCE_COLOR", "1")
	defer os.Unsetenv("SWORN_FORCE_COLOR")
	enabled = detect()
	if !Enabled() {
		t.Error("Expected Enabled()=true when SWORN_FORCE_COLOR is set")
	}
}

func TestEnabled_NonTTY(t *testing.T) {
	t.Cleanup(saveRestore())

	os.Setenv("NO_COLOR", "")
	os.Setenv("SWORN_FORCE_COLOR", "")
	// Under go test, stdout is not a TTY, so detect should return false.
	enabled = detect()
	if Enabled() {
		t.Error("Expected Enabled()=false when stdout is not a TTY")
	}
}

func TestEnabled_DisabledReturnsPlain(t *testing.T) {
	t.Cleanup(saveRestore())

	enabled = false

	tests := []struct {
		name string
		fn   func(string) string
		in   string
	}{
		{"Bold", Bold, "hello"},
		{"Dim", Dim, "hello"},
		{"Heading", Heading, "hello"},
		{"Success", Success, "hello"},
		{"Warn", Warn, "hello"},
		{"Danger", Danger, "hello"},
		{"Accent", Accent, "hello"},
		{"Verdict PASS", func(s string) string { return Verdict("PASS") }, "PASS"},
		{"Verdict FAIL", func(s string) string { return Verdict("FAIL") }, "FAIL"},
		{"Verdict BLOCKED", func(s string) string { return Verdict("BLOCKED") }, "BLOCKED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.in)
			if got != tt.in {
				t.Errorf("%q → %q; want unchanged when disabled", tt.in, got)
			}
		})
	}
}

func TestEnabled_EnabledReturnsAnsi(t *testing.T) {
	t.Cleanup(saveRestore())

	enabled = true

	tests := []struct {
		name string
		fn   func(string) string
		in   string
	}{
		{"Bold", Bold, "hello"},
		{"Dim", Dim, "hello"},
		{"Heading", Heading, "hello"},
		{"Success", Success, "hello"},
		{"Warn", Warn, "hello"},
		{"Danger", Danger, "hello"},
		{"Accent", Accent, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(tt.in)
			if got == tt.in {
				t.Errorf("%q unchanged; want ANSI wrapping when enabled", tt.in)
			}
			// Check it contains an escape sequence
			if !containsEscape(got) {
				t.Errorf("%q: no ANSI escape found", tt.in)
			}
		})
	}
}

func TestVerdict(t *testing.T) {
	t.Cleanup(saveRestore())
	enabled = true

	tests := []struct {
		token string
		want  string
	}{
		{"PASS", cGreen + "PASS" + reset},
		{"FAIL", cRed + "FAIL" + reset},
		{"BLOCKED", cYellow + "BLOCKED" + reset},
		{"SKIP", cYellow + "SKIP" + reset},
	}

	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			got := Verdict(tt.token)
			if got != tt.want {
				t.Errorf("Verdict(%q) = %q; want %q", tt.token, got, tt.want)
			}
		})
	}
}

func TestBanner(t *testing.T) {
	t.Cleanup(saveRestore())
	enabled = true

	t.Run("with title", func(t *testing.T) {
		got := Banner("init")
		if !containsEscape(got) {
			t.Error("Banner with title should contain ANSI escapes")
		}
		// The title part should be dimmed
		if !containsSubstring(got, "· init") {
			t.Errorf("Banner %q missing title separator", got)
		}
	})

	t.Run("empty title", func(t *testing.T) {
		t.Cleanup(saveRestore())
		enabled = true
		got := Banner("")
		if got == "" {
			t.Error("Banner with empty title should still return wordmark")
		}
	})
}

func TestRule(t *testing.T) {
	t.Cleanup(saveRestore())
	enabled = true

	got := Rule(10)
	// Should be dimmed box-drawing characters
	if len(got) == 0 {
		t.Error("Rule should return non-empty string")
	}
}

func TestEmptyString(t *testing.T) {
	t.Cleanup(saveRestore())
	enabled = true

	// Empty input must return empty output — never a bare escape sequence.
	if Bold("") != "" {
		t.Error("Bold(\"\") should return \"\"")
	}
	if Dim("") != "" {
		t.Error("Dim(\"\") should return \"\"")
	}
}

func TestDetect_NoColorEnv(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")
	if detect() {
		t.Error("detect() with NO_COLOR=1 should return false")
	}
}

func TestDetect_ForceColorEnv(t *testing.T) {
	os.Setenv("NO_COLOR", "")
	os.Setenv("SWORN_FORCE_COLOR", "1")
	defer os.Unsetenv("SWORN_FORCE_COLOR")
	if !detect() {
		t.Error("detect() with SWORN_FORCE_COLOR=1 should return true")
	}
}

func containsEscape(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\033' {
			return true
		}
	}
	return false
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
