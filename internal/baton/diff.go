package baton

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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

// Diff compares the committed embed against the materialised pinned source.
// It uses the same content policy as Vendor for every source file in the
// batonFileMappings and compares those exact bytes with the destination.
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

		desired, err := mappedContent(opts.SourceDir, m)
		if err != nil {
			return nil, err
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

		if !bytes.Equal(existing, desired) {
			divs = append(divs, Divergence{
				File:   m.Dest,
				Reason: "content differs from mapped source",
			})
		}
	}

	return divs, nil
}
