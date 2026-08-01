package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/swornagent/sworn/internal/cockpit"
)

type executeCall struct {
	selection Selection
	action    cockpit.Action
	answer    string
}

type fakeBackend struct {
	catalog      Catalog
	boards       map[Selection]Board
	catalogErr   error
	boardErr     error
	executeErr   error
	catalogCalls int
	boardCall    []Selection
	executed     []executeCall
}

func (f *fakeBackend) Catalog(_ context.Context) (Catalog, error) {
	f.catalogCalls++
	return f.catalog, f.catalogErr
}

func (f *fakeBackend) Board(_ context.Context, selection Selection) (Board, error) {
	f.boardCall = append(f.boardCall, selection)
	return f.boards[selection], f.boardErr
}

func (f *fakeBackend) Execute(
	_ context.Context,
	selection Selection,
	action cockpit.Action,
	answer string,
) error {
	f.executed = append(f.executed, executeCall{
		selection: selection, action: action, answer: answer,
	})
	return f.executeErr
}

func TestCatalogNavigationOpensExactBoardAndMovesGraphSelection(t *testing.T) {
	first := Selection{Release: "release-1", RunID: "run-1", Source: "source-1"}
	second := Selection{Release: "release-2", RunID: "run-2", Source: "source-2"}
	backend := &fakeBackend{
		catalog: Catalog{Entries: []CatalogEntry{
			{Selection: first, Status: "Complete", NeedsYou: "No."},
			{Selection: second, Status: "Sworn is working", NeedsYou: "No."},
		}},
		boards: map[Selection]Board{
			second: boardFixture(second),
		},
	}
	m := loadedCatalogModel(t, backend)

	updateModel(t, m, runeKey('j'))
	if m.catalogCursor != 1 {
		t.Fatalf("catalog cursor = %d, want 1", m.catalogCursor)
	}
	boardLoad := updateModel(t, m, specialKey(tea.KeyEnter))
	if m.screen != screenBoard || m.selection != second || boardLoad == nil {
		t.Fatalf("board navigation = screen %d selection %#v cmd=%v", m.screen, m.selection, boardLoad != nil)
	}
	updateModel(t, m, boardLoad())
	if len(m.board.Graph.Nodes) != 2 || m.loading {
		t.Fatalf("board = %#v loading=%t", m.board.Graph, m.loading)
	}
	updateModel(t, m, specialKey(tea.KeyDown))
	if m.nodeCursor != 1 {
		t.Fatalf("node cursor = %d, want 1", m.nodeCursor)
	}
}

func TestOpeningAnotherReleaseNeverShowsThePreviousBoard(t *testing.T) {
	first := Selection{Release: "release-1", RunID: "run-1", Source: "source-1"}
	second := Selection{Release: "release-2", RunID: "run-2", Source: "source-2"}
	backend := &fakeBackend{catalog: Catalog{Entries: []CatalogEntry{
		{Selection: second},
	}}}
	m := loadedCatalogModel(t, backend)
	m.board = boardFixture(first)

	command := updateModel(t, m, specialKey(tea.KeyEnter))
	if command == nil || m.board.Selection != second ||
		len(m.board.Graph.Nodes) != 0 || m.board.Status != "Opening release…" {
		t.Fatalf("opening board leaked previous release: %#v", m.board)
	}
}

func TestStaleAsyncBoardResultCannotReplaceCurrentSelection(t *testing.T) {
	current := Selection{Release: "release", RunID: "run", Source: "new"}
	old := Selection{Release: "release", RunID: "run", Source: "old"}
	m := &model{
		ctx: context.Background(), backend: &fakeBackend{},
		screen: screenBoard, selection: current,
		board:      Board{Selection: current, Status: "Current"},
		generation: 7, loading: true,
	}

	updateModel(t, m, boardResultMsg{
		generation: 6,
		selection:  current,
		board:      Board{Selection: current, Status: "Old generation"},
	})
	updateModel(t, m, boardResultMsg{
		generation: 7,
		selection:  old,
		board:      Board{Selection: old, Status: "Old source"},
	})
	if m.board.Status != "Current" || !m.loading {
		t.Fatalf("stale result changed board: %#v loading=%t", m.board, m.loading)
	}
}

func TestActionOverlayExecutesOnlyTheExactAdmittedAction(t *testing.T) {
	selection := Selection{Release: "release", RunID: "run", Source: "source"}
	pause := cockpit.Action{Kind: "pause", ExpectedGeneration: 4}
	retry := cockpit.Action{
		Kind: "retry", ExpectedGeneration: 4,
		WorkID: "sha256:" + strings.Repeat("a", 64), ExpectedEpoch: 2,
	}
	backend, m := readyBoardModel(selection, pause, retry)

	updateModel(t, m, runeKey('a'))
	updateModel(t, m, runeKey('j'))
	updateModel(t, m, specialKey(tea.KeyEnter))
	if m.overlay != overlayConfirm || len(backend.executed) != 0 {
		t.Fatalf("retry did not require confirmation: overlay=%d calls=%d", m.overlay, len(backend.executed))
	}
	execute := updateModel(t, m, runeKey('y'))
	if execute == nil {
		t.Fatal("confirmation did not dispatch action")
	}
	updateModel(t, m, execute())
	if len(backend.executed) != 1 || backend.executed[0].action != retry ||
		backend.executed[0].selection != selection || backend.executed[0].answer != "" {
		t.Fatalf("executed = %#v, want exact retry", backend.executed)
	}

	m.loading = false
	forged := retry
	forged.ExpectedEpoch++
	if command := m.execute(forged, ""); command != nil || len(backend.executed) != 1 {
		t.Fatal("unadmitted action was dispatched")
	}
}

func TestSuccessfulStartReturnsToCatalogBeforeAdmittingNewRun(t *testing.T) {
	selection := Selection{Release: "release", Source: "release-only"}
	start := cockpit.Action{Kind: "start"}
	backend, m := readyBoardModel(selection, start)
	backend.catalog = Catalog{Entries: []CatalogEntry{{
		Selection: Selection{
			Release: "release",
			RunID:   "new-run",
			Source:  "new-run-source",
		},
	}}}

	updateModel(t, m, runeKey('a'))
	updateModel(t, m, specialKey(tea.KeyEnter))
	execute := updateModel(t, m, runeKey('y'))
	if execute == nil {
		t.Fatal("start confirmation did not dispatch action")
	}

	refreshCatalog := updateModel(t, m, execute())
	if m.screen != screenCatalog || m.selection != (Selection{}) || refreshCatalog == nil {
		t.Fatalf(
			"successful start = screen %d selection %#v cmd=%v",
			m.screen, m.selection, refreshCatalog != nil,
		)
	}
	message := refreshCatalog()
	if _, ok := message.(catalogResultMsg); !ok {
		t.Fatalf("start refresh message = %T, want catalogResultMsg", message)
	}
	if backend.catalogCalls != 1 || len(backend.boardCall) != 0 {
		t.Fatalf(
			"start refresh calls = catalog %d board %v, want catalog only",
			backend.catalogCalls, backend.boardCall,
		)
	}
}

func TestCancelConfirmationAndBoundedMultilineAttentionAnswer(t *testing.T) {
	selection := Selection{Release: "release", RunID: "run", Source: "source"}
	cancel := cockpit.Action{Kind: "cancel", ExpectedGeneration: 3}
	answer := cockpit.Action{
		Kind: "answer_attention", ExpectedGeneration: 1,
		AttentionID: "sha256:" + strings.Repeat("b", 64),
	}
	backend, m := readyBoardModel(selection, cancel, answer)
	m.board.Attentions = []cockpit.AttentionView{{
		ID: answer.AttentionID, Generation: 1, State: "open",
		Question: "Which approved option should this work use?",
	}}

	updateModel(t, m, runeKey('a'))
	updateModel(t, m, specialKey(tea.KeyEnter))
	if m.overlay != overlayConfirm {
		t.Fatalf("cancel overlay = %d, want confirmation", m.overlay)
	}
	updateModel(t, m, runeKey('n'))
	if len(backend.executed) != 0 {
		t.Fatal("cancel executed after declining confirmation")
	}

	updateModel(t, m, runeKey('a'))
	updateModel(t, m, runeKey('j'))
	updateModel(t, m, specialKey(tea.KeyEnter))
	if m.overlay != overlayAnswer || !strings.Contains(m.View(), "Which approved option") {
		t.Fatalf("answer view unavailable: overlay=%d\n%s", m.overlay, m.View())
	}
	updateModel(t, m, runesKey("Use option A"))
	updateModel(t, m, specialKey(tea.KeyEnter))
	updateModel(t, m, runesKey("because it is approved."))
	execute := updateModel(t, m, specialKey(tea.KeyCtrlS))
	if execute == nil {
		t.Fatal("bounded answer did not dispatch")
	}
	updateModel(t, m, execute())
	want := "Use option A\nbecause it is approved."
	if len(backend.executed) != 1 || backend.executed[0].action != answer ||
		backend.executed[0].answer != want {
		t.Fatalf("answer execution = %#v, want %q", backend.executed, want)
	}

	m.answer = strings.Repeat("x", maxAnswerBytes)
	m.appendAnswer("y")
	if len(m.answer) != maxAnswerBytes {
		t.Fatalf("answer grew beyond bound: %d", len(m.answer))
	}
}

func TestRiskyActionsRequireConfirmation(t *testing.T) {
	for _, kind := range []string{"start", "cancel", "retry", "takeover"} {
		if !confirmAction(kind) {
			t.Errorf("%s does not require confirmation", kind)
		}
	}
	for _, kind := range []string{"pause", "resume", "redeliver"} {
		if confirmAction(kind) {
			t.Errorf("%s unexpectedly requires confirmation", kind)
		}
	}
}

func TestRepeatedActionsHaveDistinctOperatorLabels(t *testing.T) {
	selection := Selection{Release: "release", RunID: "run", Source: "source"}
	_, m := readyBoardModel(selection)
	firstQuestion := cockpit.Action{
		Kind: "answer_attention", ExpectedGeneration: 1, AttentionID: "question-1",
	}
	secondQuestion := cockpit.Action{
		Kind: "answer_attention", ExpectedGeneration: 1, AttentionID: "question-2",
	}
	m.board.Attentions = []cockpit.AttentionView{
		{ID: "question-1", Generation: 1, Question: "Use the approved migration?"},
		{ID: "question-2", Generation: 1, Question: "Keep the existing API?"},
	}
	labels := []string{
		m.actionLabel(firstQuestion),
		m.actionLabel(secondQuestion),
		m.actionLabel(cockpit.Action{Kind: "retry", WorkID: "sha256:" + strings.Repeat("a", 64)}),
		m.actionLabel(cockpit.Action{Kind: "retry", WorkID: "sha256:" + strings.Repeat("b", 64)}),
		m.actionLabel(cockpit.Action{Kind: "redeliver", DestinationID: "audit", MessageID: "message-1"}),
		m.actionLabel(cockpit.Action{Kind: "redeliver", DestinationID: "audit", MessageID: "message-2"}),
	}
	for index := 0; index < len(labels); index += 2 {
		if labels[index] == labels[index+1] {
			t.Fatalf("action labels are ambiguous: %q", labels[index])
		}
	}
}

func TestSmallTerminalStaysBounded(t *testing.T) {
	selection := Selection{
		Release: strings.Repeat("release", 20),
		RunID:   strings.Repeat("run", 20),
		Source:  "source",
	}
	_, m := readyBoardModel(selection, cockpit.Action{Kind: "pause"})
	m.width, m.height = 20, 6
	m.board.Status = strings.Repeat("very long status ", 20)
	m.board.What = strings.Repeat("very long activity ", 20)

	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 6 {
		t.Fatalf("small view has %d lines, want 6\n%s", len(lines), view)
	}
	for _, line := range lines {
		if got := lipgloss.Width(line); got > 20 {
			t.Fatalf("line width = %d, want <=20: %q", got, line)
		}
	}
}

func TestStaleBoardShowsNoUsableControls(t *testing.T) {
	selection := Selection{Release: "release", RunID: "run", Source: "source"}
	action := cockpit.Action{Kind: "pause", ExpectedGeneration: 2}
	backend, m := readyBoardModel(selection, action)
	m.board.Stale = true
	m.width, m.height = 100, 30

	if command := updateModel(t, m, runeKey('a')); command != nil ||
		m.overlay != overlayNone || len(backend.executed) != 0 {
		t.Fatal("stale board exposed a control")
	}
	if !strings.Contains(m.View(), "Controls disabled while stale") {
		t.Fatalf("stale explanation missing:\n%s", m.View())
	}
}

func TestConfirmedControlsRemainUsableDuringBackgroundRefresh(t *testing.T) {
	selection := Selection{Release: "release", RunID: "run", Source: "source"}
	action := cockpit.Action{Kind: "pause", ExpectedGeneration: 2}
	_, m := readyBoardModel(selection, action)
	m.loading = true

	updateModel(t, m, runeKey('a'))
	if m.overlay != overlayActions {
		t.Fatal("background refresh hid the last confirmed controls")
	}
}

func TestEstablishedBoardRefreshDoesNotFlashCheckingState(t *testing.T) {
	selection := Selection{Release: "release", RunID: "run", Source: "source"}
	_, m := readyBoardModel(selection)
	m.loading = true
	m.width, m.height = 100, 30

	view := m.View()
	if strings.Contains(view, "CHECKING") ||
		strings.Contains(view, "Checking the latest saved facts") {
		t.Fatalf("background refresh flashed loading state:\n%s", view)
	}
}

func TestRefreshWhileLoadingIsIgnored(t *testing.T) {
	backend := &fakeBackend{catalog: Catalog{}}
	m := newModel(context.Background(), "test", backend)
	if command := updateModel(t, m, runeKey('r')); command != nil {
		t.Fatalf("refresh while loading returned a command")
	}
	next := updateModel(t, m, catalogResultMsg{
		generation: 1, catalog: Catalog{},
	})
	if next == nil || m.loading || m.generation != 1 {
		t.Fatalf("completed refresh = cmd %v loading %t generation %d",
			next != nil, m.loading, m.generation)
	}
}

func TestAutomaticRefreshWaitsWhileAnOperatorIsAnswering(t *testing.T) {
	selection := Selection{Release: "release", RunID: "run", Source: "source"}
	_, m := readyBoardModel(selection, cockpit.Action{Kind: "answer_attention"})
	m.overlay = overlayAnswer
	m.answer = "unfinished answer"

	next := updateModel(t, m, refreshDueMsg{generation: m.generation})
	if next == nil || m.loading || m.answer != "unfinished answer" ||
		m.overlay != overlayAnswer {
		t.Fatalf("answering refresh = cmd %v loading %t answer %q overlay %d",
			next != nil, m.loading, m.answer, m.overlay)
	}
}

func loadedCatalogModel(t *testing.T, backend *fakeBackend) *model {
	t.Helper()
	m := newModel(context.Background(), "test", backend)
	command := m.Init()
	if command == nil {
		t.Fatal("catalog init command is nil")
	}
	updateModel(t, m, command())
	return m
}

func readyBoardModel(
	selection Selection,
	actions ...cockpit.Action,
) (*fakeBackend, *model) {
	board := boardFixture(selection)
	board.Actions = append([]cockpit.Action(nil), actions...)
	backend := &fakeBackend{boards: map[Selection]Board{selection: board}}
	m := &model{
		ctx: context.Background(), version: "test", backend: backend,
		screen: screenBoard, selection: selection, board: board,
		generation: 1,
	}
	return backend, m
}

func boardFixture(selection Selection) Board {
	return Board{
		Selection: selection,
		Graph: cockpit.Graph{Nodes: []cockpit.Node{
			{ID: "release:" + selection.Release, Kind: "release", Label: selection.Release, State: "running"},
			{ID: "slice:S01", Kind: "slice", Label: "S01", State: "ready", NextResponsibility: "implementer", HasBaton: true},
		}},
		Status: "Sworn is working", What: "Carrying the next handoff.",
		Next: "Continue with ready work.", NeedsYou: "No.",
		Checked: "Latest saved facts.", ThroughOffset: 7,
	}
}

func updateModel(t *testing.T, m *model, message tea.Msg) tea.Cmd {
	t.Helper()
	updated, command := m.Update(message)
	if updated != m {
		t.Fatal("model identity changed")
	}
	return command
}

func runeKey(value rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{value}}
}

func runesKey(value string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
}

func specialKey(value tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: value}
}
