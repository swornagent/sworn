package cockpit

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultTerminalWidth = 80
	minimumTerminalWidth = 20
)

// RenderTerminal renders a Snapshot as plain, deterministic terminal text.
//
// It intentionally emits no ANSI control sequences. Snapshot strings are
// quoted before rendering, so untrusted labels and identifiers cannot inject
// terminal controls. Use RenderTerminalWidth when the available width is
// known.
func RenderTerminal(snapshot Snapshot) string {
	return RenderTerminalWidth(snapshot, DefaultTerminalWidth)
}

// RenderTerminalWidth renders a Snapshot using the requested line width.
// Non-positive widths select DefaultTerminalWidth. Impractically small widths
// are raised to the minimum needed to keep the facts readable.
func RenderTerminalWidth(snapshot Snapshot, width int) string {
	if width <= 0 {
		width = DefaultTerminalWidth
	}
	if width < minimumTerminalWidth {
		width = minimumTerminalWidth
	}

	renderer := terminalRenderer{width: width}
	presentation := PresentSnapshot(snapshot)
	renderer.heading("SWORN DELIVERY BOARD")
	renderer.section("SUMMARY")
	renderer.line(2, "Status: "+presentation.Status)
	renderer.line(2, "What's happening: "+presentation.What)
	renderer.line(2, "Next: "+presentation.Next)
	renderer.line(2, "Needs you: "+presentation.NeedsYou)
	renderer.line(2, "Checked: "+presentation.Checked)

	renderer.section("TECHNICAL DETAILS")
	renderer.section("RUN")
	renderer.line(
		2,
		"schema="+terminalQuote(snapshot.SchemaVersion),
	)
	renderer.line(
		2,
		fields(
			"id="+terminalQuote(snapshot.Run.ID),
			"release="+terminalQuote(snapshot.Run.Release),
		),
	)
	renderer.line(
		2,
		fields(
			"state="+terminalQuote(snapshot.Run.State),
			"desired="+terminalQuote(snapshot.Run.DesiredState),
			"outcome="+terminalQuote(snapshot.Run.Outcome),
		),
	)
	renderer.line(
		2,
		fields(
			"control_generation="+strconv.FormatInt(
				snapshot.Run.ControlGeneration,
				10,
			),
			"through_offset="+strconv.FormatInt(snapshot.ThroughOffset, 10),
		),
	)
	renderer.line(
		2,
		fields(
			"target_ref="+terminalQuote(snapshot.Run.TargetRef),
			"target_head="+terminalQuote(snapshot.Run.TargetHead),
			"release_head="+terminalQuote(snapshot.Run.ReleaseHead),
		),
	)
	renderer.line(
		2,
		fields(
			"manifest="+terminalQuote(snapshot.Run.ManifestDigest),
			"plan="+terminalQuote(snapshot.Run.PlanDigest),
		),
	)

	renderer.section(
		fmt.Sprintf(
			"GRAPH nodes=%d edges=%d",
			len(snapshot.Graph.Nodes),
			len(snapshot.Graph.Edges),
		),
	)
	if len(snapshot.Graph.Nodes) == 0 {
		renderer.none()
	}
	for _, node := range snapshot.Graph.Nodes {
		nodeFields := []string{
			terminalQuote(node.Kind),
			"id=" + terminalQuote(node.ID),
			"label=" + terminalQuote(node.Label),
			"state=" + terminalQuote(node.State),
		}
		if node.Track != "" {
			nodeFields = append(
				nodeFields,
				"track="+terminalQuote(node.Track),
			)
		}
		if node.RuntimeState != "" {
			nodeFields = append(
				nodeFields,
				"runtime="+terminalQuote(node.RuntimeState),
			)
		}
		if node.Stage != "" {
			nodeFields = append(
				nodeFields,
				"stage="+terminalQuote(node.Stage),
			)
		}
		if node.Outcome != "" {
			nodeFields = append(
				nodeFields,
				"outcome="+terminalQuote(node.Outcome),
			)
		}
		if node.Attempt != 0 {
			nodeFields = append(
				nodeFields,
				"attempt="+strconv.FormatInt(node.Attempt, 10),
			)
		}
		nodeFields = append(
			nodeFields,
			"baton="+strconv.FormatBool(node.HasBaton),
		)
		if node.NextResponsibility != "" {
			nodeFields = append(
				nodeFields,
				"next="+terminalQuote(node.NextResponsibility),
			)
		}
		renderer.line(2, fields(nodeFields...))
	}
	for _, edge := range snapshot.Graph.Edges {
		renderer.line(
			2,
			fields(
				"edge",
				"id="+terminalQuote(edge.ID),
				"from="+terminalQuote(edge.From),
				"kind="+terminalQuote(edge.Kind),
				"to="+terminalQuote(edge.To),
			),
		)
	}

	renderer.section("HANDOFF")
	renderer.line(
		2,
		fields(
			"ready="+strconv.FormatBool(snapshot.Handoff.Ready),
			"nodes="+terminalList(snapshot.Handoff.Nodes),
			"responsibilities="+terminalList(
				snapshot.Handoff.Responsibilities,
			),
		),
	)

	renderer.section("OWNER")
	renderer.line(
		2,
		fields(
			"present="+strconv.FormatBool(snapshot.Runtime.Owner.Present),
			"active="+strconv.FormatBool(snapshot.Runtime.Owner.Active),
			"generation="+strconv.FormatInt(
				snapshot.Runtime.Owner.Generation,
				10,
			),
			"expires_at="+terminalTime(snapshot.Runtime.Owner.ExpiresAt),
		),
	)

	renderer.section(
		fmt.Sprintf("EFFECTS count=%d", len(snapshot.Runtime.Effects)),
	)
	if len(snapshot.Runtime.Effects) == 0 {
		renderer.none()
	}
	for _, effect := range snapshot.Runtime.Effects {
		renderer.line(
			2,
			fields(
				"id="+terminalQuote(effect.ID),
				"kind="+terminalQuote(effect.Kind),
				"state="+terminalQuote(effect.State),
				"error="+terminalQuote(effect.ErrorCode),
			),
		)
	}

	renderer.section(
		fmt.Sprintf("ATTEMPTS count=%d", len(snapshot.Runtime.Attempts)),
	)
	if len(snapshot.Runtime.Attempts) == 0 {
		renderer.none()
	}
	for _, attempt := range snapshot.Runtime.Attempts {
		renderer.line(
			2,
			fields(
				"effect="+terminalQuote(attempt.EffectID),
				"number="+strconv.FormatInt(attempt.Number, 10),
				"responsibility="+terminalQuote(
					attempt.Responsibility,
				),
				"transport="+terminalQuote(attempt.Transport),
				"input_tokens="+terminalOptionalInt(attempt.InputTokens),
				"output_tokens="+terminalOptionalInt(attempt.OutputTokens),
				"cost_micro_units="+terminalOptionalInt(
					attempt.CostMicroUnits,
				),
				"currency="+terminalOptionalString(attempt.Currency),
				"created_at="+terminalTime(attempt.CreatedAt),
			),
		)
	}

	renderer.section(
		fmt.Sprintf(
			"ATTENTIONS count=%d truncated=%t",
			len(snapshot.Runtime.Attentions),
			snapshot.Runtime.AttentionsTruncated,
		),
	)
	if len(snapshot.Runtime.Attentions) == 0 {
		renderer.none()
	}
	for _, attention := range snapshot.Runtime.Attentions {
		renderer.line(
			2,
			fields(
				"id="+terminalQuote(attention.ID),
				"lane="+terminalQuote(attention.LaneID),
				"state="+terminalQuote(attention.State),
				"generation="+strconv.FormatInt(
					attention.Generation,
					10,
				),
				"question="+terminalQuote(attention.Question),
				"answer="+terminalQuote(attention.Answer),
			),
		)
	}

	renderer.section(
		fmt.Sprintf(
			"NOTIFICATIONS count=%d truncated=%t",
			len(snapshot.Runtime.Notifications),
			snapshot.Runtime.NotificationsTruncated,
		),
	)
	if len(snapshot.Runtime.Notifications) == 0 {
		renderer.none()
	}
	for _, notification := range snapshot.Runtime.Notifications {
		renderer.line(
			2,
			fields(
				"destination="+terminalQuote(
					notification.DestinationID,
				),
				"sequence="+strconv.FormatInt(notification.Sequence, 10),
				"message="+terminalQuote(notification.MessageID),
				"source_offset="+strconv.FormatInt(
					notification.SourceEventOffset,
					10,
				),
				"state="+terminalQuote(notification.State),
				"attempts="+strconv.FormatInt(notification.Attempts, 10),
				"available_at="+terminalTime(notification.AvailableAt),
				"claimed_until="+terminalOptionalTime(
					notification.ClaimedUntil,
				),
				"delivered_at="+terminalOptionalTime(
					notification.DeliveredAt,
				),
				"error="+terminalQuote(notification.LastErrorCode),
				"created_at="+terminalTime(notification.CreatedAt),
				"updated_at="+terminalTime(notification.UpdatedAt),
			),
		)
	}

	renderer.section(
		fmt.Sprintf(
			"EVIDENCE count=%d through_offset=%d",
			len(snapshot.Evidence),
			snapshot.ThroughOffset,
		),
	)
	if len(snapshot.Evidence) == 0 {
		renderer.none()
	}
	for _, evidence := range snapshot.Evidence {
		renderer.line(
			2,
			fields(
				"offset="+strconv.FormatInt(evidence.Offset, 10),
				"kind="+terminalQuote(evidence.Kind),
				"created_at="+terminalTime(evidence.CreatedAt),
			),
		)
	}

	renderer.section(
		fmt.Sprintf("ACTIONS count=%d", len(snapshot.Actions)),
	)
	if len(snapshot.Actions) == 0 {
		renderer.none()
	}
	for _, action := range snapshot.Actions {
		actionFields := []string{
			"kind=" + terminalQuote(action.Kind),
			"expected_generation=" + strconv.FormatInt(
				action.ExpectedGeneration,
				10,
			),
		}
		if action.WorkID != "" {
			actionFields = append(
				actionFields,
				"work="+terminalQuote(action.WorkID),
			)
		}
		if action.AttentionID != "" {
			actionFields = append(
				actionFields,
				"attention="+terminalQuote(action.AttentionID),
			)
		}
		if action.ExpectedEpoch != 0 {
			actionFields = append(
				actionFields,
				"expected_epoch="+strconv.FormatInt(
					action.ExpectedEpoch,
					10,
				),
			)
		}
		if action.DestinationID != "" {
			actionFields = append(
				actionFields,
				"destination="+terminalQuote(action.DestinationID),
			)
		}
		if action.MessageID != "" {
			actionFields = append(
				actionFields,
				"message="+terminalQuote(action.MessageID),
			)
		}
		renderer.line(2, fields(actionFields...))
	}

	renderer.section(
		fmt.Sprintf("DIAGNOSTICS count=%d", len(snapshot.Diagnostics)),
	)
	if len(snapshot.Diagnostics) == 0 {
		renderer.none()
	}
	for _, diagnostic := range snapshot.Diagnostics {
		renderer.line(
			2,
			fields(
				"code="+terminalQuote(diagnostic.Code),
				"track="+terminalQuote(diagnostic.Track),
				"work="+terminalQuote(diagnostic.Work),
			),
		)
	}

	return strings.Join(renderer.lines, "\n") + "\n"
}

type terminalRenderer struct {
	width int
	lines []string
}

func (r *terminalRenderer) heading(value string) {
	r.line(0, value)
}

func (r *terminalRenderer) section(value string) {
	if len(r.lines) > 0 {
		r.lines = append(r.lines, "")
	}
	r.line(0, value)
}

func (r *terminalRenderer) none() {
	r.line(2, "(none)")
}

func (r *terminalRenderer) line(indent int, value string) {
	if value == "" {
		r.lines = append(r.lines, "")
		return
	}
	if indent < 0 {
		indent = 0
	}
	if indent >= r.width {
		indent = r.width - 1
	}
	prefix := strings.Repeat(" ", indent)
	continuationIndent := indent + 2
	if continuationIndent >= r.width {
		continuationIndent = indent
	}
	continuation := strings.Repeat(" ", continuationIndent)
	remaining := strings.TrimSpace(value)

	for remaining != "" {
		available := r.width - utf8.RuneCountInString(prefix)
		chunk, rest := splitTerminalLine(remaining, available)
		r.lines = append(r.lines, prefix+chunk)
		remaining = rest
		prefix = continuation
	}
}

func splitTerminalLine(value string, width int) (string, string) {
	if width < 1 {
		width = 1
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value, ""
	}

	cut := width
	for index := width; index > 0; index-- {
		if runes[index-1] == ' ' {
			cut = index - 1
			break
		}
	}
	if cut == 0 {
		cut = width
	}
	chunk := strings.TrimSpace(string(runes[:cut]))
	rest := strings.TrimSpace(string(runes[cut:]))
	if chunk == "" {
		return string(runes[:width]), strings.TrimSpace(string(runes[width:]))
	}
	return chunk, rest
}

func fields(values ...string) string {
	return strings.Join(values, " ")
}

func terminalQuote(value string) string {
	return strconv.QuoteToGraphic(value)
}

func terminalList(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, terminalQuote(value))
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

func terminalTime(value time.Time) string {
	if value.IsZero() {
		return "?"
	}
	return terminalQuote(value.UTC().Format(time.RFC3339Nano))
}

func terminalOptionalTime(value *time.Time) string {
	if value == nil {
		return "?"
	}
	return terminalTime(*value)
}

func terminalOptionalInt(value *int64) string {
	if value == nil {
		return "?"
	}
	return strconv.FormatInt(*value, 10)
}

func terminalOptionalString(value *string) string {
	if value == nil {
		return "?"
	}
	return terminalQuote(*value)
}
