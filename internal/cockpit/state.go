package cockpit

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	"github.com/swornagent/sworn/internal/protocol"
)

type GitStateReader struct {
	executable string
}

func NewGitStateReader(gitExecutable string) (*GitStateReader, error) {
	if !filepath.IsAbs(gitExecutable) ||
		filepath.Clean(gitExecutable) != gitExecutable {
		return nil, fail("INVALID_GIT_EXECUTABLE")
	}
	return &GitStateReader{executable: gitExecutable}, nil
}

func (r *GitStateReader) Read(
	ctx context.Context,
	run journal.Run,
) (protocol.State, error) {
	if r == nil || ctx == nil || run.Repository == "" || run.Release == "" {
		return protocol.State{}, errors.New("Protocol state is unavailable")
	}
	repository, err := gitx.Open(run.Repository, r.executable)
	if err != nil || repository.Root() != run.Repository {
		return protocol.State{}, errors.New("Protocol state is unavailable")
	}
	inert := func(
		request gitx.RecordRootRequest,
	) (gitx.RecordRootDecision, error) {
		return gitx.RecordRootDecision{
			Kind:       request.Kind,
			Repository: request.Repository,
			RecordRoot: request.RecordRoot,
			Commit:     request.Commit,
			Decision:   "inert",
		}, nil
	}
	return protocol.ReadState(
		protocol.UseGitRepository(repository),
		run.Release,
		inert,
	)
}
