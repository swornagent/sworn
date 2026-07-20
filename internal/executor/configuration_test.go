package executor

import (
	"strings"
	"testing"
	"time"
)

func TestExecutorConfigurationDigestCanonicalAndComplete(t *testing.T) {
	base := Options{
		RuntimeRoot:        "/run/user/1000/sworn-runtime",
		WritableRoot:       "/run/user/1000/sworn-writable",
		ShimArgv:           []string{"/opt/sworn", "__executor-shim", "argument"},
		BubblewrapPath:     "/usr/bin/bwrap",
		SystemdRunPath:     "/usr/bin/systemd-run",
		SystemctlPath:      "/usr/bin/systemctl",
		Limits:             DefaultLimits(),
		AllowedEnvironment: []string{"SECOND", "FIRST"},
		AllowHostNetwork:   false,
	}
	baseline := executorConfigurationDigest(base)
	if !strings.HasPrefix(baseline, "sha256:") || len(baseline) != len("sha256:")+64 {
		t.Fatalf("configuration digest = %q", baseline)
	}

	canonical := cloneExecutorOptions(base)
	canonical.AllowedEnvironment = []string{"FIRST", "SECOND", "FIRST"}
	if got := executorConfigurationDigest(canonical); got != baseline {
		t.Fatalf("allowlist set-equivalent digest = %q, want %q", got, baseline)
	}

	mutations := []struct {
		name   string
		mutate func(*Options)
	}{
		{"runtime root", func(options *Options) { options.RuntimeRoot += "-other" }},
		{"writable root", func(options *Options) { options.WritableRoot += "-other" }},
		{"shim executable", func(options *Options) { options.ShimArgv[0] += "-other" }},
		{"shim argument", func(options *Options) { options.ShimArgv[1] += "-other" }},
		{"shim argument count", func(options *Options) { options.ShimArgv = append(options.ShimArgv, "other") }},
		{"bubblewrap", func(options *Options) { options.BubblewrapPath += "-other" }},
		{"systemd-run", func(options *Options) { options.SystemdRunPath += "-other" }},
		{"systemctl", func(options *Options) { options.SystemctlPath += "-other" }},
		{"runtime limit", func(options *Options) { options.Limits.Runtime += time.Nanosecond }},
		{"memory limit", func(options *Options) { options.Limits.MemoryBytes++ }},
		{"swap limit", func(options *Options) { options.Limits.SwapBytes++ }},
		{"task limit", func(options *Options) { options.Limits.Tasks++ }},
		{"cpu limit", func(options *Options) { options.Limits.CPUPercent++ }},
		{"file limit", func(options *Options) { options.Limits.FileBytes++ }},
		{"temp limit", func(options *Options) { options.Limits.TempBytes++ }},
		{"home limit", func(options *Options) { options.Limits.HomeBytes++ }},
		{"input limit", func(options *Options) { options.Limits.InputBytes++ }},
		{"workspace limit", func(options *Options) { options.Limits.WorkspaceBytes++ }},
		{"stdout limit", func(options *Options) { options.Limits.StdoutBytes++ }},
		{"stderr limit", func(options *Options) { options.Limits.StderrBytes++ }},
		{"allowlist", func(options *Options) { options.AllowedEnvironment = append(options.AllowedEnvironment, "THIRD") }},
		{"host network", func(options *Options) { options.AllowHostNetwork = true }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			changed := cloneExecutorOptions(base)
			mutation.mutate(&changed)
			if got := executorConfigurationDigest(changed); got == baseline {
				t.Fatalf("configuration mutation did not change %q", baseline)
			}
		})
	}

	boundaryA := cloneExecutorOptions(base)
	boundaryA.ShimArgv = []string{"/opt/sworn", "ab", "c"}
	boundaryB := cloneExecutorOptions(base)
	boundaryB.ShimArgv = []string{"/opt/sworn", "a", "bc"}
	if executorConfigurationDigest(boundaryA) == executorConfigurationDigest(boundaryB) {
		t.Fatal("shim argument boundaries collided")
	}
}

func cloneExecutorOptions(options Options) Options {
	options.ShimArgv = append([]string(nil), options.ShimArgv...)
	options.AllowedEnvironment = append([]string(nil), options.AllowedEnvironment...)
	return options
}
