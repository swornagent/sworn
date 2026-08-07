package tui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/swornagent/sworn/internal/cockpit"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

const (
	defaultWidth  = 100
	defaultHeight = 30
	minimumWidth  = 20
	minimumHeight = 6
)

func (m *model) View() string {
	width, height := m.width, m.height
	if width <= 0 {
		width = defaultWidth
	}
	if height <= 0 {
		height = defaultHeight
	}
	width = max(minimumWidth, width)
	height = max(minimumHeight, height)
	bodyHeight := max(1, height-3)

	header := m.renderHeader(width)
	var body string
	if m.overlay != overlayNone {
		body = m.renderOverlay(width, bodyHeight)
	} else if m.screen == screenBoard {
		body = m.renderBoard(width, bodyHeight)
	} else {
		body = m.renderCatalog(width, bodyHeight)
	}
	body = fitBlock(body, bodyHeight)
	status := m.renderStatus(width)
	footer := m.renderFooter(width)
	return strings.Join([]string{header, body, status, footer}, "\n")
}

func (m *model) renderHeader(width int) string {
	location := "releases"
	connection := ""
	if m.screen == screenBoard {
		location = m.selection.Release
		switch {
		case m.board.Stale:
			connection = "  STALE"
		case m.loading && m.board.Status == "":
			connection = "  CHECKING"
		case m.selection.RunID == "":
			connection = "  CURRENT"
		default:
			connection = fmt.Sprintf("  LIVE · %d", m.board.ThroughOffset)
		}
	}
	content := fmt.Sprintf(
		" SWORN %s  /  %s%s",
		safeText(m.version), safeText(location), connection,
	)
	return headerStyle.Copy().Width(width).Render(truncate(content, width))
}

func (m *model) renderStatus(width int) string {
	message := m.statusMsg
	style := quietStyle
	if m.errMsg != "" {
		message = m.errMsg
		style = faultStyle
	} else if message == "" && m.loading &&
		((m.screen == screenCatalog && len(m.catalog.Entries) == 0) ||
			(m.screen == screenBoard && m.board.Status == "")) {
		message = "Checking the latest saved facts…"
	}
	return style.Copy().Width(width).Render(truncate(" "+message, width))
}

func (m *model) renderFooter(width int) string {
	content := " ? help   r refresh   q quit"
	if m.screen == screenCatalog {
		content = " ↑/k ↓/j move   enter open   ? help   q quit"
	} else {
		content = " ↑/k ↓/j move   a actions   esc releases   ? help   q quit"
	}
	if m.overlay == overlayAnswer {
		content = " ctrl+s send   enter newline   esc cancel"
	} else if m.overlay == overlayConfirm {
		content = " y confirm   n cancel"
	} else if m.overlay != overlayNone {
		content = " ↑/k ↓/j move   enter select   esc close"
	}
	return footerStyle.Copy().Width(width).Render(truncate(content, width))
}

func (m *model) renderCatalog(width, height int) string {
	lines := []string{titleStyle.Render(truncate("RELEASES", width))}
	if len(m.catalog.Entries) == 0 {
		copy := "No Sworn releases found in this project."
		if m.loading {
			copy = "Loading project releases…"
		}
		lines = append(lines, "", quietStyle.Render(truncate(copy, width)))
		return strings.Join(lines, "\n")
	}

	rowBudget := max(1, height-3)
	start, end := cursorWindow(len(m.catalog.Entries), m.catalogCursor, rowBudget)
	for index := start; index < end; index++ {
		entry := m.catalog.Entries[index]
		parts := []string{entry.Selection.Release}
		if entry.Selection.RunID != "" {
			parts = append(parts, entry.Selection.RunID)
		}
		parts = append(parts, entry.Status)
		label := "  " + strings.Join(parts, " · ")
		if index == m.catalogCursor {
			label = "▸" + label[1:]
			lines = append(lines, selectedStyle.Copy().Width(width).Render(
				truncate(safeText(label), width),
			))
		} else {
			lines = append(lines, truncate(safeText(label), width))
		}
	}
	if m.catalogCursor < len(m.catalog.Entries) {
		entry := m.catalog.Entries[m.catalogCursor]
		lines = append(lines, quietStyle.Render(truncate(
			"Needs you: "+safeText(entry.NeedsYou), width,
		)))
		lines = append(lines, quietStyle.Render(truncate(
			"Checked: "+safeText(entry.Checked), width,
		)))
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderBoard(width, height int) string {
	title := m.selection.Release
	if m.selection.RunID != "" {
		title += " · " + m.selection.RunID
	}
	lines := []string{titleStyle.Render(truncate(
		safeText(title),
		width,
	))}
	for _, fact := range []struct{ label, value string }{
		{"Status", m.board.Status},
		{"Captain authority", m.board.CaptainAuthority},
		{"Now", m.board.What},
		{"Next", m.board.Next},
		{"Needs you", m.board.NeedsYou},
		{"Checked", m.board.Checked},
	} {
		lines = append(lines, wrapFact(fact.label, fact.value, width)...)
	}
	if len(m.board.Diagnostics) > 0 {
		lines = append(
			lines,
			wrapFact(
				"Review",
				diagnosticExplanation(m.board.Diagnostics[0].Code),
				width,
			)...,
		)
	}
	if len(lines) >= height {
		return strings.Join(lines[:height], "\n")
	}
	lines = append(lines, quietStyle.Render(strings.Repeat("─", width)))
	remaining := height - len(lines)
	if remaining <= 0 {
		return strings.Join(lines, "\n")
	}
	if width < 96 {
		lines = append(lines, m.renderNarrowBoard(width, remaining)...)
		return strings.Join(lines, "\n")
	}

	leftWidth := max(20, width*2/3)
	rightWidth := max(1, width-leftWidth-3)
	left := m.graphLines(leftWidth, remaining)
	right := m.detailLines(rightWidth, remaining)
	lines = append(lines, joinColumns(left, right, leftWidth, rightWidth, remaining)...)
	return strings.Join(lines, "\n")
}

func (m *model) renderNarrowBoard(width, height int) []string {
	if height <= 0 {
		return nil
	}
	graphHeight := height
	if height >= 8 {
		graphHeight = height * 2 / 3
	}
	lines := m.graphLines(width, graphHeight)
	if len(lines) < height && len(m.board.Graph.Nodes) > 0 {
		lines = append(lines, quietStyle.Render(strings.Repeat("─", width)))
		lines = append(lines, m.detailLines(width, height-len(lines))...)
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

func (m *model) graphLines(width, height int) []string {
	if height <= 0 {
		return nil
	}
	lines := []string{titleStyle.Render(truncate(fmt.Sprintf(
		"DELIVERY GRAPH · %d work items · %d links",
		len(m.board.Graph.Nodes), len(m.board.Graph.Edges),
	), width))}
	if len(m.board.Graph.Nodes) == 0 {
		return append(lines, quietStyle.Render(truncate("No work recorded yet.", width)))
	}
	rowBudget := max(0, height-1)
	start, end := cursorWindow(len(m.board.Graph.Nodes), m.nodeCursor, rowBudget)
	for index := start; index < end; index++ {
		node := m.board.Graph.Nodes[index]
		line := graphNodeLine(node)
		if index == m.nodeCursor {
			lines = append(lines, selectedStyle.Copy().Width(width).Render(
				truncate(line, width),
			))
		} else if node.HasBaton {
			lines = append(lines, batonStyle.Render(truncate(line, width)))
		} else {
			lines = append(lines, truncate(line, width))
		}
	}
	return lines
}

func graphNodeLine(node cockpit.Node) string {
	prefix := "  "
	switch node.Kind {
	case "release":
		prefix = "◆ "
	case "track":
		prefix = "├ "
	case "slice":
		prefix = "│  ├ "
	case "assembly":
		prefix = "└ "
	}
	parts := []string{safeText(node.Label), safeText(node.State)}
	if node.NextResponsibility != "" && node.NextResponsibility != "none" {
		parts = append(parts, "next "+safeText(node.NextResponsibility))
	}
	if node.HasBaton {
		parts = append(parts, "BATON")
	}
	return prefix + strings.Join(parts, " · ")
}

func (m *model) detailLines(width, height int) []string {
	if height <= 0 {
		return nil
	}
	lines := []string{swornStyle.Render(truncate("WORK DETAIL", width))}
	if m.nodeCursor >= len(m.board.Graph.Nodes) {
		return append(lines, quietStyle.Render(truncate("Select a work item.", width)))
	}
	node := m.board.Graph.Nodes[m.nodeCursor]
	lines = append(lines, truncate(safeText(node.Label), width))
	for _, fact := range []struct{ label, value string }{
		{"Kind", node.Kind},
		{"Status", node.State},
		{"Sworn", node.RuntimeState},
		{"Step", node.Stage},
		{"Result", node.Outcome},
		{"Next owner", node.NextResponsibility},
	} {
		if fact.value != "" {
			label := quietStyle.Render(fact.label + ": ")
			lines = append(lines, label+truncate(
				safeText(fact.value), max(0, width-lipgloss.Width(label)),
			))
		}
	}
	if node.HasBaton {
		lines = append(lines, batonStyle.Render(truncate("Baton handoff recorded", width)))
	}
	if m.board.Stale {
		lines = append(lines, faultStyle.Render(truncate("Controls disabled while stale", width)))
	} else if len(m.board.Actions) > 0 {
		lines = append(lines, swornStyle.Render(truncate(fmt.Sprintf(
			"a  %d available controls", len(m.board.Actions),
		), width)))
	}
	if len(m.board.Diagnostics) > 0 {
		lines = append(lines, faultStyle.Render(truncate(
			"Needs confirmation · review the saved release",
			width,
		)))
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

func (m *model) renderOverlay(width, height int) string {
	switch m.overlay {
	case overlayHelp:
		return m.renderHelp(width, height)
	case overlayActions:
		return m.renderActions(width, height)
	case overlayConfirm:
		return m.renderConfirmation(width)
	case overlayAnswer:
		return m.renderAnswer(width, height)
	default:
		return ""
	}
}

func (m *model) renderHelp(width, height int) string {
	lines := []string{
		titleStyle.Render("HELP"),
		"↑/k  move up", "↓/j  move down", "enter  open or select",
		"a  available run controls", "r  refresh saved facts",
		"esc  close or go back", "q  quit", "?  close help",
	}
	if height < len(lines) {
		lines = lines[:height]
	}
	for index := range lines {
		lines[index] = truncate(lines[index], width)
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderActions(width, height int) string {
	lines := []string{titleStyle.Render("AVAILABLE CONTROLS")}
	if m.board.Stale {
		return strings.Join(append(lines,
			faultStyle.Render("Refresh before changing this run.")), "\n")
	}
	for index, action := range m.board.Actions {
		line := "  " + m.actionLabel(action)
		if index == m.actionCursor {
			line = "▸" + line[1:]
			lines = append(lines, selectedStyle.Copy().Width(width).Render(
				truncate(line, width),
			))
		} else {
			lines = append(lines, truncate(line, width))
		}
	}
	if len(lines) > height {
		start, end := cursorWindow(len(lines)-1, m.actionCursor, height-1)
		lines = append(lines[:1], lines[1+start:1+end]...)
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderConfirmation(width int) string {
	if m.pendingAction.Kind == "approve" && m.pendingAction.Approval != nil {
		command := m.pendingAction.Approval
		lines := []string{
			titleStyle.Render("CONFIRM EXACT PLAN APPROVAL"),
			"",
		}
		lines = append(lines, wrapFact("Release", command.Release, width)...)
		lines = append(lines, wrapFact("Project", command.Project, width)...)
		lines = append(lines, wrapFact(
			"Revision", fmt.Sprintf("%d", command.PlanRevision), width,
		)...)
		lines = append(lines, wrapFact(
			"Decision class", command.DecisionClass, width,
		)...)
		lines = append(lines, wrapExactFact("Plan digest", command.PlanDigest, width)...)
		lines = append(lines, wrapExactFact("Target head", command.TargetHead, width)...)
		lines = append(lines, "",
			batonStyle.Render("y confirm")+"  "+quietStyle.Render("n cancel"))
		return strings.Join(lines, "\n")
	}
	if binding := m.pendingAction.CaptainDelegation; binding != nil {
		lines := []string{titleStyle.Render("CONFIRM CAPTAIN AUTHORITY"), ""}
		lines = append(lines, wrapFact("Action", binding.Action, width)...)
		lines = append(lines, wrapFact("Run", binding.RunID, width)...)
		lines = append(lines, wrapExactFact("Manifest digest", binding.ManifestDigest, width)...)
		lines = append(lines, wrapFact("Actor class", binding.ActorClass, width)...)
		lines = append(lines, wrapFact("External authorizer", binding.ActorAuthority, width)...)
		if binding.CurrentEpoch > 0 {
			lines = append(lines, wrapFact("Current epoch", fmt.Sprintf("%d", binding.CurrentEpoch), width)...)
			lines = append(lines, wrapExactFact("Current digest", binding.CurrentDigest, width)...)
		}
		if m.answer != "" {
			if admitted, err := runtimepkg.ParseCaptainDelegation([]byte(m.answer)); err == nil {
				lines = append(lines, wrapFact("New epoch", fmt.Sprintf("%d", admitted.Envelope.DelegationEpoch), width)...)
				lines = append(lines, wrapExactFact("New digest", admitted.Digest, width)...)
			}
		}
		lines = append(lines, "", batonStyle.Render("y confirm")+"  "+quietStyle.Render("n cancel"))
		return strings.Join(lines, "\n")
	}
	copy := fmt.Sprintf("Confirm: %s?", m.actionLabel(m.pendingAction))
	return strings.Join([]string{
		titleStyle.Render("CONFIRM ACTION"),
		"",
		truncate(copy, width),
		"",
		batonStyle.Render("y confirm") + "  " + quietStyle.Render("n cancel"),
	}, "\n")
}

func (m *model) renderAnswer(width, height int) string {
	if m.pendingAction.Kind == "start_delegated" ||
		m.pendingAction.Kind == "captain_delegation_replace" {
		lines := []string{titleStyle.Render("CAPTAIN DELEGATION ENVELOPE")}
		lines = append(lines, wrapText("Paste the exact canonical sworn.captain-delegation/v1 envelope. It will be parsed and rebound before confirmation.", width)...)
		lines = append(lines, "", swornStyle.Render("CANONICAL JSON"))
		answerLines := strings.Split(safeMultiline(m.answer)+"█", "\n")
		budget := max(1, height-len(lines)-2)
		if len(answerLines) > budget {
			answerLines = answerLines[len(answerLines)-budget:]
		}
		for _, line := range answerLines {
			lines = append(lines, truncate(line, width))
		}
		lines = append(lines, quietStyle.Render(fmt.Sprintf("%d / %d bytes · ctrl+s validates", len(m.answer), runtimepkg.MaxCaptainDelegationBytes)))
		return strings.Join(lines, "\n")
	}
	question := "Answer the saved question so this work can continue."
	for _, attention := range m.board.Attentions {
		if attention.ID == m.pendingAction.AttentionID &&
			attention.Generation == m.pendingAction.ExpectedGeneration {
			question = attention.Question
			break
		}
	}
	lines := []string{titleStyle.Render("YOUR ANSWER IS NEEDED")}
	lines = append(lines, wrapText(question, width)...)
	lines = append(lines, "", swornStyle.Render("YOUR ANSWER"))
	answerLines := strings.Split(safeMultiline(m.answer)+"█", "\n")
	budget := max(1, height-len(lines)-2)
	if len(answerLines) > budget {
		answerLines = answerLines[len(answerLines)-budget:]
	}
	for _, line := range answerLines {
		lines = append(lines, truncate(line, width))
	}
	lines = append(lines, quietStyle.Render(truncate(fmt.Sprintf(
		"%d / %d bytes · ctrl+s sends", len(m.answer), maxAnswerBytes,
	), width)))
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func wrapFact(label, value string, width int) []string {
	return wrapText(label+": "+safeText(value), width)
}

func wrapExactFact(label, value string, width int) []string {
	label = safeText(label) + ":"
	value = safeText(value)
	if width < 1 {
		return []string{label, value}
	}
	lines := []string{truncate(label, width)}
	runes := []rune(value)
	for len(runes) > 0 {
		count := min(width, len(runes))
		lines = append(lines, string(runes[:count]))
		runes = runes[count:]
	}
	return lines
}

func diagnosticExplanation(code string) string {
	switch code {
	case "TARGET_DIVERGED":
		return "The target history changed. Reconcile it before continuing."
	case "TRACK_REF_ABSENT":
		return "A track is ready for Sworn to prepare."
	case "STALE_INPUTS":
		return "Earlier work changed; this part needs its inputs refreshed."
	case "STALE_ASSEMBLY":
		return "The combined candidate needs to be rebuilt from current work."
	case "BATON_UNAVAILABLE":
		return "Sworn could not read the current release record."
	case "SWORN_UNAVAILABLE":
		return "Sworn could not read the saved run record."
	case "ATTENTIONS_TRUNCATED":
		return "Only the most recent saved questions fit on this board."
	case "OUTBOX_TRUNCATED":
		return "Only the most recent notifications fit on this board."
	default:
		return "Sworn found something in the saved release that needs review."
	}
}

func wrapText(value string, width int) []string {
	value = safeText(value)
	if value == "" {
		return []string{""}
	}
	words := strings.Fields(value)
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{words[0]}
	for _, word := range words[1:] {
		last := len(lines) - 1
		candidate := lines[last] + " " + word
		if lipgloss.Width(candidate) <= width {
			lines[last] = candidate
		} else {
			lines = append(lines, truncate(word, width))
		}
	}
	return lines
}

func joinColumns(
	left, right []string,
	leftWidth, rightWidth, height int,
) []string {
	lines := make([]string, 0, height)
	for index := 0; index < height; index++ {
		leftLine, rightLine := "", ""
		if index < len(left) {
			leftLine = left[index]
		}
		if index < len(right) {
			rightLine = right[index]
		}
		leftLine += strings.Repeat(" ", max(0, leftWidth-lipgloss.Width(leftLine)))
		line := leftLine + quietStyle.Render(" │ ") + rightLine
		lines = append(lines, line)
	}
	return lines
}

func cursorWindow(length, cursor, budget int) (int, int) {
	if length == 0 || budget <= 0 {
		return 0, 0
	}
	if budget >= length {
		return 0, length
	}
	cursor = min(length-1, max(0, cursor))
	start := max(0, cursor-budget/2)
	if start+budget > length {
		start = length - budget
	}
	return start, start + budget
}

func fitBlock(value string, height int) string {
	lines := strings.Split(value, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	var result strings.Builder
	used := 0
	for _, r := range value {
		runeWidth := lipgloss.Width(string(r))
		if used+runeWidth+1 > width {
			break
		}
		result.WriteRune(r)
		used += runeWidth
	}
	return result.String() + "…"
}

func safeText(value string) string {
	var result strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) {
			result.WriteRune(' ')
		} else {
			result.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(result.String()), " ")
}

func safeMultiline(value string) string {
	var result strings.Builder
	for _, r := range value {
		switch {
		case r == '\n':
			result.WriteRune(r)
		case r == '\t':
			result.WriteString("  ")
		case !unicode.IsControl(r):
			result.WriteRune(r)
		}
	}
	return result.String()
}
