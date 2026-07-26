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
	valid := syscall.Fstat(descriptor, &retained) == nil &&
		syscall.Lstat(lease.path, &live) == nil &&
		retained.Dev == lease.before.Dev && retained.Ino == lease.before.Ino &&
		live.Dev == retained.Dev && live.Ino == retained.Ino &&
		safeCredentialStat(retained, lease.maximum) &&
		safeCredentialStat(live, lease.maximum)
	_ = syscall.Flock(descriptor, syscall.LOCK_UN)
	closeErr := lease.file.Close()
	lease.file = nil
	lease.path = ""
	if !valid || closeErr != nil {
		return fail("CREDENTIAL_IDENTITY_CHANGED")
	}
	return nil
}

func safeCredentialStat(stat syscall.Stat_t, maximum int64) bool {
	return stat.Mode&syscall.S_IFMT == syscall.S_IFREG &&
		stat.Mode&0o777 == 0o600 &&
		stat.Uid == uint32(os.Getuid()) &&
		stat.Nlink == 1 &&
		stat.Size > 0 && stat.Size <= maximum
}
