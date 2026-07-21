package app

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/swornagent/sworn/internal/adapter"
	configservice "github.com/swornagent/sworn/internal/config"
	"github.com/swornagent/sworn/internal/control"
	"github.com/swornagent/sworn/internal/effects"
	"github.com/swornagent/sworn/internal/engine"
	"github.com/swornagent/sworn/internal/executor"
	"github.com/swornagent/sworn/internal/repo"
	"github.com/swornagent/sworn/internal/store"
)

const (
	RunResultSchemaVersion        = "sworn-run-result-v1"
	maximumMaterializationEntries = 100_000
)

// Request selects one existing active delivery and, optionally, its one
// current work item. Run never creates or activates a delivery.
type Request struct {
	ConfigPath string
	RunID      string
	WorkID     string
}

// Result is the stable, secret-free monitoring projection of one bounded
// convergence. Command entries are absent when work was already reviewable.
type Result struct {
	SchemaVersion  string           `json:"schema_version"`
	RunID          string           `json:"run_id"`
	WorkID         string           `json:"work_id"`
	State          engine.WorkState `json:"state"`
	Revision       int64            `json:"revision"`
	BuildEffectID  string           `json:"build_effect_id,omitempty"`
	CheckEffectIDs []string         `json:"check_effect_ids,omitempty"`
	Build          *CommandResult   `json:"build,omitempty"`
	Checks         *CommandResult   `json:"checks,omitempty"`
	Admission      *CommandResult   `json:"admission,omitempty"`
	Recovery       RecoveryResult   `json:"recovery"`
}

type CommandResult struct {
	CommandID string `json:"command_id"`
	Revision  int64  `json:"revision"`
	Replayed  bool   `json:"replayed"`
}

type RecoveryResult struct {
	Interrupted   int `json:"interrupted"`
	Bound         int `json:"bound"`
	Retried       int `json:"builder_retried"`
	ChecksRetried int `json:"checks_retried"`
}

// Validate proves that one monitoring result is an internally complete
// projection of either an already-reviewable item or the three commands which
// converged it during this invocation.
func (result Result) Validate() error {
	if result.SchemaVersion != RunResultSchemaVersion || !engine.ValidID(result.RunID) ||
		!engine.ValidID(result.WorkID) || result.State != engine.WorkReviewable || result.Revision < 0 {
		return errors.New("invalid bounded run result identity or state")
	}
	if result.Recovery.Interrupted < 0 || result.Recovery.Bound < 0 || result.Recovery.Retried < 0 ||
		result.Recovery.ChecksRetried < 0 {
		return errors.New("bounded run result has a negative recovery count")
	}

	if result.BuildEffectID == "" {
		if len(result.CheckEffectIDs) != 0 || result.Build != nil || result.Checks != nil || result.Admission != nil {
			return errors.New("already-reviewable result carries command or effect identities")
		}
		return nil
	}
	if !engine.ValidID(result.BuildEffectID) || len(result.CheckEffectIDs) == 0 ||
		result.Build == nil || result.Checks == nil || result.Admission == nil {
		return errors.New("executed bounded run result is incomplete")
	}

	seen := make(map[string]struct{}, len(result.CheckEffectIDs)+4)
	addIdentity := func(value, label string) error {
		if !engine.ValidID(value) {
			return fmt.Errorf("bounded run result has an invalid %s", label)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("bounded run result duplicates identity %q", value)
		}
		seen[value] = struct{}{}
		return nil
	}
	if err := addIdentity(result.BuildEffectID, "build effect id"); err != nil {
		return err
	}
	for _, effectID := range result.CheckEffectIDs {
		if err := addIdentity(effectID, "check effect id"); err != nil {
			return err
		}
	}

	commands := []struct {
		label  string
		result *CommandResult
		want   int64
	}{
		{"build command id", result.Build, result.Revision - 2},
		{"check command id", result.Checks, result.Revision - 1},
		{"admission command id", result.Admission, result.Revision},
	}
	if result.Revision < 2 {
		return errors.New("executed bounded run result cannot contain three contiguous revisions")
	}
	for _, command := range commands {
		if err := addIdentity(command.result.CommandID, command.label); err != nil {
			return err
		}
		if command.result.Revision != command.want {
			return errors.New("bounded run command revisions are not contiguous with final state")
		}
	}
	return nil
}

// Run composes the sole production path and advances exactly one current work
// item to reviewable. It does not poll, schedule, initialize, activate, verify,
// or advance a second item.
func Run(ctx context.Context, request Request) (result Result, resultErr error) {
	if ctx == nil {
		return Result{}, errors.New("sworn run requires a context")
	}
	if err := request.validate(); err != nil {
		return Result{}, err
	}
	configuration, err := LoadConfig(request.ConfigPath)
	if err != nil {
		return Result{}, err
	}
	if err := requireExistingControlDatabase(configuration.ControlDatabase); err != nil {
		return Result{}, err
	}
	if err := requireExactDirectory(configuration.Repository.Root, "repository root", false); err != nil {
		return Result{}, err
	}
	repository, err := repo.Open(ctx, configuration.Repository.Root, configuration.Repository.Binding)
	if err != nil {
		return Result{}, fmt.Errorf("open exact repository: %w", err)
	}
	if repository.Root() != configuration.Repository.Root {
		return Result{}, errors.New("configured repository root does not name the exact Git worktree root")
	}

	privateRoots := []struct{ label, path string }{
		{"executor runtime root", configuration.Executor.RuntimeRoot},
		{"executor writable root", configuration.Executor.WritableRoot},
		{"builder workspace root", configuration.Workspaces.BuilderRoot},
		{"check workspace root", configuration.Workspaces.CheckRoot},
	}
	for _, selected := range privateRoots {
		if err := requireExactDirectory(selected.path, selected.label, true); err != nil {
			return Result{}, err
		}
	}
	if err := requireExactDirectory(configuration.ContentRuntime.Source, "content runtime source", false); err != nil {
		return Result{}, err
	}
	for _, selected := range []struct{ label, path string }{
		{"Bubblewrap executable", configuration.Executor.Bubblewrap},
		{"systemd-run executable", configuration.Executor.SystemdRun},
		{"systemctl executable", configuration.Executor.Systemctl},
	} {
		if err := requireExactExecutable(selected.path, selected.label); err != nil {
			return Result{}, err
		}
	}
	limits, err := configuration.executorLimits()
	if err != nil {
		return Result{}, err
	}
	runner, err := executor.NewLinux(executor.Options{
		RuntimeRoot:         configuration.Executor.RuntimeRoot,
		WritableRoot:        configuration.Executor.WritableRoot,
		BubblewrapPath:      configuration.Executor.Bubblewrap,
		SystemdRunPath:      configuration.Executor.SystemdRun,
		SystemctlPath:       configuration.Executor.Systemctl,
		Limits:              limits,
		CredentialFile:      configuration.Codex.ChatGPTAuthFile,
		AllowCredentialFile: true,
		AllowHostNetwork:    true,
		AllowNestedSandbox:  true,
	})
	if err != nil {
		return Result{}, fmt.Errorf("configure contained executor: %w", err)
	}
	runtime, err := executor.NewRuntimeTree(
		configuration.ContentRuntime.Source,
		configuration.ContentRuntime.Digest,
		configuration.ContentRuntime.MaximumBytes,
	)
	if err != nil {
		return Result{}, fmt.Errorf("configure content runtime: %w", err)
	}

	baseBuilder := effects.BuilderWorker{
		Runner: runner, Repository: repository,
		WorkspaceRoot: configuration.Workspaces.BuilderRoot,
	}
	builderWorker, err := adapter.NewCodexBuilder(ctx, baseBuilder, adapter.CodexBuilderOptions{
		BinaryPath: configuration.Codex.Binary,
		Model:      configuration.Codex.Model,
		Timeout:    mustSecondsDuration(configuration.Codex.TimeoutSeconds),
	})
	if err != nil {
		return Result{}, fmt.Errorf("configure pinned Codex builder: %w", err)
	}
	builderDispatchDigest, err := builderWorker.DispatchDigest()
	if err != nil {
		return Result{}, fmt.Errorf("bind builder dispatch profile: %w", err)
	}

	journal, err := store.OpenConfigured(ctx, configuration.ControlDatabase, store.ControlConfiguration{
		LocalCheckRuntimeManifestDigest: runtime.Digest(),
		BuilderDispatchDigest:           builderDispatchDigest,
		Repository:                      repository,
	})
	if err != nil {
		return Result{}, fmt.Errorf("open configured control store: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, journal.Close()) }()
	state, err := journal.State(ctx, request.RunID)
	if err != nil {
		return Result{}, fmt.Errorf("load selected delivery: %w", err)
	}
	if state.Repository != repository.Binding().RepositoryID {
		return Result{}, errors.New("selected delivery does not use the configured repository identity")
	}
	workID, err := selectCurrentWork(state, request.WorkID)
	if err != nil {
		return Result{}, err
	}

	builderWorker.Control = journal
	builder, err := control.NewBuilderService(journal, builderWorker)
	if err != nil {
		return Result{}, err
	}
	checks, err := control.NewCheckService(journal, effects.LocalCheckWorker{
		Control: journal, Runner: runner, Repository: repository, Runtime: runtime,
		WorkspaceRoot: configuration.Workspaces.CheckRoot,
		MaterializeLimits: repo.MaterializeLimits{
			Bytes: limits.WorkspaceBytes, Entries: maximumMaterializationEntries,
		},
	})
	if err != nil {
		return Result{}, err
	}
	authoritySources, err := configuration.authoritySources()
	if err != nil {
		return Result{}, err
	}
	authority, err := configservice.OpenAuthority(authoritySources, journal)
	if err != nil {
		return Result{}, fmt.Errorf("open configured authority: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, authority.Close()) }()

	ownerID := configuration.OwnerID
	if ownerID == "" {
		ownerID = deterministicOwnerID(configuration, request.RunID)
	}
	controller, recovery, err := control.StartController(
		ctx, ownerID, journal, authority.Service(), builder, checks,
	)
	if err != nil {
		return Result{}, fmt.Errorf("start recovered controller: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, controller.Close()) }()

	advanced, err := controller.AdvanceToReviewable(ctx, request.RunID, workID)
	if err != nil {
		return Result{}, fmt.Errorf("advance selected work to reviewable: %w", err)
	}
	return projectResult(request.RunID, workID, recovery, advanced)
}

func (request Request) validate() error {
	if err := validateCleanAbsolutePath(request.ConfigPath, "run config"); err != nil {
		return err
	}
	if !engine.ValidID(request.RunID) {
		return errors.New("sworn run requires a valid run id")
	}
	if request.WorkID != "" && !engine.ValidID(request.WorkID) {
		return errors.New("sworn run work id is invalid")
	}
	return nil
}

func selectCurrentWork(state engine.State, requested string) (string, error) {
	if err := state.Validate(); err != nil {
		return "", fmt.Errorf("validate selected delivery: %w", err)
	}
	if state.Phase != engine.PhaseActive {
		return "", errors.New("sworn run requires an active delivery")
	}
	for _, work := range state.Work {
		if requested != "" && work.ID != requested {
			continue
		}
		if work.State == engine.WorkWaiting {
			if requested != "" {
				return "", fmt.Errorf("work %q is waiting, not the current work item", requested)
			}
			continue
		}
		return work.ID, nil
	}
	if requested != "" {
		return "", fmt.Errorf("work %q is absent from the current delivery", requested)
	}
	return "", errors.New("active delivery has no current work item")
}

func projectResult(
	runID string,
	workID string,
	recovery control.RecoveryReport,
	advanced control.ReviewableResult,
) (Result, error) {
	var selected *engine.Work
	for index := range advanced.State.Work {
		if advanced.State.Work[index].ID == workID {
			selected = &advanced.State.Work[index]
			break
		}
	}
	if selected == nil || selected.State != engine.WorkReviewable {
		return Result{}, errors.New("bounded delivery result lacks selected reviewable work")
	}
	return Result{
		SchemaVersion: RunResultSchemaVersion,
		RunID:         runID, WorkID: workID, State: selected.State, Revision: advanced.State.Revision,
		BuildEffectID:  advanced.BuildEffectID,
		CheckEffectIDs: slices.Clone(advanced.CheckEffectIDs),
		Build:          commandResult(advanced.Build), Checks: commandResult(advanced.Checks),
		Admission: commandResult(advanced.Admission),
		Recovery: RecoveryResult{
			Interrupted: recovery.Interrupted, Bound: recovery.Bound,
			Retried: recovery.Retried, ChecksRetried: recovery.ChecksRetried,
		},
	}, nil
}

func commandResult(result store.ApplyResult) *CommandResult {
	if result.CommandID == "" {
		return nil
	}
	return &CommandResult{CommandID: result.CommandID, Revision: result.Revision, Replayed: result.Replayed}
}

func deterministicOwnerID(configuration Config, runID string) string {
	hasher := sha256.New()
	bind := func(value string) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write([]byte(value))
	}
	bind("sworn-controller-owner-v1")
	bind(configuration.ControlDatabase)
	bind(configuration.Repository.Binding.RepositoryID)
	bind(runID)
	return "controller-" + hex.EncodeToString(hasher.Sum(nil))
}

func requireExistingControlDatabase(path string) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve existing control database: %w", err)
	}
	if resolved != path {
		return errors.New("control database path contains a symbolic-link remap")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect existing control database: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("control database must be an existing non-symlink regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("control database must be private to its owner")
	}
	return nil
}

func requireExactDirectory(path, label string, private bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s must be an existing non-symlink directory", label)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return fmt.Errorf("%s contains a symbolic-link remap", label)
	}
	if private && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s must be private to its owner", label)
	}
	return nil
}

func requireExactExecutable(path, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%s must be an existing non-symlink executable regular file", label)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return fmt.Errorf("%s contains a symbolic-link remap", label)
	}
	return nil
}

func mustSecondsDuration(seconds uint64) time.Duration {
	// Config validation has already proved this conversion exact.
	return time.Duration(seconds) * time.Second
}
