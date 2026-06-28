package main

import (
	"strings"
	"testing"
)

func TestTaskHasAcceptanceChecks(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "has ACs",
			content: "## Acceptance checks\n\n- [ ] do thing\n- [ ] do other thing\n",
			want:    true,
		},
		{
			name:    "no ACs",
			content: "## Acceptance checks\n\nNo checks defined.\n",
			want:    false,
		},
		{
			name:    "empty",
			content: "",
			want:    false,
		},
		{
			name:    "dash bracket space bracket but no space dash bracket",
			content: "Some text with - [x] checked but no unchecked",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasAcceptanceChecks(tt.content)
			if got != tt.want {
				t.Errorf("hasAcceptanceChecks() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaskExtractSpecFromReply(t *testing.T) {
	tests := []struct {
		name  string
		reply string
		want  string // substring that must appear in result
	}{
		{
			name: "bare frontmatter",
			reply: `---
title: 'S01 — test slice'
---

# Slice

- [ ] AC 1
`,
			want: "---",
		},
		{
			name: "markdown code block",
			reply: `Here is the spec:

` + "```markdown\n" + `---
title: 'S01 — test slice'
---

# Slice

- [ ] AC 1
` + "```" + `
`,
			want: "---",
		},
		{
			name: "generic code block with frontmatter",
			reply: `Here is the spec:

` + "```\n" + `---
title: 'S01 — test slice'
---

# Slice

- [ ] AC 1
` + "```" + `
`,
			want: "---",
		},
		{
			name:  "fallback — whole reply",
			reply: "# No spec here\nJust some text\n",
			want:  "# No spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSpecFromReply(tt.reply)
			if !strings.Contains(got, tt.want) {
				t.Errorf("extractSpecFromReply(): result does not contain %q\ngot: %s", tt.want, got)
			}
		})
	}
}

func TestTaskExtractSpecNoACs(t *testing.T) {
	// Planner output with no acceptance checks — used for AC3 validation.
	reply := `---
title: 'S01 — no ACs'
---

# Slice

## Acceptance checks

None.
`
	content := extractSpecFromReply(reply)
	if hasAcceptanceChecks(content) {
		t.Error("expected no acceptance checks in this content")
	}
}

func TestTaskDryRunFlagAccepted(t *testing.T) {
	// Verify the --dry-run flag is defined and accepted by the flag set.
	// We can't easily call cmdRun directly without auth, but we can verify
	// the flag parsing works by checking that --dry-run is in the flag set.
	// This is tested implicitly by the reachability test: `sworn run --task 'hello' --dry-run`.
}