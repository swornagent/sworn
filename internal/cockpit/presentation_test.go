package cockpit

import (
	"strings"
	"testing"

	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

func TestPresentRunStateExplainsEveryRecordedState(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"new":               "Ready to start",
		"running":           "Sworn is working",
		"awaiting_approval": "Waiting for approval",
		"pausing":           "Pausing safely",
		"paused":            "Paused",
		"cancelling":        "Cancelling safely",
		"cancelled":         "Cancelled",
		"parked":            "Stopped and needs your attention",
		"takeover_required": "Resume required",
		"uncertain":         "Needs confirmation",
		"complete":          "Complete",
	}
	for state, want := range tests {
		presentation := PresentRunState(state)
		if presentation.Status != want ||
			presentation.What == "" ||
			presentation.Next == "" ||
			presentation.NeedsYou == "" ||
			presentation.Checked == "" {
			t.Errorf("PresentRunState(%q) = %#v", state, presentation)
		}
	}

	unknown := PresentRunState("future_state")
	if unknown.Status != "Status unavailable" {
		t.Fatalf("unknown presentation = %#v", unknown)
	}
}

func TestPresentSnapshotPrioritisesUnconfirmedFactsAndHumanAttention(
	t *testing.T,
) {
	t.Parallel()

	attention := Snapshot{
		Run: RunView{State: "parked"},
		Runtime: RuntimeView{Attentions: []AttentionView{{
			State: "open",
		}}},
	}
	if got := PresentSnapshot(attention); got.Status != "Waiting for your answer" {
		t.Fatalf("attention presentation = %#v", got)
	}

	attention.Diagnostics = []Diagnostic{{Code: "BATON_UNAVAILABLE"}}
	if got := PresentSnapshot(attention); got.Status != "Needs confirmation" {
		t.Fatalf("diagnostic presentation = %#v", got)
	}

	retry := Snapshot{
		Run:     RunView{State: "parked"},
		Actions: []Action{{Kind: "retry"}},
	}
	if got := PresentSnapshot(retry); got.Status != "Stopped after repeated failures" {
		t.Fatalf("retry presentation = %#v", got)
	}
}

// A4: the board names a degradation park instead of the flat parked text.
func TestPresentRunStateNamesDegradationPark(t *testing.T) {
	t.Parallel()

	degradation := PresentRunState("parked", &runtimepkg.ParkStatus{
		Cause:         runtimepkg.ParkCauseDegradation,
		FallbackCount: 4,
		Budget:        3,
		UnblockKnob:   runtimepkg.DegradationUnblockKnob,
	})
	if degradation.Status != "Stopped after repeated context loss" {
		t.Fatalf("status = %q, want degradation status", degradation.Status)
	}
	if !strings.Contains(degradation.What, "4 times") ||
		!strings.Contains(degradation.What, "budget of 3") {
		t.Fatalf("what = %q, want count and budget", degradation.What)
	}
	if !strings.Contains(degradation.Next, "limits.degradation_budget") {
		t.Fatalf("next = %q, want the unblock knob", degradation.Next)
	}
	if !strings.Contains(degradation.NeedsYou, "repeated context rebuilds") {
		t.Fatalf("needs_you = %q, want the rebuild review", degradation.NeedsYou)
	}

	// A nil park keeps the existing flat wording.
	flat := PresentRunState("parked")
	if flat.Status != "Stopped and needs your attention" {
		t.Fatalf("nil-park status = %q, want the flat parked status", flat.Status)
	}

	// A non-degradation park keeps the existing flat wording too.
	attention := PresentRunState("parked", &runtimepkg.ParkStatus{
		Cause: runtimepkg.ParkCauseAttention,
	})
	if attention.Status != "Stopped and needs your attention" {
		t.Fatalf("attention-park status = %q, want the flat parked status", attention.Status)
	}

	// The board snapshot carries the park through PresentSnapshot.
	snapshot := PresentSnapshot(Snapshot{
		Run: RunView{
			State: "parked",
			Park: &runtimepkg.ParkStatus{
				Cause:         runtimepkg.ParkCauseDegradation,
				FallbackCount: 4,
				Budget:        3,
				UnblockKnob:   runtimepkg.DegradationUnblockKnob,
			},
		},
	})
	if snapshot.Status != "Stopped after repeated context loss" {
		t.Fatalf("snapshot presentation = %#v", snapshot)
	}
}

func TestBoardRenderingForUnexpiredOwnerRendersRunningWithoutTakeoverHint(t *testing.T) {
	t.Parallel()

	snapshot := Snapshot{
		Run: RunView{
			ID:    "run-unexpired",
			State: "running",
		},
		Runtime: RuntimeView{
			Owner: OwnerView{
				Present: true,
				Active:  true,
			},
		},
	}

	presentation := PresentSnapshot(snapshot)
	if presentation.Status != "Sworn is working" {
		t.Fatalf("presentation.Status = %q, want %q", presentation.Status, "Sworn is working")
	}
	if presentation.NeedsYou != "No, unless Sworn asks a question." {
		t.Fatalf("presentation.NeedsYou = %q", presentation.NeedsYou)
	}
	if presentation.Next == "Take over the run so Sworn can recheck it and continue." {
		t.Fatalf("unexpired owner rendered takeover hint in Next: %q", presentation.Next)
	}
}

func TestBoardRenderingForExpiredOwnerRendersTakeoverRequiredWithHint(t *testing.T) {
	t.Parallel()

	snapshot := Snapshot{
		Run: RunView{
			ID:    "run-expired",
			State: "takeover_required",
		},
		Runtime: RuntimeView{
			Owner: OwnerView{
				Present: true,
				Active:  false,
			},
		},
	}

	presentation := PresentSnapshot(snapshot)
	if presentation.Status != "Resume required" {
		t.Fatalf("presentation.Status = %q, want %q", presentation.Status, "Resume required")
	}
	if presentation.NeedsYou != "Yes — take over the run." {
		t.Fatalf("presentation.NeedsYou = %q, want %q", presentation.NeedsYou, "Yes — take over the run.")
	}
	if presentation.Next != "Take over the run so Sworn can recheck it and continue." {
		t.Fatalf("presentation.Next = %q, want %q", presentation.Next, "Take over the run so Sworn can recheck it and continue.")
	}
}

// A3: the uncertain presentation names the verb the control gate admits,
// never the retired "Recover the run" text.
func TestPresentRunStateWithRecoveryNamesAdmissibleVerb(t *testing.T) {
	t.Parallel()

	recovery := func(action string) *runtimepkg.RecoveryAction {
		return &runtimepkg.RecoveryAction{
			Action: action,
			Reason: "fixture reason",
		}
	}
	for _, test := range []struct {
		action string
		want   string
	}{
		{"retry", "Retry the unresolved work item so Sworn can recheck it."},
		{"takeover", "Take over the run so Sworn can recheck it and continue."},
		{"resume", "Wait for the dispatch lease to expire, then resume the run so Sworn can recheck it."},
	} {
		presentation := PresentRunStateWithRecovery(
			"uncertain",
			recovery(test.action),
		)
		if presentation.Next != test.want {
			t.Fatalf(
				"recovery %q next = %q, want %q",
				test.action,
				presentation.Next,
				test.want,
			)
		}
		if strings.Contains(presentation.Next, "Recover the run") {
			t.Fatalf("retired recovery text returned: %q", presentation.Next)
		}
	}

	flat := PresentRunState("uncertain")
	if strings.Contains(flat.Next, "Recover the run") {
		t.Fatalf("flat uncertain text names the retired verb: %q", flat.Next)
	}
}
