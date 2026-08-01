package cockpit

import (
	"testing"

	"github.com/swornagent/sworn/internal/baton"
)

func TestProjectReleaseUsesBatonGraphWithoutInventingRun(t *testing.T) {
	state := baton.State{
		Release: "release-1",
		Tracks: []baton.TrackState{{
			ID: "T1",
			Slices: []*baton.SliceState{{
				Location: baton.SliceLocation{Slice: baton.Slice{ID: "S1"}},
				Status:   "ready", NextRole: "implementer",
			}},
		}},
	}

	snapshot := ProjectRelease(state)
	if len(snapshot.Graph.Nodes) != 4 ||
		!snapshot.Graph.Nodes[2].HasBaton ||
		snapshot.Graph.Nodes[2].NextResponsibility != "implementer" {
		t.Fatalf("release graph = %#v", snapshot.Graph)
	}
	if snapshot.Presentation.Status != "Ready for Implementer" ||
		snapshot.Presentation.What != "Baton has work ready to be handed over." {
		t.Fatalf("presentation = %#v", snapshot.Presentation)
	}
}

func TestProjectReleaseReportsCompleteAndDiagnostics(t *testing.T) {
	complete := ProjectRelease(baton.State{
		Release:  "complete",
		Assembly: baton.AssemblyState{Status: "complete", Outcome: "merged"},
	})
	if complete.Presentation.Status != "Complete" {
		t.Fatalf("complete presentation = %#v", complete.Presentation)
	}

	diagnostic := ProjectRelease(baton.State{
		Release: "broken",
		Diagnostics: []baton.Diagnostic{{
			Code: "INVALID_HISTORY", Track: "T1", Work: "S1",
		}},
	})
	if diagnostic.Presentation.Status != "Needs confirmation" ||
		len(diagnostic.Diagnostics) != 1 ||
		diagnostic.Diagnostics[0].Code != "INVALID_HISTORY" {
		t.Fatalf("diagnostic presentation = %#v", diagnostic)
	}
}
