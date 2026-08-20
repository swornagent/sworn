package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/gitx"
	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

type pendingTUIControls struct {
	commands []cockpit.ControlCommand
}

func TestCaptainDelegationTUILabelProjectsHumanAndDelegatedAuthority(t *testing.T) {
	if got := captainDelegationTUILabel(nil); got != "External human approval" {
		t.Fatalf("human label = %q", got)
	}
	view := &runtimepkg.CaptainDelegationView{Epoch: 3, State: "revoked", Decisions: 4, ReplanSpent: 2, ReplanBudget: 5}
	if got := captainDelegationTUILabel(view); got != "captain_plan_review epoch 3 revoked · decisions 4 · replans 2/5" {
		t.Fatalf("delegated label = %q", got)
	}
}

func (p *pendingTUIControls) Control(
	_ context.Context,
	command cockpit.ControlCommand,
) (runtimepkg.RunStatus, error) {
	p.commands = append(p.commands, command)
	if len(p.commands) == 1 {
		return runtimepkg.RunStatus{}, &cockpit.Error{
			Code: "OWNER_TRANSITION_PENDING",
		}
	}
	return runtimepkg.RunStatus{}, nil
}

func TestTUISelectionBindsManifestAuthorityButNotReleaseProgress(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestPath := filepath.Join(root, "run.json")
	if err := os.WriteFile(manifestPath, []byte("manifest one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	project := projectCatalog{paths: projectPaths{
		root: root, journal: filepath.Join(root, "sworn.db"),
		config: filepath.Join(root, "drivers.json"),
	}}
	release := projectRelease{
		name: "delivery", sourceRef: "refs/heads/release-wt/delivery",
		manifest: manifestPath, manifestDigest: sha256Digest([]byte("manifest one\n")),
	}

	selected, err := newTUISelection(project, release, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte("manifest two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	release.manifestDigest = sha256Digest([]byte("manifest two\n"))
	afterManifest, err := newTUISelection(project, release, "")
	if err != nil {
		t.Fatal(err)
	}
	if afterManifest == selected {
		t.Fatalf("changed manifest retained selection %#v", selected)
	}
}

func TestTUISelectionBindsImmutableRunManifest(t *testing.T) {
	t.Parallel()

	project := projectCatalog{paths: projectPaths{
		root: "/project", journal: "/project/sworn.db",
		config: "/project/drivers.json",
	}}
	release := projectRelease{
		name: "delivery", sourceRef: "refs/heads/release-wt/delivery",
		runs: []projectRun{{binding: journal.Run{
			ID: "run-1", ManifestDigest: "sha256:" + stringOf('a', 64),
		}}},
	}
	selected, err := newTUISelection(project, release, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	release.runs[0].binding.ManifestDigest = "sha256:" + stringOf('b', 64)
	changed, err := newTUISelection(project, release, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if changed == selected {
		t.Fatalf("changed run manifest binding retained selection %#v", selected)
	}
}

func TestResolveTUISelectionRejectsReleaseOnlySelectionAfterRunAppears(t *testing.T) {
	t.Parallel()

	project := projectCatalog{paths: projectPaths{
		root: "/project", journal: "/project/sworn.db",
		config: "/project/drivers.json",
	}}
	release := projectRelease{
		name: "delivery", sourceRef: "refs/heads/release-wt/delivery",
	}
	project.releases = []projectRelease{release}
	selection, err := newTUISelection(project, release, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, hasRun, err := resolveTUISelection(project, selection); err != nil || hasRun {
		t.Fatalf("initial release selection = hasRun %t, error %v", hasRun, err)
	}

	project.releases[0].runs = []projectRun{{binding: journal.Run{
		ID: "run-1", ManifestDigest: "sha256:" + stringOf('c', 64),
	}}}
	if _, _, _, err := resolveTUISelection(project, selection); err == nil {
		t.Fatal("release-only selection followed a run that appeared later")
	}
}

func TestProjectDiagnosticsDisableSynthesizedStart(t *testing.T) {
	root, _ := projectRepositoryFixture(t)
	gitExecutable, err := resolveGitExecutable()
	if err != nil {
		t.Fatal(err)
	}
	repository, err := gitx.Open(root, gitExecutable)
	if err != nil {
		t.Fatal(err)
	}
	actions, err := baton.NewActions(
		baton.UseGitRepository(repository),
		func(request gitx.RecordRootRequest) (gitx.RecordRootDecision, error) {
			return gitx.RecordRootDecision{
				Kind: request.Kind, Repository: request.Repository,
				RecordRoot: request.RecordRoot, Commit: request.Commit,
				Decision: "inert",
			}, nil
		},
		gitx.Identity{Name: "TUI Test Engine", Email: "engine@example.test"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := actions.RecordPlanRevision(baton.RecordPlanRevisionInput{
		PlanBytes: projectTUIPlan(t, "delivery"),
		Summary:   "Approve the TUI fixture.",
	}); err != nil {
		t.Fatal(err)
	}

	manifestDir := filepath.Join(root, ".sworn", "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projectWriteManifest(t, manifestDir, "delivery.json", root, "delivery", "run-1")
	journalPath := filepath.Join(root, ".sworn", "sworn.db")
	if err := os.WriteFile(journalPath, []byte("not a Sworn journal\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	backend := newProjectTUIBackend(root, "", "", "")
	catalog, err := backend.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Entries) != 1 {
		t.Fatalf("catalog entries = %#v", catalog.Entries)
	}
	board, err := backend.Board(context.Background(), catalog.Entries[0].Selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Actions) != 0 {
		t.Fatalf("unavailable journal exposed actions %#v", board.Actions)
	}
	if !hasTUIDiagnostic(board.Diagnostics, "SWORN_UNAVAILABLE") {
		t.Fatalf("board diagnostics = %#v", board.Diagnostics)
	}

	if err := os.Remove(journalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(manifestDir, "unrelated-invalid.json"),
		[]byte("{}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	catalog, err = backend.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	board, err = backend.Board(context.Background(), catalog.Entries[0].Selection)
	if err != nil {
		t.Fatal(err)
	}
	if !exactTUIAction(board.Actions, cockpit.Action{Kind: "start"}) {
		t.Fatalf("unrelated invalid manifest blocked valid start: %#v", board)
	}
	delegated := false
	for _, action := range board.Actions {
		if action.Kind == "start_delegated" && action.CaptainDelegation != nil &&
			action.CaptainDelegation.Action == "admit" &&
			action.CaptainDelegation.ActorClass == runtimepkg.CaptainDelegationActorClass &&
			action.CaptainDelegation.RunID == "run-1" &&
			strings.HasPrefix(action.CaptainDelegation.ManifestDigest, "sha256:") {
			delegated = true
		}
	}
	if !delegated {
		t.Fatalf("valid manifest omitted bound delegated start: %#v", board.Actions)
	}
	for _, diagnostic := range board.Diagnostics {
		if strings.HasPrefix(diagnostic.Code, "MANIFEST") ||
			diagnostic.Code == "DUPLICATE_RELEASE_MANIFEST" {
			t.Fatalf("unrelated manifest contaminated valid release: %#v", board)
		}
	}
}

func TestMissingReleaseCatalogAndBoardUseSwornOwnedLanguage(t *testing.T) {
	t.Parallel()

	root, _ := projectRepositoryFixture(t)
	manifestDir := filepath.Join(root, ".sworn", "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projectWriteManifest(t, manifestDir, "delivery.json", root, "delivery", "run-1")

	backend := newProjectTUIBackend(root, "", "", "")
	catalog, err := backend.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Entries) != 1 {
		t.Fatalf("catalog entries = %#v", catalog.Entries)
	}
	entry := catalog.Entries[0]
	for _, field := range []string{entry.Status, entry.NeedsYou, entry.Checked} {
		if strings.Contains(field, "Baton") {
			t.Fatalf("catalog entry names Baton as active authority: %#v", entry)
		}
	}
	if entry.Status != "Sworn release needs attention" ||
		entry.NeedsYou != "Yes — review this release before delivery can start." {
		t.Fatalf("catalog entry = %#v", entry)
	}

	board, err := backend.Board(context.Background(), entry.Selection)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTUIDiagnostic(board.Diagnostics, "BATON_UNAVAILABLE") {
		t.Fatalf("board diagnostics = %#v", board.Diagnostics)
	}
	for _, field := range []string{board.What, board.Next, board.NeedsYou, board.Checked} {
		if strings.Contains(field, "Baton") {
			t.Fatalf("board names Baton as active authority: %#v", board)
		}
		lower := strings.ToLower(field)
		if strings.Contains(lower, "restore") || strings.Contains(lower, "prepare") {
			t.Fatalf("board instructs restoring or preparing a release: %#v", board)
		}
	}
}

func TestExactTUIActionRequiresEveryField(t *testing.T) {
	t.Parallel()

	exact := cockpit.Action{
		Kind: "retry", ExpectedGeneration: 3,
		AttentionID: "attention", WorkID: "work", ExpectedEpoch: 2,
		DestinationID: "destination", MessageID: "message",
	}
	if !exactTUIAction([]cockpit.Action{exact}, exact) {
		t.Fatal("exact action was not admitted")
	}
	mutations := []cockpit.Action{
		withTUIAction(exact, func(value *cockpit.Action) { value.Kind = "resume" }),
		withTUIAction(exact, func(value *cockpit.Action) { value.ExpectedGeneration++ }),
		withTUIAction(exact, func(value *cockpit.Action) { value.AttentionID += "-other" }),
		withTUIAction(exact, func(value *cockpit.Action) { value.WorkID += "-other" }),
		withTUIAction(exact, func(value *cockpit.Action) { value.ExpectedEpoch++ }),
		withTUIAction(exact, func(value *cockpit.Action) { value.DestinationID += "-other" }),
		withTUIAction(exact, func(value *cockpit.Action) { value.MessageID += "-other" }),
	}
	for index, mutation := range mutations {
		if exactTUIAction([]cockpit.Action{exact}, mutation) {
			t.Fatalf("mutation %d was treated as exact: %#v", index, mutation)
		}
	}
}

func TestSameTUIRunBindingRequiresEveryImmutableField(t *testing.T) {
	t.Parallel()

	createdAt := time.Unix(1_700_000_000, 123).UTC()
	exact := journal.Run{
		ID: "run-1", ManifestDigest: "sha256:" + stringOf('d', 64),
		Repository: "/project", Release: "delivery",
		TargetRef: "refs/heads/main", CreatedAt: createdAt,
	}
	if !sameTUIRunBinding(exact, exact) {
		t.Fatal("exact run binding did not match")
	}
	sameInstant := exact
	sameInstant.CreatedAt = createdAt.In(time.FixedZone("fixture", 10*60*60))
	if !sameTUIRunBinding(exact, sameInstant) {
		t.Fatal("equal creation instant did not match")
	}
	mutations := []journal.Run{
		withTUIRun(exact, func(value *journal.Run) { value.ID += "-other" }),
		withTUIRun(exact, func(value *journal.Run) { value.ManifestDigest = "sha256:" + stringOf('e', 64) }),
		withTUIRun(exact, func(value *journal.Run) { value.Repository += "-other" }),
		withTUIRun(exact, func(value *journal.Run) { value.Release += "-other" }),
		withTUIRun(exact, func(value *journal.Run) { value.TargetRef += "-other" }),
		withTUIRun(exact, func(value *journal.Run) { value.CreatedAt = value.CreatedAt.Add(time.Nanosecond) }),
	}
	for index, mutation := range mutations {
		if sameTUIRunBinding(exact, mutation) {
			t.Fatalf("mutation %d was treated as the same binding: %#v", index, mutation)
		}
	}
}

func TestPendingTUIControlReplaysTheExactCommandUntilAccepted(t *testing.T) {
	t.Parallel()

	controls := &pendingTUIControls{}
	command := cockpit.ControlCommand{
		RunID: "run-1", CommandID: "resume-1", Kind: journal.Resume,
		ExpectedGeneration: 3,
	}
	err := replayPendingTUIControl(context.Background(), controls, command)
	if err != nil || len(controls.commands) != 2 ||
		controls.commands[0] != command || controls.commands[1] != command {
		t.Fatalf("pending control replay = %v, commands %#v", err, controls.commands)
	}
}

// A2: Quitting and reopening the TUI reattaches to the still-running run with its live state: pinned by a backend fixture over a run owned by a background drive.
func TestTUIReattachesToBackgroundDrivenRun(t *testing.T) {
	t.Parallel()

	root, _ := projectRepositoryFixture(t)
	installProjectPlan(t, root, "delivery")

	stateDir := filepath.Join(root, ".sworn")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifestDir := filepath.Join(stateDir, "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projectWriteManifest(t, manifestDir, "delivery.json", root, "delivery", "run-bg")

	manifestBody, err := os.ReadFile(filepath.Join(manifestDir, "delivery.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest := sha256Digest(manifestBody)

	journalPath := filepath.Join(stateDir, "sworn.db")
	store, err := journal.Open(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 123).UTC()
	if err := store.RegisterRun(context.Background(), journal.Run{
		ID:             "run-bg",
		ManifestDigest: manifestDigest,
		Repository:     root,
		Release:        "delivery",
		TargetRef:      "refs/heads/main",
		CreatedAt:      now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCommand(context.Background(), journal.Command{
		RunID:     "run-bg",
		ReplayKey: "manifest",
		Kind:      "start",
		Payload:   manifestBody,
		CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	// Simulate active background drive owner
	if _, err := store.AcquireOwner(context.Background(), "run-bg", now, 5*time.Minute, false); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(context.Background(), "run-bg", "started", []byte("run started in background"), now); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// 1. First TUI session attaches
	backend1 := newProjectTUIBackend(root, "", "", "")
	catalog1, err := backend1.Catalog(context.Background())
	if err != nil {
		t.Fatalf("catalog1 failed: %v", err)
	}
	if len(catalog1.Entries) != 1 || catalog1.Entries[0].Selection.RunID != "run-bg" {
		t.Fatalf("catalog1 entries = %#v", catalog1.Entries)
	}
	board1, err := backend1.Board(context.Background(), catalog1.Entries[0].Selection)
	if err != nil {
		t.Fatalf("board1 failed: %v", err)
	}
	if board1.Selection.RunID != "run-bg" {
		t.Fatalf("board1 runID = %q, want run-bg", board1.Selection.RunID)
	}
	initialOffset := board1.ThroughOffset

	// 2. Simulate quitting TUI: drop backend1 and simulate background drive continuing to append events
	store2, err := journal.Open(context.Background(), journalPath)
	if err != nil {
		t.Fatal(err)
	}
	now2 := now.Add(time.Second)
	if err := store2.AppendEvent(context.Background(), "run-bg", "work_advancing", []byte("slice in progress"), now2); err != nil {
		t.Fatal(err)
	}
	_ = store2.Close()

	// 3. Reopen TUI: create new backend instance and reattach
	backend2 := newProjectTUIBackend(root, "", "", "")
	catalog2, err := backend2.Catalog(context.Background())
	if err != nil {
		t.Fatalf("catalog2 failed: %v", err)
	}
	if len(catalog2.Entries) != 1 || catalog2.Entries[0].Selection.RunID != "run-bg" {
		t.Fatalf("catalog2 entries = %#v", catalog2.Entries)
	}
	board2, err := backend2.Board(context.Background(), catalog2.Entries[0].Selection)
	if err != nil {
		t.Fatalf("board2 failed: %v", err)
	}
	if board2.Selection.RunID != "run-bg" {
		t.Fatalf("board2 runID = %q, want run-bg", board2.Selection.RunID)
	}
	if board2.ThroughOffset <= initialOffset {
		t.Fatalf("board2 throughOffset = %d, want > initial %d", board2.ThroughOffset, initialOffset)
	}
}

// A3: Baton unreadable codes propagate from Board with diagnostics and searched manifest location.
func TestBoardPropagatesBatonDiagnosticsAndSearchedManifestDir(t *testing.T) {
	t.Parallel()

	root, head := projectRepositoryFixture(t)
	// Create release ref with no plan committed -> PLAN_NOT_FOUND
	projectCreateReleaseRef(t, root, "unplanned", head)

	manifestDir := filepath.Join(root, ".sworn", "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}

	backend := newProjectTUIBackend(root, "", "", "")
	catalog, err := backend.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Entries) != 1 {
		t.Fatalf("catalog entries = %#v", catalog.Entries)
	}
	board, err := backend.Board(context.Background(), catalog.Entries[0].Selection)
	if err != nil {
		t.Fatalf("board failed: %v", err)
	}
	if !hasTUIDiagnostic(board.Diagnostics, "PLAN_NOT_FOUND") {
		t.Fatalf("board diagnostics = %#v, want PLAN_NOT_FOUND", board.Diagnostics)
	}
	if board.ManifestDir != manifestDir {
		t.Fatalf("board ManifestDir = %q, want %q", board.ManifestDir, manifestDir)
	}
}

// A4: Config view shows resolved role matrix, operator endpoints, records root, and journal/manifest paths with source files.
func TestBackendConfigResolvesMatrixAndSourceFiles(t *testing.T) {
	t.Parallel()

	root, _ := projectRepositoryFixture(t)
	installProjectPlan(t, root, "delivery")

	stateDir := filepath.Join(root, ".sworn")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifestDir := filepath.Join(stateDir, "runs")
	if err := os.MkdirAll(manifestDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projectWriteManifest(t, manifestDir, "delivery.json", root, "delivery", "run-1")

	// Write docs/sworn/sworn.json
	projectConfigDir := filepath.Join(root, "docs", "sworn")
	if err := os.MkdirAll(projectConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}
	projectConfigJSON := `{
  "schema_version": "sworn.project-config/v1",
  "records_root": ".baton/records",
  "journals_root": ".sworn"
}`
	if err := os.WriteFile(filepath.Join(projectConfigDir, "sworn.json"), []byte(projectConfigJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	// Write operator.json
	operatorJSON := `{
  "schema_version": "sworn.operator-config/v1",
  "local": {
    "listen": "127.0.0.1:7337"
  },
  "otel": {
    "schema_version": "sworn.otel-config/v1",
    "endpoint": "http://127.0.0.1:4318",
    "headers": {}
  }
}`
	if err := os.WriteFile(filepath.Join(stateDir, "operator.json"), []byte(operatorJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	// Write drivers.json
	driverConfig := driver.DriverConfig{
		SchemaVersion: driver.DriverConfigSchemaVersion,
		Adapters: []driver.DriverAdapterConfig{
			{
				Process: &driver.DriverProcessAdapterConfig{
					Key:        "fixture-adapter",
					ID:         driver.FakeDriverID,
					Version:    driver.FakeDriverVersion,
					Executable: driver.ExecutableIdentity{Path: "/usr/bin/true", Digest: "sha256:" + stringOf('a', 64)},
				},
			},
		},
		Profiles: []driver.DriverProfile{
			{
				Key:                 "fixture",
				Adapter:             "fixture-adapter",
				Network:             driver.NetworkNone,
				CertificationModels: []string{"fixture-model"},
			},
		},
	}
	driverBytes, err := driver.EncodeDriverConfig(driverConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "drivers.json"), driverBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	backend := newProjectTUIBackend(root, "", "", "")
	cfg, err := backend.Config(context.Background())
	if err != nil {
		t.Fatalf("Config failed: %v", err)
	}

	// Operator listen & OTel
	if cfg.OperatorListen.Value != "127.0.0.1:7337" || cfg.OperatorListen.Source != filepath.Join(".sworn", "operator.json") {
		t.Fatalf("OperatorListen = %#v", cfg.OperatorListen)
	}
	if cfg.OperatorOTel.Value != "http://127.0.0.1:4318" || cfg.OperatorOTel.Source != filepath.Join(".sworn", "operator.json") {
		t.Fatalf("OperatorOTel = %#v", cfg.OperatorOTel)
	}

	// Records & paths
	if cfg.RecordsRoot.Value != ".baton/records" || cfg.RecordsRoot.Source != filepath.Join("docs", "sworn", "sworn.json") {
		t.Fatalf("RecordsRoot = %#v", cfg.RecordsRoot)
	}
	if cfg.JournalsRoot.Value != ".sworn" || cfg.JournalsRoot.Source != filepath.Join("docs", "sworn", "sworn.json") {
		t.Fatalf("JournalsRoot = %#v", cfg.JournalsRoot)
	}
	if cfg.JournalPath.Value != filepath.Join(root, ".sworn", "sworn.db") {
		t.Fatalf("JournalPath = %#v", cfg.JournalPath)
	}
	if cfg.ManifestDir.Value != filepath.Join(root, ".sworn", "runs") {
		t.Fatalf("ManifestDir = %#v", cfg.ManifestDir)
	}

	// Profiles from drivers.json
	if len(cfg.Profiles) != 1 || cfg.Profiles[0].Name != "fixture" || cfg.Profiles[0].Adapter != "fixture-adapter" {
		t.Fatalf("Profiles = %#v", cfg.Profiles)
	}

	// Role matrix from manifest
	if len(cfg.Roles) != 5 {
		t.Fatalf("Roles = %#v, want 5 entries", cfg.Roles)
	}
	manifestRelSource := filepath.Join(".sworn", "runs", "delivery.json")
	for _, r := range cfg.Roles {
		if r.Profile != "fixture" || r.Model != "fixture-model" || r.Source != manifestRelSource {
			t.Fatalf("Role entry unexpected = %#v", r)
		}
	}
}

func projectTUIPlan(t *testing.T, release string) []byte {
	t.Helper()
	metadata := baton.Metadata{
		SchemaVersion: baton.PlanVersion,
		Release:       release, Revision: 1, Repository: "fixture/sworn",
		TargetRef: "refs/heads/main", ApprovalRef: "fixture://approval/1",
		Tracks: []baton.Track{{
			ID: "T1", DependsOn: []string{},
			Slices: []baton.Slice{{
				ID: "S1", Outcome: "Prove project diagnostics close controls.",
				Scope:      baton.Scope{Include: []string{"README.md"}, Exclude: []string{}},
				Acceptance: []baton.Criterion{{ID: "AC-1", Text: "Start is unavailable."}},
				Checks:     []string{"go test ./cmd/sworn"}, Constraints: []string{},
				DependsOn: []string{}, Consumes: []string{},
			}},
		}},
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	return append(append([]byte("```baton-plan-v2\n"), body...), []byte("\n```\nFixture plan.\n")...)
}

func hasTUIDiagnostic(diagnostics []cockpit.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func withTUIAction(
	value cockpit.Action,
	change func(*cockpit.Action),
) cockpit.Action {
	change(&value)
	return value
}

func withTUIRun(value journal.Run, change func(*journal.Run)) journal.Run {
	change(&value)
	return value
}

func stringOf(value byte, count int) string {
	result := make([]byte, count)
	for index := range result {
		result[index] = value
	}
	return string(result)
}
