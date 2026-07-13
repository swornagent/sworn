//go:build !windows

package supervisor

import "syscall"

// pidAlive reports whether pid corresponds to a live process. It uses
// syscall.Kill(pid, 0) — the POSIX-specified way to test process existence
// without actually sending a signal.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, syscall.Signal(0)) == nil
}
