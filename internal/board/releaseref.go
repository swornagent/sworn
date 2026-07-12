package board

import (
	"fmt"
	"strings"
)

// ReleaseRefReader is the minimal git surface ReleaseRefFor needs.
type ReleaseRefReader interface {
	RefExists(ref string) (bool, error)
	CatFileExists(ref, path string) (bool, error)
}

// releaseRecords is the ordered list of records whose presence marks a
// release's docs directory as real on a ref.
//
// board.json is the authoritative record (ADR-0009: records-as-JSON). index.md
// is a rendered view — a board.json-native release need not carry one at all,
// so probing index.md alone misses every records-as-JSON release. It stays here
// second, for releases planned before the migration.
var releaseRecords = []string{"board.json", "index.md"}

// ReleaseWTRef returns the conventional release-worktree branch ref for a
// release.
func ReleaseWTRef(release string) string {
	return "refs/heads/release-wt/" + release
}

// ReleaseRefFor resolves the git ref that carries a release's board records.
//
// It prefers refs/heads/release-wt/<release>, falling back to HEAD when no
// release worktree branch has been materialised — the release is already merged
// to the integration branch, or was planned before its worktree was cut.
//
// It fails closed rather than silently retargeting. When the release-wt branch
// exists but carries no board record under any docs prefix, that is an error
// naming that ref. The previous behaviour quietly reassigned the ref to HEAD and
// then reported the failure against HEAD, which sent operators looking for a
// release on the integration branch that was sitting on release-wt the whole
// time.
func ReleaseRefFor(reader ReleaseRefReader, release string) (string, error) {
	ref := ReleaseWTRef(release)

	branchExists, err := reader.RefExists(ref)
	if err != nil {
		return "", fmt.Errorf("probe ref %s: %w", ref, err)
	}
	if !branchExists {
		return "HEAD", nil
	}

	for _, prefix := range docsPrefixes {
		for _, record := range releaseRecords {
			path := prefix + "/" + release + "/" + record
			exists, err := reader.CatFileExists(ref, path)
			if err != nil {
				return "", fmt.Errorf("probe %s:%s: %w", ref, path, err)
			}
			if exists {
				return ref, nil
			}
		}
	}

	return "", fmt.Errorf(
		"release %q: branch %s exists but carries no board record "+
			"(probed %s under %s) — the release worktree is out of sync with its plan",
		release, ref,
		strings.Join(releaseRecords, " / "),
		strings.Join(docsPrefixes, ", "),
	)
}
