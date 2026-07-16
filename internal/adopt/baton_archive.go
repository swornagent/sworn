package adopt

import _ "embed"

// installerArchiveV0151 is the sole compiled owner of the complete offline
// Baton v0.15.1 installer input. Runtime consumers must use
// BatonInstallerArchive rather than consulting a checkout or repository path.
//
//go:embed baton/installer-input-v0.15.1.tar
var installerArchiveV0151 []byte

// BatonInstallerArchive returns an isolated copy of the exact embedded Baton
// v0.15.1 installer input. Callers cannot mutate the process-global embed.
func BatonInstallerArchive() []byte {
	return append([]byte(nil), installerArchiveV0151...)
}
