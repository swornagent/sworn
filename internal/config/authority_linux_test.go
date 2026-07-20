//go:build linux

package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestAuthorityBundleResolverRejectsUnsafeLinuxFileIdentities(t *testing.T) {
	directory := t.TempDir()
	authority, err := OpenAuthority(testAuthorityConfiguration(directory), &discardAuthorityLedger{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = authority.Close() })
	filename, err := authorityBundleFilename(testAuthorityPlanDigest)
	if err != nil {
		t.Fatal(err)
	}
	bundlePath := filepath.Join(directory, filename)

	tests := map[string]struct {
		prepare func(*testing.T)
		want    string
	}{
		"symlink": {
			prepare: func(t *testing.T) {
				target := filepath.Join(directory, "target.json")
				if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Base(target), bundlePath); err != nil {
					t.Fatal(err)
				}
			},
			want: "non-symlink regular file",
		},
		"directory": {
			prepare: func(t *testing.T) {
				if err := os.Mkdir(bundlePath, 0o700); err != nil {
					t.Fatal(err)
				}
			},
			want: "non-symlink regular file",
		},
		"FIFO": {
			prepare: func(t *testing.T) {
				if err := syscall.Mkfifo(bundlePath, 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "non-symlink regular file",
		},
		"empty": {
			prepare: func(t *testing.T) {
				if err := os.WriteFile(bundlePath, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: "empty or exceeds",
		},
		"oversized": {
			prepare: func(t *testing.T) {
				file, err := os.Create(bundlePath)
				if err != nil {
					t.Fatal(err)
				}
				if err := file.Truncate(maximumAuthorityBundleBytes + 1); err != nil {
					_ = file.Close()
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
			},
			want: "empty or exceeds",
		},
		"multiple links": {
			prepare: func(t *testing.T) {
				target := filepath.Join(directory, "linked-target.json")
				if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(target, bundlePath); err != nil {
					t.Fatal(err)
				}
			},
			want: "want exactly one",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			for _, path := range []string{bundlePath, filepath.Join(directory, "target.json"), filepath.Join(directory, "linked-target.json")} {
				if err := os.RemoveAll(path); err != nil {
					t.Fatal(err)
				}
			}
			test.prepare(t)
			if _, _, err := authority.resolver.Resolve(context.Background(), testAuthoritySourceRef, testAuthorityPlanDigest); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("Resolve error = %v, want %q", err, test.want)
			}
		})
	}
}
