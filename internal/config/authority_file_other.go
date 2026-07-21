//go:build !linux

package config

import (
	"errors"
	"os"
)

func validateAuthorityPlatform() error {
	return errors.New("production authority bundles require Linux single-link file validation")
}

func openBundleFile(root *os.Root, name string) (*os.File, error) {
	return root.Open(name)
}

func requireSingleLink(os.FileInfo) error {
	return errors.New("single-link authority bundles are unsupported on this platform")
}
