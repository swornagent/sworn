package baton

import (
	"fmt"
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/gitx"
)

func TestListReleaseRefsReturnsValidatedOrderWithoutMutation(t *testing.T) {
	t.Parallel()

	repoPath := createActionRepository(t, "sha1")
	repository, err := gitx.Open(repoPath, actionTestGit)
	if err != nil {
		t.Fatal(err)
	}
	head := actionGit(t, repoPath, nil, nil, "rev-parse", "HEAD")
	for _, release := range []string{"zeta", "alpha"} {
		actionGit(t, repoPath, nil, nil, "update-ref", releaseRef(release), head)
	}
	beforeStatus := actionGit(t, repoPath, nil, nil, "status", "--porcelain=v2", "--branch")
	beforeRefs := actionGit(t, repoPath, nil, nil, "for-each-ref", "--format=%(refname)%09%(objectname)")

	got, err := ListReleaseRefs(UseGitRepository(repository))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 ||
		got[0] != (ReleaseRef{Release: "alpha", Ref: releaseRef("alpha"), Head: head}) ||
		got[1] != (ReleaseRef{Release: "zeta", Ref: releaseRef("zeta"), Head: head}) {
		t.Fatalf("release refs = %#v", got)
	}
	afterStatus := actionGit(t, repoPath, nil, nil, "status", "--porcelain=v2", "--branch")
	afterRefs := actionGit(t, repoPath, nil, nil, "for-each-ref", "--format=%(refname)%09%(objectname)")
	if beforeStatus != afterStatus || beforeRefs != afterRefs {
		t.Fatalf("release discovery mutated repository:\nstatus %q -> %q\nrefs %q -> %q", beforeStatus, afterStatus, beforeRefs, afterRefs)
	}
}

func TestListReleaseRefsRejectsInvalidAndNonCommitAuthority(t *testing.T) {
	t.Run("invalid release name", func(t *testing.T) {
		t.Parallel()
		repoPath := createActionRepository(t, "sha1")
		repository, err := gitx.Open(repoPath, actionTestGit)
		if err != nil {
			t.Fatal(err)
		}
		head := actionGit(t, repoPath, nil, nil, "rev-parse", "HEAD")
		actionGit(t, repoPath, nil, nil, "update-ref", releaseRef("invalid/name"), head)
		if _, err := ListReleaseRefs(UseGitRepository(repository)); ErrorCode(err) != "INVALID_RELEASE_REF" {
			t.Fatalf("invalid release ref error = %v", err)
		}
	})

	t.Run("symbolic release authority", func(t *testing.T) {
		t.Parallel()
		repoPath := createActionRepository(t, "sha1")
		repository, err := gitx.Open(repoPath, actionTestGit)
		if err != nil {
			t.Fatal(err)
		}
		actionGit(t, repoPath, nil, nil, "symbolic-ref", releaseRef("symbolic"), "refs/heads/main")
		if _, err := ListReleaseRefs(UseGitRepository(repository)); ErrorCode(err) != "INVALID_HEAD_OBJECT" {
			t.Fatalf("symbolic release ref error = %v", err)
		}
	})
}

func TestListReleaseRefsPreservesResourceBound(t *testing.T) {
	t.Parallel()

	repoPath := createActionRepository(t, "sha1")
	repository, err := gitx.Open(repoPath, actionTestGit)
	if err != nil {
		t.Fatal(err)
	}
	head := actionGit(t, repoPath, nil, nil, "rev-parse", "HEAD")
	// Build the refs independently so Git remains the authority for ref-name
	// admission while the production enumeration applies its fixed bound.
	var updates strings.Builder
	for index := 0; index <= gitx.MaxHeadRefs; index++ {
		name := fmt.Sprintf("release-%03d", index)
		fmt.Fprintf(&updates, "create %s %s\n", releaseRef(name), head)
	}
	actionGit(t, repoPath, []byte(updates.String()), nil, "update-ref", "--stdin")
	if _, err := ListReleaseRefs(UseGitRepository(repository)); ErrorCode(err) != "RESOURCE_LIMIT" {
		t.Fatalf("oversized release catalog error = %v", err)
	}
}
