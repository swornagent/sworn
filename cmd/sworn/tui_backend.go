package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
	"github.com/swornagent/sworn/internal/tui"
)

var (
	errRunBoardGit     = errors.New("Git is unavailable")
	errRunBoardJournal = errors.New("the saved run record is unavailable")
)

type tuiHostKey struct {
	journalPath string
	configPath  string
}

type projectTUIBackend struct {
	startPath   string
	journalPath string
	configPath  string
	manifestDir string
	commandID   func() (string, error)

	mu       sync.Mutex
	closed   bool
	inFlight sync.WaitGroup
	hosts    map[tuiHostKey]*tuiResidentHost
}

type tuiResidentHost struct {
	service  *runtimepkg.Service
	factory  *driver.ProductionDriverFactory
	store    *journal.Store
	commands *cockpit.CommandFacade

	// telemetry holds one silently-fail-open run-side export host per run
	// this resident host started (C3: sworn tui hosts delivery in-process
	// via StartDetached/StartWithCaptainDelegationDetached, so its runs must
	// export exactly as run's do). Teardown is bound to the resident host's
	// close path.
	telemetryMu sync.Mutex
	telemetry   []*runTelemetryHost
}

// attachTelemetry starts one run-side export host for a run this resident
// host just started. Construction is fail-open: a nil or disabled host is a
// no-op, and the run's behavior is untouched either way.
func (h *tuiResidentHost) attachTelemetry(
	ctx context.Context,
	journalPath, runID, operatorConfigPath string,
) {
	if h == nil {
		return
	}
	telemetryHost := newRunTelemetryHost(
		ctx,
		journalPath,
		runID,
		h.service,
		operatorConfigPath,
	)
	if telemetryHost == nil {
		return
	}
	h.telemetryMu.Lock()
	if h.telemetry != nil {
		h.telemetry = append(h.telemetry, telemetryHost)
	}
	h.telemetryMu.Unlock()
}

func (h *tuiResidentHost) close() error {
	if h == nil {
		return nil
	}
	// Finish telemetry hosts before closing the service and store they
	// project from; their teardown is bounded and never fails the close.
	h.telemetryMu.Lock()
	telemetryHosts := h.telemetry
	h.telemetry = nil
	h.telemetryMu.Unlock()
	for _, telemetryHost := range telemetryHosts {
		if telemetryHost != nil {
			telemetryHost.finish()
		}
	}
	var errs []error
	if h.service != nil {
		errs = append(errs, h.service.Close())
	}
	if h.store != nil {
		errs = append(errs, h.store.Close())
	}
	if h.factory != nil {
		errs = append(errs, h.factory.Close())
	}
	return errors.Join(errs...)
}

func newProjectTUIBackend(
	startPath, journalPath, configPath, manifestDir string,
) *projectTUIBackend {
	return &projectTUIBackend{
		startPath: startPath, journalPath: journalPath,
		configPath: configPath, manifestDir: manifestDir,
		commandID: newTUICommandID,
		hosts:     make(map[tuiHostKey]*tuiResidentHost),
	}
}

func (b *projectTUIBackend) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.mu.Unlock()

	b.inFlight.Wait()

	b.mu.Lock()
	defer b.mu.Unlock()
	var errs []error
	for _, host := range b.hosts {
		if host != nil {
			errs = append(errs, host.close())
		}
	}
	b.hosts = nil
	return errors.Join(errs...)
}

func (b *projectTUIBackend) getOrCreateHost(
	ctx context.Context,
	journalPath string,
	configPath string,
) (*tuiResidentHost, error) {
	key := tuiHostKey{
		journalPath: journalPath,
		configPath:  configPath,
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, errors.New("project control is unavailable")
	}
	if host, ok := b.hosts[key]; ok && host != nil {
		return host, nil
	}
	service, factory, err := openRuntimeService(ctx, journalPath, configPath)
	if err != nil {
		return nil, errors.New("project control is unavailable")
	}
	store, err := journal.Open(ctx, journalPath)
	if err != nil {
		_ = service.Close()
		if factory != nil {
			_ = factory.Close()
		}
		return nil, errors.New("project control is unavailable")
	}
	commands, err := cockpit.NewCommandFacade(service, store, nil)
	if err != nil {
		_ = store.Close()
		_ = service.Close()
		if factory != nil {
			_ = factory.Close()
		}
		return nil, errors.New("project control is unavailable")
	}
	host := &tuiResidentHost{
		service:  service,
		factory:  factory,
		store:    store,
		commands: commands,
	}
	if b.hosts == nil {
		b.hosts = make(map[tuiHostKey]*tuiResidentHost)
	}
	b.hosts[key] = host
	return host, nil
}

func (b *projectTUIBackend) commandsForRun(
	ctx context.Context,
	run projectRun,
	configPath string,
) (*cockpit.CommandFacade, error) {
	resolvedConfig := configPath
	if resolvedConfig == "" {
		resolvedConfig = run.configPath
	}
	host, err := b.getOrCreateHost(ctx, run.journalPath, resolvedConfig)
	if err != nil {
		return nil, errors.New("project control is unavailable")
	}
	binding, err := host.store.RunBinding(ctx, run.binding.ID)
	if err != nil || !sameTUIRunBinding(binding, run.binding) {
		return nil, errors.New("project control authority changed")
	}
	return host.commands, nil
}

func (b *projectTUIBackend) Catalog(
	ctx context.Context,
) (tui.Catalog, error) {
	if b == nil || ctx == nil {
		return tui.Catalog{}, errors.New("project releases are unavailable")
	}
	project, err := b.discover(ctx)
	if err != nil {
		return tui.Catalog{}, err
	}
	entries := make([]tui.CatalogEntry, 0, len(project.releases))
	for index := len(project.releases) - 1; index >= 0; index-- {
		release := project.releases[index]
		run, hasRun := latestProjectRun(release)
		runID := ""
		status := "Sworn release"
		needsYou := "Open the board to see the next handoff."
		checked := "The latest saved Sworn release."
		if hasRun {
			runID = run.binding.ID
			status = "Sworn run saved"
			needsYou = "Open the board to see whether Sworn needs you."
			checked = "Run " + runID
		}
		if release.diagnostic != "" {
			status = "Sworn release needs attention"
			if hasRun {
				status = "Sworn run · release needs attention"
			}
			needsYou = "Yes — review this release before delivery can start."
			checked = "Saved Sworn run information."
		}
		selection, err := newTUISelection(project, release, runID)
		if err != nil {
			return tui.Catalog{}, errors.New("project releases are unavailable")
		}
		entries = append(entries, tui.CatalogEntry{
			Selection: selection,
			Status:    status,
			NeedsYou:  needsYou,
			Checked:   checked,
		})
	}
	return tui.Catalog{Entries: entries}, nil
}

func (b *projectTUIBackend) Board(
	ctx context.Context,
	selection tui.Selection,
) (tui.Board, error) {
	if b == nil || ctx == nil {
		return tui.Board{}, errors.New("project board is unavailable")
	}
	project, err := b.discover(ctx)
	if err != nil {
		return tui.Board{}, err
	}
	release, run, hasRun, err := resolveTUISelection(project, selection)
	if err != nil {
		return tui.Board{}, err
	}
	if hasRun {
		snapshot, err := readRunBoard(ctx, run.binding.ID, run.journalPath)
		if err != nil || snapshot.Run.Release != release.name {
			return tui.Board{}, errors.New("project board is unavailable")
		}
		presentation := cockpit.PresentSnapshot(snapshot)
		actions := append([]cockpit.Action(nil), snapshot.Actions...)
		if snapshot.ApprovalOffer != nil {
			command := snapshot.ApprovalOffer.Command
			actions = append([]cockpit.Action{{
				Kind: "approve", Approval: &command,
			}}, actions...)
		}
		return tui.Board{
			Selection: selection, Graph: snapshot.Graph,
			Actions: actions, Attentions: snapshot.Runtime.Attentions,
			Diagnostics: snapshot.Diagnostics,
			Status:      presentation.Status, What: presentation.What,
			Next: presentation.Next, NeedsYou: presentation.NeedsYou,
			Checked:          presentation.Checked,
			CaptainAuthority: captainDelegationTUILabel(snapshot.CaptainDelegation),
			ThroughOffset:    snapshot.ThroughOffset,
			ManifestDir:      project.paths.manifestDir,
		}, nil
	}
	if release.diagnostic != "" {
		return tui.Board{
			Selection: selection,
			Diagnostics: []cockpit.Diagnostic{{
				Code: release.diagnostic,
			}},
			Status:      "Needs confirmation",
			What:        "Sworn found saved run information, but this release's saved state could not be read.",
			Next:        "Review this release, then refresh.",
			NeedsYou:    "Yes — review this release before delivery can start.",
			Checked:     "Saved run information and the local Sworn release record.",
			ManifestDir: project.paths.manifestDir,
		}, nil
	}

	stateReader, err := cockpit.NewGitStateReader(project.repository.GitExecutable())
	if err != nil {
		return tui.Board{}, errors.New("project board is unavailable")
	}
	state, err := stateReader.Read(ctx, journal.Run{
		Repository: project.paths.root,
		Release:    release.name,
	})
	if err != nil {
		code := baton.ErrorCode(err)
		if code != "" {
			return tui.Board{
				Selection: selection,
				Diagnostics: []cockpit.Diagnostic{{
					Code: code,
				}},
				Status:      "Needs confirmation",
				What:        "Sworn found saved run information, but this release's saved state could not be read.",
				Next:        "Review this release, then refresh.",
				NeedsYou:    "Yes — review this release before delivery can start.",
				Checked:     "Saved run information and the local Sworn release record.",
				ManifestDir: project.paths.manifestDir,
			}, nil
		}
		return tui.Board{}, errors.New("project board is unavailable")
	}
	snapshot := cockpit.ProjectRelease(state)
	for _, code := range project.diagnostics {
		snapshot.Diagnostics = append(
			snapshot.Diagnostics,
			cockpit.Diagnostic{Code: code},
		)
	}
	journalUnavailable := hasTUIProjectDiagnostic(
		project.diagnostics,
		"SWORN_UNAVAILABLE",
	)
	actions := []cockpit.Action{}
	if release.manifest != "" && !journalUnavailable {
		actions = append(actions, cockpit.Action{Kind: "start"})
		if body, readErr := readManifest(release.manifest); readErr == nil {
			if manifest, parseErr := runtimepkg.ParseManifest(body); parseErr == nil {
				actions = append(actions, cockpit.Action{
					Kind: "start_delegated",
					CaptainDelegation: &cockpit.CaptainDelegationAction{
						Action: "admit", RunID: manifest.RunID,
						ManifestDigest: sha256Digest(body),
						ActorClass:     runtimepkg.CaptainDelegationActorClass,
						ActorAuthority: manifest.Authority.ExternalAuthorizer,
					},
				})
			}
		}
	}
	presentation := snapshot.Presentation
	if release.manifest == "" && !journalUnavailable {
		if presentation.Next == "Start delivery." || presentation.Next == "" {
			presentation.Next = "Provide a run definition in " + project.paths.manifestDir + " before starting delivery."
		}
	}
	if journalUnavailable {
		presentation.Status = "Needs confirmation"
		presentation.What = "Sworn could not confirm the saved run records."
		presentation.Next = "Restore the saved Sworn run record, then refresh this board."
		presentation.NeedsYou = "Yes — delivery controls are disabled until the saved facts can be confirmed."
		presentation.Checked = "This release is visible, but the saved Sworn run record is unavailable."
	}
	return tui.Board{
		Selection: selection, Graph: snapshot.Graph,
		Actions: actions, Diagnostics: snapshot.Diagnostics,
		Status:      presentation.Status,
		What:        presentation.What,
		Next:        presentation.Next,
		NeedsYou:    presentation.NeedsYou,
		Checked:     presentation.Checked,
		ManifestDir: project.paths.manifestDir,
	}, nil
}

func (b *projectTUIBackend) Config(
	ctx context.Context,
) (tui.ConfigView, error) {
	if b == nil || ctx == nil {
		return tui.ConfigView{}, errors.New("project configuration is unavailable")
	}
	project, err := b.discover(ctx)
	if err != nil {
		return tui.ConfigView{}, err
	}

	configView := tui.ConfigView{
		JournalPath: tui.ConfigItem{
			Value:  project.paths.journal,
			Source: projectConfigSource(project.paths.root, project.paths.journal),
		},
		ManifestDir: tui.ConfigItem{
			Value:  project.paths.manifestDir,
			Source: projectConfigSource(project.paths.root, project.paths.manifestDir),
		},
		DriverConfig: tui.ConfigItem{
			Value:  project.paths.config,
			Source: projectConfigSource(project.paths.root, project.paths.config),
		},
	}

	projectConfig, configured, err := gitx.LoadProjectConfig(project.paths.root)
	_ = err
	projectJSONPath := filepath.Join(project.paths.root, filepath.FromSlash(gitx.ProjectConfigPath))
	projectSource := projectConfigSource(project.paths.root, projectJSONPath)
	if !configured {
		projectSource += " (default)"
	}
	configView.RecordsRoot = tui.ConfigItem{
		Value:  projectConfig.RecordsRoot,
		Source: projectSource,
	}
	configView.JournalsRoot = tui.ConfigItem{
		Value:  projectConfig.JournalsRoot,
		Source: projectSource,
	}

	if project.operatorConfig != "" {
		operatorSource := projectConfigSource(project.paths.root, project.operatorConfig)
		settings, err := loadOperatorSettings(project.operatorConfig)
		if err == nil {
			listen := settings.localListen
			if listen == "" {
				listen = "(not configured)"
			}
			configView.OperatorListen = tui.ConfigItem{
				Value:  listen,
				Source: operatorSource,
			}
			otel := ""
			if settings.otel != nil {
				otel = settings.otel.Endpoint
			}
			if otel == "" {
				otel = "(not configured)"
			}
			configView.OperatorOTel = tui.ConfigItem{
				Value:  otel,
				Source: operatorSource,
			}
		} else {
			configView.OperatorListen = tui.ConfigItem{
				Value:  "(unreadable)",
				Source: operatorSource,
			}
			configView.OperatorOTel = tui.ConfigItem{
				Value:  "(unreadable)",
				Source: operatorSource,
			}
		}
	} else {
		configView.OperatorListen = tui.ConfigItem{
			Value:  "(not configured)",
			Source: "(none)",
		}
		configView.OperatorOTel = tui.ConfigItem{
			Value:  "(not configured)",
			Source: "(none)",
		}
	}

	driverSource := projectConfigSource(project.paths.root, project.paths.config)
	if body, err := os.ReadFile(project.paths.config); err == nil {
		var driverConfig driver.DriverConfig
		if jsonErr := json.Unmarshal(body, &driverConfig); jsonErr == nil {
			for _, p := range driverConfig.Profiles {
				configView.Profiles = append(configView.Profiles, tui.ProfileViewEntry{
					Name:    p.Key,
					Adapter: p.Adapter,
					Network: string(p.Network),
					Source:  driverSource,
				})
			}
		}
	}

	seenRoles := false
	for _, release := range project.releases {
		if release.manifest == "" {
			continue
		}
		body, readErr := readManifest(release.manifest)
		if readErr != nil {
			continue
		}
		manifest, parseErr := runtimepkg.ParseManifest(body)
		if parseErr != nil {
			continue
		}
		manifestSource := projectConfigSource(project.paths.root, release.manifest)
		configView.Roles = []tui.RoleMatrixEntry{
			{Role: "planner", Profile: manifest.Roles.Planner.Profile, Model: manifest.Roles.Planner.Model, Source: manifestSource},
			{Role: "implementer", Profile: manifest.Roles.Implementer.Profile, Model: manifest.Roles.Implementer.Model, Source: manifestSource},
			{Role: "captain", Profile: manifest.Roles.Captain.Profile, Model: manifest.Roles.Captain.Model, Source: manifestSource},
			{Role: "verifier", Profile: manifest.Roles.Verifier.Profile, Model: manifest.Roles.Verifier.Model, Source: manifestSource},
		}
		if manifest.Automation != nil && manifest.Automation.Recovery.Profile != "" {
			configView.Roles = append(configView.Roles, tui.RoleMatrixEntry{
				Role: "recovery", Profile: manifest.Automation.Recovery.Profile, Model: manifest.Automation.Recovery.Model, Source: manifestSource,
			})
		}
		seenRoles = true
		break
	}

	if !seenRoles && len(configView.Profiles) > 0 {
		for _, profile := range configView.Profiles {
			configView.Roles = append(configView.Roles, tui.RoleMatrixEntry{
				Role:    "(profile)",
				Profile: profile.Name,
				Model:   "(from manifest)",
				Source:  driverSource,
			})
		}
	}

	return configView, nil
}

func projectConfigSource(root, fullPath string) string {
	if fullPath == "" {
		return "(none)"
	}
	if root != "" && strings.HasPrefix(fullPath, root) {
		rel, err := filepath.Rel(root, fullPath)
		if err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return fullPath
}

func (b *projectTUIBackend) Events(
	ctx context.Context,
	selection tui.Selection,
	after int64,
	limit int,
	track string,
) (cockpit.EventPage, error) {
	if b == nil || ctx == nil {
		return cockpit.EventPage{}, errors.New("project events are unavailable")
	}
	project, err := b.discover(ctx)
	if err != nil {
		return cockpit.EventPage{}, err
	}
	_, run, hasRun, err := resolveTUISelection(project, selection)
	if err != nil {
		return cockpit.EventPage{}, err
	}
	if !hasRun {
		return cockpit.EventPage{}, errors.New("the selected release has no active run")
	}
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		return cockpit.EventPage{}, errRunBoardGit
	}
	journalReader, err := journal.OpenReadOnly(ctx, run.journalPath)
	if err != nil {
		return cockpit.EventPage{}, errRunBoardJournal
	}
	defer journalReader.Close()
	statusReader, err := runtimepkg.OpenStatusService(ctx, run.journalPath)
	if err != nil {
		return cockpit.EventPage{}, errRunBoardJournal
	}
	defer statusReader.Close()
	stateReader, err := cockpit.NewGitStateReader(gitExecutable)
	if err != nil {
		return cockpit.EventPage{}, errRunBoardGit
	}
	projector, err := cockpit.NewProjector(
		journalReader,
		statusReader,
		stateReader,
	)
	if err != nil {
		return cockpit.EventPage{}, err
	}
	var trackArgs []string
	if track != "" {
		trackArgs = []string{track}
	}
	return projector.Events(ctx, run.binding.ID, after, limit, trackArgs...)
}

func captainDelegationTUILabel(value *runtimepkg.CaptainDelegationView) string {
	if value == nil {
		return "External human approval"
	}
	return "captain_plan_review epoch " + strconv.FormatInt(value.Epoch, 10) + " " + value.State +
		" · decisions " + strconv.FormatInt(value.Decisions, 10) +
		" · replans " + strconv.FormatInt(value.ReplanSpent, 10) + "/" + strconv.FormatInt(value.ReplanBudget, 10)
}

func (b *projectTUIBackend) Execute(
	ctx context.Context,
	selection tui.Selection,
	action cockpit.Action,
	answer string,
) error {
	if b == nil || ctx == nil {
		return errors.New("project control is unavailable")
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return errors.New("project control is unavailable")
	}
	b.inFlight.Add(1)
	b.mu.Unlock()
	defer b.inFlight.Done()

	board, err := b.Board(ctx, selection)
	if err != nil || !exactTUIAction(board.Actions, action) {
		return errors.New("the current board does not allow that action")
	}
	project, err := b.discover(ctx)
	if err != nil {
		return errors.New("project control is unavailable")
	}
	release, run, hasRun, err := resolveTUISelection(project, selection)
	if err != nil {
		return errors.New("project control is unavailable")
	}
	if action.Kind == "start" {
		if hasRun || release.manifest == "" || answer != "" {
			return errors.New("the current board does not allow that action")
		}
		return b.startTUIRun(
			ctx,
			project,
			release,
			selection,
			release.manifest,
			project.paths.journal,
			existingRegularFile(project.paths.config),
		)
	}
	if action.Kind == "start_delegated" {
		if hasRun || release.manifest == "" || action.CaptainDelegation == nil ||
			strings.TrimSpace(answer) == "" {
			return errors.New("the current board does not allow that action")
		}
		return b.startTUIDelegatedRun(
			ctx, project, release, selection, action, []byte(answer),
			project.paths.journal, existingRegularFile(project.paths.config),
		)
	}
	if !hasRun {
		return errors.New("the current board does not allow that action")
	}
	return b.executeRunAction(ctx, run, action, answer)
}

func (b *projectTUIBackend) executeRunAction(
	ctx context.Context,
	run projectRun,
	action cockpit.Action,
	answer string,
) error {
	configPath := run.configPath
	var commandID string
	switch action.Kind {
	case "approve":
		if answer != "" || action.Approval == nil {
			return errors.New("the current board does not allow that action")
		}
	case "pause", "cancel":
		if answer != "" {
			return errors.New("the current board does not allow that action")
		}
		var err error
		commandID, err = b.commandID()
		if err != nil {
			return errors.New("project control is unavailable")
		}
	case "resume", "takeover", "retry":
		if answer != "" {
			return errors.New("the current board does not allow that action")
		}
		var err error
		commandID, err = b.commandID()
		if err != nil {
			return errors.New("project control is unavailable")
		}
	case "answer_attention":
		if strings.TrimSpace(answer) == "" ||
			len(answer) > journal.MaxAttentionAnswerBytes {
			return errors.New("the answer is empty or too long")
		}
	case "redeliver":
		if answer != "" {
			return errors.New("the current board does not allow that action")
		}
	case "captain_delegation_revoke":
		if answer != "" || action.CaptainDelegation == nil ||
			action.CaptainDelegation.Action != "revoke" {
			return errors.New("the current board does not allow that action")
		}
	case "captain_delegation_replace":
		if strings.TrimSpace(answer) == "" || action.CaptainDelegation == nil ||
			action.CaptainDelegation.Action != "replace" {
			return errors.New("the current board does not allow that action")
		}
	default:
		return errors.New("the current board does not allow that action")
	}

	commands, err := b.commandsForRun(ctx, run, configPath)
	if err != nil {
		return err
	}
	switch action.Kind {
	case "approve":
		_, err = commands.Approve(ctx, *action.Approval)
	case "pause", "resume", "cancel", "takeover", "retry":
		command := cockpit.ControlCommand{
			RunID: run.binding.ID, CommandID: commandID,
			Kind:               journal.ControlKind(action.Kind),
			ExpectedGeneration: action.ExpectedGeneration,
			WorkID:             action.WorkID, ExpectedEpoch: action.ExpectedEpoch,
		}
		err = replayPendingTUIControl(ctx, commands, command)
	case "answer_attention":
		_, err = commands.AnswerAttention(ctx, cockpit.AnswerAttentionCommand{
			RunID: run.binding.ID, AttentionID: action.AttentionID,
			ExpectedGeneration: action.ExpectedGeneration,
			Answer:             answer,
		})
	case "redeliver":
		err = commands.Redeliver(ctx, cockpit.RedeliveryCommand{
			RunID: run.binding.ID, DestinationID: action.DestinationID,
			MessageID: action.MessageID,
		})
	case "captain_delegation_revoke", "captain_delegation_replace":
		binding := action.CaptainDelegation
		command := runtimepkg.CaptainDelegationCommand{
			SchemaVersion: runtimepkg.CaptainDelegationCommandVersion,
			Action:        binding.Action, RunID: binding.RunID,
			ManifestDigest: binding.ManifestDigest,
			ActorClass:     binding.ActorClass, ActorAuthority: binding.ActorAuthority,
			CurrentEpoch: binding.CurrentEpoch, CurrentDigest: binding.CurrentDigest,
		}
		if binding.Action == "replace" {
			admitted, parseErr := runtimepkg.ParseCaptainDelegation([]byte(answer))
			if parseErr != nil {
				return errors.New("the replacement envelope is invalid")
			}
			command.EnvelopeBytes = admitted.Bytes
			command.EnvelopeDigest = admitted.Digest
		}
		_, err = commands.CaptainDelegation(ctx, command)
	}
	if err != nil {
		return errors.New("the current board rejected that action")
	}
	return nil
}

type tuiControlAPI interface {
	Control(
		context.Context,
		cockpit.ControlCommand,
	) (runtimepkg.RunStatus, error)
}

func replayPendingTUIControl(
	ctx context.Context,
	controls tuiControlAPI,
	command cockpit.ControlCommand,
) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := controls.Control(ctx, command)
		if !cockpit.IsCode(err, "OWNER_TRANSITION_PENDING") {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (b *projectTUIBackend) startTUIRun(
	ctx context.Context,
	project projectCatalog,
	release projectRelease,
	selection tui.Selection,
	manifestPath, journalPath, configPath string,
) error {
	body, err := readManifest(manifestPath)
	if err != nil {
		return errors.New("the run definition is unavailable")
	}
	manifest, err := runtimepkg.ParseManifest(body)
	if err != nil || manifest.Repository != project.paths.root ||
		manifest.Release != release.name {
		return errors.New("the run definition is invalid")
	}
	manifestDigest := sha256Digest(body)
	if tuiSelectionWithManifest(
		project,
		release,
		"",
		manifestDigest,
	) != selection {
		return errors.New("the run definition changed; refresh before starting")
	}
	host, err := b.getOrCreateHost(ctx, journalPath, configPath)
	if err != nil {
		return errors.New("project control is unavailable")
	}
	status, err := host.service.StartDetached(ctx, body)
	if err != nil {
		return errors.New("the run could not be started")
	}
	if status.RunID != manifest.RunID || status.ManifestDigest != manifestDigest {
		return errors.New("the started run did not match the selected definition")
	}
	host.attachTelemetry(
		ctx,
		journalPath,
		manifest.RunID,
		project.operatorConfig,
	)
	return nil
}

func (b *projectTUIBackend) startTUIDelegatedRun(
	ctx context.Context,
	project projectCatalog,
	release projectRelease,
	selection tui.Selection,
	action cockpit.Action,
	envelope []byte,
	journalPath, configPath string,
) error {
	body, err := readManifest(release.manifest)
	if err != nil {
		return errors.New("the run definition is unavailable")
	}
	manifest, err := runtimepkg.ParseManifest(body)
	binding := action.CaptainDelegation
	if err != nil || binding == nil || binding.Action != "admit" ||
		manifest.Repository != project.paths.root || manifest.Release != release.name ||
		binding.RunID != manifest.RunID || binding.ManifestDigest != sha256Digest(body) ||
		binding.ActorClass != runtimepkg.CaptainDelegationActorClass ||
		binding.ActorAuthority != manifest.Authority.ExternalAuthorizer ||
		tuiSelectionWithManifest(project, release, "", sha256Digest(body)) != selection {
		return errors.New("the delegated run authority changed; refresh before starting")
	}
	host, err := b.getOrCreateHost(ctx, journalPath, configPath)
	if err != nil {
		return errors.New("project control is unavailable")
	}
	status, err := host.service.StartWithCaptainDelegationDetached(ctx, body, envelope)
	if err != nil {
		return errors.New("the delegated run could not be started")
	}
	if status.RunID != manifest.RunID || status.ManifestDigest != binding.ManifestDigest {
		return errors.New("the started run did not match the selected definition")
	}
	host.attachTelemetry(
		ctx,
		journalPath,
		manifest.RunID,
		project.operatorConfig,
	)
	return nil
}

func (b *projectTUIBackend) discover(
	ctx context.Context,
) (projectCatalog, error) {
	return discoverProject(
		ctx,
		b.startPath,
		b.journalPath,
		b.configPath,
		b.manifestDir,
	)
}

func readRunBoard(
	ctx context.Context,
	runID, journalPath string,
) (cockpit.Snapshot, error) {
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		return cockpit.Snapshot{}, errRunBoardGit
	}
	journalReader, err := journal.OpenReadOnly(ctx, journalPath)
	if err != nil {
		return cockpit.Snapshot{}, errRunBoardJournal
	}
	defer journalReader.Close()
	statusReader, err := runtimepkg.OpenStatusService(ctx, journalPath)
	if err != nil {
		if runtimepkg.IsCode(err, "GIT_UNAVAILABLE") {
			return cockpit.Snapshot{}, errRunBoardGit
		}
		return cockpit.Snapshot{}, errRunBoardJournal
	}
	defer statusReader.Close()
	stateReader, err := cockpit.NewGitStateReader(gitExecutable)
	if err != nil {
		return cockpit.Snapshot{}, errRunBoardGit
	}
	projector, err := cockpit.NewProjector(
		journalReader,
		statusReader,
		stateReader,
	)
	if err != nil {
		return cockpit.Snapshot{}, err
	}
	return projector.Snapshot(ctx, runID)
}

func resolveTUISelection(
	project projectCatalog,
	selection tui.Selection,
) (projectRelease, projectRun, bool, error) {
	if selection.Release == "" || selection.Source == "" {
		return projectRelease{}, projectRun{}, false,
			errors.New("project selection is unavailable")
	}
	for _, release := range project.releases {
		if release.name != selection.Release {
			continue
		}
		expected, err := newTUISelection(project, release, selection.RunID)
		if err != nil || expected != selection {
			break
		}
		if selection.RunID != "" {
			for _, run := range release.runs {
				if run.binding.ID == selection.RunID {
					return release, run, true, nil
				}
			}
			break
		}
		if len(release.runs) != 0 {
			break
		}
		return release, projectRun{}, false, nil
	}
	return projectRelease{}, projectRun{}, false,
		errors.New("project selection is no longer available")
}

func newTUISelection(
	project projectCatalog,
	release projectRelease,
	runID string,
) (tui.Selection, error) {
	manifestDigest := release.manifestDigest
	if runID != "" {
		manifestDigest = ""
		for _, run := range release.runs {
			if run.binding.ID == runID {
				manifestDigest = run.binding.ManifestDigest
				break
			}
		}
		if manifestDigest == "" {
			return tui.Selection{}, errors.New("run authority is unavailable")
		}
	}
	return tuiSelectionWithManifest(
		project,
		release,
		runID,
		manifestDigest,
	), nil
}

func tuiSelectionWithManifest(
	project projectCatalog,
	release projectRelease,
	runID, manifestDigest string,
) tui.Selection {
	authority := strings.Join([]string{
		project.paths.root,
		release.name,
		release.sourceRef,
		runID,
		manifestDigest,
		project.paths.journal,
		project.paths.config,
	}, "\x00")
	sum := sha256.Sum256([]byte(authority))
	return tui.Selection{
		Release: release.name,
		RunID:   runID,
		Source:  "sha256:" + hex.EncodeToString(sum[:]),
	}
}

func exactTUIAction(actions []cockpit.Action, target cockpit.Action) bool {
	for _, action := range actions {
		if reflect.DeepEqual(action, target) {
			return true
		}
	}
	return false
}

func hasTUIProjectDiagnostic(diagnostics []string, target string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic == target {
			return true
		}
	}
	return false
}

func newTUICommandID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "tui-" + hex.EncodeToString(value[:]), nil
}

func sameTUIRunBinding(left, right journal.Run) bool {
	return left.ID == right.ID &&
		left.ManifestDigest == right.ManifestDigest &&
		left.Repository == right.Repository &&
		left.Release == right.Release &&
		left.TargetRef == right.TargetRef &&
		left.CreatedAt.Equal(right.CreatedAt)
}

func sha256Digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
