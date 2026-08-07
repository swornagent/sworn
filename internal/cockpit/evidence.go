package cockpit

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/swornagent/sworn/internal/baton"
)

const (
	// evidenceRoot mirrors the existing project-local, non-Git convention
	// .sworn/runs and .sworn/sworn.db already use: ordinary local filesystem
	// state, never a Git tree, candidate, or Baton record.
	evidenceRoot               = ".sworn/evidence"
	maxEvidenceBundlesPerSlice = 64
)

// BoundEvidenceItem is the read-only inventory of one evidence item whose
// bundle resolved cleanly against the current state: metadata only, never
// item content. It carries no lifecycle authority; its presence or absence
// never affects status, controls, or eligibility.
type BoundEvidenceItem struct {
	Bundle string `json:"bundle"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Digest string `json:"digest"`
	Bytes  int64  `json:"bytes"`
}

func evidenceDir(root, release, sliceID string) string {
	return filepath.Join(root, evidenceRoot, release, sliceID)
}

// DiscoverBoundEvidence lists every well-formed sworn.evidence-bundle/v1
// manifest under the bounded, project-local directory
// root/.sworn/evidence/<release>/<slice>/, resolves each against state, and
// returns the item inventory of every bundle that resolves cleanly. A
// missing or empty directory is normal and returns no items and no error. A
// bundle that is malformed, or whose binding or any item's declared digest
// does not match its real bytes, is silently excluded: evidence is
// advisory, human-reviewable material, and its absence or rejection never
// fails discovery. Item content is read only to prove its digest against
// the bundle's declaration; it is never returned, logged, or sent anywhere.
func DiscoverBoundEvidence(root string, state baton.State, sliceID string) ([]BoundEvidenceItem, error) {
	if root == "" || sliceID == "" {
		return nil, nil
	}
	dir := evidenceDir(root, state.Release, sliceID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fail("EVIDENCE_DIRECTORY_UNAVAILABLE")
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type().IsRegular() && filepath.Ext(entry.Name()) == ".json" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if len(names) > maxEvidenceBundlesPerSlice {
		names = names[:maxEvidenceBundlesPerSlice]
	}
	var result []BoundEvidenceItem
	for _, name := range names {
		items, err := resolveEvidenceBundleFile(dir, name, state)
		if err != nil {
			continue
		}
		result = append(result, items...)
	}
	return result, nil
}

// ReadBoundEvidence explicitly reads and resolves the one bundle file named
// bundle under root/.sworn/evidence/<release>/<slice>/, returning its full
// item inventory or the exact rejection reason. Unlike DiscoverBoundEvidence,
// which silently excludes a bundle that fails to resolve, this fails closed
// with the real error so a caller naming one exact bundle learns why it was
// rejected. It never mutates any lifecycle record.
func ReadBoundEvidence(root string, state baton.State, sliceID, bundle string) ([]BoundEvidenceItem, error) {
	if root == "" || sliceID == "" || bundle == "" {
		return nil, fail("INVALID_REQUEST")
	}
	return resolveEvidenceBundleFile(evidenceDir(root, state.Release, sliceID), bundle, state)
}

func resolveEvidenceBundleFile(dir, name string, state baton.State) ([]BoundEvidenceItem, error) {
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil, fail("EVIDENCE_BUNDLE_NOT_FOUND")
	}
	bundle, err := baton.ParseEvidenceBundle(raw)
	if err != nil {
		return nil, err
	}
	declared := bundle.Items()
	items := make(map[string][]byte, len(declared))
	for _, item := range declared {
		body, readErr := os.ReadFile(filepath.Join(dir, item.Path))
		if readErr != nil {
			return nil, fail("EVIDENCE_ITEM_UNAVAILABLE")
		}
		items[item.Path] = body
	}
	if _, err := bundle.Resolve(state, items); err != nil {
		return nil, err
	}
	result := make([]BoundEvidenceItem, 0, len(declared))
	for _, item := range declared {
		result = append(result, BoundEvidenceItem{
			Bundle: name, Path: item.Path, Kind: item.Kind,
			Digest: item.Digest, Bytes: item.Bytes,
		})
	}
	return result, nil
}
