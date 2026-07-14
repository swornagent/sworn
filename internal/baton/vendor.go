package baton

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// VendorOpts controls the vendor behaviour.
type VendorOpts struct {
	// SourceDir is the path to a Baton checkout (e.g. ~/projects/baton).
	SourceDir string

	// RepoRoot is the path to the SwornAgent repository root. Dest paths
	// in the file mapping are resolved relative to this root.
	RepoRoot string

	// CheckOnly enables dry-run mode: diff is computed and returned but
	// no files are written.
	CheckOnly bool
}

// VendorResult carries the outcome of a Vendor run.
type VendorResult struct {
	// Diff contains the unified diff of all changes (empty if no changes).
	Diff string

	// FilesWritten is the number of embed files that were written (0 in
	// CheckOnly mode).
	FilesWritten int
}

// Vendor orchestrates the vendor-down pipeline:
//  1. Validate the source directory has all expected files.
//  2. Materialise each mapping through the shared content policy, then write it.
//  3. Returns the diff (for display) and the count of files written.
//
// In CheckOnly mode, the diff is computed against the current tree but no
// files are modified.
func Vendor(opts VendorOpts) (*VendorResult, error) {
	if err := ValidateSource(opts.SourceDir); err != nil {
		return nil, err
	}

	var diffs []string
	filesWritten := 0

	for _, m := range batonFileMappings {
		destAbs := filepath.Join(opts.RepoRoot, m.Dest)

		// Ensure the destination directory exists (skip in CheckOnly).
		if !opts.CheckOnly {
			destDir := filepath.Dir(destAbs)
			if err := os.MkdirAll(destDir, 0755); err != nil {
				return nil, fmt.Errorf("baton: cannot create dest dir %s: %w", destDir, err)
			}
		}

		desired, err := mappedContent(opts.SourceDir, m)
		if err != nil {
			return nil, err
		}

		// Compute diff for this file.
		existing, _ := os.ReadFile(destAbs)
		if !bytes.Equal(existing, desired) {
			diff := unifiedDiff(m.Dest, string(existing), string(desired))
			if diff != "" {
				diffs = append(diffs, diff)
			}

			if !opts.CheckOnly {
				if err := os.WriteFile(destAbs, desired, 0644); err != nil {
					return nil, fmt.Errorf("baton: cannot write %s: %w", m.Dest, err)
				}
				filesWritten++
			}
		}
	}

	return &VendorResult{
		Diff:         strings.Join(diffs, ""),
		FilesWritten: filesWritten,
	}, nil
}

// unifiedDiff produces a minimal unified-style diff between old and new for a
// single file. Returns an empty string if they are identical.
func unifiedDiff(path, old, new string) string {
	if old == new {
		return ""
	}
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(new, "\n")

	// Remove trailing empty string from split if content ends with newline.
	if len(oldLines) > 0 && oldLines[len(oldLines)-1] == "" {
		oldLines = oldLines[:len(oldLines)-1]
	}
	if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
		newLines = newLines[:len(newLines)-1]
	}

	// Simple diff: find common prefix and suffix, then show changes.
	prefix := commonPrefixLen(oldLines, newLines)
	suffix := commonSuffixLen(oldLines, newLines, prefix)

	oldStart := prefix + 1
	oldCount := len(oldLines) - prefix - suffix
	newStart := prefix + 1
	newCount := len(newLines) - prefix - suffix

	if oldCount == 0 && newCount == 0 {
		return ""
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "--- a/%s\n", path)
	fmt.Fprintf(&buf, "+++ b/%s\n", path)
	fmt.Fprintf(&buf, "@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount)

	for _, l := range oldLines[prefix : len(oldLines)-suffix] {
		fmt.Fprintf(&buf, "-%s\n", l)
	}
	for _, l := range newLines[prefix : len(newLines)-suffix] {
		fmt.Fprintf(&buf, "+%s\n", l)
	}

	return buf.String()
}

func commonPrefixLen(a, b []string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func commonSuffixLen(a, b []string, prefix int) int {
	la, lb := len(a), len(b)
	n := la - prefix
	if lb-prefix < n {
		n = lb - prefix
	}
	for i := 0; i < n; i++ {
		if a[la-1-i] != b[lb-1-i] {
			return i
		}
	}
	return n
}
