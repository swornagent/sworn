package cockpit

import (
	"strings"
	"testing"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
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
		snapshot.Presentation.What != "Sworn has work ready to be handed over." {
		t.Fatalf("presentation = %#v", snapshot.Presentation)
	}
}

func TestProjectReleasePresentationNeverNamesBatonAsActiveAuthority(t *testing.T) {
	for _, state := range []baton.State{
		{Release: "not-started"},
		{Release: "merged", Assembly: baton.AssemblyState{Status: "complete", Outcome: "merged"}},
		{Release: "blocked", Assembly: baton.AssemblyState{Status: "blocked"}},
		{
			Release: "ready",
			Tracks: []baton.TrackState{{
				ID: "T1",
				Slices: []*baton.SliceState{{
					Location: baton.SliceLocation{Slice: baton.Slice{ID: "S1"}},
					Status:   "ready", NextRole: "implementer",
				}},
			}},
		},
		{
			Release: "broken",
			Diagnostics: []baton.Diagnostic{{
				Code: "INVALID_HISTORY", Track: "T1", Work: "S1",
			}},
		},
	} {
		presentation := ProjectRelease(state).Presentation
		for _, field := range []string{
			presentation.Status, presentation.What,
			presentation.Next, presentation.NeedsYou, presentation.Checked,
		} {
			if strings.Contains(field, "Baton") {
				t.Fatalf(
					"release %q presentation names Baton as active authority: %#v",
					state.Release, presentation,
				)
			}
		}
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

func TestProjectNeedsYouSurfacesParkedAndAttentionRuns(t *testing.T) {
	t.Parallel()

	runs := []DiscoveredRunStatus{
		{
			Binding: journal.Run{
				ID:      "run-healthy",
				Release: "release-healthy",
			},
			Status: runtimepkg.RunStatus{
				RunID: "run-healthy",
				State: "running",
			},
		},
		{
			Binding: journal.Run{
				ID:      "run-parked",
				Release: "release-parked",
			},
			Status: runtimepkg.RunStatus{
				RunID: "run-parked",
				State: "parked",
				Effects: []runtimepkg.EffectStatus{
					{
						ID:    "attempt/" + strings.Repeat("a", 64) + "/e1/t3",
						Kind:  "driver.dispatch",
						State: string(journal.OperationalFailed),
					},
				},
			},
		},
		{
			Binding: journal.Run{
				ID:      "run-attention",
				Release: "release-attention",
			},
			Status: runtimepkg.RunStatus{
				RunID: "run-attention",
				State: "parked",
			},
			Attentions: []journal.AttentionProjection{
				{
					State:    journal.AttentionOpen,
					Question: "Which approach is preferred?",
					Attention: journal.AttentionBinding{
						ID: "att-1",
					},
				},
			},
		},
	}

	needsYou := ProjectNeedsYou(runs)
	if len(needsYou) != 2 {
		t.Fatalf("needsYou len = %d, want 2: %#v", len(needsYou), needsYou)
	}

	parkedItem := needsYou[0]
	if parkedItem.RunID != "run-parked" ||
		parkedItem.Action != "retry" ||
		parkedItem.State != "parked" ||
		parkedItem.WorkID != "sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("parkedItem = %#v", parkedItem)
	}

	attentionItem := needsYou[1]
	if attentionItem.RunID != "run-attention" ||
		attentionItem.Action != "answer_attention" ||
		attentionItem.AttentionID != "att-1" ||
		attentionItem.Reason != "Which approach is preferred?" {
		t.Fatalf("attentionItem = %#v", attentionItem)
	}

	// Test precedence: a single run that is both parked and has an open attention
	// produces exactly 1 row where answer_attention takes precedence over retry.
	both := []DiscoveredRunStatus{
		{
			Binding: journal.Run{
				ID:      "run-both",
				Release: "release-both",
			},
			Status: runtimepkg.RunStatus{
				RunID: "run-both",
				State: "parked",
				Effects: []runtimepkg.EffectStatus{
					{
						ID:    "attempt/" + strings.Repeat("b", 64) + "/e1/t3",
						Kind:  "driver.dispatch",
						State: string(journal.OperationalFailed),
					},
				},
			},
			Attentions: []journal.AttentionProjection{
				{
					State:    journal.AttentionOpen,
					Question: "Resolve attention first?",
					Attention: journal.AttentionBinding{
						ID: "att-both",
					},
				},
			},
		},
	}
	bothNeedsYou := ProjectNeedsYou(both)
	if len(bothNeedsYou) != 1 || bothNeedsYou[0].Action != "answer_attention" || bothNeedsYou[0].AttentionID != "att-both" {
		t.Fatalf("precedence bothNeedsYou = %#v", bothNeedsYou)
	}
}

// A4: the needs-you catalog names a degradation park and never presents a
// park without failed work as a retry with an empty work id.
func TestProjectNeedsYouNamesDegradationPark(t *testing.T) {
	t.Parallel()

	degradation := []DiscoveredRunStatus{
		{
			Binding: journal.Run{
				ID:      "run-degradation",
				Release: "release-degradation",
			},
			Status: runtimepkg.RunStatus{
				RunID: "run-degradation",
				State: "parked",
				Park: &runtimepkg.ParkStatus{
					Cause:         runtimepkg.ParkCauseDegradation,
					FallbackCount: 4,
					Budget:        3,
					UnblockKnob:   runtimepkg.DegradationUnblockKnob,
				},
			},
		},
	}
	needsYou := ProjectNeedsYou(degradation)
	if len(needsYou) != 1 {
		t.Fatalf("needsYou = %#v", needsYou)
	}
	item := needsYou[0]
	if item.Action != "review_park" ||
		item.WorkID != "" ||
		item.State != "parked" ||
		!strings.Contains(item.Reason, "4 times") ||
		!strings.Contains(item.Reason, "budget of 3") ||
		!strings.Contains(item.Reason, "limits.degradation_budget") {
		t.Fatalf("degradation needs-you item = %#v", item)
	}

	// A park with no failed work and no degradation fact is still never
	// presented as a retry with an empty work id.
	other := []DiscoveredRunStatus{
		{
			Binding: journal.Run{
				ID:      "run-other",
				Release: "release-other",
			},
			Status: runtimepkg.RunStatus{
				RunID: "run-other",
				State: "parked",
				Park: &runtimepkg.ParkStatus{
					Cause: runtimepkg.ParkCauseHumanAuthority,
				},
			},
		},
	}
	otherNeedsYou := ProjectNeedsYou(other)
	if len(otherNeedsYou) != 1 ||
		otherNeedsYou[0].Action != "review_park" ||
		otherNeedsYou[0].WorkID != "" {
		t.Fatalf("other park needs-you = %#v", otherNeedsYou)
	}
}

func TestBuildProjectCatalogIncludesReleasesRunsAndNeedsYou(t *testing.T) {
	t.Parallel()

	releases := []ProjectReleaseInfo{
		{
			Name:      "release-1",
			SourceRef: "refs/heads/release-wt/release-1",
			State:     "ready",
			Status:    "Ready for Implementer",
		},
		{
			Name:   "release-2",
			State:  "complete",
			Status: "Complete",
		},
	}
	runs := []DiscoveredRunStatus{
		{
			Binding: journal.Run{
				ID:      "run-1",
				Release: "release-1",
			},
			Status: runtimepkg.RunStatus{
				RunID: "run-1",
				State: "running",
			},
		},
	}
	catalog := BuildProjectCatalog(releases, runs, nil)
	if catalog.SchemaVersion != CatalogSchemaVersion {
		t.Fatalf("schema version = %q, want %q", catalog.SchemaVersion, CatalogSchemaVersion)
	}
	if len(catalog.Releases) != 2 || catalog.Releases[0].Name != "release-1" || catalog.Releases[1].State != "complete" {
		t.Fatalf("catalog releases = %#v", catalog.Releases)
	}
	if len(catalog.Runs) != 1 || catalog.Runs[0].ID != "run-1" || catalog.Runs[0].State != "running" {
		t.Fatalf("catalog runs = %#v", catalog.Runs)
	}
	if len(catalog.NeedsYou) != 0 {
		t.Fatalf("catalog needs you = %#v, want 0", catalog.NeedsYou)
	}
}
