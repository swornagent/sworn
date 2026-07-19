//go:build linux

package effects

import (
	"os"
	"syscall"
)

func workspaceRootOwnedByCurrentUser(info os.FileInfo) bool {
	statistics, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(statistics.Uid) == os.Geteuid()
}
