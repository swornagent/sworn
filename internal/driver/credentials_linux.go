//go:build linux

package driver

import (
	"os"
	"path/filepath"
	"syscall"
)

type linuxCredentialLease struct {
	file    *os.File
	path    string
	before  syscall.Stat_t
	maximum int64
	closed  bool
	rotated bool
}

// openCredentialPreflight opens the credential path for the read-only
// dispatch-preparation expiry probe. It mirrors the lease's posture (no
// symlink following, close-on-exec) but takes no lock and changes nothing:
// any failure passes preflight and the exclusive lease re-validates identity
// and shape before the credential reaches the CLI.
func openCredentialPreflight(pathValue string) (*os.File, error) {
	descriptor, err := syscall.Open(
		pathValue,
		syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(descriptor), "sworn-native-credential-preflight")
	if file == nil {
		_ = syscall.Close(descriptor)
		return nil, fail("CREDENTIAL_NOT_CERTIFIED")
	}
	return file, nil
}

func acquireFileCredential(
	pathValue string,
	workspace string,
	maximum int64,
) (nativeCredentialLease, error) {
	if pathValue == "" || !filepath.IsAbs(pathValue) ||
		filepath.Clean(pathValue) != pathValue ||
		pathBeneath(workspace, pathValue) ||
		maximum < 1 || maximum > 1_048_576 {
		return nil, fail("CREDENTIAL_NOT_CERTIFIED")
	}
	descriptor, err := syscall.Open(
		pathValue,
		syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return nil, fail("CREDENTIAL_NOT_CERTIFIED")
	}
	file := os.NewFile(uintptr(descriptor), "sworn-native-credential")
	if file == nil {
		_ = syscall.Close(descriptor)
		return nil, fail("CREDENTIAL_NOT_CERTIFIED")
	}
	ok := false
	defer func() {
		if !ok {
			_ = file.Close()
		}
	}()
	var retained syscall.Stat_t
	if syscall.Fstat(descriptor, &retained) != nil ||
		!safeCredentialStat(retained, maximum) ||
		syscall.Flock(descriptor, syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return nil, fail("CREDENTIAL_NOT_CERTIFIED")
	}
	var live syscall.Stat_t
	if syscall.Lstat(pathValue, &live) != nil ||
		live.Dev != retained.Dev || live.Ino != retained.Ino ||
		!safeCredentialStat(live, maximum) {
		_ = syscall.Flock(descriptor, syscall.LOCK_UN)
		return nil, fail("CREDENTIAL_NOT_CERTIFIED")
	}
	ok = true
	return &linuxCredentialLease{
		file: file, path: pathValue, before: retained, maximum: maximum,
	}, nil
}

func (lease *linuxCredentialLease) File() *os.File {
	if lease == nil || lease.closed {
		return nil
	}
	return lease.file
}

func (lease *linuxCredentialLease) Close() error {
	if lease == nil || lease.closed {
		return nil
	}
	lease.closed = true
	descriptor := int(lease.file.Fd())
	var retained, live syscall.Stat_t
	retainedOK := syscall.Fstat(descriptor, &retained) == nil &&
		retained.Dev == lease.before.Dev && retained.Ino == lease.before.Ino &&
		safeRetainedCredentialStat(retained, lease.maximum)
	liveOK := syscall.Lstat(lease.path, &live) == nil &&
		safeCredentialStat(live, lease.maximum)
	// Benign rotation: the retained descriptor is untouched and the live
	// path entry is a new inode on the same device with the full safe shape
	// (regular, 0600, owned, single link, bounded size). That is ordinary
	// host credential refresh racing a dispatch, never tampering: the
	// dispatch already consumed the retained inode, so the completed work
	// survives and the rotation is recorded loudly by the caller. Everything
	// else - retained drift, a missing or unsafe path entry, a device
	// change - keeps failing exactly as before.
	benign := retainedOK && liveOK &&
		live.Dev == retained.Dev && live.Ino != retained.Ino
	valid := retainedOK && liveOK &&
		live.Dev == retained.Dev && live.Ino == retained.Ino
	_ = syscall.Flock(descriptor, syscall.LOCK_UN)
	closeErr := lease.file.Close()
	lease.file = nil
	lease.path = ""
	if !valid && !benign {
		return fail("CREDENTIAL_IDENTITY_CHANGED")
	}
	if closeErr != nil {
		return fail("CREDENTIAL_IDENTITY_CHANGED")
	}
	lease.rotated = benign
	return nil
}

func (lease *linuxCredentialLease) benignRotation() bool {
	return lease != nil && lease.rotated
}

func safeCredentialStat(stat syscall.Stat_t, maximum int64) bool {
	return stat.Mode&syscall.S_IFMT == syscall.S_IFREG &&
		stat.Mode&0o777 == 0o600 &&
		stat.Uid == uint32(os.Getuid()) &&
		stat.Nlink == 1 &&
		stat.Size > 0 && stat.Size <= maximum
}

// safeRetainedCredentialStat is the close-time check for the retained lease
// inode only. An atomic-rename rotation leaves the retained inode unlinked
// (nlink 0) - it exists only through the lease descriptor, the strongest
// retention state there is - while a hard link added after acquisition
// (nlink 2) remains a failing tampering signal. Acquisition and the live
// path entry keep the strict nlink == 1 shape.
func safeRetainedCredentialStat(stat syscall.Stat_t, maximum int64) bool {
	return stat.Mode&syscall.S_IFMT == syscall.S_IFREG &&
		stat.Mode&0o777 == 0o600 &&
		stat.Uid == uint32(os.Getuid()) &&
		(stat.Nlink == 1 || stat.Nlink == 0) &&
		stat.Size > 0 && stat.Size <= maximum
}
