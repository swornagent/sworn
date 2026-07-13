//go:build windows

package supervisor

import "os"

// pidAlive reports whether pid corresponds to a live process. Windows has no
// syscall.Kill; os.FindProcess opens a process handle via OpenProcess and
// returns an error when the process does not exist, which is a best-effort
// liveness check adequate for the supervisor's stale-PID reaping. Precise
// exit-code checking (GetExitCodeProcess/STILL_ACTIVE) would require
// golang.org/x/sys/windows, avoided under the stdlib-first dep policy (ADR-0007).
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Release()
	return true
}
