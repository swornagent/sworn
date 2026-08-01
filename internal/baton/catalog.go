package baton

import "github.com/swornagent/sworn/internal/gitx"

const releaseRefPrefix = "refs/heads/release-wt/"

// ReleaseRef is one canonical local Baton release authority.
type ReleaseRef struct {
	Release string
	Ref     string
	Head    string
}

// ListReleaseRefs returns every canonical local release authority in release
// name order. A malformed or non-commit ref fails the complete projection.
func ListReleaseRefs(gitRepository GitRepository) ([]ReleaseRef, error) {
	repository := gitRepository.repository()
	if repository == nil {
		return nil, recordFail(
			"INVALID_REPOSITORY",
			"one admitted Git repository is required",
		)
	}
	refs, err := repository.ListHeadRefsUnder(releaseRefPrefix)
	if err != nil {
		return nil, translateGitError("list release refs", err)
	}
	result := make([]ReleaseRef, 0, len(refs))
	for _, ref := range refs {
		release := ref.Ref[len(releaseRefPrefix):]
		validated, err := identity(release, "release")
		if err != nil || releaseRef(validated) != ref.Ref {
			return nil, recordWrap(
				"INVALID_RELEASE_REF",
				"release authority has an invalid name",
				err,
			)
		}
		if ref.State != gitx.RefDirect || ref.Head.IsZero() || ref.Target != "" {
			return nil, recordFail(
				"INVALID_HEAD_OBJECT",
				"release authority is not one direct commit",
			)
		}
		result = append(result, ReleaseRef{
			Release: validated,
			Ref:     ref.Ref,
			Head:    ref.Head.String(),
		})
	}
	return result, nil
}
