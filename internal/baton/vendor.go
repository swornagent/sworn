package baton

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const upstreamVersionPath = "internal/adopt/baton/VERSION"

// VendorOpts controls the vendor behaviour.
type VendorOpts struct {
	// SourceDir is the path to a Baton checkout (e.g. ~/projects/baton).
	SourceDir string

	// RepoRoot is the path to the SwornAgent repository root. Dest paths
	// in the file mapping are resolved relative to this root.
	RepoRoot string

	// CheckOnly enables dry-run mode: drift is computed and returned but
	// no files are written.
	CheckOnly bool

	// VersionCandidate, when non-nil, adds VERSION to the same immutable
	// materialisation, snapshot, apply, rollback, and recovery plan as every
	// mapped destination. The public upstream command supplies this for both
	// check and write mode; local vendoring leaves it nil.
	VersionCandidate *UpstreamVersionCandidate

	// InstallerArchiveCandidate, when non-nil, adds the validated complete
	// offline installer input to the same repository transaction as mapped bytes
	// and VERSION. A nil value preserves the destination exactly.
	InstallerArchiveCandidate []byte

	// fileOps is an injected, per-invocation filesystem seam used by the
	// transaction fault matrix. Production callers leave it nil.
	fileOps *vendorFileOps
}

// VendorResult carries the outcome of a Vendor run.
type VendorResult struct {
	// Diff is a deterministic path-only drift summary (empty if no changes).
	// It deliberately never contains mapped source payload bytes.
	Diff string

	// FilesWritten is the number of transaction members replaced (0 in
	// CheckOnly mode).
	FilesWritten int
}

type vendorCandidate struct {
	path        string
	desired     []byte
	desiredMode os.FileMode
	original    vendorOriginal
	preserve    bool
}

type vendorOriginal struct {
	exists         bool
	mode           os.FileMode
	bytes          []byte
	missingParents []string
}

type vendorPlan struct {
	candidates []vendorCandidate
	changed    []int
	diff       string
	fileOps    *vendorFileOps
}

// Vendor materialises the complete candidate set before the first repository
// mutation. Check and write mode share this exact plan; write mode alone submits
// it to the rollback-protected transaction.
func Vendor(opts VendorOpts) (*VendorResult, error) {
	var repo vendorRepository
	if opts.CheckOnly {
		// Check mode must not create or inspect Git-admin transaction state.
		// It needs only the physically resolved repository root to run the
		// same source/materialisation/schema/destination preflight.
		var err error
		repo, err = resolveVendorRepositoryRoot(opts.RepoRoot)
		if err != nil {
			return nil, err
		}
	} else {
		var recovery *vendorRecovery
		var err error
		repo, recovery, err = loadVendorRecovery(opts.RepoRoot)
		if err != nil {
			return nil, err
		}
		if recovery != nil {
			if err := recoverVendorTransaction(repo, recovery); err != nil {
				return nil, err
			}
			return nil, newVendorErrorWithDetail("recovered-rerun-required", "recovery", "", recovery.root, "original repository state restored; rerun vendor command", fmt.Errorf("original repository state restored; re-run the vendor command"))
		}
	}

	plan, err := materialiseVendorPlan(opts, repo)
	if err != nil {
		return nil, err
	}
	result := &VendorResult{Diff: plan.diff}
	if opts.CheckOnly || len(plan.changed) == 0 {
		return result, nil
	}

	if err := applyVendorTransaction(repo, plan); err != nil {
		return nil, err
	}
	result.FilesWritten = len(plan.changed)
	return result, nil
}

// RecoverVendorIfPending restores or finishes cleanup for a previously
// published write transaction. It never performs ordinary vendoring. A
// successful recovery therefore returns a rerun-required error so callers do
// not combine restart handling with a new network fetch or repository update.
func RecoverVendorIfPending(repoRoot string) error {
	repo, recovery, err := loadVendorRecovery(repoRoot)
	if err != nil {
		return err
	}
	if recovery == nil {
		return nil
	}
	if err := recoverVendorTransaction(repo, recovery); err != nil {
		return err
	}
	return newVendorErrorWithDetail("recovered-rerun-required", "recovery", "", recovery.root, "repository recovery completed; rerun vendor command", fmt.Errorf("repository transaction recovery completed; re-run the vendor command"))
}

func materialiseVendorPlan(opts VendorOpts, repo vendorRepository) (*vendorPlan, error) {
	if err := ValidateSource(opts.SourceDir); err != nil {
		return nil, newVendorError("invalid-source", "preflight", "", "", err)
	}

	materialised := make(map[string]vendorCandidate, len(batonFileMappings)+2)
	seen := make(map[string]struct{}, len(batonFileMappings)+2)
	for _, mapping := range batonFileMappings {
		if err := validateVendorRelativePath(mapping.Dest); err != nil {
			return nil, newVendorError("invalid-mapping", "preflight", mapping.Dest, "", err)
		}
		if _, duplicate := seen[mapping.Dest]; duplicate {
			return nil, newVendorError("invalid-mapping", "preflight", mapping.Dest, "", fmt.Errorf("duplicate destination"))
		}
		seen[mapping.Dest] = struct{}{}

		desired, err := mappedContent(opts.SourceDir, mapping)
		if err != nil {
			var scriptErr *scriptReferenceError
			if errors.As(err, &scriptErr) {
				detail := fmt.Sprintf("unknown script reference %q", scriptErr.token)
				return nil, newVendorErrorWithDetail("materialisation-failed", "preflight", mapping.Dest, "", detail, err)
			}
			return nil, newVendorError("materialisation-failed", "preflight", mapping.Dest, "", err)
		}
		if isSchemaSource(mapping.Source) {
			name := strings.TrimSuffix(filepath.Base(mapping.Source), ".json")
			if _, err := compileSchemaBytes(name, desired); err != nil {
				return nil, newVendorError("schema-invalid", "preflight", mapping.Dest, "", err)
			}
		}
		materialised[mapping.Dest] = vendorCandidate{
			path:        filepath.ToSlash(mapping.Dest),
			desired:     append([]byte(nil), desired...),
			desiredMode: 0o644,
		}
	}

	if _, duplicate := seen[upstreamVersionPath]; duplicate {
		return nil, newVendorError("invalid-mapping", "preflight", upstreamVersionPath, "", fmt.Errorf("VERSION duplicates a mapped destination"))
	}
	// VERSION is present in every complete transaction/recovery set so a
	// tampered upstream manifest cannot masquerade as a local transaction by
	// dropping the pin tuple. Local mode preserves its original state exactly;
	// only an explicit upstream candidate can make it a changed member.
	materialised[upstreamVersionPath] = vendorCandidate{
		path:        upstreamVersionPath,
		desiredMode: 0o644,
		preserve:    opts.VersionCandidate == nil,
	}
	if _, duplicate := seen[installerArchivePath]; duplicate || installerArchivePath == upstreamVersionPath {
		return nil, newVendorError("invalid-mapping", "preflight", installerArchivePath, "", fmt.Errorf("installer archive duplicates a vendor destination"))
	}
	archiveCandidate := vendorCandidate{
		path:        installerArchivePath,
		desiredMode: 0o644,
		preserve:    opts.InstallerArchiveCandidate == nil,
	}
	if opts.InstallerArchiveCandidate != nil {
		if _, err := ValidateInstallerArchive(opts.InstallerArchiveCandidate); err != nil {
			return nil, newVendorError("installer-archive-invalid", "preflight", installerArchivePath, "", err)
		}
		archiveCandidate.desired = append([]byte(nil), opts.InstallerArchiveCandidate...)
	}
	materialised[installerArchivePath] = archiveCandidate

	candidates := make([]vendorCandidate, 0, len(materialised))
	for _, candidate := range materialised {
		candidates = append(candidates, candidate)
	}
	sortVendorCandidatesBytewise(candidates)
	lastPath := ""
	for i, candidate := range candidates {
		if i > 0 && candidate.path <= lastPath {
			return nil, newVendorError("invalid-mapping", "preflight", candidate.path, "", fmt.Errorf("materialised destinations are not unique byte-sorted paths"))
		}
		lastPath = candidate.path
	}
	for i := range candidates {
		original, err := snapshotVendorOriginal(repo, candidates[i].path)
		if err != nil {
			return nil, err
		}
		candidates[i].original = original
		if candidates[i].path == upstreamVersionPath {
			if candidates[i].preserve {
				candidates[i].desired = append([]byte(nil), original.bytes...)
				candidates[i].desiredMode = original.mode
				continue
			}
			if !original.exists {
				return nil, newVendorError("version-missing", "preflight", candidates[i].path, "", fmt.Errorf("VERSION must exist before upstream vendoring"))
			}
			replacement, err := UpstreamPinReplacement(original.bytes, *opts.VersionCandidate)
			if err != nil {
				return nil, newVendorError("version-invalid", "preflight", candidates[i].path, "", err)
			}
			candidates[i].desired = replacement
		} else if candidates[i].path == installerArchivePath && candidates[i].preserve {
			candidates[i].desired = append([]byte(nil), original.bytes...)
			candidates[i].desiredMode = original.mode
		}
	}

	changed := make([]int, 0, len(candidates))
	var diff strings.Builder
	for i := range candidates {
		candidate := &candidates[i]
		if candidate.preserve {
			continue
		}
		if candidate.original.exists && bytes.Equal(candidate.original.bytes, candidate.desired) {
			continue
		}
		changed = append(changed, i)
		kind := "changed"
		if !candidate.original.exists {
			kind = "added"
		}
		fmt.Fprintf(&diff, "%s: %s\n", kind, candidate.path)
	}

	return &vendorPlan{
		candidates: candidates,
		changed:    changed,
		diff:       diff.String(),
		fileOps:    opts.fileOps,
	}, nil
}

// sortVendorCandidatesBytewise is an MSD byte-radix sort. Each path byte is
// inspected at most once per shared prefix level and the alphabet is fixed, so
// ordering remains O(total path bytes + file count) without a second
// hand-maintained mapping index or comparison-sort term.
func sortVendorCandidatesBytewise(candidates []vendorCandidate) {
	if len(candidates) < 2 {
		return
	}
	scratch := make([]vendorCandidate, len(candidates))
	radixSortVendorCandidates(candidates, scratch, 0, len(candidates), 0)
}

func radixSortVendorCandidates(candidates, scratch []vendorCandidate, start, end, depth int) {
	if end-start < 2 {
		return
	}
	var counts [257]int
	bucketFor := func(candidate vendorCandidate) int {
		if depth == len(candidate.path) {
			return 0
		}
		return int(candidate.path[depth]) + 1
	}
	for i := start; i < end; i++ {
		counts[bucketFor(candidates[i])]++
	}
	var starts [257]int
	position := start
	for bucket, count := range counts {
		starts[bucket] = position
		position += count
	}
	next := starts
	for i := start; i < end; i++ {
		bucket := bucketFor(candidates[i])
		scratch[next[bucket]] = candidates[i]
		next[bucket]++
	}
	copy(candidates[start:end], scratch[start:end])
	for bucket := 1; bucket < len(counts); bucket++ {
		bucketStart := starts[bucket]
		radixSortVendorCandidates(candidates, scratch, bucketStart, bucketStart+counts[bucket], depth+1)
	}
}
