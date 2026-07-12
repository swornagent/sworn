package board

import (
	"strings"
	"testing"
)

// TestReleaseRefFor_BoardJSONOnlyResolvesToReleaseWT is the regression guard.
//
// The defect: the release-ref probe looked only for index.md. A board.json-native
// release (ADR-0009, records-as-JSON) carries board.json and no index.md, so the
// probe missed, the ref was silently reassigned to HEAD, and the caller reported
// "path does not exist in HEAD" for a board that was sitting on release-wt.
//
// This is the shape the defect actually takes in the wild: the Fumadocs prefix,
// board.json present, index.md absent.
func TestReleaseRefFor_BoardJSONOnlyResolvesToReleaseWT(t *testing.T) {
	const release = "2026-07-11-design-system"
	ref := ReleaseWTRef(release)

	for _, prefix := range docsPrefixes {
		t.Run(prefix, func(t *testing.T) {
			r := newFakeReader()
			r.setRef(ref)
			// board.json only — deliberately NO index.md.
			r.setContent(ref, prefix+"/"+release+"/board.json", `{"tracks":[]}`)

			got, err := ReleaseRefFor(r, release)
			if err != nil {
				t.Fatalf("ReleaseRefFor: unexpected error: %v", err)
			}
			if got != ref {
				t.Errorf("ReleaseRefFor = %q, want %q\n"+
					"a board.json-native release must resolve to its release-wt ref, "+
					"not fall back to HEAD", got, ref)
			}
		})
	}
}

// TestReleaseRefFor_LegacyIndexMDStillResolves keeps the pre-ADR-0009 releases
// working: index.md alone is still a valid marker.
func TestReleaseRefFor_LegacyIndexMDStillResolves(t *testing.T) {
	const release = "2026-06-19-safe-parallelism"
	ref := ReleaseWTRef(release)

	r := newFakeReader()
	r.setRef(ref)
	r.setContent(ref, "docs/release/"+release+"/index.md", "# release\n")

	got, err := ReleaseRefFor(r, release)
	if err != nil {
		t.Fatalf("ReleaseRefFor: unexpected error: %v", err)
	}
	if got != ref {
		t.Errorf("ReleaseRefFor = %q, want %q", got, ref)
	}
}

// TestReleaseRefFor_NoReleaseWTBranchFallsBackToHEAD covers the legitimate
// fallback: the release has no worktree branch, so it lives on the integration
// branch.
func TestReleaseRefFor_NoReleaseWTBranchFallsBackToHEAD(t *testing.T) {
	const release = "2026-05-01-already-merged"

	r := newFakeReader() // no refs registered → RefExists is false
	r.setContent("HEAD", "docs/release/"+release+"/board.json", `{"tracks":[]}`)

	got, err := ReleaseRefFor(r, release)
	if err != nil {
		t.Fatalf("ReleaseRefFor: unexpected error: %v", err)
	}
	if got != "HEAD" {
		t.Errorf("ReleaseRefFor = %q, want %q when no release-wt branch exists", got, "HEAD")
	}
}

// TestReleaseRefFor_BranchWithoutRecordsFailsClosed is the fail-closed half.
// A release-wt branch that carries no board record is a real skew: it must be an
// error naming that ref, never a quiet retarget to HEAD.
func TestReleaseRefFor_BranchWithoutRecordsFailsClosed(t *testing.T) {
	const release = "2026-07-11-design-system"
	ref := ReleaseWTRef(release)

	r := newFakeReader()
	r.setRef(ref) // branch exists, but no board.json and no index.md on it

	got, err := ReleaseRefFor(r, release)
	if err == nil {
		t.Fatalf("ReleaseRefFor = %q, nil error; want an error — a release-wt branch "+
			"with no board record must fail closed, not fall back to HEAD", got)
	}
	if !strings.Contains(err.Error(), ref) {
		t.Errorf("error %q does not name the ref it failed on (%q); "+
			"the misleading-error bug was exactly this", err, ref)
	}
}
