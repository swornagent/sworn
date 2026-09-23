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
	} else if m.screen == screenConfig {
		body = m.renderConfig(width, bodyHeight)
	} else if m.screen == screenBoard {
		body = m.renderBoard(width, bodyHeight)
	} else if m.screen == screenActivity {
		body = m.renderActivity(width, bodyHeight)
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
	if m.screen == screenConfig {
		location = "config"
	} else if m.screen == screenActivity {
		location = "activity"
		if m.activityNode != "" {
			location += " · " + m.activityNode
		}
	} else if m.screen == screenBoard {
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
			(m.screen == screenBoard && m.board.Status == "") ||
			(m.screen == screenConfig && len(m.configView.Roles) == 0 && len(m.configView.Profiles) == 0)) {
		message = "Checking the latest saved facts…"
	}
	return style.Copy().Width(width).Render(truncate(" "+message, width))
}

func (m *model) renderFooter(width int) string {
	content := " ? help   r refresh   q quit"
	if m.screen == screenCatalog {
		content = " ↑/k ↓/j move   enter open   c config   ? help   q quit"
	} else if m.screen == screenBoard {
		content = " ↑/k ↓/j move   a actions   v activity   c config   esc releases   ? help   q quit"
	} else if m.screen == screenActivity {
		content = " ↑/k ↓/j scroll   g/G top/bottom   r refresh   esc board   ? help   q quit"
	} else if m.screen == screenConfig {
		content = " esc back   r refresh   ? help   q quit"
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
		{"Lead authority", m.board.LeadAuthority},
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

func (m *model) renderActivity(width, height int) string {
	title := "WORKER ACTIVITY"
	if m.activityNode != "" {
		title += " \u00b7 " + m.activityNode
	}
	lines := []string{titleStyle.Render(truncate(safeText(title), width))}
	if width < 60 {
		lines = append(lines, quietStyle.Render(truncate("Narrow terminal: worker text truncated. Widen to see more.", width)))
	}
	if m.loading && len(m.activity.Turns) == 0 {
		lines = append(lines, "", quietStyle.Render(truncate("Loading worker activity\u2026", width)))
		return joinActivityLines(lines, height)
	}
	if len(m.activity.Turns) == 0 {
		lines = append(lines, "", quietStyle.Render(truncate("No worker turns recorded yet.", width)))
		lines = append(lines, quietStyle.Render(truncate("Turns appear here turn by turn while the worker works.", width)))
		return joinActivityLines(lines, height)
	}
	listBudget := max(1, (height-len(lines)-2)/2)
	if listBudget < 1 {
		listBudget = 1
	}
	start, end := cursorWindow(len(m.activity.Turns), m.activityCursor, listBudget)
	for index := start; index < end; index++ {
		turn := m.activity.Turns[index]
		line := activityTurnSummary(turn)
		if index == m.activityCursor {
			lines = append(lines, selectedStyle.Copy().Width(width).Render(truncate(line, width)))
		} else {
			lines = append(lines, truncate(line, width))
		}
	}
	if len(m.activity.Turns) > end-start {
		lines = append(lines, quietStyle.Render(truncate("Showing turns in order; scroll to see more.", width)))
	}
	lines = append(lines, quietStyle.Render(truncate("Checked update "+itoaOffset(m.activity.ThroughOffset), width)))
	if m.activityCursor < len(m.activity.Turns) {
		remaining := height - len(lines)
		if remaining > 1 {
			lines = append(lines, quietStyle.Render(strings.Repeat("\u2500", width)))
			details := activityTurnDetails(m.activity.Turns[m.activityCursor], width, remaining-1)
			lines = append(lines, details...)
		}
	}
	return joinActivityLines(lines, height)
}

func joinActivityLines(lines []string, height int) string {
	if len(lines) > height {
		lines = lines[:height]
	}
	return joinLines(lines)
}

func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}

func activityTurnSummary(turn cockpit.ActivityTurn) string {
	role := safeText(turn.Responsibility)
	if role == "" {
		role = safeText(turn.Role)
	}
	where := safeText(turn.Slice)
	if where == "" {
		where = safeText(turn.Track)
	}
	parts := ""
	if len(turn.Content) > 0 || len(turn.Results) > 0 {
		parts = " \u00b7 " + itoaOffset(int64(len(turn.Content))) + " text \u00b7 " + itoaOffset(int64(len(turn.Results))) + " tools"
	}
	label := where
	if label != "" {
		label = " \u00b7 " + label
	}
	return "turn " + itoaOffset(turn.Turn) + " \u00b7 " + role + label + parts
}

func activityTurnDetails(turn cockpit.ActivityTurn, width, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	var lines []string
	for _, part := range turn.Content {
		if len(lines) >= maxLines {
			break
		}
		head := safeText(part.Head)
		if part.Tail != "" && head != "" {
			head += " \u2026 " + safeText(part.Tail)
		} else if part.Tail != "" {
			head = safeText(part.Tail)
		}
		if head == "" {
			head = "(no text)"
		}
		label := safeText(string(part.Kind))
		if part.Tool != "" {
			label += " " + safeText(part.Tool)
		}
		line := label + ": " + head
		if part.OmittedBytes > 0 || part.RedactedBytes > 0 {
			line += " (+" + itoaOffset(part.OmittedBytes) + " omitted, +" + itoaOffset(part.RedactedBytes) + " redacted)"
		}
		lines = append(lines, truncate(line, width))
	}
	for _, result := range turn.Results {
		if len(lines) >= maxLines {
			break
		}
		state := "pass"
		if result.Failed {
			state = "fail"
		}
		head := safeText(result.Head)
		if head == "" && result.Tail != "" {
			head = safeText(result.Tail)
		}
		line := safeText(result.Tool) + " " + state + " " + itoaOffset(result.TotalBytes) + " bytes"
		if result.OmittedBytes > 0 || result.RedactedBytes > 0 {
			line += " (+" + itoaOffset(result.OmittedBytes) + " omitted, +" + itoaOffset(result.RedactedBytes) + " redacted)"
		}
		if head != "" {
			line += ": " + head
		}
		lines = append(lines, truncate(line, width))
	}
	if turn.DroppedEvents > 0 && len(lines) < maxLines {
		lines = append(lines, truncate("+"+itoaOffset(turn.DroppedEvents)+" dropped events", width))
	}
	return lines
}

func itoaOffset(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var digits [32]byte
	pos := len(digits)
	for value > 0 {
		pos--
		digits[pos] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		pos--
		digits[pos] = '-'
	}
	return string(digits[pos:])
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
		} else if node.HasProtocol {
			lines = append(lines, protocolStyle.Render(truncate(line, width)))
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
	if node.HasProtocol {
		parts = append(parts, "HANDOFF")
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
	if node.HasProtocol {
		lines = append(lines, protocolStyle.Render(truncate("Handoff recorded", width)))
	}
	if len(m.board.Actions) > 0 {
		lines = append(lines, swornStyle.Render(truncate(fmt.Sprintf(
			"a  %d available controls", len(m.board.Actions),
		), width)))
	} else if m.selection.RunID == "" && m.board.ManifestDir != "" {
		lines = append(lines, quietStyle.Render(truncate(
			"No run definition in "+m.board.ManifestDir,
			width,
		)))
	}
	if len(m.board.Diagnostics) > 0 {
		lines = append(lines, faultStyle.Render(truncate(
			"Needs confirmation · review the saved release",
			width,
		)))
	}
	if node.FailureTurnContext != nil {
		remaining := height - len(lines)
		lines = append(lines, failureContextLines(node.FailureTurnContext, width, remaining)...)
	}
	if node.HostCheckFailure != nil {
		remaining := height - len(lines)
		lines = append(lines, hostCheckFailureLines(node.HostCheckFailure, width, remaining)...)
	}
	if node.ProviderStall != nil {
		remaining := height - len(lines)
		lines = append(lines, providerStallLines(node.ProviderStall, width, remaining)...)
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return lines
}

func failureContextLines(ctx *runtimepkg.FailureTurnContext, width, budget int) []string {
	if ctx == nil || budget <= 0 {
		return nil
	}
	if ctx.SchemaVersion != "sworn.failure-turn-context/v1" {
		return nil
	}
	if ctx.Status != "present" && ctx.Status != "empty" && ctx.Status != "unavailable" {
		return nil
	}
	var all []string
	all = append(all, swornStyle.Render(truncate("FAILURE TURNS", width)))
	if width < 60 && ctx.Status == "present" && len(ctx.Turns) > 0 {
		all = append(all, quietStyle.Render(truncate("Narrow terminal: worker text truncated. Widen to see more.", width)))
	}
	switch ctx.Status {
	case "empty":
		all = append(all, quietStyle.Render(truncate("No worker turns recorded (failed before any turn).", width)))
	case "unavailable":
		reason := safeText(ctx.Reason)
		if reason == "" {
			reason = "unknown reason"
		}
		all = append(all, quietStyle.Render(truncate("Worker turns unavailable: "+reason+".", width)))
	case "present":
		count := len(ctx.Turns)
		status := ""
		if count == 1 {
			status = "1 turn"
		} else {
			status = itoaOffset(int64(count)) + " turns"
		}
		status += ", " + itoaOffset(ctx.Omitted) + " omitted, " + itoaOffset(ctx.DroppedMaxVisible) + " dropped (max visible)."
		all = append(all, quietStyle.Render(truncate(status, width)))
		for _, turn := range ctx.Turns {
			header := "turn " + itoaOffset(turn.Turn) + " " + safeText(turn.Kind)
			if turn.Parts > 0 {
				header += " part " + itoaOffset(turn.Part) + "/" + itoaOffset(turn.Parts)
			}
			if turn.OmittedParts > 0 {
				header += " +" + itoaOffset(turn.OmittedParts) + " parts omitted to fit"
			}
			all = append(all, truncate(header, width))
			for _, part := range turn.Content {
				body := safeText(part.Head)
				if tail := safeText(part.Tail); tail != "" {
					if body != "" {
						body += " … " + tail
					} else {
						body = tail
					}
				}
				if body == "" {
					body = "(no text)"
				}
				label := safeText(part.Kind)
				if label == "" {
					label = "text"
				}
				if part.Tool != "" {
					label += " " + safeText(part.Tool)
				}
				line := label + ": " + body
				all = append(all, truncate(line, width))
				meta := itoaOffset(part.TotalBytes) + " bytes · +" + itoaOffset(part.OmittedBytes) + " omitted, +" + itoaOffset(part.RedactedBytes) + " redacted"
				all = append(all, quietStyle.Render(truncate(meta, width)))
			}
			for _, result := range turn.Results {
				verdict := "pass"
				if result.Failed {
					verdict = "fail"
				}
				line := safeText(result.Tool) + " " + verdict + " " + itoaOffset(result.TotalBytes) + " bytes"
				if result.OmittedBytes > 0 || result.RedactedBytes > 0 {
					line += " (+" + itoaOffset(result.OmittedBytes) + " omitted, +" + itoaOffset(result.RedactedBytes) + " redacted)"
				}
				all = append(all, truncate(line, width))
				body := safeText(result.Head)
				if tail := safeText(result.Tail); tail != "" {
					if body != "" {
						body += " … " + tail
					} else {
						body = tail
					}
				}
				if body != "" {
					all = append(all, quietStyle.Render(truncate(body, width)))
				}
			}
			if turn.DroppedEvents > 0 {
				all = append(all, quietStyle.Render(truncate("+"+itoaOffset(turn.DroppedEvents)+" dropped events", width)))
			}
		}
	}
	if len(all) > budget {
		omitted := len(all) - (budget - 1)
		all = all[:budget-1]
		all = append(all, quietStyle.Render(truncate("+"+itoaOffset(int64(omitted))+" more", width)))
	}
	return all
}

func hostCheckFailureLines(fact *runtimepkg.HostCheckFailureFact, width, budget int) []string {
	if fact == nil || budget <= 0 {
		return nil
	}
	if fact.SchemaVersion != "sworn.host-check-failure-fact/v1" {
		return nil
	}
	var all []string
	all = append(all, swornStyle.Render(truncate("HOST CHECK FAILURE", width)))
	if width < 60 {
		all = append(all, quietStyle.Render(truncate("Narrow terminal: check output truncated. Widen to see more.", width)))
	}
	all = append(all, truncate("check: "+safeText(fact.Check), width))
	outcome := "outcome: " + safeText(fact.Outcome) + " (exit " + itoaOffset(int64(fact.ExitCode)) + ")"
	all = append(all, truncate(outcome, width))
	if fact.Reran {
		rerun := "re-executed: yes"
		if fact.RerunOutcome != "" {
			rerun += " (" + safeText(fact.RerunOutcome)
			if fact.RerunExitCode != nil {
				rerun += " exit " + itoaOffset(int64(*fact.RerunExitCode))
			}
			rerun += ")"
		}
		all = append(all, truncate(rerun, width))
	} else {
		all = append(all, truncate("re-executed: no", width))
	}
	switch {
	case fact.NotRunUnknown:
		all = append(all, quietStyle.Render(truncate("not run: unknown (contract unavailable)", width)))
	case len(fact.NotRun) == 0:
		all = append(all, quietStyle.Render(truncate("not run: none", width)))
	default:
		joined := strings.Join(fact.NotRun, ", ")
		all = append(all, truncate("not run: "+safeText(joined), width))
	}
	excerptLabel := "output excerpt:"
	if fact.ExcerptTruncated {
		excerptLabel += " (truncated)"
	}
	all = append(all, quietStyle.Render(truncate(excerptLabel, width)))
	if fact.Excerpt == "" {
		all = append(all, quietStyle.Render(truncate("(no output)", width)))
	} else {
		for _, line := range strings.Split(fact.Excerpt, "\n") {
			all = append(all, quietStyle.Render(truncate(safeText(line), width)))
		}
	}
	if len(all) > budget {
		omitted := len(all) - (budget - 1)
		all = all[:budget-1]
		all = append(all, quietStyle.Render(truncate("+"+itoaOffset(int64(omitted))+" more", width)))
	}
	return all
}

// providerStallLines renders S5's live wait status: a work is still inside
// its bounded backoff, not hung and not yet a park.
func providerStallLines(stall *runtimepkg.ProviderStallStatus, width, budget int) []string {
	if stall == nil || budget <= 0 {
		return nil
	}
	var all []string
	all = append(all, swornStyle.Render(truncate("PROVIDER STALL: WAITING", width)))
	if stall.FailureCode != "" {
		all = append(all, truncate("failure: "+safeText(stall.FailureCode), width))
	}
	all = append(all, truncate(
		"next probe: "+stall.NextProbeAt.UTC().Format("2006-01-02T15:04:05Z"), width,
	))
	elapsed := "elapsed: " + itoaOffset(stall.ElapsedMillis/1000) +
		"s of " + itoaOffset(stall.BoundMillis/1000) + "s bound"
	all = append(all, truncate(elapsed, width))
	if stall.LastProbeCode != "" {
		all = append(all, truncate("last probe: "+safeText(stall.LastProbeCode), width))
	} else {
		all = append(all, quietStyle.Render(truncate("last probe: none yet", width)))
	}
	if len(all) > budget {
		omitted := len(all) - (budget - 1)
		all = all[:budget-1]
		all = append(all, quietStyle.Render(truncate("+"+itoaOffset(int64(omitted))+" more", width)))
	}
	return all
}

func (m *model) renderConfig(width, height int) string {
	lines := []string{titleStyle.Render(truncate("PROJECT CONFIGURATION", width))}
	if m.configErr != "" {
		lines = append(lines, "", faultStyle.Render(truncate(m.configErr, width)))
		return strings.Join(lines, "\n")
	}
	if m.loading && len(m.configView.Roles) == 0 && len(m.configView.Profiles) == 0 {
		lines = append(lines, "", quietStyle.Render(truncate("Loading configuration…", width)))
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "")
	lines = append(lines, swornStyle.Render(truncate("ROLE MATRIX", width)))
	if len(m.configView.Roles) == 0 {
		lines = append(lines, quietStyle.Render(truncate("  No roles configured yet.", width)))
	} else {
		for _, role := range m.configView.Roles {
			label := fmt.Sprintf("  %-12s %s / %s", safeText(role.Role), safeText(role.Profile), safeText(role.Model))
			if role.Source != "" {
				label += "  " + quietStyle.Render("("+safeText(role.Source)+")")
			}
			lines = append(lines, truncate(label, width))
		}
	}

	if len(m.configView.Profiles) > 0 {
		lines = append(lines, "")
		lines = append(lines, swornStyle.Render(truncate("PROFILES", width)))
		for _, profile := range m.configView.Profiles {
			label := fmt.Sprintf("  %-12s %s (net: %s)", safeText(profile.Name), safeText(profile.Adapter), safeText(profile.Network))
			if profile.Source != "" {
				label += "  " + quietStyle.Render("("+safeText(profile.Source)+")")
			}
			lines = append(lines, truncate(label, width))
		}
	}

	lines = append(lines, "")
	lines = append(lines, swornStyle.Render(truncate("OPERATOR SERVICE", width)))
	lines = append(lines, m.renderConfigItem("Listen", m.configView.OperatorListen, width)...)
	lines = append(lines, m.renderConfigItem("Telemetry", m.configView.OperatorOTel, width)...)

	lines = append(lines, "")
	lines = append(lines, swornStyle.Render(truncate("STORAGE & PATHS", width)))
	lines = append(lines, m.renderConfigItem("Records root", m.configView.RecordsRoot, width)...)
	lines = append(lines, m.renderConfigItem("Journals root", m.configView.JournalsRoot, width)...)
	lines = append(lines, m.renderConfigItem("Journal path", m.configView.JournalPath, width)...)
	lines = append(lines, m.renderConfigItem("Manifests dir", m.configView.ManifestDir, width)...)
	if m.configView.DriverConfig.Value != "" {
		lines = append(lines, m.renderConfigItem("Drivers config", m.configView.DriverConfig, width)...)
	}

	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderConfigItem(label string, item ConfigItem, width int) []string {
	value := item.Value
	if value == "" {
		value = "(not configured)"
	}
	text := fmt.Sprintf("  %-14s %s", label+":", safeText(value))
	if item.Source != "" {
		text += "  " + quietStyle.Render("("+safeText(item.Source)+")")
	}
	return []string{truncate(text, width)}
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
		"a  available run controls", "v  worker activity", "c  project configuration",
		"r  refresh saved facts", "esc  close or go back",
		"q  quit", "?  close help",
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
	if len(m.board.Actions) == 0 {
		msg := "No controls available."
		if m.selection.RunID == "" && m.board.ManifestDir != "" {
			msg = fmt.Sprintf("No run definition found in %s. Provide a run manifest before starting delivery.", m.board.ManifestDir)
		}
		lines = append(lines, "")
		for _, wrapped := range wrapText(msg, width) {
			lines = append(lines, quietStyle.Render(wrapped))
		}
		if len(lines) > height {
			lines = lines[:height]
		}
		return strings.Join(lines, "\n")
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
			protocolStyle.Render("y confirm")+"  "+quietStyle.Render("n cancel"))
		return strings.Join(lines, "\n")
	}
	if binding := m.pendingAction.LeadDelegation; binding != nil {
		lines := []string{titleStyle.Render("CONFIRM LEAD AUTHORITY"), ""}
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
			if admitted, err := runtimepkg.ParseLeadDelegation([]byte(m.answer)); err == nil {
				lines = append(lines, wrapFact("New epoch", fmt.Sprintf("%d", admitted.Envelope.DelegationEpoch), width)...)
				lines = append(lines, wrapExactFact("New digest", admitted.Digest, width)...)
			}
		}
		lines = append(lines, "", protocolStyle.Render("y confirm")+"  "+quietStyle.Render("n cancel"))
		return strings.Join(lines, "\n")
	}
	copy := fmt.Sprintf("Confirm: %s?", m.actionLabel(m.pendingAction))
	return strings.Join([]string{
		titleStyle.Render("CONFIRM ACTION"),
		"",
		truncate(copy, width),
		"",
		protocolStyle.Render("y confirm") + "  " + quietStyle.Render("n cancel"),
	}, "\n")
}

func (m *model) renderAnswer(width, height int) string {
	if m.pendingAction.Kind == "start_delegated" ||
		m.pendingAction.Kind == "lead_delegation_replace" {
		lines := []string{titleStyle.Render("LEAD DELEGATION ENVELOPE")}
		lines = append(lines, wrapText("Paste the exact canonical sworn.lead-delegation/v1 envelope. It will be parsed and rebound before confirmation.", width)...)
		lines = append(lines, "", swornStyle.Render("CANONICAL JSON"))
		answerLines := strings.Split(safeMultiline(m.answer)+"█", "\n")
		budget := max(1, height-len(lines)-2)
		if len(answerLines) > budget {
			answerLines = answerLines[len(answerLines)-budget:]
		}
		for _, line := range answerLines {
			lines = append(lines, truncate(line, width))
		}
		lines = append(lines, quietStyle.Render(fmt.Sprintf("%d / %d bytes · ctrl+s validates", len(m.answer), runtimepkg.MaxLeadDelegationBytes)))
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
	case "PROTOCOL_UNAVAILABLE":
		return "Sworn could not read the current release record."
	case "SWORN_UNAVAILABLE":
		return "Sworn could not read the saved run record."
	case "ATTENTIONS_TRUNCATED":
		return "Only the most recent saved questions fit on this board."
	case "OUTBOX_TRUNCATED":
		return "Only the most recent notifications fit on this board."
	case "PLAN_NOT_FOUND":
		return "The release plan could not be found. Commit an approved plan before starting delivery."
	case "INVALID_PLAN_FENCE":
		return "The release plan format or version is not recognized. Format the plan with ```protocol-plan-v2 before continuing."
	case "REF_NOT_FOUND":
		return "The release reference could not be found. Check that the release branch exists in the repository."
	case "INVALID_HEAD_OBJECT":
		return "The release commit object is invalid. Check that the release commit is present in the repository."
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
