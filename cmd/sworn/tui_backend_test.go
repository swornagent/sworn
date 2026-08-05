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
