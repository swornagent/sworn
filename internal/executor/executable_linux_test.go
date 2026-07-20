//go:build linux

package executor

import "testing"

func TestBubblewrapMountsSelectedExecutableAsItsExactInput(t *testing.T) {
	t.Parallel()
	executor := &LinuxExecutor{options: Options{Limits: DefaultLimits()}}
	invocation := Invocation{
		ExecutableInput: "codex",
		Inputs:          []Input{{Name: "codex"}},
		Argv:            []string{"/inputs/codex", "exec"},
		Network:         NetworkNone,
	}
	arguments := executor.bubblewrapArgs(invocation, "/usr", "/workspace", "/runtime/inputs", false)
	want := []string{"--ro-bind", "/runtime/inputs/codex", "/inputs/codex"}
	if !containsArgumentSequence(arguments, want) {
		t.Fatalf("selected input mount absent: arguments=%q want sequence=%q", arguments, want)
	}
	if !containsArgumentSequence(arguments, []string{"--", "/inputs/codex", "exec"}) {
		t.Fatalf("selected input argv absent: %q", arguments)
	}
	if containsString(arguments, "/tools") {
		t.Fatalf("selected input introduced a parallel tools mount: %q", arguments)
	}
}

func containsArgumentSequence(arguments, sequence []string) bool {
	for start := 0; start+len(sequence) <= len(arguments); start++ {
		match := true
		for offset := range sequence {
			if arguments[start+offset] != sequence[offset] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
