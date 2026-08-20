package tui

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/swornagent/sworn/internal/cockpit"
	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

const (
	refreshInterval = 2 * time.Second
	maxAnswerBytes  = journal.MaxAttentionAnswerBytes
)

type screen uint8

const (
	screenCatalog screen = iota
	screenBoard
	screenConfig
)

type overlay uint8

const (
	overlayNone overlay = iota
	overlayHelp
	overlayActions
	overlayConfirm
	overlayAnswer
)

type model struct {
	ctx     context.Context
	version string
	backend Backend

	screen         screen
	previousScreen screen
	overlay        overlay
	width          int
	height         int

	catalog       Catalog
	catalogCursor int
	selection     Selection
	board         Board
	nodeCursor    int
	actionCursor  int
	pendingAction cockpit.Action
	answer        string

	configView ConfigView
	configErr  string

	generation uint64
	loading    bool
	executing  bool
	errMsg     string
	statusMsg  string
}

type refreshDueMsg struct{ generation uint64 }

type catalogResultMsg struct {
	generation uint64
	catalog    Catalog
	err        error
}

type boardResultMsg struct {
	generation uint64
	selection  Selection
	board      Board
	err        error
}

type configResultMsg struct {
	generation uint64
	config     ConfigView
	err        error
}

type executeResultMsg struct {
	action cockpit.Action
	err    error
}

func newModel(ctx context.Context, version string, backend Backend) *model {
	return &model{
		ctx:        ctx,
		version:    version,
		backend:    backend,
		screen:     screenCatalog,
		generation: 1,
		loading:    true,
	}
}

func (m *model) Init() tea.Cmd {
	return catalogCmd(m.ctx, m.backend, m.generation)
}

func (m *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case refreshDueMsg:
		if msg.generation != m.generation {
			return m, nil
		}
		if m.overlay != overlayNone || m.loading {
			return m, scheduleRefresh(m.generation)
		}
		return m, m.beginRefresh()
	case catalogResultMsg:
		return m, m.acceptCatalog(msg)
	case boardResultMsg:
		return m, m.acceptBoard(msg)
	case configResultMsg:
		return m, m.acceptConfig(msg)
	case executeResultMsg:
		return m, m.acceptExecution(msg)
	case tea.KeyMsg:
		return m, m.handleKey(msg)
	default:
		return m, nil
	}
}

func (m *model) handleKey(key tea.KeyMsg) tea.Cmd {
	if key.String() == "ctrl+c" {
		return tea.Quit
	}
	if m.overlay == overlayAnswer {
		return m.handleAnswerKey(key)
	}
	if key.String() == "q" && m.overlay != overlayConfirm {
		return tea.Quit
	}

	switch m.overlay {
	case overlayHelp:
		if key.String() == "?" || key.String() == "esc" {
			m.overlay = overlayNone
		}
		return nil
	case overlayActions:
		return m.handleActionKey(key)
	case overlayConfirm:
		switch key.String() {
		case "y":
			return m.execute(m.pendingAction, m.answer)
		case "n", "esc":
			m.closeOverlay()
		}
		return nil
	}

	switch key.String() {
	case "?":
		m.overlay = overlayHelp
		return nil
	case "c":
		if m.screen == screenConfig {
			m.screen = m.previousScreen
			m.errMsg = ""
			m.statusMsg = ""
			return nil
		}
		m.previousScreen = m.screen
		m.screen = screenConfig
		m.errMsg = ""
		m.statusMsg = ""
		return m.beginConfig(true)
	case "r":
		if m.loading {
			return nil
		}
		return m.beginRefresh()
	}
	if m.screen == screenConfig {
		return m.handleConfigKey(key)
	}
	if m.screen == screenCatalog {
		return m.handleCatalogKey(key)
	}
	return m.handleBoardKey(key)
}

func (m *model) handleConfigKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc", "left":
		m.screen = m.previousScreen
		m.errMsg = ""
		m.statusMsg = ""
		return nil
	}
	return nil
}

func (m *model) handleCatalogKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "j", "down":
		if m.catalogCursor+1 < len(m.catalog.Entries) {
			m.catalogCursor++
		}
	case "k", "up":
		if m.catalogCursor > 0 {
			m.catalogCursor--
		}
	case "enter", "right":
		if len(m.catalog.Entries) == 0 {
			return nil
		}
		m.selection = m.catalog.Entries[m.catalogCursor].Selection
		m.board = Board{
			Selection: m.selection,
			Status:    "Opening release…",
			What:      "Checking the latest saved facts.",
		}
		m.screen = screenBoard
		m.nodeCursor = 0
		m.errMsg = ""
		m.statusMsg = ""
		return m.beginBoard(true)
	}
	return nil
}

func (m *model) handleBoardKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc", "left":
		m.screen = screenCatalog
		m.selection = Selection{}
		m.closeOverlay()
		return m.beginCatalog(true)
	case "j", "down":
		if m.nodeCursor+1 < len(m.board.Graph.Nodes) {
			m.nodeCursor++
		}
	case "k", "up":
		if m.nodeCursor > 0 {
			m.nodeCursor--
		}
	case "a":
		if !m.controlsAllowed() {
			return nil
		}
		if len(m.board.Actions) == 0 && (m.selection.RunID != "" || m.board.ManifestDir == "") {
			return nil
		}
		m.actionCursor = 0
		m.overlay = overlayActions
	}
	return nil
}

func (m *model) handleActionKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.closeOverlay()
	case "j", "down":
		if m.actionCursor+1 < len(m.board.Actions) {
			m.actionCursor++
		}
	case "k", "up":
		if m.actionCursor > 0 {
			m.actionCursor--
		}
	case "enter":
		if !m.controlsAllowed() || m.actionCursor >= len(m.board.Actions) {
			m.closeOverlay()
			return nil
		}
		action := m.board.Actions[m.actionCursor]
		if action.Kind == "answer_attention" || action.Kind == "start_delegated" ||
			action.Kind == "captain_delegation_replace" {
			m.pendingAction = action
			m.answer = ""
			m.overlay = overlayAnswer
			return nil
		}
		if confirmAction(action.Kind) {
			m.pendingAction = action
			m.overlay = overlayConfirm
			return nil
		}
		return m.execute(action, "")
	}
	return nil
}

func (m *model) handleAnswerKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "esc":
		m.closeOverlay()
		return nil
	case "ctrl+s":
		if strings.TrimSpace(m.answer) == "" {
			m.statusMsg = "Provide the required bounded text before continuing."
			return nil
		}
		if m.pendingAction.Kind == "answer_attention" {
			return m.execute(m.pendingAction, m.answer)
		}
		if _, err := runtimepkg.ParseCaptainDelegation([]byte(m.answer)); err != nil {
			m.statusMsg = "The Captain delegation envelope is not canonical."
			return nil
		}
		m.overlay = overlayConfirm
		return nil
	case "backspace":
		m.answer = trimLastRune(m.answer)
		return nil
	case "enter":
		m.appendAnswer("\n")
		return nil
	case "tab":
		m.appendAnswer("  ")
		return nil
	}
	if key.Type == tea.KeyRunes {
		m.appendAnswer(string(key.Runes))
	}
	return nil
}

func (m *model) appendAnswer(value string) {
	var clean strings.Builder
	for _, r := range value {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			clean.WriteRune(r)
		}
	}
	value = clean.String()
	limit := maxAnswerBytes
	if m.pendingAction.Kind == "start_delegated" ||
		m.pendingAction.Kind == "captain_delegation_replace" {
		limit = runtimepkg.MaxCaptainDelegationBytes
	}
	if len(m.answer)+len(value) > limit {
		m.statusMsg = "Input is at its safe size limit."
		return
	}
	m.answer += value
}

func trimLastRune(value string) string {
	if value == "" {
		return value
	}
	_, size := utf8.DecodeLastRuneInString(value)
	return value[:len(value)-size]
}

func confirmAction(kind string) bool {
	switch kind {
	case "start", "start_delegated", "cancel", "retry", "takeover", "approve",
		"captain_delegation_revoke", "captain_delegation_replace":
		return true
	default:
		return false
	}
}

func (m *model) controlsAllowed() bool {
	return m.screen == screenBoard && !m.executing &&
		m.board.Selection == m.selection
}

func (m *model) execute(action cockpit.Action, answer string) tea.Cmd {
	if !m.controlsAllowed() || !containsAction(m.board.Actions, action) {
		m.closeOverlay()
		return nil
	}
	m.closeOverlay()
	m.executing = true
	m.statusMsg = m.actionLabel(action) + "…"
	selection := m.selection
	return func() tea.Msg {
		err := m.backend.Execute(m.ctx, selection, action, answer)
		return executeResultMsg{action: action, err: err}
	}
}

func containsAction(actions []cockpit.Action, target cockpit.Action) bool {
	for _, action := range actions {
		if reflect.DeepEqual(action, target) {
			return true
		}
	}
	return false
}

func (m *model) closeOverlay() {
	m.overlay = overlayNone
	m.pendingAction = cockpit.Action{}
	m.answer = ""
}

func (m *model) beginRefresh() tea.Cmd {
	if m.screen == screenConfig {
		return m.beginConfig(false)
	}
	if m.screen == screenBoard {
		return m.beginBoard(false)
	}
	return m.beginCatalog(false)
}

func (m *model) beginCatalog(supersede bool) tea.Cmd {
	if m.loading && !supersede {
		return nil
	}
	m.generation++
	m.loading = true
	return catalogCmd(m.ctx, m.backend, m.generation)
}

func (m *model) beginBoard(supersede bool) tea.Cmd {
	if m.loading && !supersede {
		return nil
	}
	m.generation++
	m.loading = true
	return boardCmd(m.ctx, m.backend, m.generation, m.selection)
}

func (m *model) beginConfig(supersede bool) tea.Cmd {
	if m.loading && !supersede {
		return nil
	}
	m.generation++
	m.loading = true
	return configCmd(m.ctx, m.backend, m.generation)
}

func (m *model) finishRefresh() tea.Cmd {
	m.loading = false
	return scheduleRefresh(m.generation)
}

func (m *model) acceptCatalog(msg catalogResultMsg) tea.Cmd {
	if msg.generation != m.generation || m.screen != screenCatalog {
		return nil
	}
	if msg.err != nil {
		m.errMsg = "Could not refresh releases. Showing the last confirmed list."
		return m.finishRefresh()
	}
	selected := Selection{}
	previousCursor := m.catalogCursor
	if m.catalogCursor < len(m.catalog.Entries) {
		selected = m.catalog.Entries[m.catalogCursor].Selection
	}
	m.catalog = msg.catalog
	m.catalogCursor = catalogIndex(m.catalog.Entries, selected)
	if m.catalogCursor < 0 {
		m.catalogCursor = min(len(m.catalog.Entries)-1, max(0, previousCursor))
	}
	m.errMsg = ""
	return m.finishRefresh()
}

func (m *model) acceptBoard(msg boardResultMsg) tea.Cmd {
	if msg.generation != m.generation || m.screen != screenBoard ||
		msg.selection != m.selection {
		return nil
	}
	if msg.err != nil || msg.board.Selection != m.selection {
		if m.board.Selection != m.selection {
			m.board = Board{Selection: m.selection}
		}
		m.board.Stale = true
		m.errMsg = "Live updates paused. Showing the last confirmed board."
		return m.finishRefresh()
	}
	selectedNode := ""
	if m.nodeCursor < len(m.board.Graph.Nodes) {
		selectedNode = m.board.Graph.Nodes[m.nodeCursor].ID
	}
	m.board = msg.board
	m.nodeCursor = nodeIndex(m.board.Graph.Nodes, selectedNode)
	if m.nodeCursor < 0 {
		m.nodeCursor = 0
	}
	m.errMsg = ""
	return m.finishRefresh()
}

func (m *model) acceptConfig(msg configResultMsg) tea.Cmd {
	if msg.generation != m.generation || m.screen != screenConfig {
		return nil
	}
	if msg.err != nil {
		m.configErr = "Could not load project configuration."
		return m.finishRefresh()
	}
	m.configView = msg.config
	m.configErr = ""
	return m.finishRefresh()
}

func (m *model) acceptExecution(msg executeResultMsg) tea.Cmd {
	if !m.executing {
		return nil
	}
	m.executing = false
	if msg.err != nil {
		m.board.Stale = true
		m.errMsg = "That action was not accepted. Refreshing the current board."
	} else {
		m.statusMsg = m.actionLabel(msg.action) + " accepted."
	}
	if msg.err == nil && (msg.action.Kind == "start" || msg.action.Kind == "start_delegated") {
		m.screen = screenCatalog
		m.selection = Selection{}
		m.board = Board{}
		return m.beginCatalog(true)
	}
	return m.beginBoard(true)
}

func catalogCmd(
	ctx context.Context,
	backend Backend,
	generation uint64,
) tea.Cmd {
	return func() tea.Msg {
		catalog, err := backend.Catalog(ctx)
		return catalogResultMsg{generation: generation, catalog: catalog, err: err}
	}
}

func boardCmd(
	ctx context.Context,
	backend Backend,
	generation uint64,
	selection Selection,
) tea.Cmd {
	return func() tea.Msg {
		board, err := backend.Board(ctx, selection)
		return boardResultMsg{
			generation: generation, selection: selection, board: board, err: err,
		}
	}
}

func configCmd(
	ctx context.Context,
	backend Backend,
	generation uint64,
) tea.Cmd {
	return func() tea.Msg {
		config, err := backend.Config(ctx)
		return configResultMsg{generation: generation, config: config, err: err}
	}
}

func scheduleRefresh(generation uint64) tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg {
		return refreshDueMsg{generation: generation}
	})
}

func catalogIndex(entries []CatalogEntry, selection Selection) int {
	for index, entry := range entries {
		if entry.Selection == selection {
			return index
		}
	}
	if len(entries) == 0 {
		return 0
	}
	return -1
}

func nodeIndex(nodes []cockpit.Node, id string) int {
	for index, node := range nodes {
		if node.ID == id && id != "" {
			return index
		}
	}
	if len(nodes) == 0 {
		return 0
	}
	return -1
}

func (m *model) actionLabel(action cockpit.Action) string {
	switch action.Kind {
	case "start":
		return "Start run"
	case "start_delegated":
		return "Start with Captain delegation"
	case "captain_delegation_revoke":
		return "Revoke Captain delegation"
	case "captain_delegation_replace":
		return "Replace Captain delegation"
	case "approve":
		if action.Approval != nil {
			return fmt.Sprintf(
				"Approve plan revision %d",
				action.Approval.PlanRevision,
			)
		}
		return "Approve plan"
	case "pause":
		return "Pause safely"
	case "resume":
		return "Resume run"
	case "cancel":
		return "Cancel run"
	case "takeover":
		return "Take over run"
	case "retry":
		return "Retry work " + shortActionID(action.WorkID)
	case "answer_attention":
		for _, attention := range m.board.Attentions {
			if attention.ID == action.AttentionID &&
				attention.Generation == action.ExpectedGeneration {
				return "Answer: " + shortActionText(attention.Question, 44)
			}
		}
		return "Answer question " + shortActionID(action.AttentionID)
	case "redeliver":
		return "Retry notification to " + shortActionText(
			action.DestinationID,
			24,
		) + " · " + shortActionID(action.MessageID)
	default:
		return fmt.Sprintf("Run %s", safeText(action.Kind))
	}
}

func shortActionID(value string) string {
	value = strings.TrimPrefix(safeText(value), "sha256:")
	return shortActionText(value, 12)
}

func shortActionText(value string, limit int) string {
	value = safeText(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:max(1, limit-1)]) + "…"
}
