package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// MigrationMarkerSubject is the exact commit subject the operator-gated
// reserved-records migration writes, so the one-time relocation is visible
// and attributable in the git log.
const MigrationMarkerSubject = "sworn(records): migrate reserved records root from .baton/releases to .sworn/records"

// RecordsMigration describes one exact operator-gated relocation of the
// reserved records root from the historical .baton/releases location to the
// configured .sworn/records root.
type RecordsMigration struct {
	// Releases are the release identities whose plan records were relocated.
	Releases []string
	// Commit is the migration commit built on the current branch head.
	Commit OID
	// ReleaseRefs maps each migrated release ref to its new head commit.
	ReleaseRefs map[string]OID
}

// MigrateRecordsRequest is the operator-gated request for the one-time
// reserved-records relocation. Confirmed must be set explicitly; the
// migration refuses to run as a silent side effect of anything else.
type MigrateRecordsRequest struct {
	Confirmed bool
	Identity  Identity
}

type legacyPlan struct {
	release string
	path    string
}

// MigrateLegacyRecords relocates every recorded plan from the historical
// .baton/releases root to the configured .sworn/records root. It is an
// explicit operator-gated engine pathway, never a silent side effect of
// ordinary model-directed work: it refuses a dirty tree or index, requires
// Confirmed, refuses when there is nothing to migrate, refuses to overwrite
// an already-relocated record, and refuses a second run (idempotent). The
// migration commits are built by host-side engine code and are not model
// candidates, so they never enter the seal or scope gates.
func (r *Repository) MigrateLegacyRecords(request MigrateRecordsRequest) (RecordsMigration, error) {
	if !request.Confirmed {
		return RecordsMigration{}, fail(
			"CONFIRMATION_REQUIRED",
			"migrate reserved records",
			errors.New("migration must be explicitly confirmed by the operator"),
		)
	}
	if err := ValidateIdentity(request.Identity); err != nil {
		return RecordsMigration{}, err
	}
	raw, err := r.run(nil, nil, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return RecordsMigration{}, err
	}
	if len(bytes.TrimSpace(raw)) != 0 {
		return RecordsMigration{}, fail(
			"DIRTY_WORKTREE",
			"migrate reserved records",
			errors.New("working tree or index is not clean"),
		)
	}
	branchBytes, err := r.run(nil, nil, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		return RecordsMigration{}, fail(
			"INVALID_HEAD_REF",
			"migrate reserved records",
			errors.New("HEAD is not a symbolic branch ref"),
		)
	}
	headRef := strings.TrimSpace(string(branchBytes))
	if err := ValidateHeadRef(headRef); err != nil {
		return RecordsMigration{}, err
	}
	headBytes, err := r.run(nil, nil, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return RecordsMigration{}, err
	}
	head, err := r.parseOID(strings.TrimSpace(string(headBytes)))
	if err != nil {
		return RecordsMigration{}, err
	}

	entries, err := r.ListTree(head)
	if err != nil {
		return RecordsMigration{}, err
	}
	prefix := LegacyRecordsRoot + "/"
	var legacy []legacyPlan
	seen := make(map[string]bool)
	for _, entry := range entries {
		if entry.Type != "blob" || !strings.HasPrefix(entry.Path, prefix) ||
			!strings.HasSuffix(entry.Path, "/plan.md") {
			continue
		}
		release := strings.TrimSuffix(strings.TrimPrefix(entry.Path, prefix), "/plan.md")
		if release == "" || strings.Contains(release, "/") || seen[release] {
			return RecordsMigration{}, fail(
				"MALFORMED_LEGACY_RECORD",
				"migrate reserved records",
				fmt.Errorf("unexpected legacy record path %s", entry.Path),
			)
		}
		seen[release] = true
		legacy = append(legacy, legacyPlan{release: release, path: entry.Path})
	}
	if len(legacy) == 0 {
		return RecordsMigration{}, fail(
			"NOTHING_TO_MIGRATE",
			"migrate reserved records",
			errors.New("no records remain under .baton/releases"),
		)
	}
	sort.Slice(legacy, func(i, j int) bool { return legacy[i].release < legacy[j].release })

	// Refuse to overwrite an already-relocated record.
	for _, plan := range legacy {
		present, err := r.pathPresentAt(head, recordPlanPath(DefaultRecordsRoot, plan.release))
		if err != nil {
			return RecordsMigration{}, err
		}
		if present {
			return RecordsMigration{}, fail(
				"RECORD_ALREADY_MIGRATED",
				"migrate reserved records",
				fmt.Errorf("%s already exists at the configured records root", plan.release),
			)
		}
	}

	// Read every legacy plan from the exact current head.
	planBytesByRelease := make(map[string][]byte, len(legacy))
	bodyMap, err := r.ReadBlobs(head, pathsOfLegacyPlans(legacy))
	if err != nil {
		return RecordsMigration{}, err
	}
	for _, plan := range legacy {
		planBytesByRelease[plan.release] = bodyMap[plan.path]
	}

	// Build the current-branch migration commit: remove the whole legacy
	// tree and publish every relocated plan under the configured root.
	headCommit, err := r.buildRecordsMigrationCommit(
		head, request.Identity, legacy, planBytesByRelease,
	)
	if err != nil {
		return RecordsMigration{}, err
	}

	// Build one migration commit per migrated release ref, so every release
	// ref head carries its own plan under the configured root.
	releaseRefs := make(map[string]OID, len(legacy))
	refSnapshots := []RefHead{{Ref: headRef, State: RefDirect, Head: head}}
	operations := []RefOperation{{
		Kind: UpdateRef, Ref: headRef, NewHead: &headCommit, Expected: &head,
	}}
	for _, plan := range legacy {
		ref := "refs/heads/release-wt/" + plan.release
		captured, err := r.CaptureHeadRefs([]string{ref})
		if err != nil {
			return RecordsMigration{}, err
		}
		if len(captured) != 1 || captured[0].State != RefDirect {
			// A legacy plan with no direct release ref is left in place; the
			// fallback keeps it readable and the migration reports the refs
			// it did move.
			continue
		}
		one := []legacyPlan{{release: plan.release, path: plan.path}}
		commit, err := r.buildRecordsMigrationCommit(
			captured[0].Head, request.Identity, one,
			map[string][]byte{plan.release: planBytesByRelease[plan.release]},
		)
		if err != nil {
			return RecordsMigration{}, err
		}
		releaseRefs[ref] = commit
		refSnapshots = append(refSnapshots, RefHead{Ref: ref, State: RefDirect, Head: captured[0].Head})
		operations = append(operations, RefOperation{
			Kind: UpdateRef, Ref: ref, NewHead: &commit, Expected: &captured[0].Head,
		})
	}
	sort.Slice(refSnapshots, func(i, j int) bool { return refSnapshots[i].Ref < refSnapshots[j].Ref })
	sort.Slice(operations, func(i, j int) bool { return operations[i].Ref < operations[j].Ref })
	if err := r.ApplyRefTransaction(refSnapshots, operations); err != nil {
		return RecordsMigration{}, err
	}

	// The migration commit changed the tree (relocated records added, legacy
	// tree removed). The operator was left with a clean tree, so resync the
	// index and working tree to the new HEAD exactly: this materializes the
	// relocated records and removes the now-untracked legacy files, leaving a
	// clean tree.
	if _, err := r.run(nil, nil, "reset", "--hard", "HEAD"); err != nil {
		return RecordsMigration{}, fail(
			"WORKTREE_CLEANUP_FAILED",
			"migrate reserved records",
			errors.New("could not resync the working tree to the migrated head"),
		)
	}

	releases := make([]string, 0, len(legacy))
	for _, plan := range legacy {
		releases = append(releases, plan.release)
	}
	return RecordsMigration{
		Releases:    releases,
		Commit:      headCommit,
		ReleaseRefs: releaseRefs,
	}, nil
}

func pathsOfLegacyPlans(plans []legacyPlan) []string {
	result := make([]string, len(plans))
	for index, plan := range plans {
		result[index] = plan.path
	}
	return result
}

func recordPlanPath(recordRoot, release string) string {
	return recordRoot + "/" + release + "/plan.md"
}

// buildRecordsMigrationCommit builds one migration commit on parent: every
// .baton/** path present at parent is removed and the given relocated plans
// are written under the configured records root. The commit carries the
// fixed marker subject and the explicit engine identity.
func (r *Repository) buildRecordsMigrationCommit(
	parent OID,
	identity Identity,
	plans []legacyPlan,
	planBytes map[string][]byte,
) (OID, error) {
	entries, err := r.ListTree(parent)
	if err != nil {
		return OID{}, err
	}
	changes := make([]BlobChange, 0, len(entries)+len(plans))
	legacyPrefix := LegacyRecordsRoot + "/"
	for _, entry := range entries {
		if entry.Path == LegacyRecordsRoot || strings.HasPrefix(entry.Path, legacyPrefix) {
			changes = append(changes, BlobChange{Path: entry.Path, Delete: true})
		}
	}
	for _, plan := range plans {
		body := planBytes[plan.release]
		if len(body) == 0 {
			return OID{}, fail(
				"MALFORMED_LEGACY_RECORD",
				"migrate reserved records",
				fmt.Errorf("no record bytes for %s", plan.release),
			)
		}
		changes = append(changes, BlobChange{
			Path: recordPlanPath(DefaultRecordsRoot, plan.release), Bytes: append([]byte(nil), body...),
		})
	}
	timestamp, err := r.CommitTimestamp(parent)
	if err != nil {
		return OID{}, err
	}
	prepared, err := r.prepareRecord(RecordRequest{
		Parent: parent, Changes: changes,
		Message:   MigrationMarkerSubject + "\n",
		Identity:  identity,
		Timestamp: timestamp + 1,
	})
	if err != nil {
		return OID{}, err
	}
	return prepared.Commit, nil
}
