package cockpit

import (
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"
)

func TestRenderTerminalPresentsTheTruthfulSnapshot(t *testing.T) {
	t.Parallel()

	snapshot := terminalFixture()
	first := RenderTerminal(snapshot)
	second := RenderTerminal(snapshot)
	if first != second {
		t.Fatal("terminal rendering is not deterministic")
	}

	for _, fact := range []string{
		`id="run-1" release="release-1"`,
		`state="running" desired="running" outcome=""`,
		`control_generation=4 through_offset=17`,
		`GRAPH nodes=3 edges=2`,
		`id="slice:S1" label="S1" state="ready"`,
		`baton=true`,
		`next="implementer"`,
		`kind="depends_on" to="assembly:release-1"`,
		`HANDOFF`,
		`ready=true nodes=["slice:S1"]`,
		`responsibilities=["implementer"]`,
		`present=true active=true generation=2`,
		`EFFECTS count=1`,
		`ATTEMPTS count=2`,
		`NOTIFICATIONS count=1 truncated=true`,
		`EVIDENCE count=1 through_offset=17`,
		`ACTIONS count=2`,
		`DIAGNOSTICS count=1`,
	} {
		if !strings.Contains(first, fact) {
			t.Errorf("terminal output does not contain %q:\n%s", fact, first)
		}
	}
	if strings.Contains(first, `outcome="pass"`) ||
		strings.Contains(first, "100%") {
		t.Fatalf("terminal output invented progress:\n%s", first)
	}
}

func TestRenderTerminalDistinguishesUnknownUsageFromReportedZero(
	t *testing.T,
) {
	t.Parallel()

	output := RenderTerminalWidth(terminalFixture(), 500)
	if !strings.Contains(
		output,
		`effect="effect-1" number=1 responsibility="implementer_implementation" transport="cli" input_tokens=0 output_tokens=0 cost_micro_units=0 currency="AUD"`,
	) {
		t.Fatalf("reported zero usage is not explicit:\n%s", output)
	}
	if !strings.Contains(
		output,
		`effect="effect-2" number=2 responsibility="work_verification" transport="api" input_tokens=? output_tokens=? cost_micro_units=? currency=?`,
	) {
		t.Fatalf("unknown usage is not explicit:\n%s", output)
	}
}

func TestRenderTerminalHonoursNormalAndNarrowWidths(t *testing.T) {
	t.Parallel()

	for _, width := range []int{80, 36} {
		output := RenderTerminalWidth(terminalFixture(), width)
		for number, line := range strings.Split(
			strings.TrimSuffix(output, "\n"),
			"\n",
		) {
			if got := utf8.RuneCountInString(line); got > width {
				t.Errorf(
					"width %d line %d has %d runes: %q",
					width,
					number+1,
					got,
					line,
				)
			}
		}
		for _, fact := range []string{
			"SWORN COCKPIT",
			"GRAPH nodes=3 edges=2",
			"HANDOFF",
			"NOTIFICATIONS count=1",
			"DIAGNOSTICS count=1",
		} {
			if !strings.Contains(output, fact) {
				t.Errorf(
					"width %d lost section %q:\n%s",
					width,
					fact,
					output,
				)
			}
		}
	}
}

func TestRenderTerminalQuotesHostileStrings(t *testing.T) {
	t.Parallel()

	snapshot := terminalFixture()
	snapshot.Run.ID = "run\x1b]8;;https://example.invalid\a\nnext"
	snapshot.Graph.Nodes[0].Label = "label\r\t\u009b31m\u202espoof"
	snapshot.Diagnostics[0].Work = "work\x00end"
	output := RenderTerminal(snapshot)

	for _, escaped := range []string{
		`\x1b`,
		`\a`,
		`\n`,
		`\r`,
		`\t`,
		`\u009b`,
		`\u202e`,
		`\x00`,
	} {
		if !strings.Contains(output, escaped) {
			t.Errorf("terminal output did not expose %q safely:\n%s", escaped, output)
		}
	}
	for _, value := range output {
		if value == '\n' {
			continue
		}
		if unicode.IsControl(value) || unicode.Is(unicode.Cf, value) {
			t.Fatalf(
				"terminal output contains raw control/format rune %U",
				value,
			)
		}
	}
}

func TestRenderTerminalHandlesAnEmptySnapshot(t *testing.T) {
	t.Parallel()

	output := RenderTerminal(Snapshot{})
	for _, section := range []string{
		"GRAPH nodes=0 edges=0",
		"EFFECTS count=0",
		"ATTEMPTS count=0",
		"NOTIFICATIONS count=0 truncated=false",
		"EVIDENCE count=0 through_offset=0",
		"ACTIONS count=0",
		"DIAGNOSTICS count=0",
	} {
		if !strings.Contains(output, section) {
			t.Errorf("empty output lost section %q:\n%s", section, output)
		}
	}
}

func terminalFixture() Snapshot {
	now := time.Date(2026, 7, 29, 1, 2, 3, 4, time.FixedZone("AEST", 10*60*60))
	expires := now.Add(time.Minute)
	delivered := now.Add(2 * time.Minute)
	zero := int64(0)
	currency := "AUD"

	return Snapshot{
		SchemaVersion: SnapshotSchemaVersion,
		Run: RunView{
			ID:                "run-1",
			Release:           "release-1",
			State:             "running",
			DesiredState:      "running",
			ControlGeneration: 4,
			ManifestDigest:    "sha256:manifest",
			PlanDigest:        "sha256:plan",
			TargetRef:         "refs/heads/main",
			TargetHead:        "target-head",
			ReleaseHead:       "release-head",
		},
		Graph: Graph{
			Nodes: []Node{
				{
					ID: "release:release-1", Kind: "release",
					Label: "release-1", State: "running",
				},
				{
					ID: "slice:S1", Kind: "slice", Label: "S1",
					Track: "T1", State: "ready", Stage: "implementation",
					Outcome: "none", NextResponsibility: "implementer",
					Attempt: 2, HasBaton: true,
				},
				{
					ID: "assembly:release-1", Kind: "assembly",
					Label: "Assembly", State: "waiting", Stage: "verify",
					Outcome: "none",
				},
			},
			Edges: []Edge{
				{
					ID:   "edge:contains:release:release-1:slice:S1",
					From: "release:release-1", To: "slice:S1",
					Kind: "contains",
				},
				{
					ID:   "edge:depends_on:slice:S1:assembly:release-1",
					From: "slice:S1", To: "assembly:release-1",
					Kind: "depends_on",
				},
			},
		},
		Handoff: Handoff{
			Ready:            true,
			Nodes:            []string{"slice:S1"},
			Responsibilities: []string{"implementer"},
		},
		Runtime: RuntimeView{
			Owner: OwnerView{
				Present: true, Active: true, Generation: 2,
				ExpiresAt: expires,
			},
			Effects: []EffectView{{
				ID: "effect-1", Kind: "dispatch", State: "complete",
			}},
			Attempts: []AttemptView{
				{
					EffectID:       "effect-1",
					Number:         1,
					Responsibility: "implementer_implementation",
					Transport:      "cli",
					InputTokens:    &zero,
					OutputTokens:   &zero,
					CostMicroUnits: &zero,
					Currency:       &currency,
					CreatedAt:      now,
				},
				{
					EffectID:       "effect-2",
					Number:         2,
					Responsibility: "work_verification",
					Transport:      "api",
					CreatedAt:      now.Add(time.Second),
				},
			},
			Notifications: []NotificationView{{
				DestinationID: "audit", SourceEventOffset: 16,
				Sequence: 3, MessageID: "message-3", State: "delivered",
				Attempts: 2, AvailableAt: now, DeliveredAt: &delivered,
				CreatedAt: now, UpdatedAt: delivered,
			}},
			NotificationsTruncated: true,
		},
		Evidence: []Evidence{{
			Offset: 17, Kind: "dispatch_completed", CreatedAt: now,
		}},
		Actions: []Action{
			{Kind: "pause", ExpectedGeneration: 4},
			{
				Kind: "redeliver", DestinationID: "audit",
				MessageID: "message-3",
			},
		},
		Diagnostics: []Diagnostic{{
			Code: "OUTBOX_TRUNCATED", Track: "T1", Work: "S1",
		}},
		ThroughOffset: 17,
	}
}
