//go:build !linux

package effects

import "os"

// Content-bound execution is Linux-only. Other targets retain the structural
// adapter for compilation and tests but cannot cross the real executor gate.
func workspaceRootOwnedByCurrentUser(os.FileInfo) bool { return true }
