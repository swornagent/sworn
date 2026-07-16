package baton

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/swornagent/sworn/internal/adopt"
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

	inputs, pinned, err := PinnedInstallerVendorInputs(context.Background(), opts.SourceDir, time.Unix(1, 0).UTC())
	if err != nil {
		return nil, err
	}
	if pinned {
		archivePath := filepath.Join(opts.RepoRoot, filepath.FromSlash(installerArchivePath))
		repositoryArchive, readErr := os.ReadFile(archivePath)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				divs = append(divs, Divergence{File: installerArchivePath, Reason: "file missing from embed — not vendored"})
			} else {
				return nil, fmt.Errorf("baton: cannot read installer archive %s: %w", installerArchivePath, readErr)
			}
		} else {
			if _, validateErr := ValidateInstallerArchive(repositoryArchive); validateErr != nil {
				return nil, fmt.Errorf("baton: malformed installer archive: %w", validateErr)
			}
			if !bytes.Equal(repositoryArchive, inputs.Archive) {
				divs = append(divs, Divergence{File: installerArchivePath, Reason: "archive differs from pinned source"})
			}
		}
		embeddedArchive := adopt.BatonInstallerArchive()
		if _, validateErr := ValidateInstallerArchive(embeddedArchive); validateErr != nil {
			return nil, fmt.Errorf("baton: malformed compiled installer archive: %w", validateErr)
		}
		if !bytes.Equal(embeddedArchive, inputs.Archive) {
			divs = append(divs, Divergence{File: installerArchivePath, Reason: "compiled archive differs from pinned source"})
		}

		versionPath := filepath.Join(opts.RepoRoot, filepath.FromSlash(upstreamVersionPath))
		repositoryVersion, readErr := os.ReadFile(versionPath)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				divs = append(divs, Divergence{File: upstreamVersionPath, Reason: "adopting manifest is missing"})
			} else {
				return nil, fmt.Errorf("baton: cannot read adopting manifest %s: %w", upstreamVersionPath, readErr)
			}
		} else {
			wantPin := UpstreamPin{Tag: inputs.Version.Tag, SHA: inputs.Version.SHA, Digest: inputs.Version.Digest}
			if gotPin := parseUpstreamPin(string(repositoryVersion)); gotPin != wantPin {
				divs = append(divs, Divergence{File: upstreamVersionPath, Reason: "adopting manifest pin differs from pinned source"})
			}
			embeddedVersion, embeddedErr := adopt.BatonDocsFS().ReadFile("baton/VERSION")
			if embeddedErr != nil {
				return nil, fmt.Errorf("baton: cannot read compiled adopting manifest: %w", embeddedErr)
			}
			if !bytes.Equal(repositoryVersion, embeddedVersion) {
				divs = append(divs, Divergence{File: upstreamVersionPath, Reason: "committed adopting manifest differs from compiled binary"})
			}
		}
	}

	for _, skew := range SchemaSkew() {
		divs = append(divs, Divergence{File: "internal/baton/schemas/embed.go", Reason: skew})
	}

	sort.Slice(divs, func(i, j int) bool {
		if divs[i].File == divs[j].File {
			return divs[i].Reason < divs[j].Reason
		}
		return divs[i].File < divs[j].File
	})

	return divs, nil
}
