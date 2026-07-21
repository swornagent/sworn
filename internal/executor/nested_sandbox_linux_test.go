//go:build linux

package executor

import "testing"

func TestBubblewrapNestedSandboxRequiresBothAdmissions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		requestNested   bool
		allowNested     bool
		wantDisableFlag bool
	}{
		{name: "default deny", wantDisableFlag: true},
		{name: "executor only", allowNested: true, wantDisableFlag: true},
		{name: "invocation only", requestNested: true, wantDisableFlag: true},
		{name: "double opt in", requestNested: true, allowNested: true, wantDisableFlag: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executor := &LinuxExecutor{options: Options{
				Limits:             DefaultLimits(),
				AllowNestedSandbox: test.allowNested,
			}}
			invocation := Invocation{
				NestedSandbox: test.requestNested,
				Argv:          []string{"/usr/bin/true"},
				Network:       NetworkNone,
			}
			arguments := executor.bubblewrapArgs(invocation, "/usr", "/workspace", "/inputs", false)
			if got := containsString(arguments, "--disable-userns"); got != test.wantDisableFlag {
				t.Fatalf("--disable-userns present = %t, want %t; arguments: %q", got, test.wantDisableFlag, arguments)
			}
		})
	}
}

func TestBubblewrapProbeRemainsNonNested(t *testing.T) {
	t.Parallel()
	executor := &LinuxExecutor{options: Options{AllowNestedSandbox: true}}
	arguments := executor.bubblewrapBaseArgs("/usr", NetworkNone, 1<<20, 1<<20, false)
	if !containsString(arguments, "--disable-userns") {
		t.Fatalf("probe arguments omitted --disable-userns: %q", arguments)
	}
}
