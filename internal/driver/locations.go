package driver

import (
	"github.com/swornagent/sworn/internal/gitx"
)

// tempRoot returns the configured machine/user temp root for ephemeral
// scratch (certification, projections, invocations, captures). It resolves
// from the SWORN_TEMP_ROOT override or the XDG-conformant default, never
// from a hardcoded literal, and creates the root when it is absent so
// os.MkdirTemp consumers find a usable parent; an empty result (resolution
// failure) falls back to the system temp directory through os.MkdirTemp's
// empty-directory form.
func tempRoot() string {
	root, err := gitx.ResolveTempRoot()
	if err != nil {
		return ""
	}
	return root
}

// credentialsDir returns the configured machine/user credentials directory
// (SWORN_CREDENTIALS_DIR or the XDG-conformant default).
func credentialsDir() string {
	paths, err := gitx.LoadHostPaths()
	if err != nil {
		return ""
	}
	return paths.CredentialsDir
}

// artefactHome returns the configured machine/user artefact home
// (SWORN_ARTEFACT_HOME or the XDG-conformant default).
func artefactHome() string {
	paths, err := gitx.LoadHostPaths()
	if err != nil {
		return ""
	}
	return paths.ArtefactHome
}
