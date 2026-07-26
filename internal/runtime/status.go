package runtime

import (
	"context"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
)

func (s *Service) Status(ctx context.Context, runID string) (RunStatus, error) {
	if s == nil || s.journal == nil || ctx == nil ||
		!runtimeIdentityPattern.MatchString(runID) {
		return RunStatus{}, runtimeFail("INVALID_RUN", nil)
	}
	snapshot, err := s.journal.Snapshot(ctx, runID)
	if err != nil {
		return RunStatus{}, runtimeFail("RUN_NOT_FOUND", err)
	}
	result := RunStatus{
		SchemaVersion: "sworn.run-status/v1",
		RunID:         runID, State: "new",
		ManifestDigest: snapshot.Run.ManifestDigest,
		TargetRef:      snapshot.Run.TargetRef,
		Effects:        make([]EffectStatus, 0, len(snapshot.Effects)),
	}
	hasPlan := false
	complete := false
	operationalFailure := false
	uncertain := false
	for _, command := range snapshot.Commands {
		if command.ReplayKey == "plan-proposal" {
			hasPlan = true
			plan, parseErr := baton.ParsePlan(command.Payload)
			if parseErr != nil {
				return RunStatus{}, runtimeFail("CORRUPT_JOURNAL", nil)
			}
			result.PlanDigest = plan.Digest()
		}
	}
	for _, effect := range snapshot.Effects {
		result.Effects = append(result.Effects, EffectStatus{
			ID: effect.ID, Kind: effect.Kind,
			State: string(effect.State), ErrorCode: effect.ErrorCode,
		})
		switch effect.State {
		case journal.Uncertain, journal.Claimed:
			uncertain = true
		case journal.OperationalFailed:
			operationalFailure = true
		}
		if effect.ID == "baton.merge" && effect.State == journal.Succeeded {
			complete = true
		}
	}
	if len(snapshot.Events) != 0 {
		result.EventOffset = snapshot.Events[len(snapshot.Events)-1].Offset
	}
	switch {
	case uncertain:
		result.State = "uncertain"
	case operationalFailure:
		result.State = "operational_failed"
	case complete:
		result.State = "complete"
	case hasPlan:
		result.State = "awaiting_approval"
	}
	manifest, _, loadErr := s.loadRun(ctx, runID)
	if loadErr == nil {
		repository, openErr := gitx.Open(manifest.value.Repository, s.gitExecutable)
		if openErr == nil {
			inertness := func(request gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
				return gitx.RecordRootDecision{
					Kind: request.Kind, Repository: request.Repository,
					RecordRoot: request.RecordRoot, Commit: request.Commit,
					Decision: "inert",
				}, nil
			}
			state, stateErr := baton.ReadState(
				baton.UseGitRepository(repository),
				manifest.value.Release,
				inertness,
			)
			if stateErr == nil {
				result.TargetHead = state.Refs.Target.Head
				result.ReleaseHead = state.Refs.Release.Head
				result.Outcome = state.Assembly.Outcome
				if !complete && !uncertain && !operationalFailure {
					result.State = "running"
				}
			}
		}
	}
	return result, nil
}
