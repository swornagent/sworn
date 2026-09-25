package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/swornagent/sworn/internal/driver"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

func runManifest(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: sworn manifest canonical (--manifest ABS | --driver-config ABS) [--write]")
		return 2
	}
	verb := args[0]
	rest := args[1:]
	if verb != "canonical" {
		fmt.Fprintf(stderr, "sworn manifest: unknown verb %q\n", verb)
		return 2
	}
	return runManifestCanonical(rest, stdout, stderr)
}

func runManifestCanonical(args []string, stdout, stderr io.Writer) int {
	const usageLine = "usage: sworn manifest canonical (--manifest ABS | --driver-config ABS) [--write]"
	var manifestPath, driverConfigPath string
	var write bool
	seenManifest, seenDriver, seenWrite := false, false, false
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--manifest":
			if seenManifest || index+1 >= len(args) {
				fmt.Fprintln(stderr, usageLine)
				return 2
			}
			index++
			value := args[index]
			if value == "" || strings.HasPrefix(value, "--") || !filepath.IsAbs(value) || filepath.Clean(value) != value || strings.ContainsRune(value, 0) {
				fmt.Fprintln(stderr, usageLine)
				return 2
			}
			manifestPath = value
			seenManifest = true
		case "--driver-config":
			if seenDriver || index+1 >= len(args) {
				fmt.Fprintln(stderr, usageLine)
				return 2
			}
			index++
			value := args[index]
			if value == "" || strings.HasPrefix(value, "--") || !filepath.IsAbs(value) || filepath.Clean(value) != value || strings.ContainsRune(value, 0) {
				fmt.Fprintln(stderr, usageLine)
				return 2
			}
			driverConfigPath = value
			seenDriver = true
		case "--write":
			if seenWrite {
				fmt.Fprintln(stderr, usageLine)
				return 2
			}
			seenWrite = true
			write = true
		default:
			fmt.Fprintln(stderr, usageLine)
			return 2
		}
	}
	if (manifestPath == "") == (driverConfigPath == "") {
		fmt.Fprintln(stderr, usageLine)
		return 2
	}
	if manifestPath != "" {
		body, err := readManifest(manifestPath)
		if err != nil {
			writeKnownFailure(stderr, "manifest canonical", "Could not read the manifest. Check that --manifest points to an absolute regular file.", "")
			return 1
		}
		canonical, err := runtimepkg.CanonicalManifestBytes(body)
		if err != nil {
			writeCommandFailure(stderr, "manifest canonical", "The manifest is invalid.", err)
			return 1
		}
		if write {
			if err := writeFileAtomic(manifestPath, canonical, runtimepkg.MaxManifestBytes); err != nil {
				writeKnownFailure(stderr, "manifest canonical", "Could not write the manifest. Check that --manifest points to an absolute regular file.", "")
				return 1
			}
			fmt.Fprintln(stderr, "wrote canonical manifest (--manifest)")
			return 0
		}
		if _, err := stdout.Write(canonical); err != nil {
			fmt.Fprintln(stderr, "sworn manifest canonical: output failed")
			return 1
		}
		fmt.Fprintln(stderr, "printed canonical manifest to stdout (--manifest)")
		return 0
	}
	body, err := readDriverConfigBytes(driverConfigPath)
	if err != nil {
		writeKnownFailure(stderr, "manifest canonical", "Could not read the driver config. Check that --driver-config points to an absolute regular file.", "")
		return 1
	}
	canonical, err := driver.CanonicalDriverConfigBytes(body)
	if err != nil {
		writeCommandFailure(stderr, "manifest canonical", "The driver config is invalid.", err)
		return 1
	}
	if write {
		if err := writeFileAtomic(driverConfigPath, canonical, driver.MaxDriverConfigBytes); err != nil {
			writeKnownFailure(stderr, "manifest canonical", "Could not write the driver config. Check that --driver-config points to an absolute regular file.", "")
			return 1
		}
		fmt.Fprintln(stderr, "wrote canonical driver config (--driver-config)")
		return 0
	}
	if _, err := stdout.Write(canonical); err != nil {
		fmt.Fprintln(stderr, "sworn manifest canonical: output failed")
		return 1
	}
	fmt.Fprintln(stderr, "printed canonical driver config to stdout (--driver-config)")
	return 0
}

func readDriverConfigBytes(path string) ([]byte, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, errors.New("driver config path must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() ||
		info.Mode()&os.ModeSymlink != 0 ||
		info.Size() < 1 || info.Size() > driver.MaxDriverConfigBytes {
		return nil, errors.New("driver config is not an admitted regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		return nil, errors.New("driver config changed while it was admitted")
	}
	body, err := io.ReadAll(io.LimitReader(file, driver.MaxDriverConfigBytes+1))
	if err != nil || len(body) > driver.MaxDriverConfigBytes {
		return nil, errors.New("driver config exceeds the limit")
	}
	return body, nil
}

// writeFileAtomic replaces the regular file at abs with body atomically via a
// sibling temporary file in the same directory followed by rename. It refuses
// a missing, non-regular, or symlinked target and a body larger than maxBytes,
// preserves the target's existing permission bits, and never creates parent
// directories.
func writeFileAtomic(abs string, body []byte, maxBytes int) error {
	if abs == "" || !filepath.IsAbs(abs) || filepath.Clean(abs) != abs || strings.ContainsRune(abs, 0) {
		return errors.New("path must be an absolute clean path")
	}
	if len(body) > maxBytes {
		return errors.New("content exceeds the limit")
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("target is not an admitted regular file")
	}
	perm := info.Mode().Perm()
	dir := filepath.Dir(abs)
	temp, err := os.CreateTemp(dir, ".sworn-*-tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := temp.Write(body); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Chmod(perm); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, abs); err != nil {
		return err
	}
	success = true
	return nil
}
