package cockpit

import (
	"fmt"
	"strings"

	"github.com/swornagent/sworn/internal/baton"
	"github.com/swornagent/sworn/internal/journal"
	runtimepkg "github.com/swornagent/sworn/internal/runtime"
)

// ReleaseSnapshot is the Baton view shown before a Sworn run exists.
type ReleaseSnapshot struct {
	Graph        Graph
	Diagnostics  []Diagnostic
	Presentation RunPresentation
}

// DiscoveredRunStatus holds the status and attention state of one discovered run.
type DiscoveredRunStatus struct {
	Binding    journal.Run
	Status     runtimepkg.RunStatus
	Attentions []journal.AttentionProjection
}

// ProjectReleaseInfo holds release metadata for catalog projection.
type ProjectReleaseInfo struct {
	Name       string
	SourceRef  string
	State      string
	Status     string
	Diagnostic string
}

// ReleaseStateAndStatus returns the lifecycle state and presentation status for a Baton release.
func ReleaseStateAndStatus(state baton.State) (string, string) {
	snapshot := ProjectRelease(state)
	if len(snapshot.Diagnostics) > 0 {
		return "needs_confirmation", snapshot.Presentation.Status
	}
	if state.Assembly.Status == "complete" && state.Assembly.Outcome == "merged" {
		return "complete", snapshot.Presentation.Status
	}
	if state.Assembly.Status == "blocked" {
		return "blocked", snapshot.Presentation.Status
	}
	handoff := projectHandoff(snapshot.Graph)
	if handoff.Ready {
		return "ready", snapshot.Presentation.Status
	}
	return "not_started", snapshot.Presentation.Status
}

// BuildProjectCatalog builds a ProjectCatalog from release info, discovered runs, and diagnostics.
func BuildProjectCatalog(
	releases []ProjectReleaseInfo,
	runs []DiscoveredRunStatus,
	diagnostics []Diagnostic,
) ProjectCatalog {
	catalogReleases := make([]CatalogRelease, 0, len(releases))
	for _, rel := range releases {
		catalogReleases = append(catalogReleases, CatalogRelease{
			Name:       rel.Name,
			SourceRef:  rel.SourceRef,
			State:      rel.State,
			Status:     rel.Status,
			Diagnostic: rel.Diagnostic,
		})
	}
	catalogRuns := make([]CatalogRun, 0, len(runs))
	for _, run := range runs {
		presentation := PresentRunStateWithRecovery(
			run.Status.State,
			run.Status.Recovery,
			run.Status.Park,
		)
		catalogRuns = append(catalogRuns, CatalogRun{
			ID:        run.Binding.ID,
			Release:   run.Binding.Release,
			State:     run.Status.State,
			Status:    presentation.Status,
			What:      presentation.What,
			Next:      presentation.Next,
			NeedsYou:  presentation.NeedsYou,
			CreatedAt: run.Binding.CreatedAt,
		})
	}
	needsYou := ProjectNeedsYou(runs)
	return ProjectCatalog{
		SchemaVersion: CatalogSchemaVersion,
		Releases:      catalogReleases,
		Runs:          catalogRuns,
		NeedsYou:      needsYou,
		Diagnostics:   diagnostics,
	}
}

// ProjectNeedsYou extracts human-action-required items across runs with precedence:
// 1. Open attentions ("answer_attention")
// 2. Reconciled uncertainty (the one verb the control gate admits)
// 3. Parked runs (retry for real failed work, review_park otherwise)
// 4. Takeover required ("takeover")
// 5. Awaiting approval ("approve")
func ProjectNeedsYou(runs []DiscoveredRunStatus) []NeedsYouItem {
	items := make([]NeedsYouItem, 0)
	for _, run := range runs {
		// Precedence 1: Open attention turns
		var openAttention *journal.AttentionProjection
		for index := range run.Attentions {
			if run.Attentions[index].State == journal.AttentionOpen {
				openAttention = &run.Attentions[index]
				break
			}
		}
		if openAttention != nil {
			items = append(items, NeedsYouItem{
				RunID:       run.Binding.ID,
				Release:     run.Binding.Release,
				State:       run.Status.State,
				Action:      "answer_attention",
				Reason:      openAttention.Question,
				AttentionID: openAttention.Attention.ID,
			})
			continue
		}

		// Precedence 2: Reconciled uncertainty. The runtime derived the one
		// verb the control gate admits for this shape; the needs-you row
		// carries exactly that verb and its work identity, never a verb
		// ApplyControl would refuse.
		if run.Status.Recovery != nil {
			items = append(items, NeedsYouItem{
				RunID:   run.Binding.ID,
				Release: run.Binding.Release,
				State:   run.Status.State,
				Action:  run.Status.Recovery.Action,
				Reason:  run.Status.Recovery.Reason,
				WorkID:  run.Status.Recovery.WorkID,
			})
			continue
		}

		// Precedence 3: Parked run. A park with real failed work keeps the
		// retry row with its work id; a park with no failed work is never
		// presented as a retry. A degradation park names its cause, count,
		// budget, and the manifest knob that unblocks it.
		if run.Status.State == "parked" {
			workID := ""
			for _, effect := range run.Status.Effects {
				if work, _, ok := exhaustedAttempt(effect); ok {
					workID = work
					break
				}
			}
			if workID != "" {
				items = append(items, NeedsYouItem{
					RunID:   run.Binding.ID,
					Release: run.Binding.Release,
					State:   "parked",
					Action:  "retry",
					Reason:  "Review the failed item, then retry it using the latest action.",
					WorkID:  workID,
				})
				continue
			}
			reason := "Review the stopped run before continuing."
			if run.Status.Park != nil &&
				run.Status.Park.Cause == runtimepkg.ParkCauseDegradation {
				reason = fmt.Sprintf(
					"Sworn rebuilt the model context %d times against a degradation budget of %d. Raise %s in the manifest, then resume the run.",
					run.Status.Park.FallbackCount,
					run.Status.Park.Budget,
					run.Status.Park.UnblockKnob,
				)
			}
			items = append(items, NeedsYouItem{
				RunID:   run.Binding.ID,
				Release: run.Binding.Release,
				State:   "parked",
				Action:  "review_park",
				Reason:  reason,
			})
			continue
		}

		// Precedence 4: Takeover required
		if run.Status.State == "takeover_required" {
			items = append(items, NeedsYouItem{
				RunID:   run.Binding.ID,
				Release: run.Binding.Release,
				State:   "takeover_required",
				Action:  "takeover",
				Reason:  "Take over the run so Sworn can recheck it and continue.",
			})
			continue
		}

		// Precedence 5: Awaiting approval
		if run.Status.State == "awaiting_approval" {
			items = append(items, NeedsYouItem{
				RunID:   run.Binding.ID,
				Release: run.Binding.Release,
				State:   "awaiting_approval",
				Action:  "approve",
				Reason:  "Authorize the exact plan through the configured external authorizer.",
			})
			continue
		}
	}
	return items
}

// ProjectRelease builds the same Baton graph used by a live run without
// inventing a journal, run ID, runtime state, or eligible control.
func ProjectRelease(state baton.State) ReleaseSnapshot {
	graph := projectGraph(state, "not_started", nil)
	handoff := projectHandoff(graph)
	diagnostics := make([]Diagnostic, 0, len(state.Diagnostics))
	for _, diagnostic := range state.Diagnostics {
		diagnostics = append(diagnostics, Diagnostic{
			Code:  diagnostic.Code,
			Track: diagnostic.Track,
			Work:  diagnostic.Work,
		})
	}
	return ReleaseSnapshot{
		Graph:        graph,
		Diagnostics:  diagnostics,
		Presentation: presentRelease(state, handoff, diagnostics),
	}
}

func presentRelease(
	state baton.State,
	handoff Handoff,
	diagnostics []Diagnostic,
) RunPresentation {
	presentation := RunPresentation{
		Status:   "Ready for Sworn",
		What:     "Sworn has recorded this release and its next step.",
		Next:     "Start Sworn when the run setup and AI connections are ready.",
		NeedsYou: "Only if you want to start or change the release.",
		Checked:  "The latest saved Sworn release.",
	}
	if len(diagnostics) > 0 {
		presentation.Status = "Needs confirmation"
		presentation.What = "Sworn found a problem with this release."
		presentation.Next = "Review the saved release before starting more work."
		presentation.NeedsYou = "Yes — review the release."
		return presentation
	}
	if state.Assembly.Status == "complete" &&
		state.Assembly.Outcome == "merged" {
		presentation.Status = "Complete"
		presentation.What = "Sworn's records show that this release was merged."
		presentation.Next = "No delivery work remains for this release."
		presentation.NeedsYou = "No."
		return presentation
	}
	if state.Assembly.Status == "blocked" {
		presentation.Status = "Stopped and needs your attention"
		presentation.What = "Part of this release cannot continue."
		presentation.Next = "Review the blocked work before continuing."
		presentation.NeedsYou = "Yes — review the blocked work."
		return presentation
	}
	if handoff.Ready {
		roles := make([]string, 0, len(handoff.Responsibilities))
		for _, role := range handoff.Responsibilities {
			roles = append(roles, humanRole(role))
		}
		presentation.Status = "Ready for " + strings.Join(roles, " and ")
		presentation.What = "Sworn has work ready to be handed over."
		presentation.Next = "Start Sworn to carry the work forward."
		presentation.NeedsYou = "Only if the next step asks for approval or judgment."
		return presentation
	}
	presentation.Status = "Waiting"
	presentation.What = "No work is ready to hand over yet."
	presentation.Next = "Finish the step this release is waiting for."
	presentation.NeedsYou = "Only if the release is waiting on you."
	return presentation
}

func humanRole(role string) string {
	if role == "" {
		return ""
	}
	return strings.ToUpper(role[:1]) + role[1:]
}
