package driver

import (
	"github.com/swornagent/sworn/internal/gitx"
)

// tempRoot returns the configured machine/user temp root for ephemeral
// scratch (certification, projections, invocations, captures). It resolves
// from the SWORN_TEMP_ROOT override or the XDG-conformant default, never
// from a hardcoded literal, and creates the root when it is absent so
// os.MkdirTemp consumers find a usable parent. A resolution failure is
// propagated: no consumer may silently fall back to the process/system temp
// directory through os.MkdirTemp's empty-directory form.
func tempRoot() (string, error) {
	return gitx.ResolveTempRoot()
}
