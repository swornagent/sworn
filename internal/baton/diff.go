package baton

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DiffOpts controls the diff behaviour.
type DiffOpts struct {
	// SourceDir is the path to a Baton checkout (e.g. ~/projects/baton).
	SourceDir string

	// RepoRoot is the path to the SwornAgent repository root. The committed
	// embed files are read from this root, and compared against the transformed
	// pinned source.
	RepoRoot string
}

// Divergence names one file in the committed embed that differs from the
// transformed pinned source.
type Divergence struct {
	// File is the embed-relative path (e.g. internal/adopt/baton/rules/01-reachability-gate.md).
	File string

	// Reason is a short descriptor of the divergence.
	Reason string
}

// Diff compares the committed embed against the transformed pinned source.
// It re-applies the same Transform as Vendor to every source file in the
// batonFileMappings and compares the result against the committed destination.
// An empty returned slice means the embed is in sync.
//
// Diff never writes files. ValidateSource is called first; a missing source
// file returns an error.
func Diff(opts DiffOpts) ([]Divergence, error) {
	if err := ValidateSource(opts.SourceDir); err != nil {
		return nil, err
	}

	var divs []Divergence

	for _, m := range batonFileMappings {
		destAbs := filepath.Join(opts.RepoRoot, m.Dest)

		var transformed string

		if m.Source == "claude/baton/rules.md" {
			// Concatenate all individual rules into a single document,
			// identically to Vendor.
			var buf bytes.Buffer
			for _, ruleSrc := range RuleSources() {
				srcPath := filepath.Join(opts.SourceDir, ruleSrc)
				content, err := os.ReadFile(srcPath)
				if err != nil {
					return nil, fmt.Errorf("baton: cannot read rule %s: %w", ruleSrc, err)
				}
				t, err := Transform(string(content))
				if err != nil {
					return nil, fmt.Errorf("baton: transform rule %s: %w", ruleSrc, err)
				}
				buf.WriteString(strings.TrimSpace(t))
				buf.WriteString("\n\n")
			}
			transformed = strings.TrimRight(buf.String(), "\n") + "\n"
		} else {
			srcPath := filepath.Join(opts.SourceDir, m.Source)
			content, err := os.ReadFile(srcPath)
			if err != nil {
				return nil, fmt.Errorf("baton: cannot read %s: %w", m.Source, err)
			}
			t, err := Transform(string(content))
			if err != nil {
				return nil, fmt.Errorf("baton: transform %s: %w", m.Source, err)
			}
			transformed = t
		}

		// Compare against the committed embed on disk.
		existing, err := os.ReadFile(destAbs)
		if err != nil {
			if os.IsNotExist(err) {
				divs = append(divs, Divergence{
					File:   m.Dest,
					Reason: "file missing from embed — not vendored",
				})
				continue
			}
			return nil, fmt.Errorf("baton: cannot read embed %s: %w", m.Dest, err)
		}

		if string(existing) != transformed {
			divs = append(divs, Divergence{
				File:   m.Dest,
				Reason: "content differs from transformed source",
			})
		}
	}

	return divs, nil
}