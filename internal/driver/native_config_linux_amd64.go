//go:build linux && amd64

package driver

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	linuxAMD64MemfdCreate = 319
	memfdCloseOnExec      = 0x0001
)

func unlinkedConfigFile(body []byte) (*os.File, error) {
	name := []byte("sworn-native-config\x00")
	descriptor, _, errno := syscall.Syscall(
		linuxAMD64MemfdCreate,
		uintptr(unsafe.Pointer(&name[0])),
		memfdCloseOnExec,
		0,
	)
	if errno != 0 {
		return nil, fail("NATIVE_NOT_CERTIFIED")
	}
	file := os.NewFile(descriptor, "sworn-native-config")
	if file == nil {
		_ = syscall.Close(int(descriptor))
		return nil, fail("NATIVE_NOT_CERTIFIED")
	}
	if file.Chmod(0o600) != nil {
		_ = file.Close()
		return nil, fail("NATIVE_NOT_CERTIFIED")
	}
	if written, err := file.Write(body); err != nil || written != len(body) {
		_ = file.Close()
		return nil, fail("NATIVE_NOT_CERTIFIED")
	}
	if _, err := file.Seek(0, 0); err != nil {
		_ = file.Close()
		return nil, fail("NATIVE_NOT_CERTIFIED")
	}
	return file, nil
}
