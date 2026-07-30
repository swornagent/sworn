//go:build !linux

package driver

import "context"

func readToolPath(string, string) ([]byte, error) {
	return nil, fail("UNSUPPORTED_HOST")
}

func writeToolPath(string, string, []byte) error {
	return fail("UNSUPPORTED_HOST")
}

func editToolPath(string, string, []byte, []byte) error {
	return fail("UNSUPPORTED_HOST")
}

func listToolDirectory(string, string) ([]toolPathEntry, error) {
	return nil, fail("UNSUPPORTED_HOST")
}

func scanToolText(string, string) ([]toolPathEntry, error) {
	return nil, fail("UNSUPPORTED_HOST")
}

func clearToolEntries(entries []toolPathEntry) {
	for index := range entries {
		clearBytes(entries[index].Body)
	}
}

func runToolBash(context.Context, Invocation, string, string) ([]byte, error) {
	return nil, fail("UNSUPPORTED_HOST")
}
