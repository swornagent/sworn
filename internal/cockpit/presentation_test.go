package cockpit

import "testing"

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
