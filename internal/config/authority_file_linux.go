//go:build linux

package config

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func validateAuthorityPlatform() error { return nil }

func openBundleFile(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}

func requireSingleLink(info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("file identity lacks a Linux link count")
	}
	if stat.Nlink != 1 {
		return fmt.Errorf("file has %d links; want exactly one", stat.Nlink)
	}
	return nil
}
